// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/port/out"
	"worker_server/core/service/classification"
)

// =============================================================================
// Datadog Parser
// =============================================================================
//
// Datadog email patterns:
//   - From: *@datadoghq.com (likely no-reply@datadoghq.com, alerts@datadoghq.com)
//   - From: *@synthetics.dtdg.co (synthetic monitoring)
//
// Subject patterns (status prefix in brackets):
//   - "[Triggered] [Monitor Name]"
//   - "[Warning] [Monitor Name]"
//   - "[Recovered] [Monitor Name]"
//   - "[No Data] [Monitor Name]"
//   - "[TEST] [Monitor Name]"
//
// Subject variables (in body):
//   - {{value}}: Value that breached threshold
//   - {{threshold}}: Alert threshold value
//   - {{host.name}}: Hostname
//
// NOTE: No documented X-Datadog-* custom email headers.

// DatadogParser parses Datadog notification emails.
type DatadogParser struct {
	*MonitoringBaseParser
}

// NewDatadogParser creates a new Datadog parser.
func NewDatadogParser() *DatadogParser {
	return &DatadogParser{
		MonitoringBaseParser: NewMonitoringBaseParser(ServiceDatadog),
	}
}

// Datadog-specific regex patterns
var (
	// Subject status pattern: [Status] [Monitor Name] or [Status] Monitor Name
	datadogStatusPattern  = regexp.MustCompile(`^\[(Triggered|Recovered|Warning|No Data|TEST)\]`)
	datadogMonitorPattern = regexp.MustCompile(`^\[(?:Triggered|Recovered|Warning|No Data|TEST)\]\s*\[?([^\]]+)\]?`)

	// Value/threshold extraction from body
	datadogValuePattern     = regexp.MustCompile(`(?i)(?:value|current)[:\s]*([0-9.]+)`)
	datadogThresholdPattern = regexp.MustCompile(`(?i)threshold[:\s]*([0-9.]+)`)
	datadogHostPattern      = regexp.MustCompile(`(?i)host[:\s]*([^\s\n<]+)`)

	// URL pattern
	datadogURLPattern = regexp.MustCompile(`https://(?:app\.)?datadoghq\.com/[^\s"<>]+`)
)

// CanParse checks if this parser can handle the email.
func (p *DatadogParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)

	// Check Datadog domains
	return strings.Contains(fromLower, "@datadoghq.com") ||
		strings.Contains(fromLower, "@dtdg.co") ||
		strings.Contains(fromLower, "datadog")
}

// Parse extracts structured data from Datadog emails.
func (p *DatadogParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	// Detect event from content
	event := p.detectEvent(input)

	// Extract data
	data := p.extractData(input)

	// Calculate priority based on status
	isProduction := true // Datadog is typically for production monitoring

	eventScore := p.GetEventScoreForEvent(event)
	priority, score := p.CalculateMonitoringPriority(MonitoringPriorityConfig{
		DomainScore:  classification.DomainScoreSentry, // Similar to Sentry
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
		Service:       ServiceDatadog,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:datadog:" + string(event),
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"datadog", "event:" + string(event), "severity:" + data.Severity},
	}, nil
}

// detectEvent detects the Datadog event from subject.
func (p *DatadogParser) detectEvent(input *ParserInput) MonitoringEventType {
	subject := ""
	if input.Message != nil {
		subject = input.Message.Subject
	}

	// Extract status from subject
	if matches := datadogStatusPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		status := strings.ToLower(matches[1])
		switch status {
		case "triggered":
			return MonEventAlertTriggered
		case "warning":
			return MonEventAlertTriggered // Warning is also an alert
		case "recovered":
			return MonEventAlertResolved
		case "no data":
			return MonEventAlertTriggered // No data is concerning
		case "test":
			return MonEventAlertTriggered // Test notification
		}
	}

	// Fallback: check keywords
	subjectLower := strings.ToLower(subject)
	switch {
	case strings.Contains(subjectLower, "triggered") || strings.Contains(subjectLower, "alert"):
		return MonEventAlertTriggered
	case strings.Contains(subjectLower, "recovered") || strings.Contains(subjectLower, "resolved"):
		return MonEventAlertResolved
	case strings.Contains(subjectLower, "warning") || strings.Contains(subjectLower, "warn"):
		return MonEventAlertTriggered
	}

	return MonEventAlertTriggered
}

