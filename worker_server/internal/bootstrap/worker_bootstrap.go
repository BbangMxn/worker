package bootstrap

import (
	"context"
	"os"
	"sync"

	"github.com/goccy/go-json"

	"worker_server/adapter/in/worker"
	"worker_server/adapter/out/messaging"
	"worker_server/config"
	"worker_server/pkg/logger"

	"github.com/rs/zerolog"
)

type Worker struct {
	pool                *worker.Pool
	consumer            *messaging.Consumer
	deps                *Dependencies
	ctx                 context.Context
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
	zlog                zerolog.Logger
	syncRetryScheduler  *worker.SyncRetryScheduler
	watchRenewScheduler *worker.WatchRenewScheduler
	gapSyncScheduler    *worker.GapSyncScheduler
}

func NewWorker(cfg *config.Config) (*Worker, func(), error) {
	deps, cleanup, err := NewDependencies(cfg)
	if err != nil {
		return nil, nil, err
	}

	// Logger
	zlog := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).
		With().Timestamp().Str("component", "worker").Logger()

	// Create processors with new services
	mailProcessor := worker.NewMailProcessor(
		deps.OAuthService,
		deps.MailSyncService, // 새로운 MailSyncService
		deps.GmailProvider,
		deps.MailRepo,
		deps.MailBodyRepo,
		deps.MessageProducer,
		deps.RealtimeAdapter,
	)
	aiProcessor := worker.NewAIProcessor(deps.AIService, deps.MailRepo, deps.RealtimeAdapter)
	ragProcessor := worker.NewRAGProcessor(deps.RAGIndexer, deps.StyleAnalyzer, deps.MailRepo, deps.MailBodyRepo)
	calendarProcessor := worker.NewCalendarProcessor(deps.CalendarSyncService)
	webhookProcessor := worker.NewWebhookProcessor(deps.WebhookService)

	// Create handler
	handler := worker.NewHandler(
		mailProcessor,
		aiProcessor,
		ragProcessor,
		calendarProcessor,
		webhookProcessor,
	)

	// Create intelligent pool with config (use DefaultPoolConfig as base)
	defaultConfig := worker.DefaultPoolConfig()
	poolConfig := &worker.PoolConfig{
		MinWorkers:         cfg.WorkerMin,
		MaxWorkers:         cfg.WorkerMax,
		QueueSize:          1000,
		ScaleUpThreshold:   0.8,
		ScaleDownThreshold: 0.2,
		ScaleInterval:      cfg.WorkerScaleInterval,
		IdleTimeout:        cfg.WorkerIdleTimeout,
		JobTimeout:         defaultConfig.JobTimeout,       // 기본 타임아웃 상속
		JobTimeoutByType:   defaultConfig.JobTimeoutByType, // 작업별 타임아웃 상속
	}

	// Fallback defaults
	if poolConfig.MinWorkers == 0 {
		poolConfig.MinWorkers = 4
	}
	if poolConfig.MaxWorkers == 0 {
		poolConfig.MaxWorkers = 16
	}
	if poolConfig.ScaleInterval == 0 {
		poolConfig.ScaleInterval = defaultConfig.ScaleInterval
	}
	if poolConfig.IdleTimeout == 0 {
		poolConfig.IdleTimeout = defaultConfig.IdleTimeout
	}

	pool := worker.NewPool(handler, poolConfig, zlog)

	ctx, cancel := context.WithCancel(context.Background())

	// Create schedulers for sync retry, watch renewal, and gap sync
	var syncRetryScheduler *worker.SyncRetryScheduler
	var watchRenewScheduler *worker.WatchRenewScheduler
	var gapSyncScheduler *worker.GapSyncScheduler

	if deps.SyncStateRepo != nil && deps.MailSyncService != nil {
		syncRetryScheduler = worker.NewSyncRetryScheduler(deps.SyncStateRepo, deps.MailSyncService)
		watchRenewScheduler = worker.NewWatchRenewScheduler(deps.MailSyncService)
		gapSyncScheduler = worker.NewGapSyncScheduler(deps.SyncStateRepo, deps.MailSyncService)
		logger.Info("Sync schedulers configured (retry, watch renew, gap sync)")
	}

	w := &Worker{
		pool:                pool,
		deps:                deps,
		ctx:                 ctx,
		cancel:              cancel,
		zlog:                zlog,
		syncRetryScheduler:  syncRetryScheduler,
		watchRenewScheduler: watchRenewScheduler,
		gapSyncScheduler:    gapSyncScheduler,
	}

	// Redis Stream Consumer 설정 (Redis가 있을 때만)
	if deps.Redis != nil {
		// 모든 스트림 목록
		streams := []string{
			messaging.StreamMailSync,
			messaging.StreamMailSend,
			messaging.StreamMailBatch,
			messaging.StreamMailSave,   // 메일 저장 스트림
			messaging.StreamMailModify, // 메일 상태 변경 + SSE 브로드캐스트
			messaging.StreamCalendarSync,
			messaging.StreamAIClassify,
			messaging.StreamAISummarize,
			messaging.StreamAIGenerateReply,
			messaging.StreamRAGIndex,
			messaging.StreamRAGBatchIndex,
		}

		w.consumer = messaging.NewConsumer(deps.Redis, &messaging.ConsumerConfig{
			Group:    "workspace-workers",
			Consumer: cfg.WorkerID,
			Streams:  streams,
			Handler:  &streamHandler{worker: w},
			Logger:   zlog,
		})
		logger.Info("Redis Stream Consumer configured for %d streams", len(streams))
	} else {
		logger.Warn("Redis not available, worker will only process direct submissions")
	}

	return w, cleanup, nil
}

