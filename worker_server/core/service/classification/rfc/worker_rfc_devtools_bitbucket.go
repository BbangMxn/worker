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
// Bitbucket Parser
// =============================================================================
//
// Bitbucket email patterns:
//   - From: notifications-noreply@bitbucket.org, noreply@bitbucket.org
//   - Reply-To: pullrequests-reply@bitbucket.org
//   - List-Id: <repo.workspace.bitbucket.org>
//
// Subject patterns:
//   - [workspace/repo] Title (PR #123)
//   - [workspace/repo] user approved pull request #123: Title
//   - [workspace/repo] user merged pull request #123: Title
//   - [workspace/repo] branch updated
//   - Pipeline #123 failed for workspace/repo
//
// NOTE: Bitbucket does NOT use custom X-Bitbucket-* headers like GitHub/GitLab.
// Classification relies on subject patterns and from address.

// BitbucketParser parses Bitbucket notification emails.
type BitbucketParser struct {
	*DevToolsBaseParser
}

// NewBitbucketParser creates a new Bitbucket parser.
func NewBitbucketParser() *BitbucketParser {
	return &BitbucketParser{
		DevToolsBaseParser: NewDevToolsBaseParser(ServiceBitbucket),
	}
}

// Bitbucket-specific regex patterns
var (
	// Subject patterns
	bitbucketSubjectRepoPattern = regexp.MustCompile(`^\[([^\]]+/[^\]]+)\]`)
	bitbucketPRPattern          = regexp.MustCompile(`(?i)(?:PR\s*#?|pull\s*request\s*#?)(\d+)`)
	bitbucketIssuePattern       = regexp.MustCompile(`(?:^|\s)#(\d+)(?:\s|:|$)`)
	bitbucketPipelinePattern    = regexp.MustCompile(`(?i)(?:pipeline|build)\s*#?(\d+)`)

	// Action patterns in subject
	bitbucketApprovedPattern        = regexp.MustCompile(`(?i)\bapproved\b`)
	bitbucketMergedPattern          = regexp.MustCompile(`(?i)\bmerged\b`)
	bitbucketDeclinedPattern        = regexp.MustCompile(`(?i)\bdeclined\b`)
	bitbucketChangesReqPattern      = regexp.MustCompile(`(?i)(?:changes?\s*requested|requested\s*changes?)`)
	bitbucketCommentedPattern       = regexp.MustCompile(`(?i)\bcommented\b`)
	bitbucketPushedPattern          = regexp.MustCompile(`(?i)(?:\bpushed\b|\bupdated\b|new\s+commit)`)
	bitbucketFailedPattern          = regexp.MustCompile(`(?i)\bfailed\b`)
	bitbucketPassedPattern          = regexp.MustCompile(`(?i)(?:\bpassed\b|\bsucceeded\b|\bsuccess\b)`)
	bitbucketInvitedPattern         = regexp.MustCompile(`(?i)(?:\binvited\b|\baccess\b)`)
	bitbucketReviewRequestedPattern = regexp.MustCompile(`(?i)(?:review\s*requested|added.*reviewer)`)

	// URL pattern
	bitbucketURLPattern = regexp.MustCompile(`https://bitbucket\.org/([^/]+)/([^/]+)(?:/(?:pull-requests|issues|commits|pipelines)/(\d+))?`)
)

// CanParse checks if this parser can handle the email.
func (p *BitbucketParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)

	// Check from address
	if strings.Contains(fromLower, "@bitbucket.org") {
		return true
	}

	// Check List-Id header for bitbucket.org
	if rawHeaders != nil {
		listID := strings.ToLower(rawHeaders["List-Id"])
		if strings.Contains(listID, "bitbucket.org") {
			return true
		}
	}

	return false
}

// Parse extracts structured data from Bitbucket emails.
func (p *BitbucketParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	subject := ""
	if input.Message != nil {
		subject = input.Message.Subject
	}

	// Detect event type from subject
	event := p.detectEvent(subject, input)

	// Extract data
	data := p.extractData(input)

	// Calculate priority
	reasonScore, relationScore := p.GetReasonScoreForEvent(event)
	priority, score := p.CalculateDevToolsPriority(DevToolsPriorityConfig{
		DomainScore:   classification.DomainScoreGitHub, // Similar to GitHub
		ReasonScore:   reasonScore,
		RelationScore: relationScore,
	})

	// Determine category
	category, subCat := p.DetermineDevToolsCategory(event)

	// Generate action items
	actionItems := p.GenerateDevToolsActionItems(event, data)

	// Generate entities
	entities := p.GenerateDevToolsEntities(data, "https://bitbucket.org")

	return &ParsedEmail{
		Category:      CategoryDevTools,
		Service:       ServiceBitbucket,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:bitbucket:" + string(event),
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"bitbucket", "event:" + string(event)},
	}, nil
}

