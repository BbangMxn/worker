package in

import (
	"context"
	"time"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

type CalendarService interface {
	// Calendar operations
	GetCalendar(ctx context.Context, calendarID int64) (*domain.Calendar, error)
	ListCalendars(ctx context.Context, userID uuid.UUID) ([]*domain.Calendar, error)

	// Event operations
	GetEvent(ctx context.Context, eventID int64) (*domain.CalendarEvent, error)
	ListEvents(ctx context.Context, filter *domain.CalendarEventFilter) ([]*domain.CalendarEvent, int, error)
	CreateEvent(ctx context.Context, userID uuid.UUID, req *CreateEventRequest) (*domain.CalendarEvent, error)
	UpdateEvent(ctx context.Context, eventID int64, req *UpdateEventRequest) (*domain.CalendarEvent, error)
	DeleteEvent(ctx context.Context, eventID int64) error

	// Sync
	SyncCalendars(ctx context.Context, connectionID int64) error
}

type CreateEventRequest struct {
	ConnectionID int64     `json:"connection_id,omitempty"` // For multi-account support
	CalendarID   int64     `json:"calendar_id"`
	Title        string    `json:"title"`
	Description  *string   `json:"description,omitempty"`
	Location     *string   `json:"location,omitempty"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	IsAllDay     bool      `json:"is_all_day"`
	Timezone     string    `json:"timezone"`
	Attendees    []string  `json:"attendees,omitempty"`
	Reminders    []int     `json:"reminders,omitempty"`
}

type UpdateEventRequest struct {
	Title       *string    `json:"title,omitempty"`
	Description *string    `json:"description,omitempty"`
	Location    *string    `json:"location,omitempty"`
	StartTime   *time.Time `json:"start_time,omitempty"`
	EndTime     *time.Time `json:"end_time,omitempty"`
	Attendees   []string   `json:"attendees,omitempty"`
}
