package store

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"timingreview/internal/models"
)

func Open(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			logger.Config{
				SlowThreshold:             time.Second,
				LogLevel:                  logger.Warn,
				IgnoreRecordNotFoundError: true,
			},
		),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&models.User{}, &models.Meet{}, &models.Swimmer{}, &models.Race{},
		&models.LaneEntry{}, &models.TimingReading{}, &models.ManualNote{},
		&models.ReviewCase{}, &models.Ruling{}, &models.Board{},
		&models.BoardEntry{}, &models.AuditLog{},
	); err != nil {
		return nil, err
	}
	return db, nil
}

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// Seed 首次启动写入演示数据：比赛、项目、泳道、三个岗位账号。
func Seed(db *gorm.DB) error {
	var n int64
	db.Model(&models.Meet{}).Count(&n)
	if n > 0 {
		return nil
	}
	now := time.Now()
	users := []models.User{
		{Username: "clerk", PasswordSHA: sha("clerk123"), Name: "王小录", Role: models.RoleClerk, Token: "token-clerk-001"},
		{Username: "judge", PasswordSHA: sha("judge123"), Name: "李判", Role: models.RoleJudge, Token: "token-judge-001"},
		{Username: "chief", PasswordSHA: sha("chief123"), Name: "张总裁", Role: models.RoleChief, Token: "token-chief-001"},
	}
	if err := db.Create(&users).Error; err != nil {
		return err
	}
	meet := models.Meet{Name: "2026 年全市青少年游泳锦标赛", Venue: "市体育中心游泳馆", Date: "2026-10-06", CreatedAt: now}
	if err := db.Create(&meet).Error; err != nil {
		return err
	}
	swimmers := []models.Swimmer{
		{Name: "陈一", Team: "海豚队"},
		{Name: "林二", Team: "飞鱼队"},
		{Name: "黄三", Team: "海豚队"},
		{Name: "周四", Team: "蓝鲸队"},
		{Name: "吴五", Team: "飞鱼队"},
		{Name: "赵六", Team: "蓝鲸队"},
		{Name: "钱七", Team: "白鸥队"},
		{Name: "孙八", Team: "白鸥队"},
	}
	if err := db.Create(&swimmers).Error; err != nil {
		return err
	}
	race1 := models.Race{MeetID: meet.ID, Code: "M101", EventName: "男子 100 米自由泳", Distance: 100, Stroke: "自由泳", Round: "决赛", CreatedAt: now}
	race2 := models.Race{MeetID: meet.ID, Code: "M102", EventName: "女子 200 米蛙泳", Distance: 200, Stroke: "蛙泳", Round: "预赛", CreatedAt: now}
	if err := db.Create(&race1).Error; err != nil {
		return err
	}
	if err := db.Create(&race2).Error; err != nil {
		return err
	}
	for i := 0; i < 8; i++ {
		e := models.LaneEntry{RaceID: race1.ID, LaneNo: i + 1, SwimmerID: swimmers[i].ID, Status: models.EntryActive, CreatedAt: now, UpdatedAt: now}
		if err := db.Create(&e).Error; err != nil {
			return err
		}
	}
	// M102 复用前六名运动员
	for i := 0; i < 6; i++ {
		e := models.LaneEntry{RaceID: race2.ID, LaneNo: i + 1, SwimmerID: swimmers[(i+2)%8].ID, Status: models.EntryActive, CreatedAt: now, UpdatedAt: now}
		if err := db.Create(&e).Error; err != nil {
			return err
		}
	}
	log.Printf("[seed] 已写入演示数据：账号 clerk/judge/chief（密码分别为 clerk123/judge123/chief123），项目 M101/M102")
	return nil
}
