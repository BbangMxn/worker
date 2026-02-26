// Package out defines outbound ports (driven ports) for the application.
// These interfaces represent dependencies that the application needs.
package out

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Mail Repository (Supabase/PostgreSQL - 메타데이터)
// =============================================================================

// EmailRepository defines the outbound port for mail metadata persistence.
// 이메일 메타데이터만 저장. 본문은 EmailBodyRepository (MongoDB)에 저장.
type EmailRepository interface {
	// CRUD operations
	Create(ctx context.Context, mail *MailEntity) error
	Update(ctx context.Context, mail *MailEntity) error
	Delete(ctx context.Context, id int64) error
	GetByID(ctx context.Context, id int64) (*MailEntity, error)
	GetByExternalID(ctx context.Context, connectionID int64, externalID string) (*MailEntity, error)
	GetByExternalIDs(ctx context.Context, connectionID int64, externalIDs []string) (map[string]*MailEntity, error)

	// Query operations
	List(ctx context.Context, userID uuid.UUID, req *MailListQuery) ([]*MailEntity, int, error)
	Search(ctx context.Context, userID uuid.UUID, query string, limit, offset int) ([]*MailEntity, int, error)
	ListByContact(ctx context.Context, userID uuid.UUID, contactID int64, limit, offset int) ([]*MailEntity, int, error)

	// Thread operations
	GetThreadMessages(ctx context.Context, threadID int64) ([]*MailEntity, error)
	GetThreadByID(ctx context.Context, threadID int64) (*MailThreadEntity, error)
	ListThreads(ctx context.Context, userID uuid.UUID, req *MailListQuery) ([]*MailThreadEntity, int, error)
	GetOrCreateThread(ctx context.Context, mail *MailEntity) (int64, error)
	UpdateThreadStats(ctx context.Context, threadID int64) error

	// Status updates
	UpdateReadStatus(ctx context.Context, id int64, isRead bool) error
	UpdateFolder(ctx context.Context, id int64, folder string) error
	UpdateTags(ctx context.Context, id int64, tags []string) error
	UpdateWorkflowStatus(ctx context.Context, id int64, status string, snoozeUntil *time.Time) error
	UpdateHasAttachment(ctx context.Context, id int64, hasAttachment bool) error
	AddLabel(ctx context.Context, id int64, label string) error
	RemoveLabel(ctx context.Context, id int64, label string) error

	// AI results
	UpdateAIResult(ctx context.Context, id int64, result *MailAIResult) error
	UpdateThreadAIResult(ctx context.Context, threadID int64, result *MailAIResult) error

	// Batch operations
	BatchUpdateReadStatus(ctx context.Context, ids []int64, isRead bool) error
	BatchUpdateFolder(ctx context.Context, ids []int64, folder string) error
	BatchUpdateTags(ctx context.Context, ids []int64, addTags, removeTags []string) error
	BatchUpdateWorkflowStatus(ctx context.Context, ids []int64, status string, snoozeUntil *time.Time) error
	BatchDelete(ctx context.Context, ids []int64) error
	BulkUpsert(ctx context.Context, userID uuid.UUID, connectionID int64, mails []*MailEntity) error
	DeleteByExternalIDs(ctx context.Context, connectionID int64, externalIDs []string) error

	// Statistics
	GetStats(ctx context.Context, userID uuid.UUID) (*MailStats, error)
	CountUnread(ctx context.Context, userID uuid.UUID, connectionID *int64) (int, error)
	GetCategoryStats(ctx context.Context, userID uuid.UUID, connectionID *int64) (map[string]*CategoryStatItem, error)

	// Snooze
	GetSnoozedToWake(ctx context.Context) ([]*MailEntity, error)
	UnsnoozeExpired(ctx context.Context) (int, error)

	// Resync
	GetEmailsWithPendingAttachments(ctx context.Context, userID uuid.UUID, connectionID int64) ([]*MailEntity, error)
	GetEmailsNeedingAttachmentResync(ctx context.Context, userID uuid.UUID, connectionID int64, limit int) ([]*MailEntity, error)
	GetEmailsByExternalIDsNeedingAttachments(ctx context.Context, userID uuid.UUID, connectionID int64, externalIDs []string) ([]*MailEntity, error)

	// AI pending
	ListPendingAI(ctx context.Context, userID uuid.UUID, limit int) ([]*MailEntity, error)
	CountUnclassified(ctx context.Context, connectionID int64) (int, error)
	ListUnclassifiedByConnection(ctx context.Context, connectionID int64, limit int) ([]*MailEntity, error)

	// Profile analysis
	GetSentEmails(ctx context.Context, userID uuid.UUID, connectionID int64, limit int) ([]*MailEntity, error)

	// Translation
	SaveTranslation(ctx context.Context, result *MailTranslation) error
	GetTranslation(ctx context.Context, emailID int64, targetLang string) (*MailTranslation, error)
}

