package integrations

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rpanel/backend/middleware"

	"gorm.io/gorm"
)

var (
	ErrInvalidKey = errors.New("invalid API key")
	ErrBadInput   = errors.New("invalid input")
)

const keyPrefix = "rpk_"

type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

func hashKey(k string) string {
	h := sha256.Sum256([]byte(k))
	return hex.EncodeToString(h[:])
}

func validScopes(in []string) (string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		ok := false
		for _, a := range AllScopes {
			if s == a {
				ok = true
			}
		}
		if !ok {
			return "", ErrBadInput
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return "", ErrBadInput
	}
	return strings.Join(out, ","), nil
}

type CreateInput struct {
	Name        string   `json:"name" binding:"required,max=100"`
	Scopes      []string `json:"scopes" binding:"required"`
	AllowedIPs  []string `json:"allowed_ips"`
	AllowedDirs []string `json:"allowed_dirs"`
	ExpiresDay  int      `json:"expires_days"` // 0 = never
}

// Create returns the stored key and its plain text, which cannot be recovered afterwards.
func (s *Service) Create(owner uint64, in CreateInput) (*APIKey, string, error) {
	scopes, err := validScopes(in.Scopes)
	if err != nil {
		return nil, "", err
	}
	ips := []string{}
	for _, e := range in.AllowedIPs {
		if e = strings.TrimSpace(e); e != "" {
			ips = append(ips, e)
		}
	}
	dirs, derr := cleanDirs(in.AllowedDirs)
	if _, err := middleware.ParseIPList(ips); err != nil || derr != nil || in.ExpiresDay < 0 || in.ExpiresDay > 3650 {
		if derr != nil {
			return nil, "", derr
		}
		return nil, "", ErrBadInput
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", err
	}
	plain := keyPrefix + hex.EncodeToString(raw)
	k := &APIKey{
		Name: strings.TrimSpace(in.Name), Prefix: plain[:12], KeyHash: hashKey(plain), Scopes: scopes,
		AllowedIPs: strings.Join(ips, ","), AllowedDirs: dirs, OwnerID: owner, CreatedBy: &owner,
	}
	if in.ExpiresDay > 0 {
		t := time.Now().Add(time.Duration(in.ExpiresDay) * 24 * time.Hour)
		k.ExpiresAt = &t
	}
	if err := s.db.Create(k).Error; err != nil {
		return nil, "", err
	}
	return k, plain, nil
}

type KeyView struct {
	APIKey
	Scopes      []string `json:"scopes"`
	AllowedDirs []string `json:"allowed_dirs"`
}

func view(k APIKey) KeyView {
	return KeyView{APIKey: k, Scopes: strings.Split(k.Scopes, ","), AllowedDirs: splitDirs(k.AllowedDirs)}
}

func (s *Service) List() ([]KeyView, error) {
	var ks []APIKey
	if err := s.db.Order("created_at DESC").Find(&ks).Error; err != nil {
		return nil, err
	}
	out := make([]KeyView, 0, len(ks))
	for _, k := range ks {
		out = append(out, view(k))
	}
	return out, nil
}

type UpdateInput struct {
	Name        string   `json:"name" binding:"required,max=100"`
	Scopes      []string `json:"scopes" binding:"required"`
	AllowedIPs  []string `json:"allowed_ips"`
	AllowedDirs []string `json:"allowed_dirs"`
	ExpiresAt   string   `json:"expires_at"` // YYYY-MM-DD (end of that day); empty = never
}

// Update changes a key's name, scopes, IP list and expiry. The key itself never changes, and a revoked key stays revoked.
func (s *Service) Update(uuid string, in UpdateInput) (*APIKey, error) {
	scopes, err := validScopes(in.Scopes)
	if err != nil {
		return nil, err
	}
	ips := []string{}
	for _, e := range in.AllowedIPs {
		if e = strings.TrimSpace(e); e != "" {
			ips = append(ips, e)
		}
	}
	dirs, derr := cleanDirs(in.AllowedDirs)
	if derr != nil {
		return nil, derr
	}
	if _, err := middleware.ParseIPList(ips); err != nil || strings.TrimSpace(in.Name) == "" {
		return nil, ErrBadInput
	}
	var exp *time.Time
	if in.ExpiresAt != "" {
		d, err := time.ParseInLocation("2006-01-02", in.ExpiresAt, time.Local)
		if err != nil {
			return nil, ErrBadInput
		}
		d = d.Add(24*time.Hour - time.Second)
		exp = &d
	}
	var k APIKey
	if err := s.db.Where("uuid = ?", uuid).First(&k).Error; err != nil {
		return nil, err
	}
	if k.RevokedAt != nil {
		return nil, ErrBadInput
	}
	k.Name, k.Scopes, k.AllowedIPs, k.AllowedDirs, k.ExpiresAt = strings.TrimSpace(in.Name), scopes, strings.Join(ips, ","), dirs, exp
	if err := s.db.Model(&APIKey{}).Where("id = ?", k.ID).Updates(map[string]any{"name": k.Name, "scopes": k.Scopes, "allowed_ips": k.AllowedIPs, "allowed_dirs": k.AllowedDirs, "expires_at": exp}).Error; err != nil {
		return nil, err
	}
	return &k, nil
}

