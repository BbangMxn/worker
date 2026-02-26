package mail

import (
	"context"
	"fmt"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/core/service/auth"
	"worker_server/pkg/logger"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

// =============================================================================
// ModifierService - Offline-First Modifier Queue (Phase 4)
// =============================================================================
//
// Superhuman-style modifier queue for offline operations.
// 클라이언트에서 오프라인으로 수행한 작업을 서버에 적용합니다.

type ModifierService struct {
	modifierRepo out.ModifierRepository
	emailRepo     out.EmailRepository
	emailProvider out.EmailProviderPort
	oauthService *auth.OAuthService
	realtime     out.RealtimePort
}

func NewModifierService(
	modifierRepo out.ModifierRepository,
	emailRepo out.EmailRepository,
	emailProvider out.EmailProviderPort,
	oauthService *auth.OAuthService,
	realtime out.RealtimePort,
) *ModifierService {
	return &ModifierService{
		modifierRepo: modifierRepo,
		emailRepo:     emailRepo,
		emailProvider: emailProvider,
		oauthService: oauthService,
		realtime:     realtime,
	}
}

// =============================================================================
// Queue Operations
// =============================================================================

// EnqueueModifier - 클라이언트에서 수정 작업 큐에 추가
func (s *ModifierService) EnqueueModifier(ctx context.Context, modifier *domain.Modifier) error {
	if modifier.ID == "" {
		modifier.ID = uuid.New().String()
	}
	modifier.Status = domain.ModifierStatusPending
	modifier.CreatedAt = time.Now()
	modifier.RetryCount = 0

	if err := s.modifierRepo.Create(ctx, modifier); err != nil {
		return fmt.Errorf("failed to create modifier: %w", err)
	}

	logger.Info("[ModifierService.EnqueueModifier] Enqueued %s for email %d", modifier.Type, modifier.EmailID)
	return nil
}

// ProcessPendingModifiers - 대기 중인 modifier 처리
func (s *ModifierService) ProcessPendingModifiers(ctx context.Context, connectionID int64) error {
	modifiers, err := s.modifierRepo.GetPendingByConnection(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to get pending modifiers: %w", err)
	}

	if len(modifiers) == 0 {
		return nil
	}

	logger.Info("[ModifierService.ProcessPendingModifiers] Processing %d modifiers for connection %d",
		len(modifiers), connectionID)

	// OAuth 토큰 가져오기
	token, err := s.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to get token: %w", err)
	}

	for _, modifier := range modifiers {
		if err := s.applyModifier(ctx, modifier, token); err != nil {
			logger.Error("[ModifierService] Failed to apply modifier %s: %v", modifier.ID, err)
			continue
		}
	}

	return nil
}

// applyModifier - 개별 modifier 적용
func (s *ModifierService) applyModifier(ctx context.Context, modifier *domain.Modifier, token any) error {
	// 버전 충돌 체크
	if err := s.checkVersionConflict(ctx, modifier); err != nil {
		return err
	}

	// Provider에 적용
	var applyErr error
	switch modifier.Type {
	case domain.ModifierMarkRead:
		applyErr = s.applyMarkRead(ctx, modifier, token)
	case domain.ModifierMarkUnread:
		applyErr = s.applyMarkUnread(ctx, modifier, token)
	case domain.ModifierArchive:
		applyErr = s.applyArchive(ctx, modifier, token)
	case domain.ModifierTrash:
		applyErr = s.applyTrash(ctx, modifier, token)
	case domain.ModifierStar:
		applyErr = s.applyStar(ctx, modifier, token)
	case domain.ModifierUnstar:
		applyErr = s.applyUnstar(ctx, modifier, token)
	case domain.ModifierMoveToFolder:
		applyErr = s.applyMoveToFolder(ctx, modifier, token)
	case domain.ModifierAddLabel:
		applyErr = s.applyAddLabel(ctx, modifier, token)
	case domain.ModifierRemoveLabel:
		applyErr = s.applyRemoveLabel(ctx, modifier, token)
	default:
		applyErr = fmt.Errorf("unknown modifier type: %s", modifier.Type)
	}

	if applyErr != nil {
		s.modifierRepo.MarkFailed(ctx, modifier.ID, applyErr.Error())
		s.modifierRepo.IncrementRetry(ctx, modifier.ID)
		return applyErr
	}

	// 성공 - 버전 업데이트 및 완료 마킹
	newVersion := time.Now().UnixNano()
	s.modifierRepo.MarkApplied(ctx, modifier.ID, newVersion)

	// 로컬 DB 업데이트
	s.updateLocalEmail(ctx, modifier)

	// 실시간 알림
	s.notifyModifierApplied(ctx, modifier)

	logger.Info("[ModifierService] Applied modifier %s (%s) for email %d",
		modifier.ID, modifier.Type, modifier.EmailID)
	return nil
}

