package event

import (
	"testing"
	"time"
)

func TestEquivalentJSONIgnoresObjectKeyOrdering(t *testing.T) {
	if !EquivalentJSON([]byte(`{"assignment_public_id":"a","expected_sync_version":1}`), []byte(`{"expected_sync_version":1,"assignment_public_id":"a"}`)) {
		t.Fatal("equivalent JSON objects should compare equal")
	}
}

func TestEquivalentJSONRejectsDifferentValues(t *testing.T) {
	if EquivalentJSON([]byte(`{"status":"accepted"}`), []byte(`{"status":"completed"}`)) {
		t.Fatal("different JSON values should not compare equal")
	}
}

func TestEquivalentCommandIgnoresDeliveryMetadata(t *testing.T) {
	first, err := NewCommandWithID("command-1", "employee_accept_task.v1", "staff-1", "task-1", "trace-1", time.Date(2026, 9, 1, 1, 2, 3, 4, time.UTC), map[string]any{"assignment_public_id": "assignment-1", "expected_sync_version": 1})
	if err != nil {
		t.Fatal(err)
	}
	retry := first
	retry.TraceID = "trace-2"
	retry.OccurredAt = first.OccurredAt.Add(time.Minute)
	if !EquivalentCommand(first, retry) {
		t.Fatal("delivery metadata should not change command identity")
	}
	retry.Payload = []byte(`{"assignment_public_id":"assignment-2","expected_sync_version":1}`)
	if EquivalentCommand(first, retry) {
		t.Fatal("different command payload should conflict")
	}
}
