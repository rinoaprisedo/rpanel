package menus

import (
	"errors"
)

var (
	ErrNotFound          = errors.New("menu not found")
	ErrInvalidSectionRef = errors.New("invalid menu_section_uuid")
)

type sectionWithMenus struct {
	MenuSection
	Menus []Menu `json:"menus"`
}

// Service holds business rules for the menus module: grouping menus by
// section and resolving section UUID references.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// Tree returns menus grouped by section, per spec. A menu whose section was
// deleted (soft-delete leaves the row's menu_section_id pointing at a
// section no longer in the list) is not itself deleted, so it's surfaced
// under a synthetic "Uncategorized" group instead of silently disappearing.
func (s *Service) Tree(isActive, sectionUUID string) ([]sectionWithMenus, error) {
	sections, err := s.repo.ListSections()
	if err != nil {
		return nil, err
	}
	allMenus, err := s.repo.ListMenus(isActive, sectionUUID)
	if err != nil {
		return nil, err
	}

	matched := make(map[uint64]bool, len(allMenus))
	result := make([]sectionWithMenus, 0, len(sections))
	for _, sec := range sections {
		item := sectionWithMenus{MenuSection: sec, Menus: []Menu{}}
		for _, m := range allMenus {
			if m.MenuSectionID == sec.ID {
				m.MenuSectionUUID = sec.UUID.String()
				item.Menus = append(item.Menus, m)
				matched[m.ID] = true
			}
		}
		result = append(result, item)
	}

	orphaned := make([]Menu, 0)
	for _, m := range allMenus {
		if !matched[m.ID] {
			orphaned = append(orphaned, m)
		}
	}
	if len(orphaned) > 0 {
		result = append(result, sectionWithMenus{
			MenuSection: MenuSection{Name: "Uncategorized", Icon: "FolderOpen"},
			Menus:       orphaned,
		})
	}

	return result, nil
}

func (s *Service) Get(uuidStr string) (*Menu, error) {
	menu, err := s.repo.FindByUUID(uuidStr)
	if err != nil {
		return nil, ErrNotFound
	}
	if sectionUUID, err := s.repo.SectionUUIDByID(menu.MenuSectionID); err == nil {
		menu.MenuSectionUUID = sectionUUID
	}
	return menu, nil
}

type MenuInput struct {
	MenuSectionUUID string
	Name            string
	Icon            string
	Path            string
	Order           int
	IsActive        bool
	ActorID         *uint64
}

func (s *Service) Create(in MenuInput) (*Menu, error) {
	sectionID, err := s.repo.SectionIDByUUID(in.MenuSectionUUID)
	if err != nil {
		return nil, ErrInvalidSectionRef
	}
	menu := Menu{MenuSectionID: sectionID, Name: in.Name, Icon: in.Icon, Path: in.Path, Order: in.Order, IsActive: in.IsActive}
	menu.CreatedBy = in.ActorID
	if err := s.repo.Create(&menu); err != nil {
		return nil, err
	}
	menu.MenuSectionUUID = in.MenuSectionUUID
	return &menu, nil
}

func (s *Service) Update(uuidStr string, in MenuInput) (before *Menu, after *Menu, err error) {
	menu, err := s.repo.FindByUUID(uuidStr)
	if err != nil {
		return nil, nil, ErrNotFound
	}
	old := *menu

	if in.MenuSectionUUID != "" {
		sectionID, err := s.repo.SectionIDByUUID(in.MenuSectionUUID)
		if err != nil {
			return nil, nil, ErrInvalidSectionRef
		}
		menu.MenuSectionID = sectionID
		menu.MenuSectionUUID = in.MenuSectionUUID
	}
	menu.Name = in.Name
	menu.Icon = in.Icon
	menu.Path = in.Path
	menu.Order = in.Order
	menu.IsActive = in.IsActive
	menu.UpdatedBy = in.ActorID

	if err := s.repo.Save(menu); err != nil {
		return nil, nil, err
	}
	return &old, menu, nil
}

func (s *Service) Delete(uuidStr string, actorID *uint64) (*Menu, error) {
	menu, err := s.repo.FindByUUID(uuidStr)
	if err != nil {
		return nil, ErrNotFound
	}
	if err := s.repo.Delete(menu, actorID); err != nil {
		return nil, err
	}
	return menu, nil
}

type ReorderItem struct {
	UUID  string
	Order int
}

func (s *Service) Reorder(items []ReorderItem) {
	for _, item := range items {
		_ = s.repo.UpdateOrder(item.UUID, item.Order)
	}
}
