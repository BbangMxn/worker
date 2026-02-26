// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/domain"
	"worker_server/core/service/classification"
)

// =============================================================================
// Project Management Base Parser
// =============================================================================
//
// Common patterns for Jira, Linear, Asana, Trello:
//   - Issue/Task: Created, Assigned, Mentioned, Comment, Updated, Closed
//   - Sprint/Cycle: Started, Ended
//   - Status: Changed, Priority Changed
//   - Due Date: Approaching, Overdue

// ProjectMgmtBaseParser provides common functionality for project management tool parsers.
type ProjectMgmtBaseParser struct {
	service  SaaSService
	category SaaSCategory
}

// NewProjectMgmtBaseParser creates a new base parser.
func NewProjectMgmtBaseParser(service SaaSService) *ProjectMgmtBaseParser {
	return &ProjectMgmtBaseParser{
		service:  service,
		category: CategoryProjectMgmt,
	}
}

// Service returns the service.
func (p *ProjectMgmtBaseParser) Service() SaaSService {
	return p.service
}

// Category returns the category.
func (p *ProjectMgmtBaseParser) Category() SaaSCategory {
	return p.category
}

// =============================================================================
// Common Regex Patterns
// =============================================================================

var (
	// Issue key patterns
	pmJiraKeyPattern   = regexp.MustCompile(`([A-Z][A-Z0-9]+-\d+)`)
	pmLinearKeyPattern = regexp.MustCompile(`([A-Z]+-\d+)`)

	// URL patterns
	pmJiraURLPattern   = regexp.MustCompile(`https?://[^/]+/(?:browse|jira/browse)/([A-Z][A-Z0-9]+-\d+)`)
	pmLinearURLPattern = regexp.MustCompile(`https://linear\.app/[^\s<>"]+`)
	pmAsanaURLPattern  = regexp.MustCompile(`https://app\.asana\.com/\d+/\d+/\d+`)

	// Common content patterns
	pmAssigneePattern = regexp.MustCompile(`(?i)(?:assigned to|assignee)[:\s]*@?([^\n<,]+)`)
	pmReporterPattern = regexp.MustCompile(`(?i)reporter[:\s]*@?([^\n<,]+)`)
	pmDueDatePattern  = regexp.MustCompile(`(?i)due[:\s]*(\d{1,2}[-/]\w{3}[-/]\d{2,4}|\d{4}-\d{2}-\d{2}|\w+ \d{1,2},? \d{4})`)
	pmPriorityPattern = regexp.MustCompile(`(?i)priority[:\s]*(Urgent|Highest|High|Medium|Normal|Low|Lowest|Minor|Blocker|Critical|Major|Trivial|No Priority)`)
	pmStatusPattern   = regexp.MustCompile(`(?i)status[:\s]*([\w\s-]+)`)
	pmMentionPattern  = regexp.MustCompile(`@([a-zA-Z0-9_.-]+)`)
)

// =============================================================================
// Common Extraction Methods
// =============================================================================

