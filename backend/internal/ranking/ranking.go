package ranking

import (
	"fmt"
	"math"
	"sort"

	"gorm.io/gorm"

	"timingreview/internal/models"
)

// ResolvedLane 聚合后的泳道成绩视图
type ResolvedLane struct {
	Entry         models.LaneEntry      `json:"entry"`
	LatestReading *models.TimingReading `json:"latest_reading"`
	ManualTime    *float64              `json:"manual_time"` // 裁判手记（最新一条，录入员材料不作为成绩来源）
	CaseID        uint                  `json:"case_id"`
	CaseStatus    string                `json:"case_status"`
	Ruling        *models.Ruling        `json:"ruling"`
	ResolvedTime  *float64              `json:"resolved_seconds"`
	Source        string                `json:"source"`
	Flags         []string              `json:"flags"`
	Rank          *int                  `json:"rank"`
	DisplayTime   string                `json:"display_time"`
	Note          string                `json:"note"`
}

func roundCenti(v float64) float64 { return math.Round(v*100) / 100 }

// FormatTime 秒数 -> 展示文本（200 米及以上用 m:ss.cc）
func FormatTime(seconds float64, distance int) string {
	seconds = roundCenti(seconds)
	if distance >= 200 {
		m := int(seconds) / 60
		s := seconds - float64(m*60)
		return fmt.Sprintf("%d:%05.2f", m, s)
	}
	return fmt.Sprintf("%.2f", seconds)
}

type laneAgg struct {
	entry      models.LaneEntry
	reading    *models.TimingReading // 最新批次读数
	manual     *float64              // 裁判/总裁判最新手记
	caseID     uint
	caseStatus string
	ruling     *models.Ruling
}

