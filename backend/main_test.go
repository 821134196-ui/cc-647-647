package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("打开内存数据库失败: %v", err)
	}
	if err := db.AutoMigrate(
		&User{}, &Meet{}, &Event{}, &Lane{}, &TimingReading{},
		&ManualRecord{}, &ReviewCase{}, &RankingEntry{}, &LeaderboardVersion{},
	); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	seed(db)
	return &Server{db: db}
}

func loginToken(t *testing.T, s *Server, username string) string {
	t.Helper()
	body, _ := json.Marshal(gin.H{"username": username, "password": "123456"})
	w := do(s, http.MethodPost, "/api/auth/login", "", body)
	if w.Code != http.StatusOK {
		t.Fatalf("登录 %s 失败: %d %s", username, w.Code, w.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.Token
}

func do(s *Server, method, path, token string, body any) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		var b []byte
		if raw, ok := body.([]byte); ok {
			b = raw
		} else {
			b, _ = json.Marshal(body)
		}
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.router().ServeHTTP(w, req)
	return w
}

// 取种子赛事里第1个项目 id 与指定泳道 id
func fixtureIDs(t *testing.T, s *Server) (eventID uint, laneByNo map[int]uint) {
	t.Helper()
	var ev Event
	if err := s.db.Where("name = ?", "男子100米自由泳 决赛").First(&ev).Error; err != nil {
		t.Fatal(err)
	}
	var lanes []Lane
	s.db.Where("event_id = ?", ev.ID).Find(&lanes)
	laneByNo = map[int]uint{}
	for _, l := range lanes {
		laneByNo[l.LaneNo] = l.ID
	}
	return ev.ID, laneByNo
}

func decode(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("解析响应失败: %v, body=%s", err, w.Body.String())
	}
}

