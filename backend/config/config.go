package config

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv      string
	AppPort     string
	AppHost     string
	FrontendURL string

	DBDriver string
	DBHost   string
	DBPort   string
	DBUser   string
	DBPass   string
	DBName   string

	RedisHost string
	RedisPort string
	RedisPass string
	RedisDB   int

	SessionMaxAge        int
	SessionMaxConcurrent int

	StoragePath string
	// PrivateStoragePath is derived (sibling of StoragePath, never served by
	// /uploads): files here are only reachable through the permission-checked
	// /api/v1/files endpoint.
	PrivateStoragePath string
	StorageMaxSize     int64

	// FMRoots are the directories non-superadmin users may browse in the file
	// manager (comma separated in FM_ROOTS). Superadmin is unrestricted.
	FMRoots []string

	// Website manager (Nginx).
	NginxAvailableDir string
	NginxEnabledDir   string
	NginxLogDir       string
	WebRoot           string
	AcmeRoot          string
	CertbotEmail      string

	// Panel SSL: nginx include owned by the panel (server_name + certificate), the panel's public port.
	PanelTLSConf   string
	PanelGuardConf string
	PanelSSLDir    string
	PanelPort      string

	// GitHub repository RPanel updates itself from (owner/name).
	ReleaseRepo string

	// Database manager.
	DBSecretKey string // 32 bytes, hex or any passphrase (hashed): encrypts stored DB admin passwords
	BackupDir   string

	// Docker manager: directory holding one sub-directory per compose project.
	ComposeDir string

	// Cron manager.
	CronDir     string // where /etc/cron.d-style files are written
	CronDataDir string // panel-owned dir for job scripts

	// Firewall manager: extra ports (comma separated) that must never be locked out, in addition to SSH and the panel port.
	FirewallProtectedPorts []string
	SSHDConfig             string

	// Network hardening.
	TrustedProxies []string // proxies whose X-Forwarded-For is believed (default: loopback, i.e. the local nginx)
	AllowedIPs     []string // if set, only these IPs/CIDRs may reach the panel at all

	// Web terminal.
	TerminalShell       string
	TerminalIdleMinutes int
	TerminalMaxSessions int

	LogCleanupDays           int
	RateLimitLogin           int
	RateLimitLoginLockoutMin int
	RateLimitAPI             int
}

var Cfg *Config

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, relying on environment variables")
	}

	Cfg = &Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		AppPort:     getEnv("APP_PORT", "8800"),
		AppHost:     getEnv("APP_HOST", ""),
		FrontendURL: getEnv("FRONTEND_URL", "http://localhost:8801,http://localhost:8802"),

		DBDriver: getEnv("DB_DRIVER", "postgres"),
		DBHost:   getEnv("DB_HOST", "localhost"),
		DBPort:   getEnv("DB_PORT", "5401"),
		DBUser:   getEnv("DB_USER", "postgres"),
		DBPass:   getEnv("DB_PASS", "password"),
		DBName:   getEnv("DB_NAME", "baseadmin"),

		RedisHost: getEnv("REDIS_HOST", "localhost"),
		RedisPort: getEnv("REDIS_PORT", "6301"),
		RedisPass: getEnv("REDIS_PASS", ""),
		RedisDB:   getEnvInt("REDIS_DB", 0),

		SessionMaxAge:        getEnvInt("SESSION_MAX_AGE", 86400),
		SessionMaxConcurrent: getEnvInt("SESSION_MAX_CONCURRENT", 3),

		StoragePath:    getEnv("STORAGE_PATH", "./storage/uploads"),
		FMRoots:        splitList(getEnv("FM_ROOTS", "/www/wwwroot")),
		StorageMaxSize: int64(getEnvInt("STORAGE_MAX_SIZE", 2097152)),

		NginxAvailableDir: getEnv("NGINX_SITES_AVAILABLE", "/etc/nginx/sites-available"),
		NginxEnabledDir:   getEnv("NGINX_SITES_ENABLED", "/etc/nginx/sites-enabled"),
		NginxLogDir:       getEnv("NGINX_LOG_DIR", "/var/log/nginx"),
		WebRoot:           getEnv("WEB_ROOT", "/www/wwwroot"),
		AcmeRoot:          getEnv("ACME_ROOT", "/var/www/letsencrypt"),
		CertbotEmail:      getEnv("CERTBOT_EMAIL", ""),

		PanelTLSConf:   getEnv("PANEL_TLS_CONF", ""),
		PanelGuardConf: getEnv("PANEL_GUARD_CONF", ""),
		PanelSSLDir:    getEnv("PANEL_SSL_DIR", ""),
		PanelPort:      getEnv("PANEL_PORT", ""),
		ReleaseRepo:    getEnv("RPANEL_REPO", "rinoaprisedo/rpanel"),

		DBSecretKey: getEnv("SECRET_KEY", getEnv("DB_SECRET_KEY", "")),
		BackupDir:   getEnv("BACKUP_DIR", "/www/backup/database"),
		ComposeDir:  getEnv("COMPOSE_DIR", "/www/docker"),
		CronDir:     getEnv("CRON_DIR", "/etc/cron.d"),
		CronDataDir: getEnv("CRON_DATA_DIR", "/www/cron"),

		FirewallProtectedPorts: splitList(getEnv("FIREWALL_PROTECTED_PORTS", "")),
		SSHDConfig:             getEnv("SSHD_CONFIG", "/etc/ssh/sshd_config"),
		TrustedProxies:         splitList(getEnv("TRUSTED_PROXIES", "127.0.0.1,::1")),
		AllowedIPs:             splitList(getEnv("PANEL_ALLOWED_IPS", "")),

		TerminalShell:            getEnv("TERMINAL_SHELL", "/bin/bash"),
		TerminalIdleMinutes:      getEnvInt("TERMINAL_IDLE_MINUTES", 15),
		TerminalMaxSessions:      getEnvInt("TERMINAL_MAX_SESSIONS", 3),
		LogCleanupDays:           getEnvInt("LOG_CLEANUP_DAYS", 90),
		RateLimitLogin:           getEnvInt("RATE_LIMIT_LOGIN", 5),
		RateLimitLoginLockoutMin: getEnvInt("RATE_LIMIT_LOGIN_LOCKOUT_MINUTES", 15),
		RateLimitAPI:             getEnvInt("RATE_LIMIT_API", 100),
	}

	Cfg.PrivateStoragePath = filepath.Join(filepath.Dir(filepath.Clean(Cfg.StoragePath)), "private")

	return Cfg
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
