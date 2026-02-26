// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strconv"
	"strings"

	"worker_server/core/port/out"
	"worker_server/core/service/classification"
)

// =============================================================================
// GitLab Parser
// =============================================================================
//
// GitLab-specific headers:
//   - X-GitLab-Project: Project name
//   - X-GitLab-Project-Id: Project ID
//   - X-GitLab-Project-Path: Full project path (group/subgroup/project)
//   - X-GitLab-Issue-IID: Issue internal ID
//   - X-GitLab-Issue-State: opened, closed (issue state)
//   - X-GitLab-MergeRequest-IID: MR internal ID
//   - X-GitLab-MergeRequest-State: opened, merged, closed (MR state)
//   - X-GitLab-Pipeline-Id: Pipeline ID
//   - X-GitLab-Pipeline-Status: success, failed, pending, running, canceled
//   - X-GitLab-NotificationReason: own_activity, assigned, review_requested, mentioned, subscribed

// GitLabParser parses GitLab notification emails.
type GitLabParser struct {
	*DevToolsBaseParser
}

// NewGitLabParser creates a new GitLab parser.
func NewGitLabParser() *GitLabParser {
	return &GitLabParser{
		DevToolsBaseParser: NewDevToolsBaseParser(ServiceGitLab),
	}
}

// CanParse checks if this parser can handle the email.
func (p *GitLabParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	// Check GitLab-specific headers
	if headers != nil {
		if headers.XGitLabProject != "" ||
			headers.XGitLabPipelineID != "" ||
			headers.XGitLabNotificationReason != "" {
			return true
		}
	}

	// Check raw headers
	if rawHeaders != nil {
		if rawHeaders["X-GitLab-Project-Path"] != "" ||
			rawHeaders["X-GitLab-MergeRequest-IID"] != "" ||
			rawHeaders["X-GitLab-Issue-IID"] != "" {
			return true
		}
	}

	// Check from email
	fromLower := strings.ToLower(fromEmail)
	return strings.Contains(fromLower, "gitlab.com") ||
		strings.Contains(fromLower, "gitlab")
}

// Parse extracts structured data from GitLab emails.
func (p *GitLabParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	reason := ""
	if input.Headers != nil {
		reason = strings.ToLower(input.Headers.XGitLabNotificationReason)
	}

	// Determine event type
	event := p.determineEvent(reason, input)

	// Extract data
	data := p.extractData(input)

	// Calculate priority
	domainScore := classification.DomainScoreGitLab

	// Pipeline failures get high priority
	if event == DevEventCIFailed {
		reasonScore := classification.ReasonScoreCIFailed
		relationScore := classification.RelationScoreProject
		priority, score := p.CalculateDevToolsPriority(DevToolsPriorityConfig{
			DomainScore:   domainScore,
			ReasonScore:   reasonScore,
			RelationScore: relationScore,
		})

		category, subCat := p.DetermineDevToolsCategory(event)
		actionItems := p.GenerateDevToolsActionItems(event, data)
		entities := p.GenerateDevToolsEntities(data, "https://gitlab.com")

		return &ParsedEmail{
			Category:      CategoryDevTools,
			Service:       ServiceGitLab,
			Event:         string(event),
			EmailCategory: category,
			SubCategory:   subCat,
			Priority:      priority,
			Score:         score,
			Source:        "rfc:gitlab:" + reason,
			Data:          data,
			ActionItems:   actionItems,
			Entities:      entities,
			Signals:       []string{"gitlab", "reason:" + reason, "pipeline:" + data.PipelineID},
		}, nil
	}

	// Normal priority calculation
	reasonScore, relationScore := classification.GetGitLabReasonScore(reason)
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
	entities := p.GenerateDevToolsEntities(data, "https://gitlab.com")

	return &ParsedEmail{
		Category:      CategoryDevTools,
		Service:       ServiceGitLab,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:gitlab:" + reason,
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"gitlab", "reason:" + reason},
	}, nil
}

