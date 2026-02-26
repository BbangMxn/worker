package worker

import (
	"context"
	"fmt"
	"time"

	"worker_server/adapter/out/provider"
	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/core/service/auth"
	"worker_server/core/service/email"
	"worker_server/pkg/logger"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

// MailProcessor handles mail-related jobs.
type MailProcessor struct {
	oauthService    *auth.OAuthService
	mailSyncService *mail.SyncService
	gmailProvider   *provider.GmailAdapter
	emailRepo        out.EmailRepository
	emailBodyRepo    out.EmailBodyRepository
	messageProducer out.MessageProducer
	realtime        out.RealtimePort
}

// NewMailProcessor creates a new mail processor.
func NewMailProcessor(
	oauthService *auth.OAuthService,
	mailSyncService *mail.SyncService,
	gmailProvider *provider.GmailAdapter,
	emailRepo out.EmailRepository,
	emailBodyRepo out.EmailBodyRepository,
	messageProducer out.MessageProducer,
	realtime out.RealtimePort,
) *MailProcessor {
	return &MailProcessor{
		oauthService:    oauthService,
		mailSyncService: mailSyncService,
		gmailProvider:   gmailProvider,
		emailRepo:        emailRepo,
		emailBodyRepo:    emailBodyRepo,
		messageProducer: messageProducer,
		realtime:        realtime,
	}
}

// ProcessSync processes mail sync jobs using Push-based real-time sync.
// No polling fallback - requires MailSyncService (Superhuman-style).
func (p *MailProcessor) ProcessSync(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[MailSyncPayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	logger.Info("[MailProcessor.ProcessSync] connection=%d, user=%s, full=%v, historyID=%d",
		payload.ConnectionID, payload.UserID, payload.FullSync, payload.HistoryID)

	// MailSyncService is required for real-time sync
	if p.mailSyncService == nil {
		return fmt.Errorf("mailSyncService not initialized - real-time sync requires MailSyncService")
	}

	if payload.FullSync {
		// Initial sync: fetches recent emails and sets up Gmail Watch for Push notifications
		return p.mailSyncService.InitialSync(ctx, payload.UserID, payload.ConnectionID)
	}

	// Delta sync triggered by Gmail Pub/Sub webhook
	if payload.HistoryID > 0 {
		return p.mailSyncService.DeltaSync(ctx, payload.ConnectionID, payload.HistoryID)
	}

	// No HistoryID - perform initial sync to set up Watch
	return p.mailSyncService.InitialSync(ctx, payload.UserID, payload.ConnectionID)
}

// ProcessDeltaSync processes Pub/Sub triggered delta sync.
func (p *MailProcessor) ProcessDeltaSync(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[MailSyncPayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	logger.Info("[MailProcessor.ProcessDeltaSync] connection=%d, historyID=%d",
		payload.ConnectionID, payload.HistoryID)

	if p.mailSyncService == nil {
		return fmt.Errorf("mailSyncService not initialized")
	}

	return p.mailSyncService.DeltaSync(ctx, payload.ConnectionID, payload.HistoryID)
}

// ProcessSend processes mail send jobs.
func (p *MailProcessor) ProcessSend(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[MailSendPayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	logger.Info("[MailProcessor.ProcessSend] to=%v", payload.To)

	if p.oauthService == nil || p.gmailProvider == nil {
		return fmt.Errorf("required dependencies not initialized")
	}

	// Get OAuth token
	token, err := p.oauthService.GetOAuth2Token(ctx, payload.ConnectionID)
	if err != nil {
		return fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Build outgoing message
	outgoing := &out.ProviderOutgoingMessage{
		Subject: payload.Subject,
		Body:    payload.Body,
		IsHTML:  payload.IsHTML,
	}

	// Convert string addresses to ProviderEmailAddress
	for _, to := range payload.To {
		outgoing.To = append(outgoing.To, out.ProviderEmailAddress{Email: to})
	}
	for _, cc := range payload.Cc {
		outgoing.CC = append(outgoing.CC, out.ProviderEmailAddress{Email: cc})
	}
	for _, bcc := range payload.Bcc {
		outgoing.BCC = append(outgoing.BCC, out.ProviderEmailAddress{Email: bcc})
	}

	// Send email
	result, err := p.gmailProvider.Send(ctx, token, outgoing)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	logger.Info("[MailProcessor.ProcessSend] sent successfully: %s", result.ExternalID)
	return nil
}

// =============================================================================
// SSE Broadcast - 실시간 상태 변경 알림
// =============================================================================

// broadcastStateChange sends SSE event to other clients for real-time UI sync.
func (p *MailProcessor) broadcastStateChange(ctx context.Context, payload *MailModifyPayload) {
	if p.realtime == nil || len(payload.EmailIDs) == 0 {
		return
	}

	// Action → EventType 매핑
	eventType := p.actionToEventType(payload.Action)

	// 이벤트 데이터 구성
	data := map[string]any{
		"email_ids": payload.EmailIDs,
		"action":    payload.Action,
		"timestamp": time.Now(),
	}

	// Action별 추가 데이터
	switch payload.Action {
	case "read":
		data["is_read"] = true
	case "unread":
		data["is_read"] = false
	case "star":
		data["is_starred"] = true
	case "unstar":
		data["is_starred"] = false
	case "archive":
		data["folder"] = "archive"
	case "trash":
		data["folder"] = "trash"
	case "move":
		data["folder"] = payload.TargetFolder
	case "labels":
		data["labels_added"] = payload.AddLabels
		data["labels_removed"] = payload.RemoveLabels
	}

	event := &domain.RealtimeEvent{
		Type: eventType,
		Data: data,
	}

	if err := p.realtime.Push(ctx, payload.UserID, event); err != nil {
		logger.Warn("[MailProcessor.broadcastStateChange] failed to push SSE event: %v", err)
	} else {
		logger.Debug("[MailProcessor.broadcastStateChange] pushed %s event for %d emails",
			eventType, len(payload.EmailIDs))
	}
}

// actionToEventType converts action string to domain.EventType.
func (p *MailProcessor) actionToEventType(action string) domain.EventType {
	switch action {
	case "read":
		return domain.EventEmailRead
	case "unread":
		return domain.EventEmailUnread
	case "star":
		return domain.EventEmailStarred
	case "unstar":
		return domain.EventEmailUnstarred
	case "archive":
		return domain.EventEmailArchived
	case "trash":
		return domain.EventEmailTrashed
	case "move":
		return domain.EventEmailMoved
	case "labels":
		return domain.EventEmailLabeled
	case "snooze":
		return domain.EventEmailSnoozed
	case "unsnooze":
		return domain.EventEmailUnsnoozed
	default:
		return domain.EventEmailUpdated
	}
}

// ProcessSave processes mail save jobs (async metadata save from Gmail API).
// 철학: "메일은 즉시 반환, DB 저장은 Worker에서"
// 최적화: BulkUpsert로 1번의 쿼리로 N개 저장
func (p *MailProcessor) ProcessSave(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[MailSavePayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	logger.Info("[MailProcessor.ProcessSave] user=%s, connection=%d, emails=%d",
		payload.UserID, payload.ConnectionID, len(payload.Emails))

	if p.emailRepo == nil {
		return fmt.Errorf("emailRepo not initialized")
	}

	if len(payload.Emails) == 0 {
		return nil
	}

	// Parse UserID to uuid.UUID
	userUUID, err := uuid.Parse(payload.UserID)
	if err != nil {
		return fmt.Errorf("invalid user_id format: %w", err)
	}

	// 1. Entity 변환 (배치용)
	entities := make([]*out.MailEntity, 0, len(payload.Emails))
	for _, email := range payload.Emails {
		entity := &out.MailEntity{
			UserID:        userUUID,
			ConnectionID:  payload.ConnectionID,
			Provider:      payload.Provider,
			AccountEmail:  payload.AccountEmail,
			ExternalID:    email.ExternalID,
			Subject:       email.Subject,
			FromEmail:     email.FromEmail,
			FromName:      email.FromName,
			ToEmails:      email.ToEmails,
			CcEmails:      email.CcEmails,
			Snippet:       email.Snippet,
			IsRead:        email.IsRead,
			HasAttachment: email.HasAttach,
			Folder:        email.Folder,
			Labels:        email.Labels,
			ReceivedAt:    email.ReceivedAt,
			AIStatus:      "pending",
		}
		entities = append(entities, entity)
	}

	// 2. BulkUpsert로 1번의 쿼리로 저장 (ON CONFLICT 처리)
	if err := p.emailRepo.BulkUpsert(ctx, userUUID, payload.ConnectionID, entities); err != nil {
		return fmt.Errorf("failed to bulk upsert emails: %w", err)
	}

	// 3. 저장된 메일 ID 조회 (AI 파이프라인용)
	externalIDs := make([]string, len(payload.Emails))
	for i, email := range payload.Emails {
		externalIDs[i] = email.ExternalID
	}

	savedMap, err := p.emailRepo.GetByExternalIDs(ctx, payload.ConnectionID, externalIDs)
	if err != nil {
		logger.Warn("[MailProcessor.ProcessSave] failed to get saved emails: %v", err)
	}
	if savedMap == nil {
		savedMap = make(map[string]*out.MailEntity)
	}

	var emailIDs []int64
	for _, email := range payload.Emails {
		if saved, exists := savedMap[email.ExternalID]; exists {
			emailIDs = append(emailIDs, saved.ID)
		}
	}

	logger.Info("[MailProcessor.ProcessSave] saved %d emails via BulkUpsert", len(emailIDs))

	// AI 파이프라인 작업 발행
	if p.messageProducer != nil && len(emailIDs) > 0 {
		// 1. AI 분류 작업 발행 (배치)
		classifyJob := &out.AIBatchClassifyJob{
			UserID:   payload.UserID,
			EmailIDs: emailIDs,
		}
		if err := p.messageProducer.PublishAIBatchClassify(ctx, classifyJob); err != nil {
			logger.Warn("[MailProcessor.ProcessSave] failed to publish classify job: %v", err)
		}

		// 2. Summarize is on-demand only (via AI Agent tool) - removed auto-summarize for cost optimization

		// 3. RAG 인덱싱 작업 발행 (배치)
		ragJob := &out.RAGBatchIndexJob{
			UserID:       payload.UserID,
			ConnectionID: payload.ConnectionID,
			EmailIDs:     emailIDs,
		}
		if err := p.messageProducer.PublishRAGBatchIndex(ctx, ragJob); err != nil {
			logger.Warn("[MailProcessor.ProcessSave] failed to publish RAG job: %v", err)
		}
	}

	return nil
}

// ProcessModify processes mail modify jobs (async provider sync + SSE broadcast).
// 1. 다른 클라이언트에 SSE로 상태 변경 알림 (즉시)
// 2. Provider(Gmail/Outlook)에 상태 동기화 (API 호출)
func (p *MailProcessor) ProcessModify(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[MailModifyPayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	logger.Info("[MailProcessor.ProcessModify] connection=%d, action=%s, emails=%d",
		payload.ConnectionID, payload.Action, len(payload.ExternalIDs))

	// 1. SSE Push - 다른 클라이언트에게 상태 변경 알림
	p.broadcastStateChange(ctx, payload)

	// 2. Provider 동기화
	if len(payload.ExternalIDs) == 0 {
		return nil
	}

	if p.oauthService == nil {
		return fmt.Errorf("oauthService not initialized")
	}

	// Get OAuth token
	token, err := p.oauthService.GetOAuth2Token(ctx, payload.ConnectionID)
	if err != nil {
		return fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Provider별 처리
	switch payload.Provider {
	case "google", "gmail":
		if p.gmailProvider == nil {
			return fmt.Errorf("gmailProvider not initialized")
		}
		return p.modifyGmail(ctx, token, payload)
	case "outlook", "microsoft":
		// TODO: Outlook provider 지원
		logger.Warn("[MailProcessor.ProcessModify] Outlook not yet supported")
		return nil
	default:
		return fmt.Errorf("unsupported provider: %s", payload.Provider)
	}
}

// modifyGmail uses Gmail BatchModify API to update labels on multiple messages.
func (p *MailProcessor) modifyGmail(ctx context.Context, token any, payload *MailModifyPayload) error {
	// Cast token to *oauth2.Token
	oauth2Token, ok := token.(*oauth2.Token)
	if !ok {
		return fmt.Errorf("invalid token type: expected *oauth2.Token")
	}

	// BatchModify로 한 번에 처리 (50개 제한)
	const batchSize = 50

	for i := 0; i < len(payload.ExternalIDs); i += batchSize {
		end := i + batchSize
		if end > len(payload.ExternalIDs) {
			end = len(payload.ExternalIDs)
		}
		batch := payload.ExternalIDs[i:end]

		// Gmail BatchModify API 호출
		req := &out.ProviderBatchModifyRequest{
			IDs:          batch,
			AddLabels:    payload.AddLabels,
			RemoveLabels: payload.RemoveLabels,
		}
		err := p.gmailProvider.BatchModify(ctx, oauth2Token, req)
		if err != nil {
			logger.Error("[MailProcessor.modifyGmail] BatchModify failed: %v", err)
			return fmt.Errorf("batch modify failed: %w", err)
		}

		logger.Info("[MailProcessor.modifyGmail] BatchModified %d messages: +%v -%v",
			len(batch), payload.AddLabels, payload.RemoveLabels)
	}

	return nil
}

// ProcessReply processes reply jobs.
func (p *MailProcessor) ProcessReply(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[MailReplyPayload](msg)
	if err != nil {
		logger.Error("[MailProcessor.ProcessReply] failed to parse payload: %v", err)
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	logger.Info("[MailProcessor.ProcessReply] originalID=%s, to=%v", payload.OriginalID, payload.To)

	if p.oauthService == nil || p.gmailProvider == nil {
		logger.Error("[MailProcessor.ProcessReply] required dependencies not initialized")
		return fmt.Errorf("required dependencies not initialized")
	}

	// Get OAuth token
	token, err := p.oauthService.GetOAuth2Token(ctx, payload.ConnectionID)
	if err != nil {
		logger.Error("[MailProcessor.ProcessReply] failed to get oauth token: %v", err)
		return fmt.Errorf("failed to get oauth token: %w", err)
	}

	// Build reply message
	outgoing := &out.ProviderOutgoingMessage{
		Subject: payload.Subject,
		Body:    payload.Body,
		IsHTML:  payload.IsHTML,
	}

	for _, to := range payload.To {
		outgoing.To = append(outgoing.To, out.ProviderEmailAddress{Email: to})
	}
	for _, cc := range payload.Cc {
		outgoing.CC = append(outgoing.CC, out.ProviderEmailAddress{Email: cc})
	}

	// Send reply
	result, err := p.gmailProvider.Reply(ctx, token, payload.OriginalID, outgoing)
	if err != nil {
		logger.Error("[MailProcessor.ProcessReply] failed to send reply: %v", err)
		return fmt.Errorf("failed to send reply: %w", err)
	}

	logger.Info("[MailProcessor.ProcessReply] replied successfully: %s", result.ExternalID)

	// Notify realtime clients if available
	if p.realtime != nil {
		event := &domain.RealtimeEvent{
			Type: "email.reply_sent",
			Data: map[string]any{
				"original_id": payload.OriginalID,
				"reply_id":    result.ExternalID,
			},
		}
		if err := p.realtime.Push(ctx, payload.UserID.String(), event); err != nil {
			logger.Warn("[MailProcessor.ProcessReply] failed to push realtime event: %v", err)
		}
	}

	return nil
}
