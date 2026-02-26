// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/domain"
)

// =============================================================================
// Documentation Base Parser
// =============================================================================
//
// Common patterns for Confluence, Notion, Google Docs:
//   - Page/Document: Created, Edited, Deleted, Shared
//   - Comment: Added, Replied, Resolved
//   - Mention: In page, In comment
//   - Access: Shared with you, Permission changed

// DocumentationBaseParser provides common functionality for documentation tool parsers.
type DocumentationBaseParser struct {
	service  SaaSService
	category SaaSCategory
}

// NewDocumentationBaseParser creates a new base parser.
func NewDocumentationBaseParser(service SaaSService) *DocumentationBaseParser {
	return &DocumentationBaseParser{
		service:  service,
		category: CategoryDocumentation,
	}
}

// Service returns the service.
func (p *DocumentationBaseParser) Service() SaaSService {
	return p.service
}

// Category returns the category.
func (p *DocumentationBaseParser) Category() SaaSCategory {
	return p.category
}

// =============================================================================
// Common Regex Patterns
// =============================================================================

var (
	// Action patterns
	docMentionPattern = regexp.MustCompile(`(?i)mentioned you`)
	docCommentPattern = regexp.MustCompile(`(?i)commented`)
	docRepliedPattern = regexp.MustCompile(`(?i)replied`)
	docSharedPattern  = regexp.MustCompile(`(?i)shared`)
	docEditedPattern  = regexp.MustCompile(`(?i)(?:edited|updated|changed|modified)`)
	docCreatedPattern = regexp.MustCompile(`(?i)created`)
	docDeletedPattern = regexp.MustCompile(`(?i)deleted`)
	docInvitedPattern = regexp.MustCompile(`(?i)invited`)

	// Title extraction (in quotes)
	docTitleQuotedPattern = regexp.MustCompile(`["'"]([^"'"]+)["'"]`)

	// Mention pattern
	docUserMentionPattern = regexp.MustCompile(`@([a-zA-Z0-9._-]+)`)
)

// =============================================================================
// Common Extraction Methods
// =============================================================================

