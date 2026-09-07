package mysql

import (
	"context"
	"time"

	managementrealtime "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/managementrealtime"
	"gorm.io/gorm"
)

var _ managementrealtime.Repository = (*ManagementRealtimeRepository)(nil)

type ManagementRealtimeRepository struct{ db *gorm.DB }

func NewManagementRealtimeRepository(db *gorm.DB) *ManagementRealtimeRepository {
	return &ManagementRealtimeRepository{db: db}
}

func (r *ManagementRealtimeRepository) ListManagementEvents(ctx context.Context, afterID uint64, limit int) ([]managementrealtime.Event, error) {
	if r == nil || r.db == nil {
		return nil, managementrealtime.ErrRepositoryNotConfigured
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var rows []managementRealtimeRow
	query := r.db.WithContext(ctx).Table("outbox_event AS e").
		Select("e.id, e.event_id, e.event_type, e.aggregate_type, e.aggregate_id, ti.team_id, ti.area_id, e.occurred_at, e.payload").
		Joins("LEFT JOIN task_instance AS ti ON e.aggregate_type = 'task' AND ti.public_id = e.aggregate_id").
		Where("e.id > ?", afterID).Order("e.id ASC").Limit(limit)
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]managementrealtime.Event, 0, len(rows))
	for _, row := range rows {
		result = append(result, managementrealtime.Event{ID: row.ID, EventID: row.EventID, EventType: row.EventType, AggregateType: row.AggregateType, AggregatePublicID: row.AggregatePublicID, TeamID: row.TeamID, AreaID: row.AreaID, OccurredAt: row.OccurredAt.UTC(), Payload: append([]byte(nil), row.Payload...)})
	}
	return result, nil
}

type managementRealtimeRow struct {
	ID                uint64    `gorm:"column:id"`
	EventID           string    `gorm:"column:event_id"`
	EventType         string    `gorm:"column:event_type"`
	AggregateType     string    `gorm:"column:aggregate_type"`
	AggregatePublicID string    `gorm:"column:aggregate_id"`
	TeamID            uint64    `gorm:"column:team_id"`
	AreaID            uint64    `gorm:"column:area_id"`
	OccurredAt        time.Time `gorm:"column:occurred_at"`
	Payload           []byte    `gorm:"column:payload"`
}
