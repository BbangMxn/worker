// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"strings"

	"worker_server/core/port/out"
	"worker_server/core/service/classification"
)

// =============================================================================
// PagerDuty Parser
// =============================================================================
//
// PagerDuty email patterns:
//   - From: alerts@pagerduty.com, no-reply@pagerduty.com
//   - Subject: "[TRIGGERED] Service: Alert Title"
//   - Subject: "[ACKNOWLEDGED] Service: Alert Title"
//   - Subject: "[RESOLVED] Service: Alert Title"

// PagerDutyParser parses PagerDuty notification emails.
type PagerDutyParser struct {
	*MonitoringBaseParser
}

// NewPagerDutyParser creates a new PagerDuty parser.
func NewPagerDutyParser() *PagerDutyParser {
	return &PagerDutyParser{
		MonitoringBaseParser: NewMonitoringBaseParser(ServicePagerDuty),
	}
}

// CanParse checks if this parser can handle the email.
func (p *PagerDutyParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)
	// PagerDuty sends from: alerts@pagerduty.com, no-reply@pagerduty.com
	return strings.Contains(fromLower, "@pagerduty.com") ||
		strings.Contains(fromLower, "pagerduty.com")
}

// Parse extracts structured data from PagerDuty emails.
func (p *PagerDutyParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	// Detect event from content
	event := p.detectEvent(input)

	// Extract data
	data := p.extractData(input)

	// PagerDuty is typically for production alerts
	isProduction := true

	// Calculate priority - PagerDuty has high domain score
	eventScore := p.GetEventScoreForEvent(event)
	priority, score := p.CalculateMonitoringPriority(MonitoringPriorityConfig{
		DomainScore:  classification.DomainScorePagerDuty,
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
		Service:       ServicePagerDuty,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:pagerduty:" + string(event),
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"pagerduty", "event:" + string(event), "severity:" + data.Severity},
	}, nil
}

// detectEvent detects the PagerDuty event from content.
func (p *PagerDutyParser) detectEvent(input *ParserInput) MonitoringEventType {
	subject := ""
	if input.Message != nil {
		subject = strings.ToLower(input.Message.Subject)
	}

	switch {
	case strings.Contains(subject, "[triggered]") || strings.Contains(subject, "incident triggered"):
		return MonEventAlertTriggered
	case strings.Contains(subject, "[acknowledged]"):
		return MonEventAlertAcknowledged
	case strings.Contains(subject, "[resolved]"):
		return MonEventAlertResolved
	case strings.Contains(subject, "[escalated]") || strings.Contains(subject, "escalation"):
		return MonEventAlertEscalated
	case strings.Contains(subject, "on-call") || strings.Contains(subject, "schedule"):
		return MonEventOnCallReminder
	}

	return MonEventAlertTriggered
}

// extractData extracts structured data from the email.
func (p *PagerDutyParser) extractData(input *ParserInput) *ExtractedData {
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
	}
	combined := subject + "\n" + bodyText

	// Extract status from subject [TRIGGERED], [ACKNOWLEDGED], [RESOLVED]
	data.AlertStatus = p.extractPagerDutyStatus(subject)

	// Extract service name
	data.Service = p.extractPagerDutyService(subject)
	if data.Service == "" {
		data.Service = p.ExtractService(combined)
	}

	// Extract severity/urgency
	data.Severity = p.extractPagerDutySeverity(combined)

	// Extract incident ID
	data.IncidentID = p.ExtractAlertID(combined)

	// Extract error message
	data.ErrorMessage = p.extractPagerDutyTitle(subject)

	// Extract URL
	if match := monPagerDutyURLPattern.FindString(combined); match != "" {
		data.URL = match
	}

	// Extract title
	data.Title = p.extractPagerDutyTitle(subject)

	// PagerDuty is typically production
	data.Environment = "production"

	return data
}

// extractPagerDutyStatus extracts status from subject.
func (p *PagerDutyParser) extractPagerDutyStatus(subject string) string {
	subjectLower := strings.ToLower(subject)
	switch {
	case strings.Contains(subjectLower, "[triggered]"):
		return "triggered"
	case strings.Contains(subjectLower, "[acknowledged]"):
		return "acknowledged"
	case strings.Contains(subjectLower, "[resolved]"):
		return "resolved"
	case strings.Contains(subjectLower, "[escalated]"):
		return "escalated"
	}
	return "triggered"
}

// extractPagerDutyService extracts service from subject.
func (p *PagerDutyParser) extractPagerDutyService(subject string) string {
	// Pattern: "[STATUS] Service: Title" or "Service - Title"

	// Remove status prefix
	title := subject
	statusPrefixes := []string{"[TRIGGERED] ", "[ACKNOWLEDGED] ", "[RESOLVED] ", "[ESCALATED] "}
	for _, prefix := range statusPrefixes {
		title = strings.TrimPrefix(title, prefix)
		title = strings.TrimPrefix(title, strings.ToLower(prefix))
	}

	// Extract service before : or -
	if idx := strings.Index(title, ":"); idx > 0 {
		return strings.TrimSpace(title[:idx])
	}
	if idx := strings.Index(title, " - "); idx > 0 {
		return strings.TrimSpace(title[:idx])
	}

	return ""
}

// extractPagerDutySeverity extracts severity from PagerDuty content.
func (p *PagerDutyParser) extractPagerDutySeverity(text string) string {
	// First try standard extraction
	if severity := p.ExtractSeverity(text); severity != "" {
		return p.NormalizeSeverity(severity)
	}

	// PagerDuty uses "urgency"
	textLower := strings.ToLower(text)
	switch {
	case strings.Contains(textLower, "urgency: high") || strings.Contains(textLower, "high urgency"):
		return "high"
	case strings.Contains(textLower, "urgency: low") || strings.Contains(textLower, "low urgency"):
		return "low"
	}

	return "high" // Default high for PagerDuty
}

// extractPagerDutyTitle extracts title from subject.
func (p *PagerDutyParser) extractPagerDutyTitle(subject string) string {
	title := subject

	// Remove status prefix
	statusPrefixes := []string{"[TRIGGERED] ", "[ACKNOWLEDGED] ", "[RESOLVED] ", "[ESCALATED] "}
	for _, prefix := range statusPrefixes {
		title = strings.TrimPrefix(title, prefix)
		title = strings.TrimPrefix(title, strings.ToLower(prefix))
	}

	// Remove service prefix if present
	if idx := strings.Index(title, ": "); idx > 0 {
		title = strings.TrimSpace(title[idx+2:])
	}

	return strings.TrimSpace(title)
}
