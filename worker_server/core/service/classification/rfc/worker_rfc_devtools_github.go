// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"strings"

	"worker_server/core/port/out"
	"worker_server/core/service/classification"
)

// =============================================================================
// GitHub Parser
// =============================================================================
//
// GitHub-specific headers:
//   - X-GitHub-Reason: review_requested, author, mention, assign, team_mention,
//     subscribed, state_change, push, comment, ci_activity, security_alert, your_activity
//   - X-GitHub-Sender: GitHub username
//   - X-GitHub-Severity: critical, high, moderate, low (Dependabot)
//   - Cc: xxx@noreply.github.com (alternative reason detection)

// GitHubParser parses GitHub notification emails.
type GitHubParser struct {
	*DevToolsBaseParser
}

// NewGitHubParser creates a new GitHub parser.
func NewGitHubParser() *GitHubParser {
	return &GitHubParser{
		DevToolsBaseParser: NewDevToolsBaseParser(ServiceGitHub),
	}
}

// CanParse checks if this parser can handle the email.
func (p *GitHubParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	// Check X-GitHub-Reason header
	if headers != nil && headers.XGitHubReason != "" {
		return true
	}

	// Check CC addresses for @noreply.github.com
	if headers != nil {
		for _, cc := range headers.CCAddresses {
			if strings.HasSuffix(strings.ToLower(cc), "@noreply.github.com") {
				return true
			}
		}
	}

	// Check from email
	fromLower := strings.ToLower(fromEmail)
	return strings.Contains(fromLower, "github.com") ||
		strings.Contains(fromLower, "noreply.github.com")
}

// Parse extracts structured data from GitHub emails.
func (p *GitHubParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	if input.Headers == nil {
		return nil, nil
	}

	reason := strings.ToLower(input.Headers.XGitHubReason)

	// Try to detect reason from CC addresses if header is empty
	if reason == "" {
		reason = p.detectReasonFromCC(input.Headers.CCAddresses)
	}

	// Determine event type
	event := p.determineEvent(reason, input)

	// Extract data
	data := p.extractData(input, reason)

	// Calculate priority
	domainScore := classification.DomainScoreGitHub

	// Security alerts get special handling
	if event == DevEventSecurityAlert || event == DevEventDependabotAlert {
		score := classification.GetGitHubSecurityScore(input.Headers.XGitHubSeverity)
		priority := p.ScoreToPriority(score)

		category, subCat := p.DetermineDevToolsCategory(event)
		actionItems := p.GenerateDevToolsActionItems(event, data)
		entities := p.GenerateDevToolsEntities(data, "https://github.com")

		return &ParsedEmail{
			Category:      CategoryDevTools,
			Service:       ServiceGitHub,
			Event:         string(event),
			EmailCategory: category,
			SubCategory:   subCat,
			Priority:      priority,
			Score:         score,
			Source:        "rfc:github:" + reason,
			Data:          data,
			ActionItems:   actionItems,
			Entities:      entities,
			Signals:       []string{"github", "reason:" + reason, "severity:" + input.Headers.XGitHubSeverity},
		}, nil
	}

	// Normal priority calculation
	reasonScore, relationScore := p.GetReasonScoreForEvent(event)
	priority, score := p.CalculateDevToolsPriority(DevToolsPriorityConfig{
		DomainScore:   domainScore,
		ReasonScore:   reasonScore,
		RelationScore: relationScore,
	})

	// Determine category
	category, subCat := p.DetermineDevToolsCategory(event)

	// Generate action items
	actionItems := p.GenerateDevToolsActionItems(event, data)

	// Generate entities
	entities := p.GenerateDevToolsEntities(data, "https://github.com")

	return &ParsedEmail{
		Category:      CategoryDevTools,
		Service:       ServiceGitHub,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:github:" + reason,
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"github", "reason:" + reason},
	}, nil
}

// detectReasonFromCC extracts reason from CC addresses.
func (p *GitHubParser) detectReasonFromCC(ccAddresses []string) string {
	for _, cc := range ccAddresses {
		cc = strings.ToLower(cc)
		if strings.HasSuffix(cc, "@noreply.github.com") {
			parts := strings.Split(cc, "@")
			if len(parts) > 0 {
				return parts[0]
			}
		}
	}
	return ""
}

// determineEvent determines the event type from reason and content.
func (p *GitHubParser) determineEvent(reason string, input *ParserInput) DevToolsEventType {
	subject := ""
	if input.Message != nil {
		subject = strings.ToLower(input.Message.Subject)
	}

	// Security alerts
	if reason == "security_alert" {
		if strings.Contains(subject, "dependabot") {
			return DevEventDependabotAlert
		}
		if strings.Contains(subject, "secret") {
			return DevEventSecretScanAlert
		}
		return DevEventSecurityAlert
	}

	// CI activity
	if reason == "ci_activity" {
		if strings.Contains(subject, "failed") || strings.Contains(subject, "failure") {
			return DevEventCIFailed
		}
		if strings.Contains(subject, "succeeded") || strings.Contains(subject, "success") ||
			strings.Contains(subject, "passed") {
			return DevEventCIPassed
		}
		return DevEventCIRunning
	}

	// Map reason to event
	switch reason {
	case "review_requested":
		return DevEventReviewRequested
	case "mention":
		if p.isPullRequest(subject, input) {
			return DevEventPRComment
		}
		return DevEventIssueMentioned
	case "assign":
		if p.isPullRequest(subject, input) {
			return DevEventReviewRequested
		}
		return DevEventIssueAssigned
	case "author":
		return p.determineAuthorEvent(subject, input)
	case "team_mention":
		return DevEventTeamMention
	case "comment":
		if p.isPullRequest(subject, input) {
			return DevEventPRComment
		}
		return DevEventIssueComment
	case "state_change":
		return p.determineStateChangeEvent(subject, input)
	case "push":
		return DevEventPRPush
	case "subscribed", "manual":
		return DevEventWatching
	case "your_activity":
		return DevEventOwnActivity
	}

	// Detect from subject
	if strings.Contains(subject, "release") {
		return DevEventRelease
	}

	return DevEventIssueComment
}

