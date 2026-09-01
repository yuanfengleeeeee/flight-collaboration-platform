// Package event defines versioned messages exchanged across the Core/Edge
// boundary. The package intentionally has no transport or persistence code.
package event

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
)

const (
	StatusPending    = "pending"
	StatusRetry      = "retry"
	StatusFailed     = "failed"
	StatusProcessing = "processing"
	StatusSent       = "sent"
	StatusApplied    = "applied"
	StatusRejected   = "rejected"
)

// EventEnvelope is the only shape allowed for Core-to-Edge messages.
type EventEnvelope struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	SchemaVersion int             `json:"schema_version"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Producer      string          `json:"producer"`
	CorrelationID string          `json:"correlation_id"`
	TraceID       string          `json:"trace_id"`
	Payload       json.RawMessage `json:"payload"`
}

// CommandEnvelope is the only shape allowed for Edge-to-Core commands.
type CommandEnvelope struct {
	CommandID     string          `json:"command_id"`
	CommandType   string          `json:"command_type"`
	SchemaVersion int             `json:"schema_version"`
	ActorPublicID string          `json:"actor_public_id"`
	AggregateID   string          `json:"aggregate_id"`
	OccurredAt    time.Time       `json:"occurred_at"`
	TraceID       string          `json:"trace_id"`
	Payload       json.RawMessage `json:"payload"`
}

func NewEvent(eventType, aggregateType, aggregateID, producer string, payload any) (EventEnvelope, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return EventEnvelope{}, fmt.Errorf("marshal event payload: %w", err)
	}
	eventID, err := id.NewPublicID()
	if err != nil {
		return EventEnvelope{}, err
	}
	correlationID, err := id.NewPublicID()
	if err != nil {
		return EventEnvelope{}, err
	}
	traceID, err := id.NewPublicID()
	if err != nil {
		return EventEnvelope{}, err
	}
	return EventEnvelope{
		EventID: eventID, EventType: eventType, SchemaVersion: 1,
		AggregateType: aggregateType, AggregateID: aggregateID,
		OccurredAt: time.Now().UTC(), Producer: producer,
		CorrelationID: correlationID, TraceID: traceID, Payload: encoded,
	}, nil
}

func NewCommand(commandType, actorPublicID, aggregateID, traceID string, payload any) (CommandEnvelope, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return CommandEnvelope{}, fmt.Errorf("marshal command payload: %w", err)
	}
	commandID, err := id.NewPublicID()
	if err != nil {
		return CommandEnvelope{}, err
	}
	if strings.TrimSpace(traceID) == "" {
		traceID, err = id.NewPublicID()
		if err != nil {
			return CommandEnvelope{}, err
		}
	}
	return CommandEnvelope{
		CommandID: commandID, CommandType: commandType, SchemaVersion: 1,
		ActorPublicID: actorPublicID, AggregateID: aggregateID,
		OccurredAt: time.Now().UTC(), TraceID: traceID, Payload: encoded,
	}, nil
}

// EquivalentJSON treats object key ordering and insignificant whitespace as
// persistence details. MySQL JSON columns canonicalize object keys on write,
// so raw byte comparison would reject a legitimate command replay.
func EquivalentJSON(left, right []byte) bool {
	if bytes.Equal(bytes.TrimSpace(left), bytes.TrimSpace(right)) {
		return true
	}
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

// EqualPersistedTime compares timestamps at MySQL DATETIME(6) precision.
// MySQL rounds fractional seconds when storing DATETIME values, so replay
// checks must compare the same persisted representation on both sides.
func EqualPersistedTime(left, right time.Time) bool {
	return left.UTC().Round(time.Microsecond).Equal(right.UTC().Round(time.Microsecond))
}

func (e EventEnvelope) Validate() error {
	if strings.TrimSpace(e.EventID) == "" || strings.TrimSpace(e.EventType) == "" || !strings.Contains(e.EventType, ".v") {
		return errors.New("event_id and versioned event_type are required")
	}
	if e.SchemaVersion <= 0 || strings.TrimSpace(e.AggregateType) == "" || strings.TrimSpace(e.AggregateID) == "" || e.OccurredAt.IsZero() || strings.TrimSpace(e.Producer) == "" || strings.TrimSpace(e.TraceID) == "" {
		return errors.New("event envelope metadata is incomplete")
	}
	if len(bytes.TrimSpace(e.Payload)) == 0 || !json.Valid(e.Payload) {
		return errors.New("event payload must be valid JSON")
	}
	return nil
}

func (c CommandEnvelope) Validate() error {
	if strings.TrimSpace(c.CommandID) == "" || strings.TrimSpace(c.CommandType) == "" || !strings.Contains(c.CommandType, ".v") {
		return errors.New("command_id and versioned command_type are required")
	}
	if c.SchemaVersion <= 0 || strings.TrimSpace(c.ActorPublicID) == "" || strings.TrimSpace(c.AggregateID) == "" || c.OccurredAt.IsZero() || strings.TrimSpace(c.TraceID) == "" {
		return errors.New("command envelope metadata is incomplete")
	}
	if len(bytes.TrimSpace(c.Payload)) == 0 || !json.Valid(c.Payload) {
		return errors.New("command payload must be valid JSON")
	}
	return nil
}
