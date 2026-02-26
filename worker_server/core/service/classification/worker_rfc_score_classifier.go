// Package classification implements the score-based email classification pipeline.
package classification

import (
	"context"
	"strings"

	"worker_server/core/domain"
	"worker_server/core/port/out"
)

// =============================================================================
// RFC Score Classifier (Stage 0)
// =============================================================================
//
// Category Mapping Reference (for Smart Folders):
//
// Categories:
//   - work       → Developer tools (GitHub, GitLab, Jira, CI/CD), Business tools
//   - notification → Auto-generated system notifications, alerts
//   - newsletter → Mailing lists (List-Unsubscribe), Digests
//   - marketing  → ESP-sent bulk mail, Campaigns, Promotions
//   - social     → Social network notifications
//   - finance    → Transaction receipts, Invoices, Banking
//   - shopping   → Order confirmations, Shipping updates
//   - travel     → Booking confirmations, Itineraries
//   - spam       → Precedence: junk
//   - other      → Unclassified
//
// SubCategories:
//   - developer  → GitHub, GitLab, Jira, Linear, Sentry, Vercel, AWS
//   - alert      → PagerDuty, Datadog, Uptime alerts, Security alerts
//   - notification → Auto-submitted, No-reply system emails
//   - newsletter → List-ID, List-Unsubscribe mailing lists
//   - marketing  → ESP campaigns (Mailchimp, SendGrid, etc.)
//   - security   → 2FA codes, Login alerts, Password reset
//   - calendar   → Meeting invites, Schedule changes
//   - receipt    → Payment confirmations
//   - invoice    → Billing statements
//   - shipping   → Delivery updates
//   - order      → Order confirmations

// RFCScoreClassifier performs Stage 0 classification based on RFC headers.
// Returns scored results instead of boolean classification.
type RFCScoreClassifier struct{}

// NewRFCScoreClassifier creates a new RFC header score classifier.
func NewRFCScoreClassifier() *RFCScoreClassifier {
	return &RFCScoreClassifier{}
}

// Name returns the classifier name.
func (c *RFCScoreClassifier) Name() string {
	return "rfc"
}

// Stage returns the pipeline stage number.
func (c *RFCScoreClassifier) Stage() int {
	return 0
}

// Classify performs RFC header-based classification.
func (c *RFCScoreClassifier) Classify(ctx context.Context, input *ScoreClassifierInput) (*ScoreClassifierResult, error) {
	if input.Headers == nil {
		return nil, nil
	}

	var signals []string
	var bestResult *ScoreClassifierResult

	// 0. Developer Service Headers (highest priority - specific and accurate)
	if result := c.classifyByDeveloperService(input.Headers, signals); result != nil {
		// Developer service headers are highly accurate, return immediately
		return result, nil
	}

	// 1. List-Unsubscribe (strongest newsletter signal)
	if input.Headers.ListUnsubscribe != "" || input.Headers.ListUnsubscribePost != "" {
		signals = append(signals, SignalListUnsubscribe)
		subCat := domain.SubCategoryNewsletter
		bestResult = &ScoreClassifierResult{
			Category:    domain.CategoryNewsletter,
			SubCategory: &subCat,
			Priority:    domain.PriorityLow,
			Score:       0.95,
			Source:      "rfc:list-unsubscribe",
			Signals:     signals,
			LLMUsed:     false,
		}
	}

	// 2. List-ID (mailing list)
	if input.Headers.ListID != "" && (bestResult == nil || bestResult.Score < 0.90) {
		signals = append(signals, SignalListID)
		subCat := domain.SubCategoryNewsletter
		result := &ScoreClassifierResult{
			Category:    domain.CategoryNewsletter,
			SubCategory: &subCat,
			Priority:    domain.PriorityLow,
			Score:       0.90,
			Source:      "rfc:list-id",
			Signals:     signals,
			LLMUsed:     false,
		}
		if bestResult == nil || result.Score > bestResult.Score {
			bestResult = result
		}
	}

	// 3. Precedence: bulk
	if result := c.classifyByPrecedence(input.Headers.Precedence, signals); result != nil {
		if bestResult == nil || result.Score > bestResult.Score {
			bestResult = result
		}
	}

	// 4. Auto-Submitted
	if result := c.classifyByAutoSubmitted(input.Headers, signals); result != nil {
		if bestResult == nil || result.Score > bestResult.Score {
			bestResult = result
		}
	}

	// 5. ESP Detection
	if result := c.classifyByESP(input.Headers, signals); result != nil {
		if bestResult == nil || result.Score > bestResult.Score {
			bestResult = result
		}
	}

	// 6. X-Mailer Marketing Tool
	if result := c.classifyByMailer(input.Headers.XMailer, signals); result != nil {
		if bestResult == nil || result.Score > bestResult.Score {
			bestResult = result
		}
	}

	// 7. Feedback-ID (Gmail bulk sender signal)
	if input.Headers.FeedbackID != "" {
		signals = append(signals, SignalFeedbackID)
		subCat := domain.SubCategoryMarketing
		result := &ScoreClassifierResult{
			Category:    domain.CategoryMarketing,
			SubCategory: &subCat,
			Priority:    domain.PriorityLow,
			Score:       0.80,
			Source:      "rfc:feedback-id",
			Signals:     signals,
			LLMUsed:     false,
		}
		if bestResult == nil || result.Score > bestResult.Score {
			bestResult = result
		}
	}

	// 8. No-Reply sender pattern
	if result := c.classifyByNoReply(input.Email.FromEmail, input.Headers, signals); result != nil {
		if bestResult == nil || result.Score > bestResult.Score {
			bestResult = result
		}
	}

	// Update signals on best result
	if bestResult != nil {
		bestResult.Signals = signals
	}

	return bestResult, nil
}

