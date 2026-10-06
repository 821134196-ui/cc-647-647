package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"timingreview/internal/models"
)

type ctxKey string

const userCtxKey = "currentUser"

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// AuthMiddleware 校验 Bearer Token，注入当前用户
func AuthMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		token := strings.TrimPrefix(h, "Bearer ")
		token = strings.TrimSpace(token)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "缺少登录令牌"})
			return
		}
		var u models.User
		if err := db.Where("token = ?", token).First(&u).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "令牌无效"})
			return
		}
		c.Set(userCtxKey, &u)
		c.Next()
	}
}

func currentUser(c *gin.Context) *models.User {
	v, ok := c.Get(userCtxKey)
	if !ok {
		return nil
	}
	return v.(*models.User)
}

// RequireRoles 岗位校验
func RequireRoles(roles ...string) gin.HandlerFunc {
	allow := map[string]bool{}
	for _, r := range roles {
		allow[r] = true
	}
	return func(c *gin.Context) {
		u := currentUser(c)
		if u == nil || !allow[u.Role] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "权限不足：该操作仅限 " + strings.Join(roles, " / ") + " 岗位",
				"role":  uRole(u),
				"need":  roles,
			})
			return
		}
		c.Next()
	}
}

func uRole(u *models.User) string {
	if u == nil {
		return ""
	}
	return u.Role
}
