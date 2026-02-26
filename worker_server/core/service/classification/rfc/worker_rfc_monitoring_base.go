// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/domain"
	"worker_server/core/service/classification"
)

// =============================================================================
// Monitoring Base Parser
// =============================================================================
//
// Common patterns for Sentry, PagerDuty, Datadog, OpsGenie:
//   - Alert: Triggered, Acknowledged, Resolved, Escalated
//   - Incident: Created, Updated, Resolved
//   - Error: New issue, Regression, Spike
//   - On-call: Reminder, Schedule change

// MonitoringBaseParser provides common functionality for monitoring tool parsers.
type MonitoringBaseParser struct {
	service  SaaSService
	category SaaSCategory
}

// NewMonitoringBaseParser creates a new base parser.
func NewMonitoringBaseParser(service SaaSService) *MonitoringBaseParser {
	return &MonitoringBaseParser{
		service:  service,
		category: CategoryMonitoring,
	}
}

// Service returns the service.
func (p *MonitoringBaseParser) Service() SaaSService {
	return p.service
}

// Category returns the category.
func (p *MonitoringBaseParser) Category() SaaSCategory {
	return p.category
}

// =============================================================================
// Common Regex Patterns
// =============================================================================

var (
	// Severity patterns
	monSeverityPattern = regexp.MustCompile(`(?i)(?:severity|priority|level)[:\s]*(critical|high|medium|low|warning|info|error|fatal|urgent|p1|p2|p3|p4|p5)`)

	// Error/Alert ID patterns
	monAlertIDPattern    = regexp.MustCompile(`(?i)(?:alert|incident|issue)[:\s#]*([A-Za-z0-9-]+)`)
	monErrorCountPattern = regexp.MustCompile(`(?i)(\d+)\s*(?:errors?|occurrences?|events?)`)

	// Service patterns
	monServicePattern = regexp.MustCompile(`(?i)(?:service|project|app)[:\s]*([^\n<,]+)`)
	monEnvPattern     = regexp.MustCompile(`(?i)(?:environment|env)[:\s]*(production|staging|development|dev|prod|stage)`)

	// Error message pattern
	monErrorMsgPattern = regexp.MustCompile(`(?i)(?:error|exception|message)[:\s]*["']?([^"'\n]+)["']?`)

	// URL patterns
	monSentryURLPattern    = regexp.MustCompile(`https://[^.]+\.sentry\.io/[^\s<>"]+`)
	monPagerDutyURLPattern = regexp.MustCompile(`https://[^.]+\.pagerduty\.com/[^\s<>"]+`)
	monDatadogURLPattern   = regexp.MustCompile(`https://app\.datadoghq\.com/[^\s<>"]+`)
	monOpsGenieURLPattern  = regexp.MustCompile(`https://[^.]+\.opsgenie\.com/[^\s<>"]+`)
)

// =============================================================================
// Common Extraction Methods
// =============================================================================

// ExtractSeverity extracts severity from text.
func (p *MonitoringBaseParser) ExtractSeverity(text string) string {
	if matches := monSeverityPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return strings.ToLower(strings.TrimSpace(matches[1]))
	}
	return ""
}

// NormalizeSeverity normalizes different severity formats.
func (p *MonitoringBaseParser) NormalizeSeverity(severity string) string {
	switch strings.ToLower(severity) {
	case "critical", "fatal", "urgent", "p1":
		return "critical"
	case "high", "error", "p2":
		return "high"
	case "medium", "warning", "p3":
		return "medium"
	case "low", "info", "p4", "p5":
		return "low"
	default:
		return severity
	}
}

