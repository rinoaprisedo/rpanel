package menus

import "rpanel/backend/utils"

type MenuSection struct {
	utils.AuditModel
	Name  string `gorm:"type:varchar(255);not null" json:"name"`
	Icon  string `gorm:"type:varchar(100)" json:"icon"`
	Order int    `gorm:"column:order;default:0" json:"order"`
}

func (MenuSection) TableName() string { return "menu_sections" }
