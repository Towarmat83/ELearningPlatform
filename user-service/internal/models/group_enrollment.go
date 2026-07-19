package models

import (
	"time"

	"github.com/google/uuid"
)

// GroupEnrollment records a group's enrollment in a course, identified by
// slug — every current and future member of the group is enrolled in the
// course.
type GroupEnrollment struct {
	GroupID    uuid.UUID `gorm:"column:groupid;primaryKey"`
	CourseSlug string    `gorm:"column:courseslug;primaryKey"`
	CreatedAt  time.Time `gorm:"column:createdat"`
}

// TableName pins GroupEnrollment to the group_enrollments table.
func (GroupEnrollment) TableName() string { return "group_enrollments" }
