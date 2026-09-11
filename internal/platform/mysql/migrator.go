package mysql

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

var migrationPattern = regexp.MustCompile(`^(\d+)_([a-zA-Z0-9_-]+)\.(up|down)\.sql$`)

type Migration struct {
	Version  uint64
	Name     string
	UpPath   string
	DownPath string
}

type Status struct {
	Version uint64
	Name    string
	Applied bool
}

type Migrator struct {
	db  *sql.DB
	dir string
}

func NewMigrator(db *sql.DB, dir string) *Migrator { return &Migrator{db: db, dir: dir} }

func Discover(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migration directory %s: %w", dir, err)
	}
	byVersion := make(map[uint64]*Migration)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := migrationPattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		version, err := strconv.ParseUint(match[1], 10, 64)
		if err != nil || version == 0 {
			return nil, fmt.Errorf("invalid migration version in %s", entry.Name())
		}
		migration := byVersion[version]
		if migration == nil {
			migration = &Migration{Version: version, Name: match[2]}
			byVersion[version] = migration
		} else if migration.Name != match[2] {
			return nil, fmt.Errorf("migration version %d has inconsistent names", version)
		}
		path := filepath.Join(dir, entry.Name())
		if match[3] == "up" {
			if migration.UpPath != "" {
				return nil, fmt.Errorf("migration version %d has duplicate up files", version)
			}
			migration.UpPath = path
		} else {
			if migration.DownPath != "" {
				return nil, fmt.Errorf("migration version %d has duplicate down files", version)
			}
			migration.DownPath = path
		}
	}
	result := make([]Migration, 0, len(byVersion))
	for _, migration := range byVersion {
		if migration.UpPath == "" || migration.DownPath == "" {
			return nil, fmt.Errorf("migration %d(%s) must have up and down files", migration.Version, migration.Name)
		}
		result = append(result, *migration)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version < result[j].Version })
	return result, nil
}

func (m *Migrator) Up(ctx context.Context) error {
	if err := m.ensureTable(ctx); err != nil {
		return err
	}
	migrations, err := Discover(m.dir)
	if err != nil {
		return err
	}
	current, err := m.current(ctx)
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		if migration.Version <= current {
			continue
		}
		if migration.Version != current+1 {
			return fmt.Errorf("migration sequence gap: current %d, next %d, found %d", current, current+1, migration.Version)
		}
		if err := m.apply(ctx, migration, true); err != nil {
			return err
		}
		current = migration.Version
	}
	return nil
}

// Down is intentionally not called by service startup. The command layer must
// require an explicit destructive-operation flag before invoking it.
func (m *Migrator) Down(ctx context.Context) error {
	if err := m.ensureTable(ctx); err != nil {
		return err
	}
	migrations, err := Discover(m.dir)
	if err != nil {
		return err
	}
	current, err := m.current(ctx)
	if err != nil {
		return err
	}
	if current == 0 {
		return nil
	}
	for _, migration := range migrations {
		if migration.Version == current {
			return m.apply(ctx, migration, false)
		}
	}
	return fmt.Errorf("migration file for current version %d not found", current)
}

func (m *Migrator) Status(ctx context.Context) ([]Status, error) {
	if err := m.ensureTable(ctx); err != nil {
		return nil, err
	}
	migrations, err := Discover(m.dir)
	if err != nil {
		return nil, err
	}
	rows, err := m.db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("read migration status: %w", err)
	}
	defer rows.Close()
	applied := make(map[uint64]bool)
	for rows.Next() {
		var version uint64
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan migration status: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migration status: %w", err)
	}
	result := make([]Status, 0, len(migrations))
	for _, migration := range migrations {
		result = append(result, Status{Version: migration.Version, Name: migration.Name, Applied: applied[migration.Version]})
	}
	return result, nil
}

func (m *Migrator) ensureTable(ctx context.Context) error {
	if m == nil || m.db == nil {
		return errors.New("migration database is nil")
	}
	const query = `CREATE TABLE IF NOT EXISTS schema_migrations (
version BIGINT UNSIGNED NOT NULL PRIMARY KEY,
applied_at DATETIME(6) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`
	if _, err := m.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	return nil
}

func (m *Migrator) current(ctx context.Context) (uint64, error) {
	var version sql.NullInt64
	if err := m.db.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		return 0, fmt.Errorf("read current migration: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}
	return uint64(version.Int64), nil
}

func (m *Migrator) apply(ctx context.Context, migration Migration, up bool) error {
	path := migration.DownPath
	if up {
		path = migration.UpPath
	}
	script, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", path, err)
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer tx.Rollback()
	for _, statement := range splitStatements(string(script)) {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("execute migration %d(%s): %w", migration.Version, migration.Name, err)
		}
	}
	if up {
		_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES (?, UTC_TIMESTAMP(6))", migration.Version)
	} else {
		_, err = tx.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = ?", migration.Version)
	}
	if err != nil {
		return fmt.Errorf("record migration %d: %w", migration.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", migration.Version, err)
	}
	return nil
}

func splitStatements(script string) []string {
	parts := strings.Split(script, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		if statement := strings.TrimSpace(part); statement != "" {
			statements = append(statements, statement)
		}
	}
	return statements
}
