// Package classification implements the 4-stage email classification pipeline.
//
// New Pipeline Structure (optimized for LLM cost reduction):
//
//	Stage 0: RFC Headers (~50-60%)     - List-Unsubscribe, Precedence, Auto-Submitted, ESP detection
//	Stage 1: Simple User Rules (~10%)  - Domain/Keyword matching (no LLM)
//	Stage 2: Known Domain (~10%)       - SenderProfile, KnownDomain DB
//	Stage 3: LLM + User Rules (~20-30%) - Natural language rules + Work/Personal classification
//
// This approach saves ~70-80% of LLM API costs.
package classification

import (
	"context"
	"strings"

	"worker_server/core/agent/llm"
	"worker_server/core/domain"
	"worker_server/core/port/out"

	"github.com/google/uuid"
)

// Pipeline handles the 4-stage email classification.
type Pipeline struct {
	knownDomainRepo   domain.KnownDomainRepository
	senderProfileRepo domain.SenderProfileRepository
	settingsRepo      domain.SettingsRepository
	llmClient         *llm.Client

	// Score-based classifiers (v3.12.0)
	rfcScoreClassifier     *RFCScoreClassifier
	domainScoreClassifier  *DomainScoreClassifier
	subjectScoreClassifier *SubjectScoreClassifier
}

// normalizedRules는 소문자로 정규화된 분류 규칙입니다.
// 성능 최적화: 매 분류마다 strings.ToLower 호출 제거
type normalizedRules struct {
	importantDomains  []string // 소문자, @ 제거됨
	importantSenders  []string // 소문자
	importantKeywords []string // 소문자
	ignoreSenders     []string // 소문자
	ignoreKeywords    []string // 소문자
}

// normalizeRules는 ClassificationRules를 소문자로 정규화합니다.
func normalizeRules(rules *domain.ClassificationRules) *normalizedRules {
	if rules == nil {
		return nil
	}

	n := &normalizedRules{
		importantDomains:  make([]string, len(rules.ImportantDomains)),
		importantSenders:  make([]string, len(rules.ImportantSenders)),
		importantKeywords: make([]string, len(rules.ImportantKeywords)),
		ignoreSenders:     make([]string, len(rules.IgnoreSenders)),
		ignoreKeywords:    make([]string, len(rules.IgnoreKeywords)),
	}

	for i, d := range rules.ImportantDomains {
		n.importantDomains[i] = strings.ToLower(strings.TrimPrefix(d, "@"))
	}
	for i, s := range rules.ImportantSenders {
		n.importantSenders[i] = strings.ToLower(s)
	}
	for i, k := range rules.ImportantKeywords {
		n.importantKeywords[i] = strings.ToLower(k)
	}
	for i, s := range rules.IgnoreSenders {
		n.ignoreSenders[i] = strings.ToLower(s)
	}
	for i, k := range rules.IgnoreKeywords {
		n.ignoreKeywords[i] = strings.ToLower(k)
	}

	return n
}

// NewPipeline creates a new classification pipeline.
func NewPipeline(
	knownDomainRepo domain.KnownDomainRepository,
	senderProfileRepo domain.SenderProfileRepository,
	settingsRepo domain.SettingsRepository,
	llmClient *llm.Client,
) *Pipeline {
	return &Pipeline{
		knownDomainRepo:   knownDomainRepo,
		senderProfileRepo: senderProfileRepo,
		settingsRepo:      settingsRepo,
		llmClient:         llmClient,

		// Score-based classifiers (v3.12.0)
		rfcScoreClassifier:     NewRFCScoreClassifier(),
		domainScoreClassifier:  NewDomainScoreClassifier(),
		subjectScoreClassifier: NewSubjectScoreClassifier(),
	}
}

// ClassifyInput contains all inputs needed for classification.
type ClassifyInput struct {
	UserID  uuid.UUID
	Email   *domain.Email
	Headers *out.ProviderClassificationHeaders
	Body    string
}

