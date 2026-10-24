package terminal

import (
	"errors"
	"strings"

	"rpanel/backend/utils"

	"gorm.io/gorm"
)

const (
	maxName     = 100
	maxDesc     = 255
	maxCommand  = 8 << 10
	maxTemplate = 500
)

var (
	ErrTemplateNotFound = errors.New("template not found")
	ErrTemplateInput    = errors.New("invalid input")
)

// Template is a saved command snippet that can be pasted into the terminal.
// Templates are shared by everyone with access to the terminal menu.
type Template struct {
	utils.AuditModel
	Name        string `gorm:"type:varchar(100);not null" json:"name"`
	Description string `gorm:"type:varchar(255)" json:"description"`
	Command     string `gorm:"type:text;not null" json:"command"`
}

func (Template) TableName() string { return "terminal_templates" }

type TemplateInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Command     string `json:"command"`
}

type TemplateService struct{ db *gorm.DB }

func NewTemplateService(db *gorm.DB) *TemplateService { return &TemplateService{db: db} }

func (in *TemplateInput) clean() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.Command = strings.TrimRight(in.Command, "\r\n ")
	switch {
	case in.Name == "" || len(in.Name) > maxName:
		return ErrTemplateInput
	case len(in.Description) > maxDesc:
		return ErrTemplateInput
	case strings.TrimSpace(in.Command) == "" || len(in.Command) > maxCommand:
		return ErrTemplateInput
	}
	return nil
}

func (s *TemplateService) List() ([]Template, error) {
	var out []Template
	err := s.db.Order("name ASC").Find(&out).Error
	return out, err
}

func (s *TemplateService) get(uuid string) (*Template, error) {
	var t Template
	if err := s.db.Where("uuid = ?", uuid).First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTemplateNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (s *TemplateService) Create(in TemplateInput, actor *uint64) (*Template, error) {
	if err := in.clean(); err != nil {
		return nil, err
	}
	var n int64
	s.db.Model(&Template{}).Count(&n)
	if n >= maxTemplate {
		return nil, errors.New("too many templates")
	}
	t := &Template{Name: in.Name, Description: in.Description, Command: in.Command}
	t.CreatedBy, t.UpdatedBy = actor, actor
	if err := s.db.Create(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TemplateService) Update(uuid string, in TemplateInput, actor *uint64) (*Template, error) {
	if err := in.clean(); err != nil {
		return nil, err
	}
	t, err := s.get(uuid)
	if err != nil {
		return nil, err
	}
	t.Name, t.Description, t.Command, t.UpdatedBy = in.Name, in.Description, in.Command, actor
	if err := s.db.Save(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TemplateService) Delete(uuid string, actor *uint64) (*Template, error) {
	t, err := s.get(uuid)
	if err != nil {
		return nil, err
	}
	t.DeletedBy = actor
	s.db.Model(t).Update("deleted_by", actor)
	if err := s.db.Delete(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}
