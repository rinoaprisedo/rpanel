package cronjobs

import (
	"time"

	"rpanel/backend/utils"
)

const (
	TypeShell = "shell"
	TypeURL   = "url"

	RunRunning = "running"
	RunSuccess = "success"
	RunFailed  = "failed"
	RunSkipped = "skipped"
)

type Job struct {
	utils.AuditModel
	Name       string     `gorm:"type:varchar(100);not null" json:"name"`
	Type       string     `gorm:"type:varchar(10);not null" json:"type"`
	Schedule   string     `gorm:"type:varchar(100);not null" json:"schedule"`
	Command    string     `gorm:"type:text;not null" json:"command"`    // script text or URL
	Timeout    int        `gorm:"not null;default:3600" json:"timeout"` // seconds
	Enabled    bool       `gorm:"default:true" json:"enabled"`
	LastRunAt  *time.Time `json:"last_run_at"`
	LastStatus string     `gorm:"type:varchar(10)" json:"last_status"`
}

func (Job) TableName() string { return "cron_jobs" }

// Run is one execution (scheduled or manual) of a job.
type Run struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	JobID      uint64     `gorm:"index;not null" json:"-"`
	Trigger    string     `gorm:"type:varchar(10)" json:"trigger"` // schedule | manual
	Status     string     `gorm:"type:varchar(10);not null" json:"status"`
	ExitCode   int        `json:"exit_code"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Output     string     `gorm:"type:text" json:"output"`
}

func (Run) TableName() string { return "cron_runs" }
