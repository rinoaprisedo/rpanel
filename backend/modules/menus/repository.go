package menus

import (
	"gorm.io/gorm"
)

// Repository isolates all GORM/SQL access for the menus module.
type Repository struct {
	DB *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{DB: db}
}

func (r *Repository) ListSections() ([]MenuSection, error) {
	var sections []MenuSection
	err := r.DB.Order("\"order\" asc").Find(&sections).Error
	return sections, err
}

func (r *Repository) ListMenus(isActive, sectionUUID string) ([]Menu, error) {
	q := r.DB.Model(&Menu{}).Order("\"order\" asc")
	if isActive != "" {
		q = q.Where("is_active = ?", isActive == "true")
	}
	if sectionUUID != "" {
		var sec MenuSection
		if r.DB.Where("uuid = ?", sectionUUID).First(&sec).Error == nil {
			q = q.Where("menu_section_id = ?", sec.ID)
		}
	}
	var allMenus []Menu
	err := q.Find(&allMenus).Error
	return allMenus, err
}

func (r *Repository) FindByUUID(uuidStr string) (*Menu, error) {
	var menu Menu
	if err := r.DB.Where("uuid = ?", uuidStr).First(&menu).Error; err != nil {
		return nil, err
	}
	return &menu, nil
}

func (r *Repository) SectionIDByUUID(uuidStr string) (uint64, error) {
	var section MenuSection
	if err := r.DB.Where("uuid = ?", uuidStr).First(&section).Error; err != nil {
		return 0, err
	}
	return section.ID, nil
}

func (r *Repository) SectionUUIDByID(id uint64) (string, error) {
	var section MenuSection
	if err := r.DB.Select("uuid").First(&section, id).Error; err != nil {
		return "", err
	}
	return section.UUID.String(), nil
}

func (r *Repository) Create(menu *Menu) error {
	return r.DB.Create(menu).Error
}

func (r *Repository) Save(menu *Menu) error {
	return r.DB.Save(menu).Error
}

func (r *Repository) Delete(menu *Menu, actorID *uint64) error {
	if actorID != nil {
		r.DB.Model(menu).Update("deleted_by", actorID)
	}
	return r.DB.Delete(menu).Error
}

func (r *Repository) UpdateOrder(uuidStr string, order int) error {
	return r.DB.Model(&Menu{}).Where("uuid = ?", uuidStr).Update("order", order).Error
}
