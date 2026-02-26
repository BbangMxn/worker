// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/port/out"
	"worker_server/core/service/classification"
)

// =============================================================================
// OpsGenie Parser
// =============================================================================
//
// OpsGenie email patterns:
//   - From: opsgenie@opsgenie.net (fixed, cannot be changed)
//
// Subject patterns:
//   - "Opsgenie Alert [tinyId] Incident Raised"
//   - "Opsgenie Alert [1791] Acknowledged"
//   - "Opsgenie Alert [1791] Closed"
//   - "Opsgenie Alert [1791] Escalated"
//   - "Opsgenie Alert [1791] Note Added"
//
// Priority levels:
//   - P1: Critical
//   - P2: High
//   - P3: Moderate (Default)
//   - P4: Low
//   - P5: Informational
//
// Alert ID formats:
//   - Full UUID: 70413a06-38d6-4c85-92b8-5ebc900d42e2
//   - Tiny ID: 1791 (short numeric)

// OpsGenieParser parses OpsGenie notification emails.
type OpsGenieParser struct {
	*MonitoringBaseParser
}

// NewOpsGenieParser creates a new OpsGenie parser.
func NewOpsGenieParser() *OpsGenieParser {
	return &OpsGenieParser{
		MonitoringBaseParser: NewMonitoringBaseParser(ServiceOpsGenie),
	}
}

// OpsGenie-specific regex patterns
var (
	// Subject pattern: "Opsgenie Alert [tinyId] Status"
	opsgenieSubjectPattern = regexp.MustCompile(`(?i)Opsgenie Alert \[(\d+)\]\s*(.*)`)

	// Priority pattern in body
	opsgeniePriorityPattern = regexp.MustCompile(`(?i)Priority[:\s]*(P[1-5])`)

	// UUID pattern for full alert ID
	opsgenieAlertIDPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

	// URL pattern
	opsgenieURLPattern = regexp.MustCompile(`https://[^/]*opsgenie\.com/alert/detail/[^\s"<>]+`)

	// Message pattern
	opsgenieMessagePattern = regexp.MustCompile(`(?i)Message[:\s]*([^\n]+)`)
)

// CanParse checks if this parser can handle the email.
func (p *OpsGenieParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)

	// Check OpsGenie domain
	return strings.Contains(fromLower, "@opsgenie.net") ||
		strings.Contains(fromLower, "opsgenie")
}

// Parse extracts structured data from OpsGenie emails.
func (p *OpsGenieParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	// Detect event from content
	event := p.detectEvent(input)

	// Extract data
	data := p.extractData(input)

	// Calculate priority
	isProduction := true // OpsGenie is for production incidents

	eventScore := p.GetEventScoreForEvent(event)
	priority, score := p.CalculateMonitoringPriority(MonitoringPriorityConfig{
		DomainScore:  classification.DomainScorePagerDuty, // Similar to PagerDuty
		EventScore:   eventScore,
		Severity:     data.Severity,
		IsProduction: isProduction,
	})

	// Determine category
	category, subCat := p.DetermineMonitoringCategory(event)

	// Generate action items
	actionItems := p.GenerateMonitoringActionItems(event, data)

	// Generate entities
	entities := p.GenerateMonitoringEntities(data)

	return &ParsedEmail{
		Category:      CategoryMonitoring,
		Service:       ServiceOpsGenie,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:opsgenie:" + string(event),
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"opsgenie", "event:" + string(event), "severity:" + data.Severity},
	}, nil
}

// detectEvent detects the OpsGenie event from subject.
func (p *OpsGenieParser) detectEvent(input *ParserInput) MonitoringEventType {
	subject := ""
	if input.Message != nil {
		subject = input.Message.Subject
	}

	// Extract status from subject pattern
	if matches := opsgenieSubjectPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		status := strings.ToLower(strings.TrimSpace(matches[2]))

		switch {
		case strings.Contains(status, "incident raised") || strings.Contains(status, "created"):
			return MonEventAlertTriggered
		case strings.Contains(status, "acknowledged"):
			return MonEventAlertAcknowledged
		case strings.Contains(status, "closed") || strings.Contains(status, "resolved"):
			return MonEventAlertResolved
		case strings.Contains(status, "escalated"):
			return MonEventAlertEscalated
		case strings.Contains(status, "note"):
			return MonEventAlertTriggered // Note added, keep as active
		}
	}

	// Fallback: check keywords
	subjectLower := strings.ToLower(subject)
	switch {
	case strings.Contains(subjectLower, "incident") || strings.Contains(subjectLower, "alert"):
		return MonEventAlertTriggered
	case strings.Contains(subjectLower, "acknowledged"):
		return MonEventAlertAcknowledged
	case strings.Contains(subjectLower, "closed") || strings.Contains(subjectLower, "resolved"):
		return MonEventAlertResolved
	case strings.Contains(subjectLower, "escalated"):
		return MonEventAlertEscalated
	}

	return MonEventAlertTriggered
}

