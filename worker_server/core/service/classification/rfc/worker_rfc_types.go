// Package rfc implements domain-specific email RFC parsers for SaaS tools.
//
// Architecture:
//
//	┌─────────────────────────────────────────────────────────────────────────┐
//	│                         SaaS Category Base Parsers                       │
//	│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐   │
//	│  │  DevTools    │ │  ProjectMgmt │ │Communication │ │  Monitoring  │   │
//	│  │    Base      │ │     Base     │ │     Base     │ │     Base     │   │
//	│  └──────┬───────┘ └──────┬───────┘ └──────┬───────┘ └──────┬───────┘   │
//	│         │                │                │                │           │
//	│    ┌────┴────┐      ┌────┴────┐      ┌────┴────┐      ┌────┴────┐     │
//	│    │ GitHub  │      │  Jira   │      │  Slack  │      │ Sentry  │     │
//	│    │ GitLab  │      │ Linear  │      │  Teams  │      │PagerDuty│     │
//	│    │Bitbucket│      │  Asana  │      │ Discord │      │ Datadog │     │
//	│    └─────────┘      └─────────┘      └─────────┘      └─────────┘     │
//	└─────────────────────────────────────────────────────────────────────────┘
package rfc

import (
	"worker_server/core/domain"
	"worker_server/core/port/out"
)

// =============================================================================
// SaaS Category
// =============================================================================

// SaaSCategory represents a category of SaaS tools.
type SaaSCategory string

const (
	CategoryDevTools      SaaSCategory = "dev_tools"     // GitHub, GitLab, Bitbucket
	CategoryProjectMgmt   SaaSCategory = "project_mgmt"  // Jira, Linear, Asana, Trello
	CategoryCommunication SaaSCategory = "communication" // Slack, Teams, Discord
	CategoryDocumentation SaaSCategory = "documentation" // Confluence, Notion, Google Docs
	CategoryMonitoring    SaaSCategory = "monitoring"    // Sentry, PagerDuty, Datadog, OpsGenie
	CategoryDeployment    SaaSCategory = "deployment"    // Vercel, Netlify, AWS, GCP
	CategoryFinance       SaaSCategory = "finance"       // Stripe, PayPal
	CategoryUnknown       SaaSCategory = "unknown"
)

// =============================================================================
// SaaS Service
// =============================================================================

// SaaSService represents a specific SaaS service.
type SaaSService string

// Developer Tools
const (
	ServiceGitHub    SaaSService = "github"
	ServiceGitLab    SaaSService = "gitlab"
	ServiceBitbucket SaaSService = "bitbucket"
)

// Project Management
const (
	ServiceJira   SaaSService = "jira"
	ServiceLinear SaaSService = "linear"
	ServiceAsana  SaaSService = "asana"
	ServiceTrello SaaSService = "trello"
	ServiceMonday SaaSService = "monday"
)

// Communication
const (
	ServiceSlack   SaaSService = "slack"
	ServiceTeams   SaaSService = "teams"
	ServiceDiscord SaaSService = "discord"
)

// Documentation
const (
	ServiceConfluence SaaSService = "confluence"
	ServiceNotion     SaaSService = "notion"
	ServiceGoogleDocs SaaSService = "google_docs"
)

// Monitoring & Alerting
const (
	ServiceSentry    SaaSService = "sentry"
	ServicePagerDuty SaaSService = "pagerduty"
	ServiceOpsGenie  SaaSService = "opsgenie"
	ServiceDatadog   SaaSService = "datadog"
	ServiceNewRelic  SaaSService = "newrelic"
	ServiceGrafana   SaaSService = "grafana"
)

// Deployment & Infra
const (
	ServiceVercel     SaaSService = "vercel"
	ServiceNetlify    SaaSService = "netlify"
	ServiceAWS        SaaSService = "aws"
	ServiceGCP        SaaSService = "gcp"
	ServiceAzure      SaaSService = "azure"
	ServiceCloudflare SaaSService = "cloudflare"
	ServiceHeroku     SaaSService = "heroku"
)

