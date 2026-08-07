package model

import (
	"time"

	"gorm.io/gorm"
)

// Team 班组。
type Team struct {
	ID        int64          `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Code      string         `gorm:"column:code;size:32;uniqueIndex" json:"code"`
	Name      string         `gorm:"column:name;size:64" json:"name"`
	Enabled   bool           `gorm:"column:enabled;default:true" json:"enabled"`
	CreatedAt time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (Team) TableName() string { return "team" }

// TeamMember 员工与班组关系。
type TeamMember struct {
	ID        int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	TeamID    int64     `gorm:"column:team_id;uniqueIndex:uk_team_member" json:"team_id"`
	UserID    int64     `gorm:"column:user_id;uniqueIndex:uk_team_member" json:"user_id"`
	IsPrimary bool      `gorm:"column:is_primary;default:false" json:"is_primary"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (TeamMember) TableName() string { return "team_member" }

// TaskCandidate 任务候选人，不代表已经确认分配。
type TaskCandidate struct {
	ID        int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	TaskID    int64     `gorm:"column:task_id;uniqueIndex:uk_task_candidate" json:"task_id"`
	UserID    int64     `gorm:"column:user_id;uniqueIndex:uk_task_candidate" json:"user_id"`
	Rank      int       `gorm:"column:rank" json:"rank"`
	MatchedBy string    `gorm:"column:matched_by;size:32" json:"matched_by"`
	Status    string    `gorm:"column:status;size:16" json:"status"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (TaskCandidate) TableName() string { return "task_candidate" }
