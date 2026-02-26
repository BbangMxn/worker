// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/port/out"
)

// =============================================================================
// Asana Parser
// =============================================================================
//
// Asana email patterns:
//   - From: notifications@asana.com or mail.asana.com
//   - Subject: "Assigned to You: Task Name" (note: capitalized "You")
//   - Subject: "You were mentioned in Task Name"
//   - Subject: "Comment on Task Name"

// AsanaParser parses Asana notification emails.
type AsanaParser struct {
	*ProjectMgmtBaseParser
}

// NewAsanaParser creates a new Asana parser.
func NewAsanaParser() *AsanaParser {
	return &AsanaParser{
		ProjectMgmtBaseParser: NewProjectMgmtBaseParser(ServiceAsana),
	}
}

// CanParse checks if this parser can handle the email.
func (p *AsanaParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)
	return strings.Contains(fromLower, "asana.com") ||
		strings.Contains(fromLower, "mail.asana.com")
}

// Parse extracts structured data from Asana emails.
func (p *AsanaParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	// Detect action from content
	action := p.detectAction(input)

	// Determine event type
	event := p.determineEvent(action)

	// Extract data
	data := p.extractData(input)

	// Calculate priority
	actionScore, relationScore := p.GetActionScoreForEvent(event)
	priority, score := p.CalculateProjectMgmtPriority(ProjectMgmtPriorityConfig{
		DomainScore:   0.18, // Similar to Jira/Linear
		ActionScore:   actionScore,
		RelationScore: relationScore,
	})

	// Determine category
	category, subCat := p.DetermineProjectMgmtCategory(event)

	// Generate action items
	actionItems := p.GenerateProjectMgmtActionItems(event, data)

	// Generate entities
	entities := p.GenerateProjectMgmtEntities(data)

	return &ParsedEmail{
		Category:      CategoryProjectMgmt,
		Service:       ServiceAsana,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:asana:" + action,
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"asana", "action:" + action},
	}, nil
}

// detectAction detects the Asana action from content.
func (p *AsanaParser) detectAction(input *ParserInput) string {
	subject := ""
	if input.Message != nil {
		subject = strings.ToLower(input.Message.Subject)
	}

	bodyText := ""
	if input.Body != nil {
		bodyText = strings.ToLower(input.Body.Text)
	}

	switch {
	case strings.Contains(subject, "assigned to you") || strings.Contains(subject, "assigned to you:"):
		return "assigned"
	case strings.Contains(subject, "mentioned") || strings.Contains(bodyText, "mentioned you"):
		return "mentioned"
	case strings.Contains(subject, "comment"):
		return "commented"
	case strings.Contains(subject, "created") || strings.Contains(subject, "new task"):
		return "created"
	case strings.Contains(subject, "completed") || strings.Contains(subject, "marked complete"):
		return "completed"
	case strings.Contains(subject, "due") || strings.Contains(subject, "deadline"):
		return "due"
	case strings.Contains(subject, "updated") || strings.Contains(bodyText, "made changes"):
		return "updated"
	}

	return "updated"
}

// determineEvent determines the event type from action.
func (p *AsanaParser) determineEvent(action string) ProjectMgmtEventType {
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
	case "due":
		return PMEventIssueDue
	default:
		return PMEventIssueUpdated
	}
}

// Asana-specific regex patterns
var (
	asanaTaskPatternRFC    = regexp.MustCompile(`(?i)(?:task|project)[:\s]*["']?([^"'\n]+)["']?`)
	asanaProjectPatternRFC = regexp.MustCompile(`(?i)in\s+project[:\s]*["']?([^"'\n]+)["']?`)
)

// extractData extracts structured data from the email.
func (p *AsanaParser) extractData(input *ParserInput) *ExtractedData {
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

	// Extract title from subject
	data.Title = p.extractAsanaTitle(subject)

	// Extract URL
	if match := pmAsanaURLPattern.FindString(combined); match != "" {
		data.URL = match
	}

	// Extract project
	if matches := asanaProjectPatternRFC.FindStringSubmatch(combined); len(matches) >= 2 {
		data.Project = strings.TrimSpace(matches[1])
	}

	// Extract assignee
	data.Assignee = p.ExtractAssignee(combined)

	// Extract due date
	data.DueDate = p.ExtractDueDate(combined)

	// Extract mentions
	data.Mentions = p.ExtractMentions(combined)

	return data
}

// extractAsanaTitle extracts clean title from Asana subject.
func (p *AsanaParser) extractAsanaTitle(subject string) string {
	title := subject

	// Remove common prefixes (Asana uses "Assigned to You:" with capital Y)
	prefixes := []string{
		"Assigned to You: ",
		"assigned to you: ",
		"Task assigned to you: ",
		"You were mentioned in ",
		"Comment on ",
		"New task: ",
		"Task completed: ",
		"Re: ",
	}
	for _, prefix := range prefixes {
		title = strings.TrimPrefix(title, prefix)
	}

	return strings.TrimSpace(title)
}
