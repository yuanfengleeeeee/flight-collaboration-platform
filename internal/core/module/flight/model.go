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

type Record struct {
	ID                  uint64
	PublicID            string
	DisplayNo           string
	OperatingDate       time.Time
	ScheduledAt         time.Time
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
