package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"timingreview/internal/device"
	"timingreview/internal/models"
	"timingreview/internal/store"
)

const (
	tokClerk = "token-clerk-001"
	tokJudge = "token-judge-001"
	tokChief = "token-chief-001"
)

type testEnv struct {
	t   *testing.T
	srv *Server
	r   http.Handler
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	srv := NewServer(db, device.New())
	return &testEnv{t: t, srv: srv, r: srv.Router()}
}

func (e *testEnv) do(method, path, token string, body interface{}) (int, map[string]interface{}) {
	e.t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	var out map[string]interface{}
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &out)
	}
	return w.Code, out
}

func (e *testEnv) doRaw(method, path, token string, body interface{}) (int, []byte) {
	e.t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	return w.Code, w.Body.Bytes()
}

func (e *testEnv) mustOK(code int, out map[string]interface{}, where ...string) {
	e.t.Helper()
	if code != http.StatusOK {
		e.t.Fatalf("%v: 期望 200，实际 %d：%v", where, code, out["error"])
	}
}

func (e *testEnv) raceIDByCode(code string) int {
	code2, b := e.doRaw("GET", "/api/meets", tokChief, nil)
	if code2 != http.StatusOK {
		e.t.Fatalf("查询项目失败: %d %s", code2, b)
	}
	var groups []map[string]interface{}
	if err := json.Unmarshal(b, &groups); err != nil {
		e.t.Fatalf("解析项目列表失败: %v", err)
	}
	for _, g := range groups {
		for _, rr := range g["races"].([]interface{}) {
			r := rr.(map[string]interface{})
			if r["code"] == code {
				return int(r["id"].(float64))
			}
		}
	}
	e.t.Fatalf("未找到项目 %s", code)
	return 0
}

type laneView struct {
	raw map[string]interface{}
}

func (e *testEnv) raceDetail(raceID int, token string) map[string]interface{} {
	code, out := e.do("GET", "/api/races/"+strconv.Itoa(raceID), token, nil)
	e.mustOK(code, out, "项目详情")
	return out
}

func findLane(lanes []interface{}, laneNo int) map[string]interface{} {
	for _, l := range lanes {
		m := l.(map[string]interface{})
		entry := m["entry"].(map[string]interface{})
		if int(entry["lane_no"].(float64)) == laneNo {
			return m
		}
	}
	return nil
}

func laneEntryID(l map[string]interface{}) int {
	return int(l["entry"].(map[string]interface{})["id"].(float64))
}

func laneRank(l map[string]interface{}) *int {
	if l["rank"] == nil {
		return nil
	}
	r := int(l["rank"].(float64))
	return &r
}

func laneFlags(l map[string]interface{}) []string {
	out := []string{}
	if l["flags"] == nil {
		return out
	}
	for _, f := range l["flags"].([]interface{}) {
		out = append(out, f.(string))
	}
	return out
}

func hasStrFlag(flags []string, f string) bool {
	for _, x := range flags {
		if x == f {
			return true
		}
	}
	return false
}

