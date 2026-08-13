package models

import "time"

// UserXPEvent records a single XP gain for a learner.
// The XP total is derived by summing all events for a given user.
type UserXPEvent struct {
	ID         uint      `gorm:"column:id;primaryKey;autoIncrement"`
	UserID     string    `gorm:"column:userid;type:uuid;not null;index"`
	Source     string    `gorm:"column:source;size:16;not null"` // "lesson"|"module"|"course"
	SourceSlug string    `gorm:"column:source_slug;size:255;not null"`
	Amount     int       `gorm:"column:amount;not null"`
	EarnedAt   time.Time `gorm:"column:earned_at;not null;default:now()"`
}

// TableName pins the table name so GORM doesn't infer one from the type name.
func (UserXPEvent) TableName() string {
	return "user_xp_events"
}
