// Package out defines outbound ports (driven ports) for the application.
package out

import (
	"context"
	"time"

	"golang.org/x/oauth2"
)

// =============================================================================
// Calendar Provider Port (Google Calendar, Outlook Calendar)
// =============================================================================

// CalendarProviderPort defines the outbound port for external calendar providers.
type CalendarProviderPort interface {
	// Calendar operations
	ListCalendars(ctx context.Context, token *oauth2.Token) ([]*ProviderCalendar, error)
	GetCalendar(ctx context.Context, token *oauth2.Token, calendarID string) (*ProviderCalendar, error)

	// Event operations
	ListEvents(ctx context.Context, token *oauth2.Token, query *ProviderCalendarQuery) (*ProviderCalendarListResult, error)
	GetEvent(ctx context.Context, token *oauth2.Token, calendarID, eventID string) (*ProviderCalendarEvent, error)
	CreateEvent(ctx context.Context, token *oauth2.Token, calendarID string, event *ProviderCalendarEvent) (*ProviderCalendarEvent, error)
	UpdateEvent(ctx context.Context, token *oauth2.Token, calendarID, eventID string, event *ProviderCalendarEvent) (*ProviderCalendarEvent, error)
	DeleteEvent(ctx context.Context, token *oauth2.Token, calendarID, eventID string) error

	// Sync operations
	InitialSync(ctx context.Context, token *oauth2.Token, opts *CalendarSyncOptions) (*CalendarSyncResult, error)
	IncrementalSync(ctx context.Context, token *oauth2.Token, calendarID, syncToken string) (*CalendarSyncResult, error)

	// Watch (Push Notifications)
	Watch(ctx context.Context, token *oauth2.Token, calendarID string) (*CalendarWatchResponse, error)
	StopWatch(ctx context.Context, token *oauth2.Token, channelID, resourceID string) error

	// Free/Busy
	GetFreeBusy(ctx context.Context, token *oauth2.Token, req *FreeBusyRequest) (*FreeBusyResponse, error)
}

// =============================================================================
// Calendar Sync Types
// =============================================================================

// CalendarSyncOptions represents options for calendar sync.
type CalendarSyncOptions struct {
	CalendarID string
	TimeMin    *time.Time
	TimeMax    *time.Time
	MaxResults int
}

// CalendarSyncResult represents the result of a calendar sync.
type CalendarSyncResult struct {
	Events        []*ProviderCalendarEvent
	DeletedIDs    []string
	NextSyncToken string
	NextPageToken string
	HasMore       bool
}

// CalendarWatchResponse represents the response from setting up a watch.
type CalendarWatchResponse struct {
	ChannelID  string
	ResourceID string
	Expiration time.Time
}

// =============================================================================
// Free/Busy Types
// =============================================================================

// FreeBusyRequest represents a free/busy query request.
type FreeBusyRequest struct {
	CalendarIDs []string
	TimeMin     time.Time
	TimeMax     time.Time
}

// FreeBusyResponse represents a free/busy query response.
type FreeBusyResponse struct {
	Calendars map[string][]*TimePeriod
}

// TimePeriod represents a time period.
type TimePeriod struct {
	Start time.Time
	End   time.Time
}

// =============================================================================
// Calendar Sync State
// =============================================================================

// CalendarSyncState represents the sync state for a calendar.
type CalendarSyncState struct {
	ID           int64
	UserID       string
	ConnectionID int64
	CalendarID   string
	Provider     string
	SyncToken    string
	HistoryID    uint64
	WatchID      string
	WatchExpiry  time.Time
	Status       string
	LastSyncAt   time.Time
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// =============================================================================
// Calendar Sync Repository
// =============================================================================

// CalendarSyncRepository defines the repository for calendar sync state.
type CalendarSyncRepository interface {
	GetByConnectionID(ctx context.Context, connectionID int64) (*CalendarSyncState, error)
	GetByCalendarID(ctx context.Context, connectionID int64, calendarID string) (*CalendarSyncState, error)
	GetByWatchID(ctx context.Context, watchID string) (*CalendarSyncState, error)
	Create(ctx context.Context, state *CalendarSyncState) error
	Update(ctx context.Context, state *CalendarSyncState) error
	UpdateSyncToken(ctx context.Context, connectionID int64, calendarID, syncToken string) error
	UpdateWatchExpiry(ctx context.Context, connectionID int64, calendarID string, expiry time.Time, watchID string) error
	UpdateStatus(ctx context.Context, connectionID int64, calendarID, status, errorMsg string) error
	GetExpiredWatches(ctx context.Context, before time.Time) ([]*CalendarSyncState, error)
	Delete(ctx context.Context, connectionID int64, calendarID string) error
}

// =============================================================================
// Calendar Provider Factory
// =============================================================================

// CalendarProviderFactory creates calendar providers.
type CalendarProviderFactory interface {
	CreateProvider(ctx context.Context, providerType string, token *oauth2.Token) (CalendarProviderPort, error)
	CreateProviderFromConnection(ctx context.Context, connectionID int64) (CalendarProviderPort, error)
}
