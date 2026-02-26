// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/port/out"
)

// =============================================================================
// Confluence Parser
// =============================================================================
//
// Confluence email patterns:
//   - From: confluence@{instance}.atlassian.net (Cloud)
//   - From: noreply@am.atlassian.com (Atlassian shared services)
//   - Headers: X-JIRA-FingerPrint (Atlassian ecosystem)
//
// Subject patterns (default prefix: [Confluence]):
//   - "[Confluence] {User} created "{Page Title}" in {Space Name}"
//   - "[Confluence] {User} edited "{Page Title}""
//   - "[Confluence] {User} commented on "{Page Title}""
//   - "[Confluence] {User} mentioned you in "{Page Title}""
//   - "[Confluence] {User} shared "{Page Title}" with you"
//   - "[Confluence] Daily Update"
//
// URL patterns:
//   - https://{instance}.atlassian.net/wiki/spaces/{SPACE_KEY}/pages/{pageId}/{Title}
//   - https://{instance}.atlassian.net/wiki/x/{base64EncodedId} (tiny link)

// ConfluenceParser parses Confluence notification emails.
type ConfluenceParser struct {
	*DocumentationBaseParser
}

// NewConfluenceParser creates a new Confluence parser.
func NewConfluenceParser() *ConfluenceParser {
	return &ConfluenceParser{
		DocumentationBaseParser: NewDocumentationBaseParser(ServiceConfluence),
	}
}

// Confluence-specific regex patterns
var (
	// URL patterns
	confluencePageURLPattern = regexp.MustCompile(`https://([^/]+)\.atlassian\.net/wiki/spaces/([A-Z0-9]+)/pages/(\d+)(?:/([^\s"<>]+))?`)
	confluenceTinyURLPattern = regexp.MustCompile(`https://([^/]+)\.atlassian\.net/wiki/x/([a-zA-Z0-9]+)`)

	// Subject patterns with [Confluence] prefix
	confluencePrefixPattern  = regexp.MustCompile(`^\[Confluence\]\s*`)
	confluenceCreatedPattern = regexp.MustCompile(`(?i)(.+)\s+created\s+["'"]([^"'"]+)["'"]\s+in\s+(.+)`)
	confluenceEditedPattern  = regexp.MustCompile(`(?i)(.+)\s+edited\s+["'"]([^"'"]+)["'"]`)
	confluenceCommentPattern = regexp.MustCompile(`(?i)(.+)\s+commented on\s+["'"]([^"'"]+)["'"]`)
	confluenceMentionPattern = regexp.MustCompile(`(?i)(.+)\s+mentioned you in\s+["'"]([^"'"]+)["'"]`)
	confluenceSharedPattern  = regexp.MustCompile(`(?i)(.+)\s+shared\s+["'"]([^"'"]+)["'"]\s+with you`)
	confluenceTaskPattern    = regexp.MustCompile(`(?i)(.+)\s+assigned you a task in\s+["'"]([^"'"]+)["'"]`)
	confluenceDigestPattern  = regexp.MustCompile(`(?i)daily\s+update|recommended\s+updates`)
)

// CanParse checks if this parser can handle the email.
func (p *ConfluenceParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)

	// Check Confluence-specific from addresses
	if strings.Contains(fromLower, "confluence@") && strings.Contains(fromLower, ".atlassian.net") {
		return true
	}

	// Check for Atlassian shared services (need to verify it's Confluence)
	if strings.Contains(fromLower, "@atlassian.net") || strings.Contains(fromLower, "@am.atlassian.com") {
		// Check if subject has [Confluence] prefix
		if rawHeaders != nil {
			subject := rawHeaders["Subject"]
			if strings.HasPrefix(subject, "[Confluence]") {
				return true
			}
		}
	}

	// Check X-JIRA-FingerPrint header (Atlassian ecosystem)
	if rawHeaders != nil {
		if rawHeaders["X-JIRA-FingerPrint"] != "" {
			// Need subject check to distinguish from Jira
			subject := rawHeaders["Subject"]
			if strings.HasPrefix(subject, "[Confluence]") {
				return true
			}
		}
	}

	return false
}

// Parse extracts structured data from Confluence emails.
func (p *ConfluenceParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	// Detect event from content
	event := p.detectEvent(input)

	// Extract data
	data := p.extractData(input)

	// Calculate priority
	eventScore, relationScore := p.GetEventScoreForEvent(event)
	priority, score := p.CalculateDocumentationPriority(DocumentationPriorityConfig{
		DomainScore:   0.25, // Confluence is important for enterprise
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
		Service:       ServiceConfluence,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:confluence:" + string(event),
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"confluence", "event:" + string(event)},
	}, nil
}

