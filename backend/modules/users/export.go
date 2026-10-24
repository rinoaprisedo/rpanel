package users

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"

	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

var exportColumns = []string{"Name", "Email", "Status", "Role", "Last Login", "Created At"}

func exportRow(u User) []string {
	role := ""
	if u.Role != nil {
		role = u.Role.Name
	}
	lastLogin := ""
	if u.LastLoginAt != nil {
		lastLogin = u.LastLoginAt.Format("2006-01-02 15:04:05")
	}
	return []string{u.Name, u.Email, string(u.Status), role, lastLogin, u.CreatedAt.Format("2006-01-02 15:04:05")}
}

// ExportCSV godoc
// @Summary		Export users as CSV
// @Tags			users
// @Security		SessionCookie
// @Param			search	query	string	false	"Search by name/email"
// @Param			status	query	string	false	"Filter by status"
// @Param			role	query	string	false	"Filter by role UUID"
// @Produce		text/csv
// @Success		200
// @Router			/users/export/csv [get]
func (h *Handler) ExportCSV(c *gin.Context) {
	list, err := h.Service.ListAll(c.Query("search"), c.Query("status"), c.Query("role"))
	if err != nil {
		utils.Error(c, 500, "failed to export users")
		return
	}

	filename := fmt.Sprintf("users_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "text/csv")

	w := csv.NewWriter(c.Writer)
	_ = w.Write(exportColumns)
	for _, u := range list {
		_ = w.Write(exportRow(u))
	}
	w.Flush()
}

// ExportExcel godoc
// @Summary		Export users as Excel
// @Tags			users
// @Security		SessionCookie
// @Param			search	query	string	false	"Search by name/email"
// @Param			status	query	string	false	"Filter by status"
// @Param			role	query	string	false	"Filter by role UUID"
// @Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Success		200
// @Router			/users/export/excel [get]
func (h *Handler) ExportExcel(c *gin.Context) {
	list, err := h.Service.ListAll(c.Query("search"), c.Query("status"), c.Query("role"))
	if err != nil {
		utils.Error(c, 500, "failed to export users")
		return
	}

	f := excelize.NewFile()
	defer f.Close()
	const sheet = "Users"
	f.SetSheetName("Sheet1", sheet)

	for i, col := range exportColumns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, col)
	}
	for r, u := range list {
		row := exportRow(u)
		for i, v := range row {
			cell, _ := excelize.CoordinatesToCellName(i+1, r+2)
			f.SetCellValue(sheet, cell, v)
		}
	}

	filename := fmt.Sprintf("users_%s.xlsx", time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if err := f.Write(c.Writer); err != nil {
		c.Status(http.StatusInternalServerError)
	}
}
