package mail

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/in"
	"worker_server/core/port/out"
	"worker_server/core/service/auth"
	"worker_server/core/service/common"
	"worker_server/pkg/logger"

	"github.com/google/uuid"
)

var (
	ErrRepoNotInitialized = errors.New("repository not initialized")
	ErrEmailNotFound      = errors.New("email not found")
)

type Service struct {
	domainRepo      domain.EmailRepository
	labelRepo       domain.LabelRepository
	emailRepo        out.EmailRepository   // for batch operations
	cacheService    *common.CacheService // optional caching
	provider        out.EmailProviderPort // for sending
	oauthService    *auth.OAuthService   // for token management
	messageProducer out.MessageProducer  // for async provider sync + SSE broadcast via Worker
}

func NewService(domainRepo domain.EmailRepository, labelRepo domain.LabelRepository) *Service {
	return &Service{
		domainRepo: domainRepo,
		labelRepo: labelRepo,
	}
}

// NewServiceFull creates a mail service with all dependencies
func NewServiceFull(
	domainRepo domain.EmailRepository,
	labelRepo domain.LabelRepository,
	emailRepo out.EmailRepository,
	cacheService *common.CacheService,
	provider out.EmailProviderPort,
	oauthService *auth.OAuthService,
	messageProducer out.MessageProducer,
) *Service {
	return &Service{
		domainRepo:      domainRepo,
		labelRepo:       labelRepo,
		emailRepo:        emailRepo,
		cacheService:    cacheService,
		provider:        provider,
		oauthService:    oauthService,
		messageProducer: messageProducer,
	}
}

func (s *Service) GetEmail(ctx context.Context, userID uuid.UUID, emailID int64) (*domain.Email, error) {
	if s.domainRepo == nil {
		return nil, ErrRepoNotInitialized
	}
	email, err := s.domainRepo.GetByID(emailID)
	if err != nil {
		return nil, err
	}
	if email.UserID != userID {
		return nil, common.ErrForbidden
	}
	return email, nil
}

func (s *Service) ListEmails(ctx context.Context, filter *domain.EmailFilter) ([]*domain.Email, int, error) {
	if s.domainRepo == nil {
		return []*domain.Email{}, 0, nil
	}
	return s.domainRepo.List(filter)
}

// ListEmailsHybrid - DB 조회 후 부족하면 Gmail API에서 추가 로딩 (하이브리드 방식)
func (s *Service) ListEmailsHybrid(ctx context.Context, filter *domain.EmailFilter) ([]*domain.Email, int, bool, error) {
	if s.domainRepo == nil {
		return []*domain.Email{}, 0, false, nil
	}

	// 1. DB에서 먼저 조회
	emails, total, err := s.domainRepo.List(filter)
	if err != nil {
		return nil, 0, false, err
	}

	// 2. 요청한 개수만큼 있으면 그대로 반환
	if len(emails) >= filter.Limit {
		return emails, total, false, nil
	}

	// 3. DB에 부족하면 Gmail API에서 추가 로딩
	if s.provider == nil || s.oauthService == nil {
		return emails, total, false, nil
	}

	// ConnectionID 필요
	if filter.ConnectionID == nil || *filter.ConnectionID == 0 {
		return emails, total, false, nil
	}

	// 추가로 필요한 개수 계산
	needed := filter.Limit - len(emails)
	if needed <= 0 {
		return emails, total, false, nil
	}

	// API에서 추가 로딩
	connectionID := *filter.ConnectionID
	moreEmails, hasMore, err := s.fetchFromProvider(ctx, filter.UserID, connectionID, filter.Folder, needed)
	if err != nil {
		// API 실패해도 DB 결과는 반환
		return emails, total, false, nil
	}

	// DB 결과와 API 결과 합치기
	emails = append(emails, moreEmails...)
	return emails, total + len(moreEmails), hasMore, nil
}

