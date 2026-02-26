package worker

import (
	"context"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/core/service/email"
	"worker_server/pkg/logger"
)

// =============================================================================
// GapSyncScheduler - 앱 시작 시 Gap Sync 실행 (Phase 2)
// =============================================================================
//
// 서버 시작 시 모든 활성 연결에 대해 Gap Sync를 실행합니다.
// 이후 주기적으로 idle 상태인 연결들의 gap을 체크합니다.

type GapSyncScheduler struct {
	syncRepo        out.SyncStateRepository
	mailSyncService *mail.SyncService
	checkInterval   time.Duration
	ctx             context.Context
	cancel          context.CancelFunc
	startupComplete bool
}

// NewGapSyncScheduler creates a new gap sync scheduler.
func NewGapSyncScheduler(
	syncRepo out.SyncStateRepository,
	mailSyncService *mail.SyncService,
) *GapSyncScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &GapSyncScheduler{
		syncRepo:        syncRepo,
		mailSyncService: mailSyncService,
		checkInterval:   5 * time.Minute, // 5분마다 gap 체크
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start starts the gap sync scheduler.
func (s *GapSyncScheduler) Start() {
	logger.Info("[GapSyncScheduler] Starting...")
	go s.run()
}

// Stop stops the gap sync scheduler.
func (s *GapSyncScheduler) Stop() {
	logger.Info("[GapSyncScheduler] Stopping...")
	s.cancel()
}

// run is the main loop.
func (s *GapSyncScheduler) run() {
	// 시작 시 모든 활성 연결에 대해 Gap Sync 실행
	s.runStartupGapSync()
	s.startupComplete = true

	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			logger.Info("[GapSyncScheduler] Stopped")
			return
		case <-ticker.C:
			s.checkIdleConnections()
		}
	}
}

// runStartupGapSync runs gap sync for all active connections on startup.
func (s *GapSyncScheduler) runStartupGapSync() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Minute)
	defer cancel()

	logger.Info("[GapSyncScheduler] Running startup gap sync...")

	// idle, pending, error 상태 모두 조회 (첫 동기화 안된 것도 포함)
	var states []*domain.SyncState

	// 1. idle 상태 (정상적으로 완료된 연결)
	idleStates, err := s.syncRepo.GetByStatus(ctx, domain.SyncStatusIdle)
	if err != nil {
		logger.Error("[GapSyncScheduler] Failed to get idle connections: %v", err)
	} else {
		states = append(states, idleStates...)
	}

	// 2. pending 상태 (첫 동기화 안된 연결)
	pendingStates, err := s.syncRepo.GetByStatus(ctx, domain.SyncStatusPending)
	if err != nil {
		logger.Error("[GapSyncScheduler] Failed to get pending connections: %v", err)
	} else {
		states = append(states, pendingStates...)
	}

	// 3. error 상태 (실패한 연결 - 재시도)
	errorStates, err := s.syncRepo.GetByStatus(ctx, domain.SyncStatusError)
	if err != nil {
		logger.Error("[GapSyncScheduler] Failed to get error connections: %v", err)
	} else {
		states = append(states, errorStates...)
	}

	if len(states) == 0 {
		logger.Info("[GapSyncScheduler] No active connections to sync")
		return
	}

	logger.Info("[GapSyncScheduler] Found %d connections to gap sync", len(states))

	// 동시에 최대 5개까지 처리
	semaphore := make(chan struct{}, 5)

	for _, state := range states {
		semaphore <- struct{}{}
		go func(connectionID int64) {
			defer func() { <-semaphore }()
			s.runGapSync(connectionID)
		}(state.ConnectionID)
	}

	// 모든 처리 완료 대기
	for i := 0; i < cap(semaphore); i++ {
		semaphore <- struct{}{}
	}

	logger.Info("[GapSyncScheduler] Startup gap sync completed")
}

// checkIdleConnections checks idle connections for potential gaps.
func (s *GapSyncScheduler) checkIdleConnections() {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	// Watch가 만료됐거나 오랫동안 업데이트 없는 연결 조회
	states, err := s.syncRepo.GetStaleConnections(ctx, 30*time.Minute)
	if err != nil {
		logger.Error("[GapSyncScheduler] Failed to get stale connections: %v", err)
		return
	}

	if len(states) == 0 {
		return
	}

	logger.Info("[GapSyncScheduler] Found %d stale connections to check", len(states))

	for _, state := range states {
		go s.runGapSync(state.ConnectionID)
	}
}

// runGapSync runs gap sync for a single connection.
func (s *GapSyncScheduler) runGapSync(connectionID int64) {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	if err := s.mailSyncService.GapSync(ctx, connectionID); err != nil {
		logger.Error("[GapSyncScheduler] Gap sync failed for connection %d: %v", connectionID, err)
	}
}

// SetCheckInterval sets the check interval (for testing).
func (s *GapSyncScheduler) SetCheckInterval(interval time.Duration) {
	s.checkInterval = interval
}
