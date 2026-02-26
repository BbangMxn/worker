package mail

import (
	"context"
	"errors"
	"fmt"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/core/service/auth"
	"worker_server/core/service/classification"
	"worker_server/pkg/logger"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

// =============================================================================
// SyncService - Bridgify Mail Sync (Phase 1: Progressive Loading)
// =============================================================================

const (
	FirstBatchSize     = 50  // 1단계: 즉시 표시할 메일 수
	RemainingBatchSize = 100 // 2단계: 백그라운드 배치 크기
	CheckpointInterval = 500 // 체크포인트 저장 간격 (DB 쓰기 최적화)
	SyncPeriodMonths   = 3   // 동기화 기간: 최근 N개월
)

type SyncService struct {
	emailRepo        out.EmailRepository
	emailBodyRepo    out.EmailBodyRepository
	attachmentRepo  out.AttachmentRepository
	syncRepo        out.SyncStateRepository
	emailProvider    out.EmailProviderPort
	oauthService    *auth.OAuthService
	messageProducer out.MessageProducer
	realtime        out.RealtimePort

	// RFC 분류기 (동기화 시점에 헤더 기반 분류)
	rfcClassifier *classification.RFCScoreClassifier
}

func NewSyncService(
	emailRepo out.EmailRepository,
	emailBodyRepo out.EmailBodyRepository,
	attachmentRepo out.AttachmentRepository,
	syncRepo out.SyncStateRepository,
	emailProvider out.EmailProviderPort,
	oauthService *auth.OAuthService,
	messageProducer out.MessageProducer,
	realtime out.RealtimePort,
) *SyncService {
	return &SyncService{
		emailRepo:        emailRepo,
		emailBodyRepo:    emailBodyRepo,
		attachmentRepo:  attachmentRepo,
		syncRepo:        syncRepo,
		emailProvider:    emailProvider,
		oauthService:    oauthService,
		messageProducer: messageProducer,
		realtime:        realtime,
		rfcClassifier:   classification.NewRFCScoreClassifier(),
	}
}

// =============================================================================
// InitialSync - Progressive Loading 방식 (Phase 1)
// =============================================================================
//
// 1단계: 최근 50개 즉시 가져와서 SSE로 UI 표시 (< 2초 목표)
// 2단계: 나머지 백그라운드 동기화 (체크포인트 저장)
func (s *SyncService) InitialSync(ctx context.Context, userID string, connectionID int64) error {
	startTime := time.Now()
	logger.Info("[SyncService.InitialSync] Starting for connection %d", connectionID)

	// 1. OAuth 토큰 가져오기
	token, err := s.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return s.handleSyncError(ctx, connectionID, "failed to get token", err)
	}

	// 2. Connection 정보 가져오기
	conn, err := s.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return s.handleSyncError(ctx, connectionID, "failed to get connection", err)
	}

	// 3. 동기화 상태 생성/조회
	state, err := s.getOrCreateSyncState(ctx, userID, connectionID)
	if err != nil {
		return s.handleSyncError(ctx, connectionID, "failed to get sync state", err)
	}

	// 4. 체크포인트가 있으면 이어하기
	if state.HasCheckpoint() {
		logger.Info("[SyncService.InitialSync] Resuming from checkpoint: %d/%d synced",
			state.CheckpointSyncedCount, state.CheckpointTotalCount)
		return s.resumeFromCheckpoint(ctx, state, token, conn.Email)
	}

	// 5. 새로운 동기화 시작
	s.syncRepo.UpdateStatusWithPhase(ctx, connectionID, domain.SyncStatusSyncing, domain.SyncPhaseInitialFirstBatch, "")

	// 실시간 이벤트: 동기화 시작
	s.pushSyncEvent(ctx, userID, domain.EventSyncStarted, &domain.SyncProgressData{
		ConnectionID: connectionID,
		Status:       "syncing",
		Phase:        string(domain.SyncPhaseInitialFirstBatch),
	})

	// ==========================================================================
	// 1단계: 첫 번째 배치 (50개) - 즉시 표시
	// 날짜 기반 동기화: 최근 3개월 메일만 동기화
	// ==========================================================================
	syncSince := time.Now().AddDate(0, -SyncPeriodMonths, 0)
	firstBatchResult, err := s.emailProvider.InitialSync(ctx, token, &out.ProviderSyncOptions{
		MaxResults: FirstBatchSize,
		StartDate:  &syncSince,
		// Labels를 비워서 모든 메일 가져오기 (INBOX, SENT, DRAFT, STARRED, ARCHIVED 등)
	})
	if err != nil {
		return s.handleSyncError(ctx, connectionID, "failed to fetch first batch", err)
	}

	savedCount, err := s.processMessages(ctx, firstBatchResult.Messages, userID, connectionID, conn.Email, token)
	if err != nil {
		logger.Error("[SyncService] Error processing first batch: %v", err)
	}

	// 첫 번째 배치 완료 이벤트
	s.pushSyncEvent(ctx, userID, domain.EventSyncFirstBatch, &domain.SyncProgressData{
		ConnectionID: connectionID,
		Current:      savedCount,
		Status:       "first_batch_complete",
		Phase:        string(domain.SyncPhaseInitialFirstBatch),
	})

	logger.Info("[SyncService.InitialSync] First batch complete: %d emails in %v",
		savedCount, time.Since(startTime))

	// ==========================================================================
	// 2단계: 나머지 동기화 (날짜 기반 - 3개월 내 전체)
	// 개수 제한 없이 기간 내 모든 메일 동기화
	// ==========================================================================
	if firstBatchResult.NextPageToken != "" {
		s.syncRepo.UpdateStatusWithPhase(ctx, connectionID, domain.SyncStatusSyncing, domain.SyncPhaseInitialRemaining, "")

		// 날짜 기반이므로 제한 없이 전체 동기화
		_, err = s.syncRemainingPages(ctx, state, token, conn.Email, firstBatchResult.NextPageToken, savedCount)
		if err != nil {
			// 실패해도 첫 번째 배치는 이미 저장됨 - 재시도 예약
			logger.Error("[SyncService] Remaining sync failed, scheduling retry: %v", err)
			return s.scheduleRetry(ctx, connectionID, err)
		}
	}

	// ==========================================================================
	// 완료 처리
	// ==========================================================================
	return s.completeInitialSync(ctx, state, token, firstBatchResult.NextSyncState, startTime)
}

