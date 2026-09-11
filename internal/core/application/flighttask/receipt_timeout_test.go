package flighttask

import (
	"context"
	"testing"
	"time"
)

func TestReceiptTimeoutReassignerNormalizesPolicy(t *testing.T) {
	repository := &receiptTimeoutFakeRepository{}
	reassigner := NewReceiptTimeoutReassigner(repository)
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	result, err := reassigner.Run(context.Background(), now, 0, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if repository.now != now || repository.timeout != DefaultAssignmentReceiptTimeout || repository.limit != 50 || repository.workerID != "worker" || result.Reassigned != 1 {
		t.Fatalf("normalized receipt timeout call = %#v result=%#v", repository, result)
	}
}

type receiptTimeoutFakeRepository struct {
	now      time.Time
	timeout  time.Duration
	limit    int
	workerID string
}

func (r *receiptTimeoutFakeRepository) ReassignUnreceived(_ context.Context, now time.Time, timeout time.Duration, limit int, workerID string) (ReceiptReassignmentResult, error) {
	r.now, r.timeout, r.limit, r.workerID = now, timeout, limit, workerID
	return ReceiptReassignmentResult{Scanned: 1, Reassigned: 1}, nil
}