func (s *Service) Revoke(uuid string) error {
	now := time.Now()
	res := s.db.Model(&APIKey{}).Where("uuid = ? AND revoked_at IS NULL", uuid).Update("revoked_at", &now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) Delete(uuid string) error {
	s.db.Exec("DELETE FROM api_key_logs WHERE api_key_id IN (SELECT id FROM api_keys WHERE uuid = ?)", uuid)
	res := s.db.Where("uuid = ?", uuid).Delete(&APIKey{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Authenticate checks a presented key: known, not revoked or expired, client IP allowed, owner still active, scope granted.
func (s *Service) Authenticate(token, ip, scope string) (*APIKey, error) {
	if !strings.HasPrefix(token, keyPrefix) || len(token) > 100 {
		return nil, ErrInvalidKey
	}
	var k APIKey
	if err := s.db.Where("key_hash = ?", hashKey(token)).First(&k).Error; err != nil {
		return nil, ErrInvalidKey
	}
	now := time.Now()
	if k.RevokedAt != nil || (k.ExpiresAt != nil && now.After(*k.ExpiresAt)) {
		return nil, ErrInvalidKey
	}
	if k.AllowedIPs != "" {
		nets, err := middleware.ParseIPList(strings.Split(k.AllowedIPs, ","))
		parsed := net.ParseIP(ip)
		allowed := false
		if err == nil && parsed != nil {
			for _, n := range nets {
				if n.Contains(parsed) {
					allowed = true
				}
			}
		}
		if !allowed {
			return nil, ErrInvalidKey
		}
	}
	var active int64
	s.db.Table("users").Where("id = ? AND status = ? AND deleted_at IS NULL", k.OwnerID, "active").Count(&active)
	if active == 0 {
		return nil, ErrInvalidKey
	}
	granted := false
	for _, sc := range strings.Split(k.Scopes, ",") {
		if sc == scope {
			granted = true
		}
	}
	if !granted {
		return nil, ErrScope
	}
	s.db.Model(&APIKey{}).Where("id = ?", k.ID).Updates(map[string]any{"last_used_at": now, "last_used_ip": ip})
	return &k, nil
}

var ErrScope = errors.New("API key lacks the required scope")

// ErrBadDir is returned for a folder a key may not be given; the message is shown to the admin.
type ErrBadDir struct{ Msg string }

func (e ErrBadDir) Error() string { return e.Msg }

const maxDirs = 10

// System folders a key can never be pointed at (nor anything inside them).
var protectedDirs = []string{"/etc", "/root", "/proc", "/sys", "/dev", "/boot", "/bin", "/sbin", "/usr", "/lib", "/lib32", "/lib64", "/run", "/var/lib", "/opt/rpanel"}

func splitDirs(s string) []string {
	out := []string{}
	for _, d := range strings.Split(s, "\n") {
		if d = strings.TrimSpace(d); d != "" {
			out = append(out, d)
		}
	}
	return out
}

// cleanDirs checks the folders chosen for a key and returns them joined for storage. Each has to be an existing absolute
// directory that is not "/" or a system folder (symlinks are resolved first, so a link cannot smuggle one in).
func cleanDirs(in []string) (string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, d := range in {
		if d = strings.TrimSpace(d); d == "" {
			continue
		}
		if !filepath.IsAbs(d) || strings.ContainsAny(d, "\x00\n\r") {
			return "", ErrBadDir{"folders must be absolute paths: " + d}
		}
		clean := filepath.Clean(d)
		real, err := filepath.EvalSymlinks(clean)
		if err != nil {
			return "", ErrBadDir{"folder does not exist: " + clean}
		}
		if st, err := os.Stat(real); err != nil || !st.IsDir() {
			return "", ErrBadDir{"not a folder: " + clean}
		}
		if real == "/" {
			return "", ErrBadDir{"the whole server cannot be given to a key"}
		}
		for _, p := range protectedDirs {
			// Compare against the folder itself and where it really points (/etc is /private/etc on macOS).
			for _, pp := range []string{p, realOrSelf(p)} {
				if real == pp || strings.HasPrefix(real, pp+"/") || clean == pp || strings.HasPrefix(clean, pp+"/") {
					return "", ErrBadDir{clean + " is a system folder and cannot be given to a key"}
				}
			}
		}
		if !seen[clean] {
			seen[clean] = true
			out = append(out, clean)
		}
	}
	if len(out) > maxDirs {
		return "", ErrBadDir{"at most 10 folders per key"}
	}
	return strings.Join(out, "\n"), nil
}

func realOrSelf(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