// fetchFromProvider - Gmail API에서 메일 가져와서 DB에 저장
func (s *Service) fetchFromProvider(ctx context.Context, userID uuid.UUID, connectionID int64, folder *domain.LegacyFolder, limit int) ([]*domain.Email, bool, error) {
	// OAuth 토큰 가져오기
	token, err := s.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get token: %w", err)
	}

	// Connection 정보
	conn, err := s.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get connection: %w", err)
	}

	// Gmail API 호출
	opts := &out.ProviderListOptions{
		MaxResults: limit,
	}

	// 폴더 필터 적용
	if folder != nil && *folder != "" {
		opts.Labels = []string{s.folderToGmailLabel(string(*folder))}
	}

	result, err := s.provider.ListMessages(ctx, token, opts)
	if err != nil {
		return nil, false, fmt.Errorf("failed to list messages: %w", err)
	}

	// 메시지를 도메인 객체로 변환 후 DB 저장
	var emails []*domain.Email
	for _, msg := range result.Messages {
		// 이미 DB에 있는지 확인
		if s.emailRepo != nil {
			existing, _ := s.emailRepo.GetByExternalID(ctx, connectionID, msg.ExternalID)
			if existing != nil {
				continue // 이미 있으면 스킵
			}
		}

		email := s.convertProviderMessage(msg, userID, connectionID, conn.Email)

		// DB에 저장
		if s.emailRepo != nil {
			entity := s.domainToEntity(email)
			if err := s.emailRepo.Create(ctx, entity); err == nil {
				email.ID = entity.ID
			}
		}

		emails = append(emails, email)
	}

	hasMore := result.NextPageToken != ""
	return emails, hasMore, nil
}

// folderToGmailLabel - 폴더명을 Gmail 라벨로 변환
func (s *Service) folderToGmailLabel(folder string) string {
	switch folder {
	case "inbox":
		return "INBOX"
	case "sent":
		return "SENT"
	case "drafts":
		return "DRAFT"
	case "trash":
		return "TRASH"
	case "spam":
		return "SPAM"
	case "starred":
		return "STARRED"
	default:
		return "INBOX"
	}
}

// convertProviderMessage - Provider 메시지를 도메인 객체로 변환
func (s *Service) convertProviderMessage(msg out.ProviderMailMessage, userID uuid.UUID, connectionID int64, accountEmail string) *domain.Email {
	var fromName *string
	if msg.From.Name != "" {
		fromName = &msg.From.Name
	}

	email := &domain.Email{
		UserID:       userID,
		ConnectionID: connectionID,
		Provider:     domain.MailProviderGmail,
		ProviderID:   msg.ExternalID,
		ThreadID:     msg.ExternalThreadID,
		Subject:      msg.Subject,
		FromEmail:    msg.From.Email,
		FromName:     fromName,
		IsRead:       msg.IsRead,
		HasAttach:    msg.HasAttachment,
		Folder:       domain.LegacyFolder(msg.Folder),
		Labels:       msg.Labels,
		ReceivedAt:   msg.ReceivedAt,
		Date:         msg.Date,
	}

	for _, to := range msg.To {
		email.ToEmails = append(email.ToEmails, to.Email)
	}
	for _, cc := range msg.CC {
		email.CcEmails = append(email.CcEmails, cc.Email)
	}

	return email
}

// domainToEntity - 도메인 객체를 엔티티로 변환
func (s *Service) domainToEntity(d *domain.Email) *out.MailEntity {
	var fromName string
	if d.FromName != nil {
		fromName = *d.FromName
	}

	return &out.MailEntity{
		UserID:         d.UserID,
		ConnectionID:   d.ConnectionID,
		Provider:       string(d.Provider),
		ExternalID:     d.ProviderID,
		FromEmail:      d.FromEmail,
		FromName:       fromName,
		ToEmails:       d.ToEmails,
		CcEmails:       d.CcEmails,
		Subject:        d.Subject,
		IsRead:         d.IsRead,
		Folder:         string(d.Folder),
		Labels:         d.Labels,
		ReceivedAt:     d.ReceivedAt,
		AIStatus:       "none",
		WorkflowStatus: "none",
	}
}