// ResolveRace 计算某项目当前的成绩解析（不写库），用于实时复核页与榜单快照。
func ResolveRace(db *gorm.DB, race models.Race) ([]ResolvedLane, error) {
	var entries []models.LaneEntry
	if err := db.Preload("Swimmer").Where("race_id = ?", race.ID).Order("lane_no asc").Find(&entries).Error; err != nil {
		return nil, err
	}

	// 该项目全部复核案件（含裁定），按泳道归并，取最新案件/裁定
	var cases []models.ReviewCase
	if err := db.Preload("Rulings").Where("race_id = ?", race.ID).Order("created_at asc").Find(&cases).Error; err != nil {
		return nil, err
	}
	caseByLane := map[uint]*models.ReviewCase{} // 泳道级
	var swimoffRaceIDs []uint                   // 所有重赛裁定指向的重赛项目
	for i := range cases {
		c := &cases[i]
		if c.LaneEntryID > 0 {
			caseByLane[c.LaneEntryID] = c
		}
		if len(c.Rulings) > 0 {
			last := c.Rulings[len(c.Rulings)-1]
			if last.Decision == models.DecisionSwimOff && last.SwimoffRaceID > 0 {
				swimoffRaceIDs = append(swimoffRaceIDs, last.SwimoffRaceID)
			}
		}
	}

	// 重赛项目：取其中各运动员（按 SwimmerID）最新解析成绩；同时记录参赛集合用于"待重赛"判断
	swimoffTimeBySwimmer := map[uint]float64{}
	swimoffParticipant := map[uint]bool{}
	for _, rid := range swimoffRaceIDs {
		var soRace models.Race
		if err := db.First(&soRace, rid).Error; err != nil {
			continue
		}
		soLanes, err := ResolveRace(db, soRace)
		if err != nil {
			return nil, err
		}
		for _, l := range soLanes {
			swimoffParticipant[l.Entry.SwimmerID] = true
			if l.ResolvedTime != nil {
				swimoffTimeBySwimmer[l.Entry.SwimmerID] = roundCenti(*l.ResolvedTime)
			}
		}
	}
	out := make([]ResolvedLane, 0, len(entries))
	for _, e := range entries {
		agg := laneAgg{entry: e}

		// 最新批次的原始电子读数（历史读数不覆盖，仅取最新批次参与解析）
		var reading models.TimingReading
		err := db.Where("lane_entry_id = ?", e.ID).Order("import_batch desc, id desc").First(&reading).Error
		if err == nil {
			agg.reading = &reading
		}

		// 最新一条裁判手记（录入员材料仅展示，不构成成绩）
		var note models.ManualNote
		err = db.Where("lane_entry_id = ? AND author_role IN ?", e.ID, []string{models.RoleJudge, models.RoleChief}).
			Order("created_at desc, id desc").First(&note).Error
		if err == nil {
			t := roundCenti(*note.ManualFinish)
			agg.manual = &t
		}

		if rc := caseByLane[e.ID]; rc != nil {
			agg.caseID = rc.ID
			agg.caseStatus = rc.Status
			if len(rc.Rulings) > 0 {
				r := rc.Rulings[len(rc.Rulings)-1]
				agg.ruling = &r
			}
		}

		rl := ResolvedLane{
			Entry:         e,
			LatestReading: agg.reading,
			ManualTime:    agg.manual,
			CaseID:        agg.caseID,
			CaseStatus:    agg.caseStatus,
			Ruling:        agg.ruling,
			Source:        models.SourceNone,
			Flags:         []string{},
		}

		swimoffPending := false
		switch {
		case e.Status == models.EntryWithdrawn:
			rl.Flags = append(rl.Flags, models.FlagWithdrawn)
			rl.Note = e.WithdrawReason
		case swimoffParticipant[e.SwimmerID]:
			// 运动员在已裁令的重赛名单中：重赛完成取重赛成绩，未完成则回退电子成绩并提示
			if t, ok := swimoffTimeBySwimmer[e.SwimmerID]; ok {
				tt := t
				rl.ResolvedTime = &tt
				rl.Source = models.SourceSwimOff
				rl.Flags = append(rl.Flags, models.FlagSwimOff)
				rl.Note = "并列后重赛成绩"
			} else {
				swimoffPending = true
			}
		case agg.ruling != nil && agg.ruling.Decision == models.DecisionManual:
			t := (*float64)(nil)
			if agg.ruling.ManualFinish != nil {
				tt := roundCenti(*agg.ruling.ManualFinish)
				t = &tt
			} else {
				t = agg.manual
			}
			if t != nil {
				tt := roundCenti(*t)
				rl.ResolvedTime = &tt
				rl.Source = models.SourceManual
				rl.Note = "依裁定采用手动成绩"
			}
		default:
			// 无裁定，或裁定要求采用电子成绩
			if agg.reading != nil && agg.reading.FinishTime != nil {
				t := roundCenti(*agg.reading.FinishTime)
				rl.ResolvedTime = &t
				rl.Source = models.SourceElectronic
			} else if agg.reading != nil && agg.reading.Status != models.ReadingOK {
				rl.Flags = append(rl.Flags, models.FlagMissed)
				rl.Note = "电子计时漏记，待复核"
			}
		}

		// 已裁令重赛但重赛尚未进行：回退显示电子原始成绩，并列仍可见
		if swimoffPending {
			if agg.reading != nil && agg.reading.FinishTime != nil {
				t := roundCenti(*agg.reading.FinishTime)
				rl.ResolvedTime = &t
				rl.Source = models.SourceElectronic
				rl.Note = "并列重赛待进行"
			}
		}

		if rl.ResolvedTime != nil {
			rl.DisplayTime = FormatTime(*rl.ResolvedTime, race.Distance)
		}
		out = append(out, rl)
	}

	assignRanks(out)
	markTies(out)
	return out, nil
}

// assignRanks 标准竞赛排名（1224），无成绩/撤回不占位
func assignRanks(lanes []ResolvedLane) {
	type item struct {
		idx int
		t   float64
	}
	items := make([]item, 0)
	for i, l := range lanes {
		if l.ResolvedTime != nil {
			items = append(items, item{i, *l.ResolvedTime})
		}
	}
	sort.SliceStable(items, func(a, b int) bool { return items[a].t < items[b].t })
	for i, it := range items {
		rank := i + 1
		if i > 0 && items[i-1].t == it.t {
			r := *lanes[items[i-1].idx].Rank
			rank = r
		}
		r := rank
		lanes[it.idx].Rank = &r
	}
}

// markTies 当前仍同名次的泳道标 TIE（已由重赛分出先后的不标）
func markTies(lanes []ResolvedLane) {
	count := map[int]int{}
	for _, l := range lanes {
		if l.Rank != nil {
			count[*l.Rank]++
		}
	}
	for i := range lanes {
		if lanes[i].Rank != nil && count[*lanes[i].Rank] > 1 && lanes[i].Source != models.SourceSwimOff {
			lanes[i].Flags = append(lanes[i].Flags, models.FlagTie)
		}
	}
}
