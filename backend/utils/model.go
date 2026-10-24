package utils

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AuditModel is embedded in tables that require id/uuid + audit trail + soft delete.
type AuditModel struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement" json:"-"`
	UUID      uuid.UUID      `gorm:"type:uuid;uniqueIndex;not null" json:"uuid"`
	CreatedBy *uint64        `json:"-"`
	UpdatedBy *uint64        `json:"-"`
	DeletedBy *uint64        `json:"-"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (m *AuditModel) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}
