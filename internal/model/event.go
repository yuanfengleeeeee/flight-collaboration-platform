package model

import (
	"time"

	"gorm.io/gorm"
)

// Event 事件
type Event struct {
	ID                int64          `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Type              string         `gorm:"column:type;size:64;index" json:"type"`                      // 事件类型
	Level             string         `gorm:"column:level;size:16;index" json:"level"`                    // info/warn/alert/urgent
	Source            string         `gorm:"column:source;size:16" json:"source"`                        // auto/manual
	TriggerTime       time.Time      `gorm:"column:trigger_time;index" json:"trigger_time"`
	FlightID          *int64         `gorm:"column:flight_id;index" json:"flight_id,omitempty"`
	TaskID            *int64         `gorm:"column:task_id;index" json:"task_id,omitempty"`
	AffectedPositions string         `gorm:"column:affected_positions;type:text" json:"affected_positions"` // JSON
	AffectedUsers     string         `gorm:"column:affected_users;type:text" json:"affected_users"`         // JSON
	Status            string         `gorm:"column:status;size:16;index" json:"status"`                    // pending/handling/closed/reviewed
	Title             string         `gorm:"column:title;size:255" json:"title"`
	Description       string         `gorm:"column:description;type:text" json:"description"`
	HandleLogs        string         `gorm:"column:handle_logs;type:text" json:"handle_logs"`             // JSON
	ClosedAt          *time.Time     `gorm:"column:closed_at" json:"closed_at,omitempty"`
	CreatedAt         time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (Event) TableName() string { return "event" }

// 事件类型常量
const (
	EventFlightChange    = "flight_change"     // 航班变化事件
	EventPersonnelShort  = "personnel_shortage" // 人员资源事件
	EventTaskExecution   = "task_execution"     // 任务执行事件
	EventPassenger       = "passenger"          // 旅客保障事件
	EventDevice          = "device"             // 设备资源事件
)

// 事件级别
const (
	EventLevelInfo   = "info"
	EventLevelWarn   = "warn"
	EventLevelAlert  = "alert"
	EventLevelUrgent = "urgent"
)

// 事件状态
const (
	EventStatusPending  = "pending"
	EventStatusHandling = "handling"
	EventStatusClosed   = "closed"
	EventStatusReviewed = "reviewed"
)
