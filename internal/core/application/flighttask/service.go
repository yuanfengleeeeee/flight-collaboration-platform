// Package flighttask contains the cross-module application use case for the
// first business slice: Flight arrival creates a Task and its Candidates.
package flighttask

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	flightmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/flight"
	personnelmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/personnel"
	taskmodule "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/module/task"
	coresync "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/sync"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/clock"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/event"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/shared/id"
)

const (
	OperationFlightArrived = "flight_arrived"

	ResultCreated              = "created"
	ResultCandidateShortage    = "candidate_shortage"
	ResultNoActiveTemplate     = "no_active_template"
	ResultAlreadyProcessed     = "already_processed"
	ResultFlightStatusConflict = "flight_status_conflict"

	idempotencySucceeded = "succeeded"
	idempotencyRejected  = "rejected"
)

var (
	ErrRepositoryNotConfigured = errors.New("flight task repository is not configured")
	ErrNotFound                = errors.New("flight task record not found")
	ErrDuplicate               = errors.New("flight task duplicate record")

	ErrInvalidInput         = &BusinessError{Code: "invalid_input", Message: "invalid flight arrival input"}
	ErrFlightNotFound       = &BusinessError{Code: "flight_not_found", Message: "flight not found"}
	ErrSourceEventConflict  = &BusinessError{Code: "source_event_id_conflict", Message: "source event was already used with different content"}
	ErrFlightStatusConflict = &BusinessError{Code: ResultFlightStatusConflict, Message: "flight status does not allow this arrival"}
	ErrTemplateInvalid      = &BusinessError{Code: "template_invalid", Message: "active task template is invalid"}
)

// BusinessError carries a stable API-safe code while retaining the repository
// error as an unwrap target for diagnostics and tests.
type BusinessError struct {
	Code    string
	Message string
	Cause   error
}