// =============================================================================
// resumeFromCheckpoint - 체크포인트에서 이어하기
// =============================================================================
func (s *SyncService) resumeFromCheckpoint(ctx context.Context, state *domain.SyncState, token *oauth2.Token, accountEmail string) error {
	startTime := time.Now()
	logger.Info("[SyncService.resumeFromCheckpoint] Resuming from page token, %d already synced",
		state.CheckpointSyncedCount)

	s.syncRepo.UpdateStatusWithPhase(ctx, state.ConnectionID, domain.SyncStatusSyncing, domain.SyncPhaseInitialRemaining, "")

	// 체크포인트에서 이어서 동기화 - 마지막 NextSyncState (History ID) 반환
	nextSyncState, err := s.syncRemainingPages(ctx, state, token, accountEmail, state.CheckpointPageToken, state.CheckpointSyncedCount)
	if err != nil {
		return s.scheduleRetry(ctx, state.ConnectionID, err)
	}

	// 불필요한 API 호출 제거: syncRemainingPages에서 이미 History ID를 받았음
	return s.completeInitialSync(ctx, state, token, nextSyncState, startTime)
}

// =============================================================================
// syncRemainingPages - 나머지 페이지 동기화 (체크포인트 저장)
// Returns: nextSyncState (History ID), error
// 날짜 기반 동기화: StartDate 파라미터로 기간 제한
// =============================================================================
func (s *SyncService) syncRemainingPages(ctx context.Context, state *domain.SyncState, token *oauth2.Token, accountEmail, pageToken string, currentCount int) (string, error) {
	syncedCount := currentCount
	var lastNextSyncState string
	syncSince := time.Now().AddDate(0, -SyncPeriodMonths, 0)

	for pageToken != "" {
		// 페이지 가져오기 (날짜 기반 필터 적용)
		result, err := s.emailProvider.InitialSync(ctx, token, &out.ProviderSyncOptions{
			MaxResults: RemainingBatchSize,
			PageToken:  pageToken,
			StartDate:  &syncSince,
		})
		if err != nil {
			// 실패 시 현재 체크포인트 저장
			s.syncRepo.SaveCheckpoint(ctx, state.ConnectionID, pageToken, syncedCount, 0)
			return "", fmt.Errorf("failed to fetch page: %w", err)
		}

		// 마지막 NextSyncState 저장 (History ID)
		lastNextSyncState = result.NextSyncState

		// 메시지 처리
		saved, err := s.processMessages(ctx, result.Messages, state.UserID, state.ConnectionID, accountEmail, token)
		if err != nil {
			logger.Error("[SyncService] Error processing messages: %v", err)
		}
		syncedCount += saved

		// 체크포인트 저장
		s.syncRepo.SaveCheckpoint(ctx, state.ConnectionID, result.NextPageToken, syncedCount, 0)

		// 진행 상황 이벤트
		s.pushSyncEvent(ctx, state.UserID, domain.EventSyncProgress, &domain.SyncProgressData{
			ConnectionID: state.ConnectionID,
			Current:      syncedCount,
			Status:       "syncing",
			Phase:        string(domain.SyncPhaseInitialRemaining),
		})

		logger.Info("[SyncService] Synced %d emails so far...", syncedCount)

		pageToken = result.NextPageToken
	}

	return lastNextSyncState, nil
}

// syncRemainingPagesLimited is deprecated - 날짜 기반 동기화로 전환됨
// 기존 코드 호환성을 위해 유지하지만 syncRemainingPages 사용을 권장
// func (s *SyncService) syncRemainingPagesLimited(...) error { ... }

// =============================================================================
// completeInitialSync - 초기 동기화 완료 처리
// =============================================================================
func (s *SyncService) completeInitialSync(ctx context.Context, state *domain.SyncState, token *oauth2.Token, nextSyncState string, startTime time.Time) error {
	// History ID 저장
	var historyID uint64
	fmt.Sscanf(nextSyncState, "%d", &historyID)
	s.syncRepo.UpdateHistoryIDIfGreater(ctx, state.ConnectionID, historyID)

	// Watch 설정 (Push Notification) - 실패 시 상태만 업데이트, 완료는 진행
	watchSetupSuccess := false
	watchResp, err := s.emailProvider.Watch(ctx, token)
	if err != nil {
		logger.Error("[SyncService] Failed to setup watch: %v", err)
		// Watch 실패 시 상태를 WatchExpired로 설정하여 재시도 스케줄링
		s.syncRepo.UpdateStatus(ctx, state.ConnectionID, domain.SyncStatusWatchExpired, err.Error())
	} else {
		s.syncRepo.UpdateWatchExpiry(ctx, state.ConnectionID, watchResp.Expiration, watchResp.ExternalID)
		watchSetupSuccess = true
	}

	// 체크포인트 클리어
	s.syncRepo.ClearCheckpoint(ctx, state.ConnectionID)

	// Watch 설정 성공 시에만 완료 마킹
	if watchSetupSuccess {
		s.syncRepo.MarkFirstSyncComplete(ctx, state.ConnectionID)
		s.syncRepo.ResetRetryCount(ctx, state.ConnectionID)
	} else {
		// Watch 실패 시 재시도 스케줄링 (다음 RenewExpiredWatches에서 처리됨)
		logger.Warn("[SyncService] Initial sync data saved but watch not setup - will retry")
	}

	// 소요시간 저장
	durationMs := int(time.Since(startTime).Milliseconds())
	s.syncRepo.UpdateSyncDuration(ctx, state.ConnectionID, durationMs)

	// 동기화 완료 이벤트 (데이터는 저장됨)
	status := "completed"
	if !watchSetupSuccess {
		status = "completed_watch_pending"
	}
	s.pushSyncEvent(ctx, state.UserID, domain.EventSyncCompleted, &domain.SyncProgressData{
		ConnectionID: state.ConnectionID,
		Status:       status,
	})

	logger.Info("[SyncService.InitialSync] Completed in %v (watch: %v)", time.Since(startTime), watchSetupSuccess)

	// 분류되지 않은 기존 이메일 재분류 (OAuth 재연결 후 누락된 분류 복구)
	go s.reclassifyUnclassifiedEmails(context.Background(), state.UserID, state.ConnectionID)

	return nil
}