func (s *Service) GetEmailBody(ctx context.Context, emailID int64) (*domain.EmailBody, error) {
	// CacheService가 있으면 3-tier 캐싱 사용 (Redis → MongoDB → Provider)
	if s.cacheService != nil && s.emailRepo != nil {
		// connectionID를 가져오기 위해 이메일 조회
		email, err := s.emailRepo.GetByID(ctx, emailID)
		if err != nil {
			return nil, err
		}
		if email == nil {
			return nil, ErrEmailNotFound
		}
		return s.cacheService.GetBody(ctx, emailID, email.ConnectionID)
	}

	// Fallback: MongoDB만 사용 (Provider fallback 없음)
	if s.domainRepo == nil {
		return nil, ErrRepoNotInitialized
	}
	return s.domainRepo.GetBody(emailID)
}

func (s *Service) MarkAsRead(ctx context.Context, userID uuid.UUID, emailIDs []int64) error {
	if s.domainRepo == nil {
		return ErrRepoNotInitialized
	}

	// Try batch update first (optimized path)
	if s.emailRepo != nil {
		if err := s.emailRepo.BatchUpdateReadStatus(ctx, emailIDs, true); err == nil {
			// Cache invalidation is handled by HTTP handler (optimistic patch)
			// Publish job for async provider sync + SSE broadcast (via Worker)
			go s.publishMailModifyJob(context.Background(), userID, emailIDs, "read")
			return nil
		}
	}

	// Fallback: individual updates with concurrency
	return s.batchUpdateEmails(ctx, userID, emailIDs, func(email *domain.Email) {
		email.IsRead = true
	})
}

func (s *Service) MarkAsUnread(ctx context.Context, userID uuid.UUID, emailIDs []int64) error {
	if s.domainRepo == nil {
		return ErrRepoNotInitialized
	}

	if s.emailRepo != nil {
		if err := s.emailRepo.BatchUpdateReadStatus(ctx, emailIDs, false); err == nil {
			// Cache invalidation is handled by HTTP handler (optimistic patch)
			go s.publishMailModifyJob(context.Background(), userID, emailIDs, "unread")
			return nil
		}
	}

	return s.batchUpdateEmails(ctx, userID, emailIDs, func(email *domain.Email) {
		email.IsRead = false
	})
}

func (s *Service) Star(ctx context.Context, userID uuid.UUID, emailIDs []int64) error {
	if s.domainRepo == nil {
		return ErrRepoNotInitialized
	}

	// Use BatchUpdateTags to add "starred" tag
	if s.emailRepo != nil {
		if err := s.emailRepo.BatchUpdateTags(ctx, emailIDs, []string{"starred"}, nil); err == nil {
			// Cache invalidation is handled by HTTP handler (optimistic patch)
			go s.publishMailModifyJob(context.Background(), userID, emailIDs, "star")
			return nil
		}
	}

	return s.batchUpdateEmails(ctx, userID, emailIDs, func(email *domain.Email) {
		email.IsStarred = true
	})
}

func (s *Service) Unstar(ctx context.Context, userID uuid.UUID, emailIDs []int64) error {
	if s.domainRepo == nil {
		return ErrRepoNotInitialized
	}

	// Use BatchUpdateTags to remove "starred" tag
	if s.emailRepo != nil {
		if err := s.emailRepo.BatchUpdateTags(ctx, emailIDs, nil, []string{"starred"}); err == nil {
			// Cache invalidation is handled by HTTP handler (optimistic patch)
			go s.publishMailModifyJob(context.Background(), userID, emailIDs, "unstar")
			return nil
		}
	}

	return s.batchUpdateEmails(ctx, userID, emailIDs, func(email *domain.Email) {
		email.IsStarred = false
	})
}

