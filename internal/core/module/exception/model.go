// Package exception owns the Core fact recorded when an employee reports a
// task-site exception. It deliberately contains no transport or persistence
// dependencies.
package exception

import "time"

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type Status string

const (
	StatusOpen         Status = "open"
	StatusAcknowledged Status = "acknowledged"
	StatusResolved     Status = "resolved"
	StatusRejected     Status = "rejected"
)

type Record struct {
	ID                 uint64
	PublicID           string
	TaskID             uint64
	TaskPublicID       string
	AssignmentID       uint64
	AssignmentPublicID string
	PersonnelID        uint64
	PersonnelPublicID  string
	Category           string
	Severity           Severity
	Description        string
	Status             Status
	ReportedByPublicID string
	ReportedAt         time.Time
	ResolvedByPublicID string
	ResolvedAt         *time.Time
	ResolutionNote     string
}