// =============================================================================
func (s *SyncService) DeltaSync(ctx context.Context, connectionID int64, newHistoryID uint64) error {
	logger.Info("[SyncService.DeltaSync] Starting for connection %d, historyID %d", connectionID, newHistoryID)

	// 1. 현재 상태 조회
	state, err := s.syncRepo.GetByConnectionID(ctx, connectionID)
	if err != nil || state == nil {
		return fmt.Errorf("sync state not found for connection %d", connectionID)
	}

	// 2. OAuth 토큰 가져오기
	token, err := s.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to get token: %w", err)
	}

	// 3. Connection 정보
	conn, err := s.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to get connection: %w", err)
	}

	// 4. History API로 변경사항 조회
	syncState := fmt.Sprintf("%d", state.HistoryID)
	result, err := s.emailProvider.IncrementalSync(ctx, token, syncState)
	if err != nil {
		// Full sync 필요
		if providerErr, ok := err.(*out.ProviderError); ok && providerErr.Code == out.ProviderErrSyncRequired {
			logger.Info("[SyncService.DeltaSync] Full sync required, triggering...")
			return s.InitialSync(ctx, state.UserID, connectionID)
		}
		return fmt.Errorf("failed to get history: %w", err)
	}

	// 5. 새 메시지 처리
	savedCount := 0
	for _, msg := range result.Messages {
		email := s.convertProviderMessage(msg, state.UserID, connectionID, conn.Email)

		if err := s.saveEmailWithBody(ctx, email, msg, token); err != nil {
			logger.Error("[SyncService] Failed to save email: %v", err)
			continue
		}
		savedCount++

		// AI 작업 발행 (snippet 길이 기반으로 요약 여부 결정)
		s.publishAIJobs(ctx, state.UserID, email.ID, len(msg.Snippet))

		// Push-centric: body 즉시 가져와서 SSE 푸시
		body, bodyErr := s.emailProvider.GetMessageBody(ctx, token, msg.ExternalID)
		if bodyErr != nil {
			logger.Warn("[SyncService] Failed to fetch body for push: %v", bodyErr)
			s.pushNewEmailEvent(ctx, state.UserID, email, msg.Snippet)
		} else {
			if s.emailBodyRepo != nil {
				bodyEntity := out.NewMailBodyEntity(email.ID, connectionID, msg.ExternalID)
				bodyEntity.HTML = body.HTML
				bodyEntity.Text = body.Text
				_ = s.emailBodyRepo.SaveBody(ctx, bodyEntity)
			}
			s.pushFullEmailEvent(ctx, state.UserID, email, body)
		}
	}

	// 6. 삭제된 메시지 처리
	if len(result.DeletedIDs) > 0 {
		if err := s.emailRepo.DeleteByExternalIDs(ctx, connectionID, result.DeletedIDs); err != nil {
			logger.Error("[SyncService] Failed to delete emails: %v", err)
		}
	}

	// 7. History ID 업데이트
	var nextHistoryID uint64
	fmt.Sscanf(result.NextSyncState, "%d", &nextHistoryID)
	s.syncRepo.UpdateHistoryIDIfGreater(ctx, connectionID, nextHistoryID)

	if savedCount > 0 {
		s.syncRepo.IncrementSyncCount(ctx, connectionID, savedCount)
	}

	logger.Info("[SyncService.DeltaSync] Completed: %d new, %d deleted", savedCount, len(result.DeletedIDs))
	return nil
}

