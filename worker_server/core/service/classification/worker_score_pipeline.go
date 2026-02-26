// Package classification implements the score-based email classification pipeline.
package classification

import (
	"context"
	"time"

	"worker_server/core/agent/llm"
	"worker_server/core/domain"
)

// =============================================================================
// Score Pipeline (5-Stage Classification)
// =============================================================================

// ScorePipeline orchestrates the 7-stage score-based classification.
//
// Stage 0a: RFC Headers     (~30%)  → List-Unsubscribe, Precedence, ESP, Developer Service Headers
// Stage 0b: Domain          (~20%)  → Known domain matching (GitHub, Stripe, etc.)
// Stage 0c: Subject         (~15%)  → Subject pattern matching
// Stage 1: Sender Profile   (~12%)  → Importance score from engagement
// Stage 2: User Rules       (~10%)  → Domain/keyword matching
// Stage 3: Semantic Cache   (~7%)   → Embedding similarity
// Stage 4: LLM Fallback     (~6%)   → Last resort
type ScorePipeline struct {
	config *ScorePipelineConfig

	// Classifiers
	rfcClassifier     *RFCScoreClassifier
	domainClassifier  *DomainScoreClassifier
	subjectClassifier *SubjectScoreClassifier
	senderClassifier  *SenderScoreClassifier
	ruleClassifier    *UserRuleScoreClassifier
	cacheClassifier   *SemanticCacheClassifier
	llmClassifier     *LLMScoreClassifier

	// Cache manager (for storing LLM results)
	cacheManager *SemanticCacheManager

	// Auto labeling
	labelService *AutoLabelService
}

// ScorePipelineDeps holds dependencies for creating a ScorePipeline.
type ScorePipelineDeps struct {
	SenderProfileRepo       domain.SenderProfileRepository
	ClassificationRuleRepo  domain.ScoreClassificationRuleRepository
	ClassificationCacheRepo domain.ClassificationCacheRepository
	LabelRuleRepo           domain.LabelRuleRepository
	LabelRepo               domain.LabelRepository
	SettingsRepo            domain.SettingsRepository
	LLMClient               *llm.Client
}

// NewScorePipeline creates a new score-based classification pipeline.
func NewScorePipeline(deps *ScorePipelineDeps, config *ScorePipelineConfig) *ScorePipeline {
	if config == nil {
		config = DefaultScorePipelineConfig()
	}

	p := &ScorePipeline{
		config:            config,
		rfcClassifier:     NewRFCScoreClassifier(),
		domainClassifier:  NewDomainScoreClassifier(),
		subjectClassifier: NewSubjectScoreClassifier(),
		senderClassifier:  NewSenderScoreClassifier(deps.SenderProfileRepo),
		ruleClassifier:    NewUserRuleScoreClassifier(deps.ClassificationRuleRepo),
	}

	// Semantic cache (optional)
	if config.EnableSemanticCache && deps.ClassificationCacheRepo != nil {
		p.cacheClassifier = NewSemanticCacheClassifier(deps.ClassificationCacheRepo, config.SemanticCacheThreshold)
		p.cacheManager = NewSemanticCacheManager(deps.ClassificationCacheRepo, config.SemanticCacheThreshold)
	}

	// LLM fallback
	if deps.LLMClient != nil {
		p.llmClassifier = NewLLMScoreClassifier(deps.LLMClient, deps.SettingsRepo)
	}

	// Auto labeling (optional)
	if config.EnableAutoLabeling && deps.LabelRuleRepo != nil {
		p.labelService = NewAutoLabelService(deps.LabelRuleRepo, deps.LabelRepo, nil)
	}

	return p
}

