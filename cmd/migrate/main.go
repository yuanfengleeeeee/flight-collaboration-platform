package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	platformlogger "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/logger"
	platformmysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/mysql"
	"go.uber.org/zap"
)

func main() {
	// 迁移是独立命令，不在 API/Worker 启动时隐式执行；target 决定使用
	// Core 还是 Edge 的数据库和 migration 目录。
	configPath := flag.String("config", "configs/config.v2.yaml", "Architecture v2 config path")
	target := flag.String("target", "", "database target: core or edge")
	command := flag.String("command", "status", "migration command: up, down or status")
	dir := flag.String("dir", "", "optional migration directory override")
	allowDestructive := flag.Bool("allow-destructive", false, "explicitly allow destructive down migration")
	flag.Parse()

	if *target != "core" && *target != "edge" {
		fail(nil, "target must be core or edge")
	}
	if *command == "down" && !*allowDestructive {
		fail(nil, "down requires -allow-destructive and must only run against an isolated database")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fail(err, "load config")
	}
	log, err := platformlogger.New(cfg.Log)
	if err != nil {
		fail(err, "initialize logger")
	}
	defer log.Sync()

	dbConfig := cfg.Core.DB
	defaultDir := filepath.Join("migrations", "core", "mysql")
	if *target == "edge" {
		dbConfig = cfg.Edge.DB
		defaultDir = filepath.Join("migrations", "edge", "mysql")
	}
	if *dir != "" {
		defaultDir = *dir
	}

	db, err := platformmysql.Open(dbConfig)
	if err != nil {
		fail(err, "open database")
	}
	sqlDB, err := platformmysql.SQLDB(db)
	if err != nil {
		fail(err, "get sql database")
	}
	defer sqlDB.Close()

	migrator := platformmysql.NewMigrator(sqlDB, defaultDir)
	ctx := context.Background()
	switch *command {
	case "up":
		err = migrator.Up(ctx)
	case "down":
		err = migrator.Down(ctx)
	case "status":
		var status []platformmysql.Status
		status, err = migrator.Status(ctx)
		if err == nil {
			for _, item := range status {
				state := "pending"
				if item.Applied {
					state = "applied"
				}
				fmt.Printf("%06d %-36s %s\n", item.Version, item.Name, state)
			}
		}
	default:
		fail(nil, "command must be up, down or status")
	}
	if err != nil {
		fail(err, "migration command failed")
	}
	log.Info("migration command completed", zap.String("target", *target), zap.String("command", *command), zap.String("directory", defaultDir))
}

func fail(err error, message string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", message, err)
	} else {
		fmt.Fprintln(os.Stderr, message)
	}
	os.Exit(1)
}