// extractData extracts structured data from the email.
func (p *DatadogParser) extractData(input *ParserInput) *ExtractedData {
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

	// Extract status from subject
	data.AlertStatus = p.extractDatadogStatus(subject)

	// Extract monitor name from subject
	data.Service = p.extractDatadogMonitorName(subject)
	data.Title = data.Service

	// Map status to severity
	data.Severity = p.mapStatusToSeverity(data.AlertStatus)

	// Extract URL
	if matches := datadogURLPattern.FindString(combined); matches != "" {
		data.URL = matches
	}

	// Extract value and threshold from body
	if matches := datadogValuePattern.FindStringSubmatch(bodyText); len(matches) >= 2 {
		data.Extra["value"] = matches[1]
	}
	if matches := datadogThresholdPattern.FindStringSubmatch(bodyText); len(matches) >= 2 {
		data.Extra["threshold"] = matches[1]
	}

	// Extract host
	if matches := datadogHostPattern.FindStringSubmatch(combined); len(matches) >= 2 {
		data.Extra["host"] = matches[1]
	}

	// Extract environment (look for common patterns)
	data.Environment = p.extractDatadogEnvironment(combined)

	// Extract error message from body (first significant line)
	data.ErrorMessage = p.extractDatadogMessage(bodyText)

	return data
}

// extractDatadogStatus extracts status from subject.
func (p *DatadogParser) extractDatadogStatus(subject string) string {
	if matches := datadogStatusPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		return strings.ToLower(matches[1])
	}
	return "triggered"
}

// extractDatadogMonitorName extracts monitor name from subject.
func (p *DatadogParser) extractDatadogMonitorName(subject string) string {
	if matches := datadogMonitorPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}

	// Fallback: remove status prefix and use rest
	subject = datadogStatusPattern.ReplaceAllString(subject, "")
	subject = strings.TrimSpace(subject)
	subject = strings.Trim(subject, "[]")
	return subject
}

// mapStatusToSeverity maps Datadog status to severity level.
func (p *DatadogParser) mapStatusToSeverity(status string) string {
	switch status {
	case "triggered":
		return "high"
	case "warning":
		return "medium"
	case "no data":
		return "medium"
	case "recovered":
		return "low"
	case "test":
		return "low"
	default:
		return "medium"
	}
}

// extractDatadogEnvironment extracts environment from content.
func (p *DatadogParser) extractDatadogEnvironment(text string) string {
	textLower := strings.ToLower(text)

	// Check for common environment tags
	envPatterns := []struct {
		pattern string
		env     string
	}{
		{"env:prod", "production"},
		{"env:production", "production"},
		{"env:staging", "staging"},
		{"env:stage", "staging"},
		{"env:dev", "development"},
		{"env:development", "development"},
		{"production", "production"},
		{"staging", "staging"},
	}

	for _, ep := range envPatterns {
		if strings.Contains(textLower, ep.pattern) {
			return ep.env
		}
	}

	return "production" // Default to production for Datadog
}

// extractDatadogMessage extracts error message from body.
func (p *DatadogParser) extractDatadogMessage(body string) string {
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines and common headers
		if line == "" || strings.HasPrefix(line, "http") || strings.HasPrefix(line, "View") {
			continue
		}
		// Return first substantive line (up to 200 chars)
		if len(line) > 10 {
			if len(line) > 200 {
				return line[:200] + "..."
			}
			return line
		}
	}
	return ""
}
