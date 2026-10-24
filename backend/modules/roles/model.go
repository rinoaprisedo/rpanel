package roles

import "rpanel/backend/utils"

type Role struct {
	utils.AuditModel
	// Uniqueness on name is enforced by a partial index (WHERE deleted_at IS
	// NULL) created in migrations, not by this tag — a plain uniqueIndex would
	// block re-using a name after the row holding it is soft-deleted.
	Name         string `gorm:"type:varchar(255);index;not null" json:"name"`
	Description  string `gorm:"type:text" json:"description"`
	IsSuperadmin bool   `gorm:"default:false" json:"is_superadmin"`
}

func (Role) TableName() string { return "roles" }
