package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"timingreview/internal/models"
	"timingreview/internal/ranking"
)

// importReadings 录入员从电子计时设备拉取读数，作为新批次原样入库（历史批次保留）。
func (s *Server) importReadings(c *gin.Context) {
	u := currentUser(c)
	race, ok := s.loadRace(c)
	if !ok {
		return
	}
	packets, found := s.Device.Readings(race.Code)
	if !found {
		badRequest(c, fmt.Sprintf("设备中没有场次 %s 的报文（重赛场次请进入对应重赛项目导入）", race.Code))
		return
	}
	var entries []models.LaneEntry
	s.DB.Preload("Swimmer").Where("race_id = ?", race.ID).Find(&entries)
	byLane := map[int]models.LaneEntry{}
	for _, e := range entries {
		byLane[e.LaneNo] = e
	}

	var maxBatch int64
	s.DB.Model(&models.TimingReading{}).Where("race_id = ?", race.ID).
		Select("COALESCE(MAX(import_batch), 0)").Scan(&maxBatch)
	batch := int(maxBatch) + 1
	now := time.Now()

	var imported int
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		for _, p := range packets {
			e, exists := byLane[p.LaneNo]
			if !exists {
				return fmt.Errorf("报文泳道 %d 在本项目中不存在", p.LaneNo)
			}
			if e.Swimmer.Name != p.SwimmerName {
				return fmt.Errorf("泳道 %d 报文运动员为「%s」，秩序册为「%s」，拒绝导入", p.LaneNo, p.SwimmerName, e.Swimmer.Name)
			}
			raw, _ := json.Marshal(p)
			splits := models.FloatSlice(p.Splits)
			tr := models.TimingReading{
				RaceID: race.ID, LaneEntryID: e.ID, LaneNo: p.LaneNo,
				DeviceID: p.DeviceID, Status: p.Status, Splits: splits,
				FinishTime: p.FinishTime, RawJSON: string(raw),
				ImportBatch: batch, RecordedAt: p.SentAt,
			}
			if tr.RecordedAt.IsZero() {
				tr.RecordedAt = now
			}
			if err := tx.Create(&tr).Error; err != nil {
				return err
			}
			imported++
		}
		return nil
	})
	if err != nil {
		badRequest(c, "导入中止："+err.Error())
		return
	}
	s.audit(u, "IMPORT_READINGS", fmt.Sprintf("项目 %s 第 %d 批次，%d 条原始读数", race.Code, batch, imported))
	c.JSON(200, gin.H{"ok": true, "import_batch": batch, "count": imported,
		"message": fmt.Sprintf("已原样保存第 %d 批次 %d 条读数，历史批次不受影响", batch, imported)})
}

type retransmitReq struct {
	Lane       int       `json:"lane"`
	FinishTime *float64  `json:"finish_time"`
	Splits     []float64 `json:"splits"`
}

// deviceRetransmit 录入员触发设备补发（模拟备用计时模块），随后需重新"导入"入库
func (s *Server) deviceRetransmit(c *gin.Context) {
	u := currentUser(c)
	code := c.Param("code")
	var req retransmitReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Lane <= 0 || req.FinishTime == nil {
		badRequest(c, "请提供 lane 与 finish_time")
		return
	}
	p, ok := s.Device.Retransmit(code, req.Lane, *req.FinishTime, req.Splits)
	if !ok {
		badRequest(c, "设备中不存在该场次/泳道")
		return
	}
	s.audit(u, "DEVICE_RETRANSMIT", fmt.Sprintf("场次 %s 泳道 %d 补发触壁 %.2f", code, req.Lane, *req.FinishTime))
	c.JSON(200, gin.H{"ok": true, "packet": p, "message": "设备已补发，请在项目页重新导入读数"})
}

type noteReq struct {
	Content      string    `json:"content"`
	ManualFinish *float64  `json:"manual_finish"`
	ManualSplits []float64 `json:"manual_splits"`
}