// =============================================================================
// Version Conflict Detection
// =============================================================================

func (s *ModifierService) checkVersionConflict(ctx context.Context, modifier *domain.Modifier) error {
	version, err := s.modifierRepo.GetEmailVersion(ctx, modifier.EmailID)
	if err != nil {
		return nil // 버전 정보 없으면 충돌 없음으로 처리
	}

	if version != nil && version.Version > modifier.ClientVersion {
		// 서버 버전이 더 높음 - 충돌 가능성
		conflict := &domain.Conflict{
			ID:         uuid.New().String(),
			ModifierID: modifier.ID,
			Type:       domain.ConflictTypeVersionMismatch,
			ClientState: map[string]any{
				"version": modifier.ClientVersion,
				"type":    modifier.Type,
			},
			ServerState: map[string]any{
				"version":  version.Version,
				"mod_type": version.ModType,
			},
			CreatedAt: time.Now(),
		}

		// 자동 해결 시도 (같은 유형의 수정은 최신 것 우선)
		if version.ModType == string(modifier.Type) {
			conflict.Resolution = domain.ResolutionServerWins
			now := time.Now()
			conflict.ResolvedAt = &now
			conflict.ResolvedBy = "auto"
			s.modifierRepo.CreateConflict(ctx, conflict)
			s.modifierRepo.MarkConflict(ctx, modifier.ID, conflict.ID)
			return fmt.Errorf("version conflict: server wins (same operation)")
		}

		// 다른 유형의 수정은 병합 시도
		conflict.Resolution = domain.ResolutionMerge
		now := time.Now()
		conflict.ResolvedAt = &now
		conflict.ResolvedBy = "auto"
		s.modifierRepo.CreateConflict(ctx, conflict)
		// 병합으로 처리하므로 계속 진행
	}

	return nil
}

// =============================================================================
// Provider Operations
// =============================================================================

// getEmailProviderID 이메일의 provider ID (ExternalID)를 조회
func (s *ModifierService) getEmailProviderID(ctx context.Context, emailID int64) (string, error) {
	if s.emailRepo == nil {
		return "", fmt.Errorf("mail repository not configured")
	}
	email, err := s.emailRepo.GetByID(ctx, emailID)
	if err != nil {
		return "", fmt.Errorf("failed to get email: %w", err)
	}
	if email.ExternalID == "" {
		return "", fmt.Errorf("email has no external ID")
	}
	return email.ExternalID, nil
}

// castToOAuth2Token token을 oauth2.Token으로 변환
func (s *ModifierService) castToOAuth2Token(token any) (*oauth2.Token, error) {
	if token == nil {
		return nil, fmt.Errorf("token is nil")
	}
	// oauth2.Token 타입인 경우 변환
	if oauth2Token, ok := token.(*oauth2.Token); ok {
		return oauth2Token, nil
	}
	return nil, fmt.Errorf("invalid token type: expected *oauth2.Token")
}

