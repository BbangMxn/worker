// Package classification implements the score-based email classification pipeline.
package classification

import (
	"context"

	"worker_server/core/agent/llm"
	"worker_server/core/domain"
)

// =============================================================================
// LLM Fallback Score Classifier (Stage 4)
// =============================================================================

// LLMScoreClassifier performs Stage 4 classification using LLM.
// This is the fallback when previous stages don't have high enough confidence.
type LLMScoreClassifier struct {
	llmClient    *llm.Client
	settingsRepo domain.SettingsRepository
}

// NewLLMScoreClassifier creates a new LLM score classifier.
func NewLLMScoreClassifier(llmClient *llm.Client, settingsRepo domain.SettingsRepository) *LLMScoreClassifier {
	return &LLMScoreClassifier{
		llmClient:    llmClient,
		settingsRepo: settingsRepo,
	}
}

// Name returns the classifier name.
func (c *LLMScoreClassifier) Name() string {
	return "llm"
}

// Stage returns the pipeline stage number.
func (c *LLMScoreClassifier) Stage() int {
	return 4
}

// Classify performs LLM-based classification.
func (c *LLMScoreClassifier) Classify(ctx context.Context, input *ScoreClassifierInput) (*ScoreClassifierResult, error) {
	if c.llmClient == nil {
		return nil, nil
	}

	// Get user's LLM rules (natural language)
	var userLLMRules *llm.UserLLMRules
	if c.settingsRepo != nil {
		rules, err := c.settingsRepo.GetClassificationRules(ctx, input.UserID)
		if err == nil && rules != nil {
			userLLMRules = &llm.UserLLMRules{
				HighPriorityRules:  rules.HighPriorityRules,
				LowPriorityRules:   rules.LowPriorityRules,
				CategoryRules:      rules.CategoryRules,
				CustomInstructions: rules.CustomInstructions,
			}
		}
	}

	// Call LLM
	resp, err := c.llmClient.ClassifyEmailWithUserRules(ctx, input.Email, input.Body, userLLMRules)
	if err != nil {
		// Return low-confidence default on error
		return &ScoreClassifierResult{
			Category: domain.CategoryOther,
			Priority: domain.PriorityNormal,
			Score:    0.50,
			Source:   "llm:error",
			Signals:  []string{SignalLLMClassified},
			LLMUsed:  true,
		}, nil
	}

	// Convert LLM response to result
	result := &ScoreClassifierResult{
		Category: domain.EmailCategory(resp.Category),
		Priority: domain.Priority(resp.Priority),
		Score:    resp.Score,
		Source:   "llm:classified",
		Signals:  []string{SignalLLMClassified},
		LLMUsed:  true,
	}

	if resp.SubCategory != "" {
		subCat := domain.EmailSubCategory(resp.SubCategory)
		result.SubCategory = &subCat
	}

	return result, nil
}

// =============================================================================
// LLM Rule Classifier (for ai_prompt type rules)
// =============================================================================

// LLMRuleClassifier evaluates user's natural language rules using LLM.
type LLMRuleClassifier struct {
	llmClient *llm.Client
	ruleRepo  domain.ScoreClassificationRuleRepository
}

// NewLLMRuleClassifier creates a new LLM rule classifier.
func NewLLMRuleClassifier(llmClient *llm.Client, ruleRepo domain.ScoreClassificationRuleRepository) *LLMRuleClassifier {
	return &LLMRuleClassifier{
		llmClient: llmClient,
		ruleRepo:  ruleRepo,
	}
}

// EvaluateAIPromptRules evaluates ai_prompt type rules for the given email.
// Returns matched rules with their evaluation scores.
func (c *LLMRuleClassifier) EvaluateAIPromptRules(ctx context.Context, input *ScoreClassifierInput) ([]*domain.ScoreClassificationRule, error) {
	if c.llmClient == nil || c.ruleRepo == nil {
		return nil, nil
	}

	// Get ai_prompt rules
	rules, err := c.ruleRepo.ListByUserAndType(ctx, input.UserID, domain.RuleTypeAIPrompt)
	if err != nil || len(rules) == 0 {
		return nil, nil
	}

	// Batch evaluate rules with LLM
	// Note: This could be optimized by batching multiple rules in a single LLM call
	var matchedRules []*domain.ScoreClassificationRule

	for _, rule := range rules {
		matched, err := c.evaluateRule(ctx, input, rule)
		if err != nil {
			continue
		}
		if matched {
			matchedRules = append(matchedRules, rule)
		}
	}

	return matchedRules, nil
}

// evaluateRule evaluates a single ai_prompt rule.
func (c *LLMRuleClassifier) evaluateRule(ctx context.Context, input *ScoreClassifierInput, rule *domain.ScoreClassificationRule) (bool, error) {
	// Call LLM to evaluate if the rule applies
	// The rule.Pattern contains the natural language condition
	// e.g., "이메일이 긴급한 요청을 포함하면"

	prompt := buildRuleEvaluationPrompt(input.Email, rule.Pattern)
	resp, err := c.llmClient.Complete(ctx, prompt)
	if err != nil {
		return false, err
	}

	// Parse response (expecting "yes" or "no")
	return parseRuleEvaluationResponse(resp), nil
}

// buildRuleEvaluationPrompt builds a prompt for rule evaluation.
func buildRuleEvaluationPrompt(email *domain.Email, ruleCondition string) string {
	return `다음 이메일이 규칙 조건에 해당하는지 판단해주세요.

이메일 정보:
- 발신자: ` + email.FromEmail + `
- 제목: ` + email.Subject + `
- 내용: ` + email.Snippet + `

규칙 조건: ` + ruleCondition + `

이 이메일이 위 조건에 해당하면 "yes", 해당하지 않으면 "no"만 답해주세요.`
}

// parseRuleEvaluationResponse parses the LLM response for rule evaluation.
func parseRuleEvaluationResponse(response string) bool {
	// Simple check for "yes" in response
	// Could be improved with more robust parsing
	return len(response) > 0 && (response[0] == 'y' || response[0] == 'Y')
}