// =============================================================================
// Mail Contact Repository (Contact enrichment)
// =============================================================================

// MailContactRepository defines the port for contact lookup in mail context.
type MailContactRepository interface {
	// 이메일 주소로 연락처 조회 (캐시됨)
	GetContactByEmail(ctx context.Context, userID uuid.UUID, email string) (*MailContactInfo, error)

	// 여러 이메일 주소 일괄 조회 (성능 최적화)
	BulkGetContactsByEmail(ctx context.Context, userID uuid.UUID, emails []string) (map[string]*MailContactInfo, error)

	// 메일과 연락처 연결
	LinkMailToContact(ctx context.Context, mailID int64, contactID int64) error

	// 연락처 상호작용 업데이트 (메일 주고받을 때)
	UpdateContactInteraction(ctx context.Context, userID uuid.UUID, email string) error
}

// MailContactInfo represents contact info for mail enrichment.
type MailContactInfo struct {
	ContactID  int64  `json:"contact_id"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	PhotoURL   string `json:"photo_url,omitempty"`
	Company    string `json:"company,omitempty"`
	JobTitle   string `json:"job_title,omitempty"`
	IsVIP      bool   `json:"is_vip"`
	IsFavorite bool   `json:"is_favorite"`
}

// =============================================================================
// Mail Entity (PostgreSQL)
// =============================================================================

// MailEntity represents mail domain entity for persistence.
type MailEntity struct {
	ID           int64
	ExternalID   string
	ThreadID     *int64
	ConnectionID int64
	UserID       uuid.UUID

	// Provider info
	Provider     string
	AccountEmail string

	// Message IDs for threading
	MessageID  string
	InReplyTo  string
	References []string

	// Sender/Recipients (raw emails)
	FromEmail string
	FromName  string
	ToEmails  []string
	CcEmails  []string
	BccEmails []string

	// Content (snippet만, 본문은 MongoDB)
	Subject string
	Snippet string

	// Direction
	Direction string // inbound, outbound

	// Status
	IsRead        bool
	IsStarred     bool
	IsDraft       bool
	HasAttachment bool
	IsReplied     bool
	IsForwarded   bool

	// Organization
	Folder string
	Labels []string
	Tags   []string

	// Workflow
	WorkflowStatus string
	SnoozedUntil   *time.Time

	// AI results
	AIStatus             string
	Category             string
	SubCategory          string  // Sub-category for more granular classification
	Priority             float64 // 0.0 ~ 1.0 priority score
	Sentiment            float64
	Summary              string
	ActionItem           string
	Intent               string  // action_required, fyi, urgent, follow_up, scheduling
	IsUrgent             bool    // 긴급 메일 플래그
	DueDate              *string // 감지된 마감일
	AIScore              float64 // Classification confidence score
	ClassificationSource string  // header, domain, llm, user

	// Contact link
	ContactID *int64

	// Timestamps
	ReceivedAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time

	// Search (not persisted, used for ranking)
	SearchScore float64 `json:"-"` // BM25-style search relevance score
}

// MailThreadEntity represents mail thread domain entity.
type MailThreadEntity struct {
	ID           int64
	UserID       uuid.UUID
	ConnectionID int64

	// Provider info
	Provider         string
	AccountEmail     string
	ExternalThreadID string

	// Info
	Subject      string
	Snippet      string
	Participants []string // raw emails

	// Status
	HasUnread     bool
	HasStarred    bool
	HasAttachment bool

	// Workflow
	WorkflowStatus string
	SnoozedUntil   *time.Time

	// AI results
	AIStatus string
	Category string
	Priority float64 // 0.0 ~ 1.0 priority score
	Summary  string

	// Stats
	MessageCount  int
	LastMessageID int64
	LatestAt      time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// =============================================================================
// Query/Result Types
// =============================================================================

// MailListQuery represents mail list query parameters.
type MailListQuery struct {
	// Connection
	ConnectionID *int64
	// Filters
	Folder         string
	FolderID       *int64
	Category       string
	SubCategory    string
	Labels         []string
	Tags           []string
	IsRead         *bool
	IsStarred      *bool
	HasAttachment  *bool
	Priority       *float64 // 0.0 ~ 1.0 priority score filter (minimum threshold)
	ContactID      *int64
	WorkflowStatus string
	FromEmail      string
	FromDomain     string
	LabelIDs       []int64

	// === Inbox/Category View Filters ===
	// Categories: multiple category filter (OR logic)
	// e.g., ["primary", "work", "personal"] for Inbox view
	Categories []string

	// SubCategories: multiple sub-category filter (OR logic)
	// e.g., ["receipt", "invoice"] for Receipts tab
	SubCategories []string

	// ExcludeCategories: exclude specific categories
	// e.g., ["spam", "marketing"] to filter out noise
	ExcludeCategories []string

	// ViewType: predefined view filter
	// "inbox" = personal mail only (primary, work, personal) + priority >= 0.4
	// "all" = all mail (default)
	// "category" = specific category (use Category field)
	ViewType string

	// Pagination
	Limit  int
	Offset int

	// Sorting
	OrderBy string
	Order   string
}

// MailAIResult represents AI processing result.
type MailAIResult struct {
	Status     string   // pending, processing, completed, failed
	Category   string   // primary, social, promotions, updates, forums, newsletters
	Priority   float64  // 0.0 ~ 1.0 priority score (higher = more important)
	Sentiment  float64  // -1.0 ~ 1.0 (negative ~ positive)
	Summary    string   // 요약 (사용자 언어로)
	ActionItem string   // 필요한 액션 설명
	Tags       []string // AI 생성 태그
	Intent     string   // action_required, fyi, urgent, follow_up, scheduling, feedback_request
	IsUrgent   bool     // 긴급 메일 여부 (Priority >= 0.80)
	DueDate    *string  // 마감일 감지 (있는 경우)
}

// MailStats represents mail statistics.
type MailStats struct {
	TotalCount   int
	UnreadCount  int
	StarredCount int
	ActionCount  int
	SnoozedCount int
	ByFolder     map[string]int
	ByCategory   map[string]int
	ByPriority   map[string]int // "urgent", "high", "normal", "low", "lowest"
}

// CategoryStatItem represents statistics for a single category.
type CategoryStatItem struct {
	Total  int `json:"total"`
	Unread int `json:"unread"`
}

// MailTranslation represents mail translation.
type MailTranslation struct {
	EmailID    int64
	TargetLang string
	Subject    string
	Body       string
	CreatedAt  time.Time
}

// =============================================================================
// Attachment Repository
// =============================================================================

// AttachmentRepository defines the outbound port for attachment metadata persistence.
// 실제 파일은 저장하지 않고 Provider API에서 직접 다운로드.
type AttachmentRepository interface {
	// CRUD operations
	Create(ctx context.Context, attachment *EmailAttachmentEntity) error
	CreateBatch(ctx context.Context, attachments []*EmailAttachmentEntity) error
	GetByID(ctx context.Context, id int64) (*EmailAttachmentEntity, error)
	GetByEmailID(ctx context.Context, emailID int64) ([]*EmailAttachmentEntity, error)
	GetByExternalID(ctx context.Context, emailID int64, externalID string) (*EmailAttachmentEntity, error)
	GetByContentID(ctx context.Context, emailID int64, contentID string) (*EmailAttachmentEntity, error) // 인라인 첨부파일 조회
	Delete(ctx context.Context, id int64) error
	DeleteByEmailID(ctx context.Context, emailID int64) error

	// Update operations
	UpdateExternalIDs(ctx context.Context, emailID int64, updates []AttachmentExternalIDUpdate) error // pending ID를 실제 ID로 업데이트

	// Query operations
	ListByEmails(ctx context.Context, emailIDs []int64) (map[int64][]*EmailAttachmentEntity, error)
	CountByEmailID(ctx context.Context, emailID int64) (int, error)
	GetInlineByEmailID(ctx context.Context, emailID int64) ([]*EmailAttachmentEntity, error)  // 인라인 첨부파일만 조회
	GetPendingByEmailID(ctx context.Context, emailID int64) ([]*EmailAttachmentEntity, error) // pending ID 첨부파일 조회

	// User-scoped queries (모아보기)
	ListByUser(ctx context.Context, userID uuid.UUID, query *AttachmentListQuery) ([]*AttachmentWithEmail, int, error)
	GetStatsByUser(ctx context.Context, userID uuid.UUID) (*AttachmentStats, error)
	SearchByUser(ctx context.Context, userID uuid.UUID, filename string, limit, offset int) ([]*AttachmentWithEmail, int, error)
}

// AttachmentExternalIDUpdate represents an external ID update for an attachment.
type AttachmentExternalIDUpdate struct {
	OldExternalID string // pending_xxx 형태의 임시 ID
	NewExternalID string // 실제 Gmail attachment ID
	Filename      string // 파일명으로 매칭 (백업용)
}

// AttachmentListQuery represents query options for attachment listing.
type AttachmentListQuery struct {
	ConnectionID *int64   // 특정 계정만
	MimeTypes    []string // 파일 유형 필터 (image/*, application/pdf 등)
	MinSize      *int64   // 최소 크기 (bytes)
	MaxSize      *int64   // 최대 크기 (bytes)
	StartDate    *time.Time
	EndDate      *time.Time
	Limit        int
	Offset       int
	SortBy       string // created_at, size, filename
	SortOrder    string // asc, desc
}

// AttachmentWithEmail represents attachment with associated email info.
type AttachmentWithEmail struct {
	// Attachment info
	ID         int64     `json:"id"`
	EmailID    int64     `json:"email_id"`
	ExternalID string    `json:"external_id"`
	Filename   string    `json:"filename"`
	MimeType   string    `json:"mime_type"`
	Size       int64     `json:"size"`
	IsInline   bool      `json:"is_inline"`
	CreatedAt  time.Time `json:"created_at"`

	// Email info (JOIN)
	EmailSubject    string    `json:"email_subject"`
	EmailFrom       string    `json:"email_from"`
	EmailDate       time.Time `json:"email_date"`
	ConnectionID    int64     `json:"connection_id"`
	EmailProvider   string    `json:"email_provider"`
	EmailExternalID string    `json:"email_external_id"`
}

// AttachmentStats represents attachment statistics for a user.
type AttachmentStats struct {
	TotalCount      int              `json:"total_count"`
	TotalSize       int64            `json:"total_size"`
	CountByMimeType map[string]int   `json:"count_by_mime_type"`
	SizeByMimeType  map[string]int64 `json:"size_by_mime_type"`
}

// EmailAttachmentEntity represents attachment metadata for PostgreSQL persistence.
type EmailAttachmentEntity struct {
	ID         int64     `db:"id"`
	EmailID    int64     `db:"email_id"`
	ExternalID string    `db:"external_id"` // Provider attachment ID (Gmail/Outlook)
	Filename   string    `db:"filename"`
	MimeType   string    `db:"mime_type"`
	Size       int64     `db:"size"`
	ContentID  *string   `db:"content_id"` // For inline attachments (CID)
	IsInline   bool      `db:"is_inline"`
	CreatedAt  time.Time `db:"created_at"`
}