// classifyByPrecedence analyzes Precedence header.
func (c *RFCScoreClassifier) classifyByPrecedence(precedence string, signals []string) *ScoreClassifierResult {
	if precedence == "" {
		return nil
	}

	switch strings.ToLower(precedence) {
	case "bulk":
		signals = append(signals, SignalPrecedenceBulk)
		subCat := domain.SubCategoryMarketing
		return &ScoreClassifierResult{
			Category:    domain.CategoryMarketing,
			SubCategory: &subCat,
			Priority:    domain.PriorityLowest,
			Score:       0.90,
			Source:      "rfc:precedence-bulk",
			Signals:     signals,
			LLMUsed:     false,
		}

	case "list":
		signals = append(signals, SignalBulkMail)
		subCat := domain.SubCategoryNewsletter
		return &ScoreClassifierResult{
			Category:    domain.CategoryNewsletter,
			SubCategory: &subCat,
			Priority:    domain.PriorityLow,
			Score:       0.85,
			Source:      "rfc:precedence-list",
			Signals:     signals,
			LLMUsed:     false,
		}

	case "junk":
		return &ScoreClassifierResult{
			Category: domain.CategorySpam,
			Priority: domain.PriorityLowest,
			Score:    0.85,
			Source:   "rfc:precedence-junk",
			Signals:  signals,
			LLMUsed:  false,
		}
	}

	return nil
}

// classifyByAutoSubmitted analyzes Auto-Submitted header.
func (c *RFCScoreClassifier) classifyByAutoSubmitted(headers *out.ProviderClassificationHeaders, signals []string) *ScoreClassifierResult {
	if headers.AutoSubmitted != "" && strings.ToLower(headers.AutoSubmitted) != "no" {
		signals = append(signals, SignalAutoSubmitted)
		subCat := domain.SubCategoryNotification
		return &ScoreClassifierResult{
			Category:    domain.CategoryNotification,
			SubCategory: &subCat,
			Priority:    domain.PriorityLow,
			Score:       0.92,
			Source:      "rfc:auto-submitted",
			Signals:     signals,
			LLMUsed:     false,
		}
	}

	// Microsoft X-Auto-Response-Suppress
	if headers.AutoResponseSuppress != "" {
		signals = append(signals, SignalAutoSubmitted)
		subCat := domain.SubCategoryNotification
		return &ScoreClassifierResult{
			Category:    domain.CategoryNotification,
			SubCategory: &subCat,
			Priority:    domain.PriorityLow,
			Score:       0.88,
			Source:      "rfc:auto-response-suppress",
			Signals:     signals,
			LLMUsed:     false,
		}
	}

	return nil
}

