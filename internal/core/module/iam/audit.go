package iam

import (
	"context"
	"fmt"

	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
)

type AuditWriter interface {
	WriteAudit(context.Context, coresync.AuditRecord) error
}

// WriteAudit validates and writes a business audit record through the active
// Core transaction. Audit is not a log-only side effect.
func WriteAudit(ctx context.Context, tx coresync.CoreTransaction, entry coresync.AuditRecord) error {
	if tx == nil {
		return fmt.Errorf("audit transaction is nil")
	}
	if entry.ActorType == "" || entry.ActorID == "" || entry.Action == "" || entry.ResourceType == "" || entry.ResourceID == "" || entry.Result == "" || entry.TraceID == "" {
		return fmt.Errorf("audit actor, action, resource, result and trace_id are required")
	}
	return tx.AppendAudit(ctx, entry)
}
