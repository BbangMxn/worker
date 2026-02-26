// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strconv"
	"strings"

	"worker_server/core/domain"
	"worker_server/core/service/classification"
)

// =============================================================================
// Developer Tools Base Parser
// =============================================================================
//
// Common patterns for GitHub, GitLab, Bitbucket:
//   - PR/MR: Review Request, Approved, Changes Requested, Merged, Closed
//   - Issue: Assigned, Mentioned, Comment, Closed
//   - CI/CD: Build/Pipeline Failed, Passed
//   - Security: Dependabot, Secret Scanning, Code Scanning
//   - Repository: Release, Team Mention, Watching

// DevToolsBaseParser provides common functionality for developer tool parsers.
type DevToolsBaseParser struct {
	service  SaaSService
	category SaaSCategory
}

// NewDevToolsBaseParser creates a new base parser.
func NewDevToolsBaseParser(service SaaSService) *DevToolsBaseParser {
	return &DevToolsBaseParser{
		service:  service,
		category: CategoryDevTools,
	}
}

// Service returns the service.
func (p *DevToolsBaseParser) Service() SaaSService {
	return p.service
}

// Category returns the category.
func (p *DevToolsBaseParser) Category() SaaSCategory {
	return p.category
}

// =============================================================================
// Common Regex Patterns
// =============================================================================

var (
	// Subject patterns
	devSubjectRepoPattern   = regexp.MustCompile(`^\[([^\]]+)\]`)
	devSubjectNumberPattern = regexp.MustCompile(`[#!](\d+)|\(#(\d+)\)|\(!(\d+)\)`)

	// URL patterns
	devGitHubURLPattern = regexp.MustCompile(`https://github\.com/([^/]+)/([^/]+)/(pull|issues|commit|compare)/(\d+|[a-f0-9]+)`)
	devGitLabURLPattern = regexp.MustCompile(`https://[^/]*gitlab[^/]*/([^/]+(?:/[^/]+)*)/(-/)?(?:issues|merge_requests)/(\d+)`)

	// Content patterns
	devMentionPattern = regexp.MustCompile(`@([a-zA-Z0-9](?:-?[a-zA-Z0-9]){0,38})`)
	devCommitPattern  = regexp.MustCompile(`\b([a-f0-9]{7,40})\b`)
	devBranchPattern  = regexp.MustCompile(`(?i)branch[:\s]+['\x60]?([^\s'\x60\n]+)['\x60]?`)

	// Security patterns
	devCVEPattern     = regexp.MustCompile(`CVE-\d{4}-\d+`)
	devPackagePattern = regexp.MustCompile(`(?i)package[:\s]*([^\n<]+)`)
	devVersionPattern = regexp.MustCompile(`(?i)vulnerable\s+version[s]?[:\s]*([^\n<]+)`)
	devPatchedPattern = regexp.MustCompile(`(?i)patched\s+version[s]?[:\s]*([^\n<]+)`)
)

// =============================================================================
// Common Extraction Methods
// =============================================================================