// Finance
const (
	ServiceStripe SaaSService = "stripe"
	ServicePayPal SaaSService = "paypal"
)

const (
	ServiceUnknown SaaSService = "unknown"
)

// =============================================================================
// Common Event Types (Category-level)
// =============================================================================

// DevToolsEventType represents events common to developer tools.
type DevToolsEventType string

const (
	// Code Review Events
	DevEventReviewRequested  DevToolsEventType = "review_requested"
	DevEventReviewApproved   DevToolsEventType = "review_approved"
	DevEventReviewChangesReq DevToolsEventType = "review_changes_requested"
	DevEventReviewComment    DevToolsEventType = "review_comment"

	// PR/MR Events
	DevEventPRCreated DevToolsEventType = "pr_created"
	DevEventPRMerged  DevToolsEventType = "pr_merged"
	DevEventPRClosed  DevToolsEventType = "pr_closed"
	DevEventPRComment DevToolsEventType = "pr_comment"
	DevEventPRPush    DevToolsEventType = "pr_push"

	// Issue Events
	DevEventIssueCreated   DevToolsEventType = "issue_created"
	DevEventIssueAssigned  DevToolsEventType = "issue_assigned"
	DevEventIssueMentioned DevToolsEventType = "issue_mentioned"
	DevEventIssueComment   DevToolsEventType = "issue_comment"
	DevEventIssueClosed    DevToolsEventType = "issue_closed"
	DevEventIssueReopened  DevToolsEventType = "issue_reopened"

	// CI/CD Events
	DevEventCIFailed   DevToolsEventType = "ci_failed"
	DevEventCIPassed   DevToolsEventType = "ci_passed"
	DevEventCIRunning  DevToolsEventType = "ci_running"
	DevEventCICanceled DevToolsEventType = "ci_canceled"

	// Security Events
	DevEventSecurityAlert   DevToolsEventType = "security_alert"
	DevEventDependabotAlert DevToolsEventType = "dependabot_alert"
	DevEventSecretScanAlert DevToolsEventType = "secret_scan_alert"

	// Repository Events
	DevEventRelease     DevToolsEventType = "release"
	DevEventTeamMention DevToolsEventType = "team_mention"
	DevEventWatching    DevToolsEventType = "watching"
	DevEventOwnActivity DevToolsEventType = "own_activity"
)

// ProjectMgmtEventType represents events common to project management tools.
type ProjectMgmtEventType string

const (
	// Issue/Task Events
	PMEventIssueCreated   ProjectMgmtEventType = "issue_created"
	PMEventIssueAssigned  ProjectMgmtEventType = "issue_assigned"
	PMEventIssueMentioned ProjectMgmtEventType = "issue_mentioned"
	PMEventIssueComment   ProjectMgmtEventType = "issue_comment"
	PMEventIssueUpdated   ProjectMgmtEventType = "issue_updated"
	PMEventIssueClosed    ProjectMgmtEventType = "issue_closed"
	PMEventIssueReopened  ProjectMgmtEventType = "issue_reopened"
	PMEventIssueDue       ProjectMgmtEventType = "issue_due"

	// Sprint/Cycle Events
	PMEventSprintStarted ProjectMgmtEventType = "sprint_started"
	PMEventSprintEnded   ProjectMgmtEventType = "sprint_ended"
	PMEventCycleStarted  ProjectMgmtEventType = "cycle_started"
	PMEventCycleEnded    ProjectMgmtEventType = "cycle_ended"

	// Status Events
	PMEventStatusChanged   ProjectMgmtEventType = "status_changed"
	PMEventPriorityChanged ProjectMgmtEventType = "priority_changed"
)

// CommunicationEventType represents events common to communication tools.
type CommunicationEventType string

const (
	CommEventMention         CommunicationEventType = "mention"
	CommEventDirectMessage   CommunicationEventType = "direct_message"
	CommEventChannelMessage  CommunicationEventType = "channel_message"
	CommEventThreadReply     CommunicationEventType = "thread_reply"
	CommEventReaction        CommunicationEventType = "reaction"
	CommEventChannelInvite   CommunicationEventType = "channel_invite"
	CommEventWorkspaceInvite CommunicationEventType = "workspace_invite"
	CommEventDigest          CommunicationEventType = "digest"
	CommEventSecurityAlert   CommunicationEventType = "security_alert"
)

