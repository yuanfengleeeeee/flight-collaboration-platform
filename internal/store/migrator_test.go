package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverMigrations(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"000002_add_index.up.sql",
		"000002_add_index.down.sql",
		"000001_initial_schema.up.sql",
		"000001_initial_schema.down.sql",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1;"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	migrations, err := DiscoverMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 || migrations[0].Version != 1 || migrations[1].Version != 2 {
		t.Fatalf("unexpected migrations: %+v", migrations)
	}
}

func TestDiscoverMigrationsRequiresUpAndDown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "000001_initial_schema.up.sql"), []byte("SELECT 1;"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverMigrations(dir); err == nil {
		t.Fatal("expected missing down migration error")
	}
}

func TestSplitSQLStatements(t *testing.T) {
	statements := splitSQLStatements("CREATE TABLE a (id INT);\n\nCREATE TABLE b (id INT);")
	if len(statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(statements))
	}
}