// Classify runs the email through the 7-stage classification pipeline.
// Stage 0: RFC Headers     → List-Unsubscribe, Precedence, Developer Service headers
// Stage 1: Domain Score    → Known developer/finance/shopping/travel domains
// Stage 2: Subject Score   → CI/CD, finance, shipping patterns
// Stage 3: User Rules      → ImportantDomains, Keywords
// Stage 4: Known Domain DB → SenderProfile, KnownDomain
// Stage 5: Cache           → (reserved for future)
// Stage 6: LLM             → Natural language classification
func (p *Pipeline) Classify(ctx context.Context, input *ClassifyInput) (*domain.ClassificationPipelineResult, error) {
	// Create score classifier input
	scoreInput := &ScoreClassifierInput{
		Email:   input.Email,
		Headers: input.Headers,
		Body:    input.Body,
	}

	// Stage 0: RFC Header Classification (newsletters, marketing, notifications, developer services)
	if result, err := p.rfcScoreClassifier.Classify(ctx, scoreInput); err == nil && result != nil {
		return p.scoreResultToPipelineResult(result), nil
	}

	// Stage 1: Domain Score Classification (known service domains)
	if result, err := p.domainScoreClassifier.Classify(ctx, scoreInput); err == nil && result != nil {
		return p.scoreResultToPipelineResult(result), nil
	}

	// Stage 2: Subject Pattern Classification (CI/CD, finance, shipping patterns)
	if result, err := p.subjectScoreClassifier.Classify(ctx, scoreInput); err == nil && result != nil {
		return p.scoreResultToPipelineResult(result), nil
	}

	// Stage 3: Simple User Rules (domain/keyword matching, no LLM)
	if result, err := p.classifyBySimpleUserRules(ctx, input.UserID, input.Email); err == nil && result != nil {
		return result, nil
	}

	// Stage 4: Known domain matching (SenderProfile, KnownDomain DB)
	if result, err := p.classifyByDomain(ctx, input.UserID, input.Email.FromEmail); err == nil && result != nil {
		return result, nil
	}

	// Stage 5: Cache (reserved for future)

	// Stage 6: LLM-based classification with user's natural language rules
	if p.llmClient != nil {
		return p.classifyByLLMWithUserRules(ctx, input)
	}

	// Default classification if no LLM is available
	return &domain.ClassificationPipelineResult{
		Category:   domain.CategoryOther,
		Priority:   domain.PriorityNormal,
		Source:     domain.ClassificationSourceDomain,
		Confidence: 0.5,
		LLMUsed:    false,
	}, nil
}

// ClassifyLegacy provides backward compatibility with the old API.
// Deprecated: Use Classify with ClassifyInput instead.
func (p *Pipeline) ClassifyLegacy(ctx context.Context, userID uuid.UUID, email *domain.Email, headers *EmailHeaders, body string) (*domain.ClassificationPipelineResult, error) {
	// Convert legacy headers to new format
	var providerHeaders *out.ProviderClassificationHeaders
	if headers != nil {
		providerHeaders = &out.ProviderClassificationHeaders{
			ListUnsubscribe:     headers.ListUnsubscribe,
			ListUnsubscribePost: headers.ListUnsubscribePost,
			Precedence:          headers.Precedence,
			XMailer:             headers.XMailer,
		}
		if headers.XMailchimpID != "" {
			providerHeaders.IsMailchimp = true
		}
		if headers.XCampaign != "" {
			providerHeaders.IsCampaign = true
		}
	}

	return p.Classify(ctx, &ClassifyInput{
		UserID:  userID,
		Email:   email,
		Headers: providerHeaders,
		Body:    body,
	})
}

// EmailHeaders contains relevant headers for classification (legacy).
// Deprecated: Use out.ProviderClassificationHeaders instead.
type EmailHeaders struct {
	ListUnsubscribe     string
	ListUnsubscribePost string
	Precedence          string
	XMailer             string
	ContentType         string
	XCampaign           string
	XMailchimpID        string
}

// =============================================================================
// Stage 1: Simple User Rules (no LLM)
// =============================================================================

