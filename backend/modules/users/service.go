package users

import (
	"context"
	"errors"
	"mime/multipart"

	"rpanel/backend/session"
	"rpanel/backend/storage"
	"rpanel/backend/utils"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrNotFound          = errors.New("user not found")
	ErrEmailInUse        = errors.New("failed to create user (email may already be in use)")
	ErrSuperadminLocked  = errors.New("superadmin user is protected from this action")
	ErrSuperadminRole    = errors.New("only a superadmin can assign the superadmin role")
	ErrBadStatus         = errors.New("status must be active, inactive or suspended")
	ErrWeakPassword      = errors.New("password must be at least 8 characters")
	ErrSelfDelete        = errors.New("you cannot delete your own account")
	ErrAvatarFileMissing = errors.New("avatar file is required")
)

// Service holds business rules for the users module: password hashing,
// superadmin protection, role assignment. Repository stays a thin data-access
// layer so this logic can be unit tested without a real database.
type Service struct {
	repo    *Repository
	storage storage.StorageInterface
	redis   *redis.Client
}

func NewService(repo *Repository, s storage.StorageInterface, rdb *redis.Client) *Service {
	return &Service{repo: repo, storage: s, redis: rdb}
}

// invalidateSessions force-logs-out every active session for a user. Called
// after a role or status change so a privilege downgrade (or suspension)
// takes effect immediately instead of waiting for the session's natural TTL —
// SessionAuth re-checks role/status per request, but the old token would
// otherwise keep working until it expires.
func (s *Service) invalidateSessions(userID uint64) {
	_ = session.DeleteAllForUser(context.Background(), s.redis, userID)
	_ = s.repo.DeleteAllSessions(userID)
}

func (s *Service) List(p utils.Pagination, status, roleUUID string) ([]User, int64, error) {
	return s.repo.List(p, status, roleUUID)
}

func (s *Service) ListAll(search, status, roleUUID string) ([]User, error) {
	return s.repo.ListAll(search, status, roleUUID)
}

func (s *Service) Get(uuidStr string) (*User, error) {
	user, err := s.repo.FindByUUID(uuidStr)
	if err != nil {
		return nil, ErrNotFound
	}
	return user, nil
}

type CreateInput struct {
	Name     string
	Email    string
	Password string
	Status   string
	RoleUUID string
	ActorID  *uint64
	// ActorIsSuperadmin gates handing out the superadmin role: a user manager
	// must not be able to create a superadmin (or become one).
	ActorIsSuperadmin bool
}

func validStatus(s string) bool {
	switch UserStatus(s) {
	case StatusActive, StatusInactive, StatusSuspended:
		return true
	}
	return false
}

func (s *Service) Create(in CreateInput) (*User, error) {
	if in.RoleUUID != "" && !in.ActorIsSuperadmin && s.repo.RoleIsSuperadmin(in.RoleUUID) {
		return nil, ErrSuperadminRole
	}
	if in.Status != "" && !validStatus(in.Status) {
		return nil, ErrBadStatus
	}
	if len(in.Password) < 8 {
		return nil, ErrWeakPassword
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(in.Password), 12)
	if err != nil {
		return nil, err
	}

	status := StatusActive
	if in.Status != "" {
		status = UserStatus(in.Status)
	}

	user := User{
		Name:     in.Name,
		Email:    in.Email,
		Password: string(hashed),
		Status:   status,
	}
	user.CreatedBy = in.ActorID

	if err := s.repo.Create(&user); err != nil {
		return nil, ErrEmailInUse
	}

	if in.RoleUUID != "" {
		_ = s.repo.SetRole(&user, in.RoleUUID)
	}

	return &user, nil
}

type UpdateInput struct {
	Name         string
	Email        string
	Password     string
	Status       string
	RoleUUID     string
	RoleProvided bool
	ActorID      *uint64
	// See CreateInput.ActorIsSuperadmin; additionally a non-superadmin may not
	// edit a superadmin account at all (password/email change = takeover).
	ActorIsSuperadmin bool
}

