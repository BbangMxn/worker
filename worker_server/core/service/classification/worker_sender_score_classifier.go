// Package classification implements the score-based email classification pipeline.
package classification

import (
	"context"
	"time"

	"worker_server/core/domain"
)

// =============================================================================
// Sender Profile Score Classifier (Stage 1)
// =============================================================================

// SenderScoreClassifier performs Stage 1 classification based on sender profile engagement signals.
//
// Based on research from:
// - Gmail Priority Inbox (Google Research): Social features are strongest predictors
// - SIGIR: Historical interaction count is most important for reply prediction
// - Superhuman: VIP designation for important people
//
// Priority mapping (Eisenhower Matrix inspired):
// - Score >= 0.70: PriorityHigh (Important, likely needs action)
// - Score >= 0.50: PriorityNormal (Relevant, worth reading)
// - Score >= 0.30: PriorityLow (Can be deferred)
// - Score < 0.30:  PriorityLowest (Background noise)
type SenderScoreClassifier struct {
	senderProfileRepo domain.SenderProfileRepository
}

// NewSenderScoreClassifier creates a new sender profile score classifier.
func NewSenderScoreClassifier(senderProfileRepo domain.SenderProfileRepository) *SenderScoreClassifier {
	return &SenderScoreClassifier{
		senderProfileRepo: senderProfileRepo,
	}
}

// Name returns the classifier name.
func (c *SenderScoreClassifier) Name() string {
	return "sender"
}

// Stage returns the pipeline stage number.
func (c *SenderScoreClassifier) Stage() int {
	return 1
}

// Classify performs sender profile-based classification.
func (c *SenderScoreClassifier) Classify(ctx context.Context, input *ScoreClassifierInput) (*ScoreClassifierResult, error) {
	if c.senderProfileRepo == nil {
		return nil, nil
	}

	// Get sender profile
	profile, err := c.senderProfileRepo.GetByEmail(input.UserID, input.Email.FromEmail)
	if err != nil || profile == nil {
		return nil, nil
	}

	var signals []string

	// === VIP check (highest priority - Superhuman style) ===
	if profile.IsVIP {
		signals = append(signals, SignalVIP)
		return &ScoreClassifierResult{
			Category: domain.CategoryWork,
			Priority: domain.PriorityUrgent,
			Score:    0.98,
			Source:   "sender:vip",
			Signals:  signals,
			LLMUsed:  false,
		}, nil
	}

	// === Muted check (lowest priority) ===
	if profile.IsMuted {
		signals = append(signals, SignalMuted)
		return &ScoreClassifierResult{
			Category: domain.CategoryOther,
			Priority: domain.PriorityLowest,
			Score:    0.96, // High classifier score to take priority, but low email priority
			Source:   "sender:muted",
			Signals:  signals,
			LLMUsed:  false,
		}, nil
	}

	// === Calculate importance score from engagement signals ===
	importanceScore := profile.CalculateImportanceScore()

	// === Collect signals for debugging/transparency ===
	signals = c.collectSignals(profile)

	// === Determine category and priority based on importance score ===
	category, priority := c.scoreToClassification(profile, importanceScore)

	// If learned category exists, use it (user correction)
	if profile.LearnedCategory != nil {
		category = *profile.LearnedCategory
	}

	// Build result
	result := &ScoreClassifierResult{
		Category: category,
		Priority: priority,
		Score:    importanceScore,
		Source:   c.determineSource(profile, importanceScore),
		Signals:  signals,
		LLMUsed:  false,
	}

	// Use learned sub-category if available
	if profile.LearnedSubCategory != nil {
		result.SubCategory = profile.LearnedSubCategory
	}

	// Add confirmed labels
	if len(profile.ConfirmedLabels) > 0 {
		result.Labels = profile.ConfirmedLabels
	}

	return result, nil
}

// collectSignals gathers all detected signals for transparency.
func (c *SenderScoreClassifier) collectSignals(profile *domain.SenderProfile) []string {
	var signals []string

	// Contact signal
	if profile.IsContact {
		signals = append(signals, SignalContact)
	}

	// Reply rate signals (strongest predictor per SIGIR research)
	if profile.ReplyRate >= 0.5 {
		signals = append(signals, SignalHighReplyRate)
	}

	// Read rate signals
	if profile.ReadRate >= 0.8 {
		signals = append(signals, SignalHighReadRate)
	} else if profile.ReadRate < 0.2 && profile.EmailCount > 5 {
		signals = append(signals, SignalLowReadRate)
	}

	// Delete rate signals
	if profile.DeleteRate >= 0.5 {
		signals = append(signals, SignalHighDeleteRate)
	}

	// Recency signals
	if !profile.LastSeenAt.IsZero() {
		daysSince := time.Since(profile.LastSeenAt).Hours() / 24
		if daysSince < 7 {
			signals = append(signals, SignalRecentSender)
		}
	}

	// Frequency signals
	if profile.EmailCount > 50 {
		signals = append(signals, SignalFrequentSender)
	}

	return signals
}

// determineSource returns a descriptive source based on the strongest signal.
func (c *SenderScoreClassifier) determineSource(profile *domain.SenderProfile, score float64) string {
	// High engagement signals
	if profile.ReplyRate >= 0.5 {
		return "sender:high-reply-rate"
	}
	if profile.IsContact && score >= 0.60 {
		return "sender:contact"
	}
	if profile.ReadRate >= 0.8 {
		return "sender:high-read-rate"
	}

	// Low engagement signals
	if profile.DeleteRate >= 0.5 {
		return "sender:high-delete-rate"
	}
	if profile.ReadRate < 0.2 && profile.EmailCount > 5 {
		return "sender:low-read-rate"
	}

	return "sender:engagement"
}

