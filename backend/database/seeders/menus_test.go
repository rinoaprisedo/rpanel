package seeders

import (
	"testing"

	"rpanel/backend/modules/menus"
	"rpanel/backend/modules/permissions"
	"rpanel/backend/modules/roles"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&roles.Role{}, &menus.MenuSection{}, &menus.Menu{}, &permissions.Permission{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// An install from before 1.4.0 has one "Dashboard" menu; every role that could open it must keep access to both new pages.
func TestSplitDashboardMenuKeepsPermissions(t *testing.T) {
	db := testDB(t)
	sec := menus.MenuSection{Name: "Dashboard", Icon: "LayoutDashboard", Order: 1}
	db.Create(&sec)
	old := menus.Menu{MenuSectionID: sec.ID, Name: "Dashboard", Icon: "LayoutDashboard", Path: "/", Order: 1, IsActive: true}
	db.Create(&old)
	viewer, editor := roles.Role{Name: "Viewer"}, roles.Role{Name: "Editor"}
	db.Create(&viewer)
	db.Create(&editor)
	db.Create(&permissions.Permission{RoleID: viewer.ID, MenuID: old.ID, CanView: true})
	db.Create(&permissions.Permission{RoleID: editor.ID, MenuID: old.ID, CanView: true, CanEdit: true})

	ids := seedMenus(db)
	if len(ids) == 0 {
		t.Fatal("no menus seeded")
	}
	var server, traffic menus.Menu
	if db.Where("path = ?", "monitor-server").First(&server).Error != nil || server.ID != old.ID || server.Name != "Monitor Server" {
		t.Fatalf("the old menu must be renamed in place: %+v", server)
	}
	if db.Where("path = ?", "monitor-traffic").First(&traffic).Error != nil || traffic.MenuSectionID != sec.ID {
		t.Fatalf("traffic menu missing: %+v", traffic)
	}
	var n int64
	db.Model(&menus.Menu{}).Where("path = ?", "/").Count(&n)
	if n != 0 {
		t.Error("no menu may keep the old '/' path")
	}
	for _, r := range []roles.Role{viewer, editor} {
		var p permissions.Permission
		if db.Where("role_id = ? AND menu_id = ?", r.ID, traffic.ID).First(&p).Error != nil || !p.CanView {
			t.Errorf("%s lost access to Monitor Traffic", r.Name)
		}
	}
	var ep permissions.Permission
	db.Where("role_id = ? AND menu_id = ?", editor.ID, traffic.ID).First(&ep)
	if !ep.CanEdit {
		t.Error("permission flags must be copied")
	}

	// running it again changes nothing
	seedMenus(db)
	var count int64
	db.Model(&menus.Menu{}).Where("path IN ?", []string{"monitor-server", "monitor-traffic"}).Count(&count)
	db.Model(&permissions.Permission{}).Where("menu_id = ?", traffic.ID).Count(&n)
	if count != 2 || n != 2 {
		t.Errorf("seeding must be idempotent: menus=%d traffic perms=%d", count, n)
	}
}

func TestFreshInstallGetsBothMenus(t *testing.T) {
	db := testDB(t)
	seedMenus(db)
	for _, path := range []string{"monitor-server", "monitor-traffic"} {
		var m menus.Menu
		if db.Where("path = ?", path).First(&m).Error != nil || !m.IsActive {
			t.Errorf("%s missing", path)
		}
	}
}
