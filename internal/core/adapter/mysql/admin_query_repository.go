package mysql

import (
	"context"
	"encoding/json"
	"errors"
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
		a.status, a.status_version, a.receipt_status, a.received_at, a.confirmation_id, a.confirmed_by_public_id, a.confirmed_at,
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

func (r *AdminQueryRepository) ListExceptions(ctx context.Context, filter adminquery.ExceptionFilter) ([]adminquery.Exception, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, adminquery.ErrRepositoryNotConfigured
	}
	query := r.db.WithContext(ctx).Table("task_exception AS e").
		Joins("JOIN task_instance AS ti ON ti.id = e.task_id").
		Joins("JOIN flight AS f ON f.id = ti.flight_id").
		Joins("JOIN personnel AS p ON p.id = e.personnel_id").
		Joins("JOIN team AS t ON t.id = p.team_id").
		Joins("JOIN operation_area AS oa ON oa.id = t.area_id")
	query = applyAssignmentScope(query, filter.Scope)
	if filter.Status != "" {
		query = query.Where("e.status = ?", filter.Status)
	}
	if filter.Severity != "" {
		query = query.Where("e.severity = ?", filter.Severity)
	}
	if filter.TaskPublicID != "" {
		query = query.Where("ti.public_id = ?", filter.TaskPublicID)
	}
	if filter.PersonnelPublicID != "" {
		query = query.Where("p.public_id = ?", filter.PersonnelPublicID)
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count exceptions: %w", err)
	}
	var rows []exceptionManagementRow
	if err := query.Select(`e.public_id, ti.public_id AS task_public_id, f.public_id AS flight_public_id,
		f.flight_display_no, a.public_id AS assignment_public_id, p.public_id AS personnel_public_id,
		p.display_name AS personnel_name, oa.public_id AS area_public_id, t.public_id AS team_public_id,
		e.category, e.severity, e.description, e.status, e.reported_by_public_id, e.reported_at,
		e.resolved_by_public_id, e.resolved_at, e.resolution_note`).
		Joins("JOIN task_assignment AS a ON a.id = e.assignment_id").
		Order("e.reported_at DESC, e.public_id DESC").
		Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("query exceptions: %w", err)
	}
	items := make([]adminquery.Exception, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toDomain())
	}
	return items, total, nil
}

func (r *AdminQueryRepository) GetReportOverview(ctx context.Context, filter adminquery.ReportFilter) (adminquery.ReportOverview, error) {
	if r == nil || r.db == nil {
		return adminquery.ReportOverview{}, adminquery.ErrRepositoryNotConfigured
	}
	result := adminquery.ReportOverview{
		From:             filter.From,
		To:               filter.To,
		TaskCounts:       map[string]int64{},
		FlightCounts:     map[string]int64{},
		AssignmentCounts: map[string]int64{},
		PersonnelCounts:  map[string]int64{},
		ExceptionCounts:  map[string]int64{},
	}

	taskQuery := r.db.WithContext(ctx).Table("task_instance AS ti").Joins("JOIN flight AS f ON f.id = ti.flight_id")
	taskQuery = applyTaskScope(taskQuery, filter.Scope)
	taskQuery = applyReportDateRange(taskQuery, "f", filter)
	if err := scanStatusCounts(taskQuery.Select("ti.status AS status, COUNT(*) AS count").Group("ti.status"), &result.TaskCounts); err != nil {
		return adminquery.ReportOverview{}, fmt.Errorf("count report tasks: %w", err)
	}

	flightQuery := r.db.WithContext(ctx).Table("flight AS f")
	flightQuery = applyFlightScope(flightQuery, filter.Scope)
	flightQuery = applyReportDateRange(flightQuery, "f", filter)
	if err := scanStatusCounts(flightQuery.Select("f.status AS status, COUNT(*) AS count").Group("f.status"), &result.FlightCounts); err != nil {
		return adminquery.ReportOverview{}, fmt.Errorf("count report flights: %w", err)
	}

	assignmentQuery := r.db.WithContext(ctx).Table("task_assignment AS a").Joins("JOIN task_instance AS ti ON ti.id = a.task_id").Joins("JOIN flight AS f ON f.id = ti.flight_id")
	assignmentQuery = applyTaskScope(assignmentQuery, filter.Scope)
	assignmentQuery = applyReportDateRange(assignmentQuery, "f", filter)
	if err := scanStatusCounts(assignmentQuery.Select("a.status AS status, COUNT(*) AS count").Group("a.status"), &result.AssignmentCounts); err != nil {
		return adminquery.ReportOverview{}, fmt.Errorf("count report assignments: %w", err)
	}

	personnelQuery := r.db.WithContext(ctx).Table("personnel AS p").Joins("JOIN team AS t ON t.id = p.team_id").Joins("JOIN operation_area AS oa ON oa.id = t.area_id")
	personnelQuery = applyPersonnelScope(personnelQuery, filter.Scope)
	if err := scanStatusCounts(personnelQuery.Select("p.work_state AS status, COUNT(*) AS count").Group("p.work_state"), &result.PersonnelCounts); err != nil {
		return adminquery.ReportOverview{}, fmt.Errorf("count report personnel: %w", err)
	}

	exceptionQuery := r.db.WithContext(ctx).Table("task_exception AS e").Joins("JOIN task_instance AS ti ON ti.id = e.task_id").Joins("JOIN flight AS f ON f.id = ti.flight_id")
	exceptionQuery = applyTaskScope(exceptionQuery, filter.Scope)
	exceptionQuery = applyReportDateRange(exceptionQuery, "f", filter)
	if err := scanStatusCounts(exceptionQuery.Select("e.status AS status, COUNT(*) AS count").Group("e.status"), &result.ExceptionCounts); err != nil {
		return adminquery.ReportOverview{}, fmt.Errorf("count report exceptions: %w", err)
	}
	return result, nil
}

