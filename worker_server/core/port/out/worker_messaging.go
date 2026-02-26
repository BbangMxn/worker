package out

import (
	"context"
	"time"
)

// Time alias for JSON serialization
type Time = time.Time

// MessageProducer defines the outbound port for message queue producer.
type MessageProducer interface {
	// Mail jobs
	PublishMailSend(ctx context.Context, job *MailSendJob) error
	PublishMailSync(ctx context.Context, job *MailSyncJob) error
	PublishMailSyncInit(ctx context.Context, job *MailSyncInitJob) error
	PublishMailSyncPage(ctx context.Context, job *MailSyncPageJob) error
	PublishMailBatch(ctx context.Context, job *MailBatchJob) error
	PublishMailSave(ctx context.Context, job *MailSaveJob) error     // 메타데이터 저장 (비동기)
	PublishMailModify(ctx context.Context, job *MailModifyJob) error // Provider 상태 동기화 (비동기)

	// Calendar jobs
	PublishCalendarSync(ctx context.Context, job *CalendarSyncJob) error
	PublishCalendarEvent(ctx context.Context, job *CalendarEventJob) error

	// AI jobs
	PublishAIClassify(ctx context.Context, job *AIClassifyJob) error
	PublishAIBatchClassify(ctx context.Context, job *AIBatchClassifyJob) error
	PublishAISummarize(ctx context.Context, job *AISummarizeJob) error
	PublishAITranslate(ctx context.Context, job *AITranslateJob) error
	PublishAIAutocomplete(ctx context.Context, job *AIAutocompleteJob) error
	PublishAIChat(ctx context.Context, job *AIChatJob) error
	PublishAIReply(ctx context.Context, job *AIReplyJob) error
	PublishAIGenerateReply(ctx context.Context, job *AIGenerateReplyJob) error

	// RAG jobs
	PublishRAGIndex(ctx context.Context, job *RAGIndexJob) error
	PublishRAGBatchIndex(ctx context.Context, job *RAGBatchIndexJob) error
	PublishRAGSearch(ctx context.Context, job *RAGSearchJob) error

	// Profile jobs
	PublishProfileAnalyze(ctx context.Context, job *ProfileAnalyzeJob) error

	// Priority jobs
	PublishPriority(ctx context.Context, stream string, job interface{}) error

	// Sync status
	SetSyncStatus(ctx context.Context, connectionID int64, status *SyncStatus) error
	GetSyncStatus(ctx context.Context, connectionID int64) (*SyncStatus, error)
	IncrementSyncProgress(ctx context.Context, connectionID int64, emailCount int) error
}

// Job types for message queue

// MailSendJob represents mail send job.
type MailSendJob struct {
	UserID       string   `json:"user_id"`
	ConnectionID int64    `json:"connection_id"`
	To           []string `json:"to"`
	Cc           []string `json:"cc,omitempty"`
	Bcc          []string `json:"bcc,omitempty"`
	Subject      string   `json:"subject"`
	Body         string   `json:"body"`
	ReplyToID    string   `json:"reply_to_id,omitempty"`
	Attachments  []string `json:"attachments,omitempty"`
}

// MailSyncJob represents mail sync job.
type MailSyncJob struct {
	UserID       string `json:"user_id"`
	ConnectionID int64  `json:"connection_id"`
	Provider     string `json:"provider,omitempty"`
	FullSync     bool   `json:"full_sync"`
	PageToken    string `json:"page_token,omitempty"`
	HistoryID    uint64 `json:"history_id,omitempty"` // Gmail Pub/Sub delta sync용
	Background   bool   `json:"background,omitempty"` // 백그라운드 점진적 동기화
}

// MailBatchJob represents mail batch job.
type MailBatchJob struct {
	UserID  string   `json:"user_id"`
	Action  string   `json:"action"` // read, unread, archive, trash, delete
	MailIDs []int64  `json:"mail_ids"`
	Tags    []string `json:"tags,omitempty"`
}

// MailSaveJob represents mail metadata save job (async).
// Gmail API에서 가져온 메일을 DB에 저장하는 비동기 작업
type MailSaveJob struct {
	UserID       string          `json:"user_id"`
	ConnectionID int64           `json:"connection_id"`
	AccountEmail string          `json:"account_email"`
	Provider     string          `json:"provider"`
	Emails       []MailSaveEmail `json:"emails"`
}

