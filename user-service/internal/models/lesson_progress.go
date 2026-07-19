package models

import "time"

// LessonProgress records that a user has viewed/completed a lesson within a
// course.
type LessonProgress struct {
	ID         string    `gorm:"column:id;primaryKey;default:gen_random_uuid()"`
	UserID     string    `gorm:"column:userid"`
	CourseSlug string    `gorm:"column:courseslug"`
	LessonSlug string    `gorm:"column:lessonslug"`
	ViewedAt   time.Time `gorm:"column:viewed_at"` // already snake_case in SQL
}

// TableName pins the table name so GORM doesn't infer one from the type
// name.
func (LessonProgress) TableName() string {
	return "lesson_progress"
}