func (s *ModifierService) applyMarkRead(ctx context.Context, modifier *domain.Modifier, token any) error {
	logger.Debug("[ModifierService] Mark read: email %d", modifier.EmailID)

	if s.emailProvider == nil {
		logger.Warn("[ModifierService] Warning: emailProvider not configured, skipping provider call")
		return nil
	}

	providerID, err := s.getEmailProviderID(ctx, modifier.EmailID)
	if err != nil {
		return fmt.Errorf("failed to get provider ID: %w", err)
	}

	oauth2Token, err := s.castToOAuth2Token(token)
	if err != nil {
		return err
	}

	if err := s.emailProvider.MarkAsRead(ctx, oauth2Token, providerID); err != nil {
		return fmt.Errorf("failed to mark as read: %w", err)
	}

	logger.Info("[ModifierService] Successfully marked email %d as read", modifier.EmailID)
	return nil
}

func (s *ModifierService) applyMarkUnread(ctx context.Context, modifier *domain.Modifier, token any) error {
	logger.Debug("[ModifierService] Mark unread: email %d", modifier.EmailID)

	if s.emailProvider == nil {
		logger.Warn("[ModifierService] Warning: emailProvider not configured, skipping provider call")
		return nil
	}

	providerID, err := s.getEmailProviderID(ctx, modifier.EmailID)
	if err != nil {
		return fmt.Errorf("failed to get provider ID: %w", err)
	}

	oauth2Token, err := s.castToOAuth2Token(token)
	if err != nil {
		return err
	}

	if err := s.emailProvider.MarkAsUnread(ctx, oauth2Token, providerID); err != nil {
		return fmt.Errorf("failed to mark as unread: %w", err)
	}

	logger.Info("[ModifierService] Successfully marked email %d as unread", modifier.EmailID)
	return nil
}

func (s *ModifierService) applyArchive(ctx context.Context, modifier *domain.Modifier, token any) error {
	logger.Debug("[ModifierService] Archive: email %d", modifier.EmailID)

	if s.emailProvider == nil {
		logger.Warn("[ModifierService] Warning: emailProvider not configured, skipping provider call")
		return nil
	}

	providerID, err := s.getEmailProviderID(ctx, modifier.EmailID)
	if err != nil {
		return fmt.Errorf("failed to get provider ID: %w", err)
	}

	oauth2Token, err := s.castToOAuth2Token(token)
	if err != nil {
		return err
	}

	if err := s.emailProvider.Archive(ctx, oauth2Token, providerID); err != nil {
		return fmt.Errorf("failed to archive: %w", err)
	}

	logger.Info("[ModifierService] Successfully archived email %d", modifier.EmailID)
	return nil
}

func (s *ModifierService) applyTrash(ctx context.Context, modifier *domain.Modifier, token any) error {
	logger.Debug("[ModifierService] Trash: email %d", modifier.EmailID)

	if s.emailProvider == nil {
		logger.Warn("[ModifierService] Warning: emailProvider not configured, skipping provider call")
		return nil
	}

	providerID, err := s.getEmailProviderID(ctx, modifier.EmailID)
	if err != nil {
		return fmt.Errorf("failed to get provider ID: %w", err)
	}

	oauth2Token, err := s.castToOAuth2Token(token)
	if err != nil {
		return err
	}

	if err := s.emailProvider.Trash(ctx, oauth2Token, providerID); err != nil {
		return fmt.Errorf("failed to trash: %w", err)
	}

	logger.Info("[ModifierService] Successfully trashed email %d", modifier.EmailID)
	return nil
}

func (s *ModifierService) applyStar(ctx context.Context, modifier *domain.Modifier, token any) error {
	logger.Debug("[ModifierService] Star: email %d", modifier.EmailID)

	if s.emailProvider == nil {
		logger.Warn("[ModifierService] Warning: emailProvider not configured, skipping provider call")
		return nil
	}

	providerID, err := s.getEmailProviderID(ctx, modifier.EmailID)
	if err != nil {
		return fmt.Errorf("failed to get provider ID: %w", err)
	}

	oauth2Token, err := s.castToOAuth2Token(token)
	if err != nil {
		return err
	}

	if err := s.emailProvider.Star(ctx, oauth2Token, providerID); err != nil {
		return fmt.Errorf("failed to star: %w", err)
	}

	logger.Info("[ModifierService] Successfully starred email %d", modifier.EmailID)
	return nil
}

