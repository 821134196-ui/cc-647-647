package api

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"timingreview/internal/models"
	"timingreview/internal/ranking"
)

type boardReq struct {
	CorrectionReason string `json:"correction_reason"` // 首次发布可空；更正发布必填
}

// publishBoard 总裁判发布榜单：生成新版本快照，旧版置 SUPERSEDED，不覆盖任何历史。
func (s *Server) publishBoard(c *gin.Context) {
	u := currentUser(c)
	race, ok := s.loadRace(c)
	if !ok {
		return
	}
	var req boardReq
	_ = c.ShouldBindJSON(&req)

	var lastNo int64
	s.DB.Model(&models.Board{}).Where("race_id = ?", race.ID).
		Select("COALESCE(MAX(version_no), 0)").Scan(&lastNo)
	if lastNo > 0 && strings.TrimSpace(req.CorrectionReason) == "" {
		badRequest(c, "这是更正发布，必须填写更正原因（旧榜单将保留可查）")
		return
	}

	lanes, err := ranking.ResolveRace(s.DB, *race)
	if err != nil {
		serverError(c, err)
		return
	}

	var unresolved []string
	for _, l := range lanes {
		if hasFlag(l.Flags, models.FlagMissed) {
			unresolved = append(unresolved, strconv.Itoa(l.Entry.LaneNo))
		}
	}
	if len(unresolved) > 0 {
		badRequest(c, "以下泳道存在未处理的漏记，无法发布："+strings.Join(unresolved, "、")+
			" 道（请先发起复核并由总裁判裁定，或先由设备补发后重新导入）")
		return
	}

	now := time.Now()
	versionNo := int(lastNo) + 1
	reason := strings.TrimSpace(req.CorrectionReason)
	if versionNo == 1 && reason == "" {
		reason = "初次发布"
	}

	board := models.Board{
		RaceID: race.ID, VersionNo: versionNo, Status: models.BoardCurrent,
		CorrectionReason: reason, PublishedByID: u.ID, PublisherName: u.Name, CreatedAt: now,
	}
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		// 旧版整体置为 SUPERSEDED（行仍保留）
		if err := tx.Model(&models.Board{}).
			Where("race_id = ? AND status = ?", race.ID, models.BoardCurrent).
			Update("status", models.BoardSuperseded).Error; err != nil {
			return err
		}
		if err := tx.Create(&board).Error; err != nil {
			return err
		}
		for _, l := range lanes {
			be := models.BoardEntry{
				BoardID: board.ID, LaneEntryID: l.Entry.ID, LaneNo: l.Entry.LaneNo,
				SwimmerName: l.Entry.Swimmer.Name, Team: l.Entry.Swimmer.Team,
				Rank: l.Rank, ResolvedSeconds: l.ResolvedTime, DisplayTime: l.DisplayTime,
				Source: l.Source, Flags: strings.Join(l.Flags, ","), Note: l.Note,
			}
			if err := tx.Create(&be).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		serverError(c, err)
		return
	}
	s.audit(u, "PUBLISH_BOARD",
		strings.Join([]string{"项目 ", race.Code, " 第 ", strconv.Itoa(versionNo), " 版榜单：", reason}, ""))
	c.JSON(200, gin.H{
		"ok": true, "board_id": board.ID, "version_no": versionNo,
		"message": "第 " + strconv.Itoa(versionNo) + " 版榜单已公示，旧版本仍可查询",
	})
}

// listBoards 返回某项目的全部榜单版本（含旧版元信息）
func (s *Server) listBoards(c *gin.Context) {
	race, ok := s.loadRace(c)
	if !ok {
		return
	}
	var boards []models.Board
	s.DB.Where("race_id = ?", race.ID).Order("version_no desc").Find(&boards)
	c.JSON(200, boards)
}

// boardDetail 查看某一版榜单快照（永远不变），并附相邻版本信息
func (s *Server) boardDetail(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "榜单编号错误")
		return
	}
	var board models.Board
	if err := s.DB.Preload("Entries", func(db *gorm.DB) *gorm.DB {
		return db.Order("rank is null asc, rank asc, lane_no asc")
	}).First(&board, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "榜单版本不存在"})
		return
	}
	var prev, next *models.Board
	var prevModel, nextModel models.Board
	if err := s.DB.Where("race_id = ? AND version_no = ?", board.RaceID, board.VersionNo-1).First(&prevModel).Error; err == nil {
		prev = &prevModel
	}
	if err := s.DB.Where("race_id = ? AND version_no = ?", board.RaceID, board.VersionNo+1).First(&nextModel).Error; err == nil {
		next = &nextModel
	}
	c.JSON(200, gin.H{"board": board, "previous": prev, "next": next})
}
