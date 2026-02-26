// Package out defines outbound ports (driven ports) for the application.
package out

import (
	"context"
	"time"

	"golang.org/x/oauth2"
)

// =============================================================================
// Mail Provider Port (Gmail, Outlook)
// =============================================================================

// EmailProviderPort defines the outbound port for external mail providers.
// 구현체: Gmail, Outlook 어댑터
type EmailProviderPort interface {
	// 기본 정보
	GetProviderType() string // "gmail", "outlook"

	// 인증
	MailAuthenticator

	// 동기화
	MailSyncer

	// 메시지 작업
	MailMessageReader
	MailMessageSender
	MailMessageModifier

	// 라벨/폴더
	MailLabelManager

	// 첨부파일
	MailAttachmentHandler

	// 프로필
	GetProfile(ctx context.Context, token *oauth2.Token) (*ProviderProfile, error)
}

// =============================================================================
// Sub-interfaces
// =============================================================================

// MailAuthenticator handles OAuth authentication.
type MailAuthenticator interface {
	GetAuthURL(state string) string
	ExchangeToken(ctx context.Context, code string) (*oauth2.Token, error)
	RefreshToken(ctx context.Context, token *oauth2.Token) (*oauth2.Token, error)
	ValidateToken(ctx context.Context, token *oauth2.Token) (bool, error)
}

// MailSyncer handles mail synchronization.
type MailSyncer interface {
	InitialSync(ctx context.Context, token *oauth2.Token, opts *ProviderSyncOptions) (*ProviderSyncResult, error)
	IncrementalSync(ctx context.Context, token *oauth2.Token, syncState string) (*ProviderSyncResult, error)
	Watch(ctx context.Context, token *oauth2.Token) (*ProviderWatchResponse, error)
	StopWatch(ctx context.Context, token *oauth2.Token) error
}

// MailMessageReader handles reading messages.
type MailMessageReader interface {
	GetMessage(ctx context.Context, token *oauth2.Token, externalID string) (*ProviderMailMessage, error)
	GetMessageBody(ctx context.Context, token *oauth2.Token, externalID string) (*ProviderMessageBody, error)
	ListMessages(ctx context.Context, token *oauth2.Token, opts *ProviderListOptions) (*ProviderListResult, error)
}

// MailMessageSender handles sending messages.
type MailMessageSender interface {
	Send(ctx context.Context, token *oauth2.Token, msg *ProviderOutgoingMessage) (*ProviderSendResult, error)
	Reply(ctx context.Context, token *oauth2.Token, replyToID string, msg *ProviderOutgoingMessage) (*ProviderSendResult, error)
	Forward(ctx context.Context, token *oauth2.Token, forwardID string, msg *ProviderOutgoingMessage) (*ProviderSendResult, error)
	CreateDraft(ctx context.Context, token *oauth2.Token, msg *ProviderOutgoingMessage) (*ProviderDraftResult, error)
	UpdateDraft(ctx context.Context, token *oauth2.Token, draftID string, msg *ProviderOutgoingMessage) (*ProviderDraftResult, error)
	DeleteDraft(ctx context.Context, token *oauth2.Token, draftID string) error
	SendDraft(ctx context.Context, token *oauth2.Token, draftID string) (*ProviderSendResult, error)
}

// MailMessageModifier handles modifying messages.
type MailMessageModifier interface {
	MarkAsRead(ctx context.Context, token *oauth2.Token, externalID string) error
	MarkAsUnread(ctx context.Context, token *oauth2.Token, externalID string) error
	Star(ctx context.Context, token *oauth2.Token, externalID string) error
	Unstar(ctx context.Context, token *oauth2.Token, externalID string) error
	Archive(ctx context.Context, token *oauth2.Token, externalID string) error
	Trash(ctx context.Context, token *oauth2.Token, externalID string) error
	Restore(ctx context.Context, token *oauth2.Token, externalID string) error
	Delete(ctx context.Context, token *oauth2.Token, externalID string) error
	BatchModify(ctx context.Context, token *oauth2.Token, req *ProviderBatchModifyRequest) error
}

// MailLabelManager handles label operations.
type MailLabelManager interface {
	ListLabels(ctx context.Context, token *oauth2.Token) ([]ProviderMailLabel, error)
	CreateLabel(ctx context.Context, token *oauth2.Token, name string, color *string) (*ProviderMailLabel, error)
	DeleteLabel(ctx context.Context, token *oauth2.Token, labelID string) error
	AddLabel(ctx context.Context, token *oauth2.Token, messageID, labelID string) error
	RemoveLabel(ctx context.Context, token *oauth2.Token, messageID, labelID string) error
}

