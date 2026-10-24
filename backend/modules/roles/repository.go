package roles

import (
	"rpanel/backend/modules/permissions"
	"rpanel/backend/utils"

	"gorm.io/gorm"
)

// Repository isolates all GORM/SQL access for the roles module.
type Repository struct {
	DB *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{DB: db}
}

func (r *Repository) List(p utils.Pagination) ([]Role, int64, error) {
	q := r.DB.Model(&Role{})
	if p.Search != "" {
		q = q.Where("name ILIKE ?", "%"+p.Search+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []Role
	err := q.Order(p.SortBy + " " + p.SortDir).Offset(p.Offset).Limit(p.Limit).Find(&list).Error
	return list, total, err
}

func (r *Repository) FindByUUID(uuidStr string) (*Role, error) {
	var role Role
	if err := r.DB.Where("uuid = ?", uuidStr).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *Repository) Create(role *Role) error {
	return r.DB.Create(role).Error
}

func (r *Repository) Save(role *Role) error {
	return r.DB.Save(role).Error
}

func (r *Repository) Delete(role *Role, actorID *uint64) error {
	if actorID != nil {
		r.DB.Model(role).Update("deleted_by", actorID)
	}
	return r.DB.Delete(role).Error
}

// PermissionRow is a role's permission joined with the menu it applies to,
// so the frontend can render a name/uuid without a second round-trip.
type PermissionRow struct {
	MenuUUID  string `json:"menu_uuid"`
	MenuName  string `json:"menu_name"`
	CanView   bool   `json:"can_view"`
	CanCreate bool   `json:"can_create"`
	CanEdit   bool   `json:"can_edit"`
	CanDelete bool   `json:"can_delete"`
}

func (r *Repository) Permissions(roleID uint64) ([]PermissionRow, error) {
	perms := []PermissionRow{}
	err := r.DB.Table("permissions").
		Select("menus.uuid AS menu_uuid, menus.name AS menu_name, permissions.can_view, permissions.can_create, permissions.can_edit, permissions.can_delete").
		Joins("JOIN menus ON menus.id = permissions.menu_id").
		Where("permissions.role_id = ?", roleID).
		Scan(&perms).Error
	return perms, err
}

// UserCount returns how many users currently reference this role.
func (r *Repository) UserCount(roleID uint64) (int64, error) {
	var count int64
	err := r.DB.Table("users").Where("role_id = ? AND deleted_at IS NULL", roleID).Count(&count).Error
	return count, err
}

func (r *Repository) MenuIDByUUID(menuUUID string) (uint64, error) {
	var menuID uint64
	row := r.DB.Table("menus").Select("id").Where("uuid = ?", menuUUID).Row()
	err := row.Scan(&menuID)
	return menuID, err
}

func (r *Repository) UpsertPermission(perm *permissions.Permission) error {
	var existing permissions.Permission
	result := r.DB.Where("role_id = ? AND menu_id = ?", perm.RoleID, perm.MenuID).First(&existing)
	if result.Error != nil {
		return r.DB.Create(perm).Error
	}
	perm.ID = existing.ID
	perm.UUID = existing.UUID
	return r.DB.Save(perm).Error
}
