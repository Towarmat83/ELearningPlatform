package repository

import "gorm.io/gorm"

// Repositories bundles every repository interface used by user-service's
// handlers, so State only needs to hold one field.
type Repositories struct {
	Users          UserRepository
	Settings       SettingRepository
	Enrollments    EnrollmentRepository
	LessonProgress LessonProgressRepository
	ModuleProgress ModuleProgressRepository
	Groups         GroupRepository
	Patterns       PatternRepository
	Paths          PathEnrollmentRepository
}

// NewGormRepositories builds a Repositories backed by db.
func NewGormRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		Users:          NewGormUserRepository(db),
		Settings:       NewGormSettingRepository(db),
		Enrollments:    NewGormEnrollmentRepository(db),
		LessonProgress: NewGormLessonProgressRepository(db),
		ModuleProgress: NewGormModuleProgressRepository(db),
		Groups:         NewGormGroupRepository(db),
		Patterns:       NewGormPatternRepository(db),
		Paths:          NewGormPathEnrollmentRepository(db),
	}
}