// =============================================================================
// GapSync - 앱 시작 시 갭 체크 및 복구 (Phase 2)
// =============================================================================
//
// 앱 재시작 또는 오프라인 복귀 시 호출됩니다.
// 1. 저장된 historyID와 현재 Gmail historyID 비교
// 2. 차이가 있으면 History API로 Partial Sync
// 3. 404 에러(historyID 만료)면 Full Sync 필요
func (s *SyncService) GapSync(ctx context.Context, connectionID int64) error {
	logger.Info("[SyncService.GapSync] Starting for connection %d", connectionID)

	// 1. 현재 상태 조회
	state, err := s.syncRepo.GetByConnectionID(ctx, connectionID)
	if err != nil || state == nil {
		return fmt.Errorf("sync state not found for connection %d", connectionID)
	}

	// 첫 동기화가 안됐으면 InitialSync로
	if state.IsFirstSync() {
		logger.Info("[SyncService.GapSync] First sync not complete, triggering InitialSync")
		return s.InitialSync(ctx, state.UserID, connectionID)
	}

	// 2. OAuth 토큰 가져오기
	token, err := s.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return s.handleSyncError(ctx, connectionID, "failed to get token", err)
	}

	// 3. Connection 정보
	conn, err := s.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return s.handleSyncError(ctx, connectionID, "failed to get connection", err)
	}

	// 4. 상태 업데이트
	s.syncRepo.UpdateStatusWithPhase(ctx, connectionID, domain.SyncStatusSyncing, domain.SyncPhaseGap, "")

	// 실시간 이벤트: Gap 체크 시작
	s.pushSyncEvent(ctx, state.UserID, domain.EventSyncProgress, &domain.SyncProgressData{
		ConnectionID: connectionID,
		Status:       "gap_checking",
		Phase:        string(domain.SyncPhaseGap),
	})

	// 5. History API로 변경사항 조회 (저장된 historyID 이후)
	syncState := fmt.Sprintf("%d", state.HistoryID)
	result, err := s.emailProvider.IncrementalSync(ctx, token, syncState)
	if err != nil {
		// History ID가 만료됨 (404) - Full Sync 필요
		if providerErr, ok := err.(*out.ProviderError); ok {
			if providerErr.Code == out.ProviderErrSyncRequired || providerErr.Code == out.ProviderErrNotFound {
				logger.Info("[SyncService.GapSync] History expired, triggering full resync")
				s.syncRepo.UpdateStatusWithPhase(ctx, connectionID, domain.SyncStatusSyncing, domain.SyncPhaseFullResync, "")
				return s.fullResync(ctx, state, token, conn.Email)
			}
		}
		return s.handleSyncError(ctx, connectionID, "failed to get history", err)
	}

	// 6. 변경사항이 없으면 바로 완료
	if len(result.Messages) == 0 && len(result.DeletedIDs) == 0 {
		logger.Info("[SyncService.GapSync] No changes detected, already up to date")
		s.syncRepo.UpdateStatus(ctx, connectionID, domain.SyncStatusIdle, "")
		return nil
	}

	// 7. 새 메시지 처리
	savedCount := 0
	for _, msg := range result.Messages {
		email := s.convertProviderMessage(msg, state.UserID, connectionID, conn.Email)

		if err := s.saveEmailWithBody(ctx, email, msg, token); err != nil {
			logger.Error("[SyncService.GapSync] Failed to save email: %v", err)
			continue
		}
		savedCount++

		// AI 작업 발행 (snippet 길이 기반으로 요약 여부 결정)
		s.publishAIJobs(ctx, state.UserID, email.ID, len(msg.Snippet))

		// 실시간 새 메일 알림
		s.pushNewEmailEvent(ctx, state.UserID, email, msg.Snippet)
	}

	// 8. 삭제된 메시지 처리
	deletedCount := 0
	if len(result.DeletedIDs) > 0 {
		if err := s.emailRepo.DeleteByExternalIDs(ctx, connectionID, result.DeletedIDs); err != nil {
			logger.Error("[SyncService.GapSync] Failed to delete emails: %v", err)
		} else {
			deletedCount = len(result.DeletedIDs)
		}
	}

	// 9. History ID 업데이트
	var nextHistoryID uint64
	fmt.Sscanf(result.NextSyncState, "%d", &nextHistoryID)
	s.syncRepo.UpdateHistoryIDIfGreater(ctx, connectionID, nextHistoryID)

	if savedCount > 0 {
		s.syncRepo.IncrementSyncCount(ctx, connectionID, savedCount)
	}

	// 10. 완료
	s.syncRepo.UpdateStatus(ctx, connectionID, domain.SyncStatusIdle, "")

	// 실시간 이벤트: Gap Sync 완료
	s.pushSyncEvent(ctx, state.UserID, domain.EventSyncCompleted, &domain.SyncProgressData{
		ConnectionID: connectionID,
		Current:      savedCount,
		Status:       "gap_sync_complete",
		Phase:        string(domain.SyncPhaseGap),
	})

	logger.Info("[SyncService.GapSync] Completed: %d new, %d deleted", savedCount, deletedCount)
	return nil
}

// =============================================================================
// fullResync - History ID 만료 시 전체 재동기화
// =============================================================================
//
// Gmail History ID는 일정 시간이 지나면 만료됩니다.
// 이 경우 InitialSync와 유사하지만 기존 데이터와 병합합니다.
func (s *SyncService) fullResync(ctx context.Context, state *domain.SyncState, token *oauth2.Token, accountEmail string) error {
	logger.Info("[SyncService.fullResync] Starting full resync for connection %d", state.ConnectionID)
	startTime := time.Now()

	// 실시간 이벤트: Full Resync 시작
	s.pushSyncEvent(ctx, state.UserID, domain.EventSyncProgress, &domain.SyncProgressData{
		ConnectionID: state.ConnectionID,
		Status:       "full_resync",
		Phase:        string(domain.SyncPhaseFullResync),
	})

	// 전체 메일 가져오기 (페이지네이션)
	// 날짜 기반 필터: 최근 N개월 데이터만 재동기화
	syncSince := time.Now().AddDate(0, -SyncPeriodMonths, 0)
	var pageToken string
	syncedCount := 0
	newCount := 0

	for {
		result, err := s.emailProvider.InitialSync(ctx, token, &out.ProviderSyncOptions{
			MaxResults: RemainingBatchSize,
			PageToken:  pageToken,
			StartDate:  &syncSince,
		})
		if err != nil {
			return s.handleSyncError(ctx, state.ConnectionID, "failed to fetch during resync", err)
		}

		// 배치 조회로 N+1 문제 해결 (processMessages와 동일한 패턴)
		externalIDs := make([]string, len(result.Messages))
		for i, msg := range result.Messages {
			externalIDs[i] = msg.ExternalID
		}
		existingMap, _ := s.emailRepo.GetByExternalIDs(ctx, state.ConnectionID, externalIDs)
		if existingMap == nil {
			existingMap = make(map[string]*out.MailEntity)
		}

		// 메시지 처리 (배치 조회 결과 활용)
		for _, msg := range result.Messages {
			// 이미 존재하는 이메일
			if existing, exists := existingMap[msg.ExternalID]; exists {
				syncedCount++
				// URL 기반 방식: 첨부파일 메타데이터는 DB에 저장하지 않음
				// has_attachment 플래그는 이미 설정되어 있음
				_ = existing // 사용하지 않음
				continue
			}

			// 새 이메일 저장
			email := s.convertProviderMessage(msg, state.UserID, state.ConnectionID, accountEmail)
			if err := s.saveEmailWithBody(ctx, email, msg, token); err != nil {
				logger.Error("[SyncService.fullResync] Failed to save email: %v", err)
				continue
			}
			syncedCount++
			newCount++

			// AI 작업 발행
			s.publishAIJobs(ctx, state.UserID, email.ID, len(msg.Snippet))
		}

		// 진행 상황 이벤트
		s.pushSyncEvent(ctx, state.UserID, domain.EventSyncProgress, &domain.SyncProgressData{
			ConnectionID: state.ConnectionID,
			Current:      syncedCount,
			Status:       "resyncing",
			Phase:        string(domain.SyncPhaseFullResync),
		})

		logger.Info("[SyncService.fullResync] Processed %d emails (%d new)...", syncedCount, newCount)

		// History ID는 매 배치마다 업데이트 (중간에 중단되어도 복구 가능)
		if result.NextSyncState != "" {
			var historyID uint64
			fmt.Sscanf(result.NextSyncState, "%d", &historyID)
			if historyID > 0 {
				s.syncRepo.UpdateHistoryIDIfGreater(ctx, state.ConnectionID, historyID)
			}
		}

		if !result.HasMore || result.NextPageToken == "" {
			break
		}
		pageToken = result.NextPageToken
	}

	// Watch 재설정
	watchResp, err := s.emailProvider.Watch(ctx, token)
	if err != nil {
		logger.Error("[SyncService.fullResync] Failed to setup watch: %v", err)
	} else {
		s.syncRepo.UpdateWatchExpiry(ctx, state.ConnectionID, watchResp.Expiration, watchResp.ExternalID)
	}

	// 완료
	s.syncRepo.UpdateStatus(ctx, state.ConnectionID, domain.SyncStatusIdle, "")

	// 실시간 이벤트: Full Resync 완료
	s.pushSyncEvent(ctx, state.UserID, domain.EventSyncCompleted, &domain.SyncProgressData{
		ConnectionID: state.ConnectionID,
		Current:      syncedCount,
		Status:       "resync_complete",
		Phase:        string(domain.SyncPhaseFullResync),
	})

	logger.Info("[SyncService.fullResync] Completed in %v: %d total, %d new", time.Since(startTime), syncedCount, newCount)

	// 분류되지 않은 기존 이메일 재분류 (OAuth 재연결 후 누락된 분류 복구)
	go s.reclassifyUnclassifiedEmails(context.Background(), state.UserID, state.ConnectionID)

	return nil
}

