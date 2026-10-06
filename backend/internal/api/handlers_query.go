package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"timingreview/internal/models"
	"timingreview/internal/ranking"
)

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请求格式错误")
		return
	}
	var u models.User
	if err := s.DB.Where("username = ?", req.Username).First(&u).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	if u.PasswordSHA != sha(req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	s.audit(&u, "LOGIN", "用户登录")
	c.JSON(http.StatusOK, gin.H{
		"token": u.Token,
		"user":  gin.H{"id": u.ID, "name": u.Name, "username": u.Username, "role": u.Role},
	})
}

func (s *Server) me(c *gin.Context) {
	u := currentUser(c)
	c.JSON(http.StatusOK, gin.H{"id": u.ID, "name": u.Name, "username": u.Username, "role": u.Role})
}

func (s *Server) listMeets(c *gin.Context) {
	var meets []models.Meet
	s.DB.Order("id asc").Find(&meets)
	type raceRow struct {
		models.Race
		HasReadings bool `json:"has_readings"`
	}
	out := []gin.H{}
	for _, m := range meets {
		var races []models.Race
		s.DB.Where("meet_id = ?", m.ID).Order("code asc").Find(&races)
		rRows := []gin.H{}
		for _, r := range races {
			var cnt int64
			s.DB.Model(&models.TimingReading{}).Where("race_id = ?", r.ID).Count(&cnt)
			rRows = append(rRows, gin.H{
				"id": r.ID, "meet_id": r.MeetID, "code": r.Code,
				"event_name": r.EventName, "distance": r.Distance,
				"stroke": r.Stroke, "round": r.Round,
				"parent_race_id": r.ParentRaceID, "has_readings": cnt > 0,
			})
		}
		out = append(out, gin.H{"meet": m, "races": rRows})
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) loadRace(c *gin.Context) (*models.Race, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badRequest(c, "项目编号错误")
		return nil, false
	}
	var race models.Race
	if err := s.DB.First(&race, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "项目不存在"})
		return nil, false
	}
	return &race, true
}

func (s *Server) raceDetail(c *gin.Context) {
	race, ok := s.loadRace(c)
	if !ok {
		return
	}
	lanes, err := ranking.ResolveRace(s.DB, *race)
	if err != nil {
		serverError(c, err)
		return
	}
	var notes []models.ManualNote
	s.DB.Where("race_id = ?", race.ID).Order("created_at asc").Find(&notes)
	var cases []models.ReviewCase
	s.DB.Preload("Rulings").Where("race_id = ?", race.ID).Order("created_at asc").Find(&cases)

	c.JSON(http.StatusOK, gin.H{
		"race":  race,
		"lanes": lanes,
		"notes": notes,
		"cases": cases,
	})
}

// rawReadings 返回全部批次的原始读数（只读，含漏记报文）
func (s *Server) rawReadings(c *gin.Context) {
	race, ok := s.loadRace(c)
	if !ok {
		return
	}
	var readings []models.TimingReading
	s.DB.Where("race_id = ?", race.ID).Order("import_batch asc, lane_no asc").Find(&readings)
	batches := map[int]string{}
	if len(readings) > 0 {
		var distinct []int
		s.DB.Model(&models.TimingReading{}).Where("race_id = ?", race.ID).
			Distinct("import_batch").Pluck("import_batch", &distinct)
		for _, b := range distinct {
			var ts []models.TimingReading
			s.DB.Where("race_id = ? AND import_batch = ?", race.ID, b).Order("recorded_at asc").Limit(1).Find(&ts)
			if len(ts) > 0 {
				batches[b] = ts[0].RecordedAt.Format("2006-01-02 15:04:05")
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"readings": readings, "batches": batches})
}

func (s *Server) deviceSessions(c *gin.Context) {
	c.JSON(http.StatusOK, s.Device.Sessions())
}
