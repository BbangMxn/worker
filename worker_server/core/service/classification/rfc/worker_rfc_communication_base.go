// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/domain"
	"worker_server/core/service/classification"
)

// =============================================================================
// Communication Base Parser
// =============================================================================
//
// Common patterns for Slack, Teams, Discord:
//   - Mention: @user mentioned you
//   - Direct Message: New DM from @user
//   - Channel Message: New message in #channel
//   - Thread Reply: Reply in thread
//   - Reaction: @user reacted to your message
//   - Invite: Channel/Workspace invite

// CommunicationBaseParser provides common functionality for communication tool parsers.
type CommunicationBaseParser struct {
	service  SaaSService
	category SaaSCategory
}

// NewCommunicationBaseParser creates a new base parser.
func NewCommunicationBaseParser(service SaaSService) *CommunicationBaseParser {
	return &CommunicationBaseParser{
		service:  service,
		category: CategoryCommunication,
	}
}

// Service returns the service.
func (p *CommunicationBaseParser) Service() SaaSService {
	return p.service
}

// Category returns the category.
func (p *CommunicationBaseParser) Category() SaaSCategory {
	return p.category
}

// =============================================================================
// Common Regex Patterns
// =============================================================================

var (
	// Channel patterns
	commChannelPattern = regexp.MustCompile(`#([a-zA-Z0-9_-]+)`)

	// User patterns
	commUserPattern = regexp.MustCompile(`@([a-zA-Z0-9_.-]+)`)
	commFromPattern = regexp.MustCompile(`(?i)(?:from|by)\s+@?([a-zA-Z0-9_.-]+)`)

	// URL patterns
	commSlackURLPattern   = regexp.MustCompile(`https://[^.]+\.slack\.com/[^\s<>"]+`)
	commTeamsURLPattern   = regexp.MustCompile(`https://teams\.microsoft\.com/[^\s<>"]+`)
	commDiscordURLPattern = regexp.MustCompile(`https://discord\.com/channels/[^\s<>"]+`)

	// Message patterns
	commMessagePattern = regexp.MustCompile(`(?i)(?:message|said|wrote)[:\s]*["']?([^"'\n]+)["']?`)
)

// =============================================================================
// Common Extraction Methods
// =============================================================================

