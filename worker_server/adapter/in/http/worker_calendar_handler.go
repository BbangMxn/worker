package http

import (
	"strconv"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/in"

	"github.com/gofiber/fiber/v2"
)

type CalendarHandler struct {
	calendarService in.CalendarService
}

func NewCalendarHandler(calendarService in.CalendarService) *CalendarHandler {
	return &CalendarHandler{calendarService: calendarService}
}

func (h *CalendarHandler) Register(app fiber.Router) {
	cal := app.Group("/calendar")
	cal.Get("/", h.ListCalendars)
	cal.Get("/events", h.ListEvents)
	cal.Get("/events/:id", h.GetEvent)
	cal.Post("/events", h.CreateEvent)
	cal.Put("/events/:id", h.UpdateEvent)
	cal.Delete("/events/:id", h.DeleteEvent)

	// Provider direct access endpoints
	cal.Get("/provider/calendars", h.ListCalendarsFromProvider)
	cal.Get("/provider/events", h.ListEventsFromProvider)
	cal.Post("/sync", h.SyncCalendars)
}

func (h *CalendarHandler) ListCalendars(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	calendars, err := h.calendarService.ListCalendars(c.Context(), userID)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{
		"calendars": calendars,
	})
}

func (h *CalendarHandler) ListEvents(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	filter := &domain.CalendarEventFilter{
		UserID:       userID,
		ConnectionID: GetConnectionID(c),
		Limit:        50,
	}

	if startStr := c.Query("start"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			filter.StartTime = &t
		}
	}

	if endStr := c.Query("end"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			filter.EndTime = &t
		}
	}

	if calID := c.Query("calendar_id"); calID != "" {
		if id, err := strconv.ParseInt(calID, 10, 64); err == nil {
			filter.CalendarID = &id
		}
	}

	events, total, err := h.calendarService.ListEvents(c.Context(), filter)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{
		"events": events,
		"total":  total,
	})
}

func (h *CalendarHandler) GetEvent(c *fiber.Ctx) error {
	eventID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid event id")
	}

	event, err := h.calendarService.GetEvent(c.Context(), eventID)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(event)
}

func (h *CalendarHandler) CreateEvent(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req in.CreateEventRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	// Use connection_id from request or query
	if req.ConnectionID == 0 {
		if connID := GetConnectionID(c); connID != nil {
			req.ConnectionID = *connID
		}
	}

	event, err := h.calendarService.CreateEvent(c.Context(), userID, &req)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.Status(201).JSON(event)
}

func (h *CalendarHandler) UpdateEvent(c *fiber.Ctx) error {
	eventID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid event id")
	}

	var req in.UpdateEventRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	event, err := h.calendarService.UpdateEvent(c.Context(), eventID, &req)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(event)
}

func (h *CalendarHandler) DeleteEvent(c *fiber.Ctx) error {
	eventID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid event id")
	}

	if err := h.calendarService.DeleteEvent(c.Context(), eventID); err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{"status": "deleted"})
}

// =============================================================================
// Provider Direct Access (Google Calendar / Outlook)
// =============================================================================

// ListCalendarsFromProvider fetches calendars directly from Google/Outlook API.
func (h *CalendarHandler) ListCalendarsFromProvider(c *fiber.Ctx) error {
	_, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	connectionID := c.QueryInt("connection_id", 0)
	if connectionID == 0 {
		return ErrorResponse(c, 400, "connection_id required")
	}

	// Type assert to access provider methods
	svc, ok := h.calendarService.(interface {
		ListCalendarsFromProvider(ctx interface{}, connectionID int64) (interface{}, error)
	})
	if !ok {
		return ErrorResponse(c, 500, "provider not configured")
	}

	calendars, err := svc.ListCalendarsFromProvider(c.Context(), int64(connectionID))
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{
		"calendars": calendars,
		"source":    "provider",
	})
}

// ListEventsFromProvider fetches events directly from Google/Outlook API.
func (h *CalendarHandler) ListEventsFromProvider(c *fiber.Ctx) error {
	_, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	connectionID := c.QueryInt("connection_id", 0)
	if connectionID == 0 {
		return ErrorResponse(c, 400, "connection_id required")
	}

	var startTime, endTime *time.Time
	if startStr := c.Query("start"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			startTime = &t
		}
	}
	if endStr := c.Query("end"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			endTime = &t
		}
	}

	// Type assert to access provider methods
	svc, ok := h.calendarService.(interface {
		ListEventsFromProvider(ctx interface{}, connectionID int64, start, end *time.Time) (interface{}, error)
	})
	if !ok {
		return ErrorResponse(c, 500, "provider not configured")
	}

	events, err := svc.ListEventsFromProvider(c.Context(), int64(connectionID), startTime, endTime)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{
		"events": events,
		"source": "provider",
	})
}

// SyncCalendars triggers calendar sync from provider.
func (h *CalendarHandler) SyncCalendars(c *fiber.Ctx) error {
	_, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req struct {
		ConnectionID int64 `json:"connection_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if req.ConnectionID == 0 {
		return ErrorResponse(c, 400, "connection_id required")
	}

	if err := h.calendarService.SyncCalendars(c.Context(), req.ConnectionID); err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{
		"status":  "ok",
		"message": "Calendar sync completed",
	})
}
