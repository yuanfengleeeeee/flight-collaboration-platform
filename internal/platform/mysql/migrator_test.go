package mysql

import (
	"path/filepath"
	"testing"
)

func TestDiscoverCoreAndEdgeMigrations(t *testing.T) {
	wantByTarget := map[string][]uint64{
		"core": {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13},
		"edge": {1, 2, 3, 4, 5, 6, 7},
	}
	for _, target := range []string{"core", "edge"} {
		t.Run(target, func(t *testing.T) {
			migrations, err := Discover(filepath.Join("..", "..", "..", "migrations", target, "mysql"))
			if err != nil {
				t.Fatal(err)
			}
			wantVersions := wantByTarget[target]
			if len(migrations) != len(wantVersions) {
				t.Fatalf("unexpected migrations: %#v", migrations)
			}
			for index, migration := range migrations {
				if migration.Version != wantVersions[index] || migration.UpPath == "" || migration.DownPath == "" {
					t.Fatalf("unexpected migration at index %d: %#v", index, migration)
				}
			}
		})
	}
}

func TestSplitStatements(t *testing.T) {
	statements := splitStatements("CREATE TABLE one (id INT);\n\n CREATE TABLE two (id INT); ")
	if len(statements) != 2 {
		t.Fatalf("got %d statements, want 2", len(statements))
	}
}