// DocumentationEventType represents events common to documentation tools.
type DocumentationEventType string

const (
	DocEventComment     DocumentationEventType = "comment"
	DocEventMention     DocumentationEventType = "mention"
	DocEventShared      DocumentationEventType = "shared"
	DocEventEdited      DocumentationEventType = "edited"
	DocEventPageCreated DocumentationEventType = "page_created"
	DocEventPageDeleted DocumentationEventType = "page_deleted"
)

// MonitoringEventType represents events common to monitoring tools.
type MonitoringEventType string

const (
	MonEventAlertTriggered    MonitoringEventType = "alert_triggered"
	MonEventAlertAcknowledged MonitoringEventType = "alert_acknowledged"
	MonEventAlertResolved     MonitoringEventType = "alert_resolved"
	MonEventAlertEscalated    MonitoringEventType = "alert_escalated"
	MonEventIncidentCreated   MonitoringEventType = "incident_created"
	MonEventIncidentResolved  MonitoringEventType = "incident_resolved"
	MonEventIssueNew          MonitoringEventType = "issue_new"
	MonEventIssueRegressed    MonitoringEventType = "issue_regressed"
	MonEventDigest            MonitoringEventType = "digest"
	MonEventOnCallReminder    MonitoringEventType = "oncall_reminder"
)

// DeploymentEventType represents events common to deployment tools.
type DeploymentEventType string

const (
	DeployEventSucceeded    DeploymentEventType = "deploy_succeeded"
	DeployEventFailed       DeploymentEventType = "deploy_failed"
	DeployEventCanceled     DeploymentEventType = "deploy_canceled"
	DeployEventStarted      DeploymentEventType = "deploy_started"
	DeployEventBuildFailed  DeploymentEventType = "build_failed"
	DeployEventBuildPassed  DeploymentEventType = "build_passed"
	DeployEventDomainExpiry DeploymentEventType = "domain_expiry"
	DeployEventUsageAlert   DeploymentEventType = "usage_alert"
	DeployEventBillingAlert DeploymentEventType = "billing_alert"
)

// FinanceEventType represents events common to finance tools.
type FinanceEventType string

const (
	FinEventPaymentSucceeded FinanceEventType = "payment_succeeded"
	FinEventPaymentFailed    FinanceEventType = "payment_failed"
	FinEventInvoiceCreated   FinanceEventType = "invoice_created"
	FinEventInvoicePaid      FinanceEventType = "invoice_paid"
	FinEventSubscriptionNew  FinanceEventType = "subscription_new"
	FinEventSubscriptionEnd  FinanceEventType = "subscription_end"
	FinEventDisputeCreated   FinanceEventType = "dispute_created"
	FinEventPayoutPaid       FinanceEventType = "payout_paid"
	FinEventRefundIssued     FinanceEventType = "refund_issued"
)

// =============================================================================
// Parsed Email Result
// =============================================================================

// ParsedEmail represents a fully parsed email with extracted structured data.
type ParsedEmail struct {
	// Service identification
	Category SaaSCategory `json:"category"`
	Service  SaaSService  `json:"service"`
	Event    string       `json:"event"` // Category-specific event type

	// Classification
	EmailCategory domain.EmailCategory     `json:"email_category"`
	SubCategory   *domain.EmailSubCategory `json:"sub_category,omitempty"`
	Priority      domain.Priority          `json:"priority"`
	Score         float64                  `json:"score"`
	Source        string                   `json:"source"`

	// Extracted Data
	Data *ExtractedData `json:"data,omitempty"`

	// Action Items (potential todos)
	ActionItems []ActionItem `json:"action_items,omitempty"`

	// Related Entities
	Entities []Entity `json:"entities,omitempty"`

	// Signals for debugging/logging
	Signals []string `json:"signals,omitempty"`
}

