package settings

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SettingType string

const (
	TypeString  SettingType = "string"
	TypeBoolean SettingType = "boolean"
	TypeFile    SettingType = "file"
)

type Setting struct {
	ID        uint64      `gorm:"primaryKey;autoIncrement" json:"-"`
	UUID      uuid.UUID   `gorm:"type:uuid;uniqueIndex;not null" json:"uuid"`
	Key       string      `gorm:"type:varchar(100);uniqueIndex;not null" json:"key"`
	Value     string      `gorm:"type:text" json:"value"`
	Type      SettingType `gorm:"type:varchar(20);default:string" json:"type"`
	UpdatedAt time.Time   `json:"updated_at"`
}

func (Setting) TableName() string { return "settings" }

func (s *Setting) BeforeCreate(tx *gorm.DB) error {
	if s.UUID == uuid.Nil {
		s.UUID = uuid.New()
	}
	return nil
}
