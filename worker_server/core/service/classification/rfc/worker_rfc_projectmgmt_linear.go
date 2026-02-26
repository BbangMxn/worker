// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/port/out"
	"worker_server/core/service/classification"
)

// =============================================================================
// Linear Parser
// =============================================================================
//
// NOTE: Linear does NOT send custom X-Linear-* headers in email notifications.
// Custom headers are only available via webhooks.
//
// Email detection:
//   - From: notifications@linear.app
//
// Subject patterns:
//   - [Team] Issue Title (ABC-123)
//   - [Team] @user mentioned you in Issue Title

// LinearParser parses Linear notification emails.
type LinearParser struct {
	*ProjectMgmtBaseParser
}

// NewLinearParser creates a new Linear parser.
func NewLinearParser() *LinearParser {
	return &LinearParser{
		ProjectMgmtBaseParser: NewProjectMgmtBaseParser(ServiceLinear),
	}
}

// CanParse checks if this parser can handle the email.
func (p *LinearParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	// NOTE: Linear does NOT send X-Linear-* headers in emails (only webhooks)
	// Detection is based on from email domain only
	return strings.Contains(strings.ToLower(fromEmail), "linear.app")
}

// Parse extracts structured data from Linear emails.
func (p *LinearParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	// Detect action from content
	action := p.detectAction(input)

	// Determine event type
	event := p.determineEvent(action)

	// Extract data
	data := p.extractData(input)

	// Get tool priority for adjustment
	toolPriority := ""
	if tp, ok := data.Extra["tool_priority"].(string); ok {
		toolPriority = tp
	}

	// Calculate priority
	actionScore, relationScore := p.GetActionScoreForEvent(event)
	priority, score := p.CalculateProjectMgmtPriority(ProjectMgmtPriorityConfig{
		DomainScore:   classification.DomainScoreLinear,
		ActionScore:   actionScore,
		RelationScore: relationScore,
		ToolPriority:  toolPriority,
	})

	// Determine category
	category, subCat := p.DetermineProjectMgmtCategory(event)

	// Generate action items
	actionItems := p.GenerateProjectMgmtActionItems(event, data)

	// Generate entities
	entities := p.GenerateProjectMgmtEntities(data)

	return &ParsedEmail{
		Category:      CategoryProjectMgmt,
		Service:       ServiceLinear,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:linear:" + action,
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"linear", "action:" + action},
	}, nil
}

// detectAction detects the Linear action from content.
func (p *LinearParser) detectAction(input *ParserInput) string {
	subject := ""
	if input.Message != nil {
		subject = strings.ToLower(input.Message.Subject)
	}

	bodyText := ""
	if input.Body != nil {
		bodyText = strings.ToLower(input.Body.Text)
	}

	switch {
	case strings.Contains(subject, "assigned to you") || strings.Contains(bodyText, "assigned to you"):
		return "assigned"
	case strings.Contains(subject, "mentioned you") || strings.Contains(bodyText, "mentioned you"):
		return "mentioned"
	case strings.Contains(subject, "commented") || strings.Contains(bodyText, "commented"):
		return "commented"
	case strings.Contains(subject, "created") || strings.Contains(bodyText, "created"):
		return "created"
	case strings.Contains(subject, "completed") || strings.Contains(bodyText, "completed"):
		return "completed"
	case strings.Contains(subject, "updated") || strings.Contains(bodyText, "updated"):
		return "updated"
	case strings.Contains(subject, "cycle") && strings.Contains(subject, "started"):
		return "cycle_started"
	case strings.Contains(subject, "cycle") && strings.Contains(subject, "ended"):
		return "cycle_ended"
	}

	return "updated"
}

// determineEvent determines the event type from action.
func (p *LinearParser) determineEvent(action string) ProjectMgmtEventType {
	switch action {
	case "created":
		return PMEventIssueCreated
	case "assigned":
		return PMEventIssueAssigned
	case "mentioned":
		return PMEventIssueMentioned
	case "updated":
		return PMEventIssueUpdated
	case "commented":
		return PMEventIssueComment
	case "completed":
		return PMEventIssueClosed
	case "cycle_started":
		return PMEventCycleStarted
	case "cycle_ended":
		return PMEventCycleEnded
	default:
		return PMEventIssueUpdated
	}
}

// Linear-specific regex patterns
var (
	linearTeamPatternRFC     = regexp.MustCompile(`^\[([^\]]+)\]`)
	linearIssueKeyPatternRFC = regexp.MustCompile(`\(([A-Z]+-\d+)\)`)
	linearPriorityPatternRFC = regexp.MustCompile(`(?i)priority[:\s]*(Urgent|High|Medium|Low|No Priority)`)
)

// extractData extracts structured data from the email.
func (p *LinearParser) extractData(input *ParserInput) *ExtractedData {
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

	// NOTE: Linear does not provide X-Linear-* headers in emails
	// All data is extracted from subject and body

	// Extract team from subject [Team]
	if matches := linearTeamPatternRFC.FindStringSubmatch(subject); len(matches) >= 2 {
		data.Team = matches[1]
	}

	// Extract issue key (ABC-123)
	if matches := linearIssueKeyPatternRFC.FindStringSubmatch(subject); len(matches) >= 2 {
		data.IssueKey = matches[1]
	}

	// Extract title
	data.Title = p.extractLinearTitle(subject, data.IssueKey, data.Team)

	// Extract URL
	if match := pmLinearURLPattern.FindString(combined); match != "" {
		data.URL = match
	}

	// Extract priority
	if matches := linearPriorityPatternRFC.FindStringSubmatch(combined); len(matches) >= 2 {
		data.Extra["tool_priority"] = strings.TrimSpace(matches[1])
	}

	// Extract status
	data.Status = p.ExtractStatus(combined)

	// Extract assignee
	data.Assignee = p.ExtractAssignee(combined)

	// Extract mentions
	data.Mentions = p.ExtractMentions(combined)

	return data
}

// extractLinearTitle extracts clean title from Linear subject.
func (p *LinearParser) extractLinearTitle(subject, issueKey, team string) string {
	title := subject

	// Remove [Team] prefix
	if team != "" {
		title = strings.TrimPrefix(title, "["+team+"] ")
	}
	title = linearTeamPatternRFC.ReplaceAllString(title, "")

	// Remove issue key suffix
	if issueKey != "" {
		title = strings.ReplaceAll(title, "("+issueKey+")", "")
	}

	// Remove common action phrases
	actionPhrases := []string{
		"was assigned to you",
		"mentioned you in",
		"commented on",
		"updated",
		"created",
	}
	for _, phrase := range actionPhrases {
		title = strings.ReplaceAll(title, phrase, "")
	}

	return strings.TrimSpace(title)
}
