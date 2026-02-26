// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/port/out"
)

// =============================================================================
// Vercel Parser
// =============================================================================
//
// Vercel email patterns:
//   - From: notifications@vercel.com, noreply@vercel.com
//
// Subject patterns:
//   - "Deployment failed for {project-name}"
//   - "Deployment succeeded for {project-name}" / "Deployment ready: {project-name}"
//   - "Domain configured: {domain}"
//   - "Domain misconfigured: {domain}" / "Action required: {domain} configuration"
//   - "Certificate renewal failed for {domain}"
//   - "Domain expires in {N} days: {domain}"
//   - "Usage alert: {metric} exceeded threshold"
//   - "Payment failed for invoice {invoice-id}"
//   - "{user} requested to join {team}"
//
// Notification categories:
//   - Account, Alerts, Deployment, Domain, Integrations, Usage, Edge Config
//
// NOTE: No documented X-Vercel-* custom email headers.

// VercelParser parses Vercel notification emails.
type VercelParser struct {
	*DeploymentBaseParser
}

// NewVercelParser creates a new Vercel parser.
func NewVercelParser() *VercelParser {
	return &VercelParser{
		DeploymentBaseParser: NewDeploymentBaseParser(ServiceVercel),
	}
}

// Vercel-specific regex patterns
var (
	// Subject patterns
	vercelDeployFailedPattern  = regexp.MustCompile(`(?i)deployment\s+failed\s+for\s+(.+)`)
	vercelDeployReadyPattern   = regexp.MustCompile(`(?i)(?:deployment\s+(?:succeeded|ready)|ready)[:\s]+(.+)`)
	vercelBuildFailedPattern   = regexp.MustCompile(`(?i)build\s+failed\s+for\s+(.+)`)
	vercelDomainConfigPattern  = regexp.MustCompile(`(?i)domain\s+(?:configured|misconfigured)[:\s]+(.+)`)
	vercelDomainExpiryPattern  = regexp.MustCompile(`(?i)domain\s+expires?\s+in\s+(\d+)\s+days?[:\s]+(.+)`)
	vercelCertFailedPattern    = regexp.MustCompile(`(?i)certificate\s+renewal\s+failed\s+for\s+(.+)`)
	vercelUsageAlertPattern    = regexp.MustCompile(`(?i)usage\s+alert[:\s]+(.+)`)
	vercelPaymentFailedPattern = regexp.MustCompile(`(?i)payment\s+failed\s+for\s+invoice\s+(.+)`)
	vercelTeamJoinPattern      = regexp.MustCompile(`(?i)(.+)\s+requested\s+to\s+join\s+(.+)`)

	// URL pattern
	vercelURLPattern       = regexp.MustCompile(`https://vercel\.com/[^\s"<>]+`)
	vercelDeployURLPattern = regexp.MustCompile(`https://[a-z0-9-]+\.vercel\.app`)
)

// CanParse checks if this parser can handle the email.
func (p *VercelParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)

	return strings.Contains(fromLower, "@vercel.com")
}

// Parse extracts structured data from Vercel emails.
func (p *VercelParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	subject := ""
	bodyText := ""
	if input.Message != nil {
		subject = input.Message.Subject
	}
	if input.Body != nil {
		bodyText = input.Body.Text
	}

	// Detect event
	event := p.detectVercelEvent(subject)

	// Extract data
	data := p.extractData(input)

	// Calculate priority
	eventScore := p.GetEventScoreForEvent(event)
	isUrgent := event == DeployEventBillingAlert || event == DeployEventDomainExpiry

	priority, score := p.CalculateDeploymentPriority(DeploymentPriorityConfig{
		DomainScore: 0.3, // Vercel is important for web deployments
		EventScore:  eventScore,
		IsUrgent:    isUrgent,
	})

	// Determine category
	category, subCat := p.DetermineDeploymentCategory(event)

	// Generate action items
	actionItems := p.GenerateDeploymentActionItems(event, data)

	// Generate entities
	entities := p.GenerateDeploymentEntities(data)

	return &ParsedEmail{
		Category:      CategoryDeployment,
		Service:       ServiceVercel,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:vercel:" + string(event),
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       p.generateSignals(event, subject, bodyText),
	}, nil
}

