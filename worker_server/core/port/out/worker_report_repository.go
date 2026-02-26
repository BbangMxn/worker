// Package out defines outbound ports (driven ports) for the application.
package out

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// ReportRepository (MongoDB - 리포트 저장)
// =============================================================================

// ReportType defines the type of report.
type ReportType string

const (
	ReportTypeDaily   ReportType = "daily"
	ReportTypeWeekly  ReportType = "weekly"
	ReportTypeMonthly ReportType = "monthly"
)

// ReportStatus defines the status of report.
type ReportStatus string

const (
	ReportStatusPending    ReportStatus = "pending"
	ReportStatusGenerating ReportStatus = "generating"
	ReportStatusCompleted  ReportStatus = "completed"
	ReportStatusFailed     ReportStatus = "failed"
)

// ReportRepository defines the outbound port for report storage.
type ReportRepository interface {
	// Single operations
	Save(ctx context.Context, report *ReportEntity) error
	GetByID(ctx context.Context, id string) (*ReportEntity, error)
	Delete(ctx context.Context, id string) error

	// Query operations
	GetByUserAndPeriod(ctx context.Context, userID uuid.UUID, reportType ReportType, periodStart time.Time) (*ReportEntity, error)
	ListByUser(ctx context.Context, userID uuid.UUID, opts *ReportListOptions) ([]*ReportEntity, error)
	GetLatestByUser(ctx context.Context, userID uuid.UUID, reportType ReportType) (*ReportEntity, error)

	// Cleanup operations
	DeleteExpired(ctx context.Context) (int64, error)
	DeleteByUser(ctx context.Context, userID uuid.UUID) (int64, error)
	DeleteOlderThan(ctx context.Context, before time.Time) (int64, error)

	// Stats
	GetStorageStats(ctx context.Context) (*ReportStorageStats, error)
}

// ReportListOptions defines options for listing reports.
type ReportListOptions struct {
	Type       *ReportType
	Status     *ReportStatus
	PeriodFrom *time.Time
	PeriodTo   *time.Time
	Limit      int
	Offset     int
}

// ReportEntity represents the report domain entity.
type ReportEntity struct {
	ID     string    `json:"id"`
	UserID uuid.UUID `json:"user_id"`

	// Report type and period
	Type        ReportType `json:"type"`
	PeriodStart time.Time  `json:"period_start"`
	PeriodEnd   time.Time  `json:"period_end"`

	// Report content (JSON)
	Content *ReportContent `json:"content"`

	// Status
	Status       ReportStatus `json:"status"`
	ErrorMessage string       `json:"error_message,omitempty"`

	// Compression info
	IsCompressed   bool  `json:"is_compressed"`
	OriginalSize   int64 `json:"original_size"`
	CompressedSize int64 `json:"compressed_size"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt time.Time `json:"expires_at"` // TTL 30일
}

// ReportContent represents the content of a report.
type ReportContent struct {
	// Summary
	Summary string `json:"summary"`

	// Email statistics
	EmailStats *EmailStats `json:"email_stats,omitempty"`

	// Important emails list
	ImportantEmails []ImportantEmail `json:"important_emails,omitempty"`

	// Emails that need reply
	ReplyNeeded []ReplyNeededEmail `json:"reply_needed,omitempty"`

	// Schedule suggestions extracted from emails
	ScheduleSuggestions []ScheduleSuggestion `json:"schedule_suggestions,omitempty"`

	// Action items extracted from emails
	ActionItems []ActionItem `json:"action_items,omitempty"`

	// Label/folder statistics
	LabelStats []LabelStat `json:"label_stats,omitempty"`

	// Calendar events in the period
	CalendarEvents []CalendarEvent `json:"calendar_events,omitempty"`

	// Top contacts
	TopContacts []TopContact `json:"top_contacts,omitempty"`
}

// EmailStats represents email statistics.
type EmailStats struct {
	TotalReceived    int `json:"total_received"`
	TotalSent        int `json:"total_sent"`
	UnreadCount      int `json:"unread_count"`
	ImportantCount   int `json:"important_count"`
	SpamCount        int `json:"spam_count"`
	AverageReplyTime int `json:"average_reply_time_minutes,omitempty"` // in minutes
}

// ImportantEmail represents an important email in the report.
type ImportantEmail struct {
	EmailID    int64     `json:"email_id"`
	Subject    string    `json:"subject"`
	From       string    `json:"from"`
	Summary    string    `json:"summary"`
	Priority   string    `json:"priority"`
	ReceivedAt time.Time `json:"received_at"`
}

// ReplyNeededEmail represents an email that needs a reply.
type ReplyNeededEmail struct {
	EmailID        int64     `json:"email_id"`
	Subject        string    `json:"subject"`
	From           string    `json:"from"`
	Reason         string    `json:"reason"`
	Urgency        string    `json:"urgency"` // high, medium, low
	ReceivedAt     time.Time `json:"received_at"`
	SuggestedReply string    `json:"suggested_reply,omitempty"`
}

// ScheduleSuggestion represents a schedule suggestion from email.
type ScheduleSuggestion struct {
	EmailID     int64     `json:"email_id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time,omitempty"`
	Location    string    `json:"location,omitempty"`
	Attendees   []string  `json:"attendees,omitempty"`
	Confidence  float64   `json:"confidence"` // 0.0 - 1.0
}

// ActionItem represents an action item extracted from email.
type ActionItem struct {
	EmailID     int64      `json:"email_id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	Priority    string     `json:"priority"` // high, medium, low
	Status      string     `json:"status"`   // pending, completed
}