// ExtractChannel extracts channel name from text.
func (p *CommunicationBaseParser) ExtractChannel(text string) string {
	if matches := commChannelPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// ExtractMentions extracts @mentions from text.
func (p *CommunicationBaseParser) ExtractMentions(text string) []string {
	matches := commUserPattern.FindAllStringSubmatch(text, -1)
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

// ExtractSender extracts sender from text.
func (p *CommunicationBaseParser) ExtractSender(text string) string {
	if matches := commFromPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// ExtractMessagePreview extracts message preview from text.
func (p *CommunicationBaseParser) ExtractMessagePreview(text string, maxLength int) string {
	if matches := commMessagePattern.FindStringSubmatch(text); len(matches) >= 2 {
		msg := strings.TrimSpace(matches[1])
		if len(msg) > maxLength {
			return msg[:maxLength] + "..."
		}
		return msg
	}
	return ""
}

// =============================================================================
// Common Priority Calculation
// =============================================================================

// CommunicationPriorityConfig holds priority calculation parameters.
type CommunicationPriorityConfig struct {
	DomainScore   float64
	EventScore    float64
	RelationScore float64
}

// CalculateCommunicationPriority calculates priority for communication tools.
func (p *CommunicationBaseParser) CalculateCommunicationPriority(config CommunicationPriorityConfig) (domain.Priority, float64) {
	score := classification.CalculatePriority(
		config.DomainScore,
		config.EventScore,
		config.RelationScore,
		0,
	)
	return p.ScoreToPriority(score), score
}

// GetEventScoreForEvent returns event and relation scores for common events.
func (p *CommunicationBaseParser) GetEventScoreForEvent(event CommunicationEventType) (eventScore, relationScore float64) {
	switch event {
	// Direct involvement - higher priority
	case CommEventMention:
		return classification.ReasonScoreMention, classification.RelationScoreDirect
	case CommEventDirectMessage:
		return 0.25, classification.RelationScoreDirect

	// Channel activity
	case CommEventChannelMessage:
		return 0.08, classification.RelationScoreProject
	case CommEventThreadReply:
		return 0.15, classification.RelationScoreProject

	// Reactions - low priority
	case CommEventReaction:
		return 0.03, classification.RelationScoreWatching

	// Invites
	case CommEventChannelInvite:
		return 0.12, classification.RelationScoreTeam
	case CommEventWorkspaceInvite:
		return 0.18, classification.RelationScoreTeam

	// Digests - low priority
	case CommEventDigest:
		return 0.02, classification.RelationScoreWatching

	// Security
	case CommEventSecurityAlert:
		return classification.ReasonScoreAlertCritical, classification.RelationScoreDirect

	default:
		return 0.05, classification.RelationScoreWatching
	}
}

// ScoreToPriority converts score to Priority.
func (p *CommunicationBaseParser) ScoreToPriority(score float64) domain.Priority {
	switch {
	case score >= 0.8:
		return domain.PriorityUrgent
	case score >= 0.6:
		return domain.PriorityHigh
	case score >= 0.4:
		return domain.PriorityNormal
	case score >= 0.2:
		return domain.PriorityLow
	default:
		return domain.PriorityLowest
	}
}

// =============================================================================
// Common Category Determination
// =============================================================================

// DetermineCommunicationCategory determines category based on event type.
func (p *CommunicationBaseParser) DetermineCommunicationCategory(event CommunicationEventType) (domain.EmailCategory, *domain.EmailSubCategory) {
	notifSubCat := domain.SubCategoryNotification
	alertSubCat := domain.SubCategoryAlert

	switch event {
	// Direct involvement → Notification (important)
	case CommEventMention, CommEventDirectMessage:
		return domain.CategoryNotification, &notifSubCat

	// Security → Work + Alert
	case CommEventSecurityAlert:
		return domain.CategoryWork, &alertSubCat

	// Everything else → Notification (less important)
	default:
		return domain.CategoryNotification, &notifSubCat
	}
}

// =============================================================================
// Common Action Item Generation
// =============================================================================

// GenerateCommunicationActionItems generates action items based on event type.
func (p *CommunicationBaseParser) GenerateCommunicationActionItems(event CommunicationEventType, data *ExtractedData) []ActionItem {
	var items []ActionItem

	switch event {
	case CommEventMention:
		items = append(items, ActionItem{
			Type:     ActionRespond,
			Title:    "Respond to mention in #" + data.Channel,
			URL:      data.URL,
			Priority: "medium",
		})

	case CommEventDirectMessage:
		title := "Reply to DM"
		if data.Author != "" {
			title = "Reply to DM from " + data.Author
		}
		items = append(items, ActionItem{
			Type:     ActionRespond,
			Title:    title,
			URL:      data.URL,
			Priority: "high",
		})

	case CommEventThreadReply:
		items = append(items, ActionItem{
			Type:     ActionRead,
			Title:    "Check thread reply in #" + data.Channel,
			URL:      data.URL,
			Priority: "low",
		})

	case CommEventChannelInvite:
		items = append(items, ActionItem{
			Type:     ActionRespond,
			Title:    "Respond to channel invite: #" + data.Channel,
			URL:      data.URL,
			Priority: "medium",
		})

	case CommEventWorkspaceInvite:
		items = append(items, ActionItem{
			Type:     ActionRespond,
			Title:    "Respond to workspace invite: " + data.Workspace,
			URL:      data.URL,
			Priority: "medium",
		})

	case CommEventSecurityAlert:
		items = append(items, ActionItem{
			Type:     ActionInvestigate,
			Title:    "Investigate security alert",
			URL:      data.URL,
			Priority: "urgent",
		})
	}

	return items
}

// =============================================================================
// Common Entity Extraction
// =============================================================================

// GenerateCommunicationEntities generates entities from extracted data.
func (p *CommunicationBaseParser) GenerateCommunicationEntities(data *ExtractedData) []Entity {
	var entities []Entity

	// Workspace
	if data.Workspace != "" {
		entities = append(entities, Entity{
			Type: EntityWorkspace,
			ID:   data.Workspace,
			Name: data.Workspace,
		})
	}

	// Channel
	if data.Channel != "" {
		entities = append(entities, Entity{
			Type: EntityChannel,
			ID:   data.Channel,
			Name: "#" + data.Channel,
			URL:  data.URL,
		})
	}

	// Author/Sender
	if data.Author != "" {
		entities = append(entities, Entity{
			Type: EntityUser,
			ID:   data.Author,
			Name: data.Author,
		})
	}

	// Mentioned users
	for _, mention := range data.Mentions {
		if mention != data.Author {
			entities = append(entities, Entity{
				Type: EntityUser,
				ID:   mention,
				Name: mention,
			})
		}
	}

	return entities
}
