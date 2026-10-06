package main

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
)

// msToText 将毫秒格式化为 m:ss.CC（游泳计时标准显示：百分之一秒）
func msToText(ms int64) string {
	if ms < 0 {
		return "-"
	}
	cs := ms / 10
	hundredths := cs % 100
	totalSec := cs / 100
	seconds := totalSec % 60
	minutes := totalSec / 60
	if minutes > 0 {
		return fmt.Sprintf("%d:%02d.%02d", minutes, seconds, hundredths)
	}
	return fmt.Sprintf("%d.%02d", seconds, hundredths)
}

// effectiveResult 一条泳道经规则计算后的当前成绩
type effectiveResult struct {
	Lane        Lane
	Reading     *TimingReading // 触板成绩（若存在）
	Case        *ReviewCase    // 复核案件（若存在）
	TimeMS      *int64
	Source      string // electronic / manual / swimoff / ""
	State       string // ranked / pending / withdrawn
	Decided     bool
}

// computeResults 汇总项目下每条泳道的电子读数、手记与裁定，得到当前成绩
func computeResults(db *gorm.DB, eventID uint) ([]effectiveResult, error) {
	var lanes []Lane
	if err := db.Where("event_id = ?", eventID).Order("lane_no").Find(&lanes).Error; err != nil {
		return nil, err
	}
	out := make([]effectiveResult, 0, len(lanes))
	for _, l := range lanes {
		r := effectiveResult{Lane: l, State: "pending"}

		// 触板成绩：split_index=0 且 source=pad，取最新一条（历史读数均保留）
		var rd TimingReading
		err := db.Where("lane_id = ? AND split_index = 0 AND source = 'pad' AND missed = ?",
			l.ID, false).Order("recorded_at desc, id desc").First(&rd).Error
		if err == nil {
			r.Reading = &rd
		}

		var rc ReviewCase
		err = db.Where("event_id = ? AND lane_id = ?", eventID, l.ID).Order("id desc").First(&rc).Error
		if err == nil {
			r.Case = &rc
		}

		switch {
		case rc.Status == "decided" && rc.Decision == "withdraw":
			r.State = "withdrawn"
			r.Decided = true
		case rc.Status == "decided" && rc.DecidedTimeMS != nil:
			r.TimeMS = rc.DecidedTimeMS
			r.Source = rc.Decision // electronic / manual / swimoff
			r.State = "ranked"
			r.Decided = true
		case rc.Status == "decided":
			r.State = "pending" // 裁定信息不完整
		case r.Reading != nil:
			r.TimeMS = r.Reading.TimeMS
			r.Source = "electronic"
			r.State = "ranked" // 未经裁定的电子成绩（临时状态，发布榜单后才对外公示）
		default:
			r.State = "pending" // 漏记且尚未裁定
		}
		out = append(out, r)
	}
	return out, nil
}

// rankedEntry 排序后的榜单行
type rankedEntry struct {
	Result      effectiveResult
	Rank        int
	DisplayRank string
	Special     string // tie / swimoff / withdrawn / ""
	Corrected   bool
}

// rankResults 按成绩排序并处理并列：同成绩并列同名次，后续名次顺延（1,1,3）
func rankResults(results []effectiveResult) []rankedEntry {
	ranked := make([]effectiveResult, 0)
	var withdrawn, pending []effectiveResult
	for _, r := range results {
		switch r.State {
		case "ranked":
			ranked = append(ranked, r)
		case "withdrawn":
			withdrawn = append(withdrawn, r)
		default:
			pending = append(pending, r)
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return *ranked[i].TimeMS < *ranked[j].TimeMS
	})

	entries := []rankedEntry{}
	pos := 1
	for i := 0; i < len(ranked); {
		j := i
		for j+1 < len(ranked) && *ranked[j+1].TimeMS == *ranked[i].TimeMS {
			j++
		}
		tied := j > i
		for k := i; k <= j; k++ {
			disp := fmt.Sprintf("%d", pos)
			special := ""
			if tied {
				disp = fmt.Sprintf("%d=", pos)
				special = "tie"
			}
			if ranked[k].Source == "swimoff" {
				special = "swimoff"
			}
			entries = append(entries, rankedEntry{
				Result: ranked[k], Rank: pos, DisplayRank: disp, Special: special,
			})
		}
		pos += (j - i + 1)
		i = j + 1
	}

	// 待裁定（漏记/争议未决）
	for _, r := range pending {
		entries = append(entries, rankedEntry{Result: r, Rank: 0, DisplayRank: "待裁定", Special: ""})
	}
	// 撤回
	for _, r := range withdrawn {
		entries = append(entries, rankedEntry{Result: r, Rank: 0, DisplayRank: "撤回", Special: "withdrawn"})
	}
	return entries
}

