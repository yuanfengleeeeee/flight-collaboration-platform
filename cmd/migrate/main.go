package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/common"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/config"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/store"
	"go.uber.org/zap"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	migrationsDir := flag.String("dir", "migrations/mysql", "迁移文件目录")
	command := flag.String("command", "status", "迁移命令: up/down/status")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fail("加载配置失败", err)
	}
	common.InitLogger(cfg.Log.Level, cfg.Log.Encoding, cfg.Log.Output)

	db, err := store.NewMySQL(cfg.MySQL)
	if err != nil {
		fail("连接 MySQL 失败", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		fail("获取 MySQL 连接失败", err)
	}
	defer sqlDB.Close()

	migrator := store.NewSQLMigrator(sqlDB, *migrationsDir)
	ctx := context.Background()
	switch *command {
	case "up":
		if err := migrator.Up(ctx); err != nil {
			fail("执行迁移失败", err)
		}
		fmt.Println("database migrations applied")
	case "down":
		if err := migrator.Down(ctx); err != nil {
			fail("回滚迁移失败", err)
		}
		fmt.Println("latest database migration rolled back")
	case "status":
		status, err := migrator.Status(ctx)
		if err != nil {
			fail("读取迁移状态失败", err)
		}
		for _, item := range status {
			state := "pending"
			if item.Applied {
				state = "applied"
			}
			fmt.Printf("%06d %-32s %s\n", item.Version, item.Name, state)
		}
		if len(status) == 0 {
			fmt.Println("no migrations found")
		}
	default:
		fail("不支持的迁移命令", fmt.Errorf("command=%s", *command))
	}
}

func fail(message string, err error) {
	common.L().Error(message, zap.Error(err))
	fmt.Fprintf(os.Stderr, "%s: %v\n", message, err)
	os.Exit(1)
}