func (r *AdminQueryRepository) UpdateException(ctx context.Context, input adminquery.ExceptionUpdate) error {
	if r == nil || r.db == nil {
		return adminquery.ErrRepositoryNotConfigured
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current struct {
			ID     uint64 `gorm:"column:id"`
			Status string `gorm:"column:status"`
		}
		query := tx.Table("task_exception AS e").Joins("JOIN task_instance AS ti ON ti.id = e.task_id")
		query = applyTaskScope(query, input.Scope).Where("e.public_id = ?", input.PublicID)
		if err := query.Select("e.id, e.status").First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return adminquery.ErrNotFound
			}
			return fmt.Errorf("find exception for update: %w", err)
		}
		if current.Status == input.Status {
			return nil
		}
		switch current.Status {
		case "open":
			if input.Status != "acknowledged" && input.Status != "resolved" && input.Status != "rejected" {
				return adminquery.ErrConflict
			}
		case "acknowledged":
			if input.Status != "resolved" && input.Status != "rejected" {
				return adminquery.ErrConflict
			}
		default:
			return adminquery.ErrConflict
		}
		updates := map[string]any{"status": input.Status, "resolution_note": nullableReportNote(input.ResolutionNote), "updated_at": input.Now.UTC()}
		if input.Status == "resolved" || input.Status == "rejected" {
			updates["resolved_by_public_id"] = input.ActorPublicID
			updates["resolved_at"] = input.Now.UTC()
		}
		result := tx.Model(&taskExceptionRow{}).Where("id = ? AND status = ?", current.ID, current.Status).Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("update exception status: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return adminquery.ErrConflict
		}
		now := input.Now.UTC()
		if err := tx.Create(&auditRow{ActorType: "human", ActorID: input.ActorPublicID, Action: "task_exception.update", ResourceType: "task_exception", ResourceID: input.PublicID, Result: input.Status, RequestID: input.RequestID, TraceID: input.TraceID, SourceIP: input.SourceIP, OccurredAt: now, CreatedAt: now}).Error; err != nil {
			return fmt.Errorf("create exception audit: %w", err)
		}
		return nil
	})
}

func nullableReportNote(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

type statusCountRow struct {
	Status string `gorm:"column:status"`
	Count  int64  `gorm:"column:count"`
}

func scanStatusCounts(query *gorm.DB, target *map[string]int64) error {
	var rows []statusCountRow
	if err := query.Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		(*target)[row.Status] = row.Count
	}
	return nil
}

func applyReportDateRange(query *gorm.DB, flightAlias string, filter adminquery.ReportFilter) *gorm.DB {
	if filter.From != "" {
		query = query.Where(flightAlias+".operating_date >= ?", filter.From)
	}
	if filter.To != "" {
		query = query.Where(flightAlias+".operating_date <= ?", filter.To)
	}
	return query
}

func applyTaskScope(query *gorm.DB, scope security.AccessScope) *gorm.DB {
	if scope.Global {
		return query
	}
	if scope.UserID != 0 {
		return query.Where("EXISTS (SELECT 1 FROM task_assignment AS scope_assignment WHERE scope_assignment.task_id = ti.id AND scope_assignment.personnel_id = ?)", scope.UserID)
	}
	if len(scope.TeamIDs) > 0 {
		query = query.Where("ti.team_id IN ?", scope.TeamIDs)
	}
	if len(scope.AreaIDs) > 0 {
		query = query.Where("ti.area_id IN ?", scope.AreaIDs)
	}
	return query
}

