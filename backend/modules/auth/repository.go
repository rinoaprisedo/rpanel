package auth

import (
	"log"
	"time"

	"rpanel/backend/modules/users"

	"gorm.io/gorm"
)

// Repository isolates all GORM/SQL access for the auth module. It reuses the
// users module's models (User, Session, PasswordResetToken, PasswordHistory)
// since login/session/password-reset all revolve around a user record.
type Repository struct {
	DB *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{DB: db}
}

func (r *Repository) FindUserByEmail(email string) (*users.User, error) {
	var user users.User
	if err := r.DB.Preload("Role").Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) FindUserByID(id uint64) (*users.User, error) {
	var user users.User
	if err := r.DB.Preload("Role").First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) UpdateLastLogin(user *users.User, at time.Time) {
	r.DB.Model(user).Update("last_login_at", &at)
}

func (r *Repository) CreateSession(session *users.Session) error {
	return r.DB.Create(session).Error
}

func (r *Repository) DeleteSessionByToken(token string) {
	r.DB.Where("token = ?", token).Delete(&users.Session{})
}

func (r *Repository) CreatePasswordResetToken(token *users.PasswordResetToken) error {
	return r.DB.Create(token).Error
}

func (r *Repository) FindValidResetToken(token string) (*users.PasswordResetToken, error) {
	var reset users.PasswordResetToken
	if err := r.DB.Where("token = ? AND used_at IS NULL", token).First(&reset).Error; err != nil {
		return nil, err
	}
	return &reset, nil
}

func (r *Repository) MarkResetTokenUsed(reset *users.PasswordResetToken, at time.Time) {
	r.DB.Model(reset).Update("used_at", &at)
}

func (r *Repository) UpdatePassword(user *users.User, hashed string) {
	r.DB.Model(user).Update("password", hashed)
}

func (r *Repository) CreatePasswordHistory(entry *users.PasswordHistory) {
	r.DB.Create(entry)
}

func (r *Repository) RecentPasswordHistory(userID uint64, limit int) []users.PasswordHistory {
	var histories []users.PasswordHistory
	r.DB.Where("user_id = ?", userID).Order("created_at desc").Limit(limit).Find(&histories)
	return histories
}

// rolePermissionRow mirrors PermissionDTO but with int flags: Postgres has
// no MAX(boolean) aggregate (MySQL does, since booleans are tinyint there),
// so the query aggregates 0/1 via CASE WHEN for portability across both
// supported drivers.
type rolePermissionRow struct {
	Path      string `json:"path"`
	CanView   int    `json:"can_view"`
	CanCreate int    `json:"can_create"`
	CanEdit   int    `json:"can_edit"`
	CanDelete int    `json:"can_delete"`
}

func (r *Repository) RolePermissionsFor(roleIDs []uint64) []PermissionDTO {
	perms := []PermissionDTO{}
	if len(roleIDs) == 0 {
		return perms
	}
	var rows []rolePermissionRow
	err := r.DB.Table("permissions").
		Select(`menus.path AS path,
			MAX(CASE WHEN permissions.can_view THEN 1 ELSE 0 END) AS can_view,
			MAX(CASE WHEN permissions.can_create THEN 1 ELSE 0 END) AS can_create,
			MAX(CASE WHEN permissions.can_edit THEN 1 ELSE 0 END) AS can_edit,
			MAX(CASE WHEN permissions.can_delete THEN 1 ELSE 0 END) AS can_delete`).
		Joins("JOIN menus ON menus.id = permissions.menu_id").
		Where("permissions.role_id IN ?", roleIDs).
		Group("menus.path").
		Scan(&rows).Error
	if err != nil {
		log.Printf("RolePermissionsFor query failed: %v", err)
		return perms
	}
	for _, row := range rows {
		perms = append(perms, PermissionDTO{
			Path:      row.Path,
			CanView:   row.CanView == 1,
			CanCreate: row.CanCreate == 1,
			CanEdit:   row.CanEdit == 1,
			CanDelete: row.CanDelete == 1,
		})
	}
	return perms
}