// classifyByESP detects Email Service Provider signatures.
func (c *RFCScoreClassifier) classifyByESP(headers *out.ProviderClassificationHeaders, signals []string) *ScoreClassifierResult {
	var espSignal string
	var score float64 = 0.88

	switch {
	case headers.IsMailchimp:
		espSignal = SignalMailchimp
		score = 0.90
	case headers.IsSendGrid:
		espSignal = SignalSendGrid
		score = 0.88
	case headers.IsAmazonSES:
		espSignal = SignalMarketingESP
		score = 0.85
	case headers.IsMailgun:
		espSignal = SignalMarketingESP
		score = 0.85
	case headers.IsPostmark:
		espSignal = SignalMarketingESP
		score = 0.85
	case headers.IsCampaign:
		espSignal = SignalCampaign
		score = 0.88
	default:
		return nil
	}

	signals = append(signals, espSignal)
	subCat := domain.SubCategoryMarketing
	return &ScoreClassifierResult{
		Category:    domain.CategoryMarketing,
		SubCategory: &subCat,
		Priority:    domain.PriorityLow,
		Score:       score,
		Source:      "rfc:esp-" + espSignal,
		Signals:     signals,
		LLMUsed:     false,
	}
}

// classifyByMailer checks X-Mailer for known marketing tools.
func (c *RFCScoreClassifier) classifyByMailer(mailer string, signals []string) *ScoreClassifierResult {
	if mailer == "" {
		return nil
	}

	mailerLower := strings.ToLower(mailer)

	// Known marketing email tools with confidence scores
	marketingMailers := map[string]float64{
		"mailchimp":        0.90,
		"sendgrid":         0.88,
		"mailgun":          0.85,
		"postmark":         0.85,
		"sendinblue":       0.85,
		"constant contact": 0.88,
		"campaign monitor": 0.88,
		"hubspot":          0.90,
		"marketo":          0.90,
		"klaviyo":          0.88,
		"drip":             0.85,
		"convertkit":       0.85,
		"aweber":           0.85,
		"activecampaign":   0.88,
		"salesforce":       0.85,
		"pardot":           0.88,
		"braze":            0.88,
		"iterable":         0.85,
		"customer.io":      0.85,
		"intercom":         0.85,
		"drift":            0.82,
		"mailjet":          0.85,
		"sparkpost":        0.85,
		"mandrill":         0.88,
	}

	for tool, score := range marketingMailers {
		if strings.Contains(mailerLower, tool) {
			signals = append(signals, SignalMarketingESP)
			subCat := domain.SubCategoryMarketing
			return &ScoreClassifierResult{
				Category:    domain.CategoryMarketing,
				SubCategory: &subCat,
				Priority:    domain.PriorityLow,
				Score:       score,
				Source:      "rfc:mailer-" + tool,
				Signals:     signals,
				LLMUsed:     false,
			}
		}
	}

	return nil
}

// classifyByDeveloperService detects developer service headers (GitHub, GitLab, Jira, etc.)
// These headers are highly specific and accurate, providing confident classification.
func (c *RFCScoreClassifier) classifyByDeveloperService(headers *out.ProviderClassificationHeaders, signals []string) *ScoreClassifierResult {
	// === GitHub ===
	if headers.XGitHubReason != "" {
		return c.classifyGitHub(headers, signals)
	}

	// === GitLab ===
	if headers.XGitLabProject != "" || headers.XGitLabPipelineID != "" || headers.XGitLabNotificationReason != "" {
		return c.classifyGitLab(headers, signals)
	}

	// === Jira/Atlassian ===
	if headers.XJIRAFingerprint != "" {
		signals = append(signals, SignalDeveloperService)
		subCat := domain.SubCategoryDeveloper
		return &ScoreClassifierResult{
			Category:    domain.CategoryWork,
			SubCategory: &subCat,
			Priority:    domain.PriorityNormal,
			Score:       0.95,
			Source:      "rfc:jira",
			Signals:     signals,
			LLMUsed:     false,
		}
	}

	// === Linear ===
	if headers.XLinearTeam != "" || headers.XLinearProject != "" {
		signals = append(signals, SignalDeveloperService)
		subCat := domain.SubCategoryDeveloper
		return &ScoreClassifierResult{
			Category:    domain.CategoryWork,
			SubCategory: &subCat,
			Priority:    domain.PriorityNormal,
			Score:       0.95,
			Source:      "rfc:linear",
			Signals:     signals,
			LLMUsed:     false,
		}
	}

	// === Sentry ===
	if headers.XSentryProject != "" {
		signals = append(signals, SignalDeveloperService)
		subCat := domain.SubCategoryDeveloper
		return &ScoreClassifierResult{
			Category:    domain.CategoryWork,
			SubCategory: &subCat,
			Priority:    domain.PriorityHigh, // Sentry alerts are usually important
			Score:       0.95,
			Source:      "rfc:sentry",
			Signals:     signals,
			LLMUsed:     false,
		}
	}

	// === Vercel ===
	if headers.XVercelDeploymentURL != "" {
		signals = append(signals, SignalDeveloperService)
		subCat := domain.SubCategoryDeveloper
		return &ScoreClassifierResult{
			Category:    domain.CategoryWork,
			SubCategory: &subCat,
			Priority:    domain.PriorityNormal,
			Score:       0.95,
			Source:      "rfc:vercel",
			Signals:     signals,
			LLMUsed:     false,
		}
	}

	// === AWS ===
	if headers.XAWSService != "" {
		signals = append(signals, SignalDeveloperService)
		subCat := domain.SubCategoryDeveloper
		return &ScoreClassifierResult{
			Category:    domain.CategoryWork,
			SubCategory: &subCat,
			Priority:    domain.PriorityNormal,
			Score:       0.92,
			Source:      "rfc:aws-" + headers.XAWSService,
			Signals:     signals,
			LLMUsed:     false,
		}
	}

	return nil
}