func (e *BusinessError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *BusinessError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func CodeOf(err error) string {
	var businessErr *BusinessError
	if errors.As(err, &businessErr) && businessErr != nil && businessErr.Code != "" {
		return businessErr.Code
	}
	return "internal_error"
}

func MessageOf(err error) string {
	var businessErr *BusinessError
	if errors.As(err, &businessErr) && businessErr != nil && businessErr.Message != "" {
		return businessErr.Message
	}
	return "internal server error"
}

type ArrivalInput struct {
	FlightPublicID  string
	SourceEventID   string
	OccurredAt      time.Time
	ActualArrivalAt time.Time
	Source          string
	ActorType       string
	ActorPublicID   string
	RequestID       string
	TraceID         string
	SourceIP        string
}

type CandidateView struct {
	PublicID          string   `json:"public_id"`
	PersonnelPublicID string   `json:"personnel_public_id"`
	Rank              int      `json:"rank"`
	PositionCode      string   `json:"position_code"`
	Capabilities      []string `json:"capabilities"`
	WorkState         string   `json:"work_state"`
}

type ArrivalResult struct {
	ResultCode     string          `json:"result_code"`
	Duplicate      bool            `json:"duplicate"`
	FlightPublicID string          `json:"flight_public_id"`
	FlightStatus   string          `json:"flight_status"`
	TaskPublicID   string          `json:"task_public_id,omitempty"`
	TaskStatus     string          `json:"task_status,omitempty"`
	GenerationKey  string          `json:"generation_key,omitempty"`
	CandidateCount int             `json:"candidate_count"`
	Candidates     []CandidateView `json:"candidates,omitempty"`
}

type IdempotencyRecord struct {
	PublicID          string
	OperationType     string
	IdempotencyKey    string
	RequestHash       string
	Status            string
	AggregateType     string
	AggregatePublicID string
	ActorType         string
	ActorPublicID     string
	CommandType       string
	ResultCode        string
	ResultStatus      string
	ResultPayload     json.RawMessage
	ErrorSummary      string
	RequestID         string
	TraceID           string
	FirstProcessedAt  time.Time
	LastProcessedAt   time.Time
}

type CandidateFilter struct {
	TeamID       uint64
	AreaID       uint64
	PositionCode string
	PlannedAt    time.Time
}

// Repository is the transaction boundary for the Flight → Task → Candidate
// use case. Its implementation owns GORM; the application service does not.
type Repository interface {
	FindBusinessIdempotency(ctx context.Context, operationType, key string) (IdempotencyRecord, error)
	WithinTransaction(ctx context.Context, fn func(Transaction) error) error
}

type Transaction interface {
	FindFlightForUpdate(ctx context.Context, publicID string) (flightmodule.Record, error)
	FindBusinessIdempotency(ctx context.Context, operationType, key string) (IdempotencyRecord, error)
	FindTaskByGenerationKey(ctx context.Context, generationKey string) (taskmodule.Instance, error)
	FindActiveTemplate(ctx context.Context, triggerType taskmodule.TriggerType) (taskmodule.Template, error)
	ListCandidatePersonnel(ctx context.Context, filter CandidateFilter) ([]personnelmodule.CandidateRecord, error)

	UpdateFlightArrived(ctx context.Context, flightID, expectedVersion uint64, actualArrivalAt, changedAt time.Time) error
	CreateFlightStatusHistory(ctx context.Context, value flightmodule.StatusHistory) error
	CreateTask(ctx context.Context, value *taskmodule.Instance) error
	CreateTaskStatusHistory(ctx context.Context, value taskmodule.StatusHistory) error
	CreateCandidate(ctx context.Context, value *taskmodule.Candidate) error
	CreateBusinessIdempotency(ctx context.Context, value IdempotencyRecord) error
	AppendAudit(ctx context.Context, value coresync.AuditRecord) error
	AppendOutbox(ctx context.Context, value event.EventEnvelope) error
}

type Service struct {
	repository Repository
	clock      clock.Clock
	dispatcher AutomaticTaskDispatcher
}

func NewService(repository Repository, now clock.Clock) *Service {
	if now == nil {
		now = clock.Real{}
	}
	return &Service{repository: repository, clock: now}
}

// SetAutomaticDispatcher wires the machine step after task generation. It is
// optional so generation remains independently testable and a failed selector
// leaves a durable pending_dispatch task for retry or operator attention.
func (s *Service) SetAutomaticDispatcher(dispatcher AutomaticTaskDispatcher) {
	if s != nil {
		s.dispatcher = dispatcher
	}
}

// BuildArrivalIdempotencyKey uses the source event when one exists. The
// fallback is deterministic for the same flight and observed arrival time,
// which keeps a provider retry idempotent when its transport event ID is absent.
func BuildArrivalIdempotencyKey(input ArrivalInput) string {
	if sourceEventID := strings.TrimSpace(input.SourceEventID); sourceEventID != "" {
		return sourceEventID
	}
	return fmt.Sprintf("%s:%s:%s", input.FlightPublicID, taskmodule.TriggerFlightArrived, input.ActualArrivalAt.UTC().Format(time.RFC3339Nano))
}

func (s *Service) RecordFlightArrived(ctx context.Context, input ArrivalInput) (ArrivalResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.repository == nil {
		return ArrivalResult{}, ErrRepositoryNotConfigured
	}

	normalized, err := normalizeArrivalInput(input)
	if err != nil {
		return ArrivalResult{}, err
	}
	key := BuildArrivalIdempotencyKey(normalized)
	requestHash, err := hashArrivalInput(normalized)
	if err != nil {
		return ArrivalResult{}, fmt.Errorf("hash flight arrival input: %w", err)
	}

	if existing, findErr := s.repository.FindBusinessIdempotency(ctx, OperationFlightArrived, key); findErr == nil {
		result, replayErr := replayResult(existing, requestHash)
		if replayErr != nil || result.TaskStatus != string(taskmodule.StatusPendingDispatch) {
			return result, replayErr
		}
		return s.dispatchPendingTask(ctx, result)
	} else if !errors.Is(findErr, ErrNotFound) {
		return ArrivalResult{}, fmt.Errorf("find flight arrival idempotency: %w", findErr)
	}

	var result ArrivalResult
	var businessErr error
	err = s.repository.WithinTransaction(ctx, func(tx Transaction) error {
		// The second read is required after the flight row lock: a concurrent
		// request may have committed the same source event while this request
		// was waiting for the lock.
		if existing, findErr := tx.FindBusinessIdempotency(ctx, OperationFlightArrived, key); findErr == nil {
			result, businessErr = replayResult(existing, requestHash)
			return nil
		} else if !errors.Is(findErr, ErrNotFound) {
			return fmt.Errorf("recheck flight arrival idempotency: %w", findErr)
		}

		flight, findErr := tx.FindFlightForUpdate(ctx, normalized.FlightPublicID)
		if findErr != nil {
			return fmt.Errorf("find flight %s: %w", normalized.FlightPublicID, wrapBusiness(ErrFlightNotFound, findErr))
		}
		generationKey := normalized.FlightPublicID + ":" + string(taskmodule.TriggerFlightArrived)

		if existingTask, taskErr := tx.FindTaskByGenerationKey(ctx, generationKey); taskErr == nil {
			result = resultForTask(ResultAlreadyProcessed, flight, existingTask, nil)
			if err := s.persistOutcome(ctx, tx, normalized, key, requestHash, result, idempotencySucceeded, "", s.now()); err != nil {
				return err
			}
			return s.appendAudit(ctx, tx, normalized, flight.PublicID, result.ResultCode, s.now())
		} else if !errors.Is(taskErr, ErrNotFound) {
			return fmt.Errorf("find existing task for generation key: %w", taskErr)
		}

		if flight.Status != flightmodule.StatusScheduled {
			result = ArrivalResult{ResultCode: ResultAlreadyProcessed, FlightPublicID: flight.PublicID, FlightStatus: string(flight.Status), GenerationKey: generationKey}
			if flight.Status != flightmodule.StatusArrived {
				result.ResultCode = ResultFlightStatusConflict
				businessErr = wrapBusiness(ErrFlightStatusConflict, fmt.Errorf("current status is %s", flight.Status))
			}
			status := idempotencySucceeded
			if businessErr != nil {
				status = idempotencyRejected
			}
			if err := s.persistOutcome(ctx, tx, normalized, key, requestHash, result, status, errorSummary(businessErr), s.now()); err != nil {
				return err
			}
			return s.appendAudit(ctx, tx, normalized, flight.PublicID, result.ResultCode, s.now())
		}

		template, templateErr := tx.FindActiveTemplate(ctx, taskmodule.TriggerFlightArrived)
		if templateErr != nil && !errors.Is(templateErr, ErrNotFound) {
			return fmt.Errorf("find active flight arrival template: %w", templateErr)
		}
		if templateErr == nil {
			if err := validateTemplate(template); err != nil {
				return err
			}
		}

		now := s.now()
		if err := tx.UpdateFlightArrived(ctx, flight.ID, flight.StatusVersion, normalized.ActualArrivalAt, now); err != nil {
			return fmt.Errorf("mark flight %s arrived: %w", flight.PublicID, err)
		}
		fromStatus := flightmodule.StatusScheduled
		historyPublicID, err := newPublicID()
		if err != nil {
			return fmt.Errorf("generate flight status history public id: %w", err)
		}
		if err := tx.CreateFlightStatusHistory(ctx, flightmodule.StatusHistory{
			PublicID:       historyPublicID,
			FlightID:       flight.ID,
			FromStatus:     &fromStatus,
			ToStatus:       flightmodule.StatusArrived,
			TransitionType: string(taskmodule.TriggerFlightArrived),
			SourceEventID:  normalized.SourceEventID,
			ActorType:      normalized.ActorType,
			ActorPublicID:  normalized.ActorPublicID,
			OccurredAt:     normalized.OccurredAt,
		}); err != nil {
			return fmt.Errorf("create flight status history: %w", err)
		}

		if templateErr != nil {
			result = ArrivalResult{ResultCode: ResultNoActiveTemplate, FlightPublicID: flight.PublicID, FlightStatus: string(flightmodule.StatusArrived), GenerationKey: generationKey}
			if err := s.persistOutcome(ctx, tx, normalized, key, requestHash, result, idempotencySucceeded, "", now); err != nil {
				return err
			}
			return s.appendAudit(ctx, tx, normalized, flight.PublicID, result.ResultCode, now)
		}

		plannedAt := normalized.ActualArrivalAt.Add(time.Duration(template.PlannedOffsetSeconds) * time.Second)
		people, err := tx.ListCandidatePersonnel(ctx, CandidateFilter{TeamID: template.TeamID, AreaID: template.AreaID, PositionCode: template.RequiredPositionCode, PlannedAt: plannedAt})
		if err != nil {
			return fmt.Errorf("find candidate personnel: %w", err)
		}
		eligible := filterAndSortCandidates(people, template)

		taskPublicID, err := newPublicID()
		if err != nil {
			return fmt.Errorf("generate task public id: %w", err)
		}
		taskValue := taskmodule.Instance{
			PublicID:             taskPublicID,
			FlightID:             flight.ID,
			FlightPublicID:       flight.PublicID,
			TemplateID:           template.ID,
			TemplatePublicID:     template.PublicID,
			AreaID:               template.AreaID,
			TeamID:               template.TeamID,
			TriggerType:          taskmodule.TriggerFlightArrived,
			GenerationKey:        generationKey,
			SourceEventID:        sourceEventIDOrKey(normalized.SourceEventID, key),
			TemplateVersion:      template.Version,
			RequiredPositionCode: template.RequiredPositionCode,
			RequiredCapabilities: append([]string(nil), template.RequiredCapabilities...),
			Name:                 template.Name,
			Message:              template.DefaultMessage,
			PlannedAt:            plannedAt,
			Status:               taskmodule.StatusPendingDispatch,
			StatusVersion:        0,
			SyncVersion:          0,
		}
		if err := tx.CreateTask(ctx, &taskValue); err != nil {
			return fmt.Errorf("create task instance: %w", err)
		}
		if taskValue.ID == 0 {
			return fmt.Errorf("create task instance: generated id is missing")
		}
		taskHistoryPublicID, err := newPublicID()
		if err != nil {
			return fmt.Errorf("generate task status history public id: %w", err)
		}
		if err := tx.CreateTaskStatusHistory(ctx, taskmodule.StatusHistory{
			PublicID:      taskHistoryPublicID,
			TaskID:        taskValue.ID,
			StatusVersion: 0,
			ToStatus:      taskmodule.StatusPendingDispatch,
			ActorType:     normalized.ActorType,
			ActorPublicID: normalized.ActorPublicID,
			SourceEventID: normalized.SourceEventID,
			OccurredAt:    now,
		}); err != nil {
			return fmt.Errorf("create task status history: %w", err)
		}

		candidateViews := make([]CandidateView, 0, len(eligible))
		for index, person := range eligible {
			candidatePublicID, err := newPublicID()
			if err != nil {
				return fmt.Errorf("generate candidate public id: %w", err)
			}
			candidate := taskmodule.Candidate{
				PublicID:                   candidatePublicID,
				TaskID:                     taskValue.ID,
				PersonnelID:                person.ID,
				PersonnelPublicID:          person.PublicID,
				Rank:                       index + 1,
				Status:                     taskmodule.CandidateProposed,
				MatchedPositionCode:        person.PositionCode,
				MatchedCapabilities:        append([]string(nil), person.Capabilities...),
				PersonnelWorkStateSnapshot: string(person.WorkState),
				PersonnelStateChangedAt:    person.LastStateChangedAt,
			}
			if err := tx.CreateCandidate(ctx, &candidate); err != nil {
				return fmt.Errorf("create candidate for personnel %s: %w", person.PublicID, err)
			}
			candidateViews = append(candidateViews, CandidateView{PublicID: candidate.PublicID, PersonnelPublicID: person.PublicID, Rank: candidate.Rank, PositionCode: candidate.MatchedPositionCode, Capabilities: append([]string(nil), candidate.MatchedCapabilities...), WorkState: candidate.PersonnelWorkStateSnapshot})
		}

		resultCode := ResultCreated
		if len(candidateViews) == 0 {
			resultCode = ResultCandidateShortage
		}
		result = ArrivalResult{ResultCode: resultCode, FlightPublicID: flight.PublicID, FlightStatus: string(flightmodule.StatusArrived), TaskPublicID: taskValue.PublicID, TaskStatus: string(taskValue.Status), GenerationKey: generationKey, CandidateCount: len(candidateViews), Candidates: candidateViews}
		generatedEvent, err := newTaskGeneratedEvent(result, normalized.TraceID, now)
		if err != nil {
			return err
		}
		if err := tx.AppendOutbox(ctx, generatedEvent); err != nil {
			return fmt.Errorf("append task generated outbox: %w", err)
		}
		if err := s.appendAudit(ctx, tx, normalized, taskValue.PublicID, result.ResultCode, now); err != nil {
			return err
		}
		return s.persistOutcome(ctx, tx, normalized, key, requestHash, result, idempotencySucceeded, "", now)
	})
	if err != nil {
		if errors.Is(err, ErrDuplicate) {
			if existing, findErr := s.repository.FindBusinessIdempotency(ctx, OperationFlightArrived, key); findErr == nil {
				result, replayErr := replayResult(existing, requestHash)
				if replayErr != nil || result.TaskStatus != string(taskmodule.StatusPendingDispatch) {
					return result, replayErr
				}
				return s.dispatchPendingTask(ctx, result)
			} else if !errors.Is(findErr, ErrNotFound) {
				return ArrivalResult{}, fmt.Errorf("load flight arrival after duplicate: %w", findErr)
			}
		}
		return ArrivalResult{}, fmt.Errorf("record flight arrival: %w", err)
	}
	if businessErr != nil {
		return result, businessErr
	}
	return s.dispatchPendingTask(ctx, result)
}

func (s *Service) dispatchPendingTask(ctx context.Context, result ArrivalResult) (ArrivalResult, error) {
	if s == nil || s.dispatcher == nil || result.TaskStatus != string(taskmodule.StatusPendingDispatch) || len(result.Candidates) == 0 {
		return result, nil
	}
	dispatched, err := s.dispatcher.Dispatch(ctx, result)
	if err != nil {
		return result, fmt.Errorf("automatically dispatch task %s: %w", result.TaskPublicID, err)
	}
	if dispatched.TaskPublicID != "" {
		result.TaskStatus = dispatched.TaskStatus
	}
	return result, nil
}

func normalizeArrivalInput(input ArrivalInput) (ArrivalInput, error) {
	input.FlightPublicID = strings.TrimSpace(input.FlightPublicID)
	input.SourceEventID = strings.TrimSpace(input.SourceEventID)
	input.Source = strings.TrimSpace(input.Source)
	input.ActorType = strings.TrimSpace(input.ActorType)
	input.ActorPublicID = strings.TrimSpace(input.ActorPublicID)
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.TraceID = strings.TrimSpace(input.TraceID)
	input.SourceIP = strings.TrimSpace(input.SourceIP)
	if input.FlightPublicID == "" || input.OccurredAt.IsZero() || input.ActualArrivalAt.IsZero() {
		return ArrivalInput{}, ErrInvalidInput
	}
	if len(input.SourceEventID) > 128 || len(input.Source) > 64 || len(input.ActorType) > 32 || len(input.ActorPublicID) > 36 || len(input.RequestID) > 128 || len(input.TraceID) > 128 || len(input.SourceIP) > 64 {
		return ArrivalInput{}, ErrInvalidInput
	}
	if input.Source == "" {
		return ArrivalInput{}, ErrInvalidInput
	}
	if input.Source == "manual" || !flightSourcePattern.MatchString(input.Source) {
		return ArrivalInput{}, ErrInvalidInput
	}
	if input.ActorType == "" {
		input.ActorType = "machine"
	}
	var err error
	if input.ActorPublicID == "" {
		input.ActorPublicID, err = id.NewPublicID()
		if err != nil {
			return ArrivalInput{}, fmt.Errorf("generate arrival actor public id: %w", err)
		}
	}
	if input.RequestID == "" {
		input.RequestID, err = id.NewPublicID()
		if err != nil {
			return ArrivalInput{}, fmt.Errorf("generate arrival request id: %w", err)
		}
	}
	if input.TraceID == "" {
		input.TraceID, err = id.NewPublicID()
		if err != nil {
			return ArrivalInput{}, fmt.Errorf("generate arrival trace id: %w", err)
		}
	}
	if input.SourceIP == "" {
		input.SourceIP = "internal"
	}
	input.OccurredAt = input.OccurredAt.UTC()
	input.ActualArrivalAt = input.ActualArrivalAt.UTC()
	return input, nil
}

var flightSourcePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,63}$`)

func hashArrivalInput(input ArrivalInput) (string, error) {
	canonical := struct {
		FlightPublicID  string `json:"flight_public_id"`
		SourceEventID   string `json:"source_event_id"`
		OccurredAt      string `json:"occurred_at"`
		ActualArrivalAt string `json:"actual_arrival_at"`
		Source          string `json:"source"`
	}{FlightPublicID: input.FlightPublicID, SourceEventID: input.SourceEventID, OccurredAt: input.OccurredAt.Format(time.RFC3339Nano), ActualArrivalAt: input.ActualArrivalAt.Format(time.RFC3339Nano), Source: input.Source}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func replayResult(record IdempotencyRecord, requestHash string) (ArrivalResult, error) {
	if record.RequestHash != requestHash {
		return ArrivalResult{}, wrapBusiness(ErrSourceEventConflict, fmt.Errorf("request hash differs for key %q", record.IdempotencyKey))
	}
	if len(record.ResultPayload) == 0 {
		return ArrivalResult{}, fmt.Errorf("idempotency record %s has no result payload", record.PublicID)
	}
	var result ArrivalResult
	if err := json.Unmarshal(record.ResultPayload, &result); err != nil {
		return ArrivalResult{}, fmt.Errorf("decode idempotency result %s: %w", record.PublicID, err)
	}
	result.Duplicate = true
	if record.Status == idempotencyRejected {
		cause := errors.New("previous flight arrival operation was rejected")
		if strings.TrimSpace(record.ErrorSummary) != "" {
			cause = errors.New(record.ErrorSummary)
		}
		switch result.ResultCode {
		case ResultFlightStatusConflict:
			return result, wrapBusiness(ErrFlightStatusConflict, cause)
		default:
			return result, &BusinessError{Code: "operation_rejected", Message: "flight arrival operation was rejected", Cause: cause}
		}
	}
	return result, nil
}

func resultForTask(code string, flight flightmodule.Record, value taskmodule.Instance, candidates []CandidateView) ArrivalResult {
	return ArrivalResult{ResultCode: code, FlightPublicID: flight.PublicID, FlightStatus: string(flight.Status), TaskPublicID: value.PublicID, TaskStatus: string(value.Status), GenerationKey: value.GenerationKey, CandidateCount: len(candidates), Candidates: candidates}
}

func (s *Service) persistOutcome(ctx context.Context, tx Transaction, input ArrivalInput, key, requestHash string, result ArrivalResult, status, errorSummary string, processedAt time.Time) error {
	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal flight arrival result: %w", err)
	}
	publicID, err := id.NewPublicID()
	if err != nil {
		return fmt.Errorf("generate idempotency public id: %w", err)
	}
	return tx.CreateBusinessIdempotency(ctx, IdempotencyRecord{PublicID: publicID, OperationType: OperationFlightArrived, IdempotencyKey: key, RequestHash: requestHash, Status: status, AggregateType: "flight", AggregatePublicID: input.FlightPublicID, ActorType: input.ActorType, ActorPublicID: input.ActorPublicID, ResultCode: result.ResultCode, ResultStatus: result.FlightStatus, ResultPayload: payload, ErrorSummary: errorSummary, RequestID: input.RequestID, TraceID: input.TraceID, FirstProcessedAt: processedAt, LastProcessedAt: processedAt})
}

func (s *Service) appendAudit(ctx context.Context, tx Transaction, input ArrivalInput, resourceID, result string, occurredAt time.Time) error {
	return tx.AppendAudit(ctx, coresync.AuditRecord{ActorType: input.ActorType, ActorID: input.ActorPublicID, Action: "flight.arrived", ResourceType: "flight", ResourceID: resourceID, Result: result, RequestID: input.RequestID, TraceID: input.TraceID, SourceIP: input.SourceIP, OccurredAt: occurredAt})
}

func newTaskGeneratedEvent(result ArrivalResult, traceID string, occurredAt time.Time) (event.EventEnvelope, error) {
	envelope, err := event.NewEvent("task.generated.v1", "task", result.TaskPublicID, "core-flight-task", result)
	if err != nil {
		return event.EventEnvelope{}, fmt.Errorf("create task generated event: %w", err)
	}
	envelope.TraceID = traceID
	envelope.OccurredAt = occurredAt.UTC()
	return envelope, nil
}

func filterAndSortCandidates(values []personnelmodule.CandidateRecord, template taskmodule.Template) []personnelmodule.CandidateRecord {
	result := make([]personnelmodule.CandidateRecord, 0, len(values))
	for _, value := range values {
		if value.TeamID != template.TeamID || value.AreaID != template.AreaID || value.PositionCode != template.RequiredPositionCode || value.WorkState != personnelmodule.WorkStateIdle || !value.Enabled || !hasAllCapabilities(value.Capabilities, template.RequiredCapabilities) {
			continue
		}
		result = append(result, value)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].LastStateChangedAt.Equal(result[j].LastStateChangedAt) {
			return result[i].PublicID < result[j].PublicID
		}
		return result[i].LastStateChangedAt.Before(result[j].LastStateChangedAt)
	})
	return result
}

func hasAllCapabilities(actual, required []string) bool {
	available := make(map[string]struct{}, len(actual))
	for _, capability := range actual {
		available[strings.TrimSpace(capability)] = struct{}{}
	}
	for _, capability := range required {
		if _, ok := available[strings.TrimSpace(capability)]; !ok {
			return false
		}
	}
	return true
}

func validateTemplate(value taskmodule.Template) error {
	if !value.Enabled || value.ID == 0 || value.PublicID == "" || value.TriggerType != taskmodule.TriggerFlightArrived || value.AreaID == 0 || value.TeamID == 0 || strings.TrimSpace(value.Name) == "" || strings.TrimSpace(value.RequiredPositionCode) == "" || value.PlannedOffsetSeconds < 0 {
		return ErrTemplateInvalid
	}
	return nil
}

func sourceEventIDOrKey(sourceEventID, key string) string {
	if sourceEventID != "" {
		return sourceEventID
	}
	return key
}

func (s *Service) now() time.Time {
	if s != nil && s.clock != nil {
		value := s.clock.Now()
		if !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Now().UTC()
}

func newPublicID() (string, error) { return id.NewPublicID() }

func wrapBusiness(base *BusinessError, cause error) error {
	if base == nil {
		return cause
	}
	return &BusinessError{Code: base.Code, Message: base.Message, Cause: cause}
}

func errorSummary(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