// TestMissedTouchReview 漏记 → 复核 → 手动裁定全链路 + 发布拦截
func TestMissedTouchReview(t *testing.T) {
	e := newTestEnv(t)
	rid := e.raceIDByCode("M101")
	idStr := strconv.Itoa(rid)

	// 录入员导入第 1 批设备读数
	code, out := e.do("POST", "/api/races/"+idStr+"/readings/import", tokClerk, map[string]interface{}{})
	e.mustOK(code, out, "导入读数")
	if int(out["import_batch"].(float64)) != 1 {
		t.Fatalf("应为第 1 批")
	}

	detail := e.raceDetail(rid, tokJudge)
	lanes := detail["lanes"].([]interface{})
	lane3 := findLane(lanes, 3)
	if lane3["latest_reading"] == nil {
		t.Fatal("3 道应有读数")
	}
	rd := lane3["latest_reading"].(map[string]interface{})
	if rd["status"] != models.ReadingMissed {
		t.Fatalf("3 道应为漏记，实际 %v", rd["status"])
	}
	if rd["finish_time"] != nil {
		t.Fatal("漏记报文不应有触壁时间")
	}
	if lane3["resolved_seconds"] != nil || laneRank(lane3) != nil || !hasStrFlag(laneFlags(lane3), models.FlagMissed) {
		t.Fatal("漏记泳道应无成绩、无名次并标记 MISSED")
	}

	// 未裁定漏记前禁止发布榜单
	code, out = e.do("POST", "/api/races/"+idStr+"/boards", tokChief, map[string]interface{}{})
	if code != http.StatusBadRequest {
		t.Fatalf("存在未处理漏记时发布应被拒绝，实际 %d %v", code, out["error"])
	}

	// 裁判提交手记并发起漏记复核
	l3id := laneEntryID(lane3)
	code, out = e.do("POST", "/api/races/"+idStr+"/lanes/"+strconv.Itoa(l3id)+"/notes", tokJudge,
		map[string]interface{}{"content": "三块边道秒表 55.66/55.68/55.67，录像吻合", "manual_finish": 55.67})
	e.mustOK(code, out, "裁判手记")

	code, out = e.do("POST", "/api/races/"+idStr+"/cases", tokJudge, map[string]interface{}{
		"lane_entry_id": l3id, "reason_type": models.ReasonMissedTouch,
		"summary": "3 道触板无响应，仅有分段 26.88",
	})
	e.mustOK(code, out, "发起漏记复核")
	caseID := int(out["case"].(map[string]interface{})["id"].(float64))

	// 总裁判裁定采用手动成绩（未填成绩 → 自动取最新裁判手记 55.67）
	code, out = e.do("POST", "/api/cases/"+strconv.Itoa(caseID)+"/rulings", tokChief, map[string]interface{}{
		"decision": models.DecisionManual, "rationale": "录像与边道秒表一致，触板漏记，依规程采用手动成绩",
	})
	e.mustOK(code, out, "手动裁定")

	detail = e.raceDetail(rid, tokChief)
	lane3 = findLane(detail["lanes"].([]interface{}), 3)
	if lane3["source"] != models.SourceManual {
		t.Fatalf("应采用手动成绩，实际 %v", lane3["source"])
	}
	if lane3["resolved_seconds"].(float64) != 55.67 {
		t.Fatalf("手动成绩应为 55.67，实际 %v", lane3["resolved_seconds"])
	}
	if lane3["display_time"] != "55.67" {
		t.Fatalf("展示时间错误：%v", lane3["display_time"])
	}
	if hasStrFlag(laneFlags(lane3), models.FlagMissed) {
		t.Fatal("裁定后不应再标记 MISSED")
	}
	if r := laneRank(lane3); r == nil || *r != 4 {
		// 54.97 / 55.32 / 55.32 / 55.67 → 第 4 名
		t.Fatalf("手动成绩 55.67 应排第 4，实际 %v", r)
	}
}

