package roles

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestOnlySuperadminManagesSuperadminRoles(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Role{}); err != nil {
		t.Fatal(err)
	}
	db.Exec("CREATE TABLE users (id integer primary key, role_id integer, deleted_at datetime)")
	s := NewService(NewRepository(db))

	if _, err := s.Create(RoleInput{Name: "evil", IsSuperadmin: true}); !errors.Is(err, ErrSuperadminLocked) {
		t.Fatalf("create superadmin role: %v", err)
	}
	r, err := s.Create(RoleInput{Name: "staff"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Update(r.UUID.String(), RoleInput{Name: "staff", IsSuperadmin: true}); !errors.Is(err, ErrSuperadminLocked) {
		t.Fatalf("promote role: %v", err)
	}
	if _, err := s.Create(RoleInput{Name: "root2", IsSuperadmin: true, ActorIsSuperadmin: true}); err != nil {
		t.Fatalf("superadmin may create one: %v", err)
	}

	uid := uint64(9)
	db.Exec("INSERT INTO users (id, role_id) VALUES (?, ?)", uid, r.ID)
	if err := s.UpdatePermissions(r.UUID.String(), nil, &uid, false); !errors.Is(err, ErrOwnRole) {
		t.Fatalf("own role: %v", err)
	}
	other := uint64(10)
	if err := s.UpdatePermissions(r.UUID.String(), nil, &other, false); err != nil {
		t.Fatalf("other role manager: %v", err)
	}
}
