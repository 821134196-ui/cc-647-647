package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Server struct {
	db *gorm.DB
}

func main() {
	dbPath := os.Getenv("SWIM_DB")
	if dbPath == "" {
		dbPath = "data/swim.db"
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		fmt.Println("创建数据目录失败:", err)
		os.Exit(1)
	}
	db := initDB(dbPath)

	gin.SetMode(gin.ReleaseMode)
	srv := &Server{db: db}
	r := srv.router()

	addr := os.Getenv("SWIM_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	fmt.Println("游泳计时复核系统后端已启动: http://localhost" + addr)
	if err := r.Run(addr); err != nil {
		fmt.Println("服务启动失败:", err)
		os.Exit(1)
	}
}

// ---------------- 鉴权 ----------------

func (s *Server) login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
		return
	}
	var u User
	if err := s.db.Where("username = ? AND password = ?", req.Username, req.Password).First(&u).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	tok := signToken(tokenPayload{UID: u.ID, Username: u.Username, Role: u.Role, Name: u.Name})
	c.JSON(http.StatusOK, gin.H{"token": tok, "user": u})
}

func (s *Server) me(c *gin.Context) {
	u := currentUser(c)
	c.JSON(http.StatusOK, gin.H{"id": u.UID, "username": u.Username, "name": u.Name, "role": u.Role})
}

// ---------------- 赛事 / 项目 ----------------

func (s *Server) listMeets(c *gin.Context) {
	var meets []Meet
	s.db.Order("id").Find(&meets)
	var events []Event
	s.db.Order("id").Find(&events)
	byMeet := map[uint][]Event{}
	for _, e := range events {
		byMeet[e.MeetID] = append(byMeet[e.MeetID], e)
	}
	out := []gin.H{}
	for _, m := range meets {
		out = append(out, gin.H{"meet": m, "events": byMeet[m.ID]})
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) createMeet(c *gin.Context) {
	var m Meet
	if err := c.ShouldBindJSON(&m); err != nil || m.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "赛事名称必填"})
		return
	}
	if err := s.db.Create(&m).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, m)
}

func (s *Server) createEvent(c *gin.Context) {
	meetID := c.Param("id")
	var e Event
	if err := c.ShouldBindJSON(&e); err != nil || e.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "项目名称必填"})
		return
	}
	fmt.Sscanf(meetID, "%d", &e.MeetID)
	if err := s.db.Create(&e).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, e)
}

