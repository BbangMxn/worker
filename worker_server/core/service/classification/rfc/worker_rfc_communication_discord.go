// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/port/out"
	"worker_server/core/service/classification"
)

// =============================================================================
// Discord Parser
// =============================================================================
//
// Discord email patterns:
//   - From: noreply@discord.com, notifications@discord.com
//   - From: *@m.discord.com (marketing/announcements)
//   - From: noreply@discordapp.com (legacy)
//
// Subject patterns:
//   - "[Username] mentioned you in [Server Name]"
//   - "New message from [Username]"
//   - "Direct message from [Username]"
//   - "[Username] has invited you to join [Server Name]"
//   - "[Username] wants to be your friend"
//   - "Verify Email Address for Discord"
//
// URL patterns:
//   - https://discord.com/channels/[server_id]/[channel_id]/[message_id]
//
// NOTE: Discord limits email notifications - primarily for DMs when user is offline.
// Mobile app usage often disables email notifications.

// DiscordParser parses Discord notification emails.
type DiscordParser struct {
	*CommunicationBaseParser
}

// NewDiscordParser creates a new Discord parser.
func NewDiscordParser() *DiscordParser {
	return &DiscordParser{
		CommunicationBaseParser: NewCommunicationBaseParser(ServiceDiscord),
	}
}

// Discord-specific regex patterns
var (
	// URL pattern to extract server/channel/message IDs (Snowflake IDs: 17-19 digits)
	discordURLPattern = regexp.MustCompile(`https://discord\.com/channels/(\d{17,19})/(\d{17,19})(?:/(\d{17,19}))?`)

	// Subject patterns
	discordMentionPattern       = regexp.MustCompile(`(?i)(.+)\s+mentioned you in\s+(.+)`)
	discordDMPattern            = regexp.MustCompile(`(?i)(?:new message from|direct message from)\s+(.+)`)
	discordInvitePattern        = regexp.MustCompile(`(?i)(.+)\s+(?:has )?invited you to (?:join\s+)?(.+)`)
	discordFriendPattern        = regexp.MustCompile(`(?i)(.+)\s+wants to be your friend`)
	discordVerifyPattern        = regexp.MustCompile(`(?i)verify.*email.*discord`)
	discordSecurityPattern      = regexp.MustCompile(`(?i)(?:suspicious|security|password|unauthorized)`)
	discordServerMessagePattern = regexp.MustCompile(`(?i)new message in\s+#?(.+)`)
	discordReplyPattern         = regexp.MustCompile(`(?i)(?:replied to|reply to your message)`)
)

// CanParse checks if this parser can handle the email.
func (p *DiscordParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)

	// Check Discord domains
	return strings.Contains(fromLower, "@discord.com") ||
		strings.Contains(fromLower, "@discordapp.com") ||
		strings.Contains(fromLower, "@m.discord.com")
}

// Parse extracts structured data from Discord emails.
func (p *DiscordParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	// Detect event from content
	event := p.detectEvent(input)

	// Extract data
	data := p.extractData(input)

	// Calculate priority
	eventScore, relationScore := p.GetEventScoreForEvent(event)
	priority, score := p.CalculateCommunicationPriority(CommunicationPriorityConfig{
		DomainScore:   classification.DomainScoreSlack - 0.02, // Slightly lower than Slack
		EventScore:    eventScore,
		RelationScore: relationScore,
	})

	// Determine category
	category, subCat := p.DetermineCommunicationCategory(event)

	// Generate action items
	actionItems := p.GenerateCommunicationActionItems(event, data)

	// Generate entities
	entities := p.generateDiscordEntities(data)

	return &ParsedEmail{
		Category:      CategoryCommunication,
		Service:       ServiceDiscord,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:discord:" + string(event),
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"discord", "event:" + string(event)},
	}, nil
}

