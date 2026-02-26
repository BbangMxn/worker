package worker

import (
	"time"

	"github.com/google/uuid"
)

// Priority levels for job scheduling.
type Priority int

const (
	PriorityLow      Priority = 0
	PriorityNormal   Priority = 1
	PriorityHigh     Priority = 2
	PriorityCritical Priority = 3
)

// JobType represents the type of a job.
type JobType = string

// Job types - aligned with CLAUDE.md specification
const (
	// Mail jobs
	JobMailSync      JobType = "mail.sync"
	JobMailDeltaSync         = "mail.delta_sync" // Pub/Sub 기반 증분 동기화
	JobMailBatch             = "mail.batch"
	JobMailSend              = "mail.send"
	JobMailReply             = "mail.reply"
	JobMailSave              = "mail.save"   // 비동기 메타데이터 저장
	JobMailModify            = "mail.modify" // Provider 상태 동기화

	// AI jobs
	JobAIClassify  = "ai.classify"
	JobAISummarize = "ai.summarize"
	JobAIReply     = "ai.reply"

	// Profile jobs (user analysis from sent emails)
	JobProfileAnalyze   = "profile.analyze"
	JobProfileReanalyze = "profile.reanalyze"

	// RAG jobs
	JobRAGIndex      = "rag.index"
	JobRAGBatchIndex = "rag.batch_index"

	// Report jobs
	JobReportGenerate = "report.generate"
	JobReportSchedule = "report.schedule"

	// Calendar jobs
	JobCalendarSync = "calendar.sync"

	// Webhook jobs
	JobWebhookRenew = "webhook.renew"
)

type Message struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	Priority  Priority       `json:"priority"`
	CreatedAt time.Time      `json:"created_at"`
	Retries   int            `json:"retries"`
}

func NewMessage(jobType string, payload map[string]any) *Message {
	return &Message{
		ID:        uuid.New().String(),
		Type:      jobType,
		Payload:   payload,
		Priority:  PriorityNormal,
		CreatedAt: time.Now(),
		Retries:   0,
	}
}

// NewPriorityMessage creates a message with specific priority.
func NewPriorityMessage(jobType string, payload map[string]any, priority Priority) *Message {
	return &Message{
		ID:        uuid.New().String(),
		Type:      jobType,
		Payload:   payload,
		Priority:  priority,
		CreatedAt: time.Now(),
		Retries:   0,
	}
}

// IsPriority checks if message should go to priority queue.
func (m *Message) IsPriority() bool {
	return m.Priority >= PriorityHigh
}

// Mail payloads
type MailSyncPayload struct {
	ConnectionID int64  `json:"connection_id"`
	UserID       string `json:"user_id"` // string으로 받아서 필요 시 uuid.Parse
	Provider     string `json:"provider,omitempty"`
	FullSync     bool   `json:"full_sync"`
	PageToken    string `json:"page_token,omitempty"`
	HistoryID    uint64 `json:"history_id,omitempty"` // Pub/Sub delta sync용
}

type MailSendPayload struct {
	UserID       uuid.UUID `json:"user_id"`
	ConnectionID int64     `json:"connection_id"`
	To           []string  `json:"to"`
	Cc           []string  `json:"cc,omitempty"`
	Bcc          []string  `json:"bcc,omitempty"`
	Subject      string    `json:"subject"`
	Body         string    `json:"body"`
	IsHTML       bool      `json:"is_html"`
}

type MailReplyPayload struct {
	UserID       uuid.UUID `json:"user_id"`
	ConnectionID int64     `json:"connection_id"`
	OriginalID   string    `json:"original_id"` // 원본 메일 ID
	To           []string  `json:"to"`
	Cc           []string  `json:"cc,omitempty"`
	Subject      string    `json:"subject"`
	Body         string    `json:"body"`
	IsHTML       bool      `json:"is_html"`
}

