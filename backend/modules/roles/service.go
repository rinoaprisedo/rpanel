package roles

import (
	"errors"

	"rpanel/backend/modules/permissions"
	"rpanel/backend/utils"
)

var (
	ErrNotFound         = errors.New("role not found")
	ErrNameInUse        = errors.New("failed to create role (name may already be in use)")
	ErrSuperadminLocked = errors.New("superadmin role is protected from this action")
	ErrRoleInUse        = errors.New("role is still assigned to one or more users")
)

// Service holds business rules for the roles module: superadmin protection
// and permission upsert logic. Repository stays a thin data-access layer.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(p utils.Pagination) ([]Role, int64, error) {
	return s.repo.List(p)
}

func (s *Service) Get(uuidStr string) (*Role, []PermissionRow, error) {
	role, err := s.repo.FindByUUID(uuidStr)
	if err != nil {
		return nil, nil, ErrNotFound
	}
	perms, err := s.repo.Permissions(role.ID)
	if err != nil {
		return nil, nil, err
	}
	return role, perms, nil
}

type RoleInput struct {
	Name         string
	Description  string
	IsSuperadmin bool
	ActorID      *uint64
	// Only a superadmin may create or promote a superadmin role.
	ActorIsSuperadmin bool
}

func (s *Service) Create(in RoleInput) (*Role, error) {
	if in.IsSuperadmin && !in.ActorIsSuperadmin {
		return nil, ErrSuperadminLocked
	}
	role := Role{Name: in.Name, Description: in.Description, IsSuperadmin: in.IsSuperadmin}
	role.CreatedBy = in.ActorID
	if err := s.repo.Create(&role); err != nil {
		return nil, ErrNameInUse
	}
	return &role, nil
}

func (s *Service) Update(uuidStr string, in RoleInput) (before *Role, after *Role, err error) {
	role, err := s.repo.FindByUUID(uuidStr)
	if err != nil {
		return nil, nil, ErrNotFound
	}
	old := *role

	if role.IsSuperadmin && !in.IsSuperadmin {
		return nil, nil, ErrSuperadminLocked
	}
	if (role.IsSuperadmin || in.IsSuperadmin) && !in.ActorIsSuperadmin {
		return nil, nil, ErrSuperadminLocked
	}
	role.Name = in.Name
	role.Description = in.Description
	role.UpdatedBy = in.ActorID

	if err := s.repo.Save(role); err != nil {
		return nil, nil, err
	}
	return &old, role, nil
}

func (s *Service) Delete(uuidStr string, actorID *uint64) (*Role, error) {
	role, err := s.repo.FindByUUID(uuidStr)
	if err != nil {
		return nil, ErrNotFound
	}
	if role.IsSuperadmin {
		return nil, ErrSuperadminLocked
	}
	count, err := s.repo.UserCount(role.ID)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrRoleInUse
	}
	if err := s.repo.Delete(role, actorID); err != nil {
		return nil, err
	}
	return role, nil
}

func (s *Service) Permissions(uuidStr string) ([]PermissionRow, error) {
	role, err := s.repo.FindByUUID(uuidStr)
	if err != nil {
		return nil, ErrNotFound
	}
	return s.repo.Permissions(role.ID)
}

type PermissionInput struct {
	MenuUUID  string
	CanView   bool
	CanCreate bool
	CanEdit   bool
	CanDelete bool
}

// ErrOwnRole stops a role manager from widening the permissions of the role
// they hold themselves (self-escalation); another administrator has to do it.
var ErrOwnRole = errors.New("you cannot change the permissions of your own role")

func (s *Service) UpdatePermissions(uuidStr string, items []PermissionInput, actorID *uint64, actorIsSuperadmin bool) error {
	role, err := s.repo.FindByUUID(uuidStr)
	if err != nil {
		return ErrNotFound
	}
	if role.IsSuperadmin {
		return ErrSuperadminLocked
	}
	if !actorIsSuperadmin {
		if actorID == nil {
			return ErrOwnRole
		}
		var n int64
		s.repo.DB.Table("users").Where("id = ? AND role_id = ? AND deleted_at IS NULL", *actorID, role.ID).Count(&n)
		if n > 0 {
			return ErrOwnRole
		}
	}

	for _, item := range items {
		menuID, err := s.repo.MenuIDByUUID(item.MenuUUID)
		if err != nil {
			continue
		}
		perm := permissions.Permission{
			RoleID:    role.ID,
			MenuID:    menuID,
			CanView:   item.CanView,
			CanCreate: item.CanCreate,
			CanEdit:   item.CanEdit,
			CanDelete: item.CanDelete,
		}
		_ = s.repo.UpsertPermission(&perm)
	}
	return nil
}
