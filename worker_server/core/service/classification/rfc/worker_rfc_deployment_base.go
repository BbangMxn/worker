// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/domain"
)

// =============================================================================
// Deployment Base Parser
// =============================================================================
//
// Common patterns for Vercel, Netlify, AWS, Heroku:
//   - Deploy: Started, Succeeded, Failed, Canceled
//   - Build: Started, Passed, Failed
//   - Domain: Configured, Misconfigured, Expiry, Renewal
//   - Usage: Alert, Limit reached
//   - Billing: Invoice, Payment failed

// DeploymentBaseParser provides common functionality for deployment tool parsers.
type DeploymentBaseParser struct {
	service  SaaSService
	category SaaSCategory
}

// NewDeploymentBaseParser creates a new base parser.
func NewDeploymentBaseParser(service SaaSService) *DeploymentBaseParser {
	return &DeploymentBaseParser{
		service:  service,
		category: CategoryDeployment,
	}
}

// Service returns the service.
func (p *DeploymentBaseParser) Service() SaaSService {
	return p.service
}

// Category returns the category.
func (p *DeploymentBaseParser) Category() SaaSCategory {
	return p.category
}

// =============================================================================
// Common Regex Patterns
// =============================================================================

var (
	// Status patterns
	deployFailedPattern    = regexp.MustCompile(`(?i)\b(?:failed|failure|error)\b`)
	deploySucceededPattern = regexp.MustCompile(`(?i)\b(?:succeeded|success|ready|completed|passed)\b`)
	deployStartedPattern   = regexp.MustCompile(`(?i)\b(?:started|building|deploying|in progress)\b`)
	deployCanceledPattern  = regexp.MustCompile(`(?i)\b(?:canceled|cancelled|aborted)\b`)

	// Domain patterns
	deployDomainPattern          = regexp.MustCompile(`(?i)\b(?:domain|ssl|certificate|https)\b`)
	deployDomainMisconfigPattern = regexp.MustCompile(`(?i)\b(?:misconfigured|invalid|error|issue)\b`)
	deployDomainExpiryPattern    = regexp.MustCompile(`(?i)\b(?:expir|renew)\b`)

	// Usage/Billing patterns
	deployUsagePattern   = regexp.MustCompile(`(?i)\b(?:usage|limit|quota|exceeded|threshold)\b`)
	deployBillingPattern = regexp.MustCompile(`(?i)\b(?:invoice|payment|billing|charge)\b`)

	// Project name extraction
	deployProjectPattern = regexp.MustCompile(`(?i)(?:for|project|site)[:\s]+["']?([^"'\n]+)["']?`)

	// URL patterns
	deployURLPattern = regexp.MustCompile(`https://[^\s"<>]+`)
)

// =============================================================================
// Common Extraction Methods
// =============================================================================