func (s *Service) Archive(ctx context.Context, userID uuid.UUID, emailIDs []int64) error {
	if s.domainRepo == nil {
		return ErrRepoNotInitialized
	}

	if s.emailRepo != nil {
		if err := s.emailRepo.BatchUpdateFolder(ctx, emailIDs, string(domain.LegacyFolderArchive)); err == nil {
			// Cache invalidation is handled by HTTP handler (optimistic patch)
			go s.publishMailModifyJob(context.Background(), userID, emailIDs, "archive")
			return nil
		}
	}

	return s.batchUpdateEmails(ctx, userID, emailIDs, func(email *domain.Email) {
		email.Folder = domain.LegacyFolderArchive
	})
}

func (s *Service) Trash(ctx context.Context, userID uuid.UUID, emailIDs []int64) error {
	if s.domainRepo == nil {
		return ErrRepoNotInitialized
	}

	if s.emailRepo != nil {
		if err := s.emailRepo.BatchUpdateFolder(ctx, emailIDs, string(domain.LegacyFolderTrash)); err == nil {
			// Cache invalidation is handled by HTTP handler (optimistic patch)
			go s.publishMailModifyJob(context.Background(), userID, emailIDs, "trash")
			return nil
		}
	}

	return s.batchUpdateEmails(ctx, userID, emailIDs, func(email *domain.Email) {
		email.Folder = domain.LegacyFolderTrash
	})
}

// Delete permanently deletes emails (배치 지원)
func (s *Service) Delete(ctx context.Context, userID uuid.UUID, emailIDs []int64) error {
	if s.domainRepo == nil {
		return ErrRepoNotInitialized
	}

	if s.emailRepo != nil {
		if err := s.emailRepo.BatchDelete(ctx, emailIDs); err == nil {
			// Cache invalidation is handled by HTTP handler (optimistic patch)
			go s.publishMailModifyJob(context.Background(), userID, emailIDs, "delete")
			return nil
		}
	}

	// Fallback: individual deletes
	for _, id := range emailIDs {
		email, err := s.domainRepo.GetByID(id)
		if err != nil {
			continue
		}
		if email.UserID != userID {
			continue
		}
		if err := s.domainRepo.Delete(id); err != nil {
			return err
		}
	}
	return nil
}

// MoveToFolder moves emails to a specific folder (배치 지원)
func (s *Service) MoveToFolder(ctx context.Context, userID uuid.UUID, emailIDs []int64, folder string) error {
	if s.domainRepo == nil {
		return ErrRepoNotInitialized
	}

	if s.emailRepo != nil {
		if err := s.emailRepo.BatchUpdateFolder(ctx, emailIDs, folder); err == nil {
			// Cache invalidation is handled by HTTP handler (optimistic patch)
			go s.publishMailModifyJob(context.Background(), userID, emailIDs, "move:"+folder)
			return nil
		}
	}

	return s.batchUpdateEmails(ctx, userID, emailIDs, func(email *domain.Email) {
		email.Folder = domain.LegacyFolder(folder)
	})
}

// Snooze snoozes emails until a specific time (배치 지원)
func (s *Service) Snooze(ctx context.Context, userID uuid.UUID, emailIDs []int64, until time.Time) error {
	if s.domainRepo == nil {
		return ErrRepoNotInitialized
	}

	if s.emailRepo != nil {
		// 단일 쿼리로 배치 업데이트
		if err := s.emailRepo.BatchUpdateWorkflowStatus(ctx, emailIDs, "snoozed", &until); err != nil {
			return err
		}
		// Cache invalidation is handled by HTTP handler
		go s.publishMailModifyJob(context.Background(), userID, emailIDs, "snooze")
		return nil
	}

	return s.batchUpdateEmails(ctx, userID, emailIDs, func(email *domain.Email) {
		email.WorkflowStatus = domain.WorkflowSnoozed
		email.SnoozedUntil = &until
	})
}

