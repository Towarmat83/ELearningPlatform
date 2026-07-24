package models

import "time"

// UserSkillLevel records the highest difficulty level a learner has reached
// for a given skill, derived from the courses they have completed.
type UserSkillLevel struct {
	UserID     string    `gorm:"column:userid;type:uuid;not null;uniqueIndex:idx_user_skill_unique,priority:1"`
	Skill      string    `gorm:"column:skill;not null;uniqueIndex:idx_user_skill_unique,priority:2"`
	Level      string    `gorm:"column:level;not null"`      // "beginner" | "intermediate" | "advanced"
	LevelOrder int       `gorm:"column:levelorder;not null"` // 1=beginner, 2=intermediate, 3=advanced
	UpdatedAt  time.Time `gorm:"column:updatedat;not null;default:now()"`
}

// TableName pins the table name so GORM doesn't infer one from the type name.
func (UserSkillLevel) TableName() string {
	return "user_skill_levels"
}