// determineEvent determines the event type from reason and content.
func (p *GitLabParser) determineEvent(reason string, input *ParserInput) DevToolsEventType {
	subject := ""
	if input.Message != nil {
		subject = strings.ToLower(input.Message.Subject)
	}

	// Check for pipeline status
	if input.Headers != nil && input.Headers.XGitLabPipelineID != "" {
		pipelineStatus := ""
		if input.RawHeaders != nil {
			pipelineStatus = strings.ToLower(input.RawHeaders["X-GitLab-Pipeline-Status"])
		}

		switch pipelineStatus {
		case "failed":
			return DevEventCIFailed
		case "success":
			return DevEventCIPassed
		case "canceled":
			return DevEventCIFailed // Treat canceled as failure for alerting
		case "running", "pending":
			return DevEventCIPassed // Treat as neutral
		}

		// Check subject for pipeline status
		if strings.Contains(subject, "failed") {
			return DevEventCIFailed
		}
		if strings.Contains(subject, "fixed") || strings.Contains(subject, "success") {
			return DevEventCIPassed
		}
	}

	// Check MR state from headers
	if input.RawHeaders != nil {
		mrState := strings.ToLower(input.RawHeaders["X-GitLab-MergeRequest-State"])
		switch mrState {
		case "merged":
			return DevEventPRMerged
		case "closed":
			return DevEventPRClosed
		}

		issueState := strings.ToLower(input.RawHeaders["X-GitLab-Issue-State"])
		if issueState == "closed" {
			return DevEventIssueClosed
		}
	}

	// Check MR vs Issue
	isMR := p.isMergeRequest(input)

	switch reason {
	case "review_requested":
		return DevEventReviewRequested
	case "mentioned", "directly_addressed":
		if isMR {
			return DevEventPRComment
		}
		return DevEventIssueMentioned
	case "assigned":
		if isMR {
			return DevEventReviewRequested
		}
		return DevEventIssueAssigned
	case "approval_required":
		return DevEventReviewRequested
	case "subscribed", "watching":
		return DevEventWatching
	case "own_activity":
		return DevEventOwnActivity
	}

	// Detect from subject
	if isMR {
		if strings.Contains(subject, "merged") {
			return DevEventPRMerged
		}
		if strings.Contains(subject, "closed") {
			return DevEventPRClosed
		}
		if strings.Contains(subject, "approved") {
			return DevEventReviewApproved
		}
		return DevEventPRComment
	}

	if strings.Contains(subject, "closed") {
		return DevEventIssueClosed
	}
	if strings.Contains(subject, "due") {
		return DevEventIssueComment // Could be custom event
	}

	return DevEventIssueComment
}

// isMergeRequest checks if the email is about a Merge Request.
func (p *GitLabParser) isMergeRequest(input *ParserInput) bool {
	// Check MR headers
	if input.RawHeaders != nil {
		if input.RawHeaders["X-GitLab-MergeRequest-IID"] != "" ||
			input.RawHeaders["X-GitLab-MergeRequest-ID"] != "" {
			return true
		}
	}

	// Check subject patterns
	if input.Message != nil {
		subject := strings.ToLower(input.Message.Subject)
		return strings.Contains(subject, "merge request") ||
			strings.Contains(subject, "(!")
	}

	return false
}

