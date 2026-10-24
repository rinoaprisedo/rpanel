// Package main wires up the RPanel backend: config, database, redis,
// middleware stack, module routes, and the migrate/seed CLI subcommands.
//
// @title						RPanel API
// @version					1.2.0
// @description				Admin Panel Base Project API
// @host						localhost:8800
// @BasePath					/api/v1
// @securityDefinitions.apikey	SessionCookie
// @in							cookie
// @name						session
//
//go:generate swag init -g main.go -o docs
package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"rpanel/backend/config"
	"rpanel/backend/database/migrations"
	"rpanel/backend/database/seeders"
	_ "rpanel/backend/docs"
	"rpanel/backend/middleware"
	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/modules/appstore"
	"rpanel/backend/modules/auth"
	"rpanel/backend/modules/cronjobs"
	"rpanel/backend/modules/dashboard"
	"rpanel/backend/modules/databases"
	"rpanel/backend/modules/docker"
	"rpanel/backend/modules/filemanager"
	"rpanel/backend/modules/files"
	"rpanel/backend/modules/firewall"
	"rpanel/backend/modules/menus"
	"rpanel/backend/modules/roles"
	"rpanel/backend/modules/settings"
	"rpanel/backend/modules/terminal"
	"rpanel/backend/modules/users"
	"rpanel/backend/modules/website"
	"rpanel/backend/storage"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// AppVersion is surfaced unauthenticated at GET /api/v1/version for the
// frontend footer — bump alongside the @version annotation above.
const AppVersion = "1.2.0"

