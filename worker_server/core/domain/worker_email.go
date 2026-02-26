package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Provider string

const (
	MailProviderGmail   Provider = "google" // DB enum: google, outlook
	MailProviderOutlook Provider = "outlook"
)

// LegacyFolder is the old folder type kept for backward compatibility
// Use SystemFolderKey from folder.go for new code
type LegacyFolder string

const (
	LegacyFolderInbox   LegacyFolder = "inbox"
	LegacyFolderSent    LegacyFolder = "sent"
	LegacyFolderDrafts  LegacyFolder = "drafts"
	LegacyFolderTrash   LegacyFolder = "trash"
	LegacyFolderSpam    LegacyFolder = "spam"
	LegacyFolderArchive LegacyFolder = "archive"
	LegacyFolderTodo    LegacyFolder = "todo"
)

// LegacyCategory is deprecated, use EmailCategory from classification.go instead
// Kept for backward compatibility during migration
type LegacyCategory string

const (
	LegacyCategoryPrimary   LegacyCategory = "primary"
	LegacyCategorySocial    LegacyCategory = "social"
	LegacyCategoryPromotion LegacyCategory = "promotion"
	LegacyCategoryUpdates   LegacyCategory = "updates"
	LegacyCategoryForums    LegacyCategory = "forums"
)

// Priority represents email priority as a score from 0.0 to 1.0
// Based on research from Gmail Priority Inbox, Superhuman, and SIGIR papers.
//
// Score ranges (Eisenhower Matrix inspired):
//   - 0.80 ~ 1.00: Urgent (requires immediate action)
//   - 0.60 ~ 0.79: High (important, should address soon)
//   - 0.40 ~ 0.59: Normal (relevant, worth reading)
//   - 0.20 ~ 0.39: Low (can be deferred)
//   - 0.00 ~ 0.19: Lowest (background noise)
type Priority float64

// Priority level constants (thresholds)
const (
	PriorityLowest Priority = 0.10 // 0.00 ~ 0.19
	PriorityLow    Priority = 0.30 // 0.20 ~ 0.39
	PriorityNormal Priority = 0.50 // 0.40 ~ 0.59
	PriorityHigh   Priority = 0.70 // 0.60 ~ 0.79
	PriorityUrgent Priority = 0.90 // 0.80 ~ 1.00
)

// String returns human-readable priority level
func (p Priority) String() string {
	switch {
	case p >= 0.80:
		return "urgent"
	case p >= 0.60:
		return "high"
	case p >= 0.40:
		return "normal"
	case p >= 0.20:
		return "low"
	default:
		return "lowest"
	}
}

// Level returns the priority level as a string (alias for String)
func (p Priority) Level() string {
	return p.String()
}

// UnmarshalJSON allows Priority to be unmarshaled from string, int, or float
func (p *Priority) UnmarshalJSON(data []byte) error {
	// Try float first (e.g., 0.75)
	var f float64
	if err := json.Unmarshal(data, &f); err == nil {
		*p = Priority(f)
		return nil
	}

	// Try string (e.g., "high", "urgent")
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*p = ParsePriority(s)
		return nil
	}

	// Default to normal
	*p = PriorityNormal
	return nil
}

// MarshalJSON outputs Priority as float for JSON
func (p Priority) MarshalJSON() ([]byte, error) {
	return json.Marshal(float64(p))
}

// ParsePriority converts string to Priority score
func ParsePriority(s string) Priority {
	switch s {
	case "lowest":
		return PriorityLowest
	case "low":
		return PriorityLow
	case "normal":
		return PriorityNormal
	case "high":
		return PriorityHigh
	case "urgent":
		return PriorityUrgent
	default:
		return PriorityNormal
	}
}

type WorkflowStatus string

const (
	WorkflowTodo    WorkflowStatus = "todo"
	WorkflowDone    WorkflowStatus = "done"
	WorkflowSnoozed WorkflowStatus = "snoozed"
)

