// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"strings"

	"worker_server/core/port/out"
	"worker_server/core/service/classification"
)

// =============================================================================
// Slack Parser
// =============================================================================
//
// Slack email patterns:
//   - From: notification@slack.com (singular, not notifications)
//   - Subject: "New message from @user in #channel"
//   - Subject: "@user mentioned you in #channel"
//   - Subject: "Direct message from @user"

// SlackParser parses Slack notification emails.
type SlackParser struct {
	*CommunicationBaseParser
}

// NewSlackParser creates a new Slack parser.
func NewSlackParser() *SlackParser {
	return &SlackParser{
		CommunicationBaseParser: NewCommunicationBaseParser(ServiceSlack),
	}
}

// CanParse checks if this parser can handle the email.
func (p *SlackParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)
	return strings.Contains(fromLower, "slack.com")
}

// Parse extracts structured data from Slack emails.
func (p *SlackParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	// Detect event from content
	event := p.detectEvent(input)

	// Extract data
	data := p.extractData(input)

	// Calculate priority
	eventScore, relationScore := p.GetEventScoreForEvent(event)
	priority, score := p.CalculateCommunicationPriority(CommunicationPriorityConfig{
		DomainScore:   classification.DomainScoreSlack,
		EventScore:    eventScore,
		RelationScore: relationScore,
	})

	// Determine category
	category, subCat := p.DetermineCommunicationCategory(event)

	// Generate action items
	actionItems := p.GenerateCommunicationActionItems(event, data)

	// Generate entities
	entities := p.GenerateCommunicationEntities(data)

	return &ParsedEmail{
		Category:      CategoryCommunication,
		Service:       ServiceSlack,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:slack:" + string(event),
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"slack", "event:" + string(event)},
	}, nil
}

// detectEvent detects the Slack event from content.
func (p *SlackParser) detectEvent(input *ParserInput) CommunicationEventType {
	subject := ""
	if input.Message != nil {
		subject = strings.ToLower(input.Message.Subject)
	}

	bodyText := ""
	if input.Body != nil {
		bodyText = strings.ToLower(input.Body.Text)
	}

	switch {
	case strings.Contains(subject, "mentioned you") || strings.Contains(bodyText, "mentioned you"):
		return CommEventMention
	case strings.Contains(subject, "direct message") || strings.Contains(subject, "dm from"):
		return CommEventDirectMessage
	case strings.Contains(subject, "replied in thread") || strings.Contains(bodyText, "replied to"):
		return CommEventThreadReply
	case strings.Contains(subject, "reacted") || strings.Contains(bodyText, "reacted"):
		return CommEventReaction
	case strings.Contains(subject, "invited you") && strings.Contains(subject, "channel"):
		return CommEventChannelInvite
	case strings.Contains(subject, "invited you") && strings.Contains(subject, "workspace"):
		return CommEventWorkspaceInvite
	case strings.Contains(subject, "digest") || strings.Contains(subject, "summary"):
		return CommEventDigest
	case strings.Contains(subject, "security") || strings.Contains(subject, "suspicious"):
		return CommEventSecurityAlert
	case strings.Contains(subject, "new message") || strings.Contains(subject, "message in"):
		return CommEventChannelMessage
	}

	return CommEventChannelMessage
}

// extractData extracts structured data from the email.
func (p *SlackParser) extractData(input *ParserInput) *ExtractedData {
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

	// Extract channel
	data.Channel = p.ExtractChannel(combined)

	// Extract workspace from subject
	data.Workspace = p.extractSlackWorkspace(subject)

	// Extract sender
	data.Author = p.ExtractSender(combined)
	if data.Author == "" {
		// Try to extract from subject patterns like "message from @user"
		data.Author = p.extractSlackSender(subject)
	}

	// Extract URL
	if match := commSlackURLPattern.FindString(combined); match != "" {
		data.URL = match
	}

	// Extract message preview
	data.MessageText = p.ExtractMessagePreview(bodyText, 100)

	// Extract mentions
	data.Mentions = p.ExtractMentions(combined)

	// Extract title
	data.Title = p.extractSlackTitle(subject)

	return data
}

// extractSlackWorkspace extracts workspace name from subject.
func (p *SlackParser) extractSlackWorkspace(subject string) string {
	// Pattern: "... in WorkspaceName" or "[WorkspaceName] ..."
	if strings.HasPrefix(subject, "[") {
		if idx := strings.Index(subject, "]"); idx > 0 {
			return subject[1:idx]
		}
	}
	return ""
}

// extractSlackSender extracts sender from subject patterns.
func (p *SlackParser) extractSlackSender(subject string) string {
	patterns := []string{
		"message from ", "dm from ", "direct message from ",
	}
	subjectLower := strings.ToLower(subject)
	for _, pattern := range patterns {
		if idx := strings.Index(subjectLower, pattern); idx >= 0 {
			remaining := subject[idx+len(pattern):]
			// Get until space or "in"
			if spaceIdx := strings.Index(remaining, " "); spaceIdx > 0 {
				return strings.TrimPrefix(remaining[:spaceIdx], "@")
			}
			return strings.TrimPrefix(remaining, "@")
		}
	}
	return ""
}

// extractSlackTitle extracts clean title from subject.
func (p *SlackParser) extractSlackTitle(subject string) string {
	title := subject

	// Remove [Workspace] prefix
	if strings.HasPrefix(title, "[") {
		if idx := strings.Index(title, "]"); idx > 0 {
			title = strings.TrimSpace(title[idx+1:])
		}
	}

	return strings.TrimSpace(title)
}
