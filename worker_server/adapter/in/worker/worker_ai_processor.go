package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/core/service/ai"
	"worker_server/pkg/logger"
)

// =============================================================================
// AI Processor - Optimized Batch Processing
// =============================================================================

// AIServiceInterface defines methods needed by AIProcessor
type AIServiceInterface interface {
	ClassifyEmailBatch(ctx context.Context, emailIDs []int64) ([]*domain.ClassificationResult, error)
	SummarizeEmailBatch(ctx context.Context, emailIDs []int64) (map[int64]string, error)
}

// AIProcessor handles AI-related jobs with batch optimization.
type AIProcessor struct {
	aiService        *ai.Service
	optimizedService *ai.OptimizedService
	emailRepo         out.EmailRepository
	realtime         out.RealtimePort

	// Batch accumulator
	classifyBatch  []int64
	summarizeBatch []int64
	batchMu        sync.Mutex
	batchSize      int
	batchTimeout   time.Duration
	lastBatchTime  time.Time

	// Processing control
	processingMu sync.Mutex
	isProcessing bool
}

// NewAIProcessor creates a new AI processor.
func NewAIProcessor(aiService *ai.Service, emailRepo out.EmailRepository, realtime out.RealtimePort) *AIProcessor {
	p := &AIProcessor{
		aiService:      aiService,
		emailRepo:       emailRepo,
		realtime:       realtime,
		classifyBatch:  make([]int64, 0, 10),
		summarizeBatch: make([]int64, 0, 10),
		batchSize:      10,              // 10개씩 배치 처리
		batchTimeout:   3 * time.Second, // 최대 3초 대기
		lastBatchTime:  time.Now(),
	}

	// Start batch flush goroutine
	go p.batchFlusher()

	return p
}

// NewAIProcessorOptimized creates an AI processor with optimized service.
func NewAIProcessorOptimized(optimizedService *ai.OptimizedService, emailRepo out.EmailRepository, realtime out.RealtimePort) *AIProcessor {
	p := &AIProcessor{
		optimizedService: optimizedService,
		emailRepo:         emailRepo,
		realtime:         realtime,
		classifyBatch:    make([]int64, 0, 10),
		summarizeBatch:   make([]int64, 0, 10),
		batchSize:        10,
		batchTimeout:     3 * time.Second,
		lastBatchTime:    time.Now(),
	}

	go p.batchFlusher()

	return p
}

