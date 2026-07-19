// Package models contains the GORM-mapped structs for course-service's
// persisted tables.
package models

import (
	"time"

	"github.com/lib/pq"
)

// LabCheck is a single recorded outcome of a lab module check, either
// server-verified (CheckModule) or client-reported (RecordLocalCheck).
type LabCheck struct {
	ID          int64          `gorm:"column:id;primaryKey"`
	Username    string         `gorm:"column:username"`
	CourseSlug  string         `gorm:"column:courseslug"`
	ModuleIndex int            `gorm:"column:moduleindex"`
	ModuleName  string         `gorm:"column:modulename"`
	Allow       bool           `gorm:"column:allow"`
	Violations  pq.StringArray `gorm:"column:violations;type:text[]"`
	CheckedAt   time.Time      `gorm:"column:checkedat"`
	Verified    bool           `gorm:"column:verified"`
}

// TableName pins the table name so GORM doesn't infer one from the type
// name (which would otherwise pluralize to "lab_checks" anyway, but this
// keeps the mapping explicit alongside the column tags above).
func (LabCheck) TableName() string {
	return "lab_checks"
}
