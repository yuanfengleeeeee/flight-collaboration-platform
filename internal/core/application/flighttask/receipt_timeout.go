package flighttask

import (
	"context"
	"fmt"
	"time"
)

const DefaultAssignmentReceiptTimeout = 5 * time.Minute

type ReceiptReassignmentResult struct {
	Scanned     int `json:"scanned"`
	Reassigned  int `json:"reassigned"`
	NoCandidate int `json:"no_candidate"`
}

// ReceiptTimeoutRepository owns the locking transaction. The application
// service only defines the retry policy and keeps the worker independent of
// SQL, so a future scheduler can replace the selection query safely.
type ReceiptTimeoutRepository interface {
	ReassignUnreceived(context.Context, time.Time, time.Duration, int, string) (ReceiptReassignmentResult, error)
}

type ReceiptTimeoutReassigner struct {
	repository ReceiptTimeoutRepository
}

func NewReceiptTimeoutReassigner(repository ReceiptTimeoutRepository) *ReceiptTimeoutReassigner {
	return &ReceiptTimeoutReassigner{repository: repository}
}

func (r *ReceiptTimeoutReassigner) Run(ctx context.Context, now time.Time, timeout time.Duration, limit int, workerID string) (ReceiptReassignmentResult, error) {
	if r == nil || r.repository == nil {
		return ReceiptReassignmentResult{}, fmt.Errorf("receipt timeout reassigner is not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if timeout <= 0 {
		timeout = DefaultAssignmentReceiptTimeout
	}
	if limit <= 0 {
		limit = 50
	}
	if workerID == "" {
		workerID = "worker"
	}
	return r.repository.ReassignUnreceived(ctx, now.UTC(), timeout, limit, workerID)
}