// MailAttachmentHandler handles attachments.
type MailAttachmentHandler interface {
	GetAttachment(ctx context.Context, token *oauth2.Token, messageID, attachmentID string) ([]byte, string, error)
	StreamAttachment(ctx context.Context, token *oauth2.Token, messageID, attachmentID string) (*ProviderAttachmentStream, error)

	// Upload Session for large attachments (Provider API delegation)
	// Gmail: Resumable Upload (> 5MB), Outlook: Upload Session (> 3MB)
	CreateUploadSession(ctx context.Context, token *oauth2.Token, messageID string, attachment *UploadSessionRequest) (*UploadSessionResponse, error)
	GetUploadSessionStatus(ctx context.Context, token *oauth2.Token, sessionID string) (*UploadSessionStatus, error)
	CancelUploadSession(ctx context.Context, token *oauth2.Token, sessionID string) error
}

// UploadSessionRequest represents a request to create an upload session.
type UploadSessionRequest struct {
	Filename  string `json:"filename"`
	Size      int64  `json:"size"` // Total file size in bytes
	MimeType  string `json:"mime_type"`
	IsInline  bool   `json:"is_inline"`  // For inline attachments
	ContentID string `json:"content_id"` // CID for inline attachments
}

// UploadSessionResponse represents the response from creating an upload session.
type UploadSessionResponse struct {
	SessionID    string    `json:"session_id"`     // Internal session ID
	UploadURL    string    `json:"upload_url"`     // URL for direct upload to Provider
	ExpiresAt    time.Time `json:"expires_at"`     // Session expiration time
	ChunkSize    int64     `json:"chunk_size"`     // Recommended chunk size in bytes
	MaxChunkSize int64     `json:"max_chunk_size"` // Maximum chunk size
	Provider     string    `json:"provider"`       // "gmail" or "outlook"
}

// UploadSessionStatus represents the current status of an upload session.
type UploadSessionStatus struct {
	SessionID      string `json:"session_id"`
	BytesUploaded  int64  `json:"bytes_uploaded"`
	TotalBytes     int64  `json:"total_bytes"`
	IsComplete     bool   `json:"is_complete"`
	AttachmentID   string `json:"attachment_id,omitempty"` // Set when complete
	NextRangeStart int64  `json:"next_range_start"`        // Next byte to upload
}

// =============================================================================
// Provider Types
// =============================================================================

// ProviderSyncOptions represents sync options.
type ProviderSyncOptions struct {
	MaxResults   int
	StartDate    *time.Time
	Labels       []string
	IncludeSpam  bool
	IncludeTrash bool
	PageToken    string // 페이지네이션용
}

// ProviderSyncResult represents sync result.
type ProviderSyncResult struct {
	Messages      []ProviderMailMessage
	DeletedIDs    []string
	NextSyncState string // History ID for delta sync
	NextPageToken string // 다음 페이지 토큰 (Progressive Loading용)
	HasMore       bool
}

// ProviderWatchResponse represents push notification subscription.
type ProviderWatchResponse struct {
	ExternalID string
	Expiration time.Time
	TopicName  string
}

// ProviderListOptions represents list query options.
type ProviderListOptions struct {
	Query      string
	Labels     []string
	MaxResults int
	PageToken  string
}

// ProviderListResult represents list result.
type ProviderListResult struct {
	Messages      []ProviderMailMessage
	NextPageToken string
	TotalCount    int64
}

// ProviderMailMessage represents a mail message from provider.
type ProviderMailMessage struct {
	ExternalID       string
	ExternalThreadID string
	MessageID        string
	InReplyTo        string
	References       string

	Subject    string
	Snippet    string
	From       ProviderEmailAddress
	FromAvatar string // 발신자 프로필 사진 URL
	To         []ProviderEmailAddress
	CC         []ProviderEmailAddress
	BCC        []ProviderEmailAddress

	Date          time.Time
	ReceivedAt    time.Time
	IsRead        bool
	IsStarred     bool
	Labels        []string
	Folder        string
	HasAttachment bool
	Attachments   []ProviderMailAttachment
	Size          int64

	// RFC Classification Headers (for Stage 0 classification)
	ClassificationHeaders *ProviderClassificationHeaders `json:"classification_headers,omitempty"`
}

