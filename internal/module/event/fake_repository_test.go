package event

import (
	"context"
	"sync"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/model"
)

// fakeRepository is an in-memory transactional fake for pure B3 service tests.
// A transaction works on cloned writes and only publishes them on a nil callback
// error, making rollback assertions possible without SQLite or MySQL.
type fakeRepository struct {
	mu sync.Mutex

	flights   map[int64]model.Flight
	teams     map[int64]model.Team
	templates map[int64]model.TaskTemplate
	events    map[string]model.Event
	tasks     map[int64]model.TaskInstance

	nextEventID int64
	nextTaskID  int64

	createEventErr error
	createTaskErr  error
	linkEventErr   error

	// forceDuplicateEvent simulates another transaction committing after the
	// service's precheck and before its event insert.
	forceDuplicateEvent *model.Event

	transactionCalls int
	createEventCalls int
	createTaskCalls  int
	linkCalls        int
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		flights: map[int64]model.Flight{
			101: {ID: 101, FlightNo: "MU101", Status: "arrived"},
		},
		teams: map[int64]model.Team{
			201: {ID: 201, Code: "team-201", Name: "Team 201", Enabled: true},
		},
		templates: map[int64]model.TaskTemplate{
			301: {
				ID: 301, Name: "Arrival handling", TriggerEventType: model.EventFlightArrived,
				TimeoutSeconds: 900, Version: 7, Enabled: true,
			},
		},
		events:      make(map[string]model.Event),
		tasks:       make(map[int64]model.TaskInstance),
		nextEventID: 401,
		nextTaskID:  501,
	}
}

func (r *fakeRepository) FindEventByIdempotencyKey(_ context.Context, key string) (model.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.events[key]
	if !ok {
		return model.Event{}, ErrNotFound
	}
	return cloneEvent(value), nil
}

func (r *fakeRepository) InTransaction(_ context.Context, fn func(Transaction) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.transactionCalls++
	tx := &fakeTransaction{
		repository:  r,
		events:      cloneEvents(r.events),
		tasks:       cloneTasks(r.tasks),
		nextEventID: r.nextEventID,
		nextTaskID:  r.nextTaskID,
	}
	if err := fn(tx); err != nil {
		return err
	}
	r.events = tx.events
	r.tasks = tx.tasks
	r.nextEventID = tx.nextEventID
	r.nextTaskID = tx.nextTaskID
	return nil
}

func (r *fakeRepository) eventCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func (r *fakeRepository) taskCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.tasks)
}

func (r *fakeRepository) event(key string) (model.Event, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.events[key]
	return cloneEvent(value), ok
}

func (r *fakeRepository) task(id int64) (model.TaskInstance, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.tasks[id]
	return value, ok
}

type fakeTransaction struct {
	repository  *fakeRepository
	events      map[string]model.Event
	tasks       map[int64]model.TaskInstance
	nextEventID int64
	nextTaskID  int64
}

func (tx *fakeTransaction) FindEventByIdempotencyKey(_ context.Context, key string) (model.Event, error) {
	value, ok := tx.events[key]
	if !ok {
		return model.Event{}, ErrNotFound
	}
	return cloneEvent(value), nil
}

func (tx *fakeTransaction) FindFlight(_ context.Context, id int64) (model.Flight, error) {
	value, ok := tx.repository.flights[id]
	if !ok {
		return model.Flight{}, ErrNotFound
	}
	return value, nil
}

func (tx *fakeTransaction) FindTeam(_ context.Context, id int64) (model.Team, error) {
	value, ok := tx.repository.teams[id]
	if !ok {
		return model.Team{}, ErrNotFound
	}
	return value, nil
}

func (tx *fakeTransaction) FindTaskTemplate(_ context.Context, id int64) (model.TaskTemplate, error) {
	value, ok := tx.repository.templates[id]
	if !ok {
		return model.TaskTemplate{}, ErrNotFound
	}
	return value, nil
}

func (tx *fakeTransaction) CreateEvent(_ context.Context, value *model.Event) error {
	tx.repository.createEventCalls++
	if tx.repository.createEventErr != nil {
		return tx.repository.createEventErr
	}
	if tx.repository.forceDuplicateEvent != nil {
		existing := cloneEvent(*tx.repository.forceDuplicateEvent)
		tx.repository.events[existing.IdempotencyKey] = existing
		tx.repository.forceDuplicateEvent = nil
		return ErrDuplicateKey
	}
	if _, exists := tx.events[value.IdempotencyKey]; exists {
		return ErrDuplicateKey
	}
	value.ID = tx.nextEventID
	tx.nextEventID++
	tx.events[value.IdempotencyKey] = cloneEvent(*value)
	return nil
}

func (tx *fakeTransaction) CreateTaskInstance(_ context.Context, value *model.TaskInstance) error {
	tx.repository.createTaskCalls++
	if tx.repository.createTaskErr != nil {
		return tx.repository.createTaskErr
	}
	for _, existing := range tx.tasks {
		if existing.TriggerEventID == value.TriggerEventID {
			return ErrDuplicateKey
		}
	}
	value.ID = tx.nextTaskID
	tx.nextTaskID++
	tx.tasks[value.ID] = *value
	return nil
}

func (tx *fakeTransaction) LinkEventTask(_ context.Context, eventID, taskID int64) error {
	tx.repository.linkCalls++
	if tx.repository.linkEventErr != nil {
		return tx.repository.linkEventErr
	}
	for key, value := range tx.events {
		if value.ID != eventID {
			continue
		}
		value.TaskID = int64Pointer(taskID)
		tx.events[key] = value
		return nil
	}
	return ErrNotFound
}

func cloneEvents(values map[string]model.Event) map[string]model.Event {
	cloned := make(map[string]model.Event, len(values))
	for key, value := range values {
		cloned[key] = cloneEvent(value)
	}
	return cloned
}

func cloneTasks(values map[int64]model.TaskInstance) map[int64]model.TaskInstance {
	cloned := make(map[int64]model.TaskInstance, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneEvent(value model.Event) model.Event {
	if value.FlightID != nil {
		value.FlightID = int64Pointer(*value.FlightID)
	}
	if value.TaskID != nil {
		value.TaskID = int64Pointer(*value.TaskID)
	}
	return value
}

func int64Pointer(value int64) *int64 {
	return &value
}