// TestTieRankingAndSwimOff 并列同名次（1224 排名）→ 重赛决出名次
func TestTieRankingAndSwimOff(t *testing.T) {
	e := newTestEnv(t)
	rid := e.raceIDByCode("M101")
	idStr := strconv.Itoa(rid)
	e.mustOK(e.do("POST", "/api/races/"+idStr+"/readings/import", tokClerk, nil))

	detail := e.raceDetail(rid, tokClerk)
	lanes := detail["lanes"].([]interface{})
	l5, l6 := findLane(lanes, 5), findLane(lanes, 6)
	if r5, r6 := laneRank(l5), laneRank(l6); r5 == nil || r6 == nil || *r5 != 2 || *r6 != 2 {
		t.Fatalf("55.32 两道应并列第 2，实际 %v %v", r5, r6)
	}
	if !hasStrFlag(laneFlags(l5), models.FlagTie) || !hasStrFlag(laneFlags(l6), models.FlagTie) {
		t.Fatal("并列两道都应标记 TIE")
	}
	// 标准竞赛排名 1224：并列后的下一名是第 4
	l1 := findLane(lanes, 1) // 55.84
	if r := laneRank(l1); r == nil || *r != 4 {
		t.Fatalf("并列后下一名应为第 4，实际 %v", r)
	}
	l8 := findLane(lanes, 8) // 56.99
	if r := laneRank(l8); r == nil || *r != 6 {
		t.Fatalf("56.99 应为第 6 名，实际 %v", r)
	}

	// 裁判发起并列复核，总裁判裁定重赛
	e.mustOK(e.do("POST", "/api/races/"+idStr+"/cases", tokJudge, map[string]interface{}{
		"lane_entry_id": 0, "reason_type": models.ReasonTie,
		"summary": "5、6 道电子成绩相同 55.32，按规程重赛",
	}))
	detail = e.raceDetail(rid, tokChief)
	var tieCaseID int
	for _, c := range detail["cases"].([]interface{}) {
		cm := c.(map[string]interface{})
		if cm["reason_type"] == models.ReasonTie && cm["status"] == models.CaseOpen {
			tieCaseID = int(cm["id"].(float64))
		}
	}
	if tieCaseID == 0 {
		t.Fatal("未找到待裁定的并列案件")
	}
	code, out := e.do("POST", "/api/cases/"+strconv.Itoa(tieCaseID)+"/rulings", tokChief, map[string]interface{}{
		"decision":       models.DecisionSwimOff,
		"rationale":      "并列成绩 55.32，安排重赛",
		"lane_entry_ids": []int{laneEntryID(l5), laneEntryID(l6)},
	})
	e.mustOK(code, out, "重赛裁定")

	// 重赛已创建但尚未导入：回退显示电子成绩，提示重赛待进行
	detail = e.raceDetail(rid, tokChief)
	l5 = findLane(detail["lanes"].([]interface{}), 5)
	if l5["source"] != models.SourceElectronic || l5["note"] != "并列重赛待进行" {
		t.Fatalf("重赛未进行时应回退显示电子成绩并提示，实际 source=%v note=%v", l5["source"], l5["note"])
	}

	// 侧边栏出现重赛项目；录入员导入重赛读数
	soID := e.raceIDByCode("M101-SO1")
	e.mustOK(e.do("POST", "/api/races/"+strconv.Itoa(soID)+"/readings/import", tokClerk, nil))

	detail = e.raceDetail(rid, tokChief)
	lanes = detail["lanes"].([]interface{})
	l5, l6 = findLane(lanes, 5), findLane(lanes, 6)
	// 设备模拟重赛：55.32-0.18=55.14，55.32+0.16=55.48
	if l5["source"] != models.SourceSwimOff || l5["resolved_seconds"].(float64) != 55.14 {
		t.Fatalf("5 道应采用重赛成绩 55.14，实际 %v %v", l5["source"], l5["resolved_seconds"])
	}
	if l6["source"] != models.SourceSwimOff || l6["resolved_seconds"].(float64) != 55.48 {
		t.Fatalf("6 道应采用重赛成绩 55.48，实际 %v %v", l6["source"], l6["resolved_seconds"])
	}
	r5, r6 := laneRank(l5), laneRank(l6)
	if r5 == nil || r6 == nil || *r5 == *r6 {
		t.Fatalf("重赛后应分出先后名次，实际 %v %v", r5, r6)
	}
	if hasStrFlag(laneFlags(l5), models.FlagTie) {
		t.Fatal("重赛决出后不应再标 TIE")
	}
	if !hasStrFlag(laneFlags(l5), models.FlagSwimOff) {
		t.Fatal("应标记 SWIMOFF")
	}
}

