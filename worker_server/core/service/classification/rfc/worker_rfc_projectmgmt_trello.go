// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/port/out"
)

// =============================================================================
// Trello Parser
// =============================================================================
//
// Trello email patterns:
//   - From: do-not-reply@trello.com, invitation-do-not-reply@trello.com
//   - From: noreply@id.atlassian.com (Atlassian ID related)
//   - From: *-noreply@trellobutler.com (Butler automation)
//   - Reply-To: user+token@boards.trello.com (for card comments)
//
// Subject patterns:
//   - "[User] added you to [Card Name] on [Board Name]"
//   - "[User] mentioned you on [Card Name]"
//   - "[User] commented on [Card Name]"
//   - "[Card Name] is due in 24 hours"
//   - "[User] invited you to join [Board Name]"
//
// NOTE: Trello does NOT use custom X-Trello-* headers.
// Card/Board identification relies on URL extraction from body.

// TrelloParser parses Trello notification emails.
type TrelloParser struct {
	*ProjectMgmtBaseParser
}

// NewTrelloParser creates a new Trello parser.
func NewTrelloParser() *TrelloParser {
	return &TrelloParser{
		ProjectMgmtBaseParser: NewProjectMgmtBaseParser(ServiceTrello),
	}
}

// Trello-specific regex patterns
var (
	// URL patterns for card/board extraction
	trelloBoardURLPattern = regexp.MustCompile(`https?://trello\.com/b/([a-zA-Z0-9]+)(?:/([^\s"<>]+))?`)
	trelloCardURLPattern  = regexp.MustCompile(`https?://trello\.com/c/([a-zA-Z0-9]+)(?:/(\d+)-([^\s"<>]+))?`)

	// Subject patterns
	trelloAddedPattern     = regexp.MustCompile(`(?i)(.+)\s+added you to\s+(.+?)\s+on\s+(.+)`)
	trelloMentionedPattern = regexp.MustCompile(`(?i)(.+)\s+mentioned you (?:on|in)\s+(.+)`)
	trelloCommentedPattern = regexp.MustCompile(`(?i)(.+)\s+commented on\s+(.+)`)
	trelloDuePattern       = regexp.MustCompile(`(?i)(.+)\s+is due (?:in|on|tomorrow|today)`)
	trelloInvitedPattern   = regexp.MustCompile(`(?i)(.+)\s+invited you to (?:join\s+)?(.+)`)
	trelloCompletedPattern = regexp.MustCompile(`(?i)(.+)\s+(?:marked|completed|checked)`)
	trelloMovedPattern     = regexp.MustCompile(`(?i)(.+)\s+(?:was )?moved to\s+(.+)`)
	trelloAssignedPattern  = regexp.MustCompile(`(?i)(.+)\s+assigned you`)
)

// CanParse checks if this parser can handle the email.
func (p *TrelloParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)

	// Check known Trello domains
	if strings.Contains(fromLower, "@trello.com") ||
		strings.Contains(fromLower, "@trellobutler.com") {
		return true
	}

	// Check Atlassian ID (used for Trello account emails)
	if strings.Contains(fromLower, "@id.atlassian.com") {
		// Need to check if it's Trello-related (could be Jira/Confluence too)
		// For now, we'll let the subject/body determine this
		return false // Let other Atlassian parsers handle this
	}

	// Check Reply-To for boards.trello.com
	if rawHeaders != nil {
		replyTo := strings.ToLower(rawHeaders["Reply-To"])
		if strings.Contains(replyTo, "@boards.trello.com") {
			return true
		}
	}

	return false
}

// Parse extracts structured data from Trello emails.
func (p *TrelloParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	// Detect action from content
	action := p.detectAction(input)

	// Determine event type
	event := p.determineEvent(action)

	// Extract data
	data := p.extractData(input)

	// Calculate priority
	actionScore, relationScore := p.GetActionScoreForEvent(event)
	priority, score := p.CalculateProjectMgmtPriority(ProjectMgmtPriorityConfig{
		DomainScore:   0.15, // Trello is less critical than Jira/Linear
		ActionScore:   actionScore,
		RelationScore: relationScore,
	})

	// Determine category
	category, subCat := p.DetermineProjectMgmtCategory(event)

	// Generate action items
	actionItems := p.GenerateProjectMgmtActionItems(event, data)

	// Generate entities
	entities := p.generateTrelloEntities(data)

	return &ParsedEmail{
		Category:      CategoryProjectMgmt,
		Service:       ServiceTrello,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:trello:" + action,
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"trello", "action:" + action},
	}, nil
}

