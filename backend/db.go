package main

import (
	"log"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func initDB(path string) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	if err := db.AutoMigrate(
		&User{}, &Meet{}, &Event{}, &Lane{}, &TimingReading{},
		&ManualRecord{}, &ReviewCase{}, &RankingEntry{}, &LeaderboardVersion{},
	); err != nil {
		log.Fatalf("自动迁移失败: %v", err)
	}
	seed(db)
	return db
}