// ExtractProjectName extracts project/site name from text.
func (p *DeploymentBaseParser) ExtractProjectName(text string) string {
	if matches := deployProjectPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// ExtractURL extracts URL from text.
func (p *DeploymentBaseParser) ExtractURL(text string) string {
	if matches := deployURLPattern.FindString(text); matches != "" {
		return matches
	}
	return ""
}

// DetectDeploymentEvent detects event type from subject/body.
func (p *DeploymentBaseParser) DetectDeploymentEvent(subject, body string) DeploymentEventType {
	combined := strings.ToLower(subject + " " + body)

	// Check for domain-related events first
	if deployDomainPattern.MatchString(combined) {
		if deployDomainExpiryPattern.MatchString(combined) {
			return DeployEventDomainExpiry
		}
		if deployDomainMisconfigPattern.MatchString(combined) {
			return DeployEventFailed
		}
	}

	// Check for billing events
	if deployBillingPattern.MatchString(combined) {
		if deployFailedPattern.MatchString(combined) {
			return DeployEventBillingAlert
		}
		return DeployEventBillingAlert
	}

	// Check for usage events
	if deployUsagePattern.MatchString(combined) {
		return DeployEventUsageAlert
	}

	// Check deployment/build status
	switch {
	case deployFailedPattern.MatchString(subject):
		// Distinguish between build and deploy
		if strings.Contains(combined, "build") {
			return DeployEventBuildFailed
		}
		return DeployEventFailed
	case deploySucceededPattern.MatchString(subject):
		if strings.Contains(combined, "build") {
			return DeployEventBuildPassed
		}
		return DeployEventSucceeded
	case deployCanceledPattern.MatchString(subject):
		return DeployEventCanceled
	case deployStartedPattern.MatchString(subject):
		return DeployEventStarted
	}

	return DeployEventSucceeded
}

// =============================================================================
// Common Priority Calculation
// =============================================================================

// DeploymentPriorityConfig holds priority calculation parameters.
type DeploymentPriorityConfig struct {
	DomainScore float64
	EventScore  float64
	IsUrgent    bool
}

// CalculateDeploymentPriority calculates priority for deployment tools.
func (p *DeploymentBaseParser) CalculateDeploymentPriority(config DeploymentPriorityConfig) (domain.Priority, float64) {
	// Weighted calculation
	score := config.DomainScore*0.3 + config.EventScore*0.7

	// Urgent flag boosts priority
	if config.IsUrgent {
		score = min(score*1.3, 1.0)
	}

	return p.ScoreToPriority(score), score
}

// GetEventScoreForEvent returns event score for deployment events.
func (p *DeploymentBaseParser) GetEventScoreForEvent(event DeploymentEventType) float64 {
	switch event {
	// Critical failures - high priority
	case DeployEventFailed, DeployEventBuildFailed:
		return 0.9
	case DeployEventBillingAlert:
		return 0.85
	case DeployEventDomainExpiry:
		return 0.8

	// Warnings - medium priority
	case DeployEventUsageAlert:
		return 0.6
	case DeployEventCanceled:
		return 0.5

	// Success/info - low priority
	case DeployEventSucceeded, DeployEventBuildPassed:
		return 0.3
	case DeployEventStarted:
		return 0.2

	default:
		return 0.3
	}
}

// ScoreToPriority converts score to Priority.
func (p *DeploymentBaseParser) ScoreToPriority(score float64) domain.Priority {
	switch {
	case score >= 0.8:
		return domain.PriorityUrgent
	case score >= 0.6:
		return domain.PriorityHigh
	case score >= 0.4:
		return domain.PriorityNormal
	case score >= 0.2:
		return domain.PriorityLow
	default:
		return domain.PriorityLowest
	}
}

// =============================================================================
// Common Category Determination
// =============================================================================

// DetermineDeploymentCategory determines category based on event type.
func (p *DeploymentBaseParser) DetermineDeploymentCategory(event DeploymentEventType) (domain.EmailCategory, *domain.EmailSubCategory) {
	alertSubCat := domain.SubCategoryAlert
	notifSubCat := domain.SubCategoryNotification

	switch event {
	// Failures and urgent items → Work + Alert
	case DeployEventFailed, DeployEventBuildFailed, DeployEventBillingAlert, DeployEventDomainExpiry:
		return domain.CategoryWork, &alertSubCat

	// Warnings → Notification
	case DeployEventUsageAlert, DeployEventCanceled:
		return domain.CategoryNotification, &alertSubCat

	// Success/info → Notification
	case DeployEventSucceeded, DeployEventBuildPassed, DeployEventStarted:
		return domain.CategoryNotification, &notifSubCat

	default:
		return domain.CategoryNotification, &notifSubCat
	}
}

// =============================================================================
// Common Action Item Generation
// =============================================================================

// GenerateDeploymentActionItems generates action items based on event type.
func (p *DeploymentBaseParser) GenerateDeploymentActionItems(event DeploymentEventType, data *ExtractedData) []ActionItem {
	var items []ActionItem

	switch event {
	case DeployEventFailed, DeployEventBuildFailed:
		items = append(items, ActionItem{
			Type:        ActionFix,
			Title:       "Fix deployment failure: " + data.Project,
			Description: data.ErrorMessage,
			URL:         data.URL,
			Priority:    "high",
		})

	case DeployEventBillingAlert:
		items = append(items, ActionItem{
			Type:     ActionPay,
			Title:    "Resolve billing issue",
			URL:      data.URL,
			Priority: "urgent",
		})

	case DeployEventDomainExpiry:
		items = append(items, ActionItem{
			Type:     ActionUpdate,
			Title:    "Renew domain/certificate",
			URL:      data.URL,
			Priority: "high",
		})

	case DeployEventUsageAlert:
		items = append(items, ActionItem{
			Type:     ActionInvestigate,
			Title:    "Review usage: " + data.Project,
			URL:      data.URL,
			Priority: "medium",
		})
	}

	return items
}

// =============================================================================
// Common Entity Extraction
// =============================================================================

// GenerateDeploymentEntities generates entities from extracted data.
func (p *DeploymentBaseParser) GenerateDeploymentEntities(data *ExtractedData) []Entity {
	var entities []Entity

	// Project entity
	if data.Project != "" {
		entities = append(entities, Entity{
			Type: EntityProject,
			ID:   data.Project,
			Name: data.Project,
			URL:  data.DeploymentURL,
		})
	}

	// Deployment entity
	if data.DeploymentID != "" {
		entities = append(entities, Entity{
			Type: EntityPipeline,
			ID:   data.DeploymentID,
			Name: "Deployment " + data.DeploymentID,
			URL:  data.URL,
		})
	}

	return entities
}

// Helper function
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
