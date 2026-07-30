package model

import (
	"time"

	"gorm.io/gorm"
)

// Rule 规则
type Rule struct {
	ID         int64          `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Type       string         `gorm:"column:type;size:32;index" json:"type"`             // personnel/task/flight/conflict
	Name       string         `gorm:"column:name;size:128" json:"name"`
	Condition  string         `gorm:"column:condition;type:text" json:"condition"`       // JSON 条件
	Action     string         `gorm:"column:action;type:text" json:"action"`             // JSON 动作
	Thresholds string         `gorm:"column:thresholds;type:text" json:"thresholds"`     // JSON 阈值
	Enabled    bool           `gorm:"column:enabled;default:true" json:"enabled"`
	Version    int            `gorm:"column:version;default:1" json:"version"`
	CreatedAt  time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt  time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (Rule) TableName() string { return "rule" }

// 规则类型
const (
	RulePersonnel = "personnel" // 人员资源规则
	RuleTask      = "task"      // 任务进度规则
	RuleFlight    = "flight"    // 航班变化规则
	RuleConflict  = "conflict"  // 资源冲突规则
)

// ============ 运行数据分析中心数据表 (§9.3) ============

// FlightOperationStatistics 航班运行统计表
type FlightOperationStatistics struct {
	ID                    int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	FlightID              int64     `gorm:"column:flight_id;index" json:"flight_id"`
	Date                  string    `gorm:"column:date;size:10;index" json:"date"` // YYYY-MM-DD
	TaskCount             int       `gorm:"column:task_count" json:"task_count"`
	EventCount            int       `gorm:"column:event_count" json:"event_count"`
	CompletionRate        float64   `gorm:"column:completion_rate" json:"completion_rate"`
	NormalCompletedCount  int       `gorm:"column:normal_completed_count" json:"normal_completed_count"`
	AbnormalCount         int       `gorm:"column:abnormal_count" json:"abnormal_count"`
	AvgTaskDuration       float64   `gorm:"column:avg_task_duration" json:"avg_task_duration"` // 秒
	CreatedAt             time.Time `gorm:"column:created_at" json:"created_at"`
}

func (FlightOperationStatistics) TableName() string { return "flight_operation_statistics" }

// EventStatistics 事件统计表
type EventStatistics struct {
	ID                  int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	EventType           string    `gorm:"column:event_type;size:64;index" json:"event_type"`
	StatDate            string    `gorm:"column:stat_date;size:10;index" json:"stat_date"` // YYYY-MM-DD
	Count               int       `gorm:"column:count" json:"count"`
	AvgProcessTime      float64   `gorm:"column:avg_process_time" json:"avg_process_time"` // 秒
	CloseRate           float64   `gorm:"column:close_rate" json:"close_rate"`
	AffectedFlightCount int       `gorm:"column:affected_flight_count" json:"affected_flight_count"`
	CreatedAt           time.Time `gorm:"column:created_at" json:"created_at"`
}

func (EventStatistics) TableName() string { return "event_statistics" }

// ResourceStatistics 资源统计表
type ResourceStatistics struct {
	ID             int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	RoleType       string    `gorm:"column:role_type;size:64;index" json:"role_type"` // 岗位
	StatDate       string    `gorm:"column:stat_date;size:10;index" json:"stat_date"` // YYYY-MM-DD
	TaskCount      int       `gorm:"column:task_count" json:"task_count"`
	ActiveCount    int       `gorm:"column:active_count" json:"active_count"`
	RiskCount      int       `gorm:"column:risk_count" json:"risk_count"`
	IdleCount      int       `gorm:"column:idle_count" json:"idle_count"`
	BusyCount      int       `gorm:"column:busy_count" json:"busy_count"`
	CompletedCount int       `gorm:"column:completed_count" json:"completed_count"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"created_at"`
}

func (ResourceStatistics) TableName() string { return "resource_statistics" }

// PredictionInterface 预测接口表(预留)
type PredictionInterface struct {
	ID               int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	RequestID        string    `gorm:"column:request_id;size:64;uniqueIndex" json:"request_id"`
	RequestPayload   string    `gorm:"column:request_payload;type:text" json:"request_payload"`
	PredictionStatus string    `gorm:"column:prediction_status;size:32;default:not_enabled" json:"prediction_status"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"created_at"`
}

func (PredictionInterface) TableName() string { return "prediction_interface" }
