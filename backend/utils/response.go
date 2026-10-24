package utils

import (
	"regexp"

	"github.com/gin-gonic/gin"
)

// sortColumnRe allows only column identifiers (letters, digits, underscore,
// optional single dot for table-qualified names) to prevent SQL injection via
// the sort_by query param, which callers interpolate directly into ORDER BY.
var sortColumnRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$`)

type Meta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

func Success(c *gin.Context, code int, message string, data interface{}) {
	c.JSON(code, Response{Success: true, Message: message, Data: data})
}

func SuccessList(c *gin.Context, message string, data interface{}, meta *Meta) {
	c.JSON(200, Response{Success: true, Message: message, Data: data, Meta: meta})
}

func Error(c *gin.Context, code int, message string) {
	c.JSON(code, Response{Success: false, Message: message})
}

func BuildMeta(page, limit int, total int64) *Meta {
	totalPages := 0
	if limit > 0 {
		totalPages = int((total + int64(limit) - 1) / int64(limit))
	}
	return &Meta{Page: page, Limit: limit, Total: total, TotalPages: totalPages}
}

// Pagination reads page/limit/search/sort_by/sort_dir query params with sane defaults.
type Pagination struct {
	Page    int
	Limit   int
	Search  string
	SortBy  string
	SortDir string
	Offset  int
}

func GetPagination(c *gin.Context, defaultSort string) Pagination {
	page := QueryInt(c, "page", 1)
	limit := QueryInt(c, "limit", 10)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 10
	}
	sortBy := c.DefaultQuery("sort_by", defaultSort)
	if !sortColumnRe.MatchString(sortBy) {
		sortBy = defaultSort
	}
	sortDir := c.DefaultQuery("sort_dir", "desc")
	if sortDir != "asc" && sortDir != "desc" {
		sortDir = "desc"
	}
	return Pagination{
		Page:    page,
		Limit:   limit,
		Search:  c.Query("search"),
		SortBy:  sortBy,
		SortDir: sortDir,
		Offset:  (page - 1) * limit,
	}
}

func QueryInt(c *gin.Context, key string, fallback int) int {
	v := c.Query(key)
	if v == "" {
		return fallback
	}
	n := 0
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return fallback
		}
		n = n*10 + int(ch-'0')
	}
	return n
}
