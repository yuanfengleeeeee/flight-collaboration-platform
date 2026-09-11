// Package mysql owns database connectivity and explicit SQL migrations.
package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func Open(cfg config.DatabaseConfig) (*gorm.DB, error) {
	db, err := gorm.Open(mysqlDriver.Open(cfg.DSN()), &gorm.Config{TranslateError: true})
	if err != nil {
		return nil, fmt.Errorf("open mysql %s/%s: %w", cfg.Host, cfg.Database, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get mysql sql db: %w", err)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)
	}
	return db, nil
}

func Ping(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("mysql db is nil")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get mysql sql db: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping mysql: %w", err)
	}
	return nil
}

func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get mysql sql db: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close mysql: %w", err)
	}
	return nil
}

func SQLDB(db *gorm.DB) (*sql.DB, error) {
	if db == nil {
		return nil, fmt.Errorf("mysql db is nil")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get mysql sql db: %w", err)
	}
	return sqlDB, nil
}
