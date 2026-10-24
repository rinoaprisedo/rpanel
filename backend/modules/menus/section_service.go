package menus

import "errors"

var ErrSectionNotFound = errors.New("menu section not found")

func (s *Service) ListSections() ([]MenuSection, error) {
	return s.repo.ListSections()
}

func (s *Service) GetSection(uuidStr string) (*MenuSection, error) {
	section, err := s.repo.FindSectionByUUID(uuidStr)
	if err != nil {
		return nil, ErrSectionNotFound
	}
	return section, nil
}

type SectionInput struct {
	Name    string
	Icon    string
	Order   int
	ActorID *uint64
}

func (s *Service) CreateSection(in SectionInput) (*MenuSection, error) {
	section := MenuSection{Name: in.Name, Icon: in.Icon, Order: in.Order}
	section.CreatedBy = in.ActorID
	if err := s.repo.CreateSection(&section); err != nil {
		return nil, err
	}
	return &section, nil
}

func (s *Service) UpdateSection(uuidStr string, in SectionInput) (before *MenuSection, after *MenuSection, err error) {
	section, err := s.repo.FindSectionByUUID(uuidStr)
	if err != nil {
		return nil, nil, ErrSectionNotFound
	}
	old := *section

	section.Name = in.Name
	section.Icon = in.Icon
	section.Order = in.Order
	section.UpdatedBy = in.ActorID

	if err := s.repo.SaveSection(section); err != nil {
		return nil, nil, err
	}
	return &old, section, nil
}

func (s *Service) DeleteSection(uuidStr string, actorID *uint64) (*MenuSection, error) {
	section, err := s.repo.FindSectionByUUID(uuidStr)
	if err != nil {
		return nil, ErrSectionNotFound
	}
	if err := s.repo.DeleteSection(section, actorID); err != nil {
		return nil, err
	}
	return section, nil
}

type SectionReorderItem struct {
	UUID  string
	Order int
}

func (s *Service) ReorderSections(items []SectionReorderItem) {
	for _, item := range items {
		_ = s.repo.UpdateSectionOrder(item.UUID, item.Order)
	}
}
