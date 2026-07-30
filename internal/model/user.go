package model

import (
	"time"

	"gorm.io/gorm"
)

// User 用户
type User struct {
	ID        int64          `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Name      string         `gorm:"column:name;size:64" json:"name"`
	EmployeeNo string        `gorm:"column:employee_no;size:32;uniqueIndex" json:"employee_no"` // 工号
	Phone     string         `gorm:"column:phone;size:20" json:"phone"`
	Username  string         `gorm:"column:username;size:64;uniqueIndex" json:"username"`
	Password  string         `gorm:"column:password;size:128" json:"-"`                          // bcrypt 哈希
	Role      string         `gorm:"column:role;size:32;index" json:"role"`                      // manager/leader/staff/admin
	Status    string         `gorm:"column:status;size:32;default:active" json:"status"`         // active/disabled
	CreatedAt time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (User) TableName() string { return "user" }

// 角色常量
const (
	RoleManager = "manager" // 值班经理
	RoleLeader  = "leader"  // 班组长
	RoleStaff   = "staff"   // 一线保障人员
	RoleAdmin   = "admin"   // 系统管理员
)
