// Package flight owns the Core flight facts used by the business slices.
package flight

import "time"

type Status string

const (
	StatusScheduled Status = "scheduled"
	StatusArrived   Status = "arrived"
	StatusDeparted  Status = "departed"
	StatusCancelled Status = "cancelled"
)

type SourceState string

const (
	SourceStateFresh    SourceState = "fresh"
	SourceStateStale    SourceState = "stale"
	SourceStateFallback SourceState = "fallback"
	SourceStateFailed   SourceState = "failed"
)

type Record struct {
	ID                  uint64
	PublicID            string
	DisplayNo           string
	SourceProvider      string
	ExternalFlightID    string
	OperatingDate       time.Time
	ScheduledAt         time.Time
	SourceLastSyncedAt  *time.Time
	SourceState         SourceState
	SourceLastAttemptAt *time.Time
	SourceLastError     string
	ActualArrivalAt     *time.Time
	Status              Status
	StatusVersion       uint64
	LastStatusChangedAt time.Time
}

type StatusHistory struct {
	PublicID       string
	FlightID       uint64
	FromStatus     *Status
	ToStatus       Status
	TransitionType string
	SourceEventID  string
	ActorType      string
	ActorPublicID  string
	Reason         string
	OccurredAt     time.Time
}
