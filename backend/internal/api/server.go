package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"timingreview/internal/device"
	"timingreview/internal/models"
)

type Server struct {
	DB     *gorm.DB
	Device *device.Device
}

func NewServer(db *gorm.DB, dev *device.Device) *Server {
	return &Server{DB: db, Device: dev}
}

// webDist 定位前端构建产物：run.sh 从仓库根或 backend 目录启动的两种情况
func webDist() string {
	for _, p := range []string{"./web/dist", "../web/dist"} {
		if _, err := os.Stat(filepath.Join(p, "index.html")); err == nil {
			return p
		}
	}
	return "./web/dist"
}

func (s *Server) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())

	dist := webDist()
	r.Static("/assets", filepath.Join(dist, "assets"))
	r.StaticFile("/", filepath.Join(dist, "index.html"))
	r.NoRoute(func(c *gin.Context) {
		if len(c.Request.URL.Path) >= 5 && c.Request.URL.Path[:5] == "/api/" {
			c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
			return
		}
		c.File(filepath.Join(dist, "index.html"))
	})

	api := r.Group("/api")
	{
		api.POST("/auth/login", s.login)

		auth := api.Group("")
		auth.Use(AuthMiddleware(s.DB))
		{
			auth.GET("/me", s.me)
			auth.GET("/meets", s.listMeets)
			auth.GET("/races/:id", s.raceDetail)
			auth.GET("/races/:id/readings", s.rawReadings)
			auth.GET("/races/:id/boards", s.listBoards)
			auth.GET("/boards/:id", s.boardDetail)
			auth.GET("/device/sessions", s.deviceSessions)

			// 录入员：导入设备读数 / 请求设备补发
			clerk := auth.Group("")
			clerk.Use(RequireRoles(models.RoleClerk))
			clerk.POST("/races/:id/readings/import", s.importReadings)
			clerk.POST("/device/sessions/:code/retransmit", s.deviceRetransmit)

			// 裁判 / 总裁判：手记、补充材料（录入员可提交无成绩材料）
			auth.POST("/races/:id/lanes/:laneId/notes", s.addNote)

			// 裁判、总裁判：发起复核
			judge := auth.Group("")
			judge.Use(RequireRoles(models.RoleJudge, models.RoleChief))
			judge.POST("/races/:id/cases", s.openCase)

			// 总裁判：裁定、撤回、重赛、发布榜单
			chief := auth.Group("")
			chief.Use(RequireRoles(models.RoleChief))
			chief.POST("/cases/:id/rulings", s.addRuling)
			chief.POST("/races/:id/lanes/:laneId/withdraw", s.withdraw)
			chief.POST("/races/:id/boards", s.publishBoard)
		}
	}
	return r
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func (s *Server) audit(u *models.User, action, detail string) {
	s.DB.Create(&models.AuditLog{
		UserID: u.ID, Name: u.Name, Role: u.Role, Action: action, Detail: detail,
	})
}

func badRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, gin.H{"error": msg})
}

func serverError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
