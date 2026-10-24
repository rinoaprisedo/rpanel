package migrations

import (
	"log"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/modules/cronjobs"
	"rpanel/backend/modules/databases"
	"rpanel/backend/modules/menus"
	"rpanel/backend/modules/permissions"
	"rpanel/backend/modules/roles"
	"rpanel/backend/modules/settings"
	"rpanel/backend/modules/terminal"
	"rpanel/backend/modules/users"
	"rpanel/backend/modules/website"

	"gorm.io/gorm"
)

// Run applies GORM AutoMigrate across every model in the project.
func Run(db *gorm.DB) {
	err := db.AutoMigrate(
		&users.User{},
		&roles.Role{},
		&users.Session{},
		&users.PasswordResetToken{},
		&users.PasswordHistory{},
		&menus.MenuSection{},
		&menus.Menu{},
		&permissions.Permission{},
		&settings.Setting{},
		&activity_logs.ActivityLog{},
		&website.Website{},
		&databases.Server{},
		&cronjobs.Job{},
		&cronjobs.Run{},
		&terminal.Template{},
	)
	if err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	applyPartialUniqueIndexes(db)

	log.Println("migrations applied successfully")
}

// applyPartialUniqueIndexes enforces uniqueness only among non-deleted rows.
// GORM's `uniqueIndex` tag creates a plain unique index that also covers
// soft-deleted rows, which would permanently block re-using an email/name
// after the row holding it is deleted. Idempotent: safe to run on every
// migrate.
func applyPartialUniqueIndexes(db *gorm.DB) {
	statements := []string{
		`DROP INDEX IF EXISTS idx_users_email`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_active ON users (email) WHERE deleted_at IS NULL`,
		`DROP INDEX IF EXISTS idx_roles_name`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_name_active ON roles (name) WHERE deleted_at IS NULL`,
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			log.Fatalf("failed to apply partial unique index (%s): %v", stmt, err)
		}
	}
}