func (s *ModifierService) applyUnstar(ctx context.Context, modifier *domain.Modifier, token any) error {
	logger.Debug("[ModifierService] Unstar: email %d", modifier.EmailID)

	if s.emailProvider == nil {
		logger.Warn("[ModifierService] Warning: emailProvider not configured, skipping provider call")
		return nil
	}

	providerID, err := s.getEmailProviderID(ctx, modifier.EmailID)
	if err != nil {
		return fmt.Errorf("failed to get provider ID: %w", err)
	}

	oauth2Token, err := s.castToOAuth2Token(token)
	if err != nil {
		return err
	}

	if err := s.emailProvider.Unstar(ctx, oauth2Token, providerID); err != nil {
		return fmt.Errorf("failed to unstar: %w", err)
	}

	logger.Info("[ModifierService] Successfully unstarred email %d", modifier.EmailID)
	return nil
}

func (s *ModifierService) applyMoveToFolder(ctx context.Context, modifier *domain.Modifier, token any) error {
	logger.Debug("[ModifierService] Move to folder %s: email %d", modifier.Params.Folder, modifier.EmailID)

	if s.emailProvider == nil {
		logger.Warn("[ModifierService] Warning: emailProvider not configured, skipping provider call")
		return nil
	}

	providerID, err := s.getEmailProviderID(ctx, modifier.EmailID)
	if err != nil {
		return fmt.Errorf("failed to get provider ID: %w", err)
	}

	oauth2Token, err := s.castToOAuth2Token(token)
	if err != nil {
		return err
	}

	// Folder로 이동 (trash, archive, inbox 등)
	switch modifier.Params.Folder {
	case "trash":
		err = s.emailProvider.Trash(ctx, oauth2Token, providerID)
	case "archive":
		err = s.emailProvider.Archive(ctx, oauth2Token, providerID)
	case "inbox":
		err = s.emailProvider.Restore(ctx, oauth2Token, providerID)
	default:
		// 일반 폴더 이동은 label 추가/제거로 처리
		logger.Warn("[ModifierService] Custom folder move not fully supported: %s", modifier.Params.Folder)
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to move to folder: %w", err)
	}

	logger.Info("[ModifierService] Successfully moved email %d to folder %s", modifier.EmailID, modifier.Params.Folder)
	return nil
}

func (s *ModifierService) applyAddLabel(ctx context.Context, modifier *domain.Modifier, token any) error {
	logger.Debug("[ModifierService] Add label %s: email %d", modifier.Params.Label, modifier.EmailID)

	if s.emailProvider == nil {
		logger.Warn("[ModifierService] Warning: emailProvider not configured, skipping provider call")
		return nil
	}

	providerID, err := s.getEmailProviderID(ctx, modifier.EmailID)
	if err != nil {
		return fmt.Errorf("failed to get provider ID: %w", err)
	}

	oauth2Token, err := s.castToOAuth2Token(token)
	if err != nil {
		return err
	}

	if err := s.emailProvider.AddLabel(ctx, oauth2Token, providerID, modifier.Params.Label); err != nil {
		return fmt.Errorf("failed to add label: %w", err)
	}

	logger.Info("[ModifierService] Successfully added label %s to email %d", modifier.Params.Label, modifier.EmailID)
	return nil
}