func (s *Server) addLanes(c *gin.Context) {
	eventID := c.Param("id")
	var req struct {
		Lanes []struct {
			LaneNo      int    `json:"lane_no"`
			SwimmerName string `json:"swimmer_name"`
			Team        string `json:"team"`
		} `json:"lanes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Lanes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "泳道列表必填"})
		return
	}
	var eid uint
	fmt.Sscanf(eventID, "%d", &eid)
	var created []Lane
	for _, l := range req.Lanes {
		ln := Lane{EventID: eid, LaneNo: l.LaneNo, SwimmerName: l.SwimmerName, Team: l.Team}
		if err := s.db.Create(&ln).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		created = append(created, ln)
	}
	c.JSON(http.StatusOK, created)
}

// laneDetail 聚合一条泳道的全部原始与人工数据
type laneDetail struct {
	Lane      Lane            `json:"lane"`
	Splits    []TimingReading `json:"splits"`
	Touch     *TimingReading  `json:"touch"`
	AllTouches []TimingReading `json:"all_touches"`
	Manuals   []ManualRecord  `json:"manuals"`
	Cases     []ReviewCase    `json:"cases"`
}

func (s *Server) eventDetail(c *gin.Context) {
	id := c.Param("id")
	var ev Event
	if err := s.db.First(&ev, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "项目不存在"})
		return
	}
	var lanes []Lane
	s.db.Where("event_id = ?", ev.ID).Order("lane_no").Find(&lanes)

	details := make([]laneDetail, 0, len(lanes))
	for _, l := range lanes {
		var readings []TimingReading
		s.db.Where("lane_id = ?", l.ID).Order("split_index, id").Find(&readings)
		var manuals []ManualRecord
		s.db.Where("lane_id = ?", l.ID).Order("id").Find(&manuals)
		var cases []ReviewCase
		s.db.Where("lane_id = ?", l.ID).Order("id desc").Find(&cases)

		d := laneDetail{Lane: l, Manuals: manuals, Cases: cases, Touch: nil}
		for i := range readings {
			if readings[i].SplitIndex == 0 {
				d.AllTouches = append(d.AllTouches, readings[i])
			} else {
				d.Splits = append(d.Splits, readings[i])
			}
		}
		if len(d.AllTouches) > 0 {
			latest := d.AllTouches[len(d.AllTouches)-1]
			if !latest.Missed {
				d.Touch = &latest
			}
		}
		details = append(details, d)
	}

	var board LeaderboardVersion
	boardErr := s.db.Where("event_id = ? AND is_current = ?", ev.ID, true).
		Order("version desc").Preload("Entries", func(db *gorm.DB) *gorm.DB {
			return db.Order("rank desc, lane_no")
		}).First(&board).Error

	var pendingChanges int64
	s.db.Model(&ReviewCase{}).Where("event_id = ? AND status = ?", ev.ID, "decided").Count(&pendingChanges)

	resp := gin.H{
		"event":           ev,
		"lanes":           details,
		"current_board":   nil,
		"decided_cases":   pendingChanges,
		"server_time":     time.Now().Format("2006-01-02 15:04:05"),
	}
	if boardErr == nil {
		// 排名正序展示
		entries := board.Entries
		sortEntriesAsc(entries)
		board.Entries = entries
		resp["current_board"] = board
	}
	c.JSON(http.StatusOK, resp)
}

func sortEntriesAsc(e []RankingEntry) {
	for i := 0; i < len(e); i++ {
		for j := i + 1; j < len(e); j++ {
			ri, rj := rankOrder(e[i]), rankOrder(e[j])
			if rj < ri {
				e[i], e[j] = e[j], e[i]
			}
		}
	}
}

func rankOrder(e RankingEntry) int {
	if e.Rank > 0 {
		return e.Rank
	}
	return 1000 + e.LaneNo // 待裁定/撤回沉底
}

// ---------------- 实时排名（未发布，仅供裁判席） ----------------

func (s *Server) liveRanking(c *gin.Context) {
	id := c.Param("id")
	var eid uint
	fmt.Sscanf(id, "%d", &eid)
	results, err := computeResults(s.db, eid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ranked := rankResults(results)
	entries := buildRankingEntries(eid, ranked)
	sortEntriesAsc(entries)
	c.JSON(http.StatusOK, gin.H{"entries": entries})
}

// ---------------- 电子计时设备 ----------------

func (s *Server) uploadReading(c *gin.Context) {
	var req struct {
		EventID    uint   `json:"event_id"`
		LaneNo     int    `json:"lane_no"`
		SplitIndex int    `json:"split_index"`
		SplitLabel string `json:"split_label"`
		TimeMS     *int64 `json:"time_ms"`
		Missed     bool   `json:"missed"`
		Source     string `json:"source"`
		DeviceRaw  string `json:"device_raw"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.EventID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
		return
	}
	var lane Lane
	if err := s.db.Where("event_id = ? AND lane_no = ?", req.EventID, req.LaneNo).
		First(&lane).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "泳道不存在"})
		return
	}
	src := req.Source
	if src == "" {
		src = "pad"
	}
	if req.Missed {
		req.TimeMS = nil
	}
	label := req.SplitLabel
	if label == "" && req.SplitIndex == 0 {
		label = "触壁"
	}
	rd := TimingReading{
		LaneID: lane.ID, EventID: req.EventID, SplitIndex: req.SplitIndex,
		SplitLabel: label, TimeMS: req.TimeMS, Missed: req.Missed,
		Source: src, DeviceRaw: req.DeviceRaw, RecordedAt: time.Now(),
	}
	if err := s.db.Create(&rd).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	autoOpenMissedCase(s.db, rd, lane)
	c.JSON(http.StatusOK, gin.H{"reading": rd, "auto_case_opened": rd.Missed})
}

// ---------------- 手记 / 补充材料 ----------------

func (s *Server) addManual(c *gin.Context) {
	laneID := c.Param("id")
	var lane Lane
	if err := s.db.First(&lane, laneID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "泳道不存在"})
		return
	}
	var req struct {
		TimeMS     int64  `json:"time_ms"`
		SplitLabel string `json:"split_label"`
		Note       string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
		return
	}
	u := currentUser(c)
	label := req.SplitLabel
	if label == "" {
		label = "触壁"
	}
	mr := ManualRecord{
		LaneID: lane.ID, EventID: lane.EventID, TimeMS: req.TimeMS,
		SplitLabel: label, Note: req.Note,
		AuthorID: u.UID, AuthorName: u.Name, AuthorRole: u.Role,
	}
	if err := s.db.Create(&mr).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, mr)
}