func applyFlightScope(query *gorm.DB, scope security.AccessScope) *gorm.DB {
	if scope.Global {
		return query
	}
	if scope.UserID != 0 {
		return query.Where("EXISTS (SELECT 1 FROM task_instance AS scope_task JOIN task_assignment AS scope_assignment ON scope_assignment.task_id = scope_task.id WHERE scope_task.flight_id = f.id AND scope_assignment.personnel_id = ?)", scope.UserID)
	}
	if len(scope.TeamIDs) > 0 {
		query = query.Where("EXISTS (SELECT 1 FROM task_instance AS scope_task WHERE scope_task.flight_id = f.id AND scope_task.team_id IN ?)", scope.TeamIDs)
	}
	if len(scope.AreaIDs) > 0 {
		query = query.Where("EXISTS (SELECT 1 FROM task_instance AS scope_task WHERE scope_task.flight_id = f.id AND scope_task.area_id IN ?)", scope.AreaIDs)
	}
	return query
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
	ReceiptStatus       string     `gorm:"column:receipt_status"`
	ConfirmationID      string     `gorm:"column:confirmation_id"`
	ConfirmedByPublicID string     `gorm:"column:confirmed_by_public_id"`
	ConfirmedAt         time.Time  `gorm:"column:confirmed_at"`
	ReceivedAt          *time.Time `gorm:"column:received_at"`
	AcceptedAt          *time.Time `gorm:"column:accepted_at"`
	CompletedAt         *time.Time `gorm:"column:completed_at"`
	CancelledAt         *time.Time `gorm:"column:cancelled_at"`
	CancelReason        string     `gorm:"column:cancel_reason"`
	PlannedAt           time.Time  `gorm:"column:planned_at"`
}

type exceptionManagementRow struct {
	PublicID           string     `gorm:"column:public_id"`
	TaskPublicID       string     `gorm:"column:task_public_id"`
	FlightPublicID     string     `gorm:"column:flight_public_id"`
	FlightDisplayNo    string     `gorm:"column:flight_display_no"`
	AssignmentPublicID string     `gorm:"column:assignment_public_id"`
	PersonnelPublicID  string     `gorm:"column:personnel_public_id"`
	PersonnelName      string     `gorm:"column:personnel_name"`
	AreaPublicID       string     `gorm:"column:area_public_id"`
	TeamPublicID       string     `gorm:"column:team_public_id"`
	Category           string     `gorm:"column:category"`
	Severity           string     `gorm:"column:severity"`
	Description        string     `gorm:"column:description"`
	Status             string     `gorm:"column:status"`
	ReportedByPublicID string     `gorm:"column:reported_by_public_id"`
	ReportedAt         time.Time  `gorm:"column:reported_at"`
	ResolvedByPublicID string     `gorm:"column:resolved_by_public_id"`
	ResolvedAt         *time.Time `gorm:"column:resolved_at"`
	ResolutionNote     string     `gorm:"column:resolution_note"`
}

func (row exceptionManagementRow) toDomain() adminquery.Exception {
	return adminquery.Exception{PublicID: row.PublicID, TaskPublicID: row.TaskPublicID, FlightPublicID: row.FlightPublicID, FlightDisplayNo: row.FlightDisplayNo, AssignmentPublicID: row.AssignmentPublicID, PersonnelPublicID: row.PersonnelPublicID, PersonnelName: row.PersonnelName, AreaPublicID: row.AreaPublicID, TeamPublicID: row.TeamPublicID, Category: row.Category, Severity: row.Severity, Description: row.Description, Status: row.Status, ReportedByPublicID: row.ReportedByPublicID, ReportedAt: formatTime(row.ReportedAt), ResolvedByPublicID: row.ResolvedByPublicID, ResolvedAt: formatOptionalTime(row.ResolvedAt), ResolutionNote: row.ResolutionNote}
}

func (row assignmentManagementRow) toDomain() adminquery.Assignment {
	return adminquery.Assignment{PublicID: row.PublicID, TaskPublicID: row.TaskPublicID, FlightPublicID: row.FlightPublicID, FlightDisplayNo: row.FlightDisplayNo, PersonnelPublicID: row.PersonnelPublicID, PersonnelEmployeeNo: row.PersonnelEmployeeNo, PersonnelName: row.PersonnelName, AreaPublicID: row.AreaPublicID, TeamPublicID: row.TeamPublicID, Status: row.Status, StatusVersion: row.StatusVersion, ReceiptStatus: row.ReceiptStatus, ConfirmationID: row.ConfirmationID, ConfirmedByPublicID: row.ConfirmedByPublicID, ConfirmedAt: formatTime(row.ConfirmedAt), ReceivedAt: formatOptionalTime(row.ReceivedAt), AcceptedAt: formatOptionalTime(row.AcceptedAt), CompletedAt: formatOptionalTime(row.CompletedAt), CancelledAt: formatOptionalTime(row.CancelledAt), CancelReason: row.CancelReason, PlannedAt: formatTime(row.PlannedAt)}
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
