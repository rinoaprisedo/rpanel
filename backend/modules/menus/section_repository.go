package menus

func (r *Repository) FindSectionByUUID(uuidStr string) (*MenuSection, error) {
	var section MenuSection
	if err := r.DB.Where("uuid = ?", uuidStr).First(&section).Error; err != nil {
		return nil, err
	}
	return &section, nil
}

func (r *Repository) CreateSection(section *MenuSection) error {
	return r.DB.Create(section).Error
}

func (r *Repository) SaveSection(section *MenuSection) error {
	return r.DB.Save(section).Error
}

func (r *Repository) DeleteSection(section *MenuSection, actorID *uint64) error {
	if actorID != nil {
		r.DB.Model(section).Update("deleted_by", actorID)
	}
	return r.DB.Delete(section).Error
}

func (r *Repository) UpdateSectionOrder(uuidStr string, order int) error {
	return r.DB.Model(&MenuSection{}).Where("uuid = ?", uuidStr).Update("order", order).Error
}