// detectEvent detects the Confluence event from content.
func (p *ConfluenceParser) detectEvent(input *ParserInput) DocumentationEventType {
	subject := ""
	if input.Message != nil {
		subject = input.Message.Subject
	}

	// Remove [Confluence] prefix for easier matching
	subject = confluencePrefixPattern.ReplaceAllString(subject, "")

	switch {
	case confluenceMentionPattern.MatchString(subject):
		return DocEventMention
	case confluenceCommentPattern.MatchString(subject):
		return DocEventComment
	case confluenceSharedPattern.MatchString(subject):
		return DocEventShared
	case confluenceTaskPattern.MatchString(subject):
		return DocEventMention // Task assignment is like a mention
	case confluenceEditedPattern.MatchString(subject):
		return DocEventEdited
	case confluenceCreatedPattern.MatchString(subject):
		return DocEventPageCreated
	case confluenceDigestPattern.MatchString(subject):
		return DocEventEdited // Digest is low priority
	}

	// Check for deleted
	if strings.Contains(strings.ToLower(subject), "deleted") {
		return DocEventPageDeleted
	}

	return DocEventEdited
}

// extractData extracts structured data from the email.
func (p *ConfluenceParser) extractData(input *ParserInput) *ExtractedData {
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

	// Remove [Confluence] prefix
	cleanSubject := confluencePrefixPattern.ReplaceAllString(subject, "")

	// Extract from URL
	p.extractFromURL(combined, data)

	// Extract from subject patterns
	p.extractFromSubject(cleanSubject, data)

	// Extract mentions
	data.Mentions = p.ExtractMentions(combined)

	// Store Atlassian fingerprint if present
	if input.RawHeaders != nil {
		if fp := input.RawHeaders["X-JIRA-FingerPrint"]; fp != "" {
			data.Extra["atlassian_fingerprint"] = fp
		}
	}

	return data
}

// extractFromURL extracts instance, space, and page info from Confluence URLs.
func (p *ConfluenceParser) extractFromURL(text string, data *ExtractedData) {
	// Try standard page URL
	if matches := confluencePageURLPattern.FindStringSubmatch(text); len(matches) >= 4 {
		data.URL = matches[0]
		data.Extra["instance"] = matches[1]
		data.Workspace = matches[2] // Space key
		data.Extra["page_id"] = matches[3]
		if len(matches) >= 5 && matches[4] != "" {
			// URL-encoded title
			data.Title = p.cleanURLSlug(matches[4])
		}
		return
	}

	// Try tiny URL
	if matches := confluenceTinyURLPattern.FindStringSubmatch(text); len(matches) >= 3 {
		data.URL = matches[0]
		data.Extra["instance"] = matches[1]
		data.Extra["tiny_link_id"] = matches[2]
	}
}

// extractFromSubject extracts author, title, space from subject patterns.
func (p *ConfluenceParser) extractFromSubject(subject string, data *ExtractedData) {
	// Try created pattern: "User created "Page" in Space"
	if matches := confluenceCreatedPattern.FindStringSubmatch(subject); len(matches) >= 4 {
		data.Author = strings.TrimSpace(matches[1])
		if data.Title == "" {
			data.Title = strings.TrimSpace(matches[2])
		}
		if data.Workspace == "" {
			data.Workspace = strings.TrimSpace(matches[3])
		}
		return
	}

	// Try mention pattern: "User mentioned you in "Page""
	if matches := confluenceMentionPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		if data.Title == "" {
			data.Title = strings.TrimSpace(matches[2])
		}
		return
	}

	// Try comment pattern: "User commented on "Page""
	if matches := confluenceCommentPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		if data.Title == "" {
			data.Title = strings.TrimSpace(matches[2])
		}
		return
	}

	// Try shared pattern: "User shared "Page" with you"
	if matches := confluenceSharedPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		if data.Title == "" {
			data.Title = strings.TrimSpace(matches[2])
		}
		return
	}

	// Try edited pattern: "User edited "Page""
	if matches := confluenceEditedPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		if data.Title == "" {
			data.Title = strings.TrimSpace(matches[2])
		}
		return
	}

	// Try task pattern: "User assigned you a task in "Page""
	if matches := confluenceTaskPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		if data.Title == "" {
			data.Title = strings.TrimSpace(matches[2])
		}
		return
	}

	// Fallback: extract quoted title
	if data.Title == "" {
		data.Title = p.ExtractQuotedTitle(subject)
	}

	// Extract author
	if data.Author == "" {
		data.Author = p.ExtractAuthorFromSubject(subject)
	}
}

// cleanURLSlug converts URL-encoded slug to readable title.
func (p *ConfluenceParser) cleanURLSlug(slug string) string {
	slug = strings.ReplaceAll(slug, "+", " ")
	slug = strings.ReplaceAll(slug, "%20", " ")
	slug = strings.ReplaceAll(slug, "-", " ")
	return strings.TrimSpace(slug)
}