// ProviderClassificationHeaders contains RFC headers for email classification.
type ProviderClassificationHeaders struct {
	// Mailing List Headers (RFC 2369, 2919)
	ListUnsubscribe     string `json:"list_unsubscribe,omitempty"`
	ListUnsubscribePost string `json:"list_unsubscribe_post,omitempty"`
	ListID              string `json:"list_id,omitempty"`

	// Auto/Bulk Headers (RFC 3834)
	Precedence           string `json:"precedence,omitempty"`
	AutoSubmitted        string `json:"auto_submitted,omitempty"`
	AutoResponseSuppress string `json:"auto_response_suppress,omitempty"`

	// Mailer Info
	XMailer    string `json:"x_mailer,omitempty"`
	FeedbackID string `json:"feedback_id,omitempty"`

	// ESP Detection Flags
	IsMailchimp bool `json:"is_mailchimp,omitempty"`
	IsSendGrid  bool `json:"is_sendgrid,omitempty"`
	IsAmazonSES bool `json:"is_amazon_ses,omitempty"`
	IsMailgun   bool `json:"is_mailgun,omitempty"`
	IsPostmark  bool `json:"is_postmark,omitempty"`
	IsCampaign  bool `json:"is_campaign,omitempty"`

	// === Developer Service Headers ===

	// GitHub Headers
	XGitHubReason   string `json:"x_github_reason,omitempty"`   // review_requested, author, mention, security_alert, etc.
	XGitHubSeverity string `json:"x_github_severity,omitempty"` // critical, high, moderate, low (Dependabot)
	XGitHubSender   string `json:"x_github_sender,omitempty"`   // GitHub username

	// GitLab Headers
	XGitLabProject            string `json:"x_gitlab_project,omitempty"`
	XGitLabPipelineID         string `json:"x_gitlab_pipeline_id,omitempty"`
	XGitLabNotificationReason string `json:"x_gitlab_notification_reason,omitempty"`

	// Jira/Atlassian Headers
	XJIRAFingerprint string `json:"x_jira_fingerprint,omitempty"`

	// Linear Headers
	XLinearTeam    string `json:"x_linear_team,omitempty"`
	XLinearProject string `json:"x_linear_project,omitempty"`

	// Sentry Headers
	XSentryProject string `json:"x_sentry_project,omitempty"`

	// Vercel Headers
	XVercelDeploymentURL string `json:"x_vercel_deployment_url,omitempty"`

	// AWS Headers
	XAWSService string `json:"x_aws_service,omitempty"`

	// CC Address (for GitHub notification type detection)
	CCAddresses []string `json:"cc_addresses,omitempty"`
}

// ProviderMessageBody represents message body.
type ProviderMessageBody struct {
	Text        string
	HTML        string
	Attachments []ProviderMailAttachment
}

// ProviderEmailAddress represents an email address.
type ProviderEmailAddress struct {
	Name  string
	Email string
}

// ProviderMailAttachment represents an attachment.
type ProviderMailAttachment struct {
	ID        string
	Filename  string
	MimeType  string
	Size      int64
	ContentID string // For inline attachments (CID)
	IsInline  bool   // True if embedded in email body
}

// ProviderAttachmentStream represents attachment stream.
type ProviderAttachmentStream struct {
	Reader   interface{}
	Size     int64
	MimeType string
	Filename string
}

// ProviderOutgoingMessage represents outgoing message.
type ProviderOutgoingMessage struct {
	To      []ProviderEmailAddress
	CC      []ProviderEmailAddress
	BCC     []ProviderEmailAddress
	Subject string
	Body    string
	IsHTML  bool

	Attachments []ProviderOutgoingAttachment

	InReplyTo  string
	References string
	ThreadID   string
}

// ProviderOutgoingAttachment represents outgoing attachment.
type ProviderOutgoingAttachment struct {
	Filename string
	MimeType string
	Data     []byte
}

// ProviderSendResult represents send result.
type ProviderSendResult struct {
	ExternalID       string
	ExternalThreadID string
	SentAt           time.Time
}

// ProviderDraftResult represents draft result.
type ProviderDraftResult struct {
	ExternalID string
}

// ProviderBatchModifyRequest represents batch modify request.
type ProviderBatchModifyRequest struct {
	IDs          []string
	AddLabels    []string
	RemoveLabels []string
}

// ProviderMailLabel represents a mail label.
type ProviderMailLabel struct {
	ExternalID     string
	Name           string
	Type           string
	Color          *string
	MessagesTotal  int64
	MessagesUnread int64
}

// ProviderProfile represents user profile.
type ProviderProfile struct {
	Email     string
	Name      string
	Picture   string
	HistoryID uint64
}

// =============================================================================
// Provider Error
// =============================================================================

// ProviderErrorCode represents error codes.
type ProviderErrorCode string

const (
	ProviderErrAuth         ProviderErrorCode = "auth_error"
	ProviderErrTokenExpired ProviderErrorCode = "token_expired"
	ProviderErrRateLimit    ProviderErrorCode = "rate_limit"
	ProviderErrNotFound     ProviderErrorCode = "not_found"
	ProviderErrNetwork      ProviderErrorCode = "network_error"
	ProviderErrServer       ProviderErrorCode = "server_error"
	ProviderErrInvalidInput ProviderErrorCode = "invalid_input"
	ProviderErrSyncRequired ProviderErrorCode = "full_sync_required"
)

// ProviderError represents a provider error.
type ProviderError struct {
	Provider  string
	Code      ProviderErrorCode
	Message   string
	Err       error
	Retryable bool
}

func (e *ProviderError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

// NewProviderError creates a new provider error.
func NewProviderError(provider string, code ProviderErrorCode, message string, err error, retryable bool) *ProviderError {
	return &ProviderError{
		Provider:  provider,
		Code:      code,
		Message:   message,
		Err:       err,
		Retryable: retryable,
	}
}

// =============================================================================
// Provider Factory
// =============================================================================

// MailProviderFactory creates mail providers.
type MailProviderFactory interface {
	CreateProvider(ctx context.Context, providerType string, token *oauth2.Token) (EmailProviderPort, error)
	CreateProviderFromConnection(ctx context.Context, connectionID int64) (EmailProviderPort, error)
}
