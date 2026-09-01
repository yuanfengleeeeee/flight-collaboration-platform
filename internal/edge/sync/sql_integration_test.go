package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	platformmysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/mysql"
)

func TestTaskProjectionSQLVersionRulesAgainstDocker(t *testing.T) {
	if os.Getenv("FLIGHT_RUN_BVS2_EDGE_DB_TEST") != "1" {
		t.Skip("set FLIGHT_RUN_BVS2_EDGE_DB_TEST=1 to run against an isolated Edge MySQL")
	}
	db, err := platformmysql.Open(config.DatabaseConfig{Host: envOrDefault("FLIGHT_EDGE_BVS2_DB_HOST", "127.0.0.1"), Port: envInt(t, "FLIGHT_EDGE_BVS2_DB_PORT", 3331), Username: envOrDefault("FLIGHT_EDGE_BVS2_DB_USERNAME", "edge_app"), Password: envOrDefault("FLIGHT_EDGE_BVS2_DB_PASSWORD", "change-me"), Database: envOrDefault("FLIGHT_EDGE_BVS2_DB_DATABASE", "flight_edge"), MaxIdleConns: 2, MaxOpenConns: 4, ConnMaxLifetime: 300})
	if err != nil {
		t.Fatal(err)
	}
	defer platformmysql.Close(db)
	if err := platformmysql.Ping(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	store := NewSQLStore(db)
	projection := TaskProjection{PublicID: "task-sql-1", AssignmentPublicID: "assignment-sql-1", EmployeePublicID: "employee-sql-1", FlightDisplayNo: "CA-SQL", TaskName: "SQL projection", AreaName: "A1", PlannedAt: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC), Status: "assigned", BusinessStatus: "assigned", Message: "assigned", SyncVersion: 1}
	if err := store.UpsertTaskProjection(context.Background(), projection); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertTaskProjection(context.Background(), projection); err != nil {
		t.Fatalf("same SQL projection should be idempotent: %v", err)
	}
	if err := store.UpsertTaskProjection(context.Background(), TaskProjection{PublicID: projection.PublicID, AssignmentPublicID: projection.AssignmentPublicID, EmployeePublicID: projection.EmployeePublicID, Status: "assigned", BusinessStatus: "assigned", Message: projection.Message, SyncVersion: 0}); err == nil {
		t.Fatal("zero-version projection should be rejected")
	}
	conflicting := projection
	conflicting.Message = "conflicting payload"
	if !errors.Is(store.UpsertTaskProjection(context.Background(), conflicting), ErrProjectionVersionConflict) {
		t.Fatal("expected same-version SQL projection conflict")
	}
	values, err := store.ListTaskProjections(context.Background(), projection.EmployeePublicID)
	if err != nil || len(values) != 1 || values[0].AssignmentPublicID != projection.AssignmentPublicID {
		t.Fatalf("unexpected SQL projection: %#v err=%v", values, err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(t *testing.T, key string, fallback int) int {
	t.Helper()
	if value := os.Getenv(key); value != "" {
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
			t.Fatalf("invalid %s=%q", key, value)
		}
		return parsed
	}
	return fallback
}