// ProcessClassify handles single classify job - accumulates for batch
func (p *AIProcessor) ProcessClassify(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[AIClassifyPayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	// Skip check for optimized service
	if p.optimizedService != nil {
		if skip, _ := p.optimizedService.ShouldSkipClassification(ctx, payload.EmailID); skip {
			return nil
		}
	}

	// Add to batch
	p.batchMu.Lock()
	p.classifyBatch = append(p.classifyBatch, payload.EmailID)
	shouldProcess := len(p.classifyBatch) >= p.batchSize
	p.batchMu.Unlock()

	if shouldProcess {
		p.flushClassifyBatch(ctx)
	}

	return nil
}

// ProcessClassifyBatch handles batch classify job
func (p *AIProcessor) ProcessClassifyBatch(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[AIClassifyBatchPayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	log := logger.WithFields(map[string]any{
		"job":   "ai.classify_batch",
		"count": len(payload.EmailIDs),
	})

	var results []*domain.ClassificationResult
	if p.optimizedService != nil {
		results, err = p.optimizedService.ClassifyEmailBatch(ctx, payload.EmailIDs)
	} else if p.aiService != nil {
		results, err = p.aiService.ClassifyEmailBatch(ctx, payload.EmailIDs)
	} else {
		return fmt.Errorf("AI service not initialized")
	}

	if err != nil {
		log.WithError(err).Error("batch classify failed")
		return fmt.Errorf("batch classify failed: %w", err)
	}

	log.WithField("classified", len(results)).Debug("batch classified")
	return nil
}

// ProcessSummarize handles summarize job
func (p *AIProcessor) ProcessSummarize(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[AISummarizePayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	p.batchMu.Lock()
	p.summarizeBatch = append(p.summarizeBatch, payload.EmailID)
	shouldProcess := len(p.summarizeBatch) >= p.batchSize
	p.batchMu.Unlock()

	if shouldProcess {
		p.flushSummarizeBatch(ctx)
	}

	return nil
}

// =============================================================================
// Batch Processing
// =============================================================================

func (p *AIProcessor) flushClassifyBatch(ctx context.Context) {
	p.processingMu.Lock()
	if p.isProcessing {
		p.processingMu.Unlock()
		return
	}
	p.isProcessing = true
	p.processingMu.Unlock()

	defer func() {
		p.processingMu.Lock()
		p.isProcessing = false
		p.processingMu.Unlock()
	}()

	p.batchMu.Lock()
	if len(p.classifyBatch) == 0 {
		p.batchMu.Unlock()
		return
	}
	batch := make([]int64, len(p.classifyBatch))
	copy(batch, p.classifyBatch)
	p.classifyBatch = p.classifyBatch[:0]
	p.lastBatchTime = time.Now()
	p.batchMu.Unlock()

	log := logger.WithFields(map[string]any{
		"job":   "ai.classify_flush",
		"count": len(batch),
	})

	var results []*domain.ClassificationResult
	var err error

	if p.optimizedService != nil {
		results, err = p.optimizedService.ClassifyEmailBatch(ctx, batch)
	} else if p.aiService != nil {
		results, err = p.aiService.ClassifyEmailBatch(ctx, batch)
	}

	if err != nil {
		log.WithError(err).Error("batch classify failed")
		return
	}

	p.notifyClassificationComplete(ctx, results)
	log.WithField("classified", len(results)).Debug("batch flushed")
}

func (p *AIProcessor) flushSummarizeBatch(ctx context.Context) {
	p.batchMu.Lock()
	if len(p.summarizeBatch) == 0 {
		p.batchMu.Unlock()
		return
	}
	batch := make([]int64, len(p.summarizeBatch))
	copy(batch, p.summarizeBatch)
	p.summarizeBatch = p.summarizeBatch[:0]
	p.batchMu.Unlock()

	log := logger.WithFields(map[string]any{
		"job":   "ai.summarize_flush",
		"count": len(batch),
	})

	var summaries map[int64]string
	var err error

	if p.optimizedService != nil {
		summaries, err = p.optimizedService.SummarizeEmailBatch(ctx, batch)
	} else if p.aiService != nil {
		summaries = make(map[int64]string)
		for _, emailID := range batch {
			if summary, sErr := p.aiService.SummarizeEmail(ctx, emailID, false); sErr == nil {
				summaries[emailID] = summary
			}
		}
	}

	if err != nil {
		log.WithError(err).Error("batch summarize failed")
		return
	}

	p.notifySummarizationComplete(ctx, summaries)
	log.WithField("summarized", len(summaries)).Debug("batch flushed")
}

// batchFlusher periodically flushes accumulated batches
func (p *AIProcessor) batchFlusher() {
	ticker := time.NewTicker(p.batchTimeout)
	defer ticker.Stop()

	for range ticker.C {
		p.batchMu.Lock()
		hasClassify := len(p.classifyBatch) > 0
		hasSummarize := len(p.summarizeBatch) > 0
		timeSinceLast := time.Since(p.lastBatchTime)
		p.batchMu.Unlock()

		// Flush if timeout exceeded and there are pending items
		if timeSinceLast >= p.batchTimeout {
			ctx := context.Background()
			if hasClassify {
				p.flushClassifyBatch(ctx)
			}
			if hasSummarize {
				p.flushSummarizeBatch(ctx)
			}
		}
	}
}

// ProcessGenerateReply handles reply generation job
func (p *AIProcessor) ProcessGenerateReply(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[AIReplyPayload](msg)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	// TODO: Implement reply generation via aiService
	_ = payload
	return nil
}

// =============================================================================
// Payload Types
// =============================================================================

// AISummarizePayload for summarize jobs
type AISummarizePayload struct {
	EmailID int64 `json:"email_id"`
}

// AIReplyPayload for reply generation jobs
type AIReplyPayload struct {
	EmailID      int64  `json:"email_id"`
	Instructions string `json:"instructions,omitempty"`
}

// =============================================================================
// Realtime Notifications (Phase 3)
// =============================================================================

// notifyClassificationComplete sends realtime notification for classified emails
func (p *AIProcessor) notifyClassificationComplete(ctx context.Context, results []*domain.ClassificationResult) {
	if p.realtime == nil || p.emailRepo == nil || len(results) == 0 {
		return
	}

	for _, result := range results {
		email, err := p.emailRepo.GetByID(ctx, result.EmailID)
		if err != nil || email == nil {
			continue
		}

		category := ""
		priority := ""
		if result.Category != nil {
			category = string(*result.Category)
		}
		if result.Priority != nil {
			priority = result.Priority.String()
		}

		event := &domain.RealtimeEvent{
			Type:      domain.EventEmailClassified,
			Timestamp: time.Now(),
			Data: &domain.ClassifiedData{
				EmailID:    result.EmailID,
				Category:   category,
				Priority:   priority,
				Confidence: result.Score,
			},
		}

		p.realtime.Push(ctx, email.UserID.String(), event)
	}
}

// notifySummarizationComplete sends realtime notification for summarized emails
func (p *AIProcessor) notifySummarizationComplete(ctx context.Context, summaries map[int64]string) {
	if p.realtime == nil || p.emailRepo == nil || len(summaries) == 0 {
		return
	}

	for emailID, summary := range summaries {
		email, err := p.emailRepo.GetByID(ctx, emailID)
		if err != nil || email == nil {
			continue
		}

		event := &domain.RealtimeEvent{
			Type:      domain.EventEmailSummarized,
			Timestamp: time.Now(),
			Data: &domain.SummarizedData{
				EmailID: emailID,
				Summary: summary,
			},
		}

		p.realtime.Push(ctx, email.UserID.String(), event)
	}
}
