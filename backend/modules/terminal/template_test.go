package terminal

import (
	"errors"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func tplSvc(t *testing.T) *TemplateService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Template{}); err != nil {
		t.Fatal(err)
	}
	return NewTemplateService(db)
}

func TestTemplateCRUD(t *testing.T) {
	s := tplSvc(t)
	tpl, err := s.Create(TemplateInput{Name: " Disk ", Command: "df -h\n"}, nil)
	if err != nil || tpl.Name != "Disk" || tpl.Command != "df -h" {
		t.Fatalf("create: %+v %v", tpl, err)
	}
	if _, err := s.Update(tpl.UUID.String(), TemplateInput{Name: "Disk usage", Command: "df -h /"}, nil); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List()
	if len(list) != 1 || list[0].Name != "Disk usage" {
		t.Fatalf("list: %+v", list)
	}
	if _, err := s.Delete(tpl.UUID.String(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Delete(tpl.UUID.String(), nil); !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestTemplateValidation(t *testing.T) {
	s := tplSvc(t)
	for _, in := range []TemplateInput{
		{Name: "", Command: "ls"},
		{Name: "x", Command: "  "},
		{Name: strings.Repeat("a", 101), Command: "ls"},
		{Name: "x", Command: strings.Repeat("a", maxCommand+1)},
	} {
		if _, err := s.Create(in, nil); !errors.Is(err, ErrTemplateInput) {
			t.Errorf("%+v: want ErrTemplateInput, got %v", in, err)
		}
	}
}

func TestTemplateRoutesRegister(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := func(string, string) gin.HandlerFunc { return func(c *gin.Context) {} }
	NewHandler(nil, "/bin/sh", 1, 1, "").RegisterRoutes(r.Group("/api"), g)
	NewTemplateHandler(tplSvc(t)).RegisterRoutes(r.Group("/api"), g)
}