// detectAction detects the Trello action from content.
func (p *TrelloParser) detectAction(input *ParserInput) string {
	subject := ""
	if input.Message != nil {
		subject = input.Message.Subject
	}

	subjectLower := strings.ToLower(subject)

	switch {
	case trelloAddedPattern.MatchString(subject) || trelloAssignedPattern.MatchString(subject):
		return "assigned"
	case trelloMentionedPattern.MatchString(subject):
		return "mentioned"
	case trelloCommentedPattern.MatchString(subject):
		return "commented"
	case trelloDuePattern.MatchString(subject):
		return "due"
	case trelloInvitedPattern.MatchString(subject):
		return "invited"
	case trelloCompletedPattern.MatchString(subject):
		return "completed"
	case trelloMovedPattern.MatchString(subject):
		return "moved"
	case strings.Contains(subjectLower, "created"):
		return "created"
	case strings.Contains(subjectLower, "archived"):
		return "archived"
	}

	return "updated"
}

// determineEvent determines the event type from action.
func (p *TrelloParser) determineEvent(action string) ProjectMgmtEventType {
	switch action {
	case "assigned":
		return PMEventIssueAssigned
	case "mentioned":
		return PMEventIssueMentioned
	case "commented":
		return PMEventIssueComment
	case "due":
		return PMEventIssueDue
	case "created":
		return PMEventIssueCreated
	case "completed", "archived":
		return PMEventIssueClosed
	case "moved":
		return PMEventStatusChanged
	case "invited":
		return PMEventIssueUpdated
	default:
		return PMEventIssueUpdated
	}
}

// extractData extracts structured data from the email.
func (p *TrelloParser) extractData(input *ParserInput) *ExtractedData {
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

	// Extract card URL and IDs
	if matches := trelloCardURLPattern.FindStringSubmatch(combined); len(matches) >= 2 {
		data.URL = matches[0]
		data.Extra["card_short_link"] = matches[1]
		if len(matches) >= 3 && matches[2] != "" {
			data.Extra["card_id_short"] = matches[2]
		}
	}

	// Extract board URL
	if matches := trelloBoardURLPattern.FindStringSubmatch(combined); len(matches) >= 2 {
		data.Extra["board_short_link"] = matches[1]
		if len(matches) >= 3 && matches[2] != "" {
			data.Project = p.cleanURLSlug(matches[2])
		}
	}

	// Extract from subject patterns
	p.extractFromSubject(subject, data)

	// Extract due date
	data.DueDate = p.ExtractDueDate(combined)

	// Extract mentions
	data.Mentions = p.ExtractMentions(combined)

	return data
}

// extractFromSubject extracts title, author, and board from subject patterns.
func (p *TrelloParser) extractFromSubject(subject string, data *ExtractedData) {
	// Try "added you to [Card] on [Board]" pattern
	if matches := trelloAddedPattern.FindStringSubmatch(subject); len(matches) >= 4 {
		data.Assignee = strings.TrimSpace(matches[1])
		data.Title = strings.TrimSpace(matches[2])
		if data.Project == "" {
			data.Project = strings.TrimSpace(matches[3])
		}
		return
	}

	// Try "mentioned you on [Card]" pattern
	if matches := trelloMentionedPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		data.Title = strings.TrimSpace(matches[2])
		return
	}

	// Try "commented on [Card]" pattern
	if matches := trelloCommentedPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		data.Title = strings.TrimSpace(matches[2])
		return
	}

	// Try "[Card] is due" pattern
	if matches := trelloDuePattern.FindStringSubmatch(subject); len(matches) >= 2 {
		data.Title = strings.TrimSpace(matches[1])
		return
	}

	// Try "invited you to [Board]" pattern
	if matches := trelloInvitedPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		data.Project = strings.TrimSpace(matches[2])
		return
	}

	// Fallback: use subject as title
	data.Title = subject
}

// cleanURLSlug removes URL encoding and converts to readable format.
func (p *TrelloParser) cleanURLSlug(slug string) string {
	// Replace URL-encoded characters and dashes with spaces
	slug = strings.ReplaceAll(slug, "-", " ")
	slug = strings.ReplaceAll(slug, "%20", " ")
	return strings.TrimSpace(slug)
}

// generateTrelloEntities generates entities from extracted data.
func (p *TrelloParser) generateTrelloEntities(data *ExtractedData) []Entity {
	var entities []Entity

	// Card entity
	if cardLink, ok := data.Extra["card_short_link"].(string); ok && cardLink != "" {
		entities = append(entities, Entity{
			Type: EntityIssue,
			ID:   cardLink,
			Name: data.Title,
			URL:  "https://trello.com/c/" + cardLink,
		})
	}

	// Board entity
	if boardLink, ok := data.Extra["board_short_link"].(string); ok && boardLink != "" {
		entities = append(entities, Entity{
			Type: EntityProject,
			ID:   boardLink,
			Name: data.Project,
			URL:  "https://trello.com/b/" + boardLink,
		})
	}

	// Author entity
	if data.Author != "" {
		entities = append(entities, Entity{
			Type: EntityUser,
			ID:   data.Author,
			Name: data.Author,
		})
	}

	return entities
}
