package models

import "time"

// ModuleProgress records a user's best-attempt score and pass state for a
// course module.
type ModuleProgress struct {
	ID          string     `gorm:"column:id;primaryKey;default:gen_random_uuid()"`
	UserID      string     `gorm:"column:userid"`
	CourseSlug  string     `gorm:"column:courseslug"`
	ModuleIndex int        `gorm:"column:moduleindex"`
	ModuleSlug  *string    `gorm:"column:moduleslug"`
	BestScore   int        `gorm:"column:bestscore"`
	MaxScore    int        `gorm:"column:maxscore"`
	Passed      bool       `gorm:"column:passed"`
	Attempts    int        `gorm:"column:attempts"`
	CompletedAt *time.Time `gorm:"column:completed_at"` // already snake_case in SQL
	UpdatedAt   time.Time  `gorm:"column:updatedat"`
}

// TableName pins the table name so GORM doesn't infer one from the type
// name.
func (ModuleProgress) TableName() string {
	return "module_progress"
}
