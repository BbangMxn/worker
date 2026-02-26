// Package classification implements the score-based email classification pipeline.
package classification

import (
	"context"
	"strconv"
	"strings"

	"worker_server/core/domain"
)

// =============================================================================
// User Rule Score Classifier (Stage 2)
// =============================================================================

// UserRuleScoreClassifier performs Stage 2 classification based on user-defined rules.
// Supports exact_sender, sender_domain, subject_keyword, body_keyword rules.
type UserRuleScoreClassifier struct {
	ruleRepo domain.ScoreClassificationRuleRepository
}

// NewUserRuleScoreClassifier creates a new user rule score classifier.
func NewUserRuleScoreClassifier(ruleRepo domain.ScoreClassificationRuleRepository) *UserRuleScoreClassifier {
	return &UserRuleScoreClassifier{
		ruleRepo: ruleRepo,
	}
}

// Name returns the classifier name.
func (c *UserRuleScoreClassifier) Name() string {
	return "rule"
}

// Stage returns the pipeline stage number.
func (c *UserRuleScoreClassifier) Stage() int {
	return 2
}

// Classify performs user rule-based classification.
func (c *UserRuleScoreClassifier) Classify(ctx context.Context, input *ScoreClassifierInput) (*ScoreClassifierResult, error) {
	if c.ruleRepo == nil {
		return nil, nil
	}

	// Get active rules for user
	rules, err := c.ruleRepo.ListActiveByUser(ctx, input.UserID)
	if err != nil || len(rules) == 0 {
		return nil, nil
	}

	// Prepare normalized inputs for matching
	senderLower := strings.ToLower(input.Email.FromEmail)
	senderDomain := extractDomain(input.Email.FromEmail)
	subjectLower := strings.ToLower(input.Email.Subject)
	snippetLower := strings.ToLower(input.Email.Snippet)

	var bestResult *ScoreClassifierResult
	var matchedSignals []string

	// Process rules by type priority (exact_sender → sender_domain → keywords)
	// Group rules by type for efficient processing
	rulesByType := make(map[domain.RuleType][]*domain.ScoreClassificationRule)
	for _, rule := range rules {
		rulesByType[rule.Type] = append(rulesByType[rule.Type], rule)
	}

	// 1. Check exact sender rules (highest priority, 0.99)
	for _, rule := range rulesByType[domain.RuleTypeExactSender] {
		patternLower := strings.ToLower(rule.Pattern)
		if senderLower == patternLower || strings.Contains(senderLower, patternLower) {
			result := c.applyRule(rule)
			matchedSignals = append(matchedSignals, SignalExactSender)
			if bestResult == nil || result.Score > bestResult.Score {
				bestResult = result
			}
			// Update hit count asynchronously
			go func(ruleID int64) {
				_ = c.ruleRepo.IncrementHitCount(ctx, ruleID)
			}(rule.ID)
		}
	}

	// If exact sender matched with high score, return early
	if bestResult != nil && bestResult.Score >= 0.98 {
		bestResult.Signals = matchedSignals
		return bestResult, nil
	}

	// 2. Check sender domain rules (0.95)
	for _, rule := range rulesByType[domain.RuleTypeSenderDomain] {
		patternLower := strings.ToLower(strings.TrimPrefix(rule.Pattern, "@"))
		if senderDomain == patternLower || strings.HasSuffix(senderDomain, "."+patternLower) {
			result := c.applyRule(rule)
			matchedSignals = append(matchedSignals, SignalSenderDomain)
			if bestResult == nil || result.Score > bestResult.Score {
				bestResult = result
			}
			go func(ruleID int64) {
				_ = c.ruleRepo.IncrementHitCount(ctx, ruleID)
			}(rule.ID)
		}
	}

	// 3. Check subject keyword rules (0.90)
	for _, rule := range rulesByType[domain.RuleTypeSubjectKeyword] {
		patternLower := strings.ToLower(rule.Pattern)
		if strings.Contains(subjectLower, patternLower) {
			result := c.applyRule(rule)
			matchedSignals = append(matchedSignals, SignalSubjectKeyword)
			if bestResult == nil || result.Score > bestResult.Score {
				bestResult = result
			}
			go func(ruleID int64) {
				_ = c.ruleRepo.IncrementHitCount(ctx, ruleID)
			}(rule.ID)
		}
	}

	// 4. Check body keyword rules (0.85)
	for _, rule := range rulesByType[domain.RuleTypeBodyKeyword] {
		patternLower := strings.ToLower(rule.Pattern)
		if strings.Contains(snippetLower, patternLower) {
			result := c.applyRule(rule)
			matchedSignals = append(matchedSignals, SignalBodyKeyword)
			if bestResult == nil || result.Score > bestResult.Score {
				bestResult = result
			}
			go func(ruleID int64) {
				_ = c.ruleRepo.IncrementHitCount(ctx, ruleID)
			}(rule.ID)
		}
	}

	// Note: ai_prompt rules are handled in Stage 4 (LLM Fallback)

	if bestResult != nil {
		bestResult.Signals = matchedSignals
	}

	return bestResult, nil
}