// reclassifyUnclassifiedEmails publishes ai.classify jobs for unclassified emails.
// This is called after fullResync to recover emails that failed classification before.
func (s *SyncService) reclassifyUnclassifiedEmails(ctx context.Context, userID string, connectionID int64) {
	if s.emailRepo == nil || s.messageProducer == nil {
		return
	}

	// 배치 설정: Worker pool이 처리할 수 있는 속도로 발행
	const (
		batchSize  = 50              // 한 번에 발행할 작업 수
		batchDelay = 2 * time.Second // 배치 간 대기 시간
		maxEmails  = 2000            // 최대 처리할 이메일 수
	)

	// 분류되지 않은 이메일 조회
	unclassified, err := s.emailRepo.ListUnclassifiedByConnection(ctx, connectionID, maxEmails)
	if err != nil {
		logger.Error("[SyncService.reclassifyUnclassifiedEmails] Failed to list unclassified: %v", err)
		return
	}

	if len(unclassified) == 0 {
		logger.Info("[SyncService.reclassifyUnclassifiedEmails] No unclassified emails for connection %d", connectionID)
		return
	}

	logger.Info("[SyncService.reclassifyUnclassifiedEmails] Found %d unclassified emails for connection %d, publishing in batches of %d", len(unclassified), connectionID, batchSize)

	// 배치로 나눠서 발행 (rate limiting 방지)
	published := 0
	for i := 0; i < len(unclassified); i += batchSize {
		end := i + batchSize
		if end > len(unclassified) {
			end = len(unclassified)
		}

		batch := unclassified[i:end]
		for _, email := range batch {
			s.messageProducer.PublishAIClassify(ctx, &out.AIClassifyJob{
				UserID:  userID,
				EmailID: email.ID,
			})
			published++
		}

		logger.Info("[SyncService.reclassifyUnclassifiedEmails] Published batch %d/%d (%d emails)", (i/batchSize)+1, (len(unclassified)+batchSize-1)/batchSize, len(batch))

		// 마지막 배치가 아니면 대기
		if end < len(unclassified) {
			time.Sleep(batchDelay)
		}
	}

	logger.Info("[SyncService.reclassifyUnclassifiedEmails] Completed: published %d classify jobs for connection %d", published, connectionID)
}

// =============================================================================
// 재시도 관련
// =============================================================================

func (s *SyncService) scheduleRetry(ctx context.Context, connectionID int64, err error) error {
	state, getErr := s.syncRepo.GetByConnectionID(ctx, connectionID)
	if getErr != nil || state == nil {
		return fmt.Errorf("failed to get sync state for retry: %w", getErr)
	}

	if !state.CanRetry() {
		// 최대 재시도 초과 - 수동 재시도 필요
		s.syncRepo.MarkFailed(ctx, connectionID, fmt.Sprintf("max retries exceeded: %v", err))
		logger.Error("[SyncService] Max retries exceeded for connection %d", connectionID)
		return fmt.Errorf("max retries exceeded: %w", err)
	}

	// 다음 재시도 시간 계산
	delay := domain.GetRetryDelay(state.RetryCount)
	nextRetryAt := time.Now().Add(delay)

	s.syncRepo.ScheduleRetry(ctx, connectionID, nextRetryAt)
	logger.Info("[SyncService] Scheduled retry %d for connection %d at %v",
		state.RetryCount+1, connectionID, nextRetryAt)

	return fmt.Errorf("sync failed, retry scheduled: %w", err)
}

