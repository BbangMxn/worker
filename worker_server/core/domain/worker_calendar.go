package domain

import (
	"time"

	"github.com/google/uuid"
)

type CalendarProvider string

const (
	CalendarProviderGoogle    CalendarProvider = "google"
	CalendarProviderMicrosoft CalendarProvider = "microsoft"
)

type Calendar struct {
	ID           int64            `json:"id"`
	UserID       uuid.UUID        `json:"user_id"`
	ConnectionID int64            `json:"connection_id"`
	Provider     CalendarProvider `json:"provider"`
	ProviderID   string           `json:"provider_id"`
	Name         string           `json:"name"`
	Description  *string          `json:"description,omitempty"`
	Color        *string          `json:"color,omitempty"`
	IsDefault    bool             `json:"is_default"`
	IsReadOnly   bool             `json:"is_read_only"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

type EventStatus string

const (
	EventStatusConfirmed EventStatus = "confirmed"
	EventStatusTentative EventStatus = "tentative"
	EventStatusCancelled EventStatus = "cancelled"
)

type CalendarEvent struct {
	ID         int64     `json:"id"`
	CalendarID int64     `json:"calendar_id"`
	UserID     uuid.UUID `json:"user_id"`
	ProviderID string    `json:"provider_id"`

	Title       string  `json:"title"`
	Description *string `json:"description,omitempty"`
	Location    *string `json:"location,omitempty"`

	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	IsAllDay  bool      `json:"is_all_day"`
	Timezone  string    `json:"timezone"`

	Status    EventStatus `json:"status"`
	Organizer *string     `json:"organizer,omitempty"`
	Attendees []string    `json:"attendees,omitempty"`

	// Recurrence
	IsRecurring    bool    `json:"is_recurring"`
	RecurrenceRule *string `json:"recurrence_rule,omitempty"`

	// Reminders
	Reminders []int `json:"reminders,omitempty"` // minutes before

	// Metadata
	MeetingURL *string `json:"meeting_url,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CalendarEventFilter struct {
	UserID       uuid.UUID
	ConnectionID *int64 // For multi-account support
	CalendarID   *int64
	StartTime    *time.Time
	EndTime      *time.Time
	Status       *EventStatus
	Search       *string
	Attendee     *string
	Location     *string
	Limit        int
	Offset       int
}

type CalendarRepository interface {
	// Calendar
	GetCalendarByID(id int64) (*Calendar, error)
	GetCalendarsByUser(userID uuid.UUID) ([]*Calendar, error)
	CreateCalendar(cal *Calendar) error
	UpdateCalendar(cal *Calendar) error
	DeleteCalendar(id int64) error

	// Events
	GetEventByID(id int64) (*CalendarEvent, error)
	ListEvents(filter *CalendarEventFilter) ([]*CalendarEvent, int, error)
	CreateEvent(event *CalendarEvent) error
	UpdateEvent(event *CalendarEvent) error
	DeleteEvent(id int64) error
}