// Unsnooze removes snooze from emails (배치 지원)
func (s *Service) Unsnooze(ctx context.Context, userID uuid.UUID, emailIDs []int64) error {
	if s.domainRepo == nil {
		return ErrRepoNotInitialized
	}

	if s.emailRepo != nil {
		// 단일 쿼리로 배치 업데이트
		if err := s.emailRepo.BatchUpdateWorkflowStatus(ctx, emailIDs, "none", nil); err != nil {
			return err
		}
		// Cache invalidation is handled by HTTP handler
		go s.publishMailModifyJob(context.Background(), userID, emailIDs, "unsnooze")
		return nil
	}

	return s.batchUpdateEmails(ctx, userID, emailIDs, func(email *domain.Email) {
		email.WorkflowStatus = domain.WorkflowTodo
		email.SnoozedUntil = nil
	})
}


// UpdateWorkflowStatus changes the workflow status of emails (todo, done, none)
func (s *Service) UpdateWorkflowStatus(ctx context.Context, userID uuid.UUID, emailIDs []int64, status string) error {
	if s.emailRepo == nil {
		return ErrRepoNotInitialized
	}

	// Validate status
	validStatuses := map[string]bool{"todo": true, "done": true, "none": true, "": true}
	if !validStatuses[status] {
		return fmt.Errorf("invalid workflow status: %s", status)
	}

	if err := s.emailRepo.BatchUpdateWorkflowStatus(ctx, emailIDs, status, nil); err != nil {
		return err
	}

	// Async publish for provider sync
	go s.publishMailModifyJob(context.Background(), userID, emailIDs, "workflow")
	return nil
}
// BatchAddLabels adds labels to multiple emails (배치 지원)
func (s *Service) BatchAddLabels(ctx context.Context, userID uuid.UUID, emailIDs []int64, labels []string) error {
	if s.emailRepo == nil {
		return ErrRepoNotInitialized
	}

	if err := s.emailRepo.BatchUpdateTags(ctx, emailIDs, labels, nil); err != nil {
		return err
	}

	// Cache invalidation is handled by HTTP handler
	s.publishMailModifyJobWithLabels(ctx, userID, emailIDs, labels, nil)
	return nil
}

// BatchRemoveLabels removes labels from multiple emails (배치 지원)
func (s *Service) BatchRemoveLabels(ctx context.Context, userID uuid.UUID, emailIDs []int64, labels []string) error {
	if s.emailRepo == nil {
		return ErrRepoNotInitialized
	}

	if err := s.emailRepo.BatchUpdateTags(ctx, emailIDs, nil, labels); err != nil {
		return err
	}

	// Cache invalidation is handled by HTTP handler
	s.publishMailModifyJobWithLabels(ctx, userID, emailIDs, nil, labels)
	return nil
}