// 场景1：漏记复核 —— 设备漏记 → 自动立案 → 裁判手记/补充 → 总裁判采用手动成绩 → 发布
func TestMissedTouchReviewFlow(t *testing.T) {
	s := newTestServer(t)
	device := loginToken(t, s, "device")
	judge := loginToken(t, s, "judge")
	clerk := loginToken(t, s, "clerk")
	chief := loginToken(t, s, "chief")
	eventID, lanes := fixtureIDs(t, s)
	lane4 := lanes[4]

	// 种子已生成一条漏记读数与 open 案件；设备再次补报漏记也应保持单案件
	w := do(s, http.MethodPost, "/api/device/readings", device, gin.H{
		"event_id": eventID, "lane_no": 4, "split_index": 0,
		"missed": true, "device_raw": "PAD-RETRY-TIMEOUT",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("设备上报漏记失败: %s", w.Body.String())
	}
	var openCases []ReviewCase
	s.db.Where("lane_id = ? AND status = ?", lane4, "open").Find(&openCases)
	if len(openCases) != 1 {
		t.Fatalf("漏记应保持唯一 open 案件，实际 %d 条", len(openCases))
	}
	caseID := openCases[0].ID

	// 录入员可补充材料
	w = do(s, http.MethodPost, "/api/cases/"+itoa(caseID)+"/append", clerk, gin.H{
		"description": "已调取录像，运动员双手触壁清晰",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("录入员补充材料被拒: %s", w.Body.String())
	}

	// 裁判提交手记
	w = do(s, http.MethodPost, "/api/lanes/"+itoa(lane4)+"/manual", judge, gin.H{
		"time_ms": 52940, "note": "复核手记52.94",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("裁判手记失败: %s", w.Body.String())
	}

	// 录入员无权裁定
	w = do(s, http.MethodPost, "/api/cases/"+itoa(caseID)+"/decision", clerk, gin.H{
		"decision": "manual", "time_ms": 52940,
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("录入员裁定应被403拒绝，实际 %d", w.Code)
	}
	// 裁判也无权裁定
	w = do(s, http.MethodPost, "/api/cases/"+itoa(caseID)+"/decision", judge, gin.H{
		"decision": "manual", "time_ms": 52940,
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("裁判裁定应被403拒绝，实际 %d", w.Code)
	}

	// 未裁定前发布应被拦截（有待裁定泳道）
	w = do(s, http.MethodPost, "/api/events/"+itoa(eventID)+"/publish", chief, gin.H{"change_note": "试发"})
	if w.Code != http.StatusConflict {
		t.Fatalf("存在待裁定泳道时应禁止发布，实际 %d", w.Code)
	}

	// 总裁判采用手动成绩
	w = do(s, http.MethodPost, "/api/cases/"+itoa(caseID)+"/decision", chief, gin.H{
		"decision": "manual", "note": "三表一致+录像佐证，采用手记52.94",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("总裁判手动裁定失败: %s", w.Body.String())
	}
	var rc ReviewCase
	s.db.First(&rc, caseID)
	if rc.Status != "decided" || rc.Decision != "manual" || rc.DecidedTimeMS == nil || *rc.DecidedTimeMS != 52940 {
		t.Fatalf("裁定落库异常: %+v", rc)
	}

	// 发布榜单
	w = do(s, http.MethodPost, "/api/events/"+itoa(eventID)+"/publish", chief, gin.H{"change_note": "第4道漏记复核后首发"})
	if w.Code != http.StatusOK {
		t.Fatalf("发布失败: %s", w.Body.String())
	}
	var pub struct {
		Board      LeaderboardVersion `json:"board"`
		Correction string             `json:"correction"`
	}
	decode(t, w, &pub)
	if pub.Board.Version != 1 || !pub.Board.IsCurrent {
		t.Fatalf("首发版本异常: %+v", pub.Board)
	}
	var entry4 RankingEntry
	s.db.Where("board_id = ? AND lane_id = ?", pub.Board.ID, lane4).First(&entry4)
	if entry4.TimeText != "52.94" || entry4.Source != "manual" {
		t.Fatalf("第4道榜单行应为手动52.94，实际 %+v", entry4)
	}
	// 52.94 排在 51.88x2(第1)、52.35(第3)、52.76(第4) 之后 → 第5名
	if entry4.Rank != 5 {
		t.Fatalf("第4道名次应为5，实际 %d", entry4.Rank)
	}

	// 原始读数仍可查询且保留漏记标记
	var missed []TimingReading
	s.db.Where("lane_id = ? AND missed = ?", lane4, true).Find(&missed)
	if len(missed) == 0 {
		t.Fatal("漏记原始读数被覆盖或丢失")
	}
}

// 场景2：并列排序 —— 51.88 两条应并列第1，下一名次顺延为第3
func TestTieRanking(t *testing.T) {
	s := newTestServer(t)
	chief := loginToken(t, s, "chief")
	eventID, lanes := fixtureIDs(t, s)

	// 先处理第4道漏记案件以便发布
	var rc ReviewCase
	s.db.Where("lane_id = ? AND status = ?", lanes[4], "open").First(&rc)
	w := do(s, http.MethodPost, "/api/cases/"+itoa(rc.ID)+"/decision", chief, gin.H{
		"decision": "manual", "time_ms": 52940,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("裁定失败: %s", w.Body.String())
	}
	w = do(s, http.MethodPost, "/api/events/"+itoa(eventID)+"/publish", chief, gin.H{"change_note": "首发"})
	if w.Code != http.StatusOK {
		t.Fatalf("发布失败: %s", w.Body.String())
	}
	var pub struct {
		Board LeaderboardVersion `json:"board"`
	}
	decode(t, w, &pub)

	byLane := map[int]RankingEntry{}
	for _, e := range pub.Board.Entries {
		byLane[e.LaneNo] = e
	}
	l2, l6 := byLane[2], byLane[6]
	if l2.Rank != 1 || l6.Rank != 1 || l2.DisplayRank != "1=" || l6.DisplayRank != "1=" {
		t.Fatalf("第2、6道应并列第1(1=)，实际 %+v / %+v", l2, l6)
	}
	if l2.SpecialState != "tie" || l6.SpecialState != "tie" {
		t.Fatalf("并列行应标注 tie，实际 %s/%s", l2.SpecialState, l6.SpecialState)
	}
	// 次快 52.35（第1道）应为第3名
	if byLane[1].Rank != 3 || byLane[1].DisplayRank != "3" {
		t.Fatalf("并列表后名次应顺延，第1道应为第3，实际 rank=%d disp=%s",
			byLane[1].Rank, byLane[1].DisplayRank)
	}
}

// 场景3：更正后发布新版 —— 旧版保留、原始读数不被覆盖、更正行被标记
func TestCorrectionVersioning(t *testing.T) {
	s := newTestServer(t)
	chief := loginToken(t, s, "chief")
	eventID, lanes := fixtureIDs(t, s)

	// 第一次发布：第4道采用手动 52.94
	var rc ReviewCase
	s.db.Where("lane_id = ? AND status = ?", lanes[4], "open").First(&rc)
	do(s, http.MethodPost, "/api/cases/"+itoa(rc.ID)+"/decision", chief, gin.H{
		"decision": "manual", "time_ms": 52940, "note": "初判手记",
	})
	w := do(s, http.MethodPost, "/api/events/"+itoa(eventID)+"/publish", chief, gin.H{"change_note": "v1"})
	if w.Code != http.StatusOK {
		t.Fatalf("v1发布失败: %s", w.Body.String())
	}
	var v1 struct{ Board LeaderboardVersion }
	decode(t, w, &v1)

	// 设备补传第4道电子成绩（只追加新读数，不改旧读数）
	device := loginToken(t, s, "device")
	w = do(s, http.MethodPost, "/api/device/readings", device, gin.H{
		"event_id": eventID, "lane_no": 4, "split_index": 0,
		"time_ms": 51990, "device_raw": "PAD-BACKFILL-RECOVERED",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("补传电子读数失败: %s", w.Body.String())
	}
	// 总裁判重新复核并改采电子成绩
	do(s, http.MethodPost, "/api/cases/"+itoa(rc.ID)+"/reopen", chief, gin.H{})
	do(s, http.MethodPost, "/api/cases/"+itoa(rc.ID)+"/decision", chief, gin.H{
		"decision": "electronic", "note": "存储模块恢复，电子读数有效，改采电子51.99",
	})

	w = do(s, http.MethodPost, "/api/events/"+itoa(eventID)+"/publish", chief, gin.H{"change_note": "第4道改采恢复的电子成绩"})
	if w.Code != http.StatusOK {
		t.Fatalf("v2发布失败: %s", w.Body.String())
	}
	var v2 struct {
		Board      LeaderboardVersion `json:"board"`
		Correction string             `json:"correction"`
	}
	decode(t, w, &v2)
	if v2.Board.Version != 2 || !v2.Board.IsCurrent {
		t.Fatalf("应为第2版且 current，实际 v=%d current=%v", v2.Board.Version, v2.Board.IsCurrent)
	}
	var e4v2 RankingEntry
	s.db.Where("board_id = ? AND lane_id = ?", v2.Board.ID, lanes[4]).First(&e4v2)
	if e4v2.TimeText != "51.99" || !e4v2.Corrected || e4v2.Source != "electronic" {
		t.Fatalf("v2第4道应为被更正的电子51.99，实际 %+v", e4v2)
	}
	// 51.99 介于两个 51.88 之后 → 与原并列者不同，应为第3名
	if e4v2.Rank != 3 {
		t.Fatalf("改判后第4道应为第3名，实际 %d", e4v2.Rank)
	}

	// 旧版仍可查询且内容不变
	w = do(s, http.MethodGet, "/api/events/"+itoa(eventID)+"/boards/1", chief, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("查询旧版失败: %d", w.Code)
	}
	var old LeaderboardVersion
	decode(t, w, &old)
	if old.IsCurrent {
		t.Fatal("旧版不应再是 current")
	}
	var e4v1 RankingEntry
	s.db.Where("board_id = ? AND lane_id = ?", old.ID, lanes[4]).First(&e4v1)
	if e4v1.TimeText != "52.94" || e4v1.Source != "manual" {
		t.Fatalf("旧版第4道应保留手动52.94，实际 %+v", e4v1)
	}
	// 版本列表两条都在
	w = do(s, http.MethodGet, "/api/events/"+itoa(eventID)+"/boards", chief, nil)
	var list []map[string]any
	decode(t, w, &list)
	if len(list) != 2 {
		t.Fatalf("应保留2个历史版本，实际 %d", len(list))
	}
	// 原始读数全部保留：漏记1条+补传1条
	var cnt int64
	s.db.Model(&TimingReading{}).Where("lane_id = ? AND split_index = 0", lanes[4]).Count(&cnt)
	if cnt != 2 {
		t.Fatalf("第4道触壁原始读数应保留2条（漏记+补传），实际 %d", cnt)
	}
}

// 场景4：重赛结果与撤回状态
func TestSwimOffAndWithdraw(t *testing.T) {
	s := newTestServer(t)
	chief := loginToken(t, s, "chief")
	eventID, lanes := fixtureIDs(t, s)

	// 对并列的第2道裁定重赛
	var rc2 ReviewCase
	s.db.Where("lane_id = ?", lanes[2]).First(&rc2)
	if rc2.ID == 0 {
		// 种子未给第2道立案，则新开一案
		s.db.Create(&ReviewCase{EventID: eventID, LaneID: lanes[2], Reason: "并列重赛",
			Description: "第2、6道并列，按规则重赛", Status: "open"})
		s.db.Where("lane_id = ?", lanes[2]).First(&rc2)
	}
	w := do(s, http.MethodPost, "/api/cases/"+itoa(rc2.ID)+"/decision", chief, gin.H{
		"decision": "swimoff", "time_ms": 51210, "note": "重赛成绩51.21",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("重赛裁定失败: %s", w.Body.String())
	}
	// 重赛成绩必须提供
	var rc6 ReviewCase
	s.db.Where("lane_id = ?", lanes[6]).First(&rc6)
	if rc6.ID == 0 {
		s.db.Create(&ReviewCase{EventID: eventID, LaneID: lanes[6], Reason: "并列重赛",
			Description: "第6道重赛", Status: "open"})
		s.db.Where("lane_id = ?", lanes[6]).First(&rc6)
	}
	w = do(s, http.MethodPost, "/api/cases/"+itoa(rc6.ID)+"/decision", chief, gin.H{
		"decision": "swimoff",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("重赛缺少成绩应被400拒绝，实际 %d", w.Code)
	}
	do(s, http.MethodPost, "/api/cases/"+itoa(rc6.ID)+"/decision", chief, gin.H{
		"decision": "swimoff", "time_ms": 51640,
	})

	// 第8道因犯规撤回
	s.db.Create(&ReviewCase{EventID: eventID, LaneID: lanes[8], Reason: "犯规",
		Description: "出发抢跳", Status: "open"})
	var rc8 ReviewCase
	s.db.Where("lane_id = ? AND status = ?", lanes[8], "open").First(&rc8)
	do(s, http.MethodPost, "/api/cases/"+itoa(rc8.ID)+"/decision", chief, gin.H{
		"decision": "withdraw", "note": "确认出发犯规，成绩撤回",
	})

	// 第4道漏记也需处理才能发布
	var rc4 ReviewCase
	s.db.Where("lane_id = ? AND status = ?", lanes[4], "open").First(&rc4)
	do(s, http.MethodPost, "/api/cases/"+itoa(rc4.ID)+"/decision", chief, gin.H{
		"decision": "manual", "time_ms": 52940,
	})

	w = do(s, http.MethodPost, "/api/events/"+itoa(eventID)+"/publish", chief, gin.H{"change_note": "重赛与撤回"})
	if w.Code != http.StatusOK {
		t.Fatalf("发布失败: %s", w.Body.String())
	}
	var pub struct{ Board LeaderboardVersion }
	decode(t, w, &pub)
	byLane := map[int]RankingEntry{}
	for _, e := range pub.Board.Entries {
		byLane[e.LaneNo] = e
	}
	if byLane[2].SpecialState != "swimoff" || byLane[2].TimeText != "51.21" || byLane[2].Rank != 1 {
		t.Fatalf("第2道重赛51.21应为第1且标注swimoff，实际 %+v", byLane[2])
	}
	if byLane[6].Rank != 2 || byLane[6].SpecialState != "swimoff" {
		t.Fatalf("第6道重赛51.64应为第2，实际 %+v", byLane[6])
	}
	if byLane[8].SpecialState != "withdrawn" || byLane[8].DisplayRank != "撤回" {
		t.Fatalf("第8道应为撤回状态，实际 %+v", byLane[8])
	}
}

// 场景5：岗位权限矩阵
func TestRolePermissions(t *testing.T) {
	s := newTestServer(t)
	eventID, lanes := fixtureIDs(t, s)
	lane1 := lanes[1]

	type tc struct {
		name     string
		user     string
		method   string
		path     string
		body     any
		wantCode int
	}
	cases := []tc{
		{"未登录不能读赛事", "", http.MethodGet, "/api/meets", nil, http.StatusUnauthorized},
		{"录入员可补充手记", "clerk", http.MethodPost, "/api/lanes/" + itoa(lane1) + "/manual",
			gin.H{"time_ms": 60000, "note": "x"}, http.StatusOK},
		{"录入员不能立案", "clerk", http.MethodPost, "/api/events/" + itoa(eventID) + "/cases",
			gin.H{"lane_id": lane1, "reason": "争议"}, http.StatusForbidden},
		{"录入员不能裁定", "clerk", http.MethodPost, "/api/cases/1/decision",
			gin.H{"decision": "electronic"}, http.StatusForbidden},
		{"录入员不能发布", "clerk", http.MethodPost, "/api/events/" + itoa(eventID) + "/publish",
			gin.H{}, http.StatusForbidden},
		{"录入员不能建赛事", "clerk", http.MethodPost, "/api/meets",
			gin.H{"name": "x"}, http.StatusForbidden},
		{"裁判可立案", "judge", http.MethodPost, "/api/events/" + itoa(eventID) + "/cases",
			gin.H{"lane_id": lane1, "reason": "成绩争议", "description": "触板疑似延迟"}, http.StatusOK},
		{"裁判不能发布", "judge", http.MethodPost, "/api/events/" + itoa(eventID) + "/publish",
			gin.H{}, http.StatusForbidden},
		{"裁判不能建项目", "judge", http.MethodPost, "/api/meets/1/events",
			gin.H{"name": "x"}, http.StatusForbidden},
		{"设备不能发手记", "device", http.MethodPost, "/api/lanes/" + itoa(lane1) + "/manual",
			gin.H{"time_ms": 1, "note": "x"}, http.StatusForbidden},
		{"设备可上报读数", "device", http.MethodPost, "/api/device/readings",
			gin.H{"event_id": eventID, "lane_no": 1, "split_index": 1, "time_ms": 2500}, http.StatusOK},
		{"裁判不能冒充设备上报", "judge", http.MethodPost, "/api/device/readings",
			gin.H{"event_id": eventID, "lane_no": 1, "split_index": 1, "time_ms": 2500}, http.StatusForbidden},
		{"总裁判可建赛事", "chief", http.MethodPost, "/api/meets",
			gin.H{"name": "测试赛"}, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var token string
			if c.user != "" {
				token = loginToken(t, s, c.user)
			}
			w := do(s, c.method, c.path, token, c.body)
			if w.Code != c.wantCode {
				t.Fatalf("%s: 期望 %d，实际 %d，body=%s", c.name, c.wantCode, w.Code, w.Body.String())
			}
		})
	}
}

// 场景6：电子/手动/重赛三类裁定依据校验
func TestDecisionBasisValidation(t *testing.T) {
	s := newTestServer(t)
	chief := loginToken(t, s, "chief")
	_, lanes := fixtureIDs(t, s)

	// 第4道无有效电子读数，采电子应被拒绝（只有漏记记录）
	var rc4 ReviewCase
	s.db.Where("lane_id = ? AND status = ?", lanes[4], "open").First(&rc4)
	w := do(s, http.MethodPost, "/api/cases/"+itoa(rc4.ID)+"/decision", chief, gin.H{
		"decision": "electronic",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("无电子成绩采电子应被拒绝，实际 %d", w.Code)
	}

	// 手动成绩可直接给值
	w = do(s, http.MethodPost, "/api/cases/"+itoa(rc4.ID)+"/decision", chief, gin.H{
		"decision": "manual", "time_ms": 52940, "note": "手记",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("手动裁定失败: %s", w.Body.String())
	}

	// 撤回后再裁定应被拒绝
	do(s, http.MethodPost, "/api/cases/"+itoa(rc4.ID)+"/reopen", chief, gin.H{})
	do(s, http.MethodPost, "/api/cases/"+itoa(rc4.ID)+"/decision", chief, gin.H{"decision": "withdraw"})
	w = do(s, http.MethodPost, "/api/cases/"+itoa(rc4.ID)+"/decision", chief, gin.H{
		"decision": "manual", "time_ms": 52940,
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("已撤回案件再裁定应冲突，实际 %d", w.Code)
	}
}

func itoa(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