// buildRankingEntries 转为可持久化的榜单条目（尚未设置 BoardID / Corrected）
func buildRankingEntries(eventID uint, ranked []rankedEntry) []RankingEntry {
	entries := make([]RankingEntry, 0, len(ranked))
	for _, e := range ranked {
		ent := RankingEntry{
			EventID:      eventID,
			Rank:         e.Rank,
			DisplayRank:  e.DisplayRank,
			LaneID:       e.Result.Lane.ID,
			LaneNo:       e.Result.Lane.LaneNo,
			SwimmerName:  e.Result.Lane.SwimmerName,
			Team:         e.Result.Lane.Team,
			SpecialState: e.Special,
			Source:       e.Result.Source,
		}
		if e.Result.TimeMS != nil {
			t := *e.Result.TimeMS
			ent.TimeMS = &t
			ent.TimeText = msToText(t)
		}
		entries = append(entries, ent)
	}
	return entries
}

// publishBoard 发布新一版榜单：旧版保留，只更新 current 标记；不覆盖任何历史数据
func publishBoard(db *gorm.DB, eventID uint, note, correction string, publisher tokenPayload) (LeaderboardVersion, error) {
	var board LeaderboardVersion
	err := db.Transaction(func(tx *gorm.DB) error {
		results, err := computeResults(tx, eventID)
		if err != nil {
			return err
		}
		// 存在待裁定泳道时，只允许总裁判明确带 force 发布（接口默认拦截，避免误发）
		for _, r := range results {
			if r.State == "pending" {
				return errors.New("尚有泳道成绩待裁定，不能发布完整榜单")
			}
		}

		var last LeaderboardVersion
		prevErr := tx.Where("event_id = ? AND is_current = ?", eventID, true).
			Order("version desc").First(&last).Error

		ranked := rankResults(results)
		entries := buildRankingEntries(eventID, ranked)

		// 与上一版比对，标注被更正的行
		prevByLane := map[uint]RankingEntry{}
		if prevErr == nil {
			var prevEntries []RankingEntry
			if err := tx.Where("board_id = ?", last.ID).Find(&prevEntries).Error; err != nil {
				return err
			}
			for _, pe := range prevEntries {
				prevByLane[pe.LaneID] = pe
			}
			if err := tx.Model(&LeaderboardVersion{}).Where("id = ?", last.ID).
				Update("is_current", false).Error; err != nil {
				return err
			}
		}

		nextVersion := 1
		if prevErr == nil {
			nextVersion = last.Version + 1
		}
		board = LeaderboardVersion{
			EventID:       eventID,
			Version:       nextVersion,
			ChangeNote:    note,
			Correction:    correction,
			IsCurrent:     true,
			PublishedBy:   publisher.UID,
			PublisherName: publisher.Name,
		}
		if err := tx.Create(&board).Error; err != nil {
			return err
		}
		for i := range entries {
			entries[i].BoardID = board.ID
			if pe, ok := prevByLane[entries[i].LaneID]; ok {
				if pe.Rank != entries[i].Rank ||
					(pe.TimeMS == nil) != (entries[i].TimeMS == nil) ||
					(pe.TimeMS != nil && entries[i].TimeMS != nil && *pe.TimeMS != *entries[i].TimeMS) ||
					pe.SpecialState != entries[i].SpecialState {
					entries[i].Corrected = true
				}
			} else if nextVersion > 1 {
				entries[i].Corrected = true // 上一版无成绩（如漏记），本版补入
			}
		}
		if err := tx.Create(&entries).Error; err != nil {
			return err
		}
		board.Entries = entries
		return nil
	})
	return board, err
}

// autoOpenMissedCase 设备上报漏记时自动立案，进入漏记复核流程
func autoOpenMissedCase(db *gorm.DB, rd TimingReading, lane Lane) {
	if !rd.Missed {
		return
	}
	var cnt int64
	db.Model(&ReviewCase{}).
		Where("event_id = ? AND lane_id = ? AND status = ?", lane.EventID, lane.ID, "open").
		Count(&cnt)
	if cnt > 0 {
		return
	}
	rc := ReviewCase{
		EventID:     lane.EventID,
		LaneID:      lane.ID,
		Reason:      "电子计时漏记",
		Description: fmt.Sprintf("电子计时设备未记录第%d道(%s)触壁时间，原始报文: %s，系统于 %s 自动立案待复核",
			lane.LaneNo, lane.SwimmerName, rd.DeviceRaw, time.Now().Format("15:04:05")),
		Status:      "open",
	}
	db.Create(&rc)
}
