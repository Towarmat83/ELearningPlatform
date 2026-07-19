package models

import "time"

// Enrollment records a user's enrollment in a course, identified by slug.
type Enrollment struct {
	ID         string    `gorm:"column:id;primaryKey;default:gen_random_uuid()"`
	UserID     string    `gorm:"column:userid"`
	CourseSlug string    `gorm:"column:courseslug"`
	EnrolledAt time.Time `gorm:"column:enrolledat"`
}

// TableName pins the table name so GORM doesn't infer one from the type
// name.
func (Enrollment) TableName() string {
	return "enrollments"
}
