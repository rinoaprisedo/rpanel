package users

import (
	"time"

	"rpanel/backend/modules/roles"
	"rpanel/backend/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserStatus string

const (
	StatusActive    UserStatus = "active"
	StatusInactive  UserStatus = "inactive"
	StatusSuspended UserStatus = "suspended"
)

type User struct {
	utils.AuditModel
	Name string `gorm:"type:varchar(255);not null" json:"name"`
	// Uniqueness on email is enforced by a partial index (WHERE deleted_at IS
	// NULL) created in migrations, not by this tag — a plain uniqueIndex would
	// block re-using an email after the row holding it is soft-deleted.
	Email       string     `gorm:"type:varchar(255);index;not null" json:"email"`
	Password    string     `gorm:"type:varchar(255);not null" json:"-"`
	Avatar      *string    `gorm:"type:varchar(255)" json:"avatar"`
	Status      UserStatus `gorm:"type:varchar(20);default:active" json:"status"`
	LastLoginAt *time.Time `json:"last_login_at"`

	// Two-factor authentication (TOTP). The secret is AES-GCM encrypted; recovery
	// codes are stored only as SHA-256 hashes (JSON array).
	TOTPSecretEnc string `gorm:"column:totp_secret_enc;type:text" json:"-"`
	TOTPEnabled   bool   `gorm:"column:totp_enabled;default:false" json:"-"`
	RecoveryCodes string `gorm:"column:recovery_codes;type:text" json:"-"`

	RoleID *uint64     `gorm:"index" json:"-"`
	Role   *roles.Role `gorm:"foreignKey:RoleID" json:"role,omitempty"`
}

func (User) TableName() string { return "users" }

type Session struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"-"`
	UUID      string    `gorm:"type:uuid;uniqueIndex" json:"uuid"`
	UserID    uint64    `json:"-"`
	Token     string    `gorm:"type:varchar(255);index" json:"-"`
	IPAddress string    `gorm:"type:varchar(64)" json:"ip_address"`
	UserAgent string    `gorm:"type:text" json:"user_agent"`
	ExpiredAt time.Time `json:"expired_at"`
	CreatedAt time.Time `json:"created_at"`
}

func (Session) TableName() string { return "sessions" }

func (s *Session) BeforeCreate(tx *gorm.DB) error {
	if s.UUID == "" {
		s.UUID = uuid.New().String()
	}
	return nil
}

type PasswordResetToken struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	Email     string `gorm:"type:varchar(255);index"`
	Token     string `gorm:"type:varchar(255);uniqueIndex"`
	ExpiredAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

func (PasswordResetToken) TableName() string { return "password_reset_tokens" }

type PasswordHistory struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	UserID    uint64 `gorm:"index"`
	Password  string `gorm:"type:varchar(255)"`
	CreatedAt time.Time
}

func (PasswordHistory) TableName() string { return "password_histories" }
