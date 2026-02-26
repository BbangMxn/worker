package out

import (
	"context"
	"time"
)

// EmailProvider defines the outbound port for external mail providers (Gmail, Outlook).
type EmailProvider interface {
	// Message operations
	GetMessage(ctx context.Context, messageID string) (*ProviderMessage, error)
	ListMessages(ctx context.Context, query *ProviderListQuery) (*ProviderMessageListResult, error)
	SendMessage(ctx context.Context, msg *ProviderSendRequest) (*ProviderMessage, error)

	// Status operations
	MarkRead(ctx context.Context, messageID string) error
	MarkUnread(ctx context.Context, messageID string) error
	Archive(ctx context.Context, messageID string) error
	Trash(ctx context.Context, messageID string) error
	Delete(ctx context.Context, messageID string) error
	Star(ctx context.Context, messageID string) error
	Unstar(ctx context.Context, messageID string) error

	// Label operations
	AddLabel(ctx context.Context, messageID string, label string) error
	RemoveLabel(ctx context.Context, messageID string, label string) error
	GetLabels(ctx context.Context) ([]*ProviderLabel, error)

	// Subscription operations
	Subscribe(ctx context.Context, webhookURL string) (*ProviderSubscription, error)
	Unsubscribe(ctx context.Context, subscriptionID string) error
	RenewSubscription(ctx context.Context, subscriptionID string) (*ProviderSubscription, error)

	// Provider info
	GetProviderName() string
	GetEmail() string
}

// CalendarProvider defines the outbound port for external calendar providers.
type CalendarProvider interface {
	// Event operations
	GetEvent(ctx context.Context, calendarID, eventID string) (*ProviderCalendarEvent, error)
	ListEvents(ctx context.Context, query *ProviderCalendarQuery) (*ProviderCalendarListResult, error)
	CreateEvent(ctx context.Context, calendarID string, event *ProviderCalendarEvent) (*ProviderCalendarEvent, error)
	UpdateEvent(ctx context.Context, calendarID, eventID string, event *ProviderCalendarEvent) (*ProviderCalendarEvent, error)
	DeleteEvent(ctx context.Context, calendarID, eventID string) error

	// Calendar operations
	ListCalendars(ctx context.Context) ([]*ProviderCalendar, error)

	// Provider info
	GetProviderName() string
}

// ContactProvider defines the outbound port for external contact providers.
type ContactProvider interface {
	// Contact operations
	GetContact(ctx context.Context, contactID string) (*ProviderContact, error)
	ListContacts(ctx context.Context, pageToken string, pageSize int) (*ProviderContactListResult, error)
	CreateContact(ctx context.Context, contact *ProviderContact) (*ProviderContact, error)
	UpdateContact(ctx context.Context, contactID string, contact *ProviderContact) (*ProviderContact, error)
	DeleteContact(ctx context.Context, contactID string) error

	// Provider info
	GetProviderName() string
}

// Provider types

// ProviderMessage represents a message from provider.
type ProviderMessage struct {
	ID           string
	ThreadID     string
	From         string
	To           []string
	Cc           []string
	Bcc          []string
	Subject      string
	Snippet      string
	BodyHTML     string
	BodyText     string
	IsRead       bool
	IsStarred    bool
	Labels       []string
	Attachments  []*ProviderAttachment
	ReceivedAt   time.Time
	InternalDate int64
	InReplyTo    string // Message-ID of the email this is a reply to
	References   string // Message-IDs of the email chain
}

// ProviderAttachment represents an attachment from provider.
type ProviderAttachment struct {
	ID        string
	Name      string
	MimeType  string
	Size      int64
	ContentID string
	IsInline  bool
	Data      []byte
}

// ProviderLabel represents a label from provider.
type ProviderLabel struct {
	ID   string
	Name string
	Type string // system, user
}

// ProviderListQuery represents list query parameters.
type ProviderListQuery struct {
	Query     string
	PageToken string
	PageSize  int
	After     *time.Time
	Before    *time.Time
	StartDate *time.Time // 동기화 시작 날짜 (최근 N일)
	IDsOnly   bool       // true면 메시지 ID만 조회 (페이지 탐색용)
}

// ProviderMessageListResult represents message list result.
type ProviderMessageListResult struct {
	Messages      []*ProviderMessage
	NextPageToken string
	ResultSize    int
}

// ProviderSendRequest represents send request.
type ProviderSendRequest struct {
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	BodyHTML    string
	BodyText    string
	ReplyToID   string
	Attachments []*ProviderAttachment
}

// ProviderSubscription represents push subscription.
type ProviderSubscription struct {
	ID         string
	ResourceID string
	ExpiresAt  time.Time
}

// ProviderCalendarEvent represents a calendar event from provider.
type ProviderCalendarEvent struct {
	ID              string
	CalendarID      string
	Title           string
	Description     string
	Location        string
	StartTime       time.Time
	EndTime         time.Time
	Timezone        string
	IsAllDay        bool
	IsRecurring     bool
	RecurrenceRule  string
	Attendees       []*ProviderAttendee
	OrganizerEmail  string
	Status          string
	Visibility      string
	MeetingURL      string
	MeetingProvider string
	Color           string
	Reminders       []*ProviderReminder

	// SendNotifications controls whether to notify attendees
	// Google Calendar: "all", "externalOnly", "none"
	// Outlook: true/false
	SendNotifications string
}

// ProviderAttendee represents an attendee from provider.
type ProviderAttendee struct {
	Email    string
	Name     string
	Status   string
	Optional bool
}

// ProviderReminder represents a reminder from provider.
type ProviderReminder struct {
	Method  string
	Minutes int
}

// ProviderCalendar represents a calendar from provider.
type ProviderCalendar struct {
	ID          string
	Name        string
	Description string
	Color       string
	IsPrimary   bool
	IsSelected  bool
}

// ProviderCalendarQuery represents calendar query parameters.
type ProviderCalendarQuery struct {
	CalendarID   string
	TimeMin      *time.Time
	TimeMax      *time.Time
	PageToken    string
	PageSize     int
	SingleEvents bool
	OrderBy      string
}

// ProviderCalendarListResult represents calendar list result.
type ProviderCalendarListResult struct {
	Events        []*ProviderCalendarEvent
	NextPageToken string
}

// ProviderContact represents a contact from provider.
type ProviderContact struct {
	ID         string
	Name       string
	Email      string
	Phone      string
	PhotoURL   string
	Company    string
	JobTitle   string
	Department string
	Notes      string
}

// ProviderContactListResult represents contact list result.
type ProviderContactListResult struct {
	Contacts      []*ProviderContact
	NextPageToken string
	TotalCount    int
}
