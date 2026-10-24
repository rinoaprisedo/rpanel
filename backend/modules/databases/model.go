package databases

import "rpanel/backend/utils"

const (
	EngineMySQL    = "mysql" // MySQL and MariaDB
	EnginePostgres = "postgres"
)

// Server is a database server the panel administers (local or remote).
type Server struct {
	utils.AuditModel
	Name       string `gorm:"type:varchar(64);uniqueIndex;not null" json:"name"`
	Engine     string `gorm:"type:varchar(10);not null" json:"engine"`
	Host       string `gorm:"type:varchar(253);not null" json:"host"`
	Port       int    `gorm:"not null" json:"port"`
	AdminUser  string `gorm:"type:varchar(64);not null" json:"admin_user"`
	AdminPassE string `gorm:"column:admin_pass_enc;type:text;not null" json:"-"` // AES-GCM, never serialized
	Local      bool   `gorm:"default:false" json:"local"`
}

func (Server) TableName() string { return "database_servers" }