// applyRule converts a matched rule to a classification result.
func (c *UserRuleScoreClassifier) applyRule(rule *domain.ScoreClassificationRule) *ScoreClassifierResult {
	result := &ScoreClassifierResult{
		Score:   rule.Score,
		Source:  "rule:" + string(rule.Type),
		LLMUsed: false,
	}

	switch rule.Action {
	case domain.ScoreRuleActionAssignCategory:
		result.Category = domain.EmailCategory(rule.Value)
		result.Priority = c.defaultPriorityForCategory(result.Category)

	case domain.ScoreRuleActionAssignPriority:
		result.Priority = parsePriority(rule.Value)
		result.Category = domain.CategoryOther // Default, may be overridden

	case domain.ScoreRuleActionAssignLabel:
		labelID, err := strconv.ParseInt(rule.Value, 10, 64)
		if err == nil {
			result.Labels = []int64{labelID}
		}
		result.Category = domain.CategoryOther
		result.Priority = domain.PriorityNormal

	case domain.ScoreRuleActionMarkImportant:
		result.Category = domain.CategoryWork
		result.Priority = domain.PriorityHigh

	case domain.ScoreRuleActionMarkSpam:
		result.Category = domain.CategorySpam
		result.Priority = domain.PriorityLowest
	}

	return result
}

// defaultPriorityForCategory returns the default priority for a category.
func (c *UserRuleScoreClassifier) defaultPriorityForCategory(category domain.EmailCategory) domain.Priority {
	switch category {
	case domain.CategoryWork, domain.CategoryFinance:
		return domain.PriorityHigh
	case domain.CategoryPersonal:
		return domain.PriorityNormal
	case domain.CategoryNewsletter, domain.CategoryMarketing, domain.CategorySocial:
		return domain.PriorityLow
	case domain.CategorySpam:
		return domain.PriorityLowest
	default:
		return domain.PriorityNormal
	}
}

// =============================================================================
// Rule Matcher Utilities
// =============================================================================

// RuleMatcher provides optimized rule matching for large rule sets.
type RuleMatcher struct {
	exactSenders map[string]*domain.ScoreClassificationRule // O(1) lookup
	domainRules  []*domain.ScoreClassificationRule          // Linear scan (usually small)
	keywordRules []*domain.ScoreClassificationRule          // Linear scan
}

// NewRuleMatcher creates a new rule matcher from a list of rules.
func NewRuleMatcher(rules []*domain.ScoreClassificationRule) *RuleMatcher {
	m := &RuleMatcher{
		exactSenders: make(map[string]*domain.ScoreClassificationRule),
	}

	for _, rule := range rules {
		switch rule.Type {
		case domain.RuleTypeExactSender:
			key := strings.ToLower(rule.Pattern)
			// Keep the rule with highest score
			if existing, ok := m.exactSenders[key]; !ok || rule.Score > existing.Score {
				m.exactSenders[key] = rule
			}
		case domain.RuleTypeSenderDomain:
			m.domainRules = append(m.domainRules, rule)
		case domain.RuleTypeSubjectKeyword, domain.RuleTypeBodyKeyword:
			m.keywordRules = append(m.keywordRules, rule)
		}
	}

	return m
}

// MatchExactSender finds a rule matching the exact sender email.
func (m *RuleMatcher) MatchExactSender(email string) *domain.ScoreClassificationRule {
	return m.exactSenders[strings.ToLower(email)]
}

// MatchDomain finds rules matching the sender domain.
func (m *RuleMatcher) MatchDomain(senderDomain string) []*domain.ScoreClassificationRule {
	var matches []*domain.ScoreClassificationRule
	domainLower := strings.ToLower(senderDomain)

	for _, rule := range m.domainRules {
		pattern := strings.ToLower(strings.TrimPrefix(rule.Pattern, "@"))
		if domainLower == pattern || strings.HasSuffix(domainLower, "."+pattern) {
			matches = append(matches, rule)
		}
	}

	return matches
}

// MatchKeywords finds rules matching keywords in text.
func (m *RuleMatcher) MatchKeywords(subject, body string) []*domain.ScoreClassificationRule {
	var matches []*domain.ScoreClassificationRule
	subjectLower := strings.ToLower(subject)
	bodyLower := strings.ToLower(body)

	for _, rule := range m.keywordRules {
		pattern := strings.ToLower(rule.Pattern)

		switch rule.Type {
		case domain.RuleTypeSubjectKeyword:
			if strings.Contains(subjectLower, pattern) {
				matches = append(matches, rule)
			}
		case domain.RuleTypeBodyKeyword:
			if strings.Contains(bodyLower, pattern) {
				matches = append(matches, rule)
			}
		}
	}

	return matches
}