func (s *SyncService) handleSyncError(ctx context.Context, connectionID int64, message string, err error) error {
	fullErr := fmt.Errorf("%s: %w", message, err)
	logger.Error("[SyncService] Error: %v", fullErr)

	// If token expired, don't retry - user needs to re-authenticate
	if errors.Is(err, auth.ErrTokenExpired) {
		logger.Warn("[SyncService] Token expired for connection %d, not scheduling retry", connectionID)
		s.syncRepo.UpdateStatus(ctx, connectionID, domain.SyncStatusError, "OAuth token expired - reconnection required")

		// Send SSE event to notify user about token expiration
		if state, stateErr := s.syncRepo.GetByConnectionID(ctx, connectionID); stateErr == nil && state != nil {
			s.pushSyncEvent(ctx, state.UserID, domain.EventTokenExpired, &domain.SyncProgressData{
				ConnectionID: connectionID,
				Status:       "token_expired",
			})
		}
		return fullErr
	}

	s.syncRepo.UpdateStatus(ctx, connectionID, domain.SyncStatusError, fullErr.Error())
	return s.scheduleRetry(ctx, connectionID, fullErr)
}

// =============================================================================
// Watch 관리
// =============================================================================

func (s *SyncService) RenewExpiredWatches(ctx context.Context) error {
	expireBefore := time.Now().Add(24 * time.Hour)
	states, err := s.syncRepo.GetExpiredWatches(ctx, expireBefore)
	if err != nil {
		return fmt.Errorf("failed to get expired watches: %w", err)
	}

	logger.Info("[SyncService.RenewExpiredWatches] Found %d watches to renew", len(states))

	for _, state := range states {
		token, err := s.oauthService.GetOAuth2Token(ctx, state.ConnectionID)
		if err != nil {
			logger.Error("[SyncService] Failed to get token for connection %d: %v", state.ConnectionID, err)
			continue
		}

		watchResp, err := s.emailProvider.Watch(ctx, token)
		if err != nil {
			logger.Error("[SyncService] Failed to renew watch for connection %d: %v", state.ConnectionID, err)
			s.syncRepo.UpdateStatus(ctx, state.ConnectionID, domain.SyncStatusWatchExpired, err.Error())
			continue
		}

		s.syncRepo.UpdateWatchExpiry(ctx, state.ConnectionID, watchResp.Expiration, watchResp.ExternalID)
		logger.Info("[SyncService] Renewed watch for connection %d, expires %v", state.ConnectionID, watchResp.Expiration)
	}

	return nil
}

// =============================================================================
// Helper 메서드
// =============================================================================

