package models

import (
	"time"

	"github.com/google/uuid"
)

// MarkdownPattern is a reusable markdown rendering rule (global or
// course-scoped) with an HTML/CSS/JS implementation.
type MarkdownPattern struct {
	ID          uuid.UUID  `gorm:"column:id;primaryKey"`
	Name        string     `gorm:"column:name"`
	Label       string     `gorm:"column:label"`
	Description string     `gorm:"column:description"`
	Parameter   string     `gorm:"column:parameter"`
	HTML        string     `gorm:"column:html"`
	CSS         string     `gorm:"column:css"`
	JS          string     `gorm:"column:js"`
	Scope       string     `gorm:"column:scope"`
	CreatedBy   *uuid.UUID `gorm:"column:createdby"`
	FromConfig  bool       `gorm:"column:from_config"`
	CreatedAt   time.Time  `gorm:"column:createdat"`
	UpdatedAt   time.Time  `gorm:"column:updatedat"`
}

func (MarkdownPattern) TableName() string { return "markdown_patterns" }