// isPullRequest checks if the email is about a Pull Request.
func (p *GitHubParser) isPullRequest(subject string, input *ParserInput) bool {
	prPatterns := []string{"pull request", "pr #", "merged #", "review requested"}
	for _, pattern := range prPatterns {
		if strings.Contains(subject, pattern) {
			return true
		}
	}
	return false
}

// determineAuthorEvent determines event type for author notifications.
func (p *GitHubParser) determineAuthorEvent(subject string, input *ParserInput) DevToolsEventType {
	if p.isPullRequest(subject, input) {
		if strings.Contains(subject, "merged") {
			return DevEventPRMerged
		}
		if strings.Contains(subject, "closed") {
			return DevEventPRClosed
		}
		if strings.Contains(subject, "approved") {
			return DevEventReviewApproved
		}
		if strings.Contains(subject, "changes requested") {
			return DevEventReviewChangesReq
		}
		return DevEventPRComment
	}

	if strings.Contains(subject, "closed") {
		return DevEventIssueClosed
	}
	if strings.Contains(subject, "reopened") {
		return DevEventIssueReopened
	}
	return DevEventIssueComment
}

// determineStateChangeEvent determines state change event type.
func (p *GitHubParser) determineStateChangeEvent(subject string, input *ParserInput) DevToolsEventType {
	if p.isPullRequest(subject, input) {
		if strings.Contains(subject, "merged") {
			return DevEventPRMerged
		}
		if strings.Contains(subject, "closed") {
			return DevEventPRClosed
		}
	}
	if strings.Contains(subject, "closed") {
		return DevEventIssueClosed
	}
	if strings.Contains(subject, "reopened") {
		return DevEventIssueReopened
	}
	return DevEventIssueComment
}

// extractData extracts structured data from the email.
func (p *GitHubParser) extractData(input *ParserInput, reason string) *ExtractedData {
	data := &ExtractedData{
		Extra: make(map[string]interface{}),
	}

	if input.Message == nil {
		return data
	}

	subject := input.Message.Subject

	// Extract from List-Id header
	if input.RawHeaders != nil {
		if listID := input.RawHeaders["List-Id"]; listID != "" {
			data.Repository = p.parseListID(listID)
		}
	}

	// Extract repo from subject if not from List-Id
	if data.Repository == "" {
		data.Repository = p.ExtractRepoFromSubject(subject)
	}

	// Extract number
	num := p.ExtractNumberFromSubject(subject)
	if p.isPullRequest(strings.ToLower(subject), input) {
		data.PRNumber = num
	} else {
		data.IssueNumber = num
	}

	// Extract title
	data.Title = p.ExtractTitleFromSubject(subject, data.Repository, num)

	// Extract from headers
	if input.Headers != nil {
		data.Author = input.Headers.XGitHubSender
		data.Severity = input.Headers.XGitHubSeverity

		// Labels from raw headers
		if input.RawHeaders != nil {
			if labels := input.RawHeaders["X-GitHub-Labels"]; labels != "" {
				data.Labels = splitAndTrim(labels, ",")
			}
			if assignees := input.RawHeaders["X-GitHub-Assignees"]; assignees != "" {
				data.Assignees = splitAndTrim(assignees, ",")
			}
		}
	}

	// Extract from body
	if input.Body != nil {
		bodyText := input.Body.Text
		if bodyText == "" {
			bodyText = input.Body.HTML
		}

		data.URL = p.extractGitHubURL(bodyText)
		data.Mentions = p.ExtractMentions(bodyText)
		data.CommitHash = p.ExtractCommitHash(bodyText)
		data.Branch = p.ExtractBranch(bodyText)

		// Security info
		if reason == "security_alert" {
			p.ExtractSecurityInfo(data, bodyText)
		}
	}

	return data
}

// parseListID parses repository from List-Id header.
func (p *GitHubParser) parseListID(listID string) string {
	// Format: <repo.owner.github.com> or repo.owner.github.com
	listID = strings.Trim(listID, "<>")
	parts := strings.Split(listID, ".")

	if len(parts) >= 3 && parts[len(parts)-1] == "com" && parts[len(parts)-2] == "github" {
		owner := parts[len(parts)-3]
		if len(parts) >= 4 {
			repo := parts[len(parts)-4]
			return owner + "/" + repo
		}
		return owner
	}
	return ""
}

// extractGitHubURL extracts GitHub URL from body.
func (p *GitHubParser) extractGitHubURL(text string) string {
	if matches := devGitHubURLPattern.FindStringSubmatch(text); len(matches) >= 1 {
		return matches[0]
	}
	return ""
}

// splitAndTrim splits string and trims each part.
func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