// addNote 手记与补充材料。录入员只能交文字材料；手记成绩仅裁判/总裁判有效。
func (s *Server) addNote(c *gin.Context) {
	u := currentUser(c)
	race, ok := s.loadRace(c)
	if !ok {
		return
	}
	laneID, err := strconv.Atoi(c.Param("laneId"))
	if err != nil {
		badRequest(c, "泳道编号错误")
		return
	}
	var entry models.LaneEntry
	if err := s.DB.Where("id = ? AND race_id = ?", laneID, race.ID).First(&entry).Error; err != nil {
		c.JSON(404, gin.H{"error": "泳道不存在"})
		return
	}
	var req noteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请求格式错误")
		return
	}
	if strings.TrimSpace(req.Content) == "" && req.ManualFinish == nil {
		badRequest(c, "请填写材料内容或手记成绩")
		return
	}
	if u.Role == models.RoleClerk && req.ManualFinish != nil {
		c.JSON(403, gin.H{"error": "录入员只能补充文字材料，手记成绩必须由裁判提交，不得据此改变名次"})
		return
	}
	note := models.ManualNote{
		RaceID: race.ID, LaneEntryID: entry.ID, AuthorID: u.ID,
		AuthorName: u.Name, AuthorRole: u.Role, Content: req.Content,
		ManualFinish: req.ManualFinish, ManualSplits: models.FloatSlice(req.ManualSplits),
		CreatedAt: time.Now(),
	}
	if err := s.DB.Create(&note).Error; err != nil {
		serverError(c, err)
		return
	}
	action := "ADD_MATERIAL"
	if req.ManualFinish != nil {
		action = "ADD_MANUAL_NOTE"
	}
	s.audit(u, action, fmt.Sprintf("项目 %s 泳道 %d：%s", race.Code, entry.LaneNo, strings.TrimSpace(req.Content)))
	c.JSON(200, gin.H{"ok": true, "note": note})
}

type caseReq struct {
	LaneEntryID uint   `json:"lane_entry_id"`
	ReasonType  string `json:"reason_type"`
	Summary     string `json:"summary"`
	LaneIDs     []uint `json:"lane_entry_ids"`
}

// openCase 裁判发起复核（漏记 / 争议 / 并列 / 撤回）
func (s *Server) openCase(c *gin.Context) {
	u := currentUser(c)
	race, ok := s.loadRace(c)
	if !ok {
		return
	}
	var req caseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请求格式错误")
		return
	}
	valid := map[string]bool{
		models.ReasonMissedTouch: true, models.ReasonDispute: true,
		models.ReasonTie: true, models.ReasonWithdrawal: true,
	}
	if !valid[req.ReasonType] {
		badRequest(c, "复核原因类型无效")
		return
	}
	if req.ReasonType != models.ReasonTie && req.LaneEntryID == 0 {
		badRequest(c, "该复核类型必须指定具体泳道")
		return
	}
	if strings.TrimSpace(req.Summary) == "" {
		badRequest(c, "请填写复核说明")
		return
	}
	// 不允许对已有未结案件重复发起
	var dup int64
	s.DB.Model(&models.ReviewCase{}).
		Where("race_id = ? AND status = ? AND (lane_entry_id = ? OR (? = 0 AND lane_entry_id = 0))",
			race.ID, models.CaseOpen, req.LaneEntryID, req.LaneEntryID).Count(&dup)
	if dup > 0 {
		badRequest(c, "该项目/泳道已有进行中的复核案件")
		return
	}
	rc := models.ReviewCase{
		RaceID: race.ID, LaneEntryID: req.LaneEntryID, OpenedByID: u.ID,
		OpenerName: u.Name, ReasonType: req.ReasonType, Summary: req.Summary,
		Status: models.CaseOpen, CreatedAt: time.Now(),
	}
	if err := s.DB.Create(&rc).Error; err != nil {
		serverError(c, err)
		return
	}
	s.audit(u, "OPEN_CASE", fmt.Sprintf("项目 %s 案件#%d 类型 %s：%s", race.Code, rc.ID, req.ReasonType, req.Summary))
	c.JSON(200, gin.H{"ok": true, "case": rc})
}