// ---------------- 复核案件 ----------------

func (s *Server) openCase(c *gin.Context) {
	id := c.Param("id")
	var eid uint
	fmt.Sscanf(id, "%d", &eid)
	var req struct {
		LaneID      uint   `json:"lane_id"`
		Reason      string `json:"reason"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.LaneID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择泳道并填写争议原因"})
		return
	}
	var lane Lane
	if err := s.db.Where("id = ? AND event_id = ?", req.LaneID, eid).First(&lane).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "泳道不属于该项目"})
		return
	}
	// 同一泳道已有未决案件则不重复立案
	var cnt int64
	s.db.Model(&ReviewCase{}).Where("lane_id = ? AND status = ?", lane.ID, "open").Count(&cnt)
	if cnt > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "该泳道已有进行中的复核案件，请在原案件中补充材料"})
		return
	}
	u := currentUser(c)
	desc := req.Description
	if u.Role == "judge" {
		desc = "【裁判" + u.Name + "立案】" + desc
	}
	rc := ReviewCase{
		EventID: eid, LaneID: lane.ID, Reason: req.Reason,
		Description: desc, Status: "open",
	}
	if err := s.db.Create(&rc).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rc)
}

func (s *Server) appendCase(c *gin.Context) {
	id := c.Param("id")
	var rc ReviewCase
	if err := s.db.First(&rc, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "案件不存在"})
		return
	}
	var req struct {
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Description) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "补充内容不能为空"})
		return
	}
	u := currentUser(c)
	roleName := map[string]string{"judge": "裁判", "clerk": "录入员", "chief": "总裁判"}[u.Role]
	line := fmt.Sprintf("\n[%s %s %s 补充] %s",
		time.Now().Format("01-02 15:04"), roleName, u.Name, req.Description)
	if err := s.db.Model(&rc).Update("description", rc.Description+line).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.db.First(&rc, rc.ID)
	c.JSON(http.StatusOK, rc)
}

func (s *Server) decideCase(c *gin.Context) {
	id := c.Param("id")
	var rc ReviewCase
	if err := s.db.First(&rc, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "案件不存在"})
		return
	}
	if rc.Status == "decided" && rc.Decision == "withdraw" {
		c.JSON(http.StatusConflict, gin.H{"error": "案件已撤回，如需改判请先由总裁判重新开启复核"})
		return
	}
	var req struct {
		Decision string `json:"decision"` // electronic / manual / swimoff / withdraw
		TimeMS   *int64 `json:"time_ms"`
		Note     string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
		return
	}

	var decided *int64
	switch req.Decision {
	case "electronic":
		var rd TimingReading
		if err := s.db.Where("lane_id = ? AND split_index = 0 AND source = 'pad' AND missed = ?",
			rc.LaneID, false).Order("recorded_at desc, id desc").First(&rd).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无有效电子触壁成绩可采用"})
			return
		}
		t := *rd.TimeMS
		decided = &t
	case "manual":
		t := req.TimeMS
		if t == nil {
			var mr ManualRecord
			if err := s.db.Where("lane_id = ?", rc.LaneID).Order("id desc").First(&mr).Error; err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "采用手动成绩须提供成绩或先提交手记"})
				return
			}
			t = &mr.TimeMS
		}
		decided = t
	case "swimoff":
		if req.TimeMS == nil || *req.TimeMS <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "采用重赛结果须录入重赛成绩"})
			return
		}
		decided = req.TimeMS
	case "withdraw":
		// 无需成绩
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "裁定类型无效"})
		return
	}

	u := currentUser(c)
	now := time.Now()
	rc.Status = "decided"
	rc.Decision = req.Decision
	rc.DecidedTimeMS = decided
	rc.DecisionNote = req.Note
	rc.RefereeID = &u.UID
	rc.RefereeName = u.Name
	rc.DecidedAt = &now
	if err := s.db.Save(&rc).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rc)
}

func (s *Server) reopenCase(c *gin.Context) {
	id := c.Param("id")
	var rc ReviewCase
	if err := s.db.First(&rc, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "案件不存在"})
		return
	}
	u := currentUser(c)
	note := fmt.Sprintf("\n[%s 总裁判%s 重新开启复核]", time.Now().Format("01-02 15:04"), u.Name)
	rc.Status = "open"
	rc.Decision = ""
	rc.DecidedTimeMS = nil
	rc.DecisionNote = rc.DecisionNote + note
	rc.RefereeID = nil
	rc.RefereeName = ""
	rc.DecidedAt = nil
	if err := s.db.Save(&rc).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rc)
}

// ---------------- 榜单版本 ----------------

func (s *Server) listBoards(c *gin.Context) {
	id := c.Param("id")
	var boards []LeaderboardVersion
	s.db.Where("event_id = ?", id).Order("version desc").Find(&boards)
	out := []gin.H{}
	for _, b := range boards {
		var cnt int64
		s.db.Model(&RankingEntry{}).Where("board_id = ?", b.ID).Count(&cnt)
		out = append(out, gin.H{
			"id": b.ID, "version": b.Version, "change_note": b.ChangeNote,
			"correction": b.Correction, "is_current": b.IsCurrent,
			"created_at": b.CreatedAt, "publisher_name": b.PublisherName, "entries_count": cnt,
		})
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) getBoard(c *gin.Context) {
	id := c.Param("id")
	version := c.Param("version")
	var board LeaderboardVersion
	if err := s.db.Where("event_id = ? AND version = ?", id, version).
		Preload("Entries").First(&board).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "该版本榜单不存在"})
		return
	}
	sortEntriesAsc(board.Entries)
	c.JSON(http.StatusOK, board)
}

// summarizeCorrection 对比上一版，生成人读更正摘要
func summarizeCorrection(prev *LeaderboardVersion, entries []RankingEntry, db *gorm.DB) string {
	if prev == nil {
		return fmt.Sprintf("首版榜单，共 %d 条成绩", len(entries))
	}
	var prevEntries []RankingEntry
	db.Where("board_id = ?", prev.ID).Find(&prevEntries)
	prevByLane := map[uint]RankingEntry{}
	for _, e := range prevEntries {
		prevByLane[e.LaneID] = e
	}
	var parts []string
	for _, e := range entries {
		old, ok := prevByLane[e.LaneID]
		switch {
		case !ok && e.TimeMS != nil:
			parts = append(parts, fmt.Sprintf("%s(第%d道)补入成绩%s", e.SwimmerName, e.LaneNo, e.TimeText))
		case !ok && e.SpecialState == "withdrawn":
			parts = append(parts, fmt.Sprintf("%s(第%d道)撤回", e.SwimmerName, e.LaneNo))
		case ok && e.SpecialState == "withdrawn" && old.SpecialState != "withdrawn":
			parts = append(parts, fmt.Sprintf("%s(第%d道)撤回", e.SwimmerName, e.LaneNo))
		case ok && e.TimeMS != nil && (old.TimeMS == nil || *old.TimeMS != *e.TimeMS):
			parts = append(parts, fmt.Sprintf("%s(第%d道)成绩%s→%s(%s)",
				e.SwimmerName, e.LaneNo, old.TimeText, e.TimeText, sourceName(e.Source)))
		case ok && old.Rank != e.Rank:
			parts = append(parts, fmt.Sprintf("%s(第%d道)名次%s→%s",
				e.SwimmerName, e.LaneNo, old.DisplayRank, e.DisplayRank))
		case ok && old.SpecialState != e.SpecialState && e.SpecialState == "tie":
			parts = append(parts, fmt.Sprintf("%s(第%d道)判定并列第%s", e.SwimmerName, e.LaneNo, e.DisplayRank))
		}
	}
	if len(parts) == 0 {
		return "内容与上一版一致（重新公示）"
	}
	return strings.Join(parts, "；")
}

func sourceName(s string) string {
	return map[string]string{
		"electronic": "电子", "manual": "手动", "swimoff": "重赛",
	}[s]
}

func (s *Server) publish(c *gin.Context) {
	id := c.Param("id")
	var eid uint
	fmt.Sscanf(id, "%d", &eid)
	var ev Event
	if err := s.db.First(&ev, eid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "项目不存在"})
		return
	}
	var req struct {
		ChangeNote string `json:"change_note"`
	}
	_ = c.ShouldBindJSON(&req)

	var last *LeaderboardVersion
	var prev LeaderboardVersion
	if err := s.db.Where("event_id = ? AND is_current = ?", eid, true).
		Order("version desc").First(&prev).Error; err == nil {
		last = &prev
	}

	board, err := publishBoard(s.db, eid, req.ChangeNote, "", currentUser(c))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	board.Correction = summarizeCorrection(last, board.Entries, s.db)
	s.db.Model(&board).Update("correction", board.Correction)
	s.db.Preload("Entries").First(&board, board.ID)
	sortEntriesAsc(board.Entries)
	c.JSON(http.StatusOK, gin.H{"board": board, "correction": board.Correction})
}