// Classify runs the email through the 5-stage score pipeline.
func (p *ScorePipeline) Classify(ctx context.Context, input *ScoreClassifierInput) (*ClassificationPipelineResultV2, error) {
	startTime := time.Now()

	var allResults []*ScoreClassifierResult
	var bestResult *ScoreClassifierResult
	var allSignals []string

	// Stage 0a: RFC Headers (including developer service headers)
	if result, err := p.rfcClassifier.Classify(ctx, input); err == nil && result != nil {
		allResults = append(allResults, result)
		allSignals = append(allSignals, result.Signals...)
		if bestResult == nil || result.Score > bestResult.Score {
			bestResult = result
		}
		// Early exit for high-confidence RFC results
		if result.Score >= p.config.EarlyExitThreshold {
			return p.buildResult(bestResult, allResults, allSignals, startTime), nil
		}
	}

	// Stage 0b: Domain-based classification (known service domains)
	if result, err := p.domainClassifier.Classify(ctx, input); err == nil && result != nil {
		allResults = append(allResults, result)
		allSignals = append(allSignals, result.Signals...)
		if bestResult == nil || result.Score > bestResult.Score {
			bestResult = result
		}
		// Early exit for high-confidence domain results
		if result.Score >= p.config.EarlyExitThreshold {
			return p.buildResult(bestResult, allResults, allSignals, startTime), nil
		}
	}

	// Stage 0c: Subject pattern-based classification
	if result, err := p.subjectClassifier.Classify(ctx, input); err == nil && result != nil {
		allResults = append(allResults, result)
		allSignals = append(allSignals, result.Signals...)
		if bestResult == nil || result.Score > bestResult.Score {
			bestResult = result
		}
		// Early exit for high-confidence subject pattern results
		if result.Score >= p.config.EarlyExitThreshold {
			return p.buildResult(bestResult, allResults, allSignals, startTime), nil
		}
	}

	// Stage 1: Sender Profile
	if result, err := p.senderClassifier.Classify(ctx, input); err == nil && result != nil {
		allResults = append(allResults, result)
		allSignals = append(allSignals, result.Signals...)
		if bestResult == nil || result.Score > bestResult.Score {
			bestResult = result
		}
		if result.Score >= p.config.EarlyExitThreshold {
			return p.buildResult(bestResult, allResults, allSignals, startTime), nil
		}
	}

	// Stage 2: User Rules
	if p.ruleClassifier != nil {
		if result, err := p.ruleClassifier.Classify(ctx, input); err == nil && result != nil {
			allResults = append(allResults, result)
			allSignals = append(allSignals, result.Signals...)
			if bestResult == nil || result.Score > bestResult.Score {
				bestResult = result
			}
			if result.Score >= p.config.EarlyExitThreshold {
				return p.buildResult(bestResult, allResults, allSignals, startTime), nil
			}
		}
	}

	// Stage 3: Semantic Cache
	if p.cacheClassifier != nil && len(input.Embedding) > 0 {
		if result, err := p.cacheClassifier.Classify(ctx, input); err == nil && result != nil {
			allResults = append(allResults, result)
			allSignals = append(allSignals, result.Signals...)
			if bestResult == nil || result.Score > bestResult.Score {
				bestResult = result
			}
			// Cache hit with good score, no need for LLM
			if result.Score >= p.config.LLMFallbackThreshold {
				return p.buildResult(bestResult, allResults, allSignals, startTime), nil
			}
		}
	}

	// Stage 4: LLM Fallback (only if best score is below threshold)
	if p.llmClassifier != nil && (bestResult == nil || bestResult.Score < p.config.LLMFallbackThreshold) {
		if result, err := p.llmClassifier.Classify(ctx, input); err == nil && result != nil {
			allResults = append(allResults, result)
			allSignals = append(allSignals, result.Signals...)
			if bestResult == nil || result.Score > bestResult.Score {
				bestResult = result
			}

			// Store LLM result in cache for future use
			if p.cacheManager != nil && len(input.Embedding) > 0 {
				go func() {
					_ = p.cacheManager.Store(ctx, input, result)
				}()
			}
		}
	}

	// Default result if nothing matched
	if bestResult == nil {
		bestResult = &ScoreClassifierResult{
			Category: domain.CategoryOther,
			Priority: domain.PriorityNormal,
			Score:    0.50,
			Source:   "default",
			LLMUsed:  false,
		}
	}

	return p.buildResult(bestResult, allResults, allSignals, startTime), nil
}

// ClassifyWithAutoLabel runs classification and applies auto-labeling.
func (p *ScorePipeline) ClassifyWithAutoLabel(ctx context.Context, input *ScoreClassifierInput) (*ClassificationPipelineResultV2, error) {
	result, err := p.Classify(ctx, input)
	if err != nil {
		return nil, err
	}

	// Apply auto-labeling
	if p.labelService != nil {
		labels, err := p.labelService.ApplyLabels(ctx, input.UserID, input.Email, input.Embedding)
		if err == nil && len(labels) > 0 {
			// Merge labels
			labelMap := make(map[int64]bool)
			for _, l := range result.Labels {
				labelMap[l] = true
			}
			for _, l := range labels {
				labelMap[l] = true
			}

			result.Labels = make([]int64, 0, len(labelMap))
			for l := range labelMap {
				result.Labels = append(result.Labels, l)
			}
		}
	}

	return result, nil
}

// buildResult converts the best score result to the final pipeline result.
func (p *ScorePipeline) buildResult(best *ScoreClassifierResult, all []*ScoreClassifierResult, signals []string, startTime time.Time) *ClassificationPipelineResultV2 {
	result := &ClassificationPipelineResultV2{
		Category:         best.Category,
		SubCategory:      best.SubCategory,
		Priority:         best.Priority,
		Labels:           best.Labels,
		Score:            best.Score,
		Stage:            p.stageNameFromSource(best.Source),
		Source:           best.Source,
		AllResults:       all,
		Signals:          signals,
		LLMUsed:          best.LLMUsed,
		ProcessingTimeMs: time.Since(startTime).Milliseconds(),
	}

	return result
}

// stageNameFromSource extracts stage name from source string.
func (p *ScorePipeline) stageNameFromSource(source string) string {
	if len(source) == 0 {
		return "unknown"
	}

	// Source format: "stage:detail" (e.g., "rfc:list-unsubscribe", "sender:vip")
	for i, c := range source {
		if c == ':' {
			return source[:i]
		}
	}
	return source
}

// =============================================================================
// Pipeline Statistics
// =============================================================================

// PipelineStats holds statistics about classification pipeline performance.
type PipelineStats struct {
	TotalClassified int64
	ByStage         map[string]int64
	LLMUsed         int64
	AvgProcessingMs float64
	CacheHitRate    float64
}

// GetStats returns pipeline statistics (would be tracked in production).
func (p *ScorePipeline) GetStats() *PipelineStats {
	// In production, this would be tracked via metrics
	return &PipelineStats{
		ByStage: map[string]int64{
			"rfc":    0,
			"sender": 0,
			"rule":   0,
			"cache":  0,
			"llm":    0,
		},
	}
}
