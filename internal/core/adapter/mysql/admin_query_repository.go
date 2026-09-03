package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	adminquery "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/core/application/adminquery"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
	"gorm.io/gorm"
)

var _ adminquery.Repository = (*AdminQueryRepository)(nil)

type AdminQueryRepository struct{ db *gorm.DB }

func NewAdminQueryRepository(db *gorm.DB) *AdminQueryRepository { return &AdminQueryRepository{db: db} }

func (r *AdminQueryRepository) ListPersonnel(ctx context.Context, filter adminquery.PersonnelFilter) ([]adminquery.Personnel, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, adminquery.ErrRepositoryNotConfigured
	}
	query := r.db.WithContext(ctx).Table("personnel AS p").
		Joins("JOIN team AS t ON t.id = p.team_id").
		Joins("JOIN operation_area AS oa ON oa.id = t.area_id")
	query = applyPersonnelScope(query, filter.Scope)
	if filter.WorkState != "" {
		query = query.Where("p.work_state = ?", filter.WorkState)
	}
	if filter.TeamPublicID != "" {
		query = query.Where("t.public_id = ?", filter.TeamPublicID)
	}
	if filter.AreaPublicID != "" {
		query = query.Where("oa.public_id = ?", filter.AreaPublicID)
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count personnel: %w", err)
	}
	var rows []personnelManagementRow
	if err := query.Select(`p.public_id, p.user_public_id, p.employee_no, p.display_name,
		oa.public_id AS area_public_id, oa.name AS area_name, t.public_id AS team_public_id,
		t.name AS team_name, p.position_code, p.capabilities, p.work_state, p.status_version,
		p.last_state_changed_at, p.unavailable_reason, p.enabled`).
		Order("p.display_name ASC, p.public_id ASC").
		Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("query personnel: %w", err)
	}
	items := make([]adminquery.Personnel, 0, len(rows))
	for _, row := range rows {
		value, err := row.toDomain()
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func (r *AdminQueryRepository) ListAssignments(ctx context.Context, filter adminquery.AssignmentFilter) ([]adminquery.Assignment, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, adminquery.ErrRepositoryNotConfigured
	}
	query := r.db.WithContext(ctx).Table("task_assignment AS a").
		Joins("JOIN task_instance AS ti ON ti.id = a.task_id").
		Joins("JOIN flight AS f ON f.id = ti.flight_id").
		Joins("JOIN personnel AS p ON p.id = a.personnel_id").
		Joins("JOIN team AS t ON t.id = p.team_id").
		Joins("JOIN operation_area AS oa ON oa.id = t.area_id")
	query = applyAssignmentScope(query, filter.Scope)
	if filter.Status != "" {
		query = query.Where("a.status = ?", filter.Status)
	}
	if filter.TaskPublicID != "" {
		query = query.Where("ti.public_id = ?", filter.TaskPublicID)
	}
	if filter.PersonnelPublicID != "" {
		query = query.Where("p.public_id = ?", filter.PersonnelPublicID)
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count assignments: %w", err)
	}
	var rows []assignmentManagementRow
	if err := query.Select(`a.public_id, ti.public_id AS task_public_id, f.public_id AS flight_public_id,
		f.flight_display_no, p.public_id AS personnel_public_id, p.employee_no AS personnel_employee_no,
		p.display_name AS personnel_name, oa.public_id AS area_public_id, t.public_id AS team_public_id,
		a.status, a.status_version, a.confirmation_id, a.confirmed_by_public_id, a.confirmed_at,
		a.accepted_at, a.completed_at, a.cancelled_at, a.cancel_reason, ti.planned_at`).
		Order("ti.planned_at ASC, a.public_id ASC").
		Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("query assignments: %w", err)
	}
	items := make([]adminquery.Assignment, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toDomain())
	}
	return items, total, nil
}

func applyPersonnelScope(query *gorm.DB, scope security.AccessScope) *gorm.DB {
	if scope.Global {
		return query
	}
	if scope.UserID != 0 {
		return query.Where("p.id = ?", scope.UserID)
	}
	if len(scope.TeamIDs) > 0 {
		query = query.Where("p.team_id IN ?", scope.TeamIDs)
	}
	if len(scope.AreaIDs) > 0 {
		query = query.Where("t.area_id IN ?", scope.AreaIDs)
	}
	return query
}

func applyAssignmentScope(query *gorm.DB, scope security.AccessScope) *gorm.DB {
	if scope.Global {
		return query
	}
	if scope.UserID != 0 {
		return query.Where("p.id = ?", scope.UserID)
	}
	if len(scope.TeamIDs) > 0 {
		query = query.Where("p.team_id IN ?", scope.TeamIDs)
	}
	if len(scope.AreaIDs) > 0 {
		query = query.Where("t.area_id IN ?", scope.AreaIDs)
	}
	return query
}

