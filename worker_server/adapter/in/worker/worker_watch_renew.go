package worker

import (
	"context"
	"time"

	"worker_server/core/service/email"
	"worker_server/pkg/logger"
)

// =============================================================================
// WatchRenewScheduler - Gmail Watch 갱신 스케줄러
// =============================================================================
//
// Gmail Watch는 7일마다 만료됩니다. 이 스케줄러는 만료 24시간 전에 갱신합니다.

type WatchRenewScheduler struct {
	mailSyncService *mail.SyncService
	checkInterval   time.Duration
	ctx             context.Context
	cancel          context.CancelFunc
}

// NewWatchRenewScheduler creates a new watch renew scheduler.
func NewWatchRenewScheduler(mailSyncService *mail.SyncService) *WatchRenewScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &WatchRenewScheduler{
		mailSyncService: mailSyncService,
		checkInterval:   1 * time.Hour, // 1시간마다 체크
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start starts the watch renew scheduler.
func (s *WatchRenewScheduler) Start() {
	logger.Info("[WatchRenewScheduler] Starting with interval %v", s.checkInterval)
	go s.run()
}

// Stop stops the watch renew scheduler.
func (s *WatchRenewScheduler) Stop() {
	logger.Info("[WatchRenewScheduler] Stopping...")
	s.cancel()
}

// run is the main loop that checks for expiring watches.
func (s *WatchRenewScheduler) run() {
	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	// 시작 시 즉시 한 번 체크
	s.renewExpiringWatches()

	for {
		select {
		case <-s.ctx.Done():
			logger.Info("[WatchRenewScheduler] Stopped")
			return
		case <-ticker.C:
			s.renewExpiringWatches()
		}
	}
}

// renewExpiringWatches renews watches that are about to expire.
func (s *WatchRenewScheduler) renewExpiringWatches() {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	if err := s.mailSyncService.RenewExpiredWatches(ctx); err != nil {
		logger.Error("[WatchRenewScheduler] Failed to renew watches: %v", err)
	}
}

// SetCheckInterval sets the check interval (for testing).
func (s *WatchRenewScheduler) SetCheckInterval(interval time.Duration) {
	s.checkInterval = interval
}
