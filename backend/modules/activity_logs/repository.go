package activity_logs

import (
	"rpanel/backend/utils"

	"gorm.io/gorm"
)

// Repository isolates all GORM/SQL access for querying activity logs. The
// write path (LogActivity/worker in logger.go) is intentionally kept as a
// package-level async pipeline since it's called from every other module —
// threading a Repository instance through all of them for a fire-and-forget
// log write would add DI overhead without a testability payoff.
type Repository struct {
	DB *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{DB: db}
}

type ListFilter struct {
	User      string
	Module    string
	Action    string
	IPAddress string
	DateFrom  string
	DateTo    string
}

func (r *Repository) List(p utils.Pagination, f ListFilter) ([]ActivityLog, int64, error) {
	q := r.DB.Model(&ActivityLog{})

	if f.User != "" {
		like := "%" + f.User + "%"
		q = q.Joins("JOIN users ON users.id = activity_logs.user_id").Where("users.name LIKE ? OR users.email LIKE ?", like, like)
	}
	if f.Module != "" {
		q = q.Where("module = ?", f.Module)
	}
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	if f.IPAddress != "" {
		q = q.Where("ip_address LIKE ?", "%"+f.IPAddress+"%")
	}
	if f.DateFrom != "" {
		q = q.Where("created_at >= ?", f.DateFrom)
	}
	if f.DateTo != "" {
		q = q.Where("created_at <= ?", f.DateTo)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var list []ActivityLog
	err := q.Order(p.SortBy + " " + p.SortDir).Offset(p.Offset).Limit(p.Limit).Find(&list).Error
	return list, total, err
}

func (r *Repository) FindByUUID(uuidStr string) (*ActivityLog, error) {
	var entry ActivityLog
	if err := r.DB.Where("uuid = ?", uuidStr).First(&entry).Error; err != nil {
		return nil, err
	}
	return &entry, nil
}

// UsersByID looks up {uuid, name} for a set of user IDs. Done as a plain
// table query (not a real GORM association) because the users module
// imports activity_logs for LogActivity — importing users.User back here
// would create an import cycle.
func (r *Repository) UsersByID(ids []uint64) map[uint64]UserRef {
	result := make(map[uint64]UserRef, len(ids))
	if len(ids) == 0 {
		return result
	}
	var rows []struct {
		ID   uint64
		UUID string
		Name string
	}
	r.DB.Table("users").Select("id, uuid, name").Where("id IN ?", ids).Scan(&rows)
	for _, row := range rows {
		result[row.ID] = UserRef{UUID: row.UUID, Name: row.Name}
	}
	return result
}