// ExtractedData contains structured information extracted from the email.
type ExtractedData struct {
	// === Common Fields ===
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	Author      string `json:"author,omitempty"`

	// === Developer Tools Specific ===
	Repository string `json:"repository,omitempty"` // owner/repo
	Branch     string `json:"branch,omitempty"`
	CommitHash string `json:"commit_hash,omitempty"`
	PRNumber   int    `json:"pr_number,omitempty"`
	MRNumber   int    `json:"mr_number,omitempty"` // GitLab

	// === Project Management Specific ===
	IssueNumber int    `json:"issue_number,omitempty"`
	IssueKey    string `json:"issue_key,omitempty"` // PROJ-123 (Jira/Linear)
	Project     string `json:"project,omitempty"`
	ProjectID   string `json:"project_id,omitempty"`
	Team        string `json:"team,omitempty"`
	Sprint      string `json:"sprint,omitempty"`
	Status      string `json:"status,omitempty"`
	DueDate     string `json:"due_date,omitempty"`

	// === Communication Specific ===
	Channel     string `json:"channel,omitempty"`
	ThreadID    string `json:"thread_id,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
	MessageText string `json:"message_text,omitempty"`

	// === Monitoring Specific ===
	AlertID      string `json:"alert_id,omitempty"`
	IncidentID   string `json:"incident_id,omitempty"`
	AlertStatus  string `json:"alert_status,omitempty"` // triggered, acknowledged, resolved
	Severity     string `json:"severity,omitempty"`     // critical, high, medium, low
	Service      string `json:"service,omitempty"`
	Environment  string `json:"environment,omitempty"` // production, staging, dev
	ErrorMessage string `json:"error_message,omitempty"`
	ErrorCount   int    `json:"error_count,omitempty"`

	// === Security Specific ===
	CVE            string   `json:"cve,omitempty"`
	Package        string   `json:"package,omitempty"`
	VulnVersion    string   `json:"vuln_version,omitempty"`
	PatchedVersion string   `json:"patched_version,omitempty"`
	CVSS           float64  `json:"cvss,omitempty"`
	AffectedFiles  []string `json:"affected_files,omitempty"`

	// === Deployment Specific ===
	DeploymentID  string `json:"deployment_id,omitempty"`
	DeploymentURL string `json:"deployment_url,omitempty"`
	DeploymentEnv string `json:"deployment_env,omitempty"`
	BuildStatus   string `json:"build_status,omitempty"`
	BuildDuration string `json:"build_duration,omitempty"`
	PipelineID    string `json:"pipeline_id,omitempty"`
	WorkflowName  string `json:"workflow_name,omitempty"`

	// === Finance Specific ===
	Amount        string `json:"amount,omitempty"`
	Currency      string `json:"currency,omitempty"`
	TransactionID string `json:"transaction_id,omitempty"`
	InvoiceNumber string `json:"invoice_number,omitempty"`
	CustomerID    string `json:"customer_id,omitempty"`
	CustomerEmail string `json:"customer_email,omitempty"`

	// === User/Assignment Info ===
	Assignee  string   `json:"assignee,omitempty"`
	Assignees []string `json:"assignees,omitempty"`
	Reviewer  string   `json:"reviewer,omitempty"`
	Reviewers []string `json:"reviewers,omitempty"`
	Mentions  []string `json:"mentions,omitempty"`

	// === Labels/Tags ===
	Labels []string `json:"labels,omitempty"`
	Tags   []string `json:"tags,omitempty"`

	// === Extra (service-specific fields) ===
	Extra map[string]interface{} `json:"extra,omitempty"`
}

// =============================================================================
// Action Items
// =============================================================================

// ActionItem represents a potential todo item extracted from the email.
type ActionItem struct {
	Type        ActionType `json:"type"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	URL         string     `json:"url,omitempty"`
	Priority    string     `json:"priority,omitempty"` // urgent, high, medium, low
	DueDate     string     `json:"due_date,omitempty"`
}

// ActionType represents the type of action to be taken.
type ActionType string