// TestRolePermissions 岗位权限矩阵
func TestRolePermissions(t *testing.T) {
	e := newTestEnv(t)
	rid := e.raceIDByCode("M101")
	idStr := strconv.Itoa(rid)
	e.mustOK(e.do("POST", "/api/races/"+idStr+"/readings/import", tokClerk, nil))
	detail := e.raceDetail(rid, tokChief)
	l3 := findLane(detail["lanes"].([]interface{}), 3)
	l3id := laneEntryID(l3)

	// 未认证 → 401
	if code, _ := e.do("GET", "/api/meets", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("未带令牌应 401，实际 %d", code)
	}
	// 录入员：不能提交手记成绩
	if code, out := e.do("POST", "/api/races/"+idStr+"/lanes/"+strconv.Itoa(l3id)+"/notes", tokClerk,
		map[string]interface{}{"manual_finish": 55.67, "content": "x"}); code != http.StatusForbidden {
		t.Fatalf("录入员提交手记成绩应 403，实际 %d %v", code, out["error"])
	}
	// 录入员：可以只补文字材料
	e.mustOK(e.do("POST", "/api/races/"+idStr+"/lanes/"+strconv.Itoa(l3id)+"/notes", tokClerk,
		map[string]interface{}{"content": "终点录像已存档待查"}))
	// 录入员：不能发起复核
	if code, _ := e.do("POST", "/api/races/"+idStr+"/cases", tokClerk, map[string]interface{}{
		"lane_entry_id": l3id, "reason_type": models.ReasonMissedTouch, "summary": "x"}); code != http.StatusForbidden {
		t.Fatalf("录入员发起复核应 403，实际 %d", code)
	}
	// 裁判：可以发起复核
	e.mustOK(e.do("POST", "/api/races/"+idStr+"/cases", tokJudge, map[string]interface{}{
		"lane_entry_id": l3id, "reason_type": models.ReasonMissedTouch, "summary": "触板漏记"}))
	detail = e.raceDetail(rid, tokJudge)
	var caseID int
	for _, c := range detail["cases"].([]interface{}) {
		cm := c.(map[string]interface{})
		if cm["lane_entry_id"] != nil && int(cm["lane_entry_id"].(float64)) == l3id && cm["status"] == models.CaseOpen {
			caseID = int(cm["id"].(float64))
		}
	}
	// 裁判：不能终局裁定
	if code, _ := e.do("POST", "/api/cases/"+strconv.Itoa(caseID)+"/rulings", tokJudge, map[string]interface{}{
		"decision": models.DecisionManual, "rationale": "x"}); code != http.StatusForbidden {
		t.Fatalf("裁判终局裁定应 403，实际 %d", code)
	}
	// 裁判：不能发布榜单
	if code, _ := e.do("POST", "/api/races/"+idStr+"/boards", tokJudge, map[string]interface{}{}); code != http.StatusForbidden {
		t.Fatalf("裁判发布榜单应 403，实际 %d", code)
	}
	// 裁判：不能撤回
	if code, _ := e.do("POST", "/api/races/"+idStr+"/lanes/"+strconv.Itoa(l3id)+"/withdraw", tokJudge,
		map[string]interface{}{"reason": "x"}); code != http.StatusForbidden {
		t.Fatalf("裁判撤回应 403，实际 %d", code)
	}
	// 总裁判：裁定无依据 → 400
	if code, _ := e.do("POST", "/api/cases/"+strconv.Itoa(caseID)+"/rulings", tokChief, map[string]interface{}{
		"decision": models.DecisionManual, "rationale": "  "}); code != http.StatusBadRequest {
		t.Fatalf("裁定无依据应 400，实际 %d", code)
	}
	// 总裁判：正常裁定
	e.mustOK(e.do("POST", "/api/cases/"+strconv.Itoa(caseID)+"/rulings", tokChief, map[string]interface{}{
		"decision": models.DecisionElectronic, "rationale": "复核后确认电子读数有效"}))
}