// detectEvent detects the Bitbucket event from subject and content.
func (p *BitbucketParser) detectEvent(subject string, input *ParserInput) DevToolsEventType {
	subjectLower := strings.ToLower(subject)

	// Check for pipeline/build notifications first
	if bitbucketPipelinePattern.MatchString(subject) || strings.Contains(subjectLower, "pipeline") || strings.Contains(subjectLower, "build") {
		if bitbucketFailedPattern.MatchString(subject) {
			return DevEventCIFailed
		}
		if bitbucketPassedPattern.MatchString(subject) {
			return DevEventCIPassed
		}
	}

	// Check for PR-related events
	isPR := bitbucketPRPattern.MatchString(subject) || strings.Contains(subjectLower, "pull request")

	if isPR {
		switch {
		case bitbucketApprovedPattern.MatchString(subject):
			return DevEventReviewApproved
		case bitbucketMergedPattern.MatchString(subject):
			return DevEventPRMerged
		case bitbucketDeclinedPattern.MatchString(subject):
			return DevEventPRClosed
		case bitbucketChangesReqPattern.MatchString(subject):
			return DevEventReviewChangesReq
		case bitbucketCommentedPattern.MatchString(subject):
			return DevEventPRComment
		case bitbucketReviewRequestedPattern.MatchString(subject):
			return DevEventReviewRequested
		case bitbucketPushedPattern.MatchString(subject):
			return DevEventPRPush
		}
		// Default PR event
		return DevEventPRComment
	}

	// Check for issue-related events
	if bitbucketIssuePattern.MatchString(subject) || strings.Contains(subjectLower, "issue") {
		switch {
		case bitbucketCommentedPattern.MatchString(subject):
			return DevEventIssueComment
		case strings.Contains(subjectLower, "created"):
			return DevEventIssueCreated
		case strings.Contains(subjectLower, "closed"):
			return DevEventIssueClosed
		case strings.Contains(subjectLower, "assigned"):
			return DevEventIssueAssigned
		}
		return DevEventIssueComment
	}

	// Check for push/commit events
	if bitbucketPushedPattern.MatchString(subject) {
		return DevEventPRPush
	}

	// Check for access/invitation events
	if bitbucketInvitedPattern.MatchString(subject) {
		return DevEventWatching
	}

	// Default
	return DevEventIssueComment
}

// extractData extracts structured data from the email.
func (p *BitbucketParser) extractData(input *ParserInput) *ExtractedData {
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

	// Extract repository from subject [workspace/repo]
	if matches := bitbucketSubjectRepoPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		data.Repository = matches[1]
	}

	// Extract PR number
	if matches := bitbucketPRPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			data.PRNumber = num
		}
	}

	// Extract issue number (only if not a PR)
	if data.PRNumber == 0 {
		if matches := bitbucketIssuePattern.FindStringSubmatch(subject); len(matches) >= 2 {
			if num, err := strconv.Atoi(matches[1]); err == nil {
				data.IssueNumber = num
			}
		}
	}

	// Extract pipeline/build number
	if matches := bitbucketPipelinePattern.FindStringSubmatch(subject); len(matches) >= 2 {
		data.PipelineID = matches[1]
	}

	// Extract title
	data.Title = p.extractBitbucketTitle(subject, data.Repository, data.PRNumber)

	// Extract URL from body
	if matches := bitbucketURLPattern.FindStringSubmatch(combined); len(matches) >= 1 {
		data.URL = matches[0]
		// Also extract workspace/repo from URL if not found in subject
		if data.Repository == "" && len(matches) >= 3 {
			data.Repository = matches[1] + "/" + matches[2]
		}
	}

	// Extract mentions
	data.Mentions = p.ExtractMentions(combined)

	// Extract branch
	data.Branch = p.ExtractBranch(combined)

	// Extract commit hash
	data.CommitHash = p.ExtractCommitHash(combined)

	// Extract author from body
	data.Author = p.extractBitbucketAuthor(subject)

	return data
}

// extractBitbucketTitle extracts clean title from subject.
func (p *BitbucketParser) extractBitbucketTitle(subject string, repo string, prNumber int) string {
	title := subject

	// Remove [workspace/repo] prefix
	title = bitbucketSubjectRepoPattern.ReplaceAllString(title, "")

	// Remove PR number patterns
	title = bitbucketPRPattern.ReplaceAllString(title, "")

	// Remove action verbs
	actionPatterns := []string{
		"approved pull request",
		"merged pull request",
		"declined pull request",
		"commented on pull request",
		"pushed to",
		"updated",
	}
	titleLower := strings.ToLower(title)
	for _, pattern := range actionPatterns {
		if idx := strings.Index(titleLower, pattern); idx >= 0 {
			// Try to extract the actual title after the action
			afterAction := title[idx+len(pattern):]
			if colonIdx := strings.Index(afterAction, ":"); colonIdx >= 0 {
				title = strings.TrimSpace(afterAction[colonIdx+1:])
				break
			}
		}
	}

	// Remove Re: prefix
	title = strings.TrimPrefix(title, "Re: ")

	// Remove (PR #123) suffix
	title = regexp.MustCompile(`\s*\(PR\s*#?\d+\)\s*$`).ReplaceAllString(title, "")

	return strings.TrimSpace(title)
}

// extractBitbucketAuthor extracts author from subject patterns like "user approved...".
func (p *BitbucketParser) extractBitbucketAuthor(subject string) string {
	// Pattern: [repo] username action...
	// After removing repo prefix, first word is often the username
	title := bitbucketSubjectRepoPattern.ReplaceAllString(subject, "")
	title = strings.TrimSpace(title)

	// Split and get first word (potential username)
	parts := strings.Fields(title)
	if len(parts) > 1 {
		// Check if second word is an action verb
		actionVerbs := []string{"approved", "merged", "declined", "commented", "pushed", "created", "updated", "requested"}
		secondWord := strings.ToLower(parts[1])
		for _, verb := range actionVerbs {
			if secondWord == verb {
				return parts[0]
			}
		}
	}

	return ""
}