// classifyBySimpleUserRules performs Stage 1: Simple rule-based classification.
// This handles domain/keyword matching without requiring LLM.
func (p *Pipeline) classifyBySimpleUserRules(ctx context.Context, userID uuid.UUID, email *domain.Email) (*domain.ClassificationPipelineResult, error) {
	if p.settingsRepo == nil {
		return nil, nil
	}

	rules, err := p.settingsRepo.GetClassificationRules(ctx, userID)
	if err != nil || rules == nil {
		return nil, nil
	}

	// 규칙 정규화 (소문자 변환은 한 번만)
	normalized := normalizeRules(rules)
	if normalized == nil {
		return nil, nil
	}

	// 이메일 필드도 한 번만 소문자 변환
	senderDomain := extractDomain(email.FromEmail)
	subjectLower := strings.ToLower(email.Subject)
	snippetLower := strings.ToLower(email.Snippet)
	senderLower := strings.ToLower(email.FromEmail)

	// Check IgnoreSenders first (low priority)
	for _, pattern := range normalized.ignoreSenders {
		if strings.Contains(senderLower, pattern) {
			return &domain.ClassificationPipelineResult{
				Category:   domain.CategoryOther,
				Priority:   domain.PriorityLow,
				Source:     domain.ClassificationSourceUser,
				Confidence: 0.95,
				LLMUsed:    false,
			}, nil
		}
	}

	// Check IgnoreKeywords (low priority)
	for _, keyword := range normalized.ignoreKeywords {
		if strings.Contains(subjectLower, keyword) || strings.Contains(snippetLower, keyword) {
			return &domain.ClassificationPipelineResult{
				Category:   domain.CategoryOther,
				Priority:   domain.PriorityLow,
				Source:     domain.ClassificationSourceUser,
				Confidence: 0.90,
				LLMUsed:    false,
			}, nil
		}
	}

	// Check ImportantSenders (exact match, high priority)
	for _, sender := range normalized.importantSenders {
		if senderLower == sender || strings.Contains(senderLower, sender) {
			return &domain.ClassificationPipelineResult{
				Category:   domain.CategoryWork,
				Priority:   domain.PriorityHigh,
				Source:     domain.ClassificationSourceUser,
				Confidence: 0.98,
				LLMUsed:    false,
			}, nil
		}
	}

	// Check ImportantDomains (high priority)
	for _, domainPattern := range normalized.importantDomains {
		if strings.HasSuffix(senderDomain, domainPattern) || senderDomain == domainPattern {
			return &domain.ClassificationPipelineResult{
				Category:   domain.CategoryWork,
				Priority:   domain.PriorityHigh,
				Source:     domain.ClassificationSourceUser,
				Confidence: 0.95,
				LLMUsed:    false,
			}, nil
		}
	}

	// Check ImportantKeywords (high priority)
	for _, keyword := range normalized.importantKeywords {
		if strings.Contains(subjectLower, keyword) || strings.Contains(snippetLower, keyword) {
			return &domain.ClassificationPipelineResult{
				Category:   domain.CategoryWork,
				Priority:   domain.PriorityHigh,
				Source:     domain.ClassificationSourceUser,
				Confidence: 0.90,
				LLMUsed:    false,
			}, nil
		}
	}

	return nil, nil
}

// =============================================================================
// Stage 2: Known Domain
// =============================================================================

// classifyByDomain performs Stage 2: Domain-based classification.
func (p *Pipeline) classifyByDomain(ctx context.Context, userID uuid.UUID, fromEmail string) (*domain.ClassificationPipelineResult, error) {
	// Extract domain from email
	domainName := extractDomain(fromEmail)
	if domainName == "" {
		return nil, nil
	}

	// Check sender profile first (user-specific learning)
	if p.senderProfileRepo != nil {
		profile, err := p.senderProfileRepo.GetByEmail(userID, fromEmail)
		if err == nil && profile != nil && profile.LearnedCategory != nil {
			return &domain.ClassificationPipelineResult{
				Category:    *profile.LearnedCategory,
				SubCategory: profile.LearnedSubCategory,
				Priority:    p.priorityFromProfile(profile),
				Source:      domain.ClassificationSourceDomain,
				Confidence:  0.85,
				LLMUsed:     false,
			}, nil
		}
	}

	// Check known domains (global)
	if p.knownDomainRepo != nil {
		knownDomain, err := p.knownDomainRepo.GetByDomain(domainName)
		if err == nil && knownDomain != nil {
			return &domain.ClassificationPipelineResult{
				Category:    knownDomain.Category,
				SubCategory: knownDomain.SubCategory,
				Priority:    p.priorityFromCategory(knownDomain.Category),
				Source:      domain.ClassificationSourceDomain,
				Confidence:  knownDomain.Confidence,
				LLMUsed:     false,
			}, nil
		}
	}

	return nil, nil
}

// =============================================================================
// Stage 3: LLM with User Rules
// =============================================================================

