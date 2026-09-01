package mysql_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	coremysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/adapter/mysql"
	coreapp "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application"
	flighttask "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flighttask"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/iam"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	platformmysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/mysql"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	sharedEvent "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
	"gorm.io/gorm"
)

// This test is opt-in because the normal unit-test command must not require a
// developer's local Docker services. It targets the isolated Core MySQL
// container used by the Architecture v2 local compose stack.
func TestFlightTaskSQLAgainstDocker(t *testing.T) {
	if os.Getenv("FLIGHT_RUN_BVS2_DB_TEST") != "1" {
		t.Skip("set FLIGHT_RUN_BVS2_DB_TEST=1 to run against the isolated Core MySQL")
	}

	db, err := platformmysql.Open(config.DatabaseConfig{
		Host:            envOrDefault("FLIGHT_BVS2_DB_HOST", "127.0.0.1"),
		Port:            envIntOrDefault(t, "FLIGHT_BVS2_DB_PORT", 3310),
		Username:        envOrDefault("FLIGHT_BVS2_DB_USERNAME", "core_app"),
		Password:        envOrDefault("FLIGHT_BVS2_DB_PASSWORD", "change-me"),
		Database:        envOrDefault("FLIGHT_BVS2_DB_DATABASE", "flight_core"),
		MaxIdleConns:    2,
		MaxOpenConns:    5,
		ConnMaxLifetime: 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := platformmysql.Ping(context.Background(), db); err != nil {
		_ = platformmysql.Close(db)
		t.Fatal(err)
	}

	fixture, err := seedFlightTaskFixture(db)
	if err != nil {
		_ = platformmysql.Close(db)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		fixture.cleanup(db)
		if err := platformmysql.Close(db); err != nil {
			t.Logf("close Core MySQL: %v", err)
		}
	})

	service := flighttask.NewService(coremysql.NewFlightTaskRepository(db), fixedClock{value: fixture.now})
	input := flighttask.ArrivalInput{
		FlightPublicID:  fixture.flightPublicID,
		SourceEventID:   fixture.sourceEventID,
		OccurredAt:      fixture.actualArrivalAt,
		ActualArrivalAt: fixture.actualArrivalAt,
		Source:          "docker-integration-test",
		ActorType:       "machine",
		ActorPublicID:   fixture.actorPublicID,
		RequestID:       fixture.requestID,
		TraceID:         fixture.traceID,
		SourceIP:        "127.0.0.1",
	}

	result, err := service.RecordFlightArrived(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.ResultCode != flighttask.ResultCreated || result.Duplicate || result.CandidateCount != 1 || result.TaskPublicID == "" {
		t.Fatalf("unexpected first result: %#v", result)
	}

	queryService := flighttask.NewTaskQueryService(coremysql.NewFlightTaskRepository(db), iam.NewAuthorizer())
	queryPrincipal := security.Principal{Type: security.HumanPrincipal, PublicID: newID(), Roles: []string{security.RoleLeader}, Scopes: security.AccessScope{Global: true}}
	listed, err := queryService.ListTasks(context.Background(), queryPrincipal, flighttask.TaskQueryFilter{Status: string(taskmodule.StatusAwaitingConfirmation), FlightPublicID: fixture.flightPublicID, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if listed.Total != 1 || len(listed.Items) != 1 || len(listed.Items[0].Candidates) != 1 || listed.Items[0].Candidates[0].PersonnelPublicID != fixture.eligiblePersonnelPublicID {
		t.Fatalf("unexpected task query result: %#v", listed)
	}
	queried, err := queryService.GetTask(context.Background(), queryPrincipal, result.TaskPublicID)
	if err != nil {
		t.Fatal(err)
	}
	if queried.PublicID != result.TaskPublicID || queried.Assignment != nil {
		t.Fatalf("unexpected task detail before confirmation: %#v", queried)
	}

	var flightStatus string
	if err := db.Table("flight").Select("status").Where("public_id = ?", fixture.flightPublicID).Scan(&flightStatus).Error; err != nil {
		t.Fatal(err)
	}
	if flightStatus != "arrived" {
		t.Fatalf("flight status = %q, want arrived", flightStatus)
	}

	var candidateRows []struct {
		PersonnelPublicID string `gorm:"column:personnel_public_id"`
		Rank              int    `gorm:"column:candidate_rank"`
	}
	if err := db.Table("task_candidate AS tc").
		Select("p.public_id AS personnel_public_id, tc.candidate_rank").
		Joins("JOIN personnel AS p ON p.id = tc.personnel_id").
		Where("tc.task_id = (SELECT id FROM task_instance WHERE public_id = ?)", result.TaskPublicID).
		Order("tc.candidate_rank ASC").Find(&candidateRows).Error; err != nil {
		t.Fatal(err)
	}
	if len(candidateRows) != 1 || candidateRows[0].PersonnelPublicID != fixture.eligiblePersonnelPublicID || candidateRows[0].Rank != 1 {
		t.Fatalf("unexpected SQL candidates: %#v", candidateRows)
	}

	assertCount(t, db, "task_instance", "public_id = ?", 1, result.TaskPublicID)
	assertCount(t, db, "task_candidate", "task_id = (SELECT id FROM task_instance WHERE public_id = ?)", 1, result.TaskPublicID)
	assertCount(t, db, "flight_status_history", "flight_id = (SELECT id FROM flight WHERE public_id = ?)", 1, fixture.flightPublicID)
	assertCount(t, db, "task_status_history", "task_id = (SELECT id FROM task_instance WHERE public_id = ?)", 1, result.TaskPublicID)
	assertCount(t, db, "audit_log", "resource_id = ? AND action = ?", 1, result.TaskPublicID, "flight.arrived")
	assertCount(t, db, "outbox_event", "aggregate_id = ? AND event_type = ?", 1, result.TaskPublicID, "task.generated.v1")
	assertCount(t, db, "business_idempotency_record", "operation_type = ? AND idempotency_key = ?", 1, flighttask.OperationFlightArrived, fixture.sourceEventID)

	duplicate, err := service.RecordFlightArrived(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate.Duplicate || duplicate.TaskPublicID != result.TaskPublicID || duplicate.ResultCode != result.ResultCode {
		t.Fatalf("unexpected duplicate result: %#v", duplicate)
	}
	assertCount(t, db, "task_instance", "public_id = ?", 1, result.TaskPublicID)
	assertCount(t, db, "outbox_event", "aggregate_id = ? AND event_type = ?", 1, result.TaskPublicID, "task.generated.v1")

	confirmationID := "bvs204-confirm-" + newID()
	leaderPublicID := newID()
	confirmationService := flighttask.NewConfirmationService(coremysql.NewFlightTaskRepository(db), iam.NewAuthorizer(), fixedClock{value: fixture.now})
	confirmation, err := confirmationService.ConfirmTask(context.Background(), flighttask.ConfirmationInput{
		Principal:    security.Principal{Type: security.HumanPrincipal, PublicID: leaderPublicID, Roles: []string{security.RoleLeader}, Scopes: security.AccessScope{Global: true}},
		TaskPublicID: result.TaskPublicID, CandidatePublicID: result.Candidates[0].PublicID, ConfirmationID: confirmationID,
		ExpectedTaskVersion: 0, RequestID: newID(), TraceID: newID(), SourceIP: "127.0.0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if confirmation.ResultCode != flighttask.ResultTaskAssigned || confirmation.Duplicate || confirmation.AssignmentPublicID == "" {
		t.Fatalf("unexpected leader confirmation result: %#v", confirmation)
	}
	queried, err = queryService.GetTask(context.Background(), queryPrincipal, result.TaskPublicID)
	if err != nil {
		t.Fatal(err)
	}
	if queried.Status != string(taskmodule.StatusAssigned) || queried.Assignment == nil || queried.Assignment.PublicID != confirmation.AssignmentPublicID {
		t.Fatalf("unexpected task detail after confirmation: %#v", queried)
	}
	assertCount(t, db, "task_instance", "public_id = ? AND status = ? AND status_version = ?", 1, result.TaskPublicID, "assigned", 1)
	assertCount(t, db, "task_candidate", "task_id = (SELECT id FROM task_instance WHERE public_id = ?) AND status = ?", 1, result.TaskPublicID, "selected")
	assertCount(t, db, "task_assignment", "public_id = ? AND status = ?", 1, confirmation.AssignmentPublicID, "confirmed")
	assertCount(t, db, "personnel", "public_id = ? AND work_state = ? AND status_version = ?", 1, fixture.eligiblePersonnelPublicID, "reserved", 1)
	assertCount(t, db, "task_assignment_status_history", "assignment_id = (SELECT id FROM task_assignment WHERE public_id = ?)", 1, confirmation.AssignmentPublicID)
	assertCount(t, db, "personnel_status_history", "assignment_id = (SELECT id FROM task_assignment WHERE public_id = ?)", 1, confirmation.AssignmentPublicID)
	assertCount(t, db, "audit_log", "resource_id = ? AND action = ? AND result = ?", 1, result.TaskPublicID, "task.confirm", "assigned")
	assertCount(t, db, "outbox_event", "aggregate_id = ? AND event_type = ?", 1, result.TaskPublicID, "task.assigned.v1")
	assertCount(t, db, "business_idempotency_record", "operation_type = ? AND idempotency_key = ?", 1, flighttask.OperationTaskConfirm, confirmationID)

	replayed, err := confirmationService.ConfirmTask(context.Background(), flighttask.ConfirmationInput{
		Principal:    security.Principal{Type: security.HumanPrincipal, PublicID: leaderPublicID, Roles: []string{security.RoleLeader}, Scopes: security.AccessScope{Global: true}},
		TaskPublicID: result.TaskPublicID, CandidatePublicID: result.Candidates[0].PublicID, ConfirmationID: confirmationID,
		ExpectedTaskVersion: 0, RequestID: newID(), TraceID: newID(), SourceIP: "127.0.0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Duplicate || replayed.AssignmentPublicID != confirmation.AssignmentPublicID {
		t.Fatalf("unexpected confirmation replay: %#v", replayed)
	}

	repository := coremysql.NewFlightTaskRepository(db)
	acceptPayload := struct {
		AssignmentPublicID  string    `json:"assignment_public_id"`
		ExpectedSyncVersion uint64    `json:"expected_sync_version"`
		ClientOccurredAt    time.Time `json:"client_occurred_at"`
	}{AssignmentPublicID: confirmation.AssignmentPublicID, ExpectedSyncVersion: confirmation.SyncVersion, ClientOccurredAt: fixture.now}
	acceptCommand, err := sharedEvent.NewCommand(flighttask.CommandEmployeeAcceptTask, fixture.eligiblePersonnelPublicID, result.TaskPublicID, fixture.traceID, acceptPayload)
	if err != nil {
		t.Fatal(err)
	}
	duplicateCommand, err := repository.ProcessCommand(context.Background(), acceptCommand, coreapp.HandleCommand)
	if err != nil || duplicateCommand {
		t.Fatalf("unexpected employee accept result duplicate=%v err=%v", duplicateCommand, err)
	}
	assertCount(t, db, "task_instance", "public_id = ? AND status = ? AND status_version = ? AND sync_version = ?", 1, result.TaskPublicID, "in_progress", 2, 2)
	assertCount(t, db, "task_assignment", "public_id = ? AND status = ? AND status_version = ?", 1, confirmation.AssignmentPublicID, "accepted", 1)
	assertCount(t, db, "personnel", "public_id = ? AND work_state = ? AND status_version = ?", 1, fixture.eligiblePersonnelPublicID, "busy", 2)

	duplicateCommand, err = repository.ProcessCommand(context.Background(), acceptCommand, coreapp.HandleCommand)
	if err != nil || !duplicateCommand {
		t.Fatalf("employee accept replay duplicate=%v err=%v", duplicateCommand, err)
	}
	assertCount(t, db, "task_assignment_status_history", "assignment_id = (SELECT id FROM task_assignment WHERE public_id = ?)", 2, confirmation.AssignmentPublicID)
	assertCount(t, db, "outbox_event", "aggregate_id = ? AND event_type = ?", 1, result.TaskPublicID, flighttask.EventTaskAccepted)

	completePayload := struct {
		AssignmentPublicID  string    `json:"assignment_public_id"`
		ExpectedSyncVersion uint64    `json:"expected_sync_version"`
		ClientOccurredAt    time.Time `json:"client_occurred_at"`
	}{AssignmentPublicID: confirmation.AssignmentPublicID, ExpectedSyncVersion: confirmation.SyncVersion + 1, ClientOccurredAt: fixture.now}
	completeCommand, err := sharedEvent.NewCommand(flighttask.CommandEmployeeCompleteTask, fixture.eligiblePersonnelPublicID, result.TaskPublicID, fixture.traceID, completePayload)
	if err != nil {
		t.Fatal(err)
	}
	if duplicateCommand, err := repository.ProcessCommand(context.Background(), completeCommand, coreapp.HandleCommand); err != nil || duplicateCommand {
		t.Fatalf("unexpected employee complete result duplicate=%v err=%v", duplicateCommand, err)
	}
	assertCount(t, db, "task_instance", "public_id = ? AND status = ? AND status_version = ? AND sync_version = ?", 1, result.TaskPublicID, "completed", 3, 3)
	assertCount(t, db, "task_assignment", "public_id = ? AND status = ? AND status_version = ?", 1, confirmation.AssignmentPublicID, "completed", 2)
	assertCount(t, db, "personnel", "public_id = ? AND work_state = ? AND status_version = ?", 1, fixture.eligiblePersonnelPublicID, "idle", 3)
	assertCount(t, db, "task_status_history", "task_id = (SELECT id FROM task_instance WHERE public_id = ?)", 4, result.TaskPublicID)
	assertCount(t, db, "task_assignment_status_history", "assignment_id = (SELECT id FROM task_assignment WHERE public_id = ?)", 3, confirmation.AssignmentPublicID)
	assertCount(t, db, "personnel_status_history", "assignment_id = (SELECT id FROM task_assignment WHERE public_id = ?)", 3, confirmation.AssignmentPublicID)
	assertCount(t, db, "outbox_event", "aggregate_id = ? AND event_type = ?", 1, result.TaskPublicID, flighttask.EventTaskCompleted)
}

func TestTaskCancellationSQLAgainstDocker(t *testing.T) {
	if os.Getenv("FLIGHT_RUN_BVS2_DB_TEST") != "1" {
		t.Skip("set FLIGHT_RUN_BVS2_DB_TEST=1 to run against the isolated Core MySQL")
	}

	db, err := platformmysql.Open(config.DatabaseConfig{
		Host:         envOrDefault("FLIGHT_BVS2_DB_HOST", "127.0.0.1"),
		Port:         envIntOrDefault(t, "FLIGHT_BVS2_DB_PORT", 3310),
		Username:     envOrDefault("FLIGHT_BVS2_DB_USERNAME", "core_app"),
		Password:     envOrDefault("FLIGHT_BVS2_DB_PASSWORD", "change-me"),
		Database:     envOrDefault("FLIGHT_BVS2_DB_DATABASE", "flight_core"),
		MaxIdleConns: 2, MaxOpenConns: 5, ConnMaxLifetime: 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := platformmysql.Ping(context.Background(), db); err != nil {
		_ = platformmysql.Close(db)
		t.Fatal(err)
	}

	fixture, err := seedFlightTaskFixture(db)
	if err != nil {
		_ = platformmysql.Close(db)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		fixture.cleanup(db)
		if err := platformmysql.Close(db); err != nil {
			t.Logf("close Core MySQL: %v", err)
		}
	})

	repository := coremysql.NewFlightTaskRepository(db)
	arrival, err := (flighttask.NewService(repository, fixedClock{value: fixture.now})).RecordFlightArrived(context.Background(), flighttask.ArrivalInput{
		FlightPublicID: fixture.flightPublicID, SourceEventID: fixture.sourceEventID,
		OccurredAt: fixture.actualArrivalAt, ActualArrivalAt: fixture.actualArrivalAt,
		Source: "docker-cancellation-test", ActorType: "machine", ActorPublicID: fixture.actorPublicID,
		RequestID: fixture.requestID, TraceID: fixture.traceID, SourceIP: "127.0.0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmation, err := flighttask.NewConfirmationService(repository, iam.NewAuthorizer(), fixedClock{value: fixture.now}).ConfirmTask(context.Background(), flighttask.ConfirmationInput{
		Principal:    security.Principal{Type: security.HumanPrincipal, PublicID: newID(), Roles: []string{security.RoleLeader}, Scopes: security.AccessScope{Global: true}},
		TaskPublicID: arrival.TaskPublicID, CandidatePublicID: arrival.Candidates[0].PublicID,
		ConfirmationID: "bvs206-confirm-" + newID(), ExpectedTaskVersion: 0,
		RequestID: newID(), TraceID: newID(), SourceIP: "127.0.0.1",
	})
	if err != nil {
		t.Fatal(err)
	}

	cancellationID := newID()
	cancellationService := flighttask.NewCancellationService(repository, iam.NewAuthorizer(), fixedClock{value: fixture.now})
	cancellationInput := flighttask.CancellationInput{
		Principal:    security.Principal{Type: security.HumanPrincipal, PublicID: newID(), Roles: []string{security.RoleManager}, Scopes: security.AccessScope{Global: true}},
		TaskPublicID: arrival.TaskPublicID, CancellationID: cancellationID, ExpectedTaskVersion: confirmation.TaskVersion,
		Reason: "aircraft handling plan changed", RequestID: newID(), TraceID: newID(), SourceIP: "127.0.0.1",
	}
	cancelled, err := cancellationService.CancelTask(context.Background(), cancellationInput)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.ResultCode != flighttask.ResultCancellationApplied || cancelled.TaskStatus != string(taskmodule.StatusCancelled) || cancelled.TaskVersion != confirmation.TaskVersion+1 || cancelled.SyncVersion != confirmation.SyncVersion+1 {
		t.Fatalf("unexpected SQL cancellation result: %#v", cancelled)
	}
	assertCount(t, db, "task_instance", "public_id = ? AND status = ? AND status_version = ? AND sync_version = ?", 1, arrival.TaskPublicID, "cancelled", 2, 2)
	assertCount(t, db, "task_candidate", "task_id = (SELECT id FROM task_instance WHERE public_id = ?) AND status = ?", 1, arrival.TaskPublicID, "selected")
	assertCount(t, db, "task_assignment", "public_id = ? AND status = ? AND status_version = ? AND cancellation_id = ?", 1, confirmation.AssignmentPublicID, "cancelled", 1, cancellationID)
	assertCount(t, db, "personnel", "public_id = ? AND work_state = ? AND status_version = ?", 1, fixture.eligiblePersonnelPublicID, "idle", 2)
	assertCount(t, db, "task_status_history", "task_id = (SELECT id FROM task_instance WHERE public_id = ?)", 3, arrival.TaskPublicID)
	assertCount(t, db, "task_assignment_status_history", "assignment_id = (SELECT id FROM task_assignment WHERE public_id = ?) AND to_status = ? AND cancellation_id = ?", 1, confirmation.AssignmentPublicID, "cancelled", cancellationID)
	assertCount(t, db, "personnel_status_history", "assignment_id = (SELECT id FROM task_assignment WHERE public_id = ?) AND to_state = ? AND command_id = ?", 1, confirmation.AssignmentPublicID, "idle", cancellationID)
	assertCount(t, db, "audit_log", "resource_id = ? AND action = ? AND result = ?", 1, arrival.TaskPublicID, "task.cancel", "cancelled")
	assertCount(t, db, "outbox_event", "aggregate_id = ? AND event_type = ? AND correlation_id = ?", 1, arrival.TaskPublicID, flighttask.EventTaskCancelled, cancellationID)
	assertCount(t, db, "business_idempotency_record", "operation_type = ? AND idempotency_key = ?", 1, flighttask.OperationTaskCancel, cancellationID)

	replayed, err := cancellationService.CancelTask(context.Background(), cancellationInput)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Duplicate || replayed.AssignmentPublicID != cancelled.AssignmentPublicID {
		t.Fatalf("unexpected SQL cancellation replay: %#v", replayed)
	}
	assertCount(t, db, "task_assignment_status_history", "assignment_id = (SELECT id FROM task_assignment WHERE public_id = ?) AND to_status = ?", 1, confirmation.AssignmentPublicID, "cancelled")
	assertCount(t, db, "outbox_event", "aggregate_id = ? AND event_type = ?", 1, arrival.TaskPublicID, flighttask.EventTaskCancelled)
}

type fixedClock struct{ value time.Time }

func (c fixedClock) Now() time.Time { return c.value }

type flightTaskFixture struct {
	now                       time.Time
	actualArrivalAt           time.Time
	areaPublicID              string
	teamPublicID              string
	teamMemberPublicIDs       []string
	personnelPublicIDs        []string
	eligiblePersonnelPublicID string
	flightPublicID            string
	templatePublicID          string
	sourceEventID             string
	actorPublicID             string
	requestID                 string
	traceID                   string
}

func seedFlightTaskFixture(db *gorm.DB) (flightTaskFixture, error) {
	now := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	fixture := flightTaskFixture{
		now:              now,
		actualArrivalAt:  now.Add(10 * time.Minute),
		areaPublicID:     newID(),
		teamPublicID:     newID(),
		flightPublicID:   newID(),
		templatePublicID: newID(),
		sourceEventID:    "bvs203-arrival-" + newID(),
		actorPublicID:    newID(),
		requestID:        newID(),
		traceID:          newID(),
	}
	fixture.eligiblePersonnelPublicID = newID()
	fixture.personnelPublicIDs = []string{fixture.eligiblePersonnelPublicID, newID(), newID()}
	fixture.teamMemberPublicIDs = []string{newID(), newID(), newID()}
	userPublicIDs := []string{newID(), newID(), newID()}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO operation_area (public_id, code, name, enabled) VALUES (?, ?, ?, 1)`, fixture.areaPublicID, "bvs203-area-"+fixture.areaPublicID, "BVS2-03 Test Area").Error; err != nil {
			return fmt.Errorf("insert operation area: %w", err)
		}
		areaID, err := lookupID(tx, "operation_area", fixture.areaPublicID)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO team (public_id, area_id, code, name, enabled) VALUES (?, ?, ?, ?, 1)`, fixture.teamPublicID, areaID, "bvs203-team-"+fixture.teamPublicID, "BVS2-03 Test Team").Error; err != nil {
			return fmt.Errorf("insert team: %w", err)
		}
		teamID, err := lookupID(tx, "team", fixture.teamPublicID)
		if err != nil {
			return err
		}

		personnelValues := []struct {
			capabilities string
			workState    string
		}{
			{capabilities: `["ramp","radio"]`, workState: "idle"},
			{capabilities: `["radio"]`, workState: "idle"},
			{capabilities: `["ramp"]`, workState: "busy"},
		}
		for i, value := range personnelValues {
			if err := tx.Exec(`INSERT INTO personnel (public_id, user_public_id, employee_no, display_name, team_id, position_code, capabilities, work_state, last_state_changed_at, enabled) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`, fixture.personnelPublicIDs[i], userPublicIDs[i], "bvs203-employee-"+fixture.personnelPublicIDs[i], "BVS2-03 Test Person", teamID, "ramp", value.capabilities, value.workState, now.Add(time.Duration(-10-i)*time.Minute)).Error; err != nil {
				return fmt.Errorf("insert personnel %d: %w", i, err)
			}
			personnelID, err := lookupID(tx, "personnel", fixture.personnelPublicIDs[i])
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO team_member (public_id, team_id, personnel_id, is_primary, joined_at) VALUES (?, ?, ?, 1, ?)`, fixture.teamMemberPublicIDs[i], teamID, personnelID, now.Add(-24*time.Hour)).Error; err != nil {
				return fmt.Errorf("insert team member %d: %w", i, err)
			}
		}

		if err := tx.Exec(`INSERT INTO flight (public_id, flight_display_no, operating_date, scheduled_at, status, status_version, last_status_changed_at) VALUES (?, ?, ?, ?, 'scheduled', 0, ?)`, fixture.flightPublicID, "BVS203-"+fixture.flightPublicID, now.Format("2006-01-02"), now.Add(30*time.Minute), now).Error; err != nil {
			return fmt.Errorf("insert flight: %w", err)
		}
		if err := tx.Exec(`INSERT INTO task_template (public_id, name, trigger_type, template_version, enabled, target_area_id, target_team_id, required_position_code, required_capabilities, planned_offset_seconds, default_message) VALUES (?, ?, 'flight_arrived', 1, 1, ?, ?, 'ramp', ?, 900, ?)`, fixture.templatePublicID, "BVS2-03 Arrival Task", areaID, teamID, `["ramp"]`, "BVS2-03 integration task").Error; err != nil {
			return fmt.Errorf("insert task template: %w", err)
		}
		return nil
	})
	if err != nil {
		return flightTaskFixture{}, err
	}
	return fixture, nil
}

func (fixture flightTaskFixture) cleanup(db *gorm.DB) {
	var taskID, flightID uint64
	db.Table("flight").Select("id").Where("public_id = ?", fixture.flightPublicID).Scan(&flightID)
	var taskPublicID string
	if flightID != 0 {
		db.Table("task_instance").Select("id").Where("flight_id = ?", flightID).Scan(&taskID)
		db.Table("task_instance").Select("public_id").Where("id = ?", taskID).Scan(&taskPublicID)
	}
	db.Exec("DELETE FROM business_idempotency_record WHERE operation_type = ? AND idempotency_key = ?", flighttask.OperationFlightArrived, fixture.sourceEventID)
	if taskID != 0 {
		db.Exec("DELETE FROM business_idempotency_record WHERE operation_type = ? AND aggregate_public_id = ?", flighttask.OperationTaskConfirm, taskPublicID)
		db.Exec("DELETE FROM business_idempotency_record WHERE operation_type = ? AND aggregate_public_id = ?", flighttask.OperationTaskCancel, taskPublicID)
		db.Exec("DELETE FROM personnel_status_history WHERE assignment_id IN (SELECT id FROM task_assignment WHERE task_id = ?)", taskID)
		db.Exec("DELETE FROM task_assignment_status_history WHERE assignment_id IN (SELECT id FROM task_assignment WHERE task_id = ?)", taskID)
		db.Exec("DELETE FROM task_assignment WHERE task_id = ?", taskID)
	}
	db.Exec("DELETE FROM audit_log WHERE resource_id IN (?, ?)", fixture.flightPublicID, taskPublicID)
	if taskPublicID != "" {
		db.Exec("DELETE FROM outbox_event WHERE aggregate_id = ?", taskPublicID)
	}
	if taskID != 0 {
		db.Exec("DELETE FROM task_status_history WHERE task_id = ?", taskID)
		db.Exec("DELETE FROM task_candidate WHERE task_id = ?", taskID)
		db.Exec("DELETE FROM task_instance WHERE id = ?", taskID)
	}
	if flightID != 0 {
		db.Exec("DELETE FROM flight_status_history WHERE flight_id = ?", flightID)
		db.Exec("DELETE FROM flight WHERE id = ?", flightID)
	}
	for _, publicID := range fixture.teamMemberPublicIDs {
		db.Exec("DELETE FROM team_member WHERE public_id = ?", publicID)
	}
	for _, publicID := range fixture.personnelPublicIDs {
		db.Exec("DELETE FROM personnel WHERE public_id = ?", publicID)
	}
	db.Exec("DELETE FROM task_template WHERE public_id = ?", fixture.templatePublicID)
	db.Exec("DELETE FROM team WHERE public_id = ?", fixture.teamPublicID)
	db.Exec("DELETE FROM operation_area WHERE public_id = ?", fixture.areaPublicID)
}

func lookupID(db *gorm.DB, table, publicID string) (uint64, error) {
	var value uint64
	if err := db.Table(table).Select("id").Where("public_id = ?", publicID).Scan(&value).Error; err != nil {
		return 0, fmt.Errorf("lookup %s %s: %w", table, publicID, err)
	}
	if value == 0 {
		return 0, fmt.Errorf("lookup %s %s returned no id", table, publicID)
	}
	return value, nil
}

func assertCount(t *testing.T, db *gorm.DB, table, condition string, want int64, args ...any) {
	t.Helper()
	var count int64
	if err := db.Table(table).Where(condition, args...).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("%s count for %q = %d, want %d", table, condition, count, want)
	}
}

func newID() string {
	value, err := id.NewPublicID()
	if err != nil {
		panic(err)
	}
	return value
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envIntOrDefault(t *testing.T, key string, fallback int) int {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 || parsed > 65535 {
		t.Fatalf("%s must be a valid TCP port", key)
	}
	return parsed
}