func (s *SyncService) getOrCreateSyncState(ctx context.Context, userID string, connectionID int64) (*domain.SyncState, error) {
	state, err := s.syncRepo.GetByConnectionID(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	if state == nil {
		state = &domain.SyncState{
			UserID:       userID,
			ConnectionID: connectionID,
			Provider:     domain.MailProviderGmail,
			Status:       domain.SyncStatusPending,
			MaxRetries:   5,
		}
		if err := s.syncRepo.Create(ctx, state); err != nil {
			return nil, err
		}
	}

	return state, nil
}

func (s *SyncService) processMessages(ctx context.Context, messages []out.ProviderMailMessage, userID string, connectionID int64, accountEmail string, token *oauth2.Token) (int, error) {
	if len(messages) == 0 {
		return 0, nil
	}

	// 1. ExternalID 목록 추출
	externalIDs := make([]string, len(messages))
	for i, msg := range messages {
		externalIDs[i] = msg.ExternalID
	}

	// 2. 배치 조회 (1번의 DB 쿼리로 N+1 문제 해결)
	existingMap, _ := s.emailRepo.GetByExternalIDs(ctx, connectionID, externalIDs)
	if existingMap == nil {
		existingMap = make(map[string]*out.MailEntity)
	}

	// 3. 새 메일만 필터링 + Entity 변환 (배치 처리용)
	var newEmails []*domain.Email
	var newEntities []*out.MailEntity
	var newMessages []out.ProviderMailMessage

	for _, msg := range messages {
		if _, exists := existingMap[msg.ExternalID]; exists {
			continue
		}
		email := s.convertProviderMessage(msg, userID, connectionID, accountEmail)
		entity := s.domainToEntity(email)

		// 첨부파일 플래그 설정 (has:attachment 쿼리 결과 기반)
		if msg.HasAttachment || len(msg.Attachments) > 0 {
			entity.HasAttachment = true
		}

		newEmails = append(newEmails, email)
		newEntities = append(newEntities, entity)
		newMessages = append(newMessages, msg)
	}

	if len(newEntities) == 0 {
		return 0, nil
	}

	// 4. 배치 삽입 (BulkUpsert: ON CONFLICT로 중복 처리)
	userUUID := uuid.MustParse(userID)
	if err := s.emailRepo.BulkUpsert(ctx, userUUID, connectionID, newEntities); err != nil {
		logger.Error("[SyncService] BulkUpsert failed: %v", err)
		// 폴백: 개별 저장 시도
		return s.processMessagesFallback(ctx, newEmails, newMessages, userID, connectionID, accountEmail, token)
	}

	// 5. 저장된 ID 조회 (AI 작업 발행용)
	savedMap, err := s.emailRepo.GetByExternalIDs(ctx, connectionID, externalIDs)
	if err != nil {
		logger.Warn("[SyncService] Failed to get saved IDs: %v", err)
	}

	// 6. AI 작업 일괄 발행 (RFC로 이미 분류된 경우 분류 작업 건너뜀)
	for i, email := range newEmails {
		if savedMap != nil {
			if saved, ok := savedMap[email.ProviderID]; ok {
				email.ID = saved.ID
			}
		}
		if email.ID > 0 {
			// RFC로 이미 분류된 경우 분류 작업 건너뜀
			alreadyClassified := email.AICategory != nil
			s.publishAIJobsWithClassification(ctx, userID, email.ID, len(newMessages[i].Snippet), alreadyClassified)
		}
	}

	logger.Info("[SyncService] Batch saved %d emails", len(newEntities))
	return len(newEntities), nil
}

// processMessagesFallback 개별 저장 폴백 (배치 실패 시)
func (s *SyncService) processMessagesFallback(ctx context.Context, emails []*domain.Email, messages []out.ProviderMailMessage, userID string, connectionID int64, accountEmail string, token *oauth2.Token) (int, error) {
	savedCount := 0
	for i, email := range emails {
		msg := messages[i]
		if err := s.saveEmailWithBody(ctx, email, msg, token); err != nil {
			logger.Error("[SyncService] Fallback save failed: %v", err)
			continue
		}
		savedCount++
		s.publishAIJobs(ctx, userID, email.ID, len(msg.Snippet))
	}
	return savedCount, nil
}

func (s *SyncService) saveEmailWithBody(ctx context.Context, email *domain.Email, msg out.ProviderMailMessage, token *oauth2.Token) error {
	// 중복 체크는 processMessages에서 이미 완료됨
	entity := s.domainToEntity(email)

	// Phase 1 최적화: metadata에서 이미 추출된 첨부파일 정보로 has_attachment 설정
	if len(msg.Attachments) > 0 {
		entity.HasAttachment = true
	}

	if err := s.emailRepo.Create(ctx, entity); err != nil {
		return err
	}
	email.ID = entity.ID

	// URL 기반 방식: 첨부파일 메타데이터는 DB에 저장하지 않음
	// has_attachment 플래그는 has:attachment 쿼리 결과로 이미 설정됨

	// 본문은 Lazy 캐싱 (Phase 2): 사용자가 이메일 열 때 가져옴
	// 기존 비동기 캐싱 제거 → API에서 필요할 때 Provider 호출

	return nil
}

func (s *SyncService) fetchAndCacheBody(ctx context.Context, emailID, connectionID int64, externalID string, token *oauth2.Token) {
	body, err := s.emailProvider.GetMessageBody(ctx, token, externalID)
	if err != nil {
		logger.Error("[SyncService] Failed to fetch body for email %d: %v", emailID, err)
		return
	}

	bodyEntity := out.NewMailBodyEntity(emailID, connectionID, externalID)
	bodyEntity.HTML = body.HTML
	bodyEntity.Text = body.Text

	for _, att := range body.Attachments {
		bodyEntity.Attachments = append(bodyEntity.Attachments, out.AttachmentEntity{
			ID:        att.ID,
			Name:      att.Filename,
			MimeType:  att.MimeType,
			Size:      att.Size,
			ContentID: att.ContentID,
			IsInline:  att.IsInline,
		})
	}

	bodyEntity.OriginalSize = int64(len(body.HTML) + len(body.Text))

	if err := s.emailBodyRepo.SaveBody(ctx, bodyEntity); err != nil {
		logger.Error("[SyncService] Failed to cache body for email %d: %v", emailID, err)
	}

	// 첨부파일 메타데이터 저장 (PostgreSQL)
	if len(body.Attachments) > 0 {
		// 첨부파일이 있으면 has_attachment 플래그 업데이트
		if s.emailRepo != nil {
			if err := s.emailRepo.UpdateHasAttachment(ctx, emailID, true); err != nil {
				logger.Error("[SyncService] Failed to update has_attachment for email %d: %v", emailID, err)
			}
		}

		// URL 기반 방식: 첨부파일 메타데이터는 DB에 저장하지 않음
	}
}

// saveAttachments is removed - URL 기반 방식으로 변경
// 첨부파일 메타데이터는 DB에 저장하지 않고 Provider에서 직접 가져옴

func (s *SyncService) publishAIJobs(ctx context.Context, userID string, emailID int64, contentLength int) {
	s.publishAIJobsWithClassification(ctx, userID, emailID, contentLength, false)
}

// publishAIJobsWithClassification publishes AI jobs, optionally skipping classification.
// alreadyClassified: true if email was already classified by RFC at sync time
func (s *SyncService) publishAIJobsWithClassification(ctx context.Context, userID string, emailID int64, contentLength int, alreadyClassified bool) {
	if s.messageProducer == nil {
		return
	}

	// 1. Classify only if not already classified by RFC
	if !alreadyClassified {
		s.messageProducer.PublishAIClassify(ctx, &out.AIClassifyJob{UserID: userID, EmailID: emailID})
	}

	// 2. Summarize is on-demand only (via AI Agent tool) - removed auto-summarize for cost optimization

	// 3. RAG indexing (for semantic search) - always needed
	s.messageProducer.PublishRAGIndex(ctx, &out.RAGIndexJob{UserID: userID, EmailID: emailID})
}

// =============================================================================
// SSE 이벤트 발송
// =============================================================================

func (s *SyncService) pushSyncEvent(ctx context.Context, userID string, eventType domain.EventType, data *domain.SyncProgressData) {
	if s.realtime == nil {
		return
	}
	s.realtime.Push(ctx, userID, &domain.RealtimeEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	})
}

func (s *SyncService) pushNewEmailEvent(ctx context.Context, userID string, email *domain.Email, snippet string) {
	if s.realtime == nil {
		return
	}
	s.realtime.Push(ctx, userID, &domain.RealtimeEvent{
		Type:      domain.EventNewEmail,
		Timestamp: time.Now(),
		Data: &domain.NewEmailData{
			EmailID:   email.ID,
			Subject:   email.Subject,
			From:      email.FromEmail,
			FromName:  stringValue(email.FromName),
			Snippet:   snippet,
			Folder:    string(email.Folder),
			IsRead:    email.IsRead,
			HasAttach: email.HasAttach,
		},
	})
}