// streamHandler adapts Redis Stream messages to Worker Pool
type streamHandler struct {
	worker *Worker
}

func (h *streamHandler) Handle(ctx context.Context, stream string, data []byte) error {
	logger.Info("[StreamHandler] Received message from stream: %s", stream)

	// Parse the job data
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		logger.Error("[StreamHandler] Failed to parse payload: %v", err)
		return err
	}

	// Map stream to job type
	jobType := streamToJobType(stream)
	logger.Info("[StreamHandler] Job type: %s, payload: %v", jobType, payload)

	// Create worker message
	msg := worker.NewMessage(jobType, payload)

	// Submit to pool
	if !h.worker.pool.Submit(msg) {
		logger.Error("[StreamHandler] Failed to submit job to pool: %s", jobType)
	} else {
		logger.Info("[StreamHandler] Job submitted to pool: %s", jobType)
	}

	return nil
}

// streamToJobType maps Redis stream names to job types
func streamToJobType(stream string) string {
	switch stream {
	case messaging.StreamMailSync:
		return worker.JobMailSync
	case messaging.StreamMailSend:
		return worker.JobMailSend
	case messaging.StreamMailBatch:
		return worker.JobMailBatch
	case messaging.StreamMailSave:
		return worker.JobMailSave
	case messaging.StreamMailModify:
		return worker.JobMailModify
	case messaging.StreamCalendarSync:
		return worker.JobCalendarSync
	case messaging.StreamAIClassify:
		return worker.JobAIClassify
	case messaging.StreamAISummarize:
		return worker.JobAISummarize
	case messaging.StreamAIGenerateReply:
		return worker.JobAIReply
	case messaging.StreamRAGIndex:
		return worker.JobRAGIndex
	case messaging.StreamRAGBatchIndex:
		return worker.JobRAGBatchIndex
	default:
		return stream
	}
}

func (w *Worker) Start() {
	// Worker Pool 시작
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.pool.Start()
	}()

	// Redis Stream Consumer 시작 (있을 경우)
	if w.consumer != nil {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.zlog.Info().Msg("Starting Redis Stream Consumer...")
			if err := w.consumer.Run(w.ctx); err != nil && err != context.Canceled {
				w.zlog.Error().Err(err).Msg("Redis Stream Consumer error")
			}
		}()
	}

	// Sync Retry Scheduler 시작
	if w.syncRetryScheduler != nil {
		w.syncRetryScheduler.Start()
		w.zlog.Info().Msg("Started Sync Retry Scheduler")
	}

	// Watch Renew Scheduler 시작
	if w.watchRenewScheduler != nil {
		w.watchRenewScheduler.Start()
		w.zlog.Info().Msg("Started Watch Renew Scheduler")
	}

	// Gap Sync Scheduler 시작 (서버 시작 시 모든 연결 gap 체크)
	if w.gapSyncScheduler != nil {
		w.gapSyncScheduler.Start()
		w.zlog.Info().Msg("Started Gap Sync Scheduler")
	}

	// Block until context is cancelled
	<-w.ctx.Done()
}

func (w *Worker) Stop() {
	w.cancel()

	// Stop schedulers
	if w.syncRetryScheduler != nil {
		w.syncRetryScheduler.Stop()
	}
	if w.watchRenewScheduler != nil {
		w.watchRenewScheduler.Stop()
	}
	if w.gapSyncScheduler != nil {
		w.gapSyncScheduler.Stop()
	}

	w.pool.Stop()
	w.wg.Wait()
}

func (w *Worker) Submit(msg *worker.Message) bool {
	if msg.IsPriority() {
		return w.pool.SubmitPriority(msg)
	}
	return w.pool.Submit(msg)
}

func (w *Worker) SubmitPriority(msg *worker.Message) bool {
	return w.pool.SubmitPriority(msg)
}

func (w *Worker) GetMetrics() worker.PoolMetrics {
	return w.pool.GetMetrics()
}

func (w *Worker) Dependencies() *Dependencies {
	return w.deps
}
