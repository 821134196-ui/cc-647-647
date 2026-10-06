package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// router 装配全部路由（main 与测试共用）
func (s *Server) router() *gin.Engine {
	r := gin.Default()
	api := r.Group("/api")
	{
		api.POST("/auth/login", s.login)

		authed := api.Group("")
		authed.Use(authRequired(s.db))
		{
			authed.GET("/me", s.me)

			authed.GET("/meets", s.listMeets)
			authed.GET("/events/:id", s.eventDetail)
			authed.GET("/events/:id/live", s.liveRanking)
			authed.GET("/events/:id/boards", s.listBoards)
			authed.GET("/events/:id/boards/:version", s.getBoard)

			// 赛事与项目编排：总裁判
			authed.POST("/meets", rolesRequired("chief"), s.createMeet)
			authed.POST("/meets/:id/events", rolesRequired("chief"), s.createEvent)
			authed.POST("/events/:id/lanes", rolesRequired("chief", "clerk"), s.addLanes)

			// 电子计时设备（本地模拟接口）
			authed.POST("/device/readings", rolesRequired("device", "chief"), s.uploadReading)

			// 手记与补充材料：裁判、录入员、总裁判均可（只作证据，不改名次）
			authed.POST("/lanes/:id/manual", rolesRequired("judge", "clerk", "chief"), s.addManual)

			// 立案 / 补充依据
			authed.POST("/events/:id/cases", rolesRequired("judge", "chief"), s.openCase)
			authed.POST("/cases/:id/append", rolesRequired("judge", "clerk", "chief"), s.appendCase)

			// 裁定与发布：仅总裁判
			authed.POST("/cases/:id/decision", rolesRequired("chief"), s.decideCase)
			authed.POST("/cases/:id/reopen", rolesRequired("chief"), s.reopenCase)
			authed.POST("/events/:id/publish", rolesRequired("chief"), s.publish)
		}
	}

	// 生产模式：前端已构建时由 Go 一并托管（可用 SWIM_FRONTEND_DIR 指定目录）
	distDir := os.Getenv("SWIM_FRONTEND_DIR")
	if distDir == "" {
		distDir = "frontend/dist"
	}
	if _, err := os.Stat(filepath.Join(distDir, "index.html")); err == nil {
		r.Static("/assets", filepath.Join(distDir, "assets"))
		r.StaticFile("/favicon.svg", filepath.Join(distDir, "favicon.svg"))
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
				return
			}
			c.File(filepath.Join(distDir, "index.html"))
		})
	}
	return r
}
