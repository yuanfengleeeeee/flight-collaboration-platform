package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const migrationTable = "schema_migrations"

var migrationFilePattern = regexp.MustCompile(`^(\d+)_([a-zA-Z0-9_-]+)\.(up|down)\.sql$`)

// Migration 描述一个可升级/回滚的数据库版本。
type Migration struct {
	Version  uint64
	Name     string
	UpPath   string
	DownPath string
}

// MigrationStatus 描述一个迁移文件当前是否已执行。
type MigrationStatus struct {
	Version uint64
	Name    string
	Applied bool
}

// SQLMigrator 执行根目录 migrations/mysql 下的版本化 SQL。
type SQLMigrator struct {
	db  *sql.DB
	dir string
}

// NewSQLMigrator 创建 SQL 迁移器。
func NewSQLMigrator(db *sql.DB, dir string) *SQLMigrator {
	return &SQLMigrator{db: db, dir: dir}
}

// DiscoverMigrations 读取并校验迁移文件，要求每个版本同时具备 up/down 文件。
func DiscoverMigrations(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读取迁移目录失败: %w", err)
	}

	byVersion := make(map[uint64]*Migration)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := migrationFilePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}

		version, err := strconv.ParseUint(match[1], 10, 64)
		if err != nil || version == 0 {
			return nil, fmt.Errorf("迁移版本无效: %s", entry.Name())
		}
		migration, ok := byVersion[version]
		if !ok {
			migration = &Migration{Version: version, Name: match[2]}
			byVersion[version] = migration
		} else if migration.Name != match[2] {
			return nil, fmt.Errorf("迁移版本 %d 的名称不一致", version)
		}

		path := filepath.Join(dir, entry.Name())
		if match[3] == "up" {
			if migration.UpPath != "" {
				return nil, fmt.Errorf("迁移版本 %d 存在多个 up 文件", version)
			}
			migration.UpPath = path
		} else {
			if migration.DownPath != "" {
				return nil, fmt.Errorf("迁移版本 %d 存在多个 down 文件", version)
			}
			migration.DownPath = path
		}
	}

	migrations := make([]Migration, 0, len(byVersion))
	for _, migration := range byVersion {
		if migration.UpPath == "" || migration.DownPath == "" {
			return nil, fmt.Errorf("迁移版本 %d(%s) 缺少 up 或 down 文件", migration.Version, migration.Name)
		}
		migrations = append(migrations, *migration)
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	return migrations, nil
}

// Up 执行所有尚未应用的迁移。
func (m *SQLMigrator) Up(ctx context.Context) error {
	if err := m.ensureMigrationTable(ctx); err != nil {
		return err
	}
	migrations, err := DiscoverMigrations(m.dir)
	if err != nil {
		return err
	}
	current, err := m.currentVersion(ctx)
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		if migration.Version <= current {
			continue
		}
		if migration.Version != current+1 {
			return fmt.Errorf("迁移版本不连续: 当前 %d, 下一个应为 %d, 实际为 %d", current, current+1, migration.Version)
		}
		if err := m.apply(ctx, migration, true); err != nil {
			return err
		}
		current = migration.Version
	}
	return nil
}

// Down 回滚最近一次已应用的迁移。
func (m *SQLMigrator) Down(ctx context.Context) error {
	if err := m.ensureMigrationTable(ctx); err != nil {
		return err
	}
	migrations, err := DiscoverMigrations(m.dir)
	if err != nil {
		return err
	}
	current, err := m.currentVersion(ctx)
	if err != nil {
		return err
	}
	if current == 0 {
		return nil
	}

	var target *Migration
	for i := range migrations {
		if migrations[i].Version == current {
			target = &migrations[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("找不到当前迁移版本 %d 的文件", current)
	}
	return m.apply(ctx, *target, false)
}

// Status 返回所有迁移的当前状态。
func (m *SQLMigrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	if err := m.ensureMigrationTable(ctx); err != nil {
		return nil, err
	}
	migrations, err := DiscoverMigrations(m.dir)
	if err != nil {
		return nil, err
	}

	rows, err := m.db.QueryContext(ctx, "SELECT version FROM "+migrationTable)
	if err != nil {
		return nil, fmt.Errorf("查询迁移状态失败: %w", err)
	}
	defer rows.Close()

	applied := make(map[uint64]bool)
	for rows.Next() {
		var version uint64
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("读取迁移状态失败: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历迁移状态失败: %w", err)
	}

	status := make([]MigrationStatus, 0, len(migrations))
	for _, migration := range migrations {
		status = append(status, MigrationStatus{
			Version: migration.Version,
			Name:    migration.Name,
			Applied: applied[migration.Version],
		})
	}
	return status, nil
}

func (m *SQLMigrator) ensureMigrationTable(ctx context.Context) error {
	if m.db == nil {
		return errors.New("数据库连接不能为空")
	}
	const query = "CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT UNSIGNED NOT NULL PRIMARY KEY, applied_at DATETIME(3) NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci"
	if _, err := m.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("创建迁移状态表失败: %w", err)
	}
	return nil
}

func (m *SQLMigrator) currentVersion(ctx context.Context) (uint64, error) {
	var version sql.NullInt64
	if err := m.db.QueryRowContext(ctx, "SELECT MAX(version) FROM "+migrationTable).Scan(&version); err != nil {
		return 0, fmt.Errorf("查询当前迁移版本失败: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}
	return uint64(version.Int64), nil
}

func (m *SQLMigrator) apply(ctx context.Context, migration Migration, up bool) error {
	path := migration.DownPath
	if up {
		path = migration.UpPath
	}
	script, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取迁移文件 %s 失败: %w", path, err)
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启迁移事务失败: %w", err)
	}
	defer tx.Rollback()

	if err := execSQLScript(ctx, tx, string(script)); err != nil {
		return fmt.Errorf("执行迁移 %d(%s) 失败: %w", migration.Version, migration.Name, err)
	}
	if up {
		_, err = tx.ExecContext(ctx, "INSERT INTO "+migrationTable+" (version, applied_at) VALUES (?, CURRENT_TIMESTAMP(3))", migration.Version)
	} else {
		_, err = tx.ExecContext(ctx, "DELETE FROM "+migrationTable+" WHERE version = ?", migration.Version)
	}
	if err != nil {
		return fmt.Errorf("更新迁移状态失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交迁移事务失败: %w", err)
	}
	return nil
}

type sqlExecer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func execSQLScript(ctx context.Context, execer sqlExecer, script string) error {
	for _, statement := range splitSQLStatements(script) {
		if _, err := execer.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func splitSQLStatements(script string) []string {
	parts := strings.Split(script, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		statement := strings.TrimSpace(part)
		if statement != "" {
			statements = append(statements, statement)
		}
	}
	return statements
}
