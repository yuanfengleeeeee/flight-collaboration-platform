package model

import (
	"time"

	"gorm.io/gorm"
)

// TaskTemplate 任务模板
type TaskTemplate struct {
	ID                   int64          `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Name                 string         `gorm:"column:name;size:64" json:"name"`
	Phase                string         `gorm:"column:phase;size:32;index" json:"phase"`                    // 阶段
	RequiredPosition     string         `gorm:"column:required_position;size:64" json:"required_position"`   // 需求岗位
	RequiredCapability    string         `gorm:"column:required_capability;size:64" json:"required_capability"`
	RequiredCount        int            `gorm:"column:required_count;default:1" json:"required_count"`
	TimeoutSeconds       int            `gorm:"column:timeout_seconds" json:"timeout_seconds"`
	WarningAdvanceSeconds int           `gorm:"column:warning_advance_seconds" json:"warning_advance_seconds"`
	Version              int            `gorm:"column:version;default:1" json:"version"`
	Enabled              bool           `gorm:"column:enabled;default:true" json:"enabled"`
	CreatedAt            time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt            time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt            gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (TaskTemplate) TableName() string { return "task_template" }

// TaskInstance 任务实例
type TaskInstance struct {
	ID           int64          `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	FlightID     int64          `gorm:"column:flight_id;index" json:"flight_id"`
	TemplateID   int64          `gorm:"column:template_id;index" json:"template_id"`
	PlannedStart time.Time      `gorm:"column:planned_start" json:"planned_start"`
	PlannedEnd   time.Time      `gorm:"column:planned_end" json:"planned_end"`
	ActualStart  *time.Time     `gorm:"column:actual_start" json:"actual_start,omitempty"`
	ActualEnd    *time.Time     `gorm:"column:actual_end" json:"actual_end,omitempty"`
	Status       string         `gorm:"column:status;size:32;index" json:"status"` // pending/ongoing/completed/cancelled/timeout
	AssignedCount int           `gorm:"column:assigned_count;default:0" json:"assigned_count"`
	CreatedAt    time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (TaskInstance) TableName() string { return "task_instance" }

// 任务状态常量
const (
	TaskStatusPending   = "pending"
	TaskStatusOngoing   = "ongoing"
	TaskStatusCompleted = "completed"
	TaskStatusCancelled = "cancelled"
	TaskStatusTimeout   = "timeout"
)

// TaskAssignment 任务分配
type TaskAssignment struct {
	ID        int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	TaskID    int64     `gorm:"column:task_id;index" json:"task_id"`
	UserID    int64     `gorm:"column:user_id;index" json:"user_id"`
	Status    string    `gorm:"column:status;size:32" json:"status"` // assigned/accepted/finished
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (TaskAssignment) TableName() string { return "task_assignment" }
