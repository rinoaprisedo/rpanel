package integrations

import (
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	ctxKeyNote   = "api_key_note"
	keepPerKey   = 500 // newest requests kept per key
	pruneEveryN  = 50
	maxNoteBytes = 300
)

// KeyLog is one request made with an API key.
type KeyLog struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	APIKeyID  uint64    `gorm:"index:idx_key_logs_key,priority:1;not null" json:"-"`
	Method    string    `gorm:"type:varchar(10)" json:"method"`
	Route     string    `gorm:"type:varchar(200)" json:"route"`
	Note      string    `gorm:"type:varchar(300)" json:"note"`
	Status    int       `json:"status"`
	IPAddress string    `gorm:"type:varchar(64)" json:"ip_address"`
	UserAgent string    `gorm:"type:varchar(300)" json:"user_agent"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

func (KeyLog) TableName() string { return "api_key_logs" }

var logCount atomic.Uint64

// record stores a finished request. Upload chunks and their status polls are only kept when they failed or the handler
// attached a note (the final chunk), so a large upload is one row rather than hundreds.
func (s *Service) record(c *gin.Context, k *APIKey) {
	route := c.FullPath()
	status := c.Writer.Status()
	note, _ := c.Get(ctxKeyNote)
	n, _ := note.(string)
	if (route == APIPrefix+"/deploy/upload/chunk" || route == APIPrefix+"/deploy/upload/status") && n == "" && status < 400 {
		return
	}
	if len(n) > maxNoteBytes {
		n = n[:maxNoteBytes]
	}
	ua := c.Request.UserAgent()
	if len(ua) > 300 {
		ua = ua[:300]
	}
	s.db.Create(&KeyLog{APIKeyID: k.ID, Method: c.Request.Method, Route: route, Note: n, Status: status, IPAddress: c.ClientIP(), UserAgent: ua, CreatedAt: time.Now()})
	if logCount.Add(1)%pruneEveryN == 0 {
		s.db.Exec("DELETE FROM api_key_logs WHERE api_key_id = ? AND id <= (SELECT id FROM api_key_logs WHERE api_key_id = ? ORDER BY id DESC OFFSET ? LIMIT 1)", k.ID, k.ID, keepPerKey)
	}
}

// Logs returns a key's newest requests, optionally those before the given id.
func (s *Service) Logs(uuid string, before uint64, limit int) ([]KeyLog, bool, error) {
	var k APIKey
	if err := s.db.Unscoped().Where("uuid = ?", uuid).First(&k).Error; err != nil {
		return nil, false, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := s.db.Where("api_key_id = ?", k.ID).Order("id DESC").Limit(limit + 1)
	if before > 0 {
		q = q.Where("id < ?", before)
	}
	var out []KeyLog
	if err := q.Find(&out).Error; err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

func parseUint(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}
