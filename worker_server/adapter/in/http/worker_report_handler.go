package http

import (
	"strconv"
	"time"

	"worker_server/core/service/report"

	"github.com/gofiber/fiber/v2"
)

// ReportHandler handles report requests.
type ReportHandler struct {
	reportService *report.Service
}

// NewReportHandler creates a new report handler.
func NewReportHandler(reportService *report.Service) *ReportHandler {
	return &ReportHandler{
		reportService: reportService,
	}
}

// Register registers report routes.
func (h *ReportHandler) Register(router fiber.Router) {
	reports := router.Group("/reports")

	reports.Get("/", h.ListReports)
	reports.Get("/latest/:type", h.GetLatestReport)
	reports.Get("/:id", h.GetReport)
	reports.Post("/generate", h.GenerateReport)

	// Convenience endpoints
	reports.Post("/daily", h.GenerateDailyReport)
	reports.Post("/weekly", h.GenerateWeeklyReport)
	reports.Post("/monthly", h.GenerateMonthlyReport)
}

// =============================================================================
// Handlers
// =============================================================================

// ListReports returns a list of reports.
func (h *ReportHandler) ListReports(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.reportService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Report service not available")
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	reports, total, err := h.reportService.ListReports(c.Context(), userID, limit, offset)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{
		"reports": reports,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

// GetReport returns a single report.
func (h *ReportHandler) GetReport(c *fiber.Ctx) error {
	if h.reportService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Report service not available")
	}

	reportID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid report ID")
	}

	report, err := h.reportService.GetReport(c.Context(), reportID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Report not found")
	}

	return c.JSON(report)
}

// GetLatestReport returns the latest report of a specific type.
func (h *ReportHandler) GetLatestReport(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.reportService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Report service not available")
	}

	reportType := c.Params("type")
	if reportType == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Report type is required")
	}

	report, err := h.reportService.GetLatestReport(c.Context(), userID, reportType)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Report not found")
	}

	return c.JSON(report)
}

// GenerateReportRequest represents report generation request.
type GenerateReportRequest struct {
	Type      string `json:"type"`                 // daily, weekly, monthly, custom
	StartDate string `json:"start_date,omitempty"` // YYYY-MM-DD
	EndDate   string `json:"end_date,omitempty"`   // YYYY-MM-DD
}

// GenerateReport generates a new report.
func (h *ReportHandler) GenerateReport(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.reportService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Report service not available")
	}

	var req GenerateReportRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Type == "" {
		req.Type = "daily"
	}

	var startDate, endDate time.Time
	now := time.Now()

	switch req.Type {
	case "daily":
		startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		endDate = startDate.Add(24 * time.Hour)
	case "weekly":
		// Start from Monday of current week
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		startDate = time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, now.Location())
		endDate = startDate.Add(7 * 24 * time.Hour)
	case "monthly":
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		endDate = startDate.AddDate(0, 1, 0)
	case "custom":
		if req.StartDate == "" || req.EndDate == "" {
			return fiber.NewError(fiber.StatusBadRequest, "start_date and end_date are required for custom reports")
		}
		startDate, err = time.Parse("2006-01-02", req.StartDate)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "Invalid start_date format (use YYYY-MM-DD)")
		}
		endDate, err = time.Parse("2006-01-02", req.EndDate)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "Invalid end_date format (use YYYY-MM-DD)")
		}
		endDate = endDate.Add(24 * time.Hour) // Include the end date
	default:
		return fiber.NewError(fiber.StatusBadRequest, "Invalid report type")
	}

	report, err := h.reportService.GenerateReport(c.Context(), userID, req.Type, startDate, endDate)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(report)
}

// GenerateDailyReport generates a daily report.
func (h *ReportHandler) GenerateDailyReport(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.reportService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Report service not available")
	}

	now := time.Now()
	startDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endDate := startDate.Add(24 * time.Hour)

	report, err := h.reportService.GenerateReport(c.Context(), userID, "daily", startDate, endDate)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(report)
}

// GenerateWeeklyReport generates a weekly report.
func (h *ReportHandler) GenerateWeeklyReport(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.reportService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Report service not available")
	}

	now := time.Now()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	startDate := time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, now.Location())
	endDate := startDate.Add(7 * 24 * time.Hour)

	report, err := h.reportService.GenerateReport(c.Context(), userID, "weekly", startDate, endDate)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(report)
}

// GenerateMonthlyReport generates a monthly report.
func (h *ReportHandler) GenerateMonthlyReport(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.reportService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Report service not available")
	}

	now := time.Now()
	startDate := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	endDate := startDate.AddDate(0, 1, 0)

	report, err := h.reportService.GenerateReport(c.Context(), userID, "monthly", startDate, endDate)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(report)
}
