// Package classification implements the score-based email classification pipeline.
package classification

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

// =============================================================================
// Auto Label Service
// =============================================================================

// AutoLabelService handles automatic email labeling based on rules and learning.
type AutoLabelService struct {
	ruleRepo  domain.LabelRuleRepository
	labelRepo domain.LabelRepository
	emailRepo EmailRepository // for getting similar emails
}

// EmailRepository is a minimal interface for email operations needed by auto-labeling.
type EmailRepository interface {
	GetByID(ctx context.Context, emailID int64) (*domain.Email, error)
	GetByLabel(ctx context.Context, userID uuid.UUID, labelID int64, limit int) ([]*domain.Email, error)
	GetEmbedding(ctx context.Context, emailID int64) ([]float32, error)
}

// NewAutoLabelService creates a new auto label service.
func NewAutoLabelService(
	ruleRepo domain.LabelRuleRepository,
	labelRepo domain.LabelRepository,
	emailRepo EmailRepository,
) *AutoLabelService {
	return &AutoLabelService{
		ruleRepo:  ruleRepo,
		labelRepo: labelRepo,
		emailRepo: emailRepo,
	}
}

// ApplyLabels applies auto-labeling rules to an email.
// Returns the list of label IDs that should be applied.
func (s *AutoLabelService) ApplyLabels(ctx context.Context, userID uuid.UUID, email *domain.Email, embedding []float32) ([]int64, error) {
	if s.ruleRepo == nil {
		return nil, nil
	}

	// Get active rules for user
	rules, err := s.ruleRepo.ListActiveByUser(ctx, userID)
	if err != nil || len(rules) == 0 {
		return nil, nil
	}

	// Prepare email fields for matching
	senderLower := strings.ToLower(email.FromEmail)
	senderDomain := extractDomain(email.FromEmail)
	subjectLower := strings.ToLower(email.Subject)
	bodyLower := strings.ToLower(email.Snippet)

	// Track which labels to apply
	labelScores := make(map[int64]float64) // labelID -> best score

	// Group rules by type for efficient processing
	for _, rule := range rules {
		var matched bool
		var score float64

		switch rule.Type {
		case domain.LabelRuleExactSender:
			patternLower := strings.ToLower(rule.Pattern)
			if senderLower == patternLower || strings.Contains(senderLower, patternLower) {
				matched = true
				score = rule.Score
			}

		case domain.LabelRuleSenderDomain:
			patternLower := strings.ToLower(strings.TrimPrefix(rule.Pattern, "@"))
			if senderDomain == patternLower || strings.HasSuffix(senderDomain, "."+patternLower) {
				matched = true
				score = rule.Score
			}

		case domain.LabelRuleSubjectKeyword:
			patternLower := strings.ToLower(rule.Pattern)
			if strings.Contains(subjectLower, patternLower) {
				matched = true
				score = rule.Score
			}

		case domain.LabelRuleBodyKeyword:
			patternLower := strings.ToLower(rule.Pattern)
			if strings.Contains(bodyLower, patternLower) {
				matched = true
				score = rule.Score
			}

		case domain.LabelRuleEmbedding:
			// Pattern format: "ref:{email_id}"
			if len(embedding) > 0 && s.emailRepo != nil {
				refEmailID := parseRefEmailID(rule.Pattern)
				if refEmailID > 0 {
					refEmbedding, err := s.emailRepo.GetEmbedding(ctx, refEmailID)
					if err == nil && len(refEmbedding) > 0 {
						similarity := CosineSimilarity(embedding, refEmbedding)
						if similarity >= 0.90 { // High threshold for embedding match
							matched = true
							score = rule.Score * similarity
						}
					}
				}
			}

		case domain.LabelRuleAIPrompt:
			// AI prompt rules are not applied automatically
			// They require explicit LLM evaluation
			continue
		}

		if matched {
			// Update hit count asynchronously
			go func(ruleID int64) {
				_ = s.ruleRepo.IncrementHitCount(ctx, ruleID)
			}(rule.ID)

			// Keep the highest score for each label
			if existing, ok := labelScores[rule.LabelID]; !ok || score > existing {
				labelScores[rule.LabelID] = score
			}
		}
	}

	// Collect labels with score >= 0.85
	var labels []int64
	for labelID, score := range labelScores {
		if score >= 0.85 {
			labels = append(labels, labelID)
		}
	}

	return labels, nil
}

// OnUserAddLabel is called when a user manually adds a label to an email.
// This learns patterns from the email and creates auto-label rules.
func (s *AutoLabelService) OnUserAddLabel(ctx context.Context, userID uuid.UUID, emailID, labelID int64) error {
	if s.ruleRepo == nil || s.emailRepo == nil {
		return nil
	}

	// Get the email
	email, err := s.emailRepo.GetByID(ctx, emailID)
	if err != nil || email == nil {
		return err
	}

	// Check if exact sender rule already exists
	existingRule, _ := s.ruleRepo.FindByPattern(ctx, userID, labelID, domain.LabelRuleExactSender, email.FromEmail)
	if existingRule != nil {
		// Rule already exists, skip
		return nil
	}

	// Get emails with the same label for pattern analysis
	sameLabeled, err := s.emailRepo.GetByLabel(ctx, userID, labelID, 100)
	if err != nil {
		sameLabeled = []*domain.Email{}
	}

	// Extract patterns
	patterns := s.extractPatterns(email, sameLabeled)

	// Create rules for high-confidence patterns
	for _, pattern := range patterns {
		if pattern.Confidence >= 0.85 {
			rule := &domain.LabelRule{
				UserID:        userID,
				LabelID:       labelID,
				Type:          pattern.Type,
				Pattern:       pattern.Value,
				Score:         pattern.Confidence,
				IsAutoCreated: true,
				IsActive:      true,
			}

			// Check if rule already exists
			existing, _ := s.ruleRepo.FindByPattern(ctx, userID, labelID, pattern.Type, pattern.Value)
			if existing == nil {
				_ = s.ruleRepo.Create(ctx, rule)
			}
		}
	}

	// Create embedding-based rule if embedding exists
	embedding, err := s.emailRepo.GetEmbedding(ctx, emailID)
	if err == nil && len(embedding) > 0 {
		embeddingRule := &domain.LabelRule{
			UserID:        userID,
			LabelID:       labelID,
			Type:          domain.LabelRuleEmbedding,
			Pattern:       fmt.Sprintf("ref:%d", emailID),
			Score:         0.90,
			IsAutoCreated: true,
			IsActive:      true,
		}

		existing, _ := s.ruleRepo.FindByPattern(ctx, userID, labelID, domain.LabelRuleEmbedding, embeddingRule.Pattern)
		if existing == nil {
			_ = s.ruleRepo.Create(ctx, embeddingRule)
		}
	}

	return nil
}

