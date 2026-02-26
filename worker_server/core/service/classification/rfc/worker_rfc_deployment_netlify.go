// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/port/out"
)

// =============================================================================
// Netlify Parser
// =============================================================================
//
// Netlify email patterns:
//   - From: no-reply@netlify.com (identity-generated emails)
//   - From: formresponses@netlify.com (form submissions)
//   - From: team@netlify.com (team notifications)
//
// Subject patterns:
//   - "Your recent build for {site-name} has failed"
//   - "Your recent build for {site-name} has succeeded"
//   - "Deploy started for {site-name}"
//   - "Deploy deleted for {site-name}"
//   - "Deploys locked for {site-name}"
//   - "Deploys unlocked for {site-name}"
//   - "{site-name} is back up" (recovery)
//   - "{site-name} build now failing" (regression)
//   - "New submission from {form-name}" (form)
//
// Deploy events:
//   - deploy_started, deploy_succeeded, deploy_failed, deploy_deleted
//   - deploy_locked, deploy_unlocked
//   - deploy_request_pending, deploy_request_accepted, deploy_request_rejected
//
// NOTE: Netlify uses X-Webhook-Signature for webhooks (not email).

// NetlifyParser parses Netlify notification emails.
type NetlifyParser struct {
	*DeploymentBaseParser
}

// NewNetlifyParser creates a new Netlify parser.
func NewNetlifyParser() *NetlifyParser {
	return &NetlifyParser{
		DeploymentBaseParser: NewDeploymentBaseParser(ServiceNetlify),
	}
}

// Netlify-specific regex patterns
var (
	// Subject patterns
	netlifyBuildFailedPattern    = regexp.MustCompile(`(?i)(?:build|deploy).*for\s+(.+?)\s+has\s+failed`)
	netlifyBuildSucceededPattern = regexp.MustCompile(`(?i)(?:build|deploy).*for\s+(.+?)\s+has\s+succeeded`)
	netlifyDeployStartedPattern  = regexp.MustCompile(`(?i)deploy\s+started\s+for\s+(.+)`)
	netlifyDeployDeletedPattern  = regexp.MustCompile(`(?i)deploy\s+deleted\s+for\s+(.+)`)
	netlifyDeployLockedPattern   = regexp.MustCompile(`(?i)deploys\s+(?:locked|unlocked)\s+for\s+(.+)`)
	netlifyRecoveryPattern       = regexp.MustCompile(`(?i)(.+?)\s+is\s+back\s+up`)
	netlifyRegressionPattern     = regexp.MustCompile(`(?i)(.+?)\s+(?:build\s+)?now\s+failing`)
	netlifyFormSubmissionPattern = regexp.MustCompile(`(?i)new\s+submission\s+from\s+(.+)`)

	// URL patterns
	netlifyURLPattern     = regexp.MustCompile(`https://(?:app\.)?netlify\.com/[^\s"<>]+`)
	netlifySiteURLPattern = regexp.MustCompile(`https://[a-z0-9-]+\.netlify\.app`)
)

// CanParse checks if this parser can handle the email.
func (p *NetlifyParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)

	return strings.Contains(fromLower, "@netlify.com")
}

// Parse extracts structured data from Netlify emails.
func (p *NetlifyParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	subject := ""
	if input.Message != nil {
		subject = input.Message.Subject
	}

	// Detect event
	event := p.detectNetlifyEvent(subject)

	// Extract data
	data := p.extractData(input)

	// Calculate priority
	eventScore := p.GetEventScoreForEvent(event)
	isUrgent := event == DeployEventFailed || event == DeployEventBuildFailed

	priority, score := p.CalculateDeploymentPriority(DeploymentPriorityConfig{
		DomainScore: 0.28, // Netlify is important for static sites
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
		Service:       ServiceNetlify,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:netlify:" + string(event),
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"netlify", "event:" + string(event)},
	}, nil
}

// detectNetlifyEvent detects the Netlify event from subject.
func (p *NetlifyParser) detectNetlifyEvent(subject string) DeploymentEventType {
	subjectLower := strings.ToLower(subject)

	switch {
	// Failures
	case netlifyBuildFailedPattern.MatchString(subject):
		return DeployEventBuildFailed
	case netlifyRegressionPattern.MatchString(subject):
		return DeployEventBuildFailed

	// Success
	case netlifyBuildSucceededPattern.MatchString(subject):
		return DeployEventSucceeded
	case netlifyRecoveryPattern.MatchString(subject):
		return DeployEventSucceeded

	// Started
	case netlifyDeployStartedPattern.MatchString(subject):
		return DeployEventStarted

	// Other states
	case netlifyDeployDeletedPattern.MatchString(subject):
		return DeployEventCanceled
	case strings.Contains(subjectLower, "locked"):
		return DeployEventCanceled
	case strings.Contains(subjectLower, "unlocked"):
		return DeployEventStarted

	// Form submission (treat as low priority notification)
	case netlifyFormSubmissionPattern.MatchString(subject):
		return DeployEventSucceeded
	}

	// Fallback
	return p.DetectDeploymentEvent(subject, "")
}

// extractData extracts structured data from the email.
func (p *NetlifyParser) extractData(input *ParserInput) *ExtractedData {
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

	// Extract site name from subject
	data.Project = p.extractNetlifySiteName(subject)

	// Extract URLs
	if matches := netlifyURLPattern.FindString(combined); matches != "" {
		data.URL = matches
	}
	if matches := netlifySiteURLPattern.FindString(combined); matches != "" {
		data.DeploymentURL = matches
	}

	// Check for form submission
	if matches := netlifyFormSubmissionPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		data.Extra["form_name"] = matches[1]
	}

	// Set title
	data.Title = p.extractNetlifyTitle(subject, data.Project)

	// Extract error message from body
	data.ErrorMessage = p.extractNetlifyError(bodyText)

	return data
}

// extractNetlifySiteName extracts site name from subject.
func (p *NetlifyParser) extractNetlifySiteName(subject string) string {
	// Try various patterns
	patterns := []*regexp.Regexp{
		netlifyBuildFailedPattern,
		netlifyBuildSucceededPattern,
		netlifyDeployStartedPattern,
		netlifyDeployDeletedPattern,
		netlifyDeployLockedPattern,
		netlifyRecoveryPattern,
		netlifyRegressionPattern,
	}

	for _, pattern := range patterns {
		if matches := pattern.FindStringSubmatch(subject); len(matches) >= 2 {
			return strings.TrimSpace(matches[1])
		}
	}

	// Fallback
	return p.ExtractProjectName(subject)
}

// extractNetlifyTitle extracts clean title from subject.
func (p *NetlifyParser) extractNetlifyTitle(subject, project string) string {
	if project != "" {
		return project
	}
	return subject
}

// extractNetlifyError extracts error message from body.
func (p *NetlifyParser) extractNetlifyError(body string) string {
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		lineLower := strings.ToLower(line)
		if strings.Contains(lineLower, "error") || strings.Contains(lineLower, "failed") || strings.Contains(lineLower, "build log") {
			if len(line) > 200 {
				return line[:200] + "..."
			}
			return line
		}
	}
	return ""
}
