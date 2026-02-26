// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/port/out"
)

// =============================================================================
// Notion Parser
// =============================================================================
//
// Notion email patterns:
//   - From: notify@mail.notion.so (primary)
//   - From: no-reply@mail.notion.so
//   - From: team@makenotion.com (support/account)
//
// Subject patterns (no prefix, direct format):
//   - "{User} mentioned you in "{Page Title}""
//   - "{User} commented on "{Page Title}""
//   - "{User} replied to your comment in "{Page Title}""
//   - "{User} shared "{Page Title}" with you"
//   - "Reminder: {Page Title}"
//   - "{User} assigned you to "{Page Title}""
//   - "{User} added you to "{Page Title}""
//   - "{User} invited you to {Workspace Name}"
//   - "Your weekly update from {Workspace Name}"
//
// URL patterns:
//   - https://www.notion.so/{Page-Title-With-Dashes}-{32characterPageId}
//   - https://www.notion.so/{WorkspaceName}/{Page-Title}-{pageId}
//
// NOTE: Email only sent if user inactive in app for 5+ minutes
// (unless "Always send email notifications" enabled)

// NotionParser parses Notion notification emails.
type NotionParser struct {
	*DocumentationBaseParser
}

// NewNotionParser creates a new Notion parser.
func NewNotionParser() *NotionParser {
	return &NotionParser{
		DocumentationBaseParser: NewDocumentationBaseParser(ServiceNotion),
	}
}

// Notion-specific regex patterns
var (
	// URL pattern - extracts 32-character page ID (UUID without hyphens)
	notionPageURLPattern = regexp.MustCompile(`https://(?:www\.)?notion\.so/(?:([^/]+)/)?(?:[^/]+-)?([a-f0-9]{32})`)

	// Subject patterns
	notionMentionPattern  = regexp.MustCompile(`(?i)(.+)\s+mentioned you in\s+["'"]([^"'"]+)["'"]`)
	notionCommentPattern  = regexp.MustCompile(`(?i)(.+)\s+commented on\s+["'"]([^"'"]+)["'"]`)
	notionRepliedPattern  = regexp.MustCompile(`(?i)(.+)\s+replied to your comment in\s+["'"]([^"'"]+)["'"]`)
	notionSharedPattern   = regexp.MustCompile(`(?i)(.+)\s+shared\s+["'"]([^"'"]+)["'"]\s+with you`)
	notionReminderPattern = regexp.MustCompile(`(?i)Reminder:\s+(.+)`)
	notionAssignedPattern = regexp.MustCompile(`(?i)(.+)\s+assigned you to\s+["'"]([^"'"]+)["'"]`)
	notionAddedPattern    = regexp.MustCompile(`(?i)(.+)\s+added you to\s+["'"]([^"'"]+)["'"]`)
	notionInvitedPattern  = regexp.MustCompile(`(?i)(.+)\s+invited you to\s+(.+)`)
	notionWeeklyPattern   = regexp.MustCompile(`(?i)Your weekly update from\s+(.+)`)
	notionUpdatesPattern  = regexp.MustCompile(`(?i)Updates in\s+["'"]([^"'"]+)["'"]`)
)

// CanParse checks if this parser can handle the email.
func (p *NotionParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)

	// Check Notion domains
	return strings.Contains(fromLower, "@notion.so") ||
		strings.Contains(fromLower, "@mail.notion.so") ||
		strings.Contains(fromLower, "@makenotion.com")
}

// Parse extracts structured data from Notion emails.
func (p *NotionParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	// Detect event from content
	event := p.detectEvent(input)

	// Extract data
	data := p.extractData(input)

	// Calculate priority
	eventScore, relationScore := p.GetEventScoreForEvent(event)
	priority, score := p.CalculateDocumentationPriority(DocumentationPriorityConfig{
		DomainScore:   0.22, // Notion is popular for personal/team docs
		EventScore:    eventScore,
		RelationScore: relationScore,
	})

	// Determine category
	category, subCat := p.DetermineDocumentationCategory(event)

	// Generate action items
	actionItems := p.GenerateDocumentationActionItems(event, data)

	// Generate entities
	entities := p.GenerateDocumentationEntities(data)

	return &ParsedEmail{
		Category:      CategoryDocumentation,
		Service:       ServiceNotion,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:notion:" + string(event),
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"notion", "event:" + string(event)},
	}, nil
}