type Email struct {
	ID           int64     `json:"id"`
	UserID       uuid.UUID `json:"user_id"`
	ConnectionID int64     `json:"connection_id"`
	Provider     Provider  `json:"provider"`
	AccountEmail string    `json:"account_email"` // OAuth account email
	ProviderID   string    `json:"provider_id"`
	ThreadID     string    `json:"thread_id"`

	// Headers
	Subject   string    `json:"subject"`
	FromEmail string    `json:"from_email"`
	FromName  *string   `json:"from_name,omitempty"`
	ToEmails  []string  `json:"to_emails"`
	CcEmails  []string  `json:"cc_emails,omitempty"`
	BccEmails []string  `json:"bcc_emails,omitempty"`
	ReplyTo   *string   `json:"reply_to,omitempty"`
	Date      time.Time `json:"date"`
	Snippet   string    `json:"snippet"` // 이메일 미리보기 텍스트

	// Folder & Labels
	Folder   LegacyFolder `json:"folder"`              // Legacy: system folder key
	FolderID *int64       `json:"folder_id,omitempty"` // New: reference to folders table
	Labels   []string     `json:"labels,omitempty"`

	// Flags
	IsRead    bool `json:"is_read"`
	IsStarred bool `json:"is_starred"`
	HasAttach bool `json:"has_attachments"`

	// AI Classification (updated to use new types)
	AICategory           *EmailCategory        `json:"ai_category,omitempty"`
	AISubCategory        *EmailSubCategory     `json:"ai_sub_category,omitempty"`
	AIPriority           *Priority             `json:"ai_priority,omitempty"`
	AISummary            *string               `json:"ai_summary,omitempty"`
	AITags               []string              `json:"ai_tags,omitempty"`
	AIScore              *float64              `json:"ai_score,omitempty"`
	ClassificationSource *ClassificationSource `json:"classification_source,omitempty"`

	// RFC Classification Headers (for Stage 0 classification)
	ClassificationHeaders *ClassificationHeaders `json:"classification_headers,omitempty"`

	// Workflow
	WorkflowStatus WorkflowStatus `json:"workflow_status"`
	SnoozedUntil   *time.Time     `json:"snoozed_until,omitempty"`

	// Timestamps
	ReceivedAt time.Time  `json:"received_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
}

type EmailBody struct {
	EmailID      int64         `json:"email_id"`
	TextBody     string        `json:"text_body"`
	HTMLBody     string        `json:"html_body"`
	IsCompressed bool          `json:"is_compressed"`
	Attachments  []*Attachment `json:"attachments,omitempty"` // 첨부파일 메타 포함
}

// Attachment represents an email attachment metadata
type Attachment struct {
	ID         int64  `json:"id"`
	EmailID    int64  `json:"email_id"`
	ExternalID string `json:"external_id"` // Provider attachment ID
	Filename   string `json:"filename"`
	MimeType   string `json:"mime_type"`
	Size       int64  `json:"size"`
	ContentID  string `json:"content_id,omitempty"` // For inline attachments
	IsInline   bool   `json:"is_inline"`
}

// AttachmentDownload represents attachment data for download
type AttachmentDownload struct {
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
	Data     []byte `json:"data"`
}

// EmailListItem is a lightweight DTO for list views (목록 조회용 경량 DTO)
// 전체 Email 대비 약 40% 크기 감소
type EmailListItem struct {
	ID            int64             `json:"id"`
	ConnectionID  int64             `json:"connection_id"`
	ProviderID    string            `json:"provider_id"`
	Subject       string            `json:"subject"`
	FromEmail     string            `json:"from_email"`
	FromName      *string           `json:"from_name,omitempty"`
	Snippet       string            `json:"snippet"`
	Folder        LegacyFolder      `json:"folder"`
	FolderID      *int64            `json:"folder_id,omitempty"`
	IsRead        bool              `json:"is_read"`
	IsStarred     bool              `json:"is_starred"`
	HasAttach     bool              `json:"has_attachments"`
	AICategory    *EmailCategory    `json:"ai_category,omitempty"`
	AISubCategory *EmailSubCategory `json:"ai_sub_category,omitempty"`
	AIPriority    *Priority         `json:"ai_priority,omitempty"`
	PriorityLevel *string           `json:"priority_level,omitempty"` // "urgent", "high", "normal", "low", "lowest"
	ReceivedAt    time.Time         `json:"received_at"`
}

// ToListItem converts Email to lightweight EmailListItem
func (e *Email) ToListItem() *EmailListItem {
	item := &EmailListItem{
		ID:            e.ID,
		ConnectionID:  e.ConnectionID,
		ProviderID:    e.ProviderID,
		Subject:       e.Subject,
		FromEmail:     e.FromEmail,
		FromName:      e.FromName,
		Snippet:       e.Snippet,
		Folder:        e.Folder,
		FolderID:      e.FolderID,
		IsRead:        e.IsRead,
		IsStarred:     e.IsStarred,
		HasAttach:     e.HasAttach,
		AICategory:    e.AICategory,
		AISubCategory: e.AISubCategory,
		AIPriority:    e.AIPriority,
		ReceivedAt:    e.ReceivedAt,
	}

	// Add priority level string for frontend convenience
	if e.AIPriority != nil {
		level := e.AIPriority.Level()
		item.PriorityLevel = &level
	}

	return item
}

// ToListItems converts slice of Email to EmailListItem
func ToListItems(emails []*Email) []*EmailListItem {
	items := make([]*EmailListItem, len(emails))
	for i, e := range emails {
		items[i] = e.ToListItem()
	}
	return items
}

type EmailFilter struct {
	UserID         uuid.UUID
	ConnectionID   *int64
	Folder         *LegacyFolder
	FolderID       *int64
	Category       *EmailCategory
	SubCategory    *EmailSubCategory
	Priority       *Priority
	MinPriority    *Priority // Minimum priority threshold (>= this value)
	IsRead         *bool
	IsStarred      *bool
	HasAttachment  *bool // has:attachment filter (Provider-compatible)
	Search         *string
	FromEmail      *string
	FromDomain     *string
	DateFrom       *time.Time
	DateTo         *time.Time
	LabelIDs       []int64
	WorkflowStatus *WorkflowStatus
	Limit          int
	Offset         int

	// === Inbox/Category View Filters ===
	// ViewType: predefined view filter
	// "inbox" = personal mail only (primary, work, personal) in inbox folder
	// "todo" = action required emails (high priority + unread)
	// "all" = all mail (default)
	ViewType *string

	// Categories: multiple category filter (OR logic)
	Categories []EmailCategory

	// SubCategories: multiple sub-category filter (OR logic)
	SubCategories []EmailSubCategory

	// ExcludeCategories: exclude specific categories
	ExcludeCategories []EmailCategory

	// === TODO View Filters ===
	// ActionRequired: filter emails that require user action
	// true = review_requested, mention, assign, approval_needed, etc.
	ActionRequired *bool

	// === Sorting ===
	// SortBy: "date" (default), "priority"
	// "priority" = ai_priority DESC, email_date DESC (TODO view)
	SortBy string
}

type EmailRepository interface {
	GetByID(id int64) (*Email, error)
	GetByProviderID(userID uuid.UUID, provider Provider, providerID string) (*Email, error)
	GetByThreadID(threadID string) ([]*Email, error)
	GetByDateRange(userID uuid.UUID, startDate, endDate time.Time) ([]*Email, error)
	List(filter *EmailFilter) ([]*Email, int, error)
	Create(email *Email) error
	CreateBatch(emails []*Email) error
	Update(email *Email) error
	Delete(id int64) error

	// Body operations (MongoDB)
	GetBody(emailID int64) (*EmailBody, error)
	SaveBody(body *EmailBody) error
}

// ScheduleSuggestion AI가 이메일에서 추출한 일정 제안
type ScheduleSuggestion struct {
	EmailID     int64     `json:"email_id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Location    string    `json:"location,omitempty"`
	Attendees   []string  `json:"attendees,omitempty"`
	Confidence  float64   `json:"confidence"`
	Source      string    `json:"source"` // 추출된 원본 텍스트
}

// ActionItem AI가 이메일에서 추출한 액션 아이템
type ActionItem struct {
	EmailID     int64      `json:"email_id"`
	Description string     `json:"description"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	Priority    Priority   `json:"priority"`
	Assignee    string     `json:"assignee,omitempty"`
	Status      string     `json:"status"` // pending, completed
	Confidence  float64    `json:"confidence"`
}

// RAGSearchResult represents a semantic search result
type RAGSearchResult struct {
	EmailID int64     `json:"email_id"`
	Subject string    `json:"subject"`
	From    string    `json:"from"`
	Date    time.Time `json:"date"`
	Snippet string    `json:"snippet"`
	Summary *string   `json:"summary,omitempty"`
	Score   float64   `json:"score"`
}

// RAGSearcher interface for semantic search
type RAGSearcher interface {
	SearchEmails(ctx interface{}, userID uuid.UUID, query string, limit int, includeSent, includeReceived bool) ([]*RAGSearchResult, error)
}