// scoreToClassification converts importance score to category and priority.
//
// Threshold design based on Eisenhower Matrix:
// - >= 0.70: Q1/Q2 (Important) → High priority
// - >= 0.50: Q3 (Urgent but less important) → Normal priority
// - >= 0.30: Q4 (Neither) → Low priority
// - < 0.30: Background → Lowest priority
func (c *SenderScoreClassifier) scoreToClassification(profile *domain.SenderProfile, score float64) (domain.EmailCategory, domain.Priority) {
	// === High importance (score >= 0.70) ===
	// Likely needs action - Q1/Q2 in Eisenhower Matrix
	if score >= 0.70 {
		// High reply rate = definitely work/actionable
		if profile.ReplyRate >= 0.3 {
			return domain.CategoryWork, domain.PriorityHigh
		}
		// Contact with high engagement = personal importance
		if profile.IsContact {
			return domain.CategoryPersonal, domain.PriorityHigh
		}
		return domain.CategoryWork, domain.PriorityHigh
	}

	// === Medium-high importance (score >= 0.50) ===
	// Worth reading - relevant communication
	if score >= 0.50 {
		if profile.ReplyRate >= 0.2 {
			return domain.CategoryWork, domain.PriorityNormal
		}
		if profile.IsContact {
			return domain.CategoryPersonal, domain.PriorityNormal
		}
		return domain.CategoryPersonal, domain.PriorityNormal
	}

	// === Medium importance (score >= 0.30) ===
	// Can be deferred
	if score >= 0.30 {
		// High delete rate = not valuable
		if profile.DeleteRate >= 0.3 {
			return domain.CategoryOther, domain.PriorityLow
		}
		return domain.CategoryOther, domain.PriorityNormal
	}

	// === Low importance (score < 0.30) ===
	// Background noise
	if score >= 0.15 {
		return domain.CategoryOther, domain.PriorityLow
	}

	// Very low - likely spam/noise
	return domain.CategoryOther, domain.PriorityLowest
}

// =============================================================================
// Sender Profile Updater
// =============================================================================

// SenderProfileUpdater updates sender profiles based on email interactions.
type SenderProfileUpdater struct {
	repo domain.SenderProfileRepository
}

// NewSenderProfileUpdater creates a new sender profile updater.
func NewSenderProfileUpdater(repo domain.SenderProfileRepository) *SenderProfileUpdater {
	return &SenderProfileUpdater{repo: repo}
}

// OnEmailReceived updates profile when a new email is received.
func (u *SenderProfileUpdater) OnEmailReceived(ctx context.Context, email *domain.Email) error {
	if u.repo == nil {
		return nil
	}

	profile, err := u.repo.GetByEmail(email.UserID, email.FromEmail)
	if err != nil {
		return err
	}

	if profile == nil {
		// Create new profile
		profile = &domain.SenderProfile{
			UserID:      email.UserID,
			Email:       email.FromEmail,
			Domain:      extractDomain(email.FromEmail),
			EmailCount:  1,
			FirstSeenAt: email.ReceivedAt,
			LastSeenAt:  email.ReceivedAt,
		}
		return u.repo.Create(profile)
	}

	// Update existing profile
	if err := u.repo.IncrementEmailCount(profile.ID); err != nil {
		return err
	}

	return u.repo.UpdateLastSeen(profile.ID, email.ReceivedAt)
}

// OnEmailRead updates profile when an email is read.
func (u *SenderProfileUpdater) OnEmailRead(ctx context.Context, email *domain.Email) error {
	if u.repo == nil {
		return nil
	}

	profile, err := u.repo.GetByEmail(email.UserID, email.FromEmail)
	if err != nil || profile == nil {
		return err
	}

	// Calculate new read rate
	// newRate = (oldRate * oldCount + 1) / newCount
	if profile.EmailCount > 0 {
		newRate := (profile.ReadRate*float64(profile.EmailCount-1) + 1) / float64(profile.EmailCount)
		if err := u.repo.UpdateReadRate(profile.ID, newRate); err != nil {
			return err
		}
	}

	return u.repo.IncrementInteractionCount(profile.ID)
}

// OnEmailReplied updates profile when an email is replied to.
func (u *SenderProfileUpdater) OnEmailReplied(ctx context.Context, email *domain.Email) error {
	if u.repo == nil {
		return nil
	}

	profile, err := u.repo.GetByEmail(email.UserID, email.FromEmail)
	if err != nil || profile == nil {
		return err
	}

	// Calculate new reply rate
	if profile.EmailCount > 0 {
		newRate := (profile.ReplyRate*float64(profile.EmailCount-1) + 1) / float64(profile.EmailCount)
		if err := u.repo.UpdateReplyRate(profile.ID, newRate); err != nil {
			return err
		}
	}

	return u.repo.IncrementInteractionCount(profile.ID)
}

// OnEmailDeleted updates profile when an email is deleted.
func (u *SenderProfileUpdater) OnEmailDeleted(ctx context.Context, email *domain.Email) error {
	if u.repo == nil {
		return nil
	}

	profile, err := u.repo.GetByEmail(email.UserID, email.FromEmail)
	if err != nil || profile == nil {
		return err
	}

	// Calculate new delete rate
	if profile.EmailCount > 0 {
		newRate := (profile.DeleteRate*float64(profile.EmailCount-1) + 1) / float64(profile.EmailCount)
		return u.repo.UpdateDeleteRate(profile.ID, newRate)
	}

	return nil
}
