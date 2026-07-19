package models

import "github.com/google/uuid"

// UserGroup is a row in the user_groups join table linking a user to a
// group. UserID stays a plain string (not uuid.UUID) to match the
// codebase-wide convention that user IDs are handled as raw strings
// (e.g. claims.Subject).
type UserGroup struct {
	UserID  string    `gorm:"column:userid;primaryKey"`
	GroupID uuid.UUID `gorm:"column:groupid;primaryKey"`
}

// TableName pins UserGroup to the user_groups table.
func (UserGroup) TableName() string { return "user_groups" }