// detectEvent detects the Discord event from content.
func (p *DiscordParser) detectEvent(input *ParserInput) CommunicationEventType {
	subject := ""
	if input.Message != nil {
		subject = input.Message.Subject
	}

	subjectLower := strings.ToLower(subject)

	// Check for security alerts first (highest priority)
	if discordSecurityPattern.MatchString(subject) {
		return CommEventSecurityAlert
	}

	// Check for verification (transactional)
	if discordVerifyPattern.MatchString(subject) {
		return CommEventDigest // Treat as low priority
	}

	// Check for mentions
	if discordMentionPattern.MatchString(subject) || strings.Contains(subjectLower, "mentioned you") {
		return CommEventMention
	}

	// Check for DMs
	if discordDMPattern.MatchString(subject) || strings.Contains(subjectLower, "direct message") {
		return CommEventDirectMessage
	}

	// Check for replies
	if discordReplyPattern.MatchString(subject) {
		return CommEventThreadReply
	}

	// Check for server invites
	if discordInvitePattern.MatchString(subject) {
		return CommEventWorkspaceInvite
	}

	// Check for friend requests
	if discordFriendPattern.MatchString(subject) {
		return CommEventChannelInvite // Use channel invite for social
	}

	// Check for channel messages
	if discordServerMessagePattern.MatchString(subject) || strings.Contains(subjectLower, "new message") {
		return CommEventChannelMessage
	}

	// Check for digest/summary
	if strings.Contains(subjectLower, "digest") || strings.Contains(subjectLower, "summary") {
		return CommEventDigest
	}

	return CommEventChannelMessage
}

// extractData extracts structured data from the email.
func (p *DiscordParser) extractData(input *ParserInput) *ExtractedData {
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

	// Extract Discord URL with server/channel/message IDs
	if matches := discordURLPattern.FindStringSubmatch(combined); len(matches) >= 3 {
		data.URL = matches[0]
		data.Extra["server_id"] = matches[1]
		data.Extra["channel_id"] = matches[2]
		if len(matches) >= 4 && matches[3] != "" {
			data.Extra["message_id"] = matches[3]
		}
	}

	// Extract from subject patterns
	p.extractFromSubject(subject, data)

	// Extract channel from content
	if data.Channel == "" {
		data.Channel = p.ExtractChannel(combined)
	}

	// Extract mentions
	data.Mentions = p.ExtractMentions(combined)

	// Extract message preview
	data.MessageText = p.ExtractMessagePreview(bodyText, 100)

	return data
}

// extractFromSubject extracts author, server, channel from subject.
func (p *DiscordParser) extractFromSubject(subject string, data *ExtractedData) {
	// Try mention pattern: "[User] mentioned you in [Server]"
	if matches := discordMentionPattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		data.Workspace = strings.TrimSpace(matches[2])
		data.Title = subject
		return
	}

	// Try DM pattern: "New message from [User]" or "Direct message from [User]"
	if matches := discordDMPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		data.Author = strings.TrimSpace(matches[1])
		data.Title = "Direct Message from " + data.Author
		return
	}

	// Try invite pattern: "[User] invited you to join [Server]"
	if matches := discordInvitePattern.FindStringSubmatch(subject); len(matches) >= 3 {
		data.Author = strings.TrimSpace(matches[1])
		data.Workspace = strings.TrimSpace(matches[2])
		data.Title = subject
		return
	}

	// Try friend pattern: "[User] wants to be your friend"
	if matches := discordFriendPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		data.Author = strings.TrimSpace(matches[1])
		data.Title = subject
		return
	}

	// Try server message pattern: "New message in #[Channel]"
	if matches := discordServerMessagePattern.FindStringSubmatch(subject); len(matches) >= 2 {
		data.Channel = strings.TrimPrefix(strings.TrimSpace(matches[1]), "#")
		data.Title = subject
		return
	}

	// Fallback
	data.Title = subject
}

// generateDiscordEntities generates entities from extracted data.
func (p *DiscordParser) generateDiscordEntities(data *ExtractedData) []Entity {
	var entities []Entity

	// Server entity
	if serverID, ok := data.Extra["server_id"].(string); ok && serverID != "" {
		entities = append(entities, Entity{
			Type: EntityWorkspace,
			ID:   serverID,
			Name: data.Workspace,
		})
	}

	// Channel entity
	if channelID, ok := data.Extra["channel_id"].(string); ok && channelID != "" {
		name := data.Channel
		if name == "" {
			name = "Channel"
		}
		entities = append(entities, Entity{
			Type: EntityChannel,
			ID:   channelID,
			Name: name,
			URL:  data.URL,
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
