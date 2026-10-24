package activity_logs

import (
	"errors"

	"rpanel/backend/utils"
)

var ErrNotFound = errors.New("activity log not found")

// UserRef is the actor identity attached to a log entry for display
// ("Jane Doe" instead of a bare user_id, falling back to "System" when nil).
type UserRef struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// LogWithUser wraps ActivityLog with the resolved actor, since the raw model
// only carries a hidden numeric user_id.
type LogWithUser struct {
	ActivityLog
	User *UserRef `json:"user,omitempty"`
}

// Service holds business rules for querying/retaining activity logs.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(p utils.Pagination, f ListFilter) ([]LogWithUser, int64, error) {
	list, total, err := s.repo.List(p, f)
	if err != nil {
		return nil, 0, err
	}
	return s.attachUsers(list), total, nil
}

func (s *Service) Get(uuidStr string) (*LogWithUser, error) {
	entry, err := s.repo.FindByUUID(uuidStr)
	if err != nil {
		return nil, ErrNotFound
	}
	wrapped := s.attachUsers([]ActivityLog{*entry})
	return &wrapped[0], nil
}

func (s *Service) attachUsers(list []ActivityLog) []LogWithUser {
	ids := make([]uint64, 0, len(list))
	seen := make(map[uint64]bool)
	for _, entry := range list {
		if entry.UserID != nil && !seen[*entry.UserID] {
			ids = append(ids, *entry.UserID)
			seen[*entry.UserID] = true
		}
	}
	users := s.repo.UsersByID(ids)

	result := make([]LogWithUser, len(list))
	for i, entry := range list {
		wrapped := LogWithUser{ActivityLog: entry}
		if entry.UserID != nil {
			if ref, ok := users[*entry.UserID]; ok {
				wrapped.User = &ref
			}
		}
		result[i] = wrapped
	}
	return result
}

func (s *Service) Cleanup(retentionDays int) (int64, error) {
	return CleanupOld(retentionDays)
}
