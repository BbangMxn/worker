package in

import (
	"context"
	"time"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

type EmailService interface {
	// Email operations
	GetEmail(ctx context.Context, userID uuid.UUID, emailID int64) (*domain.Email, error)
	ListEmails(ctx context.Context, filter *domain.EmailFilter) ([]*domain.Email, int, error)
	GetEmailBody(ctx context.Context, emailID int64) (*domain.EmailBody, error)

	// Email actions (배치 지원)
	MarkAsRead(ctx context.Context, userID uuid.UUID, emailIDs []int64) error
	MarkAsUnread(ctx context.Context, userID uuid.UUID, emailIDs []int64) error
	Star(ctx context.Context, userID uuid.UUID, emailIDs []int64) error
	Unstar(ctx context.Context, userID uuid.UUID, emailIDs []int64) error
	Archive(ctx context.Context, userID uuid.UUID, emailIDs []int64) error
	Trash(ctx context.Context, userID uuid.UUID, emailIDs []int64) error
	Delete(ctx context.Context, userID uuid.UUID, emailIDs []int64) error
	MoveToFolder(ctx context.Context, userID uuid.UUID, emailIDs []int64, folder string) error
	Snooze(ctx context.Context, userID uuid.UUID, emailIDs []int64, until time.Time) error
	Unsnooze(ctx context.Context, userID uuid.UUID, emailIDs []int64) error
	UpdateWorkflowStatus(ctx context.Context, userID uuid.UUID, emailIDs []int64, status string) error

	// Send
	SendEmail(ctx context.Context, userID uuid.UUID, req *SendEmailRequest) (*domain.Email, error)
	ReplyEmail(ctx context.Context, userID uuid.UUID, emailID int64, req *ReplyEmailRequest) (*domain.Email, error)
	ForwardEmail(ctx context.Context, userID uuid.UUID, emailID int64, req *ForwardEmailRequest) (*domain.Email, error)

	// Sync
	SyncEmails(ctx context.Context, connectionID int64) error

	// Labels (배치 지원)
	AddLabels(ctx context.Context, userID uuid.UUID, emailID int64, labelIDs []int64) error
	RemoveLabels(ctx context.Context, userID uuid.UUID, emailID int64, labelIDs []int64) error
	BatchAddLabels(ctx context.Context, userID uuid.UUID, emailIDs []int64, labels []string) error
	BatchRemoveLabels(ctx context.Context, userID uuid.UUID, emailIDs []int64, labels []string) error
}

type SendEmailRequest struct {
	ConnectionID int64        `json:"connection_id,omitempty"` // For multi-account support
	To           []string     `json:"to"`
	Cc           []string     `json:"cc,omitempty"`
	Bcc          []string     `json:"bcc,omitempty"`
	Subject      string       `json:"subject"`
	Body         string       `json:"body"`
	IsHTML       bool         `json:"is_html"`
	Attachments  []Attachment `json:"attachments,omitempty"`
}

type ReplyEmailRequest struct {
	Body        string       `json:"body"`
	IsHTML      bool         `json:"is_html"`
	ReplyAll    bool         `json:"reply_all"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

type ForwardEmailRequest struct {
	To          []string     `json:"to"`
	Message     string       `json:"message,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Data        []byte `json:"data"`
}
