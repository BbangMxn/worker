// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"strings"

	"worker_server/core/port/out"
	"worker_server/core/service/classification"
)

// =============================================================================
// Jira Parser
// =============================================================================
//
// Jira-specific headers:
//   - X-JIRA-FingerPrint: Unique identifier for Jira instance
//   - X-Atlassian-Instance: Instance URL
//   - X-Atlassian-MailAction: Action type (IssueCreated, IssueUpdated, etc.)
//   - X-Atlassian-User: User who triggered the action
//
// Subject patterns:
//   - [JIRA] (PROJECT-123) Issue Summary
//   - [PROJECT-123] Issue Summary

// JiraParser parses Jira notification emails.
type JiraParser struct {
	*ProjectMgmtBaseParser
}

// NewJiraParser creates a new Jira parser.
func NewJiraParser() *JiraParser {
	return &JiraParser{
		ProjectMgmtBaseParser: NewProjectMgmtBaseParser(ServiceJira),
	}
}

// CanParse checks if this parser can handle the email.
func (p *JiraParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	// Check Jira-specific header
	if headers != nil && headers.XJIRAFingerprint != "" {
		return true
	}

	// Check raw headers for Atlassian
	if rawHeaders != nil {
		if rawHeaders["X-Atlassian-MailAction"] != "" ||
			rawHeaders["X-Atlassian-Instance"] != "" {
			return true
		}
	}

	// Check from email
	fromLower := strings.ToLower(fromEmail)
	return strings.Contains(fromLower, "jira") ||
		strings.Contains(fromLower, "atlassian")
}

// Parse extracts structured data from Jira emails.
func (p *JiraParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	// Detect action from headers or content
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
		DomainScore:   classification.DomainScoreJira,
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
		Service:       ServiceJira,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:jira:" + action,
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"jira", "action:" + action},
	}, nil
}

// detectAction detects the Jira action from headers or content.
func (p *JiraParser) detectAction(input *ParserInput) string {
	// Check X-Atlassian-MailAction header
	if input.RawHeaders != nil {
		if action := input.RawHeaders["X-Atlassian-MailAction"]; action != "" {
			return strings.ToLower(action)
		}
	}

	// Detect from subject/body
	subject := ""
	if input.Message != nil {
		subject = strings.ToLower(input.Message.Subject)
	}

	bodyText := ""
	if input.Body != nil {
		bodyText = strings.ToLower(input.Body.Text)
	}

	switch {
	case strings.Contains(subject, "created") || strings.Contains(bodyText, "created this issue"):
		return "issuecreated"
	case strings.Contains(subject, "assigned") || strings.Contains(bodyText, "assigned to you"):
		return "issueassigned"
	case strings.Contains(subject, "commented") || strings.Contains(bodyText, "added a comment"):
		return "issuecommented"
	case strings.Contains(subject, "updated") || strings.Contains(bodyText, "made changes"):
		return "issueupdated"
	case strings.Contains(subject, "resolved") || strings.Contains(bodyText, "resolved"):
		return "issueresolved"
	case strings.Contains(subject, "closed") || strings.Contains(bodyText, "closed"):
		return "issueclosed"
	case strings.Contains(subject, "reopened") || strings.Contains(bodyText, "reopened"):
		return "issuereopened"
	case strings.Contains(subject, "mentioned") || strings.Contains(bodyText, "mentioned you"):
		return "issuementioned"
	case strings.Contains(subject, "sprint") && strings.Contains(subject, "started"):
		return "sprintstarted"
	case strings.Contains(subject, "sprint") && (strings.Contains(subject, "completed") || strings.Contains(subject, "ended")):
		return "sprintcompleted"
	}

	return "issueupdated"
}

// determineEvent determines the event type from action.
func (p *JiraParser) determineEvent(action string) ProjectMgmtEventType {
	switch action {
	case "issuecreated":
		return PMEventIssueCreated
	case "issueassigned":
		return PMEventIssueAssigned
	case "issuementioned":
		return PMEventIssueMentioned
	case "issueupdated":
		return PMEventIssueUpdated
	case "issuecommented":
		return PMEventIssueComment
	case "issueresolved", "issueclosed":
		return PMEventIssueClosed
	case "issuereopened":
		return PMEventIssueReopened
	case "sprintstarted":
		return PMEventSprintStarted
	case "sprintcompleted":
		return PMEventSprintEnded
	default:
		return PMEventIssueUpdated
	}
}

// extractData extracts structured data from the email.
func (p *JiraParser) extractData(input *ParserInput) *ExtractedData {
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

	// Extract issue key
	data.IssueKey = p.ExtractIssueKey(subject)
	if data.IssueKey == "" {
		data.IssueKey = p.ExtractIssueKey(bodyText)
	}

	// Extract project from key
	data.Project = p.ExtractProjectFromKey(data.IssueKey)

	// Extract title
	data.Title = p.ExtractTitle(subject, data.IssueKey)

	// Extract URL
	if matches := pmJiraURLPattern.FindStringSubmatch(combined); len(matches) >= 1 {
		data.URL = matches[0]
	}

	// Extract assignee
	data.Assignee = p.ExtractAssignee(combined)

	// Extract reporter/author
	data.Author = p.ExtractReporter(combined)

	// Extract priority
	if priority := p.ExtractPriority(combined); priority != "" {
		data.Extra["tool_priority"] = priority
	}

	// Extract status
	data.Status = p.ExtractStatus(combined)

	// Extract due date
	data.DueDate = p.ExtractDueDate(combined)

	// Extract sprint
	if matches := jiraSprintPatternRFC.FindStringSubmatch(combined); len(matches) >= 2 {
		data.Sprint = strings.TrimSpace(matches[1])
	}

	// Extract from Atlassian headers
	if input.RawHeaders != nil {
		if user := input.RawHeaders["X-Atlassian-User"]; user != "" {
			data.Author = user
		}
	}

	return data
}

var jiraSprintPatternRFC = pmStatusPattern // Reuse for now, could be more specific