// ExtractQuotedTitle extracts title from quoted text in subject/body.
func (p *DocumentationBaseParser) ExtractQuotedTitle(text string) string {
	if matches := docTitleQuotedPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// ExtractMentions extracts @mentions from text.
func (p *DocumentationBaseParser) ExtractMentions(text string) []string {
	matches := docUserMentionPattern.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var mentions []string

	for _, match := range matches {
		if len(match) >= 2 && !seen[match[1]] {
			seen[match[1]] = true
			mentions = append(mentions, match[1])
		}
	}

	return mentions
}

// ExtractAuthorFromSubject extracts author from "User action" subject pattern.
func (p *DocumentationBaseParser) ExtractAuthorFromSubject(subject string) string {
	// Pattern: "Username commented on..." or "Username mentioned you..."
	// Look for first word before action verbs
	actionVerbs := []string{"commented", "mentioned", "shared", "edited", "updated", "created", "deleted", "invited", "replied"}

	subjectLower := strings.ToLower(subject)
	for _, verb := range actionVerbs {
		if idx := strings.Index(subjectLower, verb); idx > 0 {
			// Get text before the verb
			before := strings.TrimSpace(subject[:idx])
			// Get last word (or the whole thing if no spaces)
			parts := strings.Fields(before)
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	}

	return ""
}

// =============================================================================
// Common Priority Calculation
// =============================================================================

// DocumentationPriorityConfig holds priority calculation parameters.
type DocumentationPriorityConfig struct {
	DomainScore   float64
	EventScore    float64
	RelationScore float64
}

// CalculateDocumentationPriority calculates priority for documentation tools.
func (p *DocumentationBaseParser) CalculateDocumentationPriority(config DocumentationPriorityConfig) (domain.Priority, float64) {
	// Weighted calculation
	score := config.DomainScore*0.3 + config.EventScore*0.5 + config.RelationScore*0.2

	return p.ScoreToPriority(score), score
}

// GetEventScoreForEvent returns event and relation scores for documentation events.
func (p *DocumentationBaseParser) GetEventScoreForEvent(event DocumentationEventType) (eventScore, relationScore float64) {
	switch event {
	// Direct involvement - high priority
	case DocEventMention:
		return 0.8, 0.9 // Mentioned directly
	case DocEventComment:
		return 0.6, 0.7 // Comment on your content
	case DocEventShared:
		return 0.5, 0.8 // Shared with you directly

	// Content changes - medium priority
	case DocEventEdited:
		return 0.4, 0.5 // Page you follow edited
	case DocEventPageCreated:
		return 0.3, 0.4 // New page in workspace

	// Low priority
	case DocEventPageDeleted:
		return 0.2, 0.3

	default:
		return 0.2, 0.3
	}
}

// ScoreToPriority converts score to Priority.
func (p *DocumentationBaseParser) ScoreToPriority(score float64) domain.Priority {
	switch {
	case score >= 0.7:
		return domain.PriorityHigh
	case score >= 0.5:
		return domain.PriorityNormal
	case score >= 0.3:
		return domain.PriorityLow
	default:
		return domain.PriorityLowest
	}
}

// =============================================================================
// Common Category Determination
// =============================================================================

// DetermineDocumentationCategory determines category based on event type.
func (p *DocumentationBaseParser) DetermineDocumentationCategory(event DocumentationEventType) (domain.EmailCategory, *domain.EmailSubCategory) {
	notifSubCat := domain.SubCategoryNotification

	switch event {
	// Direct involvement → Work
	case DocEventMention, DocEventComment, DocEventShared:
		return domain.CategoryWork, nil

	// Content changes → Notification
	case DocEventEdited, DocEventPageCreated, DocEventPageDeleted:
		return domain.CategoryNotification, &notifSubCat

	default:
		return domain.CategoryNotification, &notifSubCat
	}
}

// =============================================================================
// Common Action Item Generation
// =============================================================================

// GenerateDocumentationActionItems generates action items based on event type.
func (p *DocumentationBaseParser) GenerateDocumentationActionItems(event DocumentationEventType, data *ExtractedData) []ActionItem {
	var items []ActionItem

	switch event {
	case DocEventMention:
		items = append(items, ActionItem{
			Type:     ActionRespond,
			Title:    "Respond to mention in " + data.Title,
			URL:      data.URL,
			Priority: "medium",
		})

	case DocEventComment:
		items = append(items, ActionItem{
			Type:     ActionRespond,
			Title:    "Reply to comment on " + data.Title,
			URL:      data.URL,
			Priority: "medium",
		})

	case DocEventShared:
		items = append(items, ActionItem{
			Type:     ActionRead,
			Title:    "Review shared document: " + data.Title,
			URL:      data.URL,
			Priority: "low",
		})

	case DocEventEdited:
		items = append(items, ActionItem{
			Type:     ActionRead,
			Title:    "Review changes to " + data.Title,
			URL:      data.URL,
			Priority: "low",
		})
	}

	return items
}

// =============================================================================
// Common Entity Extraction
// =============================================================================

// GenerateDocumentationEntities generates entities from extracted data.
func (p *DocumentationBaseParser) GenerateDocumentationEntities(data *ExtractedData) []Entity {
	var entities []Entity

	// Document/Page entity
	if data.Title != "" {
		pageID := ""
		if id, ok := data.Extra["page_id"].(string); ok {
			pageID = id
		}
		entities = append(entities, Entity{
			Type: EntityDocument,
			ID:   pageID,
			Name: data.Title,
			URL:  data.URL,
		})
	}

	// Workspace/Space entity
	if data.Workspace != "" {
		entities = append(entities, Entity{
			Type: EntityWorkspace,
			ID:   data.Workspace,
			Name: data.Workspace,
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