// classifyGitHub classifies GitHub emails based on X-GitHub-Reason header.
// Reference: https://docs.github.com/en/account-and-profile/managing-subscriptions-and-notifications-on-github
//
// Priority = DomainScore(0.18) + ReasonScore + RelationScore
// Security alerts use fixed priority values.
//
// Inbox Strategy:
//   - CategoryWork: Direct involvement → show in Inbox
//   - CategoryNotification: Passive watching → can be filtered
func (c *RFCScoreClassifier) classifyGitHub(headers *out.ProviderClassificationHeaders, signals []string) *ScoreClassifierResult {
	reason := strings.ToLower(headers.XGitHubReason)
	signals = append(signals, SignalDeveloperService, "github:"+reason)

	subCat := domain.SubCategoryDeveloper

	// Security alerts → fixed priority (bypass calculation)
	if reason == "security_alert" {
		alertSubCat := domain.SubCategoryAlert
		priority := GetGitHubSecurityScore(headers.XGitHubSeverity)
		return &ScoreClassifierResult{
			Category:    domain.CategoryWork,
			SubCategory: &alertSubCat,
			Priority:    domain.Priority(priority),
			Score:       0.99,
			Source:      "rfc:github-security-alert",
			Signals:     signals,
			LLMUsed:     false,
		}
	}

	// Calculate priority: Domain + Reason + Relation
	reasonScore, relationScore := GetGitHubReasonScore(reason)
	priority := CalculatePriority(DomainScoreGitHub, reasonScore, relationScore, 0)

	result := &ScoreClassifierResult{
		Category:    domain.CategoryWork,
		SubCategory: &subCat,
		Priority:    domain.Priority(priority),
		Score:       0.95,
		Signals:     signals,
		LLMUsed:     false,
	}

	// Determine category and source based on reason
	switch reason {
	// === INBOX (CategoryWork) - Direct involvement ===
	case "review_requested":
		result.Source = "rfc:github-review-requested"
	case "author":
		result.Source = "rfc:github-author"
	case "mention":
		result.Source = "rfc:github-mention"
	case "assign":
		result.Source = "rfc:github-assign"
	case "team_mention":
		result.Source = "rfc:github-team-mention"
	case "ci_activity":
		result.Source = "rfc:github-ci"

	// === NOTIFICATION (CategoryNotification) - Passive watching ===
	case "subscribed":
		result.Category = domain.CategoryNotification
		result.Source = "rfc:github-subscribed"
	case "push":
		result.Category = domain.CategoryNotification
		result.Source = "rfc:github-push"
	case "manual":
		result.Category = domain.CategoryNotification
		result.Source = "rfc:github-manual"
	case "your_activity":
		result.Category = domain.CategoryNotification
		result.Source = "rfc:github-your-activity"
	case "comment":
		result.Category = domain.CategoryNotification
		result.Source = "rfc:github-comment"
	case "state_change":
		result.Category = domain.CategoryNotification
		result.Source = "rfc:github-state-change"

	default:
		// Unknown reason → pass to LLM for classification
		return nil
	}

	return result
}

// getGitHubSecurityPriority is deprecated, use GetGitHubSecurityScore instead.
func (c *RFCScoreClassifier) getGitHubSecurityPriority(severity string) domain.Priority {
	switch strings.ToLower(severity) {
	case "critical":
		return domain.PriorityUrgent
	case "high":
		return domain.PriorityHigh
	case "moderate", "medium":
		return domain.PriorityNormal
	case "low":
		return domain.PriorityLow
	default:
		return domain.PriorityHigh // Default high for security
	}
}

