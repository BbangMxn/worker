// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"strings"

	"worker_server/core/port/out"
	"worker_server/core/service/classification"
)

// =============================================================================
// Microsoft Teams Parser
// =============================================================================
//
// Teams email patterns:
//   - From: noreply@email.teams.microsoft.com
//   - Subject: "@user mentioned you in ChannelName"
//   - Subject: "New message in ChannelName"
//   - Subject: "Chat message from @user"

// TeamsParser parses Microsoft Teams notification emails.
type TeamsParser struct {
	*CommunicationBaseParser
}

// NewTeamsParser creates a new Teams parser.
func NewTeamsParser() *TeamsParser {
	return &TeamsParser{
		CommunicationBaseParser: NewCommunicationBaseParser(ServiceTeams),
	}
}

// CanParse checks if this parser can handle the email.
func (p *TeamsParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)
	// Teams sends from multiple domains:
	// - noreply@email.teams.microsoft.com (older)
	// - teams.mail.microsoft (newer)
	// - microsoft365.com
	return strings.Contains(fromLower, "teams.microsoft.com") ||
		strings.Contains(fromLower, "teams.mail.microsoft") ||
		strings.Contains(fromLower, "microsoft365.com")
}

// Parse extracts structured data from Teams emails.
func (p *TeamsParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	// Detect event from content
	event := p.detectEvent(input)

	// Extract data
	data := p.extractData(input)

	// Calculate priority - Teams slightly higher than Slack for enterprise
	eventScore, relationScore := p.GetEventScoreForEvent(event)
	priority, score := p.CalculateCommunicationPriority(CommunicationPriorityConfig{
		DomainScore:   classification.DomainScoreSlack + 0.02, // Slightly higher for enterprise
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
		Service:       ServiceTeams,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:teams:" + string(event),
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"teams", "event:" + string(event)},
	}, nil
}

// detectEvent detects the Teams event from content.
func (p *TeamsParser) detectEvent(input *ParserInput) CommunicationEventType {
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
	case strings.Contains(subject, "chat message") || strings.Contains(subject, "direct message"):
		return CommEventDirectMessage
	case strings.Contains(subject, "replied") || strings.Contains(bodyText, "replied to"):
		return CommEventThreadReply
	case strings.Contains(subject, "reacted"):
		return CommEventReaction
	case strings.Contains(subject, "added you") && strings.Contains(subject, "channel"):
		return CommEventChannelInvite
	case strings.Contains(subject, "added you") && strings.Contains(subject, "team"):
		return CommEventWorkspaceInvite
	case strings.Contains(subject, "digest") || strings.Contains(subject, "activity summary"):
		return CommEventDigest
	case strings.Contains(subject, "new message") || strings.Contains(subject, "posted in"):
		return CommEventChannelMessage
	}

	return CommEventChannelMessage
}

// extractData extracts structured data from the email.
func (p *TeamsParser) extractData(input *ParserInput) *ExtractedData {
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

	// Extract channel/team
	data.Channel = p.extractTeamsChannel(subject)
	data.Workspace = p.extractTeamsTeam(subject)

	// Extract sender
	data.Author = p.ExtractSender(combined)

	// Extract URL
	if match := commTeamsURLPattern.FindString(combined); match != "" {
		data.URL = match
	}

	// Extract message preview
	data.MessageText = p.ExtractMessagePreview(bodyText, 100)

	// Extract mentions
	data.Mentions = p.ExtractMentions(combined)

	// Extract title
	data.Title = p.extractTeamsTitle(subject)

	return data
}

// extractTeamsChannel extracts channel name from subject.
func (p *TeamsParser) extractTeamsChannel(subject string) string {
	// Pattern: "... in ChannelName" or "ChannelName > SubChannel"
	subjectLower := strings.ToLower(subject)

	// Look for "in [channel]" pattern
	if idx := strings.LastIndex(subjectLower, " in "); idx > 0 {
		channel := subject[idx+4:]
		// Remove trailing parts
		if endIdx := strings.Index(channel, " |"); endIdx > 0 {
			channel = channel[:endIdx]
		}
		return strings.TrimSpace(channel)
	}

	return ""
}

// extractTeamsTeam extracts team name from subject.
func (p *TeamsParser) extractTeamsTeam(subject string) string {
	// Pattern: "[TeamName] ..." or "... | TeamName"
	if idx := strings.LastIndex(subject, " | "); idx > 0 {
		return strings.TrimSpace(subject[idx+3:])
	}
	return ""
}

// extractTeamsTitle extracts clean title from subject.
func (p *TeamsParser) extractTeamsTitle(subject string) string {
	title := subject

	// Remove team suffix
	if idx := strings.LastIndex(title, " | "); idx > 0 {
		title = title[:idx]
	}

	return strings.TrimSpace(title)
}
