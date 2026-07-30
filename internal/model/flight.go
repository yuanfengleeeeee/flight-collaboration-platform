package model

import (
	"time"

	"gorm.io/gorm"
)

// Flight 航班
type Flight struct {
	ID              int64          `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	FlightNo        string         `gorm:"column:flight_no;size:32;index" json:"flight_no"`          // 航班号 CA1234
	AircraftType    string         `gorm:"column:aircraft_type;size:32" json:"aircraft_type"`        // 机型
	Origin          string         `gorm:"column:origin;size:8" json:"origin"`                       // 出发机场
	Destination     string         `gorm:"column:destination;size:8" json:"destination"`             // 到达机场
	PlannedDeparture time.Time     `gorm:"column:planned_departure" json:"planned_departure"`         // 计划起飞
	PlannedArrival   time.Time     `gorm:"column:planned_arrival" json:"planned_arrival"`             // 计划到达
	ActualDeparture  *time.Time    `gorm:"column:actual_departure" json:"actual_departure,omitempty"`
	ActualArrival    *time.Time    `gorm:"column:actual_arrival" json:"actual_arrival,omitempty"`
	Gate            string         `gorm:"column:gate;size:16" json:"gate"`                          // 登机口
	Stand           string         `gorm:"column:stand;size:16" json:"stand"`                        // 机位
	Status          string         `gorm:"column:status;size:32;index" json:"status"`                // 航班状态
	Source          string         `gorm:"column:source;size:16;default:manual" json:"source"`       // manual / sync
	CreatedAt       time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (Flight) TableName() string { return "flight" }