// classifyByLLMWithUserRules performs Stage 3: LLM classification with user's natural language rules.
func (p *Pipeline) classifyByLLMWithUserRules(ctx context.Context, input *ClassifyInput) (*domain.ClassificationPipelineResult, error) {
	// Get user's LLM rules (natural language)
	var userLLMRules *llm.UserLLMRules
	if p.settingsRepo != nil {
		rules, err := p.settingsRepo.GetClassificationRules(ctx, input.UserID)
		if err == nil && rules != nil {
			userLLMRules = &llm.UserLLMRules{
				HighPriorityRules:  rules.HighPriorityRules,
				LowPriorityRules:   rules.LowPriorityRules,
				CategoryRules:      rules.CategoryRules,
				CustomInstructions: rules.CustomInstructions,
			}
		}
	}

	// Call LLM with user rules
	resp, err := p.llmClient.ClassifyEmailWithUserRules(ctx, input.Email, input.Body, userLLMRules)
	if err != nil {
		// Fallback to default on LLM error
		return &domain.ClassificationPipelineResult{
			Category:   domain.CategoryOther,
			Priority:   domain.PriorityNormal,
			Source:     domain.ClassificationSourceLLM,
			Confidence: 0.5,
			LLMUsed:    true,
		}, nil
	}

	// Validate and convert response to domain types
	validatedCategory := ValidateCategory(resp.Category)
	category := domain.EmailCategory(validatedCategory)

	// Validate priority (0.0 ~ 1.0)
	validatedPriority := ValidatePriority(resp.Priority)
	priority := domain.Priority(validatedPriority)

	result := &domain.ClassificationPipelineResult{
		Category:   category,
		Priority:   priority,
		Source:     domain.ClassificationSourceLLM,
		Confidence: resp.Score,
		LLMUsed:    true,
	}

	// Set sub-category if valid
	if resp.SubCategory != "" {
		validatedSubCat := ValidateSubCategory(resp.SubCategory)
		if validatedSubCat != "" {
			subCat := domain.EmailSubCategory(validatedSubCat)
			result.SubCategory = &subCat
		}
	}

	return result, nil
}

// =============================================================================
// Helpers
// =============================================================================

// scoreResultToPipelineResult converts ScoreClassifierResult to ClassificationPipelineResult.
func (p *Pipeline) scoreResultToPipelineResult(result *ScoreClassifierResult) *domain.ClassificationPipelineResult {
	pipelineResult := &domain.ClassificationPipelineResult{
		Category:    result.Category,
		SubCategory: result.SubCategory,
		Priority:    result.Priority,
		Source:      domain.ClassificationSourceHeader,
		Confidence:  result.Score,
		LLMUsed:     result.LLMUsed,
	}
	// Set source based on signal
	if len(result.Signals) > 0 {
		if strings.HasPrefix(result.Source, "domain:") {
			pipelineResult.Source = domain.ClassificationSourceDomain
		}
		// Header-based (RFC, subject patterns) use ClassificationSourceHeader
	}
	return pipelineResult
}

// priorityFromProfile determines priority based on sender profile.
func (p *Pipeline) priorityFromProfile(profile *domain.SenderProfile) domain.Priority {
	if profile.IsVIP {
		return domain.PriorityHigh
	}
	if profile.IsMuted {
		return domain.PriorityLowest
	}
	// Use read/reply rate to determine importance
	if profile.ReplyRate > 0.5 {
		return domain.PriorityHigh
	}
	if profile.ReadRate < 0.2 {
		return domain.PriorityLow
	}
	return domain.PriorityNormal
}

// priorityFromCategory determines default priority based on category.
func (p *Pipeline) priorityFromCategory(category domain.EmailCategory) domain.Priority {
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

// extractDomain extracts the domain from an email address.
func extractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(parts[1])
}

// UpdateSenderProfile updates sender profile based on email interaction.
func (p *Pipeline) UpdateSenderProfile(ctx context.Context, userID uuid.UUID, email *domain.Email, result *domain.ClassificationPipelineResult) error {
	if p.senderProfileRepo == nil {
		return nil
	}

	profile, err := p.senderProfileRepo.GetByEmail(userID, email.FromEmail)
	if err != nil {
		return err
	}

	if profile == nil {
		// Create new profile
		profile = &domain.SenderProfile{
			UserID:      userID,
			Email:       email.FromEmail,
			Domain:      extractDomain(email.FromEmail),
			EmailCount:  1,
			FirstSeenAt: email.ReceivedAt,
			LastSeenAt:  email.ReceivedAt,
		}
		if result.Source == domain.ClassificationSourceLLM {
			profile.LearnedCategory = &result.Category
			profile.LearnedSubCategory = result.SubCategory
		}
		return p.senderProfileRepo.Create(profile)
	}

	// Update existing profile
	if err := p.senderProfileRepo.IncrementEmailCount(profile.ID); err != nil {
		return err
	}
	return p.senderProfileRepo.UpdateLastSeen(profile.ID, email.ReceivedAt)
}