type personnelManagementRow struct {
	PublicID           string    `gorm:"column:public_id"`
	UserPublicID       string    `gorm:"column:user_public_id"`
	EmployeeNo         string    `gorm:"column:employee_no"`
	DisplayName        string    `gorm:"column:display_name"`
	AreaPublicID       string    `gorm:"column:area_public_id"`
	AreaName           string    `gorm:"column:area_name"`
	TeamPublicID       string    `gorm:"column:team_public_id"`
	TeamName           string    `gorm:"column:team_name"`
	PositionCode       string    `gorm:"column:position_code"`
	Capabilities       []byte    `gorm:"column:capabilities"`
	WorkState          string    `gorm:"column:work_state"`
	StatusVersion      uint64    `gorm:"column:status_version"`
	LastStateChangedAt time.Time `gorm:"column:last_state_changed_at"`
	UnavailableReason  string    `gorm:"column:unavailable_reason"`
	Enabled            bool      `gorm:"column:enabled"`
}

func (row personnelManagementRow) toDomain() (adminquery.Personnel, error) {
	var capabilities []string
	if len(strings.TrimSpace(string(row.Capabilities))) > 0 && string(row.Capabilities) != "null" {
		if err := json.Unmarshal(row.Capabilities, &capabilities); err != nil {
			return adminquery.Personnel{}, fmt.Errorf("decode personnel %s capabilities: %w", row.PublicID, err)
		}
	}
	return adminquery.Personnel{PublicID: row.PublicID, UserPublicID: row.UserPublicID, EmployeeNo: row.EmployeeNo, DisplayName: row.DisplayName, AreaPublicID: row.AreaPublicID, AreaName: row.AreaName, TeamPublicID: row.TeamPublicID, TeamName: row.TeamName, PositionCode: row.PositionCode, Capabilities: capabilities, WorkState: row.WorkState, StatusVersion: row.StatusVersion, LastStateChangedAt: formatTime(row.LastStateChangedAt), UnavailableReason: row.UnavailableReason, Enabled: row.Enabled}, nil
}

type assignmentManagementRow struct {
	PublicID            string     `gorm:"column:public_id"`
	TaskPublicID        string     `gorm:"column:task_public_id"`
	FlightPublicID      string     `gorm:"column:flight_public_id"`
	FlightDisplayNo     string     `gorm:"column:flight_display_no"`
	PersonnelPublicID   string     `gorm:"column:personnel_public_id"`
	PersonnelEmployeeNo string     `gorm:"column:personnel_employee_no"`
	PersonnelName       string     `gorm:"column:personnel_name"`
	AreaPublicID        string     `gorm:"column:area_public_id"`
	TeamPublicID        string     `gorm:"column:team_public_id"`
	Status              string     `gorm:"column:status"`
	StatusVersion       uint64     `gorm:"column:status_version"`
	ConfirmationID      string     `gorm:"column:confirmation_id"`
	ConfirmedByPublicID string     `gorm:"column:confirmed_by_public_id"`
	ConfirmedAt         time.Time  `gorm:"column:confirmed_at"`
	AcceptedAt          *time.Time `gorm:"column:accepted_at"`
	CompletedAt         *time.Time `gorm:"column:completed_at"`
	CancelledAt         *time.Time `gorm:"column:cancelled_at"`
	CancelReason        string     `gorm:"column:cancel_reason"`
	PlannedAt           time.Time  `gorm:"column:planned_at"`
}

func (row assignmentManagementRow) toDomain() adminquery.Assignment {
	return adminquery.Assignment{PublicID: row.PublicID, TaskPublicID: row.TaskPublicID, FlightPublicID: row.FlightPublicID, FlightDisplayNo: row.FlightDisplayNo, PersonnelPublicID: row.PersonnelPublicID, PersonnelEmployeeNo: row.PersonnelEmployeeNo, PersonnelName: row.PersonnelName, AreaPublicID: row.AreaPublicID, TeamPublicID: row.TeamPublicID, Status: row.Status, StatusVersion: row.StatusVersion, ConfirmationID: row.ConfirmationID, ConfirmedByPublicID: row.ConfirmedByPublicID, ConfirmedAt: formatTime(row.ConfirmedAt), AcceptedAt: formatOptionalTime(row.AcceptedAt), CompletedAt: formatOptionalTime(row.CompletedAt), CancelledAt: formatOptionalTime(row.CancelledAt), CancelReason: row.CancelReason, PlannedAt: formatTime(row.PlannedAt)}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02T15:04:05.999999Z07:00")
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	formatted := formatTime(*value)
	return &formatted
}
