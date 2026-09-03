package event

import (
	"context"
	"errors"
	"fmt"

	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/model"
	"gorm.io/gorm"
)

// GORMRepository is the MySQL/GORM implementation used by the B3 service.
type GORMRepository struct {
	db *gorm.DB
}

// NewGORMRepository constructs a repository over an existing GORM MySQL handle.
func NewGORMRepository(db *gorm.DB) *GORMRepository {
	return &GORMRepository{db: db}
}

// FindEventByIdempotencyKey loads a previously recorded event outside a transaction.
func (r *GORMRepository) FindEventByIdempotencyKey(ctx context.Context, key string) (model.Event, error) {
	db, err := r.scoped(ctx)
	if err != nil {
		return model.Event{}, err
	}
	return findEventByIdempotencyKey(ctx, db, key)
}

// InTransaction executes the B3 writes in one database transaction.
func (r *GORMRepository) InTransaction(ctx context.Context, fn func(Transaction) error) error {
	if fn == nil {
		return fmt.Errorf("transaction callback is required: %w", ErrInvalidInput)
	}
	db, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		return fn(&gormTransaction{db: tx.WithContext(ctx)})
	})
}

func (r *GORMRepository) scoped(ctx context.Context) (*gorm.DB, error) {
	if r == nil || r.db == nil {
		return nil, ErrRepositoryUnavailable
	}
	return r.db.WithContext(ctx), nil
}

type gormTransaction struct {
	db *gorm.DB
}

func (tx *gormTransaction) FindEventByIdempotencyKey(ctx context.Context, key string) (model.Event, error) {
	return findEventByIdempotencyKey(ctx, tx.db, key)
}

func (tx *gormTransaction) FindFlight(ctx context.Context, id int64) (model.Flight, error) {
	var value model.Flight
	err := tx.db.WithContext(ctx).First(&value, id).Error
	return value, normalizeNotFound(err)
}

func (tx *gormTransaction) FindTeam(ctx context.Context, id int64) (model.Team, error) {
	var value model.Team
	err := tx.db.WithContext(ctx).First(&value, id).Error
	return value, normalizeNotFound(err)
}

func (tx *gormTransaction) FindTaskTemplate(ctx context.Context, id int64) (model.TaskTemplate, error) {
	var value model.TaskTemplate
	err := tx.db.WithContext(ctx).First(&value, id).Error
	return value, normalizeNotFound(err)
}

func (tx *gormTransaction) CreateEvent(ctx context.Context, value *model.Event) error {
	if value == nil {
		return fmt.Errorf("event is required: %w", ErrInvalidInput)
	}
	if err := tx.db.WithContext(ctx).Create(value).Error; err != nil {
		return normalizeDuplicateKey(err)
	}
	return nil
}

func (tx *gormTransaction) CreateTaskInstance(ctx context.Context, value *model.TaskInstance) error {
	if value == nil {
		return fmt.Errorf("task instance is required: %w", ErrInvalidInput)
	}
	if err := tx.db.WithContext(ctx).Create(value).Error; err != nil {
		return normalizeDuplicateKey(err)
	}
	return nil
}

func (tx *gormTransaction) LinkEventTask(ctx context.Context, eventID, taskID int64) error {
	result := tx.db.WithContext(ctx).Model(&model.Event{}).Where("id = ?", eventID).Update("task_id", taskID)
	if result.Error != nil {
		return normalizeDuplicateKey(result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrNotFound
	}
	return nil
}

func findEventByIdempotencyKey(ctx context.Context, db *gorm.DB, key string) (model.Event, error) {
	var value model.Event
	err := db.WithContext(ctx).Where("idempotency_key = ?", key).First(&value).Error
	return value, normalizeNotFound(err)
}

func normalizeNotFound(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return err
}

func normalizeDuplicateKey(err error) error {
	if isDuplicateKeyError(err) {
		return fmt.Errorf("%w: %w", ErrDuplicateKey, err)
	}
	return err
}

func isDuplicateKeyError(err error) bool {
	var mysqlErr *drivermysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