// extractData extracts structured data from the email.
func (p *GitLabParser) extractData(input *ParserInput) *ExtractedData {
	data := &ExtractedData{
		Extra: make(map[string]interface{}),
	}

	if input.Message == nil {
		return data
	}

	subject := input.Message.Subject

	// Extract from raw headers
	if input.RawHeaders != nil {
		data.Project = input.RawHeaders["X-GitLab-Project"]
		data.ProjectID = input.RawHeaders["X-GitLab-Project-Id"]

		if path := input.RawHeaders["X-GitLab-Project-Path"]; path != "" {
			data.Repository = path
		}

		// Issue IID
		if iid := input.RawHeaders["X-GitLab-Issue-IID"]; iid != "" {
			if num, err := strconv.Atoi(iid); err == nil {
				data.IssueNumber = num
			}
		}

		// MR IID
		if iid := input.RawHeaders["X-GitLab-MergeRequest-IID"]; iid != "" {
			if num, err := strconv.Atoi(iid); err == nil {
				data.MRNumber = num
			}
		}

		// Pipeline info
		data.PipelineID = input.RawHeaders["X-GitLab-Pipeline-Id"]
		data.BuildStatus = input.RawHeaders["X-GitLab-Pipeline-Status"]

		// State info (added headers)
		if issueState := input.RawHeaders["X-GitLab-Issue-State"]; issueState != "" {
			data.Extra["issue_state"] = issueState
		}
		if mrState := input.RawHeaders["X-GitLab-MergeRequest-State"]; mrState != "" {
			data.Extra["mr_state"] = mrState
		}
	}

	// Extract from headers (pre-parsed)
	if input.Headers != nil {
		if data.Project == "" {
			data.Project = input.Headers.XGitLabProject
		}
		if data.PipelineID == "" {
			data.PipelineID = input.Headers.XGitLabPipelineID
		}
	}

	// Extract from subject
	data.Title = p.extractGitLabTitle(subject)

	// Extract numbers from subject if not from headers
	if data.IssueNumber == 0 && data.MRNumber == 0 {
		issueNum, mrNum := p.extractGitLabNumbers(subject)
		data.IssueNumber = issueNum
		data.MRNumber = mrNum
	}

	// Extract from body
	if input.Body != nil {
		bodyText := input.Body.Text
		if bodyText == "" {
			bodyText = input.Body.HTML
		}

		data.URL = p.extractGitLabURL(bodyText)
		data.Author = p.extractAuthor(bodyText)
		data.Mentions = p.ExtractMentions(bodyText)
		data.Branch = p.ExtractBranch(bodyText)
		data.CommitHash = p.ExtractCommitHash(bodyText)
	}

	return data
}

// extractGitLabTitle extracts clean title from subject.
func (p *GitLabParser) extractGitLabTitle(subject string) string {
	title := subject

	// Remove [project] prefix
	title = devSubjectRepoPattern.ReplaceAllString(title, "")

	// Remove (#123) or (!123) patterns
	title = devSubjectNumberPattern.ReplaceAllString(title, "")

	// Remove Re: prefix
	title = strings.TrimPrefix(title, "Re: ")

	// Remove | separator content
	if idx := strings.Index(title, "|"); idx > 0 {
		title = strings.TrimSpace(title[idx+1:])
	}

	return strings.TrimSpace(title)
}

// extractGitLabNumbers extracts issue/MR numbers from subject.
func (p *GitLabParser) extractGitLabNumbers(subject string) (issueNum, mrNum int) {
	// Match (!123) for MR
	for _, match := range devSubjectNumberPattern.FindAllStringSubmatch(subject, -1) {
		for i, m := range match {
			if m != "" && i > 0 {
				if num, err := strconv.Atoi(m); err == nil {
					// Determine if MR or Issue based on context
					if strings.Contains(subject, "(!") || strings.Contains(subject, "merge request") {
						mrNum = num
					} else {
						issueNum = num
					}
					break
				}
			}
		}
	}
	return
}

// extractGitLabURL extracts GitLab URL from body.
func (p *GitLabParser) extractGitLabURL(text string) string {
	if matches := devGitLabURLPattern.FindStringSubmatch(text); len(matches) >= 1 {
		return matches[0]
	}
	return ""
}

// extractAuthor extracts the author from body text.
func (p *GitLabParser) extractAuthor(text string) string {
	// Look for "by @username" or "from @username" patterns
	authorPattern := regexp.MustCompile(`(?:by|from)\s+@?([a-zA-Z0-9_-]+)`)
	if matches := authorPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}
