package models

import (
	"time"

	"github.com/google/uuid"
)

// Group is a row in the groups table — either a locally-created admin group
// or one mirrored from an IdP during SSO login.
type Group struct {
	ID          uuid.UUID `gorm:"column:id;primaryKey"`
	Name        string    `gorm:"column:name"`
	Description *string   `gorm:"column:description"`
	Source      string    `gorm:"column:source"`
	CreatedAt   time.Time `gorm:"column:createdat"`
	UpdatedAt   time.Time `gorm:"column:updatedat"`
}

// TableName pins Group to the groups table.
func (Group) TableName() string { return "groups" }