// TestBoardVersionsAndRawHistory 榜单版本化、撤回标记、原始读数历史不覆盖
func TestBoardVersionsAndRawHistory(t *testing.T) {
	e := newTestEnv(t)

	// --- M101：初版（含并列）→ 重赛后更正 ---
	rid := e.raceIDByCode("M101")
	idStr := strconv.Itoa(rid)
	e.mustOK(e.do("POST", "/api/races/"+idStr+"/readings/import", tokClerk, nil))
	// 先解决 3 道漏记，使初版可发布
	detail := e.raceDetail(rid, tokChief)
	l3 := findLane(detail["lanes"].([]interface{}), 3)
	l3id := laneEntryID(l3)
	e.mustOK(e.do("POST", "/api/races/"+idStr+"/lanes/"+strconv.Itoa(l3id)+"/notes", tokJudge,
		map[string]interface{}{"content": "手记 55.67", "manual_finish": 55.67}))
	e.mustOK(e.do("POST", "/api/races/"+idStr+"/cases", tokJudge, map[string]interface{}{
		"lane_entry_id": l3id, "reason_type": models.ReasonMissedTouch, "summary": "漏记"}))
	detail = e.raceDetail(rid, tokJudge)
	var c3 int
	for _, c := range detail["cases"].([]interface{}) {
		cm := c.(map[string]interface{})
		if cm["status"] == models.CaseOpen {
			c3 = int(cm["id"].(float64))
		}
	}
	e.mustOK(e.do("POST", "/api/cases/"+strconv.Itoa(c3)+"/rulings", tokChief, map[string]interface{}{
		"decision": models.DecisionManual, "rationale": "采用边道手记"}))

	// 发布 V1
	code, out := e.do("POST", "/api/races/"+idStr+"/boards", tokChief, map[string]interface{}{})
	e.mustOK(code, out, "发布V1")
	v1ID := int(out["board_id"].(float64))
	if int(out["version_no"].(float64)) != 1 {
		t.Fatal("应为第 1 版")
	}

	// V1 快照：5、6 道并列第 2
	code, bv1 := e.do("GET", "/api/boards/"+strconv.Itoa(v1ID), tokClerk, nil)
	e.mustOK(code, bv1, "查看V1")
	v1Entries := bv1["board"].(map[string]interface{})["entries"].([]interface{})
	var v1l5, v1l8 map[string]interface{}
	for _, en := range v1Entries {
		m := en.(map[string]interface{})
		if int(m["lane_no"].(float64)) == 5 {
			v1l5 = m
		}
		if int(m["lane_no"].(float64)) == 8 {
			v1l8 = m
		}
	}
	if v1l5["rank"].(float64) != 2 || v1l5["flags"] != models.FlagTie {
		t.Fatalf("V1 中 5 道应为并列第2，实际 rank=%v flags=%v", v1l5["rank"], v1l5["flags"])
	}
	if v1l8["source"] != models.SourceElectronic || v1l8["rank"] == nil {
		t.Fatalf("V1 中 8 道应为电子有效成绩，实际 %+v", v1l8)
	}

	// 更正发布必须填写原因
	if code, _ := e.do("POST", "/api/races/"+idStr+"/boards", tokChief, map[string]interface{}{}); code != http.StatusBadRequest {
		t.Fatalf("更正发布无原因应 400，实际 %d", code)
	}

	// 8 道撤回后发布 V2
	e.mustOK(e.do("POST", "/api/races/"+idStr+"/lanes/"+strconv.Itoa(laneEntryID(findLane(e.raceDetail(rid, tokChief)["lanes"].([]interface{}), 8)))+"/withdraw",
		tokChief, map[string]interface{}{"reason": "检录点名三次未到"}))
	code, out = e.do("POST", "/api/races/"+idStr+"/boards", tokChief, map[string]interface{}{
		"correction_reason": "8 道弃权撤回",
	})
	e.mustOK(code, out, "发布V2")
	v2ID := int(out["board_id"].(float64))

	// 版本列表：两版都在，V1 已置 SUPERSEDED
	code, listBytes := e.doRaw("GET", "/api/races/"+idStr+"/boards", tokClerk, nil)
	if code != http.StatusOK {
		t.Fatalf("版本列表应 200，实际 %d", code)
	}
	var listArr []map[string]interface{}
	if err := json.Unmarshal(listBytes, &listArr); err != nil {
		t.Fatalf("解析版本列表失败: %v", err)
	}
	bStatus := map[int]string{}
	for _, m := range listArr {
		bStatus[int(m["version_no"].(float64))] = m["status"].(string)
	}
	if bStatus[1] != models.BoardSuperseded || bStatus[2] != models.BoardCurrent {
		t.Fatalf("版本状态错误：%v", bStatus)
	}

	// V1 旧快照内容不变（8 道仍有电子成绩）
	_, bv1 = e.do("GET", "/api/boards/"+strconv.Itoa(v1ID), tokClerk, nil)
	for _, en := range bv1["board"].(map[string]interface{})["entries"].([]interface{}) {
		m := en.(map[string]interface{})
		if int(m["lane_no"].(float64)) == 8 {
			if m["rank"] == nil || m["source"] != models.SourceElectronic {
				t.Fatalf("V1 历史快照不得被覆盖，8 道应仍有名次/电子成绩，实际 %+v", m)
			}
		}
	}
	// V2 中 8 道撤回、无名次
	_, bv2 := e.do("GET", "/api/boards/"+strconv.Itoa(v2ID), tokClerk, nil)
	for _, en := range bv2["board"].(map[string]interface{})["entries"].([]interface{}) {
		m := en.(map[string]interface{})
		if int(m["lane_no"].(float64)) == 8 {
			if m["rank"] != nil || m["flags"] != models.FlagWithdrawn {
				t.Fatalf("V2 中 8 道应撤回无名次，实际 rank=%v flags=%v", m["rank"], m["flags"])
			}
		}
	}

	// --- M102：设备补发后重新导入，旧批次漏记读数仍可查询 ---
	rid2 := e.raceIDByCode("M102")
	id2 := strconv.Itoa(rid2)
	e.mustOK(e.do("POST", "/api/races/"+id2+"/readings/import", tokClerk, nil)) // 第 1 批（4 道 PARTIAL）
	l4 := findLane(e.raceDetail(rid2, tokClerk)["lanes"].([]interface{}), 4)
	if l4["latest_reading"].(map[string]interface{})["status"] != models.ReadingPartial {
		t.Fatal("M102 4 道初始应为 PARTIAL")
	}
	e.mustOK(e.do("POST", "/api/device/sessions/M102/retransmit", tokClerk, map[string]interface{}{
		"lane": 4, "finish_time": 151.9}))
	e.mustOK(e.do("POST", "/api/races/"+id2+"/readings/import", tokClerk, nil)) // 第 2 批

	code, raw := e.do("GET", "/api/races/"+id2+"/readings", tokClerk, nil)
	e.mustOK(code, raw, "原始读数历史")
	var l4rows []map[string]interface{}
	for _, r := range raw["readings"].([]interface{}) {
		m := r.(map[string]interface{})
		if int(m["lane_no"].(float64)) == 4 {
			l4rows = append(l4rows, m)
		}
	}
	if len(l4rows) != 2 {
		t.Fatalf("4 道应有两批历史读数，实际 %d", len(l4rows))
	}
	b1, b2 := l4rows[0], l4rows[1]
	if b1["status"] != models.ReadingPartial || b1["finish_time"] != nil {
		t.Fatalf("第 1 批漏记读数应原样保留，实际 %+v", b1)
	}
	if b2["status"] != models.ReadingOK || b2["finish_time"].(float64) != 151.9 {
		t.Fatalf("第 2 批应为补发成功 151.90，实际 %+v", b2)
	}
}