// pushFullEmailEvent - Push-centric SSE: body 포함 전체 데이터 전송
// DeltaSync에서 새 메일 도착 시 body까지 즉시 가져와서 클라이언트에 푸시
func (s *SyncService) pushFullEmailEvent(ctx context.Context, userID string, email *domain.Email, body *out.ProviderMessageBody) {
	if s.realtime == nil {
		return
	}

	var htmlBody, textBody string
	if body != nil {
		htmlBody = body.HTML
		textBody = body.Text
	}

	s.realtime.Push(ctx, userID, &domain.RealtimeEvent{
		Type:      domain.EventNewEmail,
		Timestamp: time.Now(),
		Data: &domain.FullEmailData{
			EmailID:      email.ID,
			ProviderID:   email.ProviderID,
			ThreadID:     email.ThreadID,
			Subject:      email.Subject,
			From:         email.FromEmail,
			FromName:     stringValue(email.FromName),
			To:           email.ToEmails,
			Cc:           email.CcEmails,
			Snippet:      email.Snippet,
			Body:         htmlBody,
			BodyText:     textBody,
			Folder:       string(email.Folder),
			LabelIDs:     email.Labels,
			IsRead:       email.IsRead,
			IsStarred:    email.IsStarred,
			HasAttach:    email.HasAttach,
			ReceivedAt:   email.ReceivedAt.Format(time.RFC3339),
			ConnectionID: email.ConnectionID,
		},
	})
}

// =============================================================================
// 변환 메서드
// =============================================================================

func (s *SyncService) convertProviderMessage(msg out.ProviderMailMessage, userID string, connectionID int64, accountEmail string) *domain.Email {
	var fromName *string
	if msg.From.Name != "" {
		fromName = &msg.From.Name
	}

	email := &domain.Email{
		UserID:       uuid.MustParse(userID),
		ConnectionID: connectionID,
		Provider:     domain.MailProviderGmail,
		AccountEmail: accountEmail,
		ProviderID:   msg.ExternalID,
		ThreadID:     msg.ExternalThreadID,
		Subject:      msg.Subject,
		Snippet:      msg.Snippet,
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

	// RFC 분류 적용 (동기화 시점에 헤더 기반 분류)
	s.applyRFCClassification(email, msg.ClassificationHeaders)

	return email
}

// applyRFCClassification applies RFC header-based classification at sync time.
// This saves LLM costs by classifying newsletters, notifications, etc. without AI.
func (s *SyncService) applyRFCClassification(email *domain.Email, headers *out.ProviderClassificationHeaders) {
	if s.rfcClassifier == nil {
		return
	}

	input := &classification.ScoreClassifierInput{
		UserID:  email.UserID,
		Email:   email,
		Headers: headers,
	}

	result, err := s.rfcClassifier.Classify(context.Background(), input)
	if err != nil || result == nil {
		return // RFC 분류 실패 시 나중에 LLM으로 분류
	}

	// RFC 분류 결과 적용
	email.AICategory = &result.Category
	if result.SubCategory != nil {
		email.AISubCategory = result.SubCategory
	}
	email.AIPriority = &result.Priority
	email.AIScore = &result.Score

	source := domain.ClassificationSourceHeader
	email.ClassificationSource = &source

	logger.Debug("[SyncService] RFC classified: %s -> %s (score=%.2f, source=%s)",
		email.FromEmail, result.Category, result.Score, result.Source)
}

func (s *SyncService) domainToEntity(d *domain.Email) *out.MailEntity {
	var fromName string
	if d.FromName != nil {
		fromName = *d.FromName
	}

	// Determine direction based on folder
	direction := "inbound"
	if d.Folder == "sent" || d.Folder == "drafts" {
		direction = "outbound"
	}

	entity := &out.MailEntity{
		UserID:         d.UserID,
		ConnectionID:   d.ConnectionID,
		Provider:       string(d.Provider),
		AccountEmail:   d.AccountEmail,
		ExternalID:     d.ProviderID,
		FromEmail:      d.FromEmail,
		FromName:       fromName,
		ToEmails:       d.ToEmails,
		CcEmails:       d.CcEmails,
		Subject:        d.Subject,
		Snippet:        d.Snippet,
		Direction:      direction,
		IsRead:         d.IsRead,
		IsDraft:        d.Folder == "drafts",
		HasAttachment:  d.HasAttach,
		IsReplied:      false, // Will be determined by threading analysis
		IsForwarded:    false, // Will be determined by threading analysis
		Folder:         string(d.Folder),
		Labels:         d.Labels,
		ReceivedAt:     d.ReceivedAt,
		AIStatus:       "pending", // Will be classified by AI pipeline
		WorkflowStatus: "none",
	}

	// RFC 분류 결과 반영 (동기화 시점에 이미 분류된 경우)
	if d.AICategory != nil {
		entity.Category = string(*d.AICategory)
		entity.AIStatus = "completed"
	}
	if d.AISubCategory != nil {
		entity.SubCategory = string(*d.AISubCategory)
	}
	if d.AIPriority != nil {
		entity.Priority = float64(*d.AIPriority)
	}
	if d.AIScore != nil {
		entity.AIScore = *d.AIScore
	}
	if d.ClassificationSource != nil {
		entity.ClassificationSource = string(*d.ClassificationSource)
	}

	return entity
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// =============================================================================
// FetchFromProvider - 실시간 조회용 (기존 호환)
// =============================================================================
func (s *SyncService) FetchFromProvider(ctx context.Context, userID string, connectionID int64, limit int) ([]*domain.Email, error) {
	token, err := s.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	conn, err := s.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	result, err := s.emailProvider.InitialSync(ctx, token, &out.ProviderSyncOptions{
		MaxResults: limit,
	})
	if err != nil {
		return nil, err
	}

	var emails []*domain.Email
	for _, msg := range result.Messages {
		email := s.convertProviderMessage(msg, userID, connectionID, conn.Email)
		entity := s.domainToEntity(email)
		if err := s.emailRepo.Create(ctx, entity); err != nil {
			logger.WithError(err).Error("[SyncService.FetchFromProvider] Save error")
		}
		email.ID = entity.ID
		emails = append(emails, email)
	}

	return emails, nil
}
