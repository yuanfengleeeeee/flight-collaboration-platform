// Package task owns Core task facts, templates and candidate records.
package task

import "time"

type TriggerType string

const TriggerFlightArrived TriggerType = "flight_arrived"

type Status string

const (
	// StatusPendingDispatch means the flight event has generated a task and
	// candidates, but the automatic selector has not yet committed an
	// assignment. It is intentionally distinct from human confirmation.
	StatusPendingDispatch      Status = "pending_dispatch"
	StatusAwaitingConfirmation Status = "awaiting_confirmation"
	StatusAssigned             Status = "assigned"
	StatusInProgress           Status = "in_progress"
	// StatusPaused means a manager-approved operational change has temporarily
	// removed the task from execution. It is not an employee refusal state.
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
	StatusCancelled Status = "cancelled"
)

type ChangeAction string

const (
	ChangeActionPause      ChangeAction = "pause"
	ChangeActionReassign   ChangeAction = "reassign"
	ChangeActionReschedule ChangeAction = "reschedule"
	ChangeActionCancel     ChangeAction = "cancel"
	ChangeActionResume     ChangeAction = "resume"
)

type ChangeRequestStatus string

const (
	ChangeRequestPending  ChangeRequestStatus = "pending"
	ChangeRequestApproved ChangeRequestStatus = "approved"
	ChangeRequestRejected ChangeRequestStatus = "rejected"
	ChangeRequestApplied  ChangeRequestStatus = "applied"
	ChangeRequestFailed   ChangeRequestStatus = "failed"
)

type CandidateStatus string

const (
	CandidateProposed    CandidateStatus = "proposed"
	CandidateSelected    CandidateStatus = "selected"
	CandidateRejected    CandidateStatus = "rejected"
	CandidateInvalidated CandidateStatus = "invalidated"
)

type Template struct {
	ID                   uint64
	PublicID             string
	Name                 string
	TriggerType          TriggerType
	Version              uint
	Enabled              bool
	AreaID               uint64
	TeamID               uint64
	RequiredPositionCode string
	RequiredCapabilities []string
	PlannedOffsetSeconds int
	DefaultMessage       string
}

type Instance struct {
	ID                   uint64
	PublicID             string
	FlightID             uint64
	FlightPublicID       string
	FlightDisplayNo      string
	TemplateID           uint64
	TemplatePublicID     string
	AreaID               uint64
	AreaName             string
	TeamID               uint64
	TriggerType          TriggerType
	GenerationKey        string
	SourceEventID        string
	TemplateVersion      uint
	RequiredPositionCode string
	RequiredCapabilities []string
	Name                 string
	Message              string
	PlannedAt            time.Time
	Status               Status
	StatusVersion        uint64
	SyncVersion          uint64
}

type ChangeRequest struct {
	ID                      uint64
	PublicID                string
	TaskID                  uint64
	TaskPublicID            string
	ExceptionID             uint64
	ExceptionPublicID       string
	Action                  ChangeAction
	Reason                  string
	TargetCandidatePublicID string
	TargetPlannedAt         *time.Time
	Status                  ChangeRequestStatus
	RequestedByPublicID     string
	RequestedAt             time.Time
	ReviewedByPublicID      string
	ReviewedAt              *time.Time
	ReviewNote              string
	AppliedAt               *time.Time
	FailureReason           string
	RequestID               string
	TraceID                 string
}

type StatusHistory struct {
	PublicID      string
	TaskID        uint64
	StatusVersion uint64
	FromStatus    *Status
	ToStatus      Status
	Reason        string
	ActorType     string
	ActorPublicID string
	CommandID     string
	SourceEventID string
	OccurredAt    time.Time
}

type Candidate struct {
	ID                         uint64
	PublicID                   string
	TaskID                     uint64
	PersonnelID                uint64
	PersonnelPublicID          string
	Rank                       int
	Status                     CandidateStatus
	MatchedPositionCode        string
	MatchedCapabilities        []string
	PersonnelWorkStateSnapshot string
	PersonnelStateChangedAt    time.Time
	RejectionReason            string
	SelectedAt                 *time.Time
	InvalidatedAt              *time.Time
}

type AssignmentStatus string

const (
	AssignmentConfirmed AssignmentStatus = "confirmed"
	AssignmentAccepted  AssignmentStatus = "accepted"
	AssignmentCompleted AssignmentStatus = "completed"
	AssignmentCancelled AssignmentStatus = "cancelled"
)

type AssignmentReceiptStatus string

const (
	AssignmentReceiptPending  AssignmentReceiptStatus = "pending"
	AssignmentReceiptReceived AssignmentReceiptStatus = "received"
)

type Assignment struct {
	ID                  uint64
	PublicID            string
	TaskID              uint64
	CandidateID         uint64
	PersonnelID         uint64
	PersonnelPublicID   string
	Status              AssignmentStatus
	StatusVersion       uint64
	ReceiptStatus       AssignmentReceiptStatus
	ReceivedAt          *time.Time
	ConfirmationID      string
	ConfirmedByPublicID string
	ConfirmedAt         time.Time
}

type AssignmentStatusHistory struct {
	PublicID       string
	AssignmentID   uint64
	StatusVersion  uint64
	FromStatus     *AssignmentStatus
	ToStatus       AssignmentStatus
	Reason         string
	ActorType      string
	ActorPublicID  string
	CommandID      string
	ConfirmationID string
	CancellationID string
	OccurredAt     time.Time
}