// ExtractAlertID extracts alert/incident ID from text.
func (p *MonitoringBaseParser) ExtractAlertID(text string) string {
	if matches := monAlertIDPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// ExtractErrorCount extracts error count from text.
func (p *MonitoringBaseParser) ExtractErrorCount(text string) int {
	if matches := monErrorCountPattern.FindStringSubmatch(text); len(matches) >= 2 {
		// Simple conversion
		var count int
		for _, c := range matches[1] {
			if c >= '0' && c <= '9' {
				count = count*10 + int(c-'0')
			}
		}
		return count
	}
	return 0
}

// ExtractService extracts service name from text.
func (p *MonitoringBaseParser) ExtractService(text string) string {
	if matches := monServicePattern.FindStringSubmatch(text); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// ExtractEnvironment extracts environment from text.
func (p *MonitoringBaseParser) ExtractEnvironment(text string) string {
	if matches := monEnvPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return strings.ToLower(strings.TrimSpace(matches[1]))
	}
	return ""
}

// ExtractErrorMessage extracts error message from text.
func (p *MonitoringBaseParser) ExtractErrorMessage(text string, maxLength int) string {
	if matches := monErrorMsgPattern.FindStringSubmatch(text); len(matches) >= 2 {
		msg := strings.TrimSpace(matches[1])
		if len(msg) > maxLength {
			return msg[:maxLength] + "..."
		}
		return msg
	}
	return ""
}

// =============================================================================
// Common Priority Calculation
// =============================================================================

// MonitoringPriorityConfig holds priority calculation parameters.
type MonitoringPriorityConfig struct {
	DomainScore  float64
	EventScore   float64
	Severity     string
	IsProduction bool
}

// CalculateMonitoringPriority calculates priority for monitoring tools.
func (p *MonitoringBaseParser) CalculateMonitoringPriority(config MonitoringPriorityConfig) (domain.Priority, float64) {
	// Adjust score based on severity
	severityScore := p.GetSeverityScore(config.Severity)

	// Production environment gets higher priority
	envMultiplier := 1.0
	if config.IsProduction {
		envMultiplier = 1.2
	}

	score := classification.CalculatePriority(
		config.DomainScore,
		config.EventScore*envMultiplier,
		0, // relation score not used for monitoring
		severityScore,
	)

	return p.ScoreToPriority(score), score
}

// GetSeverityScore returns severity score.
func (p *MonitoringBaseParser) GetSeverityScore(severity string) float64 {
	switch p.NormalizeSeverity(severity) {
	case "critical":
		return classification.SeverityScoreCritical
	case "high":
		return classification.SeverityScoreHigh
	case "medium":
		return classification.SeverityScoreMedium
	case "low":
		return classification.SeverityScoreLow
	default:
		return classification.SeverityScoreInfo
	}
}

// GetEventScoreForEvent returns event score for monitoring events.
func (p *MonitoringBaseParser) GetEventScoreForEvent(event MonitoringEventType) float64 {
	switch event {
	case MonEventAlertTriggered, MonEventIncidentCreated:
		return classification.ReasonScoreAlertCritical
	case MonEventAlertEscalated:
		return classification.ReasonScoreAlertCritical
	case MonEventAlertAcknowledged:
		return 0.10
	case MonEventAlertResolved, MonEventIncidentResolved:
		return 0.05
	case MonEventIssueNew:
		return classification.ReasonScoreAlertWarning
	case MonEventIssueRegressed:
		return classification.ReasonScoreAlertWarning + 0.05
	case MonEventDigest:
		return 0.02
	case MonEventOnCallReminder:
		return 0.15
	default:
		return 0.10
	}
}

// ScoreToPriority converts score to Priority.
func (p *MonitoringBaseParser) ScoreToPriority(score float64) domain.Priority {
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

// DetermineMonitoringCategory determines category based on event type.
func (p *MonitoringBaseParser) DetermineMonitoringCategory(event MonitoringEventType) (domain.EmailCategory, *domain.EmailSubCategory) {
	alertSubCat := domain.SubCategoryAlert
	notifSubCat := domain.SubCategoryNotification

	switch event {
	// Active alerts → Work + Alert
	case MonEventAlertTriggered, MonEventIncidentCreated, MonEventAlertEscalated,
		MonEventIssueNew, MonEventIssueRegressed:
		return domain.CategoryWork, &alertSubCat

	// Resolved/Acknowledged → Notification
	case MonEventAlertResolved, MonEventAlertAcknowledged, MonEventIncidentResolved:
		return domain.CategoryNotification, &notifSubCat

	// Digests and reminders → Notification
	case MonEventDigest, MonEventOnCallReminder:
		return domain.CategoryNotification, &notifSubCat

	default:
		return domain.CategoryWork, &alertSubCat
	}
}

// =============================================================================
// Common Action Item Generation
// =============================================================================

// GenerateMonitoringActionItems generates action items based on event type.
func (p *MonitoringBaseParser) GenerateMonitoringActionItems(event MonitoringEventType, data *ExtractedData) []ActionItem {
	var items []ActionItem

	// Determine priority based on severity
	priority := "medium"
	switch p.NormalizeSeverity(data.Severity) {
	case "critical":
		priority = "urgent"
	case "high":
		priority = "high"
	}

	// Production environment always high priority
	if data.Environment == "production" || data.Environment == "prod" {
		if priority == "medium" {
			priority = "high"
		}
	}

	alertRef := data.AlertID
	if alertRef == "" {
		alertRef = data.Service
	}

	switch event {
	case MonEventAlertTriggered, MonEventIncidentCreated:
		items = append(items, ActionItem{
			Type:        ActionInvestigate,
			Title:       "Investigate alert: " + alertRef,
			Description: data.ErrorMessage,
			URL:         data.URL,
			Priority:    priority,
		})

	case MonEventAlertEscalated:
		items = append(items, ActionItem{
			Type:        ActionAcknowledge,
			Title:       "Urgent: Escalated alert " + alertRef,
			Description: data.ErrorMessage,
			URL:         data.URL,
			Priority:    "urgent",
		})

	case MonEventIssueNew:
		items = append(items, ActionItem{
			Type:        ActionInvestigate,
			Title:       "New error in " + data.Service,
			Description: data.ErrorMessage,
			URL:         data.URL,
			Priority:    priority,
		})

	case MonEventIssueRegressed:
		items = append(items, ActionItem{
			Type:        ActionFix,
			Title:       "Regression: " + data.ErrorMessage,
			Description: "Previously resolved issue has regressed",
			URL:         data.URL,
			Priority:    "high",
		})

	case MonEventOnCallReminder:
		items = append(items, ActionItem{
			Type:     ActionRead,
			Title:    "On-call reminder",
			URL:      data.URL,
			Priority: "medium",
		})
	}

	return items
}

// =============================================================================
// Common Entity Extraction
// =============================================================================

// GenerateMonitoringEntities generates entities from extracted data.
func (p *MonitoringBaseParser) GenerateMonitoringEntities(data *ExtractedData) []Entity {
	var entities []Entity

	// Service
	if data.Service != "" {
		entities = append(entities, Entity{
			Type: EntityService,
			ID:   data.Service,
			Name: data.Service,
		})
	}

	// Alert
	if data.AlertID != "" {
		entities = append(entities, Entity{
			Type: EntityAlert,
			ID:   data.AlertID,
			Name: "Alert " + data.AlertID,
			URL:  data.URL,
		})
	}

	// Incident
	if data.IncidentID != "" {
		entities = append(entities, Entity{
			Type: EntityIncident,
			ID:   data.IncidentID,
			Name: "Incident " + data.IncidentID,
			URL:  data.URL,
		})
	}

	return entities
}