// LabelStat represents label statistics.
type LabelStat struct {
	LabelID     int64  `json:"label_id"`
	LabelName   string `json:"label_name"`
	Count       int    `json:"count"`
	UnreadCount int    `json:"unread_count"`
}

// CalendarEvent represents a calendar event in the report.
type CalendarEvent struct {
	EventID   string    `json:"event_id"`
	Title     string    `json:"title"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Location  string    `json:"location,omitempty"`
	IsAllDay  bool      `json:"is_all_day"`
	Status    string    `json:"status"` // confirmed, tentative, cancelled
}

// TopContact represents a top contact.
type TopContact struct {
	Email         string `json:"email"`
	Name          string `json:"name,omitempty"`
	SentCount     int    `json:"sent_count"`
	ReceivedCount int    `json:"received_count"`
}

// ReportStorageStats represents report storage statistics.
type ReportStorageStats struct {
	TotalCount     int64                `json:"total_count"`
	TotalSize      int64                `json:"total_size"`
	CompressedSize int64                `json:"compressed_size"`
	AvgCompression float64              `json:"avg_compression"`
	ExpiredCount   int64                `json:"expired_count"`
	ByType         map[ReportType]int64 `json:"by_type"`
	OldestEntry    *time.Time           `json:"oldest_entry,omitempty"`
	NewestEntry    *time.Time           `json:"newest_entry,omitempty"`
}

// =============================================================================
// Helper Functions
// =============================================================================

// DefaultReportTTLDays is the default TTL for reports.
const DefaultReportTTLDays = 90

// NewReportEntity creates a new report entity with default TTL.
func NewReportEntity(userID uuid.UUID, reportType ReportType, periodStart, periodEnd time.Time) *ReportEntity {
	now := time.Now()
	return &ReportEntity{
		ID:          uuid.New().String(),
		UserID:      userID,
		Type:        reportType,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Status:      ReportStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
		ExpiresAt:   now.AddDate(0, 0, DefaultReportTTLDays),
	}
}

// IsExpired returns true if the report has expired.
func (r *ReportEntity) IsExpired() bool {
	return time.Now().After(r.ExpiresAt)
}

// MarkCompleted marks the report as completed.
func (r *ReportEntity) MarkCompleted(content *ReportContent) {
	r.Content = content
	r.Status = ReportStatusCompleted
	r.UpdatedAt = time.Now()
}

// MarkFailed marks the report as failed.
func (r *ReportEntity) MarkFailed(err string) {
	r.ErrorMessage = err
	r.Status = ReportStatusFailed
	r.UpdatedAt = time.Now()
}