// MailSaveEmail represents email data for batch save.
type MailSaveEmail struct {
	ExternalID string   `json:"external_id"`
	ThreadID   string   `json:"thread_id,omitempty"`
	Subject    string   `json:"subject"`
	FromEmail  string   `json:"from_email"`
	FromName   string   `json:"from_name,omitempty"`
	ToEmails   []string `json:"to_emails,omitempty"`
	CcEmails   []string `json:"cc_emails,omitempty"`
	Snippet    string   `json:"snippet,omitempty"`
	IsRead     bool     `json:"is_read"`
	HasAttach  bool     `json:"has_attachment"`
	Folder     string   `json:"folder"`
	Labels     []string `json:"labels,omitempty"`
	ReceivedAt Time     `json:"received_at"`
}

// MailModifyJob represents mail state modification job for Provider sync + SSE broadcast.
// DB 상태 변경 후 Provider(Gmail/Outlook)에 비동기로 동기화 + 다른 클라이언트에 SSE 전송
type MailModifyJob struct {
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

// CalendarSyncJob represents calendar sync job.
type CalendarSyncJob struct {
	UserID       string `json:"user_id"`
	ConnectionID int64  `json:"connection_id"`
	CalendarID   string `json:"calendar_id,omitempty"`
	FullSync     bool   `json:"full_sync"`
}

// CalendarEventJob represents calendar event job.
type CalendarEventJob struct {
	UserID       string      `json:"user_id"`
	ConnectionID int64       `json:"connection_id"`
	Action       string      `json:"action"` // create, update, delete
	EventID      int64       `json:"event_id,omitempty"`
	Event        interface{} `json:"event,omitempty"`
}

// EventCreateJob represents event creation job.
type EventCreateJob struct {
	UserID      string   `json:"user_id"`
	CalendarID  int64    `json:"calendar_id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Location    string   `json:"location,omitempty"`
	StartTime   Time     `json:"start_time"`
	EndTime     Time     `json:"end_time"`
	IsAllDay    bool     `json:"is_all_day"`
	TimeZone    string   `json:"time_zone,omitempty"`
	Attendees   []string `json:"attendees,omitempty"`
}

// EventUpdateJob represents event update job.
type EventUpdateJob struct {
	UserID      string `json:"user_id"`
	EventID     int64  `json:"event_id"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Location    string `json:"location,omitempty"`
	StartTime   Time   `json:"start_time,omitempty"`
	EndTime     Time   `json:"end_time,omitempty"`
}

// EventDeleteJob represents event delete job.
type EventDeleteJob struct {
	UserID  string `json:"user_id"`
	EventID int64  `json:"event_id"`
}

// EventBatchDeleteJob represents event batch delete job.
type EventBatchDeleteJob struct {
	UserID   string  `json:"user_id"`
	EventIDs []int64 `json:"event_ids"`
}

// AIClassifyJob represents AI classify job.
type AIClassifyJob struct {
	UserID        string   `json:"user_id"`
	EmailID       int64    `json:"email_id"`
	From          string   `json:"from,omitempty"`
	FromName      string   `json:"from_name,omitempty"`
	To            []string `json:"to,omitempty"`
	Subject       string   `json:"subject,omitempty"`
	Body          string   `json:"body,omitempty"`
	Snippet       string   `json:"snippet,omitempty"`
	HasAttachment bool     `json:"has_attachment,omitempty"`
	IsReply       bool     `json:"is_reply,omitempty"`
}

// AIBatchClassifyJob represents AI batch classify job for multiple emails.
type AIBatchClassifyJob struct {
	UserID   string            `json:"user_id"`
	EmailIDs []int64           `json:"email_ids"`
	Emails   []AIClassifyEmail `json:"emails,omitempty"` // 이미 로드된 이메일 정보 (DB 재조회 방지)
}

// AIClassifyEmail represents email data for batch classification.
type AIClassifyEmail struct {
	EmailID       int64    `json:"email_id"`
	From          string   `json:"from"`
	FromName      string   `json:"from_name,omitempty"`
	To            []string `json:"to,omitempty"`
	Subject       string   `json:"subject"`
	Snippet       string   `json:"snippet"`
	HasAttachment bool     `json:"has_attachment,omitempty"`
	IsReply       bool     `json:"is_reply,omitempty"`
}

// AISummarizeJob represents AI summarize job.
type AISummarizeJob struct {
	UserID        string `json:"user_id"`
	EmailID       int64  `json:"email_id"`
	ThreadID      *int64 `json:"thread_id,omitempty"`
	Subject       string `json:"subject,omitempty"`
	Body          string `json:"body,omitempty"`
	Level         string `json:"level,omitempty"`          // brief, detailed
	Language      string `json:"language,omitempty"`       // 요약 언어 (ko, en, ja 등), 비어있으면 사용자 설정 언어
	AutoGenerated bool   `json:"auto_generated,omitempty"` // 자동 생성 여부 (동기화 시 자동 발행)
}

// AITranslateJob represents AI translate job.
type AITranslateJob struct {
	UserID     string `json:"user_id"`
	EmailID    int64  `json:"email_id,omitempty"`
	Text       string `json:"text,omitempty"`
	TargetLang string `json:"target_lang"`
}

// AIAutocompleteJob represents AI autocomplete job.
type AIAutocompleteJob struct {
	UserID    string `json:"user_id"`
	Prefix    string `json:"prefix"`
	Text      string `json:"text,omitempty"`
	Context   string `json:"context,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
	MaxLen    int    `json:"max_len,omitempty"`
}

// AIChatJob represents AI chat job.
type AIChatJob struct {
	UserID      string      `json:"user_id"`
	SessionID   string      `json:"session_id,omitempty"`
	Message     string      `json:"message"`
	Model       string      `json:"model,omitempty"`
	Temperature float32     `json:"temperature,omitempty"`
	Stream      bool        `json:"stream,omitempty"`
	History     interface{} `json:"history,omitempty"`
	Context     string      `json:"context,omitempty"`
}

// AIReplyJob represents AI reply generation job.
type AIReplyJob struct {
	UserID          string `json:"user_id"`
	EmailID         int64  `json:"email_id"`
	OriginalFrom    string `json:"original_from,omitempty"`
	OriginalSubject string `json:"original_subject,omitempty"`
	OriginalBody    string `json:"original_body,omitempty"`
	Tone            string `json:"tone,omitempty"`
	Intent          string `json:"intent,omitempty"`
	Instructions    string `json:"instructions,omitempty"`
}

// AIGenerateReplyJob represents AI generate reply job (alias).
type AIGenerateReplyJob = AIReplyJob

// RAGIndexJob represents RAG index job.
type RAGIndexJob struct {
	UserID  string `json:"user_id"`
	EmailID int64  `json:"email_id"`
}

// RAGSearchJob represents RAG search job.
type RAGSearchJob struct {
	UserID    string  `json:"user_id"`
	Query     string  `json:"query"`
	TopK      int     `json:"top_k,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
}

// RAGBatchIndexJob represents RAG batch index job for initial sync.
type RAGBatchIndexJob struct {
	UserID       string  `json:"user_id"`
	ConnectionID int64   `json:"connection_id"`
	EmailIDs     []int64 `json:"email_ids"`
}

// RAGReindexJob represents RAG reindex job.
type RAGReindexJob struct {
	UserID       string `json:"user_id"`
	ConnectionID int64  `json:"connection_id"`
	FullReindex  bool   `json:"full_reindex"`
}

// ProfileAnalyzeJob represents user profile analysis job.
type ProfileAnalyzeJob struct {
	UserID       string `json:"user_id"`
	ConnectionID int64  `json:"connection_id"`
	SampleSize   int    `json:"sample_size"` // Number of sent emails to analyze (default: 50)
}

// =============================================================================
// Sync Status
// =============================================================================

// SyncPhase represents sync phase.
type SyncPhase string

const (
	SyncPhaseRecent SyncPhase = "recent" // 최근 30일 (우선)
	SyncPhaseFull   SyncPhase = "full"   // 전체 히스토리
)

// SyncStatus represents connection sync status.
type SyncStatus struct {
	ConnectionID int64     `json:"connection_id"`
	Phase        SyncPhase `json:"phase"`
	Status       string    `json:"status"` // pending, syncing, completed, failed
	TotalPages   int       `json:"total_pages"`
	SyncedPages  int       `json:"synced_pages"`
	TotalEmails  int       `json:"total_emails"`
	SyncedEmails int       `json:"synced_emails"`
	ErrorMessage string    `json:"error_message,omitempty"`
	StartedAt    *Time     `json:"started_at,omitempty"`
	CompletedAt  *Time     `json:"completed_at,omitempty"`
}

// MailSyncInitJob represents initial sync job that discovers all pages.
type MailSyncInitJob struct {
	UserID       string    `json:"user_id"`
	ConnectionID int64     `json:"connection_id"`
	Provider     string    `json:"provider"`
	Phase        SyncPhase `json:"phase"`
	StartDate    *Time     `json:"start_date,omitempty"` // 최근 N일 동기화용
}

// MailSyncPageJob represents a single page sync job.
type MailSyncPageJob struct {
	UserID       string    `json:"user_id"`
	ConnectionID int64     `json:"connection_id"`
	Provider     string    `json:"provider"`
	Phase        SyncPhase `json:"phase"`
	PageToken    string    `json:"page_token"`
	PageNumber   int       `json:"page_number"`
	TotalPages   int       `json:"total_pages"`
}