func main() {
	cfg := config.Load()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "migrate":
			db := config.ConnectDatabase(cfg)
			migrations.Run(db)
			return
		case "seed":
			db := config.ConnectDatabase(cfg)
			seeders.Run(db)
			return
		case "cron-run":
			// Invoked by the system cron daemon: run one job and exit non-zero on failure.
			if len(os.Args) < 3 {
				log.Fatal("usage: cron-run <job-uuid>")
			}
			db := config.ConnectDatabase(cfg)
			run, err := cronjobs.NewService(db, cfg.CronDir, cfg.CronDataDir).RunByUUID(context.Background(), os.Args[2])
			if err != nil {
				log.Fatalf("cron-run: %v", err)
			}
			if run.Status == cronjobs.RunFailed {
				os.Exit(1)
			}
			return
		case "disable-2fa":
			// Emergency reset when an admin lost both authenticator and recovery codes.
			if len(os.Args) < 3 {
				log.Fatal("usage: disable-2fa <email>")
			}
			db := config.ConnectDatabase(cfg)
			var u users.User
			if err := db.Where("email = ?", os.Args[2]).First(&u).Error; err != nil {
				log.Fatalf("user not found: %v", err)
			}
			if err := auth.NewTwoFactor(db, nil, "", "").Reset(&u); err != nil {
				log.Fatalf("reset failed: %v", err)
			}
			log.Printf("two-factor authentication disabled for %s", u.Email)
			return
		case "set-admin":
			db := config.ConnectDatabase(cfg)
			seeders.SetAdmin(db, os.Getenv("RPANEL_ADMIN_EMAIL"), os.Getenv("RPANEL_ADMIN_PASSWORD"))
			return
		}
	}

	db := config.ConnectDatabase(cfg)
	rdb := config.ConnectRedis(cfg)
	activity_logs.StartWorker(db)

	localStorage := storage.NewLocalStorage(cfg.StoragePath, "/uploads", cfg.StorageMaxSize)
	// QC screenshots often show private data: served only via /api/v1/files.
	privateStorage := storage.NewLocalStorage(cfg.PrivateStoragePath, "/api/v1/files", cfg.StorageMaxSize)

	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	// Only a local reverse proxy may set X-Forwarded-For; otherwise the client
	// IP (used by the IP allow-list, rate limits and the lockout guard) is spoofable.
	if err := router.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		log.Fatalf("invalid TRUSTED_PROXIES: %v", err)
	}
	allowed, err := middleware.ParseIPList(cfg.AllowedIPs)
	if err != nil {
		log.Fatalf("invalid PANEL_ALLOWED_IPS: %v", err)
	}
	router.Use(middleware.IPAllowList(allowed))
	router.Use(gin.Recovery())
	router.Use(middleware.CORSMiddleware(cfg.FrontendURL))
	router.Use(middleware.SecurityHeaders())
	defaultLimit := middleware.RequestSizeLimit(10 << 20)
	uploadLimit := middleware.RequestSizeLimit(4 << 30)
	router.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, filemanager.APIPrefix+"/upload") {
			uploadLimit(c)
			return
		}
		defaultLimit(c)
	})
	router.Use(middleware.CSRF(cfg.AppEnv == "production"))
	router.Use(middleware.XSSSanitizer(filemanager.APIPrefix, docker.APIPrefix, cronjobs.APIPrefix, terminal.TemplateAPIPrefix, databases.APIPrefix))
	router.Use(middleware.MaintenanceMode(rdb, db))
	router.Use(middleware.RateLimit(rdb, "api", cfg.RateLimitAPI, time.Minute, func(c *gin.Context) string { return c.ClientIP() }))

	router.Static("/uploads", cfg.StoragePath)

	sessionAuth := middleware.SessionAuth(rdb, db)
	guard := func(menuPath, action string) gin.HandlerFunc {
		return func(c *gin.Context) {
			sessionAuth(c)
			if c.IsAborted() {
				return
			}
			middleware.RequirePermission(db, menuPath, action)(c)
		}
	}
	requireSuperadmin := func(c *gin.Context) {
		if !utils.IsSuperadmin(c) {
			utils.Error(c, 403, "superadmin access required")
			c.Abort()
			return
		}
		c.Next()
	}

	if cfg.AppEnv != "production" {
		router.GET("/swagger/*any", sessionAuth, requireSuperadmin, ginSwagger.WrapHandler(swaggerFiles.Handler))
	}
	uploadLimiter := middleware.RateLimit(rdb, "upload", 10, time.Minute, func(c *gin.Context) string {
		return c.ClientIP()
	})

	api := router.Group("/api/v1")

	api.GET("/version", func(c *gin.Context) {
		utils.Success(c, 200, "ok", gin.H{"version": AppVersion})
	})

	authHandler := auth.NewHandler(db, rdb, cfg)
	authHandler.RegisterRoutes(api, sessionAuth)

	usersHandler := users.NewHandler(db, cfg, localStorage, rdb)
	usersHandler.RegisterRoutes(api, guard, uploadLimiter)

	rolesHandler := roles.NewHandler(db)
	rolesHandler.RegisterRoutes(api, guard)

	menusHandler := menus.NewHandler(db)
	menusHandler.RegisterRoutes(api, sessionAuth, guard)
	menusHandler.RegisterSectionRoutes(api, guard)

	settingsHandler := settings.NewHandler(db, rdb, localStorage)
	settingsHandler.RegisterRoutes(api, guard, uploadLimiter)

	activityLogsHandler := activity_logs.NewHandler(db, cfg)
	activityLogsHandler.RegisterRoutes(api, guard)

	dashboard.Version = AppVersion
	dashboardHandler := dashboard.NewHandler(context.Background(), db, cfg)
	dashboardHandler.RegisterRoutes(api, guard)

	firewallHandler := firewall.NewHandler(cfg)
	firewallHandler.RegisterRoutes(api, guard)

	cronHandler := cronjobs.NewHandler(db, cfg)
	cronHandler.RegisterRoutes(api, guard)
	cronHandler.SyncOnStart()

	dockerHandler := docker.NewHandler(cfg)
	dockerHandler.RegisterRoutes(api, guard)

	databasesHandler := databases.NewHandler(db, cfg)
	databasesHandler.RegisterRoutes(api, guard)

	appStoreHandler := appstore.NewHandler()
	appStoreHandler.RegisterRoutes(api, guard)

	websiteHandler := website.NewHandler(db, cfg)
	websiteHandler.RegisterRoutes(api, guard)

	terminalHandler := terminal.NewHandler(context.Background(), cfg.TerminalShell, cfg.TerminalIdleMinutes, cfg.TerminalMaxSessions, cfg.FrontendURL)
	terminalHandler.RegisterRoutes(api, guard)
	terminal.NewTemplateHandler(terminal.NewTemplateService(db)).RegisterRoutes(api, guard)

	fileManagerHandler := filemanager.NewHandler(cfg.FMRoots)
	fileManagerHandler.RegisterRoutes(api, guard)

	filesHandler := files.NewHandler(privateStorage)
	filesHandler.RegisterRoutes(api, guard)

	log.Printf("RPanel backend listening on :%s (env=%s)", cfg.AppPort, cfg.AppEnv)
	if err := router.Run(cfg.AppHost + ":" + cfg.AppPort); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
