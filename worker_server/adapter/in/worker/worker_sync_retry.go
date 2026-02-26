package worker

import (
	"context"
	"time"

	"worker_server/core/port/out"
	"worker_server/core/service/email"
	"worker_server/pkg/logger"
)

// =============================================================================
// SyncRetryScheduler - 동기화 재시도 스케줄러
// =============================================================================
//
// 주기적으로 GetPendingRetries를 호출하여 재시도가 필요한 동기화 작업을 처리합니다.
// Progressive Loading 실패 시 체크포인트에서 이어서 동기화합니다.

type SyncRetryScheduler struct {
	syncRepo        out.SyncStateRepository
	mailSyncService *mail.SyncService
	checkInterval   time.Duration
	ctx             context.Context
	cancel          context.CancelFunc
}

// NewSyncRetryScheduler creates a new sync retry scheduler.
func NewSyncRetryScheduler(
	syncRepo out.SyncStateRepository,
	mailSyncService *mail.SyncService,
) *SyncRetryScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &SyncRetryScheduler{
		syncRepo:        syncRepo,
		mailSyncService: mailSyncService,
		checkInterval:   30 * time.Second, // 30초마다 체크
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start starts the retry scheduler.
func (s *SyncRetryScheduler) Start() {
	logger.Info("[SyncRetryScheduler] Starting with interval %v", s.checkInterval)
	go s.run()
}

// Stop stops the retry scheduler.
func (s *SyncRetryScheduler) Stop() {
	logger.Info("[SyncRetryScheduler] Stopping...")
	s.cancel()
}

// run is the main loop that checks for pending retries.
func (s *SyncRetryScheduler) run() {
	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	// 시작 시 즉시 한 번 체크
	s.processPendingRetries()

	for {
		select {
		case <-s.ctx.Done():
			logger.Info("[SyncRetryScheduler] Stopped")
			return
		case <-ticker.C:
			s.processPendingRetries()
		}
	}
}

// processPendingRetries processes all sync states that need retry.
func (s *SyncRetryScheduler) processPendingRetries() {
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
	defer cancel()

	// 재시도가 필요한 상태 조회
	states, err := s.syncRepo.GetPendingRetries(ctx, time.Now())
	if err != nil {
		logger.Error("[SyncRetryScheduler] Failed to get pending retries: %v", err)
		return
	}

	if len(states) == 0 {
		return
	}

	logger.Info("[SyncRetryScheduler] Found %d pending retries", len(states))

	for _, state := range states {
		// 각 재시도를 별도 고루틴으로 처리 (병렬 처리)
		go s.processRetry(state.ConnectionID, state.UserID)
	}
}

// processRetry processes a single retry.
func (s *SyncRetryScheduler) processRetry(connectionID int64, userID string) {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	logger.Info("[SyncRetryScheduler] Processing retry for connection %d", connectionID)

	// 재시도 횟수 증가
	if err := s.syncRepo.IncrementRetryCount(ctx, connectionID); err != nil {
		logger.Error("[SyncRetryScheduler] Failed to increment retry count for %d: %v", connectionID, err)
		return
	}

	// InitialSync 호출 (체크포인트가 있으면 자동으로 이어서 처리)
	if err := s.mailSyncService.InitialSync(ctx, userID, connectionID); err != nil {
		logger.Error("[SyncRetryScheduler] Retry failed for connection %d: %v", connectionID, err)
		// 실패 시 SyncService 내부에서 다음 재시도를 스케줄링함
		return
	}

	logger.Info("[SyncRetryScheduler] Retry successful for connection %d", connectionID)
}

// SetCheckInterval sets the check interval (for testing).
func (s *SyncRetryScheduler) SetCheckInterval(interval time.Duration) {
	s.checkInterval = interval
}
