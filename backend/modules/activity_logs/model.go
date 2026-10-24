package activity_logs

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ActivityAction string

const (
	ActionCreate ActivityAction = "create"
	ActionUpdate ActivityAction = "update"
	ActionDelete ActivityAction = "delete"
	ActionLogin  ActivityAction = "login"
	ActionLogout ActivityAction = "logout"
	ActionView   ActivityAction = "view"
)

type ActivityLog struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement" json:"-"`
	UUID      uuid.UUID      `gorm:"type:uuid;uniqueIndex;not null" json:"uuid"`
	UserID    *uint64        `json:"-"`
	Action    ActivityAction `gorm:"type:varchar(20);not null" json:"action"`
	Module    string         `gorm:"type:varchar(100)" json:"module"`
	RecordID  *string        `gorm:"type:varchar(100)" json:"record_id"`
	OldValue  *string        `gorm:"type:jsonb" json:"old_value"`
	NewValue  *string        `gorm:"type:jsonb" json:"new_value"`
	IPAddress string         `gorm:"type:varchar(64)" json:"ip_address"`
	UserAgent string         `gorm:"type:text" json:"user_agent"`
	CreatedAt time.Time      `json:"created_at"`
}

func (ActivityLog) TableName() string { return "activity_logs" }

func (a *ActivityLog) BeforeCreate(tx *gorm.DB) error {
	if a.UUID == uuid.Nil {
		a.UUID = uuid.New()
	}
	return nil
}
