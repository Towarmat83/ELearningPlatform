package models

import (
	"time"

	"github.com/google/uuid"
)

// PathEnrollment records that a user has enrolled in a learning path.
type PathEnrollment struct {
	UserID     uuid.UUID `gorm:"column:userid;primaryKey"`
	PathSlug   string    `gorm:"column:path_slug;primaryKey"`
	EnrolledAt time.Time `gorm:"column:enrolledat"`
}

func (PathEnrollment) TableName() string { return "path_enrollments" }
