package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// User 系统用户。role: chief(总裁判) / judge(裁判) / clerk(录入员) / device(电子计时设备)
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"uniqueIndex;not null" json:"username"`
	Password     string    `json:"-"`
	Name         string    `json:"name"`
	Role         string    `gorm:"not null" json:"role"`
	RegisterTime time.Time `json:"-"`
}

var tokenSecret = []byte("swim-review-local-secret-2026")

type tokenPayload struct {
	UID      uint   `json:"uid"`
	Username string `json:"u"`
	Role     string `json:"r"`
	Name     string `json:"n"`
}

func signToken(p tokenPayload) string {
	body, _ := json.Marshal(p)
	b := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, tokenSecret)
	mac.Write([]byte(b))
	s := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return b + "." + s
}

func parseToken(t string) (tokenPayload, bool) {
	var p tokenPayload
	parts := strings.SplitN(t, ".", 2)
	if len(parts) != 2 {
		return p, false
	}
	mac := hmac.New(sha256.New, tokenSecret)
	mac.Write([]byte(parts[0]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[1])) {
		return p, false
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return p, false
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return p, false
	}
	return p, true
}

// currentUser 从上下文取出当前登录用户
func currentUser(c *gin.Context) tokenPayload {
	v, ok := c.Get("user")
	if !ok {
		return tokenPayload{}
	}
	return v.(tokenPayload)
}

// authRequired 必须登录
func authRequired(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		h = strings.TrimPrefix(h, "Bearer ")
		p, ok := parseToken(h)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录或令牌无效"})
			return
		}
		var u User
		if err := db.First(&u, p.UID).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
			return
		}
		c.Set("user", p)
		c.Next()
	}
}

// rolesRequired 限制岗位
func rolesRequired(roles ...string) gin.HandlerFunc {
	allow := map[string]bool{}
	for _, r := range roles {
		allow[r] = true
	}
	return func(c *gin.Context) {
		u := currentUser(c)
		if !allow[u.Role] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "当前岗位无权执行该操作", "role": u.Role})
			return
		}
		c.Next()
	}
}
