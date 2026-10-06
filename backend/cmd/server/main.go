package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"timingreview/internal/api"
	"timingreview/internal/device"
	"timingreview/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP 监听地址")
	dbPath := flag.String("db", "data/timing.db", "SQLite 数据库路径")
	flag.Parse()

	if dir := filepath.Dir(*dbPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("创建数据目录失败: %v", err)
		}
	}
	db, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	if err := store.Seed(db); err != nil {
		log.Fatalf("初始化数据失败: %v", err)
	}
	dev := device.New()
	srv := api.NewServer(db, dev)

	log.Printf("游泳计时复核系统已启动：http://localhost%s", *addr)
	log.Printf("前端页面与 API 同端口；账号 clerk/judge/chief，密码 clerk123/judge123/chief123")
	if err := srv.Router().Run(*addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