// MailSavePayload represents mail metadata save job payload.
type MailSavePayload struct {
	UserID       string              `json:"user_id"` // string으로 받아서 필요 시 uuid.Parse
	ConnectionID int64               `json:"connection_id"`
	AccountEmail string              `json:"account_email"`
	Provider     string              `json:"provider"`
	Emails       []MailSaveEmailData `json:"emails"`
}

// MailSaveEmailData represents individual email data for batch save.
type MailSaveEmailData struct {
	ExternalID string    `json:"external_id"`
	ThreadID   string    `json:"thread_id,omitempty"`
	Subject    string    `json:"subject"`
	FromEmail  string    `json:"from_email"`
	FromName   string    `json:"from_name,omitempty"`
	ToEmails   []string  `json:"to_emails,omitempty"`
	CcEmails   []string  `json:"cc_emails,omitempty"`
	Snippet    string    `json:"snippet,omitempty"`
	IsRead     bool      `json:"is_read"`
	HasAttach  bool      `json:"has_attachment"`
	Folder     string    `json:"folder"`
	Labels     []string  `json:"labels,omitempty"`
	ReceivedAt time.Time `json:"received_at"`
}

// MailModifyPayload represents mail state modification job for Provider sync.
type MailModifyPayload struct {
	UserID       string   `json:"user_id"`
	ConnectionID int64    `json:"connection_id"`
	Provider     string   `json:"provider"`      // google, outlook
	Action       string   `json:"action"`        // read, unread, star, unstar, archive, trash, move, labels
	EmailIDs     []int64  `json:"email_ids"`     // DB email IDs (for SSE broadcast)
	ExternalIDs  []string `json:"external_ids"`  // Provider message IDs
	AddLabels    []string `json:"add_labels"`    // Labels to add (Gmail)
	RemoveLabels []string `json:"remove_labels"` // Labels to remove (Gmail)
	TargetFolder string   `json:"target_folder"` // Target folder for move
}

// AI payloads
type AIClassifyPayload struct {
	EmailID int64     `json:"email_id"`
	UserID  uuid.UUID `json:"user_id"`
}

type AIClassifyBatchPayload struct {
	EmailIDs []int64   `json:"email_ids"`
	UserID   uuid.UUID `json:"user_id"`
}

// Profile payloads
type ProfileAnalyzePayload struct {
	UserID       uuid.UUID `json:"user_id"`
	ConnectionID int64     `json:"connection_id"`
	SampleSize   int       `json:"sample_size"` // default 50
}

// RAG payloads
type RAGIndexPayload struct {
	EmailID   int64     `json:"email_id"`
	UserID    uuid.UUID `json:"user_id"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	FromEmail string    `json:"from_email"`
	Direction string    `json:"direction"` // inbound, outbound
	Folder    string    `json:"folder"`
}

type RAGBatchIndexPayload struct {
	Emails []RAGIndexPayload `json:"emails"`
}

// Report payloads
type ReportGeneratePayload struct {
	UserID     uuid.UUID `json:"user_id"`
	ReportType string    `json:"report_type"` // daily, weekly, monthly
	StartDate  time.Time `json:"start_date"`
	EndDate    time.Time `json:"end_date"`
}

type ReportSchedulePayload struct {
	UserID     uuid.UUID `json:"user_id"`
	ReportType string    `json:"report_type"`
	Schedule   string    `json:"schedule"` // cron expression
}

// Calendar payloads
type CalendarSyncPayload struct {
	ConnectionID int64  `json:"connection_id"`
	UserID       string `json:"user_id"`
	CalendarID   string `json:"calendar_id,omitempty"` // Provider's calendar ID for delta sync
	FullSync     bool   `json:"full_sync"`
	SyncToken    string `json:"sync_token,omitempty"` // For incremental sync
}

// Webhook payloads
type WebhookRenewPayload struct {
	WebhookID    int64 `json:"webhook_id,omitempty"`
	ConnectionID int64 `json:"connection_id,omitempty"`
	RenewAll     bool  `json:"renew_all"` // If true, renew all expiring webhooks
}
