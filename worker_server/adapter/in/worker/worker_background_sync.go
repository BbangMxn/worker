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
// BackgroundSyncScheduler - 백그라운드 점진적 동기화 (하이브리드 방식)
// =============================================================================
//
// Initial Sync에서 200개만 가져온 후, 백그라운드에서 나머지 메일을 점진적으로 동기화합니다.
// 체크포인트가 저장된 연결들을 찾아서 계속 동기화합니다.

const (
	BackgroundSyncBatchSize = 100 // 한 번에 가져올 메일 수
	BackgroundSyncInterval  = 1 * time.Minute
	BackgroundSyncMaxPerRun = 500 // 한 번 실행에 최대 동기화할 메일 수
)

type BackgroundSyncScheduler struct {
	syncRepo        out.SyncStateRepository
	mailSyncService *mail.SyncService
	messageProducer out.MessageProducer
	checkInterval   time.Duration
	ctx             context.Context
	cancel          context.CancelFunc
}

// NewBackgroundSyncScheduler creates a new background sync scheduler.
func NewBackgroundSyncScheduler(
	syncRepo out.SyncStateRepository,
	mailSyncService *mail.SyncService,
	messageProducer out.MessageProducer,
) *BackgroundSyncScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &BackgroundSyncScheduler{
		syncRepo:        syncRepo,
		mailSyncService: mailSyncService,
		messageProducer: messageProducer,
		checkInterval:   BackgroundSyncInterval,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start starts the background sync scheduler.
func (s *BackgroundSyncScheduler) Start() {
	logger.Info("[BackgroundSyncScheduler] Starting...")
	go s.run()
}

// Stop stops the background sync scheduler.
func (s *BackgroundSyncScheduler) Stop() {
	logger.Info("[BackgroundSyncScheduler] Stopping...")
	s.cancel()
}

// run is the main loop.
func (s *BackgroundSyncScheduler) run() {
	// 시작 후 30초 대기 (서버 안정화)
	time.Sleep(30 * time.Second)

	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			logger.Info("[BackgroundSyncScheduler] Stopped")
			return
		case <-ticker.C:
			s.processIncompleteSync()
		}
	}
}

// processIncompleteSync finds connections with saved checkpoints and continues syncing.
func (s *BackgroundSyncScheduler) processIncompleteSync() {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	// 체크포인트가 있는 연결들 조회 (Initial Sync 미완료)
	states, err := s.syncRepo.GetAllWithCheckpoint(ctx)
	if err != nil {
		logger.Error("[BackgroundSyncScheduler] Failed to get connections with checkpoint: %v", err)
		return
	}

	if len(states) == 0 {
		return // 진행할 동기화 없음
	}

	logger.Info("[BackgroundSyncScheduler] Found %d connections with pending sync", len(states))

	// 각 연결에 대해 백그라운드 동기화 작업 발행
	for _, state := range states {
		// syncing 상태가 아닌 것만 처리 (이미 동기화 중인 건 스킵)
		if state.Status == domain.SyncStatusSyncing {
			continue
		}

		// 백그라운드 동기화 작업 발행
		if s.messageProducer != nil {
			job := &out.MailSyncJob{
				UserID:       state.UserID,
				ConnectionID: state.ConnectionID,
				Provider:     string(state.Provider),
				FullSync:     false,
				Background:   true, // 백그라운드 동기화 표시
			}
			if err := s.messageProducer.PublishMailSync(ctx, job); err != nil {
				logger.Error("[BackgroundSyncScheduler] Failed to publish sync job for connection %d: %v",
					state.ConnectionID, err)
			} else {
				logger.Info("[BackgroundSyncScheduler] Published background sync job for connection %d",
					state.ConnectionID)
			}
		}
	}
}

// SetCheckInterval sets the check interval (for testing).
func (s *BackgroundSyncScheduler) SetCheckInterval(interval time.Duration) {
	s.checkInterval = interval
}