// detectEvent detects the Notion event from content.
func (p *NotionParser) detectEvent(input *ParserInput) DocumentationEventType {
	subject := ""
	if input.Message != nil {
		subject = input.Message.Subject
	}

	switch {
	case notionMentionPattern.MatchString(subject):
		return DocEventMention
	case notionCommentPattern.MatchString(subject), notionRepliedPattern.MatchString(subject):
		return DocEventComment
	case notionSharedPattern.MatchString(subject):
		return DocEventShared
	case notionAssignedPattern.MatchString(subject), notionAddedPattern.MatchString(subject):
		return DocEventMention // Assignment is like a mention
	case notionReminderPattern.MatchString(subject):
		return DocEventMention // Reminder needs action
	case notionInvitedPattern.MatchString(subject):
		return DocEventShared // Workspace invite
	case notionWeeklyPattern.MatchString(subject):
		return DocEventEdited // Digest/summary
	case notionUpdatesPattern.MatchString(subject):
		return DocEventEdited
	}

	return DocEventEdited
}

// extractData extracts structured data from the email.
func (p *NotionParser) extractData(input *ParserInput) *ExtractedData {
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

	// Extract from URL
	p.extractFromURL(combined, data)

	// Extract from subject patterns
	p.extractFromSubject(subject, data)

	// Extract mentions
	data.Mentions = p.ExtractMentions(combined)

	return data
}

// extractFromURL extracts workspace and page info from Notion URLs.
func (p *NotionParser) extractFromURL(text string, data *ExtractedData) {
	if matches := notionPageURLPattern.FindStringSubmatch(text); len(matches) >= 3 {
		data.URL = matches[0]
		if matches[1] != "" {
			data.Workspace = matches[1]
		}
		data.Extra["page_id"] = matches[2]
	}
}

// extractFromSubject extracts author, title, workspace from subject patterns.
func (p *NotionParser) extractFromSubject(subject string, data *ExtractedData) {
	// Try mention pattern: "User mentioned you in "Page""
	if matches := notionMentionPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		if data.Title == "" {
			data.Title = strings.TrimSpace(matches[2])
		}
		return
	}

	// Try comment pattern: "User commented on "Page""
	if matches := notionCommentPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		if data.Title == "" {
			data.Title = strings.TrimSpace(matches[2])
		}
		return
	}

	// Try replied pattern: "User replied to your comment in "Page""
	if matches := notionRepliedPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		if data.Title == "" {
			data.Title = strings.TrimSpace(matches[2])
		}
		return
	}

	// Try shared pattern: "User shared "Page" with you"
	if matches := notionSharedPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		if data.Title == "" {
			data.Title = strings.TrimSpace(matches[2])
		}
		return
	}

	// Try assigned pattern: "User assigned you to "Page""
	if matches := notionAssignedPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		if data.Title == "" {
			data.Title = strings.TrimSpace(matches[2])
		}
		return
	}

	// Try added pattern: "User added you to "Page""
	if matches := notionAddedPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		if data.Title == "" {
			data.Title = strings.TrimSpace(matches[2])
		}
		return
	}

	// Try reminder pattern: "Reminder: Page"
	if matches := notionReminderPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		data.Title = strings.TrimSpace(matches[1])
		return
	}

	// Try invited pattern: "User invited you to Workspace"
	if matches := notionInvitedPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		if data.Workspace == "" {
			data.Workspace = strings.TrimSpace(matches[2])
		}
		return
	}

	// Try weekly update pattern: "Your weekly update from Workspace"
	if matches := notionWeeklyPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		if data.Workspace == "" {
			data.Workspace = strings.TrimSpace(matches[1])
		}
		data.Title = subject
		return
	}

	// Try updates pattern: "Updates in "Page""
	if matches := notionUpdatesPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		if data.Title == "" {
			data.Title = strings.TrimSpace(matches[1])
		}
		return
	}

	// Fallback: extract quoted title
	if data.Title == "" {
		data.Title = p.ExtractQuotedTitle(subject)
	}

	// Fallback: use subject as title
	if data.Title == "" {
		data.Title = subject
	}

	// Extract author
	if data.Author == "" {
		data.Author = p.ExtractAuthorFromSubject(subject)
	}
}
