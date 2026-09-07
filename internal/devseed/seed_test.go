package devseed

import (
	"testing"
	"time"
)

func TestDeterministicIDIsStableUUID(t *testing.T) {
	first := deterministicID("DEMO", 20260903, "task", 1)
	second := deterministicID("DEMO", 20260903, "task", 1)
	if first != second || len(first) != 36 || first[14] != '5' {
		t.Fatalf("unexpected deterministic ID: %q %q", first, second)
	}
}

func TestOptionsRequireEnoughPersonnelForTaskCandidates(t *testing.T) {
	_, err := (Options{PersonnelCount: 3, Password: "Flight123!", Now: time.Now()}).normalized()
	if err == nil {
		t.Fatal("expected minimum personnel validation error")
	}
}

func TestSeededTaskStatusesCoverEmployeeLifecycle(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		seen[seededTaskStatus(i)] = true
	}
        for _, status := range []string{"pending_dispatch", "assigned", "in_progress", "completed", "cancelled"} {
		if !seen[status] {
			t.Fatalf("seed status %q is not covered", status)
		}
	}
}

func TestSeedPersonnelSelectionKeepsActiveAssignmentsDistinct(t *testing.T) {
	people := []PersonnelSeed{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}
	states := map[uint64]string{1: "idle", 2: "idle", 3: "idle", 4: "idle"}

	first, err := selectSeedPersonnel(people, "assigned", 0, states)
	if err != nil {
		t.Fatalf("select first active personnel: %v", err)
	}
	states[first.ID] = mergePersonnelSeedState(states[first.ID], "reserved")
	second, err := selectSeedPersonnel(people, "in_progress", 0, states)
	if err != nil {
		t.Fatalf("select second active personnel: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("active seeded assignments reused personnel %d", first.ID)
	}
}

func TestMergePersonnelSeedStateDoesNotDowngradeActiveState(t *testing.T) {
	if got := mergePersonnelSeedState("busy", "idle"); got != "busy" {
		t.Fatalf("busy state was downgraded to %q", got)
	}
	if got := mergePersonnelSeedState("reserved", "busy"); got != "busy" {
		t.Fatalf("reserved state was not upgraded to busy: %q", got)
	}
}

func TestSeedOutboxEventTypeMatchesProjectionLifecycle(t *testing.T) {
	want := map[string]string{
		"assigned":    "task.assigned.v1",
		"in_progress": "task.started.v1",
		"completed":   "task.completed.v1",
		"cancelled":   "task.cancelled.v1",
	}
	for status, expected := range want {
		got, err := seedOutboxEventType(status)
		if err != nil || got != expected {
			t.Fatalf("status %q mapped to %q with err %v, want %q", status, got, err, expected)
		}
	}
        if _, err := seedOutboxEventType("pending_dispatch"); err == nil {
                t.Fatal("expected pending_dispatch to have no outbound assignment event")
	}
}
