package domain

import (
	"time"

	"github.com/google/uuid"
)

// EmailCategory represents the main AI-assigned email category
type EmailCategory string

const (
	// === Core Categories ===
	CategoryPrimary      EmailCategory = "primary"      // Important emails requiring attention
	CategoryWork         EmailCategory = "work"         // Work-related emails
	CategoryPersonal     EmailCategory = "personal"     // Personal correspondence
	CategoryNotification EmailCategory = "notification" // Auto-generated notifications

	// === Content Categories ===
	CategoryNewsletter EmailCategory = "newsletter" // Newsletters and digests
	CategoryMarketing  EmailCategory = "marketing"  // Marketing and promotional
	CategorySocial     EmailCategory = "social"     // Social network notifications

	// === Transaction Categories ===
	CategoryFinance  EmailCategory = "finance"  // Financial transactions, invoices, receipts
	CategoryTravel   EmailCategory = "travel"   // Travel bookings, confirmations
	CategoryShopping EmailCategory = "shopping" // E-commerce orders, shipping
	CategoryReceipts EmailCategory = "receipts" // Purchase receipts
	CategoryBilling  EmailCategory = "billing"  // Bills, invoices, statements

	// === Service Categories ===
	CategoryDeveloper     EmailCategory = "developer"     // GitHub, GitLab, CI/CD, code reviews
	CategoryMonitoring    EmailCategory = "monitoring"    // Sentry, PagerDuty, alerts
	CategoryDeployment    EmailCategory = "deployment"    // Vercel, Netlify, AWS deployments
	CategoryProjectMgmt   EmailCategory = "project_mgmt"  // Jira, Linear, Asana tasks
	CategoryDocumentation EmailCategory = "documentation" // Confluence, Notion docs
	CategoryCommunication EmailCategory = "communication" // Slack, Teams, Discord

	// === Security & System ===
	CategorySecurity EmailCategory = "security" // Security alerts, 2FA, login attempts
	CategoryAccount  EmailCategory = "account"  // Account-related (password reset, verification)
	CategorySystem   EmailCategory = "system"   // System notifications

	// === Low Priority ===
	CategorySpam  EmailCategory = "spam"  // Spam/unwanted
	CategoryOther EmailCategory = "other" // Uncategorized
	CategoryBulk  EmailCategory = "bulk"  // Bulk/mass emails
	CategoryLegal EmailCategory = "legal" // Legal documents, terms updates
)

// EmailSubCategory represents fine-grained classification under Category
type EmailSubCategory string

const (
	// === Finance SubCategories ===
	SubCategoryReceipt      EmailSubCategory = "receipt"      // Purchase receipts
	SubCategoryInvoice      EmailSubCategory = "invoice"      // Invoices to pay
	SubCategoryPayment      EmailSubCategory = "payment"      // Payment confirmations
	SubCategoryRefund       EmailSubCategory = "refund"       // Refund notifications
	SubCategoryDispute      EmailSubCategory = "dispute"      // Payment disputes
	SubCategorySubscription EmailSubCategory = "subscription" // Subscription updates
	SubCategoryPayout       EmailSubCategory = "payout"       // Payouts received
	SubCategoryStatement    EmailSubCategory = "statement"    // Account statements

	// === Shopping SubCategories ===
	SubCategoryShipping EmailSubCategory = "shipping" // Shipping notifications
	SubCategoryOrder    EmailSubCategory = "order"    // Order confirmations
	SubCategoryDelivery EmailSubCategory = "delivery" // Delivery updates
	SubCategoryReturn   EmailSubCategory = "return"   // Return/exchange

	// === Travel SubCategories ===
	SubCategoryTravel    EmailSubCategory = "travel"    // Travel bookings
	SubCategoryFlight    EmailSubCategory = "flight"    // Flight updates
	SubCategoryHotel     EmailSubCategory = "hotel"     // Hotel reservations
	SubCategoryItinerary EmailSubCategory = "itinerary" // Trip itineraries

	// === Work SubCategories ===
	SubCategoryCalendar EmailSubCategory = "calendar" // Calendar invites/updates
	SubCategoryMeeting  EmailSubCategory = "meeting"  // Meeting notifications
	SubCategoryTask     EmailSubCategory = "task"     // Task assignments
	SubCategoryProject  EmailSubCategory = "project"  // Project updates

	// === Developer SubCategories ===
	SubCategoryDeveloper   EmailSubCategory = "developer"    // General dev notifications
	SubCategoryCodeReview  EmailSubCategory = "code_review"  // PR/MR review requests
	SubCategoryBuild       EmailSubCategory = "build"        // Build/CI notifications
	SubCategoryDeploy      EmailSubCategory = "deploy"       // Deployment notifications
	SubCategoryIssue       EmailSubCategory = "issue"        // Issue updates
	SubCategoryMerge       EmailSubCategory = "merge"        // PR/MR merged
	SubCategoryRelease     EmailSubCategory = "release"      // Release notifications
	SubCategorySecurityDev EmailSubCategory = "security_dev" // Security vulnerabilities (Dependabot)

	// === Monitoring SubCategories ===
	SubCategoryAlert    EmailSubCategory = "alert"    // System alerts
	SubCategoryIncident EmailSubCategory = "incident" // Incident notifications
	SubCategoryResolved EmailSubCategory = "resolved" // Issue resolved
	SubCategoryWarning  EmailSubCategory = "warning"  // Warning notifications
	SubCategoryDigest   EmailSubCategory = "digest"   // Alert digests/summaries

	// === Communication SubCategories ===
	SubCategoryMention EmailSubCategory = "mention" // @mentions
	SubCategoryDM      EmailSubCategory = "dm"      // Direct messages
	SubCategoryChannel EmailSubCategory = "channel" // Channel messages
	SubCategoryThread  EmailSubCategory = "thread"  // Thread replies
	SubCategoryInvite  EmailSubCategory = "invite"  // Invitations

	// === Documentation SubCategories ===
	SubCategoryComment EmailSubCategory = "comment" // Comments on docs
	SubCategoryEdit    EmailSubCategory = "edit"    // Document edits
	SubCategoryShare   EmailSubCategory = "share"   // Document shared

	// === Social SubCategories ===
	SubCategorySNS        EmailSubCategory = "sns"        // Social network notifications
	SubCategoryFollow     EmailSubCategory = "follow"     // New followers
	SubCategoryLike       EmailSubCategory = "like"       // Likes/reactions
	SubCategoryConnection EmailSubCategory = "connection" // Connection requests

	// === Account SubCategories ===
	SubCategoryAccount  EmailSubCategory = "account"  // Account updates
	SubCategorySecurity EmailSubCategory = "security" // Security alerts
	SubCategoryPassword EmailSubCategory = "password" // Password reset
	SubCategoryVerify   EmailSubCategory = "verify"   // Email verification
	SubCategory2FA      EmailSubCategory = "2fa"      // Two-factor auth

	// === Marketing SubCategories ===
	SubCategoryNewsletter   EmailSubCategory = "newsletter"   // Newsletters
	SubCategoryMarketing    EmailSubCategory = "marketing"    // Marketing emails
	SubCategoryDeal         EmailSubCategory = "deal"         // Deals/promotions
	SubCategoryAnnouncement EmailSubCategory = "announcement" // Product announcements
	SubCategoryEvent        EmailSubCategory = "event"        // Event invitations

	// === System SubCategories ===
	SubCategoryNotification EmailSubCategory = "notification" // Auto-generated notifications
	SubCategoryUpdate       EmailSubCategory = "update"       // System updates
	SubCategoryMaintenance  EmailSubCategory = "maintenance"  // Maintenance notices
)