// ExtractIssueKey extracts issue key from text (e.g., PROJ-123).
func (p *ProjectMgmtBaseParser) ExtractIssueKey(text string) string {
	var pattern *regexp.Regexp
	switch p.service {
	case ServiceJira:
		pattern = pmJiraKeyPattern
	case ServiceLinear:
		pattern = pmLinearKeyPattern
	default:
		pattern = pmJiraKeyPattern
	}

	if matches := pattern.FindStringSubmatch(text); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// ExtractProjectFromKey extracts project code from issue key.
func (p *ProjectMgmtBaseParser) ExtractProjectFromKey(issueKey string) string {
	if idx := strings.Index(issueKey, "-"); idx > 0 {
		return issueKey[:idx]
	}
	return ""
}

// ExtractTitle extracts clean title from subject.
func (p *ProjectMgmtBaseParser) ExtractTitle(subject, issueKey string) string {
	title := subject

	// Remove common prefixes
	prefixes := []string{"[JIRA] ", "Re: "}
	for _, prefix := range prefixes {
		title = strings.TrimPrefix(title, prefix)
	}

	// Remove issue key patterns
	if issueKey != "" {
		title = strings.ReplaceAll(title, "("+issueKey+")", "")
		title = strings.ReplaceAll(title, "["+issueKey+"]", "")
		title = strings.TrimPrefix(title, issueKey+" ")
		title = strings.TrimPrefix(title, issueKey+": ")
	}

	return strings.TrimSpace(title)
}

// ExtractAssignee extracts assignee from text.
func (p *ProjectMgmtBaseParser) ExtractAssignee(text string) string {
	if matches := pmAssigneePattern.FindStringSubmatch(text); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// ExtractReporter extracts reporter from text.
func (p *ProjectMgmtBaseParser) ExtractReporter(text string) string {
	if matches := pmReporterPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// ExtractDueDate extracts due date from text.
func (p *ProjectMgmtBaseParser) ExtractDueDate(text string) string {
	if matches := pmDueDatePattern.FindStringSubmatch(text); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// ExtractPriority extracts priority from text.
func (p *ProjectMgmtBaseParser) ExtractPriority(text string) string {
	if matches := pmPriorityPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// ExtractStatus extracts status from text.
func (p *ProjectMgmtBaseParser) ExtractStatus(text string) string {
	if matches := pmStatusPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// ExtractMentions extracts @mentions from text.
func (p *ProjectMgmtBaseParser) ExtractMentions(text string) []string {
	matches := pmMentionPattern.FindAllStringSubmatch(text, -1)
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

// =============================================================================
// Common Priority Calculation
// =============================================================================

// ProjectMgmtPriorityConfig holds priority calculation parameters.
type ProjectMgmtPriorityConfig struct {
	DomainScore   float64
	ActionScore   float64 // based on action type (assigned, mentioned, etc.)
	RelationScore float64
	ToolPriority  string // tool-specific priority (Urgent, High, etc.)
}

// CalculateProjectMgmtPriority calculates priority for project management tools.
func (p *ProjectMgmtBaseParser) CalculateProjectMgmtPriority(config ProjectMgmtPriorityConfig) (domain.Priority, float64) {
	// Adjust action score based on tool priority
	adjustedActionScore := config.ActionScore
	switch strings.ToLower(config.ToolPriority) {
	case "urgent", "highest", "blocker":
		adjustedActionScore += 0.25
	case "high", "critical":
		adjustedActionScore += 0.15
	case "medium", "major", "normal":
		adjustedActionScore += 0.05
	case "low", "minor":
		adjustedActionScore -= 0.05
	case "lowest", "trivial", "no priority":
		adjustedActionScore -= 0.10
	}

	score := classification.CalculatePriority(
		config.DomainScore,
		adjustedActionScore,
		config.RelationScore,
		0,
	)
	return p.ScoreToPriority(score), score
}

// GetActionScoreForEvent returns action and relation scores for common events.
func (p *ProjectMgmtBaseParser) GetActionScoreForEvent(event ProjectMgmtEventType) (actionScore, relationScore float64) {
	switch event {
	// Direct involvement - high priority
	case PMEventIssueAssigned:
		return classification.ReasonScoreAssign, classification.RelationScoreDirect
	case PMEventIssueMentioned:
		return classification.ReasonScoreMention, classification.RelationScoreDirect

	// Comments and updates
	case PMEventIssueComment:
		return classification.ReasonScoreComment, classification.RelationScoreProject
	case PMEventIssueUpdated:
		return 0.10, classification.RelationScoreProject
	case PMEventIssueCreated:
		return 0.15, classification.RelationScoreProject

	// Due dates
	case PMEventIssueDue:
		return 0.25, classification.RelationScoreDirect

	// Closed/completed
	case PMEventIssueClosed:
		return 0.05, classification.RelationScoreWatching
	case PMEventIssueReopened:
		return 0.15, classification.RelationScoreProject

	// Sprint/Cycle events
	case PMEventSprintStarted, PMEventCycleStarted:
		return 0.12, classification.RelationScoreTeam
	case PMEventSprintEnded, PMEventCycleEnded:
		return 0.08, classification.RelationScoreTeam

	// Status changes
	case PMEventStatusChanged:
		return 0.08, classification.RelationScoreWatching
	case PMEventPriorityChanged:
		return 0.10, classification.RelationScoreProject

	default:
		return 0.05, classification.RelationScoreWatching
	}
}

// ScoreToPriority converts score to Priority.
func (p *ProjectMgmtBaseParser) ScoreToPriority(score float64) domain.Priority {
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

// DetermineProjectMgmtCategory determines category based on event type.
func (p *ProjectMgmtBaseParser) DetermineProjectMgmtCategory(event ProjectMgmtEventType) (domain.EmailCategory, *domain.EmailSubCategory) {
	devSubCat := domain.SubCategoryDeveloper
	notifSubCat := domain.SubCategoryNotification

	switch event {
	// Direct involvement → Work
	case PMEventIssueAssigned, PMEventIssueMentioned, PMEventIssueDue:
		return domain.CategoryWork, &devSubCat

	// Sprint/Cycle → Work
	case PMEventSprintStarted, PMEventSprintEnded, PMEventCycleStarted, PMEventCycleEnded:
		return domain.CategoryWork, &devSubCat

	// Closed items → Notification
	case PMEventIssueClosed:
		return domain.CategoryNotification, &notifSubCat

	// Updates and status changes → Notification (lower priority)
	case PMEventStatusChanged, PMEventPriorityChanged:
		return domain.CategoryNotification, &notifSubCat

	default:
		return domain.CategoryWork, &devSubCat
	}
}

// =============================================================================
// Common Action Item Generation
// =============================================================================

// GenerateProjectMgmtActionItems generates action items based on event type.
func (p *ProjectMgmtBaseParser) GenerateProjectMgmtActionItems(event ProjectMgmtEventType, data *ExtractedData) []ActionItem {
	var items []ActionItem

	issueRef := data.IssueKey
	if issueRef == "" && data.Title != "" {
		issueRef = data.Title
	}

	switch event {
	case PMEventIssueAssigned:
		priority := "medium"
		if toolPriority := data.Extra["tool_priority"]; toolPriority != nil {
			tp := strings.ToLower(toolPriority.(string))
			if tp == "urgent" || tp == "highest" || tp == "blocker" || tp == "critical" {
				priority = "high"
			}
		}
		items = append(items, ActionItem{
			Type:     ActionFix,
			Title:    "Work on " + issueRef,
			URL:      data.URL,
			Priority: priority,
			DueDate:  data.DueDate,
		})

	case PMEventIssueMentioned:
		items = append(items, ActionItem{
			Type:     ActionRespond,
			Title:    "Respond to mention in " + issueRef,
			URL:      data.URL,
			Priority: "medium",
		})

	case PMEventIssueComment:
		items = append(items, ActionItem{
			Type:     ActionRead,
			Title:    "Review comment on " + issueRef,
			URL:      data.URL,
			Priority: "low",
		})

	case PMEventIssueDue:
		items = append(items, ActionItem{
			Type:     ActionFix,
			Title:    "Issue due soon: " + issueRef,
			URL:      data.URL,
			Priority: "high",
			DueDate:  data.DueDate,
		})

	case PMEventSprintStarted, PMEventCycleStarted:
		items = append(items, ActionItem{
			Type:     ActionRead,
			Title:    "Review sprint/cycle plan: " + data.Sprint,
			URL:      data.URL,
			Priority: "medium",
		})
	}

	return items
}

// =============================================================================
// Common Entity Extraction
// =============================================================================

// GenerateProjectMgmtEntities generates entities from extracted data.
func (p *ProjectMgmtBaseParser) GenerateProjectMgmtEntities(data *ExtractedData) []Entity {
	var entities []Entity

	// Project
	if data.Project != "" {
		entities = append(entities, Entity{
			Type: EntityProject,
			ID:   data.Project,
			Name: data.Project,
		})
	}

	// Team
	if data.Team != "" {
		entities = append(entities, Entity{
			Type: EntityTeam,
			ID:   data.Team,
			Name: data.Team,
		})
	}

	// Issue
	if data.IssueKey != "" {
		entities = append(entities, Entity{
			Type: EntityIssue,
			ID:   data.IssueKey,
			Name: data.IssueKey,
			URL:  data.URL,
		})
	}

	// Assignee
	if data.Assignee != "" {
		entities = append(entities, Entity{
			Type: EntityUser,
			ID:   data.Assignee,
			Name: data.Assignee,
		})
	}

	// Author
	if data.Author != "" && data.Author != data.Assignee {
		entities = append(entities, Entity{
			Type: EntityUser,
			ID:   data.Author,
			Name: data.Author,
		})
	}

	return entities
}