type rulingReq struct {
	Decision     string   `json:"decision"`
	ManualFinish *float64 `json:"manual_finish"`
	Rationale    string   `json:"rationale"`
	LaneIDs      []uint   `json:"lane_entry_ids"` // 重赛涉及泳道，缺省取当前并列泳道
}

// addRuling 总裁判终局裁定：采用电子 / 手动 / 重赛结果
func (s *Server) addRuling(c *gin.Context) {
	u := currentUser(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "案件编号错误")
		return
	}
	var rc models.ReviewCase
	if err := s.DB.First(&rc, id).Error; err != nil {
		c.JSON(404, gin.H{"error": "复核案件不存在"})
		return
	}
	var req rulingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请求格式错误")
		return
	}
	if req.Decision != models.DecisionElectronic && req.Decision != models.DecisionManual && req.Decision != models.DecisionSwimOff {
		badRequest(c, "裁定结论无效")
		return
	}
	if strings.TrimSpace(req.Rationale) == "" {
		badRequest(c, "裁定必须填写依据")
		return
	}
	var race models.Race
	if err := s.DB.First(&race, rc.RaceID).Error; err != nil {
		serverError(c, err)
		return
	}

	ruling := models.Ruling{
		CaseID: rc.ID, DecidedByID: u.ID, DeciderName: u.Name,
		Decision: req.Decision, Rationale: req.Rationale, CreatedAt: time.Now(),
	}

	switch req.Decision {
	case models.DecisionManual:
		if rc.LaneEntryID == 0 {
			badRequest(c, "采用手动成绩的裁定必须针对具体泳道案件")
			return
		}
		if req.ManualFinish == nil {
			// 取该泳道最新裁判手记
			var note models.ManualNote
			err := s.DB.Where("lane_entry_id = ? AND author_role IN ? AND manual_finish IS NOT NULL",
				rc.LaneEntryID, []string{models.RoleJudge, models.RoleChief}).
				Order("created_at desc").First(&note).Error
			if err != nil {
				badRequest(c, "该泳道没有可采用的裁判手记，请在裁定中明确手动成绩")
				return
			}
			req.ManualFinish = note.ManualFinish
		}
		t := *req.ManualFinish
		ruling.ManualFinish = &t
	case models.DecisionSwimOff:
		soRaceID, names, times, err := s.ensureSwimOff(&race, rc.LaneEntryID, req.LaneIDs)
		if err != nil {
			badRequest(c, err.Error())
			return
		}
		ruling.SwimoffRaceID = soRaceID
		_ = names
		_ = times
	}

	now := time.Now()
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&ruling).Error; err != nil {
			return err
		}
		return tx.Model(&models.ReviewCase{}).Where("id = ?", rc.ID).
			Updates(map[string]interface{}{"status": models.CaseClosed, "closed_at": &now}).Error
	})
	if err != nil {
		serverError(c, err)
		return
	}
	s.audit(u, "RULING", fmt.Sprintf("案件#%d 项目 %s 结论 %s：%s", rc.ID, race.Code, req.Decision, req.Rationale))
	c.JSON(200, gin.H{"ok": true, "ruling": ruling, "message": "裁定已记录，名次以榜单发布为准"})
}

