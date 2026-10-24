package permissions

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Permission maps a role's allowed actions on a menu. Kept minimal per spec
// (no soft-delete/audit fields listed for this table).
type Permission struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"-"`
	UUID      uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"uuid"`
	RoleID    uint64    `gorm:"not null;index" json:"-"`
	MenuID    uint64    `gorm:"not null;index" json:"-"`
	CanView   bool      `gorm:"default:false" json:"can_view"`
	CanCreate bool      `gorm:"default:false" json:"can_create"`
	CanEdit   bool      `gorm:"default:false" json:"can_edit"`
	CanDelete bool      `gorm:"default:false" json:"can_delete"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Permission) TableName() string { return "permissions" }

func (p *Permission) BeforeCreate(tx *gorm.DB) error {
	if p.UUID == uuid.Nil {
		p.UUID = uuid.New()
	}
	return nil
}
