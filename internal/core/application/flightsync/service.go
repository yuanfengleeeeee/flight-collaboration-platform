// Package flightsync is the Core application boundary for upstream flight
// sources. It accepts normalized records quickly, then applies them to the
// flight fact model from a durable inbox in a separate worker step.
package flightsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	flighttask "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/flighttask"
	flightintegration "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/integration/flight"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/clock"
)

const (
	RecordTypeSchedule = "schedule"
	RecordTypeEvent    = "event"

	maxBatchRecords        = 500
	maxApplyAttempts       = 12
	defaultFreshnessWindow = 10 * time.Minute
)

var (
	ErrRepositoryNotConfigured = errors.New("flight source repository is not configured")
	ErrInvalidInput            = errors.New("invalid flight source input")
	ErrUnsupportedEvent        = errors.New("unsupported flight source event")
)

var providerPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,63}$`)

type IngestInput struct {
	Provider  string
	Schedules []flightintegration.Schedule
	Events    []flightintegration.Event
	Now       time.Time
}

type StageResult struct {
	Accepted  int `json:"accepted"`
	Updated   int `json:"updated"`
	Duplicate int `json:"duplicate"`
}

type PendingRecord struct {
	ID                uint64
	PublicID          string
	Provider          string
	RecordType        string
	ExternalRecordID  string
	ExternalFlightID  string
	FlightDisplayNo   string
	OperatingDate     *time.Time
	ScheduledAt       *time.Time
	EventStatus       string
	OccurredAt        *time.Time
	ActualArrivalAt   *time.Time
	ActualDepartureAt *time.Time
	Reason            string
	Payload           []byte
	Attempts          int
}

type ApplyResult struct {
	Claimed int `json:"claimed"`
	Applied int `json:"applied"`
	Retried int `json:"retried"`
	Failed  int `json:"failed"`
}

type SourceState string

const (
	SourceFresh    SourceState = "fresh"
	SourceStale    SourceState = "stale"
	SourceFallback SourceState = "fallback"
	SourceFailed   SourceState = "failed"
)

type SourceHealth struct {
	Provider                   string      `json:"provider"`
	State                      SourceState `json:"state"`
	LastAttemptAt              *time.Time  `json:"last_attempt_at,omitempty"`
	LastSuccessAt              *time.Time  `json:"last_success_at,omitempty"`
	LastFailureAt              *time.Time  `json:"last_failure_at,omitempty"`
	LastError                  string      `json:"last_error,omitempty"`
	FallbackEnabled            bool        `json:"fallback_enabled"`
	LastReconciliationAt       *time.Time  `json:"last_reconciliation_at,omitempty"`
	LastReconciliationStatus   string      `json:"last_reconciliation_status,omitempty"`
	LastReconciliationMismatch int         `json:"last_reconciliation_mismatch_count,omitempty"`
}

type SyncWindowResult struct {
	StageResult
	Schedules []flightintegration.Schedule `json:"-"`
	Events    []flightintegration.Event    `json:"-"`
}

type ReconciliationResult struct {
	PublicID             string    `json:"public_id"`
	Provider             string    `json:"provider"`
	WindowFrom           time.Time `json:"window_from"`
	WindowTo             time.Time `json:"window_to"`
	UpstreamCount        int       `json:"upstream_count"`
	CoreCount            int       `json:"core_count"`
	PendingApplyCount    int       `json:"pending_apply_count"`
	UpstreamMissingCount int       `json:"upstream_missing_count"`
	CoreMissingCount     int       `json:"core_missing_count"`
	UpstreamMissingIDs   []string  `json:"upstream_missing_ids,omitempty"`
	CoreMissingIDs       []string  `json:"core_missing_ids,omitempty"`
	PendingApplyIDs      []string  `json:"pending_apply_ids,omitempty"`
	Status               string    `json:"status"`
	CheckedAt            time.Time `json:"checked_at"`
}

type ReconciliationRepository interface {
	ReconcileSchedules(context.Context, string, time.Time, time.Time, []flightintegration.Schedule, time.Time) (ReconciliationResult, error)
}

type ReconciliationHealthRepository interface {
	LatestReconciliation(context.Context, string) (ReconciliationResult, error)
}

type Repository interface {
	Stage(context.Context, IngestInput) (StageResult, error)
	ClaimPending(context.Context, int, time.Time, string, time.Duration) ([]PendingRecord, error)
	MarkApplied(context.Context, uint64, string, time.Time) error
	MarkRetry(context.Context, uint64, string, time.Time, string) error
	MarkFailed(context.Context, uint64, string, string) error
	ApplySchedule(context.Context, PendingRecord, flightintegration.Schedule, time.Time) error
	FindFlightPublicID(context.Context, string, string) (string, error)
	ApplyEvent(context.Context, PendingRecord, flightintegration.Event, time.Time) error
}

type HealthRepository interface {
	RecordSourceAttempt(context.Context, string, time.Time) error
	RecordSourceSuccess(context.Context, string, time.Time) error
	RecordSourceFailure(context.Context, string, time.Time, string) error
	GetSourceHealth(context.Context, string) (SourceHealth, error)
}

type ArrivalRecorder interface {
	RecordFlightArrived(context.Context, flighttask.ArrivalInput) (flighttask.ArrivalResult, error)
}

type Service struct {
	repository Repository
	arrival    ArrivalRecorder
	clock      clock.Clock
}

func NewService(repository Repository, arrival ArrivalRecorder, now clock.Clock) *Service {
	if now == nil {
		now = clock.Real{}
	}
	return &Service{repository: repository, arrival: arrival, clock: now}
}

func (s *Service) Ingest(ctx context.Context, input IngestInput) (StageResult, error) {
	if s == nil || s.repository == nil {
		return StageResult{}, ErrRepositoryNotConfigured
	}
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	if !providerPattern.MatchString(input.Provider) || input.Provider == "manual" {
		return StageResult{}, ErrInvalidInput
	}
	if len(input.Schedules)+len(input.Events) == 0 || len(input.Schedules)+len(input.Events) > maxBatchRecords {
		return StageResult{}, ErrInvalidInput
	}
	now := input.Now
	if now.IsZero() {
		now = s.clock.Now()
	}
	now = now.UTC()
	for i := range input.Schedules {
		input.Schedules[i].Provider = input.Provider
		if err := validateSchedule(input.Schedules[i]); err != nil {
			return StageResult{}, err
		}
	}
	for i := range input.Events {
		input.Events[i].Provider = input.Provider
		if err := validateEvent(input.Events[i]); err != nil {
			return StageResult{}, err
		}
	}
	input.Now = now
	result, err := s.repository.Stage(ctx, input)
	if health, ok := s.repository.(HealthRepository); ok {
		if err != nil {
			_ = health.RecordSourceFailure(ctx, input.Provider, now, trimError(err))
		} else {
			_ = health.RecordSourceSuccess(ctx, input.Provider, now)
		}
	}
	return result, err
}

func (s *Service) SyncWindow(ctx context.Context, provider flightintegration.Provider, from, to time.Time) (StageResult, error) {
	result, err := s.SyncWindowWithRecords(ctx, provider, from, to)
	return result.StageResult, err
}

func (s *Service) SyncWindowWithRecords(ctx context.Context, provider flightintegration.Provider, from, to time.Time) (SyncWindowResult, error) {
	if provider == nil || from.IsZero() || to.IsZero() || !to.After(from) {
		return SyncWindowResult{}, ErrInvalidInput
	}
	providerName := strings.ToLower(strings.TrimSpace(provider.Name()))
	attemptedAt := s.clock.Now().UTC()
	if health, ok := s.repository.(HealthRepository); ok {
		_ = health.RecordSourceAttempt(ctx, providerName, attemptedAt)
	}
	schedules, err := provider.ListSchedules(ctx, from.UTC(), to.UTC())
	if err != nil {
		s.recordProviderFailure(ctx, providerName, attemptedAt, err)
		return SyncWindowResult{}, fmt.Errorf("list schedules from %s: %w", provider.Name(), err)
	}
	events, err := provider.ListEvents(ctx, from.UTC(), to.UTC())
	if err != nil {
		s.recordProviderFailure(ctx, providerName, attemptedAt, err)
		return SyncWindowResult{}, fmt.Errorf("list events from %s: %w", provider.Name(), err)
	}
	if len(schedules)+len(events) == 0 {
		if health, ok := s.repository.(HealthRepository); ok {
			_ = health.RecordSourceSuccess(ctx, providerName, attemptedAt)
		}
		return SyncWindowResult{Schedules: schedules, Events: events}, nil
	}
	result, err := s.Ingest(ctx, IngestInput{Provider: provider.Name(), Schedules: schedules, Events: events, Now: attemptedAt})
	if err != nil {
		s.recordProviderFailure(ctx, providerName, attemptedAt, err)
	}
	return SyncWindowResult{StageResult: result, Schedules: schedules, Events: events}, err
}

func (s *Service) ReconcileSchedules(ctx context.Context, provider string, from, to time.Time, schedules []flightintegration.Schedule, checkedAt time.Time) (ReconciliationResult, error) {
	if s == nil || s.repository == nil {
		return ReconciliationResult{}, ErrRepositoryNotConfigured
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !providerPattern.MatchString(provider) || provider == "manual" || from.IsZero() || to.IsZero() || !to.After(from) {
		return ReconciliationResult{}, ErrInvalidInput
	}
	if checkedAt.IsZero() {
		checkedAt = s.clock.Now().UTC()
	}
	repository, ok := s.repository.(ReconciliationRepository)
	if !ok {
		return ReconciliationResult{}, ErrRepositoryNotConfigured
	}
	return repository.ReconcileSchedules(ctx, provider, from.UTC(), to.UTC(), schedules, checkedAt.UTC())
}

func (s *Service) LatestReconciliation(ctx context.Context, provider string) (ReconciliationResult, error) {
	if s == nil || s.repository == nil {
		return ReconciliationResult{}, ErrRepositoryNotConfigured
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !providerPattern.MatchString(provider) || provider == "manual" {
		return ReconciliationResult{}, ErrInvalidInput
	}
	repository, ok := s.repository.(ReconciliationHealthRepository)
	if !ok {
		return ReconciliationResult{}, ErrRepositoryNotConfigured
	}
	return repository.LatestReconciliation(ctx, provider)
}

func (s *Service) SourceHealth(ctx context.Context, provider string) (SourceHealth, error) {
	if s == nil || s.repository == nil {
		return SourceHealth{}, ErrRepositoryNotConfigured
	}
	health, ok := s.repository.(HealthRepository)
	if !ok {
		return SourceHealth{Provider: strings.ToLower(strings.TrimSpace(provider)), State: SourceStale, FallbackEnabled: true}, nil
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !providerPattern.MatchString(provider) || provider == "manual" {
		return SourceHealth{}, ErrInvalidInput
	}
	value, err := health.GetSourceHealth(ctx, provider)
	if err != nil {
		return SourceHealth{}, err
	}
	if value.State == SourceFresh && value.LastSuccessAt != nil && s.clock.Now().UTC().Sub(value.LastSuccessAt.UTC()) > defaultFreshnessWindow {
		value.State = SourceStale
	}
	if reconciliation, ok := s.repository.(ReconciliationHealthRepository); ok {
		if latest, reconciliationErr := reconciliation.LatestReconciliation(ctx, provider); reconciliationErr == nil {
			if !latest.CheckedAt.IsZero() {
				checkedAt := latest.CheckedAt
				value.LastReconciliationAt = &checkedAt
				value.LastReconciliationStatus = latest.Status
				value.LastReconciliationMismatch = latest.UpstreamMissingCount + latest.CoreMissingCount
			}
		}
	}
	return value, nil
}

func (s *Service) recordProviderFailure(ctx context.Context, provider string, at time.Time, cause error) {
	if health, ok := s.repository.(HealthRepository); ok {
		_ = health.RecordSourceFailure(ctx, provider, at, trimError(cause))
	}
}

func (s *Service) ApplyPending(ctx context.Context, limit int, workerID string) (ApplyResult, error) {
	if s == nil || s.repository == nil {
		return ApplyResult{}, ErrRepositoryNotConfigured
	}
	if limit <= 0 || limit > maxBatchRecords {
		limit = 50
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return ApplyResult{}, ErrInvalidInput
	}
	now := s.clock.Now().UTC()
	records, err := s.repository.ClaimPending(ctx, limit, now, workerID, time.Minute)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("claim flight source records: %w", err)
	}
	result := ApplyResult{Claimed: len(records)}
	for _, record := range records {
		applyErr := s.applyRecord(ctx, record, now)
		if applyErr == nil {
			if err := s.repository.MarkApplied(ctx, record.ID, workerID, now); err != nil {
				return result, fmt.Errorf("mark flight source record %d applied: %w", record.ID, err)
			}
			result.Applied++
			continue
		}
		reason := trimError(applyErr)
		if record.Attempts+1 >= maxApplyAttempts {
			if err := s.repository.MarkFailed(ctx, record.ID, workerID, reason); err != nil {
				return result, fmt.Errorf("mark flight source record %d failed: %w", record.ID, err)
			}
			result.Failed++
			continue
		}
		next := now.Add(retryDelay(record.Attempts + 1))
		if err := s.repository.MarkRetry(ctx, record.ID, workerID, next, reason); err != nil {
			return result, fmt.Errorf("mark flight source record %d retry: %w", record.ID, err)
		}
		result.Retried++
	}
	return result, nil
}

func (s *Service) applyRecord(ctx context.Context, record PendingRecord, now time.Time) error {
	switch record.RecordType {
	case RecordTypeSchedule:
		var schedule flightintegration.Schedule
		if err := json.Unmarshal(record.Payload, &schedule); err != nil {
			return fmt.Errorf("decode schedule payload: %w", err)
		}
		return s.repository.ApplySchedule(ctx, record, schedule, now)
	case RecordTypeEvent:
		var event flightintegration.Event
		if err := json.Unmarshal(record.Payload, &event); err != nil {
			return fmt.Errorf("decode event payload: %w", err)
		}
		if event.Status == "arrived" {
			if s.arrival == nil {
				return ErrRepositoryNotConfigured
			}
			flightPublicID, err := s.repository.FindFlightPublicID(ctx, event.Provider, event.ExternalFlightID)
			if err != nil {
				return fmt.Errorf("resolve arrived flight %s: %w", event.ExternalFlightID, err)
			}
			actualArrivalAt := event.OccurredAt
			if event.ActualArrivalAt != nil {
				actualArrivalAt = *event.ActualArrivalAt
			}
			_, err = s.arrival.RecordFlightArrived(ctx, flighttask.ArrivalInput{FlightPublicID: flightPublicID, SourceEventID: sourceEventID(event.Provider, event.ExternalEventID), OccurredAt: event.OccurredAt, ActualArrivalAt: actualArrivalAt, Source: event.Provider, ActorType: "machine"})
			return err
		}
		return s.repository.ApplyEvent(ctx, record, event, now)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedEvent, record.RecordType)
	}
}

func validateSchedule(value flightintegration.Schedule) error {
	if value.Provider == "" || strings.TrimSpace(value.ExternalFlightID) == "" || strings.TrimSpace(value.FlightDisplayNo) == "" || value.OperatingDate.IsZero() || value.ScheduledAt.IsZero() {
		return ErrInvalidInput
	}
	return nil
}

func validateEvent(value flightintegration.Event) error {
	if value.Provider == "" || strings.TrimSpace(value.ExternalEventID) == "" || strings.TrimSpace(value.ExternalFlightID) == "" || value.OccurredAt.IsZero() {
		return ErrInvalidInput
	}
	switch value.Status {
	case "arrived", "departed", "cancelled":
		return nil
	case "delayed":
		if value.ScheduledAt == nil || value.ScheduledAt.IsZero() {
			return ErrInvalidInput
		}
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedEvent, value.Status)
	}
}

func sourceEventID(provider, externalID string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + ":" + strings.TrimSpace(externalID)
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 8 {
		attempt = 8
	}
	return time.Duration(1<<uint(attempt-1)) * time.Second
}

func trimError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	if len(value) > 1024 {
		return value[:1024]
	}
	return value
}
