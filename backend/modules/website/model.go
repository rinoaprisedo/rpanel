package website

import "rpanel/backend/utils"

const (
	TypeStatic = "static"
	TypePHP    = "php"
	TypeProxy  = "proxy"

	StatusRunning = "running"
	StatusStopped = "stopped"
)

type Website struct {
	utils.AuditModel
	Domain      string `gorm:"type:varchar(253);uniqueIndex;not null" json:"domain"`
	Type        string `gorm:"type:varchar(10);not null" json:"type"`
	RootDir     string `gorm:"type:varchar(500)" json:"root_dir"`
	PHPVersion  string `gorm:"type:varchar(5)" json:"php_version"`
	ProxyTarget string `gorm:"type:varchar(500)" json:"proxy_target"`
	SSLEnabled  bool   `gorm:"default:false" json:"ssl_enabled"`
	ForceHTTPS  bool   `gorm:"default:false" json:"force_https"`
	Status      string `gorm:"type:varchar(10);not null;default:running" json:"status"`
}

func (Website) TableName() string { return "websites" }
