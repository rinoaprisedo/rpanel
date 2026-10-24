package seeders

import (
	"log"

	"rpanel/backend/config"
	"rpanel/backend/modules/menus"
	"rpanel/backend/modules/permissions"
	"rpanel/backend/modules/roles"
	"rpanel/backend/modules/settings"
	"rpanel/backend/modules/users"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Run seeds the superadmin role/user, default menu structure, permissions,
// and default settings. Safe to re-run (idempotent via FirstOrCreate).
func Run(db *gorm.DB) {
	superadminRole := seedSuperadminRole(db)
	seedSuperadminUser(db, superadminRole)
	if config.Cfg.AppEnv != "production" {
		seedUser(db, superadminRole, "Rino Aprisedo", "rinoaprisedo@gmail.com", "password!!")
	}
	menuIDs := seedMenus(db)
	seedPermissions(db, superadminRole.ID, menuIDs)
	seedSettings(db)
	log.Println("seed completed successfully")
}

func seedSuperadminRole(db *gorm.DB) roles.Role {
	var role roles.Role
	db.Where("name = ?", "Superadmin").Attrs(roles.Role{
		Name:         "Superadmin",
		Description:  "Full system access. Cannot be deleted or demoted.",
		IsSuperadmin: true,
	}).FirstOrCreate(&role)
	return role
}

func seedSuperadminUser(db *gorm.DB, role roles.Role) {
	var user users.User
	result := db.Where("email = ?", "admin@example.com").First(&user)
	if result.Error == nil {
		return
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte("Admin@12345"), 12)
	if err != nil {
		log.Fatalf("failed to hash superadmin password: %v", err)
	}
	user = users.User{
		Name:     "Super Admin",
		Email:    "admin@example.com",
		Password: string(hashed),
		Status:   users.StatusActive,
		RoleID:   &role.ID,
	}
	if err := db.Create(&user).Error; err != nil {
		log.Fatalf("failed to seed superadmin user: %v", err)
	}
}

func seedUser(db *gorm.DB, role roles.Role, name, email, password string) {
	var user users.User
	result := db.Where("email = ?", email).First(&user)
	if result.Error == nil {
		return
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		log.Fatalf("failed to hash user password: %v", err)
	}
	user = users.User{
		Name:     name,
		Email:    email,
		Password: string(hashed),
		Status:   users.StatusActive,
		RoleID:   &role.ID,
	}
	if err := db.Create(&user).Error; err != nil {
		log.Fatalf("failed to seed user: %v", err)
	}
}

type menuDef struct {
	Name string
	Icon string
	Path string
}

func seedMenus(db *gorm.DB) []uint64 {
	removeMergedMenus(db)
	sections := []struct {
		Section menus.MenuSection
		Menus   []menuDef
	}{
		{
			Section: menus.MenuSection{Name: "Dashboard", Icon: "LayoutDashboard", Order: 1},
			Menus: []menuDef{
				{Name: "Dashboard", Icon: "LayoutDashboard", Path: "/"},
			},
		},
		{
			Section: menus.MenuSection{Name: "Server", Icon: "Server", Order: 2},
			Menus: []menuDef{
				{Name: "Websites", Icon: "Globe", Path: "websites"},
				{Name: "File Manager", Icon: "FolderOpen", Path: "file-manager"},
				{Name: "Terminal", Icon: "Terminal", Path: "terminal"},
				{Name: "Databases", Icon: "Database", Path: "databases"},
				{Name: "Docker", Icon: "Container", Path: "docker"},
				{Name: "Cron", Icon: "Clock", Path: "cron"},
				{Name: "Firewall", Icon: "ShieldCheck", Path: "firewall"},
				{Name: "App Store", Icon: "Store", Path: "app-store"},
			},
		},
		{
			Section: menus.MenuSection{Name: "User Management", Icon: "Users", Order: 3},
			Menus: []menuDef{
				{Name: "Users", Icon: "User", Path: "users"},
				{Name: "Roles", Icon: "Shield", Path: "roles"},
			},
		},
		{
			Section: menus.MenuSection{Name: "System", Icon: "Settings", Order: 4},
			Menus: []menuDef{
				{Name: "Menus", Icon: "Menu", Path: "menus"},
				{Name: "Settings", Icon: "Settings", Path: "settings"},
				{Name: "Activity Logs", Icon: "History", Path: "activity-logs"},
			},
		},
	}

	var menuIDs []uint64
	for _, s := range sections {
		var section menus.MenuSection
		db.Where("name = ?", s.Section.Name).Attrs(s.Section).FirstOrCreate(&section)

		for i, m := range s.Menus {
			var menu menus.Menu
			db.Where("path = ?", m.Path).Attrs(menus.Menu{
				MenuSectionID: section.ID,
				Name:          m.Name,
				Icon:          m.Icon,
				Path:          m.Path,
				Order:         i + 1,
				IsActive:      true,
			}).FirstOrCreate(&menu)
			menuIDs = append(menuIDs, menu.ID)
		}
	}
	return menuIDs
}

func seedPermissions(db *gorm.DB, roleID uint64, menuIDs []uint64) {
	for _, menuID := range menuIDs {
		var perm permissions.Permission
		db.Where("role_id = ? AND menu_id = ?", roleID, menuID).Attrs(permissions.Permission{
			RoleID:    roleID,
			MenuID:    menuID,
			CanView:   true,
			CanCreate: true,
			CanEdit:   true,
			CanDelete: true,
		}).FirstOrCreate(&perm)
	}
}

func seedSettings(db *gorm.DB) {
	defaults := map[string]settings.SettingType{
		"app_name":         settings.TypeString,
		"app_logo":         settings.TypeFile,
		"app_favicon":      settings.TypeFile,
		"primary_color":    settings.TypeString,
		"maintenance_mode": settings.TypeBoolean,
	}
	values := map[string]string{
		"app_name":         "BaseProject",
		"app_logo":         "",
		"app_favicon":      "",
		"primary_color":    "#35494d",
		"maintenance_mode": "false",
	}
	for key, t := range defaults {
		var s settings.Setting
		db.Where("key = ?", key).Attrs(settings.Setting{Key: key, Value: values[key], Type: t}).FirstOrCreate(&s)
	}
}

// removeMergedMenus drops the standalone "menu-sections" menu (its screen is
// now a tab of Menus) together with the permission rows that pointed at it.
func removeMergedMenus(db *gorm.DB) {
	var old menus.Menu
	if db.Where("path = ?", "menu-sections").First(&old).Error != nil {
		return
	}
	db.Unscoped().Where("menu_id = ?", old.ID).Delete(&permissions.Permission{})
	db.Delete(&old)
}