// Update returns the pre-update snapshot alongside the saved user so callers
// can log a before/after activity diff.
func (s *Service) Update(uuidStr string, in UpdateInput) (before *User, after *User, err error) {
	user, err := s.repo.FindByUUID(uuidStr)
	if err != nil {
		return nil, nil, ErrNotFound
	}
	if !in.ActorIsSuperadmin {
		if hasSuperadminRole(user) {
			return nil, nil, ErrSuperadminLocked
		}
		if in.RoleProvided && in.RoleUUID != "" && s.repo.RoleIsSuperadmin(in.RoleUUID) {
			return nil, nil, ErrSuperadminRole
		}
	}
	if in.Status != "" && !validStatus(in.Status) {
		return nil, nil, ErrBadStatus
	}
	if in.Password != "" && len(in.Password) < 8 {
		return nil, nil, ErrWeakPassword
	}
	old := *user
	statusChanged := in.Status != "" && UserStatus(in.Status) != old.Status
	roleChanged := in.RoleProvided && (old.Role == nil || old.Role.UUID.String() != in.RoleUUID)
	passwordChanged := in.Password != ""

	if in.Name != "" {
		user.Name = in.Name
	}
	if in.Email != "" {
		user.Email = in.Email
	}
	if passwordChanged {
		hashed, err := bcrypt.GenerateFromPassword([]byte(in.Password), 12)
		if err != nil {
			return nil, nil, err
		}
		user.Password = string(hashed)
	}
	if in.Status != "" {
		user.Status = UserStatus(in.Status)
	}
	user.UpdatedBy = in.ActorID

	if err := s.repo.Save(user); err != nil {
		return nil, nil, err
	}
	if in.RoleProvided {
		_ = s.repo.SetRole(user, in.RoleUUID)
	}

	// A password set here is an admin forcing a new credential onto another
	// user's account (unlike self-service ChangePassword) — kick out any
	// existing sessions immediately, same as ResetPassword does.
	if statusChanged || roleChanged || passwordChanged {
		s.invalidateSessions(user.ID)
	}

	return &old, user, nil
}

// Delete removes a user. A superadmin account can be deleted, but only by
// another superadmin, and nobody can delete their own account.
func (s *Service) Delete(uuidStr string, actorID *uint64, actorIsSuperadmin bool) (*User, error) {
	user, err := s.repo.FindByUUID(uuidStr)
	if err != nil {
		return nil, ErrNotFound
	}
	if actorID != nil && *actorID == user.ID {
		return nil, ErrSelfDelete
	}
	if hasSuperadminRole(user) && !actorIsSuperadmin {
		return nil, ErrSuperadminLocked
	}
	if err := s.repo.Delete(user, actorID); err != nil {
		return nil, err
	}
	s.invalidateSessions(user.ID)
	return user, nil
}

func (s *Service) UpdateStatus(uuidStr, status string) (oldStatus UserStatus, user *User, err error) {
	if !validStatus(status) {
		return "", nil, ErrBadStatus
	}
	user, err = s.repo.FindByUUID(uuidStr)
	if err != nil {
		return "", nil, ErrNotFound
	}
	if hasSuperadminRole(user) {
		return "", nil, ErrSuperadminLocked
	}
	oldStatus = user.Status
	user.Status = UserStatus(status)
	if err := s.repo.Save(user); err != nil {
		return "", nil, err
	}
	if oldStatus != user.Status {
		s.invalidateSessions(user.ID)
	}
	return oldStatus, user, nil
}

func (s *Service) UploadAvatar(uuidStr string, file multipart.File, header *multipart.FileHeader, actorIsSuperadmin bool) (string, error) {
	user, err := s.repo.FindByUUID(uuidStr)
	if err != nil {
		return "", ErrNotFound
	}
	if hasSuperadminRole(user) && !actorIsSuperadmin {
		return "", ErrSuperadminLocked
	}

	path, err := s.storage.Upload(file, header, "avatars")
	if err != nil {
		return "", err
	}

	user.Avatar = &path
	if err := s.repo.Save(user); err != nil {
		return "", err
	}
	return s.storage.GetURL(path), nil
}

func hasSuperadminRole(u *User) bool {
	return u.Role != nil && u.Role.IsSuperadmin
}