func (s *Service) SendEmail(ctx context.Context, userID uuid.UUID, req *in.SendEmailRequest) (*domain.Email, error) {
	if s.provider == nil || s.oauthService == nil {
		return nil, errors.New("mail provider or oauth service not configured")
	}

	// Get OAuth connection
	var conn *domain.OAuthConnection
	var err error

	if req.ConnectionID > 0 {
		conn, err = s.oauthService.GetConnection(ctx, req.ConnectionID)
	} else {
		conn, err = s.oauthService.GetConnectionByUserID(ctx, userID, "google")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := s.oauthService.GetOAuth2Token(ctx, conn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Build outgoing message
	outgoing := &out.ProviderOutgoingMessage{
		Subject: req.Subject,
		Body:    req.Body,
		IsHTML:  req.IsHTML,
	}

	for _, addr := range req.To {
		outgoing.To = append(outgoing.To, out.ProviderEmailAddress{Email: addr})
	}
	for _, addr := range req.Cc {
		outgoing.CC = append(outgoing.CC, out.ProviderEmailAddress{Email: addr})
	}
	for _, addr := range req.Bcc {
		outgoing.BCC = append(outgoing.BCC, out.ProviderEmailAddress{Email: addr})
	}

	// Add attachments
	for _, att := range req.Attachments {
		outgoing.Attachments = append(outgoing.Attachments, out.ProviderOutgoingAttachment{
			Filename: att.Filename,
			MimeType: att.ContentType,
			Data:     att.Data,
		})
	}

	// Send email
	result, err := s.provider.Send(ctx, token, outgoing)
	if err != nil {
		return nil, fmt.Errorf("failed to send email: %w", err)
	}

	// Return domain email (without persisting - will be synced later)
	return &domain.Email{
		ProviderID: result.ExternalID,
		Subject:    req.Subject,
		ToEmails:   req.To,
		FromEmail:  conn.Email,
		Date:       result.SentAt,
	}, nil
}

func (s *Service) ReplyEmail(ctx context.Context, userID uuid.UUID, emailID int64, req *in.ReplyEmailRequest) (*domain.Email, error) {
	if s.provider == nil || s.oauthService == nil {
		return nil, errors.New("mail provider or oauth service not configured")
	}

	// Get original email
	if s.domainRepo == nil {
		return nil, ErrRepoNotInitialized
	}

	original, err := s.domainRepo.GetByID(emailID)
	if err != nil {
		return nil, fmt.Errorf("failed to get original email: %w", err)
	}

	// Get OAuth connection from the original email's connection
	conn, err := s.oauthService.GetConnection(ctx, original.ConnectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := s.oauthService.GetOAuth2Token(ctx, conn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Build reply message
	outgoing := &out.ProviderOutgoingMessage{
		Subject: "Re: " + original.Subject,
		Body:    req.Body,
		IsHTML:  req.IsHTML,
	}

	// Set recipients based on ReplyAll flag
	outgoing.To = append(outgoing.To, out.ProviderEmailAddress{Email: original.FromEmail})

	if req.ReplyAll {
		// Add all original recipients except self
		for _, to := range original.ToEmails {
			if to != conn.Email {
				outgoing.CC = append(outgoing.CC, out.ProviderEmailAddress{Email: to})
			}
		}
	}

	// Add attachments
	for _, att := range req.Attachments {
		outgoing.Attachments = append(outgoing.Attachments, out.ProviderOutgoingAttachment{
			Filename: att.Filename,
			MimeType: att.ContentType,
			Data:     att.Data,
		})
	}

	// Send reply
	result, err := s.provider.Reply(ctx, token, original.ProviderID, outgoing)
	if err != nil {
		return nil, fmt.Errorf("failed to send reply: %w", err)
	}

	return &domain.Email{
		ProviderID: result.ExternalID,
		Subject:    outgoing.Subject,
		ToEmails:   []string{original.FromEmail},
		FromEmail:  conn.Email,
		Date:       result.SentAt,
	}, nil
}

func (s *Service) ForwardEmail(ctx context.Context, userID uuid.UUID, emailID int64, req *in.ForwardEmailRequest) (*domain.Email, error) {
	if s.provider == nil || s.oauthService == nil {
		return nil, errors.New("mail provider or oauth service not configured")
	}

	// Get original email
	if s.domainRepo == nil {
		return nil, ErrRepoNotInitialized
	}

	original, err := s.domainRepo.GetByID(emailID)
	if err != nil {
		return nil, fmt.Errorf("failed to get original email: %w", err)
	}

	// Get OAuth connection
	conn, err := s.oauthService.GetConnection(ctx, original.ConnectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Get OAuth token
	token, err := s.oauthService.GetOAuth2Token(ctx, conn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Build forward message
	body := req.Message
	if body != "" {
		body += "\n\n---------- Forwarded message ----------\n"
	}
	body += fmt.Sprintf("From: %s\nSubject: %s\n\n", original.FromEmail, original.Subject)

	outgoing := &out.ProviderOutgoingMessage{
		Subject: "Fwd: " + original.Subject,
		Body:    body,
		IsHTML:  false,
	}

	for _, addr := range req.To {
		outgoing.To = append(outgoing.To, out.ProviderEmailAddress{Email: addr})
	}

	// Add attachments
	for _, att := range req.Attachments {
		outgoing.Attachments = append(outgoing.Attachments, out.ProviderOutgoingAttachment{
			Filename: att.Filename,
			MimeType: att.ContentType,
			Data:     att.Data,
		})
	}

	// Send forward
	result, err := s.provider.Send(ctx, token, outgoing)
	if err != nil {
		return nil, fmt.Errorf("failed to forward email: %w", err)
	}

	return &domain.Email{
		ProviderID: result.ExternalID,
		Subject:    outgoing.Subject,
		ToEmails:   req.To,
		FromEmail:  conn.Email,
		Date:       result.SentAt,
	}, nil
}

func (s *Service) SyncEmails(ctx context.Context, connectionID int64) error {
	// TODO: Implement via provider
	return nil
}

func (s *Service) AddLabels(ctx context.Context, userID uuid.UUID, emailID int64, labelIDs []int64) error {
	if s.domainRepo == nil || s.labelRepo == nil {
		return ErrRepoNotInitialized
	}
	email, err := s.domainRepo.GetByID(emailID)
	if err != nil {
		return err
	}
	if email.UserID != userID {
		return common.ErrForbidden
	}

	for _, labelID := range labelIDs {
		if err := s.labelRepo.AddEmailLabel(emailID, labelID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) RemoveLabels(ctx context.Context, userID uuid.UUID, emailID int64, labelIDs []int64) error {
	if s.domainRepo == nil || s.labelRepo == nil {
		return ErrRepoNotInitialized
	}
	email, err := s.domainRepo.GetByID(emailID)
	if err != nil {
		return err
	}
	if email.UserID != userID {
		return common.ErrForbidden
	}

	for _, labelID := range labelIDs {
		if err := s.labelRepo.RemoveEmailLabel(emailID, labelID); err != nil {
			return err
		}
	}
	return nil
}

// =============================================================================
// Batch Update Helpers
// =============================================================================

// batchUpdateEmails updates multiple emails concurrently with a modifier function
func (s *Service) batchUpdateEmails(ctx context.Context, userID uuid.UUID, emailIDs []int64, modifier func(*domain.Email)) error {
	if len(emailIDs) == 0 {
		return nil
	}

	// Use concurrency for batch updates
	const maxConcurrency = 10
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	errCh := make(chan error, len(emailIDs))

	for _, id := range emailIDs {
		wg.Add(1)
		go func(emailID int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			email, err := s.domainRepo.GetByID(emailID)
			if err != nil {
				errCh <- fmt.Errorf("get email %d: %w", emailID, err)
				return
			}
			if email.UserID != userID {
				return // Skip unauthorized
			}

			modifier(email)

			if err := s.domainRepo.Update(email); err != nil {
				errCh <- fmt.Errorf("update email %d: %w", emailID, err)
			}
		}(id)
	}

	wg.Wait()
	close(errCh)

	// Collect errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("batch update failed with %d errors: %v", len(errs), errs[0])
	}

	return nil
}

// invalidateEmailCache is deprecated - cache invalidation is now handled by HTTP handler
// using optimistic patch (PatchReadStatus, PatchStarStatus, PatchFolder, RemoveFromCache)
// This function is kept for backward compatibility but does nothing.
func (s *Service) invalidateEmailCache(ctx context.Context, userID uuid.UUID, emailIDs []int64) {
	// DEPRECATED: Cache invalidation moved to HTTP handler for optimistic updates
	// The handler uses EmailListCache.Patch*() methods instead of full invalidation
	// See: adapter/in/http/mail.go
}

// publishMailModifyJob publishes a mail modify job for async provider sync + SSE broadcast
func (s *Service) publishMailModifyJob(ctx context.Context, userID uuid.UUID, emailIDs []int64, action string) {
	if s.messageProducer == nil || s.emailRepo == nil {
		return
	}

	// Group emails by connection for batch processing
	type connectionData struct {
		externalIDs []string
		emailIDs    []int64
	}
	connectionEmails := make(map[int64]*connectionData) // connectionID -> data

	for _, emailID := range emailIDs {
		email, err := s.emailRepo.GetByID(ctx, emailID)
		if err != nil || email == nil {
			continue
		}
		if connectionEmails[email.ConnectionID] == nil {
			connectionEmails[email.ConnectionID] = &connectionData{}
		}
		connectionEmails[email.ConnectionID].externalIDs = append(connectionEmails[email.ConnectionID].externalIDs, email.ExternalID)
		connectionEmails[email.ConnectionID].emailIDs = append(connectionEmails[email.ConnectionID].emailIDs, emailID)
	}

	// Publish job for each connection
	for connectionID, data := range connectionEmails {
		// Get provider type from connection
		provider := "google" // default
		if s.oauthService != nil {
			if conn, err := s.oauthService.GetConnection(ctx, connectionID); err == nil {
				provider = string(conn.Provider)
			}
		}

		job := &out.MailModifyJob{
			UserID:       userID.String(),
			ConnectionID: connectionID,
			Provider:     provider,
			Action:       action,
			EmailIDs:     data.emailIDs,    // DB IDs for SSE broadcast
			ExternalIDs:  data.externalIDs, // Provider IDs for API sync
		}

		// Set labels based on action (Gmail specific)
		switch action {
		case "read":
			job.RemoveLabels = []string{"UNREAD"}
		case "unread":
			job.AddLabels = []string{"UNREAD"}
		case "star":
			job.AddLabels = []string{"STARRED"}
		case "unstar":
			job.RemoveLabels = []string{"STARRED"}
		case "archive":
			job.RemoveLabels = []string{"INBOX"}
		case "trash":
			job.AddLabels = []string{"TRASH"}
			job.RemoveLabels = []string{"INBOX"}
		}

		if err := s.messageProducer.PublishMailModify(ctx, job); err != nil {
			// Log error but don't fail the operation
			// DB is already updated, provider sync will retry
			logger.WithFields(map[string]any{
				"user_id":       userID.String(),
				"connection_id": connectionID,
				"action":        action,
				"email_count":   len(data.emailIDs),
			}).WithError(err).Error("failed to publish mail modify job")
			continue
		}
	}
}

// publishMailModifyJobWithLabels publishes a mail modify job with custom labels
func (s *Service) publishMailModifyJobWithLabels(ctx context.Context, userID uuid.UUID, emailIDs []int64, addLabels, removeLabels []string) {
	if s.messageProducer == nil || s.emailRepo == nil {
		return
	}

	// Group emails by connection
	type connectionData struct {
		externalIDs []string
		emailIDs    []int64
	}
	connectionEmails := make(map[int64]*connectionData)

	for _, emailID := range emailIDs {
		email, err := s.emailRepo.GetByID(ctx, emailID)
		if err != nil || email == nil {
			continue
		}
		if connectionEmails[email.ConnectionID] == nil {
			connectionEmails[email.ConnectionID] = &connectionData{}
		}
		connectionEmails[email.ConnectionID].externalIDs = append(connectionEmails[email.ConnectionID].externalIDs, email.ExternalID)
		connectionEmails[email.ConnectionID].emailIDs = append(connectionEmails[email.ConnectionID].emailIDs, emailID)
	}

	// Publish job for each connection
	for connectionID, data := range connectionEmails {
		provider := "google"
		if s.oauthService != nil {
			if conn, err := s.oauthService.GetConnection(ctx, connectionID); err == nil {
				provider = string(conn.Provider)
			}
		}

		job := &out.MailModifyJob{
			UserID:       userID.String(),
			ConnectionID: connectionID,
			Provider:     provider,
			Action:       "labels",
			EmailIDs:     data.emailIDs,
			ExternalIDs:  data.externalIDs,
			AddLabels:    addLabels,
			RemoveLabels: removeLabels,
		}

		if err := s.messageProducer.PublishMailModify(ctx, job); err != nil {
			logger.WithFields(map[string]any{
				"user_id":       userID.String(),
				"connection_id": connectionID,
				"action":        "labels",
				"email_count":   len(data.emailIDs),
				"add_labels":    addLabels,
				"remove_labels": removeLabels,
			}).WithError(err).Error("failed to publish mail modify job with labels")
		}
	}
}
