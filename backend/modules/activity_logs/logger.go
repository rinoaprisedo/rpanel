package activity_logs

import (
	"encoding/json"
	"log"
	"time"

	"gorm.io/gorm"
)

type logEntry struct {
	UserID    *uint64
	Action    ActivityAction
	Module    string
	RecordID  string
	OldValue  interface{}
	NewValue  interface{}
	IPAddress string
	UserAgent string
}

var logChan chan logEntry
var db *gorm.DB

// StartWorker launches the background goroutine that persists activity logs
// from the buffered channel so request handlers never block on logging I/O.
func StartWorker(database *gorm.DB) {
	db = database
	logChan = make(chan logEntry, 1000)
	go worker()
}

func worker() {
	for e := range logChan {
		record := ActivityLog{
			UserID:    e.UserID,
			Action:    e.Action,
			Module:    e.Module,
			IPAddress: e.IPAddress,
			UserAgent: e.UserAgent,
		}
		if e.RecordID != "" {
			record.RecordID = &e.RecordID
		}
		if e.OldValue != nil {
			if b, err := json.Marshal(e.OldValue); err == nil {
				s := string(b)
				record.OldValue = &s
			}
		}
		if e.NewValue != nil {
			if b, err := json.Marshal(e.NewValue); err == nil {
				s := string(b)
				record.NewValue = &s
			}
		}
		if err := db.Create(&record).Error; err != nil {
			log.Printf("activity log write failed: %v", err)
		}
	}
}

// LogActivity queues an activity log entry for async persistence. Safe to
// call directly (it never blocks unless the buffer of 1000 is full).
func LogActivity(userID *uint64, action ActivityAction, module, recordID string, oldValue, newValue interface{}, ip, userAgent string) {
	if logChan == nil {
		return
	}
	select {
	case logChan <- logEntry{
		UserID: userID, Action: action, Module: module, RecordID: recordID,
		OldValue: oldValue, NewValue: newValue, IPAddress: ip, UserAgent: userAgent,
	}:
	default:
		log.Println("activity log channel full, dropping entry")
	}
}

// CleanupOld deletes activity logs older than the configured retention window.
func CleanupOld(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	res := db.Unscoped().Where("created_at < ?", cutoff).Delete(&ActivityLog{})
	return res.RowsAffected, res.Error
}
