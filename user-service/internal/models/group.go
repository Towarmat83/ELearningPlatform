package models

import (
	"time"

	"github.com/google/uuid"
)

// Group is a row in the groups table — either a locally-created admin group
// or one mirrored from an IdP during SSO login.
type Group struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	Name        string    `gorm:"column:name;size:255;not null;uniqueIndex"`
	Description *string   `gorm:"column:description"`
	Source      string    `gorm:"column:source;size:32;not null;default:local"`
	CreatedAt   time.Time `gorm:"column:createdat;not null;default:now()"`
	UpdatedAt   time.Time `gorm:"column:updatedat;not null;default:now()"`
}

// TableName pins Group to the groups table.
func (Group) TableName() string { return "groups" }
