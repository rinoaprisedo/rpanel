package users

import (
	"time"

	"rpanel/backend/modules/roles"
	"rpanel/backend/utils"

	"gorm.io/gorm"
)

// Repository isolates all GORM/SQL access for the users module so Service
// stays testable against an interface instead of a concrete *gorm.DB.
type Repository struct {
	DB *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{DB: db}
}

func (r *Repository) List(p utils.Pagination, status, roleUUID string) ([]User, int64, error) {
	q := r.DB.Model(&User{}).Preload("Role")

	if p.Search != "" {
		like := "%" + p.Search + "%"
		q = q.Where("name ILIKE ? OR email ILIKE ?", like, like)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if roleUUID != "" {
		q = q.Joins("JOIN roles ON roles.id = users.role_id").
			Where("roles.uuid = ?", roleUUID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var list []User
	err := q.Order(p.SortBy + " " + p.SortDir).Offset(p.Offset).Limit(p.Limit).Find(&list).Error
	return list, total, err
}

// ListAll returns every user matching the filters, unpaginated, for export.
func (r *Repository) ListAll(search, status, roleUUID string) ([]User, error) {
	q := r.DB.Model(&User{}).Preload("Role")

	if search != "" {
		like := "%" + search + "%"
		q = q.Where("name ILIKE ? OR email ILIKE ?", like, like)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if roleUUID != "" {
		q = q.Joins("JOIN roles ON roles.id = users.role_id").
			Where("roles.uuid = ?", roleUUID)
	}

	var list []User
	err := q.Order("created_at desc").Find(&list).Error
	return list, err
}

func (r *Repository) FindByUUID(uuidStr string) (*User, error) {
	var user User
	if err := r.DB.Preload("Role").Where("uuid = ?", uuidStr).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) Create(user *User) error {
	return r.DB.Create(user).Error
}

func (r *Repository) Save(user *User) error {
	return r.DB.Save(user).Error
}

func (r *Repository) Delete(user *User, actorID *uint64) error {
	if actorID != nil {
		r.DB.Model(user).Update("deleted_by", actorID)
	}
	return r.DB.Delete(user).Error
}

// SetRole resolves a role UUID to its numeric ID and assigns it to the user.
// An empty roleUUID clears the user's role.
func (r *Repository) SetRole(user *User, roleUUID string) error {
	if roleUUID == "" {
		user.RoleID = nil
		return r.DB.Model(user).Update("role_id", nil).Error
	}
	var role roles.Role
	if err := r.DB.Where("uuid = ?", roleUUID).First(&role).Error; err != nil {
		return err
	}
	user.RoleID = &role.ID
	return r.DB.Model(user).Update("role_id", role.ID).Error
}

func (r *Repository) UpdateLastLogin(user *User, at time.Time) error {
	return r.DB.Model(user).Update("last_login_at", &at).Error
}

// DeleteAllSessions removes every DB session row for a user, used to mirror a
// Redis-side force-logout-all (e.g. after a role or status change).
func (r *Repository) DeleteAllSessions(userID uint64) error {
	return r.DB.Where("user_id = ?", userID).Delete(&Session{}).Error
}
