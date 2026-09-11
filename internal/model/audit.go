package model

import "time"

// AuditLog 记录关键状态和权限操作。
type AuditLog struct {
	ID           int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	ActorUserID  *int64    `gorm:"column:actor_user_id;index" json:"actor_user_id,omitempty"`
	Action       string    `gorm:"column:action;size:64;index" json:"action"`
	ResourceType string    `gorm:"column:resource_type;size:64;index" json:"resource_type"`
	ResourceID   int64     `gorm:"column:resource_id;index" json:"resource_id"`
	FromStatus   string    `gorm:"column:from_status;size:32" json:"from_status"`
	ToStatus     string    `gorm:"column:to_status;size:32" json:"to_status"`
	Metadata     string    `gorm:"column:metadata;type:text" json:"metadata"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
}

func (AuditLog) TableName() string { return "audit_log" }
