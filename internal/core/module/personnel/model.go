// Package personnel owns personnel facts and work-state vocabulary.
package personnel

import "time"

type WorkState string

const (
	WorkStateIdle        WorkState = "idle"
	WorkStateReserved    WorkState = "reserved"
	WorkStateBusy        WorkState = "busy"
	WorkStateUnavailable WorkState = "unavailable"
)

// CandidateRecord is a read model returned by the Personnel port. The Task
// use case may inspect it, but only a Personnel application may later change
// the work state.
type CandidateRecord struct {
	ID                 uint64
	PublicID           string
	UserPublicID       string
	TeamID             uint64
	AreaID             uint64
	PositionCode       string
	Capabilities       []string
	WorkState          WorkState
	StatusVersion      uint64
	LastStateChangedAt time.Time
	Enabled            bool
}

// StatusHistory records a personnel work-state transition together with the
// business fact that caused it. The Personnel boundary remains the owner of
// this state; other use cases only request the transition through a port.
type StatusHistory struct {
	PublicID      string
	PersonnelID   uint64
	StatusVersion uint64
	FromState     *WorkState
	ToState       WorkState
	Reason        string
	ActorType     string
	ActorPublicID string
	AssignmentID  uint64
	CommandID     string
	OccurredAt    time.Time
}
