package model

import "time"

// Notification 站内通知记录；真实推送渠道由后续适配器接入。
type Notification struct {
	ID        int64      `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	UserID    int64      `gorm:"column:user_id;index" json:"user_id"`
	TaskID    *int64     `gorm:"column:task_id;index" json:"task_id,omitempty"`
	EventID   *int64     `gorm:"column:event_id;index" json:"event_id,omitempty"`
	DedupeKey string     `gorm:"column:dedupe_key;size:191;uniqueIndex" json:"dedupe_key"`
	Type      string     `gorm:"column:type;size:32" json:"type"`
	Status    string     `gorm:"column:status;size:16;index" json:"status"`
	Title     string     `gorm:"column:title;size:255" json:"title"`
	Content   string     `gorm:"column:content;type:text" json:"content"`
	SentAt    *time.Time `gorm:"column:sent_at" json:"sent_at,omitempty"`
	ReadAt    *time.Time `gorm:"column:read_at" json:"read_at,omitempty"`
	CreatedAt time.Time  `gorm:"column:created_at" json:"created_at"`
}

func (Notification) TableName() string { return "notification" }
