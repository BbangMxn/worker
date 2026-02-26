// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"strings"

	"worker_server/core/port/out"
	"worker_server/core/service/classification"
)

// =============================================================================
// Sentry Parser
// =============================================================================
//
// Sentry-specific patterns:
//   - From: noreply@sentry.io, alerts@sentry.io
//   - Headers: X-Sentry-Project
//   - Subject: "[Project] Error: message"
//   - Subject: "Issue Alert: message"

// SentryParser parses Sentry notification emails.
type SentryParser struct {
	*MonitoringBaseParser
}

// NewSentryParser creates a new Sentry parser.
func NewSentryParser() *SentryParser {
	return &SentryParser{
		MonitoringBaseParser: NewMonitoringBaseParser(ServiceSentry),
	}
}

// CanParse checks if this parser can handle the email.
func (p *SentryParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	// Check Sentry-specific header
	if headers != nil && headers.XSentryProject != "" {
		return true
	}

	// Check from email
	fromLower := strings.ToLower(fromEmail)
	return strings.Contains(fromLower, "sentry.io") ||
		strings.Contains(fromLower, "getsentry.com")
}

// Parse extracts structured data from Sentry emails.
func (p *SentryParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	// Detect event from content
	event := p.detectEvent(input)

	// Extract data
	data := p.extractData(input)

	// Check if production
	isProduction := data.Environment == "production" || data.Environment == "prod"

	// Calculate priority
	eventScore := p.GetEventScoreForEvent(event)
	priority, score := p.CalculateMonitoringPriority(MonitoringPriorityConfig{
		DomainScore:  classification.DomainScoreSentry,
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
		Service:       ServiceSentry,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:sentry:" + string(event),
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"sentry", "event:" + string(event), "severity:" + data.Severity},
	}, nil
}

// detectEvent detects the Sentry event from content.
func (p *SentryParser) detectEvent(input *ParserInput) MonitoringEventType {
	subject := ""
	if input.Message != nil {
		subject = strings.ToLower(input.Message.Subject)
	}

	bodyText := ""
	if input.Body != nil {
		bodyText = strings.ToLower(input.Body.Text)
	}

	switch {
	case strings.Contains(subject, "new issue") || strings.Contains(bodyText, "new issue"):
		return MonEventIssueNew
	case strings.Contains(subject, "regression") || strings.Contains(bodyText, "regressed"):
		return MonEventIssueRegressed
	case strings.Contains(subject, "resolved") || strings.Contains(bodyText, "resolved"):
		return MonEventAlertResolved
	case strings.Contains(subject, "escalated"):
		return MonEventAlertEscalated
	case strings.Contains(subject, "digest") || strings.Contains(subject, "weekly report"):
		return MonEventDigest
	case strings.Contains(subject, "alert") || strings.Contains(subject, "triggered"):
		return MonEventAlertTriggered
	}

	return MonEventIssueNew
}

// extractData extracts structured data from the email.
func (p *SentryParser) extractData(input *ParserInput) *ExtractedData {
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

	// Extract from Sentry headers
	if input.Headers != nil && input.Headers.XSentryProject != "" {
		data.Service = input.Headers.XSentryProject
	}

	// Extract project from subject [Project]
	if data.Service == "" {
		data.Service = p.extractSentryProject(subject)
	}

	// Extract service from body if still empty
	if data.Service == "" {
		data.Service = p.ExtractService(combined)
	}

	// Extract severity
	data.Severity = p.extractSentrySeverity(combined)

	// Extract environment
	data.Environment = p.ExtractEnvironment(combined)

	// Extract error message
	data.ErrorMessage = p.extractSentryError(subject, bodyText)

	// Extract error count
	data.ErrorCount = p.ExtractErrorCount(combined)

	// Extract URL
	if match := monSentryURLPattern.FindString(combined); match != "" {
		data.URL = match
	}

	// Extract alert ID
	data.AlertID = p.ExtractAlertID(combined)

	// Extract title
	data.Title = p.extractSentryTitle(subject)

	return data
}

// extractSentryProject extracts project from subject [Project].
func (p *SentryParser) extractSentryProject(subject string) string {
	if strings.HasPrefix(subject, "[") {
		if idx := strings.Index(subject, "]"); idx > 0 {
			return subject[1:idx]
		}
	}
	return ""
}

// extractSentrySeverity extracts severity from Sentry content.
func (p *SentryParser) extractSentrySeverity(text string) string {
	// First try standard severity extraction
	if severity := p.ExtractSeverity(text); severity != "" {
		return p.NormalizeSeverity(severity)
	}

	// Sentry-specific patterns
	textLower := strings.ToLower(text)
	switch {
	case strings.Contains(textLower, "critical") || strings.Contains(textLower, "fatal"):
		return "critical"
	case strings.Contains(textLower, "error"):
		return "high"
	case strings.Contains(textLower, "warning"):
		return "medium"
	case strings.Contains(textLower, "info"):
		return "low"
	}

	return "high" // Default high for Sentry
}

// extractSentryError extracts error message from Sentry email.
func (p *SentryParser) extractSentryError(subject, body string) string {
	// Try to extract from subject first (usually has the error)
	title := subject

	// Remove [Project] prefix
	if strings.HasPrefix(title, "[") {
		if idx := strings.Index(title, "]"); idx > 0 {
			title = strings.TrimSpace(title[idx+1:])
		}
	}

	// Remove common prefixes
	prefixes := []string{"Error: ", "Exception: ", "Issue Alert: ", "New Issue: "}
	for _, prefix := range prefixes {
		title = strings.TrimPrefix(title, prefix)
	}

	if title != "" && len(title) < 200 {
		return title
	}

	// Fall back to body extraction
	return p.ExtractErrorMessage(body, 150)
}

// extractSentryTitle extracts clean title from subject.
func (p *SentryParser) extractSentryTitle(subject string) string {
	title := subject

	// Remove [Project] prefix
	if strings.HasPrefix(title, "[") {
		if idx := strings.Index(title, "]"); idx > 0 {
			title = strings.TrimSpace(title[idx+1:])
		}
	}

	return strings.TrimSpace(title)
}
