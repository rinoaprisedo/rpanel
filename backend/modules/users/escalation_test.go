package users

import (
	"errors"
	"testing"

	"rpanel/backend/modules/roles"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// A user manager without superadmin rights must not be able to create, become
// or take over a superadmin.
func escalationEnv(t *testing.T) (*Service, roles.Role, roles.Role, *User) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&roles.Role{}, &User{}); err != nil {
		t.Fatal(err)
	}
	super := roles.Role{Name: "super", IsSuperadmin: true}
	staff := roles.Role{Name: "staff"}
	db.Create(&super)
	db.Create(&staff)
	admin := &User{Name: "root", Email: "root@x.io", Password: "x", Status: StatusActive, RoleID: &super.ID}
	db.Create(admin)
	return NewService(NewRepository(db), nil, nil), super, staff, admin
}

func TestNonSuperadminCannotCreateSuperadmin(t *testing.T) {
	s, super, staff, _ := escalationEnv(t)
	if _, err := s.Create(CreateInput{Name: "a", Email: "a@x.io", Password: "Passw0rd!", RoleUUID: super.UUID.String()}); !errors.Is(err, ErrSuperadminRole) {
		t.Fatalf("want ErrSuperadminRole, got %v", err)
	}
	if _, err := s.Create(CreateInput{Name: "b", Email: "b@x.io", Password: "Passw0rd!", RoleUUID: staff.UUID.String()}); err != nil {
		t.Fatalf("staff role must be allowed: %v", err)
	}
	if _, err := s.Create(CreateInput{Name: "c", Email: "c@x.io", Password: "short"}); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("short password: %v", err)
	}
	if _, err := s.Create(CreateInput{Name: "d", Email: "d@x.io", Password: "Passw0rd!", Status: "god"}); !errors.Is(err, ErrBadStatus) {
		t.Fatalf("bad status: %v", err)
	}
}

func TestNonSuperadminCannotTakeOverOrPromote(t *testing.T) {
	s, super, staff, admin := escalationEnv(t)
	if _, _, err := s.Update(admin.UUID.String(), UpdateInput{Password: "Hacked123!"}); !errors.Is(err, ErrSuperadminLocked) {
		t.Fatalf("editing a superadmin: %v", err)
	}
	u, err := s.Create(CreateInput{Name: "m", Email: "m@x.io", Password: "Passw0rd!", RoleUUID: staff.UUID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Update(u.UUID.String(), UpdateInput{RoleProvided: true, RoleUUID: super.UUID.String()}); !errors.Is(err, ErrSuperadminRole) {
		t.Fatalf("promoting to superadmin: %v", err)
	}
	if _, err := s.UploadAvatar(admin.UUID.String(), nil, nil, false); !errors.Is(err, ErrSuperadminLocked) {
		t.Fatalf("superadmin avatar: %v", err)
	}
}