const (
	ActionReview      ActionType = "review"      // Review PR/MR/document
	ActionFix         ActionType = "fix"         // Fix issue/bug/alert
	ActionRespond     ActionType = "respond"     // Respond to comment/mention/message
	ActionApprove     ActionType = "approve"     // Approve MR/request
	ActionAcknowledge ActionType = "acknowledge" // Acknowledge alert/incident
	ActionResolve     ActionType = "resolve"     // Resolve issue/incident
	ActionDeploy      ActionType = "deploy"      // Deploy/rollback
	ActionInvestigate ActionType = "investigate" // Investigate alert/incident
	ActionPay         ActionType = "pay"         // Pay invoice
	ActionRead        ActionType = "read"        // Read/review content
	ActionUpdate      ActionType = "update"      // Update status/info
)

// =============================================================================
// Entity
// =============================================================================

// Entity represents a related entity mentioned in the email.
type Entity struct {
	Type EntityType `json:"type"`
	ID   string     `json:"id"`
	Name string     `json:"name,omitempty"`
	URL  string     `json:"url,omitempty"`
}

// EntityType represents the type of entity.
type EntityType string

const (
	EntityUser       EntityType = "user"
	EntityRepository EntityType = "repository"
	EntityProject    EntityType = "project"
	EntityTeam       EntityType = "team"
	EntityIssue      EntityType = "issue"
	EntityPR         EntityType = "pull_request"
	EntityMR         EntityType = "merge_request"
	EntityCommit     EntityType = "commit"
	EntityBranch     EntityType = "branch"
	EntityPipeline   EntityType = "pipeline"
	EntityIncident   EntityType = "incident"
	EntityAlert      EntityType = "alert"
	EntityService    EntityType = "service"
	EntityChannel    EntityType = "channel"
	EntityWorkspace  EntityType = "workspace"
	EntityDocument   EntityType = "document"
	EntityInvoice    EntityType = "invoice"
)

// =============================================================================
// Parser Interface
// =============================================================================

// Parser defines the interface for service-specific RFC parsers.
type Parser interface {
	// Service returns the SaaS service this parser handles.
	Service() SaaSService

	// Category returns the SaaS category.
	Category() SaaSCategory

	// CanParse checks if this parser can handle the given email.
	CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool

	// Parse extracts structured data from the email.
	Parse(input *ParserInput) (*ParsedEmail, error)
}

// ParserInput contains all information needed for parsing.
type ParserInput struct {
	// Provider data
	Message *out.ProviderMailMessage
	Body    *out.ProviderMessageBody

	// Pre-extracted headers
	Headers *out.ProviderClassificationHeaders

	// Raw headers (for service-specific parsing)
	RawHeaders map[string]string
}

// =============================================================================
// Parser Registry
// =============================================================================

// Registry manages service-specific parsers.
type Registry struct {
	parsers map[SaaSService]Parser
	// Category-indexed for fallback
	categoryParsers map[SaaSCategory][]Parser
}

// NewRegistry creates a new parser registry.
func NewRegistry() *Registry {
	return &Registry{
		parsers:         make(map[SaaSService]Parser),
		categoryParsers: make(map[SaaSCategory][]Parser),
	}
}

// Register adds a parser to the registry.
func (r *Registry) Register(p Parser) {
	r.parsers[p.Service()] = p
	r.categoryParsers[p.Category()] = append(r.categoryParsers[p.Category()], p)
}

// FindParser finds the appropriate parser for the given email.
func (r *Registry) FindParser(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) Parser {
	for _, p := range r.parsers {
		if p.CanParse(headers, fromEmail, rawHeaders) {
			return p
		}
	}
	return nil
}

// Parse parses the email using the appropriate parser.
func (r *Registry) Parse(input *ParserInput) (*ParsedEmail, error) {
	fromEmail := ""
	if input.Message != nil {
		fromEmail = input.Message.From.Email
	}

	parser := r.FindParser(input.Headers, fromEmail, input.RawHeaders)
	if parser == nil {
		return nil, nil // No parser found
	}

	return parser.Parse(input)
}

// GetParsersByCategory returns all parsers for a given category.
func (r *Registry) GetParsersByCategory(category SaaSCategory) []Parser {
	return r.categoryParsers[category]
}