func (s *ModifierService) applyRemoveLabel(ctx context.Context, modifier *domain.Modifier, token any) error {
	logger.Debug("[ModifierService] Remove label %s: email %d", modifier.Params.Label, modifier.EmailID)

	if s.emailProvider == nil {
		logger.Warn("[ModifierService] Warning: emailProvider not configured, skipping provider call")
		return nil
	}

	providerID, err := s.getEmailProviderID(ctx, modifier.EmailID)
	if err != nil {
		return fmt.Errorf("failed to get provider ID: %w", err)
	}

	oauth2Token, err := s.castToOAuth2Token(token)
	if err != nil {
		return err
	}

	if err := s.emailProvider.RemoveLabel(ctx, oauth2Token, providerID, modifier.Params.Label); err != nil {
		return fmt.Errorf("failed to remove label: %w", err)
	}

	logger.Info("[ModifierService] Successfully removed label %s from email %d", modifier.Params.Label, modifier.EmailID)
	return nil
}

// =============================================================================
// Local DB Update
// =============================================================================

func (s *ModifierService) updateLocalEmail(ctx context.Context, modifier *domain.Modifier) {
	if s.emailRepo == nil {
		return
	}

	switch modifier.Type {
	case domain.ModifierMarkRead:
		s.emailRepo.UpdateReadStatus(ctx, modifier.EmailID, true)
	case domain.ModifierMarkUnread:
		s.emailRepo.UpdateReadStatus(ctx, modifier.EmailID, false)
	case domain.ModifierArchive:
		s.emailRepo.UpdateFolder(ctx, modifier.EmailID, "archive")
	case domain.ModifierTrash:
		s.emailRepo.UpdateFolder(ctx, modifier.EmailID, "trash")
	case domain.ModifierMoveToFolder:
		s.emailRepo.UpdateFolder(ctx, modifier.EmailID, modifier.Params.Folder)
	}

	// 버전 업데이트
	version := &domain.EmailVersion{
		EmailID:   modifier.EmailID,
		Version:   time.Now().UnixNano(),
		ModType:   string(modifier.Type),
		ModSource: "client",
		ModAt:     time.Now(),
	}
	s.modifierRepo.UpdateEmailVersion(ctx, version)
}

// =============================================================================
// Realtime Notification
// =============================================================================

func (s *ModifierService) notifyModifierApplied(ctx context.Context, modifier *domain.Modifier) {
	if s.realtime == nil {
		return
	}

	event := &domain.RealtimeEvent{
		Type:      domain.EventEmailUpdated,
		Timestamp: time.Now(),
		Data: map[string]any{
			"email_id":    modifier.EmailID,
			"modifier_id": modifier.ID,
			"type":        modifier.Type,
			"status":      "applied",
		},
	}

	s.realtime.Push(ctx, modifier.UserID, event)
}

// =============================================================================
// Conflict Resolution API
// =============================================================================

// GetUnresolvedConflicts - 사용자의 미해결 충돌 조회
func (s *ModifierService) GetUnresolvedConflicts(ctx context.Context, userID string) ([]*domain.Conflict, error) {
	return s.modifierRepo.GetUnresolvedConflicts(ctx, userID)
}

// ResolveConflict - 충돌 수동 해결
func (s *ModifierService) ResolveConflict(ctx context.Context, conflictID string, resolution domain.ConflictResolution) error {
	if err := s.modifierRepo.ResolveConflict(ctx, conflictID, resolution); err != nil {
		return err
	}

	conflict, err := s.modifierRepo.GetConflictByModifier(ctx, conflictID)
	if err != nil {
		return err
	}

	// ClientWins면 modifier 재적용
	if resolution == domain.ResolutionClientWins && conflict != nil {
		modifier, err := s.modifierRepo.GetByID(ctx, conflict.ModifierID)
		if err != nil {
			return err
		}
		modifier.Status = domain.ModifierStatusPending
		return s.modifierRepo.Update(ctx, modifier)
	}

	return nil
}

// =============================================================================
// Cleanup
// =============================================================================

// CleanupOldModifiers - 오래된 적용 완료 modifier 정리
func (s *ModifierService) CleanupOldModifiers(ctx context.Context, olderThan time.Duration) (int64, error) {
	before := time.Now().Add(-olderThan)
	return s.modifierRepo.DeleteAppliedBefore(ctx, before)
}