// classifyGitLab classifies GitLab emails based on GitLab headers.
//
// Priority = DomainScore(0.18) + ReasonScore + RelationScore
//
// Inbox Strategy:
//   - CategoryWork: mentioned, assigned, review_requested
//   - CategoryNotification: pipeline, subscribed, watching
func (c *RFCScoreClassifier) classifyGitLab(headers *out.ProviderClassificationHeaders, signals []string) *ScoreClassifierResult {
	signals = append(signals, SignalDeveloperService, "gitlab")

	reason := strings.ToLower(headers.XGitLabNotificationReason)
	subCat := domain.SubCategoryDeveloper

	// Calculate priority: Domain + Reason + Relation
	reasonScore, relationScore := GetGitLabReasonScore(reason)
	priority := CalculatePriority(DomainScoreGitLab, reasonScore, relationScore, 0)

	result := &ScoreClassifierResult{
		Category:    domain.CategoryWork,
		SubCategory: &subCat,
		Priority:    domain.Priority(priority),
		Score:       0.95,
		Signals:     signals,
		LLMUsed:     false,
	}

	// Determine category and source based on reason
	switch reason {
	// === INBOX (CategoryWork) - Direct involvement ===
	case "mentioned", "directly_addressed":
		result.Source = "rfc:gitlab-mention"
	case "assigned":
		result.Source = "rfc:gitlab-assigned"
	case "review_requested":
		result.Source = "rfc:gitlab-review"
	case "approval_required":
		result.Source = "rfc:gitlab-approval"

	// === NOTIFICATION (CategoryNotification) - Passive watching ===
	case "subscribed", "watching":
		result.Category = domain.CategoryNotification
		result.Source = "rfc:gitlab-subscribed"
	case "own_activity":
		result.Category = domain.CategoryNotification
		result.Source = "rfc:gitlab-own-activity"

	default:
		// Pipeline or unknown → Notification
		if headers.XGitLabPipelineID != "" {
			result.Category = domain.CategoryNotification
			result.Source = "rfc:gitlab-pipeline"
		} else {
			// Unknown reason → pass to LLM
			return nil
		}
	}

	return result
}

// classifyByNoReply detects no-reply sender patterns.
func (c *RFCScoreClassifier) classifyByNoReply(fromEmail string, headers *out.ProviderClassificationHeaders, signals []string) *ScoreClassifierResult {
	fromLower := strings.ToLower(fromEmail)

	noReplyPatterns := []string{
		"noreply@", "no-reply@", "donotreply@", "do-not-reply@",
		"mailer-daemon@", "postmaster@", "notifications@", "alert@",
	}

	isNoReply := false
	for _, pattern := range noReplyPatterns {
		if strings.Contains(fromLower, pattern) {
			isNoReply = true
			break
		}
	}

	if !isNoReply {
		return nil
	}

	signals = append(signals, SignalNoReply)

	// No-reply + Auto-Submitted = higher confidence notification
	if headers.AutoSubmitted != "" && strings.ToLower(headers.AutoSubmitted) != "no" {
		subCat := domain.SubCategoryNotification
		return &ScoreClassifierResult{
			Category:    domain.CategoryNotification,
			SubCategory: &subCat,
			Priority:    domain.PriorityNormal,
			Score:       0.85,
			Source:      "rfc:noreply-auto",
			Signals:     signals,
			LLMUsed:     false,
		}
	}

	// No-reply + ESP = likely transaction/marketing
	isESP := headers.IsMailchimp || headers.IsSendGrid || headers.IsAmazonSES ||
		headers.IsMailgun || headers.IsPostmark
	if isESP {
		subCat := domain.SubCategoryNotification
		return &ScoreClassifierResult{
			Category:    domain.CategoryNotification,
			SubCategory: &subCat,
			Priority:    domain.PriorityNormal,
			Score:       0.78,
			Source:      "rfc:noreply-esp",
			Signals:     signals,
			LLMUsed:     false,
		}
	}

	// Just no-reply, lower confidence
	subCat := domain.SubCategoryNotification
	return &ScoreClassifierResult{
		Category:    domain.CategoryNotification,
		SubCategory: &subCat,
		Priority:    domain.PriorityNormal,
		Score:       0.70,
		Source:      "rfc:noreply",
		Signals:     signals,
		LLMUsed:     false,
	}
}