// ExtractRepoFromSubject extracts repository name from subject [owner/repo].
func (p *DevToolsBaseParser) ExtractRepoFromSubject(subject string) string {
	if matches := devSubjectRepoPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// ExtractNumberFromSubject extracts issue/PR number from subject.
func (p *DevToolsBaseParser) ExtractNumberFromSubject(subject string) int {
	matches := devSubjectNumberPattern.FindStringSubmatch(subject)
	if len(matches) > 1 {
		for _, m := range matches[1:] {
			if m != "" {
				if num, err := strconv.Atoi(m); err == nil {
					return num
				}
			}
		}
	}
	return 0
}

// ExtractTitleFromSubject extracts clean title from subject.
func (p *DevToolsBaseParser) ExtractTitleFromSubject(subject, repo string, number int) string {
	title := subject

	// Remove [repo] prefix
	if repo != "" {
		title = strings.TrimPrefix(title, "["+repo+"] ")
	}
	title = devSubjectRepoPattern.ReplaceAllString(title, "")

	// Remove (#123) or !123 suffix
	title = devSubjectNumberPattern.ReplaceAllString(title, "")

	// Remove Re: prefix
	title = strings.TrimPrefix(title, "Re: ")

	return strings.TrimSpace(title)
}

// ExtractMentions extracts @mentions from text.
func (p *DevToolsBaseParser) ExtractMentions(text string) []string {
	matches := devMentionPattern.FindAllStringSubmatch(text, -1)
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

// ExtractCommitHash extracts commit hash from text.
func (p *DevToolsBaseParser) ExtractCommitHash(text string) string {
	if matches := devCommitPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// ExtractBranch extracts branch name from text.
func (p *DevToolsBaseParser) ExtractBranch(text string) string {
	if matches := devBranchPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// ExtractSecurityInfo extracts CVE and package info from text.
func (p *DevToolsBaseParser) ExtractSecurityInfo(data *ExtractedData, text string) {
	if match := devCVEPattern.FindString(text); match != "" {
		data.CVE = match
	}
	if matches := devPackagePattern.FindStringSubmatch(text); len(matches) >= 2 {
		data.Package = strings.TrimSpace(matches[1])
	}
	if matches := devVersionPattern.FindStringSubmatch(text); len(matches) >= 2 {
		data.VulnVersion = strings.TrimSpace(matches[1])
	}
	if matches := devPatchedPattern.FindStringSubmatch(text); len(matches) >= 2 {
		data.PatchedVersion = strings.TrimSpace(matches[1])
	}
}

// =============================================================================
// Common Priority Calculation
// =============================================================================

// DevToolsPriorityConfig holds priority calculation parameters.
type DevToolsPriorityConfig struct {
	DomainScore   float64
	ReasonScore   float64
	RelationScore float64
	SeverityScore float64
}

// CalculateDevToolsPriority calculates priority for developer tools.
func (p *DevToolsBaseParser) CalculateDevToolsPriority(config DevToolsPriorityConfig) (domain.Priority, float64) {
	score := classification.CalculatePriority(
		config.DomainScore,
		config.ReasonScore,
		config.RelationScore,
		config.SeverityScore,
	)
	return p.ScoreToPriority(score), score
}

// GetReasonScoreForEvent returns reason and relation scores for common events.
func (p *DevToolsBaseParser) GetReasonScoreForEvent(event DevToolsEventType) (reasonScore, relationScore float64) {
	switch event {
	// Direct involvement - high priority
	case DevEventReviewRequested:
		return classification.ReasonScoreReviewRequested, classification.RelationScoreDirect
	case DevEventIssueMentioned:
		return classification.ReasonScoreMention, classification.RelationScoreDirect
	case DevEventIssueAssigned:
		return classification.ReasonScoreAssign, classification.RelationScoreDirect
	case DevEventTeamMention:
		return classification.ReasonScoreTeamMention, classification.RelationScoreTeam

	// Author/owner activity
	case DevEventReviewApproved, DevEventReviewChangesReq:
		return classification.ReasonScoreAuthor, classification.RelationScoreDirect
	case DevEventPRMerged:
		return classification.ReasonScoreAuthor, classification.RelationScoreDirect

	// CI/CD
	case DevEventCIFailed:
		return classification.ReasonScoreCIFailed, classification.RelationScoreProject
	case DevEventCIPassed:
		return classification.ReasonScoreCIPassed, classification.RelationScoreProject

	// Passive watching - low priority
	case DevEventIssueComment, DevEventPRComment:
		return classification.ReasonScoreComment, classification.RelationScoreWatching
	case DevEventWatching, DevEventOwnActivity:
		return classification.ReasonScoreSubscribed, classification.RelationScoreWatching
	case DevEventPRPush:
		return classification.ReasonScorePush, classification.RelationScoreWatching

	// Security alerts - high priority
	case DevEventSecurityAlert, DevEventDependabotAlert, DevEventSecretScanAlert:
		return classification.ReasonScoreAlertCritical, classification.RelationScoreProject

	default:
		return 0.05, classification.RelationScoreWatching
	}
}

// ScoreToPriority converts score to Priority.
func (p *DevToolsBaseParser) ScoreToPriority(score float64) domain.Priority {
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

// DetermineDevToolsCategory determines category based on event type.
func (p *DevToolsBaseParser) DetermineDevToolsCategory(event DevToolsEventType) (domain.EmailCategory, *domain.EmailSubCategory) {
	devSubCat := domain.SubCategoryDeveloper
	alertSubCat := domain.SubCategoryAlert
	notifSubCat := domain.SubCategoryNotification

	switch event {
	// Security alerts → Work + Alert
	case DevEventSecurityAlert, DevEventDependabotAlert, DevEventSecretScanAlert:
		return domain.CategoryWork, &alertSubCat

	// Direct involvement → Work
	case DevEventReviewRequested, DevEventIssueMentioned, DevEventIssueAssigned,
		DevEventTeamMention, DevEventReviewApproved, DevEventReviewChangesReq,
		DevEventCIFailed:
		return domain.CategoryWork, &devSubCat

	// Passive watching → Notification
	case DevEventWatching, DevEventOwnActivity, DevEventCIPassed, DevEventPRPush:
		return domain.CategoryNotification, &notifSubCat

	// State changes → Notification
	case DevEventIssueClosed, DevEventPRClosed, DevEventPRMerged:
		return domain.CategoryNotification, &notifSubCat

	default:
		return domain.CategoryWork, &devSubCat
	}
}

// =============================================================================
// Common Action Item Generation
// =============================================================================

// GenerateDevToolsActionItems generates action items based on event type.
func (p *DevToolsBaseParser) GenerateDevToolsActionItems(event DevToolsEventType, data *ExtractedData) []ActionItem {
	var items []ActionItem

	switch event {
	case DevEventReviewRequested:
		items = append(items, ActionItem{
			Type:     ActionReview,
			Title:    p.FormatActionTitle("Review", data.PRNumber, data.MRNumber, data.Title),
			URL:      data.URL,
			Priority: "high",
		})

	case DevEventReviewChangesReq:
		items = append(items, ActionItem{
			Type:     ActionFix,
			Title:    p.FormatActionTitle("Address feedback for", data.PRNumber, data.MRNumber, data.Title),
			URL:      data.URL,
			Priority: "high",
		})

	case DevEventIssueAssigned:
		items = append(items, ActionItem{
			Type:     ActionFix,
			Title:    p.FormatActionTitle("Work on", data.IssueNumber, 0, data.Title),
			URL:      data.URL,
			Priority: "medium",
		})

	case DevEventIssueMentioned, DevEventTeamMention:
		items = append(items, ActionItem{
			Type:     ActionRespond,
			Title:    p.FormatActionTitle("Respond to mention in", data.IssueNumber, data.PRNumber, data.Title),
			URL:      data.URL,
			Priority: "medium",
		})

	case DevEventCIFailed:
		items = append(items, ActionItem{
			Type:        ActionFix,
			Title:       "Fix CI failure: " + data.WorkflowName,
			Description: "Branch: " + data.Branch,
			URL:         data.URL,
			Priority:    "high",
		})

	case DevEventSecurityAlert, DevEventDependabotAlert:
		priority := "high"
		if data.Severity == "critical" {
			priority = "urgent"
		}
		items = append(items, ActionItem{
			Type:        ActionFix,
			Title:       "Fix security vulnerability in " + data.Package,
			Description: "CVE: " + data.CVE + ", Severity: " + data.Severity,
			URL:         data.URL,
			Priority:    priority,
		})
	}

	return items
}

// FormatActionTitle formats action title with PR/MR/Issue number.
func (p *DevToolsBaseParser) FormatActionTitle(prefix string, issueNum, prNum int, title string) string {
	var numStr string
	if prNum > 0 {
		if p.service == ServiceGitLab {
			numStr = " !" + strconv.Itoa(prNum)
		} else {
			numStr = " #" + strconv.Itoa(prNum)
		}
	} else if issueNum > 0 {
		numStr = " #" + strconv.Itoa(issueNum)
	}

	if title != "" {
		return prefix + numStr + ": " + title
	}
	return prefix + numStr
}

// =============================================================================
// Common Entity Extraction
// =============================================================================

// GenerateDevToolsEntities generates entities from extracted data.
func (p *DevToolsBaseParser) GenerateDevToolsEntities(data *ExtractedData, baseURL string) []Entity {
	var entities []Entity

	// Repository
	if data.Repository != "" {
		entities = append(entities, Entity{
			Type: EntityRepository,
			ID:   data.Repository,
			Name: data.Repository,
			URL:  baseURL + "/" + data.Repository,
		})
	}

	// PR
	if data.PRNumber > 0 && data.Repository != "" {
		prPath := "/pull/"
		if p.service == ServiceGitLab {
			prPath = "/-/merge_requests/"
		}
		entities = append(entities, Entity{
			Type: EntityPR,
			ID:   strconv.Itoa(data.PRNumber),
			Name: p.FormatPRName(data.PRNumber),
			URL:  baseURL + "/" + data.Repository + prPath + strconv.Itoa(data.PRNumber),
		})
	}

	// MR (GitLab)
	if data.MRNumber > 0 && data.Repository != "" {
		entities = append(entities, Entity{
			Type: EntityMR,
			ID:   strconv.Itoa(data.MRNumber),
			Name: "MR !" + strconv.Itoa(data.MRNumber),
			URL:  baseURL + "/" + data.Repository + "/-/merge_requests/" + strconv.Itoa(data.MRNumber),
		})
	}

	// Issue
	if data.IssueNumber > 0 && data.Repository != "" {
		issuePath := "/issues/"
		if p.service == ServiceGitLab {
			issuePath = "/-/issues/"
		}
		entities = append(entities, Entity{
			Type: EntityIssue,
			ID:   strconv.Itoa(data.IssueNumber),
			Name: "Issue #" + strconv.Itoa(data.IssueNumber),
			URL:  baseURL + "/" + data.Repository + issuePath + strconv.Itoa(data.IssueNumber),
		})
	}

	// Author
	if data.Author != "" {
		entities = append(entities, Entity{
			Type: EntityUser,
			ID:   data.Author,
			Name: data.Author,
			URL:  baseURL + "/" + data.Author,
		})
	}

	// Mentioned users
	for _, mention := range data.Mentions {
		if mention != data.Author {
			entities = append(entities, Entity{
				Type: EntityUser,
				ID:   mention,
				Name: mention,
				URL:  baseURL + "/" + mention,
			})
		}
	}

	return entities
}

// FormatPRName formats PR name based on service.
func (p *DevToolsBaseParser) FormatPRName(num int) string {
	if p.service == ServiceGitLab {
		return "MR !" + strconv.Itoa(num)
	}
	return "PR #" + strconv.Itoa(num)
}
