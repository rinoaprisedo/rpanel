package settings

import (
	"context"
	"errors"
	"mime/multipart"

	"rpanel/backend/storage"

	"github.com/redis/go-redis/v9"
)

var ErrInvalidUploadTarget = errors.New("target must be app_logo or app_favicon")

// Service holds business rules for the settings module: key/value upsert and
// invalidating the maintenance-mode cache when it changes.
type Service struct {
	repo    *Repository
	redis   *redis.Client
	storage storage.StorageInterface
}

func NewService(repo *Repository, rdb *redis.Client, s storage.StorageInterface) *Service {
	return &Service{repo: repo, redis: rdb, storage: s}
}

func (s *Service) List() (map[string]string, error) {
	list, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(list))
	for _, item := range list {
		result[item.Key] = item.Value
	}
	return result, nil
}

// publicKeys are readable without a session — pure branding, never anything
// sensitive (mail credentials, API keys, etc live under other keys in the
// same table and must stay behind the authenticated List()).
var publicKeys = map[string]bool{
	"app_name":      true,
	"app_logo":      true,
	"app_favicon":   true,
	"primary_color": true,
}

// Public returns only the whitelisted branding settings, for the login page
// (and anywhere else pre-auth) to render the app name/logo/color correctly
// without needing a session.
func (s *Service) Public() (map[string]string, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(publicKeys))
	for key := range publicKeys {
		if v, ok := all[key]; ok {
			result[key] = v
		}
	}
	return result, nil
}

func (s *Service) BulkUpdate(payload map[string]string) error {
	for key, value := range payload {
		if err := s.repo.Upsert(key, value, TypeString); err != nil {
			return err
		}
	}
	if _, ok := payload["maintenance_mode"]; ok {
		s.redis.Del(context.Background(), "settings:maintenance_mode")
	}
	return nil
}

var uploadTargets = map[string]bool{"app_logo": true, "app_favicon": true}

func (s *Service) Upload(target string, file multipart.File, header *multipart.FileHeader) (string, error) {
	if !uploadTargets[target] {
		return "", ErrInvalidUploadTarget
	}

	path, err := s.storage.Upload(file, header, "settings")
	if err != nil {
		return "", err
	}
	if err := s.repo.Upsert(target, path, TypeFile); err != nil {
		return "", err
	}
	return s.storage.GetURL(path), nil
}