// detectVercelEvent detects the Vercel event from subject.
func (p *VercelParser) detectVercelEvent(subject string) DeploymentEventType {
	switch {
	// Failures - high priority
	case vercelDeployFailedPattern.MatchString(subject):
		return DeployEventFailed
	case vercelBuildFailedPattern.MatchString(subject):
		return DeployEventBuildFailed
	case vercelPaymentFailedPattern.MatchString(subject):
		return DeployEventBillingAlert
	case vercelCertFailedPattern.MatchString(subject):
		return DeployEventDomainExpiry

	// Domain issues
	case strings.Contains(strings.ToLower(subject), "misconfigured"):
		return DeployEventFailed
	case vercelDomainExpiryPattern.MatchString(subject):
		return DeployEventDomainExpiry

	// Usage
	case vercelUsageAlertPattern.MatchString(subject):
		return DeployEventUsageAlert

	// Success
	case vercelDeployReadyPattern.MatchString(subject):
		return DeployEventSucceeded
	case vercelDomainConfigPattern.MatchString(subject) && !strings.Contains(strings.ToLower(subject), "misconfigured"):
		return DeployEventSucceeded

	// Started
	case strings.Contains(strings.ToLower(subject), "building") || strings.Contains(strings.ToLower(subject), "deploying"):
		return DeployEventStarted
	}

	// Fallback to base detection
	return p.DetectDeploymentEvent(subject, "")
}

// extractData extracts structured data from the email.
func (p *VercelParser) extractData(input *ParserInput) *ExtractedData {
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

	// Extract project from subject patterns
	data.Project = p.extractVercelProject(subject)

	// Extract URLs
	if matches := vercelURLPattern.FindString(combined); matches != "" {
		data.URL = matches
	}
	if matches := vercelDeployURLPattern.FindString(combined); matches != "" {
		data.DeploymentURL = matches
	}

	// Extract domain if domain-related
	if domain := p.extractVercelDomain(subject); domain != "" {
		data.Extra["domain"] = domain
	}

	// Extract expiry days if applicable
	if matches := vercelDomainExpiryPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Extra["expiry_days"] = matches[1]
		data.Extra["domain"] = matches[2]
	}

	// Extract invoice ID if payment failure
	if matches := vercelPaymentFailedPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		data.InvoiceNumber = matches[1]
	}

	// Set title
	data.Title = p.extractVercelTitle(subject, data.Project)

	// Extract error message from body
	data.ErrorMessage = p.extractVercelError(bodyText)

	return data
}

// extractVercelProject extracts project name from subject.
func (p *VercelParser) extractVercelProject(subject string) string {
	// Try deployment patterns
	if matches := vercelDeployFailedPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	if matches := vercelDeployReadyPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	if matches := vercelBuildFailedPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	if matches := vercelUsageAlertPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}

	// Fallback: use base extraction
	return p.ExtractProjectName(subject)
}

// extractVercelDomain extracts domain from subject.
func (p *VercelParser) extractVercelDomain(subject string) string {
	if matches := vercelDomainConfigPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	if matches := vercelCertFailedPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// extractVercelTitle extracts clean title from subject.
func (p *VercelParser) extractVercelTitle(subject, project string) string {
	if project != "" {
		return project
	}
	return subject
}

// extractVercelError extracts error message from body.
func (p *VercelParser) extractVercelError(body string) string {
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		lineLower := strings.ToLower(line)
		if strings.Contains(lineLower, "error") || strings.Contains(lineLower, "failed") {
			if len(line) > 200 {
				return line[:200] + "..."
			}
			return line
		}
	}
	return ""
}

// generateSignals generates signals for the parsed email.
func (p *VercelParser) generateSignals(event DeploymentEventType, subject, body string) []string {
	signals := []string{"vercel", "event:" + string(event)}

	if strings.Contains(strings.ToLower(subject), "production") {
		signals = append(signals, "env:production")
	}
	if strings.Contains(strings.ToLower(subject), "preview") {
		signals = append(signals, "env:preview")
	}

	return signals
}
