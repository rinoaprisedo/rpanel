package integrations

import (
	"time"

	"rpanel/backend/utils"
)

// Scopes an API key can carry. A key never gets more than its scopes, whatever its owner may do in the panel.
const (
	ScopeUpload = "deploy:upload" // upload files into the deploy folders
	ScopeDocker = "deploy:docker" // docker load, compose files, compose up/down
	ScopeSSH    = "deploy:ssh"    // register a public key for SSH login
)

var AllScopes = []string{ScopeUpload, ScopeDocker, ScopeSSH}

// APIKey authenticates an external tool (Vibecoder). Only the SHA-256 of the key is stored; the key itself is shown once.
type APIKey struct {
	utils.AuditModel
	Name        string     `gorm:"type:varchar(100);not null" json:"name"`
	Prefix      string     `gorm:"type:varchar(16);not null" json:"prefix"`
	KeyHash     string     `gorm:"type:varchar(64);uniqueIndex;not null" json:"-"`
	Scopes      string     `gorm:"type:varchar(200);not null" json:"-"` // comma separated
	AllowedIPs  string     `gorm:"type:varchar(500)" json:"allowed_ips"`
	AllowedDirs string     `gorm:"type:text" json:"-"` // extra folders this key may use, one per line, on top of the deploy folders
	OwnerID     uint64     `gorm:"index;not null" json:"-"`
	ExpiresAt   *time.Time `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	LastUsedIP  string     `gorm:"type:varchar(64)" json:"last_used_ip"`
	RevokedAt   *time.Time `json:"revoked_at"`
}

func (APIKey) TableName() string { return "api_keys" }