// OnUserRemoveLabel is called when a user removes a label from an email.
// This can be used to adjust rules (not implemented yet).
func (s *AutoLabelService) OnUserRemoveLabel(ctx context.Context, userID uuid.UUID, emailID, labelID int64) error {
	// Could implement negative learning here
	// For now, we don't adjust rules on removal
	return nil
}

// =============================================================================
// Pattern Extraction
// =============================================================================

// Pattern represents an extracted pattern from email analysis.
type Pattern struct {
	Type       domain.LabelRuleType
	Value      string
	Confidence float64
}

// extractPatterns analyzes an email and similar emails to extract labeling patterns.
func (s *AutoLabelService) extractPatterns(email *domain.Email, sameLabeled []*domain.Email) []Pattern {
	var patterns []Pattern

	// Analyze sender frequency
	senderCount := s.countSender(email.FromEmail, sameLabeled)
	if senderCount >= 2 || (len(sameLabeled) > 0 && float64(senderCount)/float64(len(sameLabeled)) > 0.3) {
		confidence := 0.80 + float64(senderCount)*0.05
		if confidence > 0.99 {
			confidence = 0.99
		}
		patterns = append(patterns, Pattern{
			Type:       domain.LabelRuleExactSender,
			Value:      email.FromEmail,
			Confidence: confidence,
		})
	}

	// Analyze domain frequency
	senderDomain := extractDomain(email.FromEmail)
	domainCount := s.countDomain(senderDomain, sameLabeled)
	if domainCount >= 3 || (len(sameLabeled) > 0 && float64(domainCount)/float64(len(sameLabeled)) > 0.4) {
		confidence := 0.75 + float64(domainCount)*0.03
		if confidence > 0.95 {
			confidence = 0.95
		}
		patterns = append(patterns, Pattern{
			Type:       domain.LabelRuleSenderDomain,
			Value:      senderDomain,
			Confidence: confidence,
		})
	}

	// Analyze subject keywords (simple TF-IDF-like approach)
	keywords := s.extractKeywords(email.Subject, sameLabeled)
	for _, kw := range keywords {
		if kw.Confidence >= 0.80 {
			patterns = append(patterns, Pattern{
				Type:       domain.LabelRuleSubjectKeyword,
				Value:      kw.Value,
				Confidence: kw.Confidence,
			})
		}
	}

	return patterns
}

// countSender counts how many emails are from the same sender.
func (s *AutoLabelService) countSender(sender string, emails []*domain.Email) int {
	count := 1 // Include the current email
	senderLower := strings.ToLower(sender)

	for _, e := range emails {
		if strings.ToLower(e.FromEmail) == senderLower {
			count++
		}
	}

	return count
}

// countDomain counts how many emails are from the same domain.
func (s *AutoLabelService) countDomain(domain string, emails []*domain.Email) int {
	count := 1 // Include the current email
	domainLower := strings.ToLower(domain)

	for _, e := range emails {
		emailDomain := extractDomain(e.FromEmail)
		if strings.ToLower(emailDomain) == domainLower {
			count++
		}
	}

	return count
}

// Keyword represents an extracted keyword with confidence.
type Keyword struct {
	Value      string
	Confidence float64
}

// extractKeywords extracts significant keywords from subject.
func (s *AutoLabelService) extractKeywords(subject string, sameLabeled []*domain.Email) []Keyword {
	var keywords []Keyword

	// Simple word frequency analysis
	// In production, this would use TF-IDF or similar
	words := strings.Fields(strings.ToLower(subject))

	// Skip common words and very short words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"re": true, "fwd": true, "fw": true, "에": true, "의": true, "을": true, "를": true,
	}

	for _, word := range words {
		if len(word) < 3 || stopWords[word] {
			continue
		}

		// Count occurrences in same-labeled emails
		count := 0
		for _, e := range sameLabeled {
			if strings.Contains(strings.ToLower(e.Subject), word) {
				count++
			}
		}

		if count >= 2 {
			confidence := 0.70 + float64(count)*0.05
			if confidence > 0.90 {
				confidence = 0.90
			}
			keywords = append(keywords, Keyword{
				Value:      word,
				Confidence: confidence,
			})
		}
	}

	return keywords
}

// parseRefEmailID extracts email ID from "ref:{id}" pattern.
func parseRefEmailID(pattern string) int64 {
	if !strings.HasPrefix(pattern, "ref:") {
		return 0
	}
	idStr := strings.TrimPrefix(pattern, "ref:")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0
	}
	return id
}
