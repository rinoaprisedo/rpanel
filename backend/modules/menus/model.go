package menus

import "rpanel/backend/utils"

type Menu struct {
	utils.AuditModel
	MenuSectionID   uint64 `gorm:"not null;index" json:"-"`
	Name            string `gorm:"type:varchar(255);not null" json:"name"`
	Icon            string `gorm:"type:varchar(100)" json:"icon"`
	Path            string `gorm:"type:varchar(255)" json:"path"`
	Order           int    `gorm:"column:order;default:0" json:"order"`
	IsActive        bool   `gorm:"default:true" json:"is_active"`
	MenuSectionUUID string `gorm:"-" json:"menu_section_uuid"`
}

func (Menu) TableName() string { return "menus" }