// ClassificationSource indicates how the classification was determined
type ClassificationSource string

const (
	ClassificationSourceHeader ClassificationSource = "header" // Header-based rules (List-Unsubscribe, etc.)
	ClassificationSourceDomain ClassificationSource = "domain" // Known domain matching
	ClassificationSourceLLM    ClassificationSource = "llm"    // LLM-based classification
	ClassificationSourceUser   ClassificationSource = "user"   // User manually set
)

// ClassificationRule represents a user-defined classification rule
type ClassificationRule struct {
	ID          int64     `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	IsActive    bool      `json:"is_active"`
	Priority    int       `json:"priority"` // Higher = processed first

	// Conditions (AND logic)
	Conditions []RuleCondition `json:"conditions"`

	// Actions
	Actions []RuleAction `json:"actions"`

	// Stats
	MatchCount  int64      `json:"match_count"`
	LastMatchAt *time.Time `json:"last_match_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ConditionField string

const (
	ConditionFieldFrom      ConditionField = "from"
	ConditionFieldTo        ConditionField = "to"
	ConditionFieldSubject   ConditionField = "subject"
	ConditionFieldBody      ConditionField = "body"
	ConditionFieldDomain    ConditionField = "domain"
	ConditionFieldHasAttach ConditionField = "has_attachment"
)

type ConditionOperator string

const (
	OperatorContains    ConditionOperator = "contains"
	OperatorNotContains ConditionOperator = "not_contains"
	OperatorEquals      ConditionOperator = "equals"
	OperatorNotEquals   ConditionOperator = "not_equals"
	OperatorStartsWith  ConditionOperator = "starts_with"
	OperatorEndsWith    ConditionOperator = "ends_with"
	OperatorMatches     ConditionOperator = "matches" // regex
)

type RuleCondition struct {
	Field    ConditionField    `json:"field"`
	Operator ConditionOperator `json:"operator"`
	Value    string            `json:"value"`
}

type ActionType string

const (
	ActionSetCategory ActionType = "set_category"
	ActionSetPriority ActionType = "set_priority"
	ActionAddLabel    ActionType = "add_label"
	ActionArchive     ActionType = "archive"
	ActionMarkRead    ActionType = "mark_read"
	ActionForward     ActionType = "forward"
)

type RuleAction struct {
	Type  ActionType `json:"type"`
	Value string     `json:"value"`
}

type ClassificationResult struct {
	EmailID     int64                `json:"email_id"`
	Category    *EmailCategory       `json:"category,omitempty"`
	SubCategory *EmailSubCategory    `json:"sub_category,omitempty"`
	Priority    *Priority            `json:"priority,omitempty"`
	Summary     *string              `json:"summary,omitempty"`
	Tags        []string             `json:"tags,omitempty"`
	Score       float64              `json:"score"`
	Source      ClassificationSource `json:"source"` // header, domain, llm, user

	// Which rules matched
	MatchedRules []int64 `json:"matched_rules,omitempty"`

	// Processing info
	ProcessedAt time.Time `json:"processed_at"`
	ModelUsed   string    `json:"model_used"`
	TokensUsed  int       `json:"tokens_used"`
}

// ClassificationPipelineResult represents the result of the 3-stage classification pipeline
type ClassificationPipelineResult struct {
	Category    EmailCategory        `json:"category"`
	SubCategory *EmailSubCategory    `json:"sub_category,omitempty"`
	Priority    Priority             `json:"priority"`
	Source      ClassificationSource `json:"source"`
	Confidence  float64              `json:"confidence"`

	// Cost tracking
	LLMUsed    bool `json:"llm_used"`
	TokensUsed int  `json:"tokens_used,omitempty"`
}
