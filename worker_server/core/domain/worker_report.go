package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type ReportType string

const (
	ReportTypeDaily   ReportType = "daily"
	ReportTypeWeekly  ReportType = "weekly"
	ReportTypeMonthly ReportType = "monthly"
)

// Report contains email report with action items and schedule suggestions
type Report struct {
	ID         int64     `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	ReportType string    `json:"report_type"`
	StartDate  time.Time `json:"start_date"`
	EndDate    time.Time `json:"end_date"`

	// Email counts
	TotalEmails      int `json:"total_emails"`
	ImportantEmails  int `json:"important_emails"`
	NeedsReplyEmails int `json:"needs_reply_emails"`

	// LLM-generated summaries
	ImportantSummary   string `json:"important_summary"`
	ReplyNeededSummary string `json:"reply_needed_summary"`

	// Extracted suggestions (LLM)
	ScheduleSuggestions []ScheduleSuggestion `json:"schedule_suggestions"`
	ActionItems         []ActionItem         `json:"action_items"`

	// Stats
	CategoryBreakdown map[EmailCategory]int `json:"category_breakdown,omitempty"`
	PriorityBreakdown map[Priority]int      `json:"priority_breakdown,omitempty"`
	TopSenders        []SenderStat          `json:"top_senders,omitempty"`

	GeneratedAt time.Time `json:"generated_at"`
}

// EmailReport is the legacy report type for backward compatibility
type EmailReport struct {
	ID          int64      `json:"id"`
	UserID      uuid.UUID  `json:"user_id"`
	Type        ReportType `json:"type"`
	PeriodStart time.Time  `json:"period_start"`
	PeriodEnd   time.Time  `json:"period_end"`

	// Email stats
	TotalReceived int `json:"total_received"`
	TotalSent     int `json:"total_sent"`
	TotalRead     int `json:"total_read"`
	TotalUnread   int `json:"total_unread"`

	// Category breakdown
	CategoryBreakdown map[EmailCategory]int `json:"category_breakdown"`

	// Priority breakdown
	PriorityBreakdown map[Priority]int `json:"priority_breakdown"`

	// Top senders
	TopSenders []SenderStat `json:"top_senders"`

	// Response time
	AvgResponseTimeMinutes float64 `json:"avg_response_time_minutes"`

	// AI usage
	AIClassifiedCount  int `json:"ai_classified_count"`
	AIRepliesGenerated int `json:"ai_replies_generated"`

	GeneratedAt time.Time `json:"generated_at"`
}

type SenderStat struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
	Count int    `json:"count"`
}

type ReportRepository interface {
	// New Report methods
	GetByID(ctx context.Context, id int64) (*Report, error)
	GetLatest(ctx context.Context, userID uuid.UUID, reportType string) (*Report, error)
	ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Report, int, error)
	Create(ctx context.Context, report *Report) error

	// Legacy methods
	GetLegacyByID(id int64) (*EmailReport, error)
	GetLatestByType(userID uuid.UUID, reportType ReportType) (*EmailReport, error)
	List(userID uuid.UUID, limit int) ([]*EmailReport, error)
	CreateLegacy(report *EmailReport) error
}
