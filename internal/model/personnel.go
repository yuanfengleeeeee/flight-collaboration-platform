package model

import (
	"time"

	"gorm.io/gorm"
)

// PersonnelStatus 人员状态
type PersonnelStatus struct {
	ID            int64          `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	UserID        int64          `gorm:"column:user_id;uniqueIndex" json:"user_id"`
	Status        string         `gorm:"column:status;size:16;index" json:"status"` // idle/busy/completed/unavailable/unknown
	CurrentTaskID *int64         `gorm:"column:current_task_id" json:"current_task_id,omitempty"`
	LastEventTime time.Time      `gorm:"column:last_event_time" json:"last_event_time"`
	Source        string         `gorm:"column:source;size:16;default:task" json:"source"` // task / rfid
	CreatedAt     time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (PersonnelStatus) TableName() string { return "personnel_status" }

// 人员状态常量
const (
	PersonnelIdle         = "idle"         // 空闲
	PersonnelBusy         = "busy"         // 保障中
	PersonnelCompleted    = "completed"    // 已完成(短暂态)
	PersonnelUnavailable  = "unavailable"  // 不可用
	PersonnelUnknown      = "unknown"      // 未知
)

// 状态来源
const (
	SourceTask = "task" // 软件事件
	SourceRFID = "rfid" // RFID 设备(后期接入)
)

// Position 岗位
type Position struct {
	ID         int64          `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Category   string         `gorm:"column:category;size:64;index" json:"category"` // 岗位类别
	Name       string         `gorm:"column:name;size:64;uniqueIndex" json:"name"`   // 岗位名称
	Capabilities string       `gorm:"column:capabilities;type:text" json:"capabilities"` // JSON 能力标签数组
	Enabled    bool           `gorm:"column:enabled;default:true" json:"enabled"`
	CreatedAt  time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt  time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (Position) TableName() string { return "position" }

// UserPosition 用户岗位关联
type UserPosition struct {
	ID         int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	UserID     int64     `gorm:"column:user_id;uniqueIndex:idx_user_position" json:"user_id"`
	PositionID int64     `gorm:"column:position_id;uniqueIndex:idx_user_position" json:"position_id"`
	IsPrimary  bool      `gorm:"column:is_primary;default:false" json:"is_primary"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
}

func (UserPosition) TableName() string { return "user_position" }
