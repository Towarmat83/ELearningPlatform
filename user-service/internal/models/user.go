// Package models contains the GORM-mapped structs for user-service's
// persisted tables.
package models

import (
	"time"

	"github.com/google/uuid"
)

// User is a platform account, either local (password-based) or SSO-backed
// (OIDC/LDAP), synced with groups derived from an identity provider.
type User struct {
	ID             uuid.UUID `gorm:"column:id;primaryKey;default:gen_random_uuid()"`
	Username       string    `gorm:"column:username"`
	Email          string    `gorm:"column:email"`
	PasswordHash   *string   `gorm:"column:password_hash"` // nil for SSO-only accounts
	Role           string    `gorm:"column:role"`
	AvatarURL      *string   `gorm:"column:avatarurl"`
	Bio            *string   `gorm:"column:bio"`
	IsActive       bool      `gorm:"column:isactive"`
	AuthProvider   string    `gorm:"column:authprovider"`
	ProviderUserID *string   `gorm:"column:provider_user_id"`
	CreatedAt      time.Time `gorm:"column:createdat"`
	UpdatedAt      time.Time `gorm:"column:updatedat"`
}

// TableName pins the table name so GORM doesn't infer one from the type
// name (which would otherwise pluralize to "users" anyway, but this keeps
// the mapping explicit alongside the column tags above).
func (User) TableName() string {
	return "users"
}