// ensureSwimOff 找到或创建重赛项目、泳道并在设备登记重赛场次
func (s *Server) ensureSwimOff(parent *models.Race, laneEntryID uint, laneIDs []uint) (uint, []string, []float64, error) {
	lanes, err := ranking.ResolveRace(s.DB, *parent)
	if err != nil {
		return 0, nil, nil, err
	}
	pick := map[uint]bool{}
	for _, id := range laneIDs {
		pick[id] = true
	}
	var chosen []ranking.ResolvedLane
	for _, l := range lanes {
		if len(pick) > 0 {
			if pick[l.Entry.ID] && l.ResolvedTime != nil {
				chosen = append(chosen, l)
			}
			continue
		}
		if laneEntryID > 0 {
			// 泳道级案件：取与其同名次的全部泳道
			if l.Rank != nil && lanes != nil {
				for _, l2 := range lanes {
					if l2.Entry.ID == laneEntryID && l2.Rank != nil && l.Rank != nil && *l.Rank == *l2.Rank {
						chosen = append(chosen, l)
						break
					}
				}
			}
		} else if hasFlag(l.Flags, models.FlagTie) {
			chosen = append(chosen, l)
		}
	}
	// 去重（laneEntryID 分支可能重复加入）
	seen := map[uint]bool{}
	uniq := chosen[:0]
	for _, l := range chosen {
		if !seen[l.Entry.ID] {
			seen[l.Entry.ID] = true
			uniq = append(uniq, l)
		}
	}
	chosen = uniq
	if len(chosen) < 2 {
		return 0, nil, nil, fmt.Errorf("当前没有可重赛的并列泳道（至少需要两名成绩相同的运动员）")
	}

	// 同一并列组只允许一个重赛场次：以父项目下已有重赛 + 裁定记录判重
	var cnt int64
	s.DB.Model(&models.Race{}).Where("parent_race_id = ?", parent.ID).Count(&cnt)
	var soRace models.Race
	code := fmt.Sprintf("%s-SO%d", parent.Code, cnt+1)
	now := time.Now()
	soRace = models.Race{
		MeetID: parent.MeetID, Code: code,
		EventName: parent.EventName + "（重赛）", Distance: parent.Distance,
		Stroke: parent.Stroke, Round: "重赛", ParentRaceID: parent.ID, CreatedAt: now,
	}
	if err := s.DB.Create(&soRace).Error; err != nil {
		return 0, nil, nil, err
	}
	names := make([]string, 0, len(chosen))
	times := make([]float64, 0, len(chosen))
	for i, l := range chosen {
		le := models.LaneEntry{
			RaceID: soRace.ID, LaneNo: i + 1, SwimmerID: l.Entry.SwimmerID,
			Status: models.EntryActive, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.DB.Create(&le).Error; err != nil {
			return 0, nil, nil, err
		}
		names = append(names, l.Entry.Swimmer.Name)
		times = append(times, *l.ResolvedTime)
	}
	s.Device.RegisterSwimOff(code, names, times)
	return soRace.ID, names, times, nil
}

type withdrawReq struct {
	Reason string `json:"reason"`
}

// withdraw 总裁判裁定撤回
func (s *Server) withdraw(c *gin.Context) {
	u := currentUser(c)
	race, ok := s.loadRace(c)
	if !ok {
		return
	}
	laneID, err := strconv.Atoi(c.Param("laneId"))
	if err != nil {
		badRequest(c, "泳道编号错误")
		return
	}
	var req withdrawReq
	_ = c.ShouldBindJSON(&req)
	if strings.TrimSpace(req.Reason) == "" {
		badRequest(c, "撤回必须填写原因")
		return
	}
	var entry models.LaneEntry
	if err := s.DB.Where("id = ? AND race_id = ?", laneID, race.ID).First(&entry).Error; err != nil {
		c.JSON(404, gin.H{"error": "泳道不存在"})
		return
	}
	now := time.Now()
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.LaneEntry{}).Where("id = ?", entry.ID).
			Updates(map[string]interface{}{"status": models.EntryWithdrawn, "withdraw_reason": req.Reason, "updated_at": now}).Error; err != nil {
			return err
		}
		rc := models.ReviewCase{
			RaceID: race.ID, LaneEntryID: entry.ID, OpenedByID: u.ID, OpenerName: u.Name,
			ReasonType: models.ReasonWithdrawal, Summary: "撤回：" + req.Reason,
			Status: models.CaseClosed, CreatedAt: now, ClosedAt: &now,
		}
		return tx.Create(&rc).Error
	})
	if err != nil {
		serverError(c, err)
		return
	}
	s.audit(u, "WITHDRAW", fmt.Sprintf("项目 %s 泳道 %d：%s", race.Code, entry.LaneNo, req.Reason))
	c.JSON(200, gin.H{"ok": true, "message": "已记录撤回，发布后榜单将保留该泳道并标注撤回"})
}

func hasFlag(flags []string, f string) bool {
	for _, x := range flags {
		if x == f {
			return true
		}
	}
	return false
}
