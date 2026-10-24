package website

import "rpanel/backend/utils"

const (
	TypeStatic = "static"
	TypePHP    = "php"
	TypeProxy  = "proxy"

	StatusRunning = "running"
	StatusStopped = "stopped"
)

// ProxyRule forwards one URL prefix of a site to another service.
type ProxyRule struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Target   string `json:"target"`
	PassHost bool   `json:"pass_host"` // true: send the visitor's Host header; false: the target's own
	Enabled  bool   `json:"enabled"`
}

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
	// ProxyRules overlay reverse-proxy locations on any site type.
	ProxyRules []ProxyRule `gorm:"type:text;serializer:json" json:"proxy_rules"`
	// ExtraConfig is raw nginx directives added to the site's server blocks (superadmin only).
	ExtraConfig string `gorm:"type:text" json:"-"`
}

func (Website) TableName() string { return "websites" }