// extractData extracts structured data from the email.
func (p *OpsGenieParser) extractData(input *ParserInput) *ExtractedData {
	data := &ExtractedData{
		Extra: make(map[string]interface{}),
	}

	if input.Message == nil {
		return data
	}

	subject := input.Message.Subject
	bodyText := ""
	if input.Body != nil {
		bodyText = input.Body.Text
		if bodyText == "" {
			bodyText = input.Body.HTML
		}
	}
	combined := subject + "\n" + bodyText

	// Extract tiny ID and status from subject
	if matches := opsgenieSubjectPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.AlertID = matches[1] // Tiny ID
		data.AlertStatus = p.normalizeOpsGenieStatus(matches[2])
	}

	// Extract full UUID from body
	if matches := opsgenieAlertIDPattern.FindString(combined); matches != "" {
		data.IncidentID = matches
	}

	// Extract priority from body
	if matches := opsgeniePriorityPattern.FindStringSubmatch(combined); len(matches) >= 2 {
		data.Extra["priority_level"] = matches[1]
		data.Severity = p.mapOpsgeniePriorityToSeverity(matches[1])
	} else {
		data.Severity = "medium" // Default P3
	}

	// Extract URL
	if matches := opsgenieURLPattern.FindString(combined); matches != "" {
		data.URL = matches
	}

	// Extract message
	if matches := opsgenieMessagePattern.FindStringSubmatch(combined); len(matches) >= 2 {
		data.ErrorMessage = strings.TrimSpace(matches[1])
	}

	// Extract title from subject (after status)
	data.Title = p.extractOpsGenieTitle(subject)

	// Set service name
	data.Service = "OpsGenie Alert"

	// Default to production
	data.Environment = "production"

	return data
}

// normalizeOpsGenieStatus normalizes the status text.
func (p *OpsGenieParser) normalizeOpsGenieStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))

	switch {
	case strings.Contains(status, "incident raised") || strings.Contains(status, "created"):
		return "triggered"
	case strings.Contains(status, "acknowledged"):
		return "acknowledged"
	case strings.Contains(status, "closed") || strings.Contains(status, "resolved"):
		return "resolved"
	case strings.Contains(status, "escalated"):
		return "escalated"
	default:
		return "triggered"
	}
}

// mapOpsgeniePriorityToSeverity maps P1-P5 to severity.
func (p *OpsGenieParser) mapOpsgeniePriorityToSeverity(priority string) string {
	switch strings.ToUpper(priority) {
	case "P1":
		return "critical"
	case "P2":
		return "high"
	case "P3":
		return "medium"
	case "P4":
		return "low"
	case "P5":
		return "info"
	default:
		return "medium"
	}
}

// extractOpsGenieTitle extracts title from subject.
func (p *OpsGenieParser) extractOpsGenieTitle(subject string) string {
	// Remove "Opsgenie Alert [tinyId] Status" prefix
	if matches := opsgenieSubjectPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		status := strings.TrimSpace(matches[2])

		// If status contains a colon, the part after is the title
		if idx := strings.Index(status, ":"); idx >= 0 {
			return strings.TrimSpace(status[idx+1:])
		}

		// Otherwise, the status itself might contain the title
		// Remove known status words
		statusWords := []string{"incident raised", "acknowledged", "closed", "resolved", "escalated", "note added"}
		result := status
		for _, word := range statusWords {
			result = strings.TrimPrefix(strings.ToLower(result), word)
		}
		if result = strings.TrimSpace(result); result != "" {
			return result
		}

		return status
	}

	return subject
}
