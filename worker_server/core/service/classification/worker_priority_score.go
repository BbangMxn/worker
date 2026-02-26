// Package classification implements the score-based email classification pipeline.
package classification

// =============================================================================
// Priority Scoring System
// =============================================================================
//
// Priority is calculated as float64 (0.00 ~ 1.00) using:
//   1. Critical events get fixed max values (no calculation)
//   2. Normal events = DomainScore + ReasonScore + RelationScore + SeverityScore
//
// Final priority determines inbox ordering (higher = more important)

// -----------------------------------------------------------------------------
// Critical Fixed Values (no calculation, immediate assignment)
// -----------------------------------------------------------------------------
const (
	// Infrastructure Critical - 최고 우선순위
	PriorityServerDown     float64 = 0.99 // 서버 다운
	PriorityServerCrash    float64 = 0.99 // 서버 크래시
	PrioritySecurityBreach float64 = 0.98 // 보안 침해
	PriorityDataLoss       float64 = 0.98 // 데이터 손실

	// Security Critical
	PrioritySecurityCritical float64 = 0.95 // Critical 보안 취약점
	PrioritySecurityHigh     float64 = 0.88 // High 보안 취약점
)

// -----------------------------------------------------------------------------
// Domain Base Scores (도메인별 기본 점수)
// -----------------------------------------------------------------------------
const (
	// === Infrastructure & Alerting (높은 기본 점수) ===
	DomainScorePagerDuty   float64 = 0.35
	DomainScoreOpsGenie    float64 = 0.35
	DomainScoreDatadog     float64 = 0.30
	DomainScoreSentry      float64 = 0.28
	DomainScoreNewRelic    float64 = 0.28
	DomainScoreGrafana     float64 = 0.28
	DomainScoreUptimeRobot float64 = 0.30
	DomainScorePingdom     float64 = 0.30
	DomainScoreCloudflare  float64 = 0.25

	// === Developer Tools ===
	DomainScoreGitHub    float64 = 0.18
	DomainScoreGitLab    float64 = 0.18
	DomainScoreBitbucket float64 = 0.18
	DomainScoreJira      float64 = 0.20
	DomainScoreLinear    float64 = 0.20
	DomainScoreVercel    float64 = 0.15
	DomainScoreNetlify   float64 = 0.15
	DomainScoreAWS       float64 = 0.22
	DomainScoreGCP       float64 = 0.22
	DomainScoreAzure     float64 = 0.22

	// === Finance (중요) ===
	DomainScoreBank    float64 = 0.30
	DomainScoreStripe  float64 = 0.25
	DomainScorePayPal  float64 = 0.25
	DomainScoreFintech float64 = 0.22

	// === Business Tools ===
	DomainScoreSlack    float64 = 0.15
	DomainScoreZoom     float64 = 0.18
	DomainScoreCalendly float64 = 0.20
	DomainScoreNotion   float64 = 0.12

	// === Shopping & Travel ===
	DomainScoreShopping float64 = 0.15
	DomainScoreTravel   float64 = 0.18
	DomainScoreShipping float64 = 0.15

	// === Social & Marketing (낮은 기본 점수) ===
	DomainScoreSocial     float64 = 0.08
	DomainScoreNewsletter float64 = 0.05
	DomainScoreMarketing  float64 = 0.03
)

// -----------------------------------------------------------------------------
// Reason Scores (이유/액션별 점수)
// -----------------------------------------------------------------------------
const (
	// === Direct Action Required (높음) ===
	ReasonScoreReviewRequested float64 = 0.30
	ReasonScoreMention         float64 = 0.28
	ReasonScoreAssign          float64 = 0.30
	ReasonScoreApprovalNeeded  float64 = 0.28

	// === Author/Owner Activity (중요) ===
	ReasonScoreAuthor float64 = 0.25
	ReasonScoreOwner  float64 = 0.25

	// === Team Related ===
	ReasonScoreTeamMention float64 = 0.18

	// === Passive Watching (낮음) ===
	ReasonScoreComment      float64 = 0.10
	ReasonScoreStateChange  float64 = 0.08
	ReasonScoreSubscribed   float64 = 0.05
	ReasonScorePush         float64 = 0.03
	ReasonScoreYourActivity float64 = 0.00 // 내 활동 에코는 0점

	// === CI/CD ===
	ReasonScoreCIFailed      float64 = 0.22
	ReasonScoreCIPassed      float64 = 0.05
	ReasonScoreDeployFailed  float64 = 0.25
	ReasonScoreDeploySuccess float64 = 0.08

	// === Alerts ===
	ReasonScoreAlertCritical float64 = 0.40
	ReasonScoreAlertWarning  float64 = 0.20
	ReasonScoreAlertInfo     float64 = 0.08
)

// -----------------------------------------------------------------------------
// Relation Scores (관계 점수 - 나와의 관련성)
// -----------------------------------------------------------------------------
const (
	RelationScoreDirect   float64 = 0.20 // 나한테 직접
	RelationScoreTeam     float64 = 0.12 // 내 팀에게
	RelationScoreProject  float64 = 0.08 // 내 프로젝트
	RelationScoreWatching float64 = 0.03 // 구독/워칭
	RelationScoreNone     float64 = 0.00 // 관계 없음
)

// -----------------------------------------------------------------------------
// Severity Scores (심각도 점수)
// -----------------------------------------------------------------------------
const (
	SeverityScoreCritical float64 = 0.30
	SeverityScoreHigh     float64 = 0.20
	SeverityScoreMedium   float64 = 0.10
	SeverityScoreLow      float64 = 0.05
	SeverityScoreInfo     float64 = 0.00
)

// -----------------------------------------------------------------------------
// Helper Functions
// -----------------------------------------------------------------------------

// CalculatePriority calculates final priority from component scores.
// Returns value capped at 1.0
func CalculatePriority(domainScore, reason, relation, severity float64) float64 {
	total := domainScore + reason + relation + severity
	if total > 1.0 {
		return 1.0
	}
	if total < 0.0 {
		return 0.0
	}
	return total
}

// ValidatePriority ensures priority is within valid range (0.0 ~ 1.0).
// Returns clamped value.
func ValidatePriority(p float64) float64 {
	if p < 0.0 {
		return 0.0
	}
	if p > 1.0 {
		return 1.0
	}
	// Handle NaN and Inf
	if p != p { // NaN check
		return 0.5
	}
	return p
}

// ValidCategory checks if category is a valid EmailCategory.
var ValidCategories = map[string]bool{
	"primary":      true,
	"work":         true,
	"personal":     true,
	"newsletter":   true,
	"notification": true,
	"marketing":    true,
	"social":       true,
	"finance":      true,
	"travel":       true,
	"shopping":     true,
	"spam":         true,
	"other":        true,
}

// IsValidCategory checks if category string is valid.
func IsValidCategory(cat string) bool {
	return ValidCategories[cat]
}

// ValidateCategory returns valid category or default "other".
func ValidateCategory(cat string) string {
	if ValidCategories[cat] {
		return cat
	}
	return "other"
}

// ValidSubCategories for validation (matches DB enum)
var ValidSubCategories = map[string]bool{
	"receipt":      true,
	"invoice":      true,
	"shipping":     true,
	"order":        true,
	"travel":       true,
	"calendar":     true,
	"account":      true,
	"security":     true,
	"sns":          true,
	"comment":      true,
	"newsletter":   true,
	"marketing":    true,
	"deal":         true,
	"notification": true,
	"alert":        true,
	"developer":    true,
}

// IsValidSubCategory checks if sub_category string is valid.
func IsValidSubCategory(subCat string) bool {
	return ValidSubCategories[subCat]
}

// ValidateSubCategory returns valid sub_category or empty string.
func ValidateSubCategory(subCat string) string {
	if ValidSubCategories[subCat] {
		return subCat
	}
	return ""
}

// IsCriticalEvent checks if this should bypass calculation and get fixed max value.
func IsCriticalEvent(signal string) (float64, bool) {
	criticalSignals := map[string]float64{
		"server_down":     PriorityServerDown,
		"server_crash":    PriorityServerCrash,
		"is_down":         PriorityServerDown,
		"crashed":         PriorityServerCrash,
		"security_breach": PrioritySecurityBreach,
		"data_loss":       PriorityDataLoss,
		"critical_vuln":   PrioritySecurityCritical,
	}

	if priority, ok := criticalSignals[signal]; ok {
		return priority, true
	}
	return 0, false
}

// GetGitHubReasonScore returns reason + relation scores for GitHub X-GitHub-Reason.
func GetGitHubReasonScore(reason string) (reasonScore, relationScore float64) {
	switch reason {
	// Direct involvement → high scores
	case "review_requested":
		return ReasonScoreReviewRequested, RelationScoreDirect
	case "mention":
		return ReasonScoreMention, RelationScoreDirect
	case "assign":
		return ReasonScoreAssign, RelationScoreDirect
	case "author":
		return ReasonScoreAuthor, RelationScoreDirect
	case "team_mention":
		return ReasonScoreTeamMention, RelationScoreTeam
	case "ci_activity":
		return ReasonScoreCIFailed, RelationScoreProject // Assume failed for higher score

	// Passive watching → low scores
	case "subscribed":
		return ReasonScoreSubscribed, RelationScoreWatching
	case "manual":
		return ReasonScoreSubscribed, RelationScoreWatching
	case "push":
		return ReasonScorePush, RelationScoreWatching
	case "comment":
		return ReasonScoreComment, RelationScoreWatching
	case "state_change":
		return ReasonScoreStateChange, RelationScoreWatching
	case "your_activity":
		return ReasonScoreYourActivity, RelationScoreNone

	default:
		return 0.05, RelationScoreWatching
	}
}

// GetGitHubSecurityScore returns fixed priority for GitHub security alerts.
func GetGitHubSecurityScore(severity string) float64 {
	switch severity {
	case "critical":
		return PrioritySecurityCritical
	case "high":
		return PrioritySecurityHigh
	case "moderate", "medium":
		return CalculatePriority(DomainScoreGitHub, ReasonScoreAlertWarning, RelationScoreProject, SeverityScoreMedium)
	case "low":
		return CalculatePriority(DomainScoreGitHub, ReasonScoreAlertInfo, RelationScoreProject, SeverityScoreLow)
	default:
		return PrioritySecurityHigh // Default high for unknown security
	}
}

// GetGitLabReasonScore returns reason + relation scores for GitLab notifications.
func GetGitLabReasonScore(reason string) (reasonScore, relationScore float64) {
	switch reason {
	case "mentioned", "directly_addressed":
		return ReasonScoreMention, RelationScoreDirect
	case "assigned":
		return ReasonScoreAssign, RelationScoreDirect
	case "review_requested":
		return ReasonScoreReviewRequested, RelationScoreDirect
	case "approval_required":
		return ReasonScoreApprovalNeeded, RelationScoreDirect
	case "subscribed", "watching":
		return ReasonScoreSubscribed, RelationScoreWatching
	case "own_activity":
		return ReasonScoreYourActivity, RelationScoreNone
	default:
		return 0.05, RelationScoreWatching
	}
}

// -----------------------------------------------------------------------------
// Domain Score Lookup
// -----------------------------------------------------------------------------

// DomainScoreMap contains domain → base score mappings
var DomainScoreMap = map[string]float64{
	// === Infrastructure & Alerting ===
	"pagerduty.com":    DomainScorePagerDuty,
	"opsgenie.com":     DomainScoreOpsGenie,
	"datadoghq.com":    DomainScoreDatadog,
	"sentry.io":        DomainScoreSentry,
	"newrelic.com":     DomainScoreNewRelic,
	"grafana.com":      DomainScoreGrafana,
	"grafana.net":      DomainScoreGrafana,
	"uptimerobot.com":  DomainScoreUptimeRobot,
	"pingdom.com":      DomainScorePingdom,
	"cloudflare.com":   DomainScoreCloudflare,
	"betteruptime.com": DomainScoreUptimeRobot,
	"statuspage.io":    0.25,

	// === Developer Tools ===
	"github.com":    DomainScoreGitHub,
	"gitlab.com":    DomainScoreGitLab,
	"bitbucket.org": DomainScoreBitbucket,
	"atlassian.com": DomainScoreJira,
	"atlassian.net": DomainScoreJira,
	"linear.app":    DomainScoreLinear,
	"vercel.com":    DomainScoreVercel,
	"netlify.com":   DomainScoreNetlify,
	"heroku.com":    0.15,
	"railway.app":   0.15,
	"render.com":    0.15,
	"fly.io":        0.15,

	// === Cloud Providers ===
	"amazonaws.com":    DomainScoreAWS,
	"cloud.google.com": DomainScoreGCP,
	"azure.com":        DomainScoreAzure,
	"digitalocean.com": 0.18,

	// === Finance ===
	"stripe.com":   DomainScoreStripe,
	"paypal.com":   DomainScorePayPal,
	"brex.com":     DomainScoreFintech,
	"mercury.com":  DomainScoreFintech,
	"wise.com":     DomainScoreFintech,
	"revolut.com":  DomainScoreFintech,
	"plaid.com":    DomainScoreFintech,
	"coinbase.com": 0.20,
	// Korean Banks
	"kbstar.com":    DomainScoreBank,
	"shinhan.com":   DomainScoreBank,
	"wooribank.com": DomainScoreBank,
	"hanabank.com":  DomainScoreBank,
	"kakaobank.com": DomainScoreBank,
	"toss.im":       DomainScoreFintech,

	// === Business Tools ===
	"slack.com":    DomainScoreSlack,
	"zoom.us":      DomainScoreZoom,
	"calendly.com": DomainScoreCalendly,
	"notion.so":    DomainScoreNotion,
	"figma.com":    0.12,
	"miro.com":     0.12,
	"loom.com":     0.10,

	// === Shopping & Shipping ===
	"amazon.com":      DomainScoreShopping,
	"shopify.com":     DomainScoreShopping,
	"fedex.com":       DomainScoreShipping,
	"ups.com":         DomainScoreShipping,
	"dhl.com":         DomainScoreShipping,
	"cjlogistics.com": DomainScoreShipping,

	// === Travel ===
	"booking.com": DomainScoreTravel,
	"airbnb.com":  DomainScoreTravel,
	"expedia.com": DomainScoreTravel,
	"uber.com":    0.15,
	"lyft.com":    0.15,

	// === Social (low priority) ===
	"facebook.com":     DomainScoreSocial,
	"facebookmail.com": DomainScoreSocial,
	"instagram.com":    DomainScoreSocial,
	"twitter.com":      DomainScoreSocial,
	"x.com":            DomainScoreSocial,
	"linkedin.com":     0.12, // Slightly higher for professional
	"tiktok.com":       DomainScoreSocial,
	"discord.com":      0.10,
	"reddit.com":       DomainScoreSocial,

	// === Marketing (lowest priority) ===
	"mailchimp.com":       DomainScoreMarketing,
	"sendgrid.net":        DomainScoreMarketing,
	"constantcontact.com": DomainScoreMarketing,
	"hubspot.com":         DomainScoreMarketing,
}

// GetDomainScore returns the base score for a domain.
// Returns default 0.10 if domain not found.
func GetDomainScore(domain string) float64 {
	if score, ok := DomainScoreMap[domain]; ok {
		return score
	}
	// Check for subdomain matches (e.g., mail.github.com → github.com)
	for knownDomain, score := range DomainScoreMap {
		if len(domain) > len(knownDomain) && domain[len(domain)-len(knownDomain)-1] == '.' &&
			domain[len(domain)-len(knownDomain):] == knownDomain {
			return score * 0.95 // Slightly lower for subdomain
		}
	}
	return 0.10 // Default score
}

// -----------------------------------------------------------------------------
// Subject Pattern Scores
// -----------------------------------------------------------------------------
const (
	// Critical patterns (high scores)
	SubjectScoreServerDown    float64 = 0.50
	SubjectScoreCrash         float64 = 0.50
	SubjectScoreSecurityAlert float64 = 0.40

	// CI/CD patterns
	SubjectScoreBuildFailed   float64 = 0.25
	SubjectScoreBuildPassed   float64 = 0.05
	SubjectScoreDeployFailed  float64 = 0.28
	SubjectScoreDeploySuccess float64 = 0.08

	// Finance patterns
	SubjectScorePaymentFailed  float64 = 0.30
	SubjectScorePaymentSuccess float64 = 0.15
	SubjectScoreInvoice        float64 = 0.18
	SubjectScoreReceipt        float64 = 0.12

	// Shipping patterns
	SubjectScoreShipped   float64 = 0.15
	SubjectScoreDelivered float64 = 0.12
	SubjectScoreOrder     float64 = 0.15

	// Calendar patterns
	SubjectScoreMeetingInvite float64 = 0.20
	SubjectScoreMeetingCancel float64 = 0.18

	// Marketing (low)
	SubjectScorePromotion  float64 = 0.03
	SubjectScoreNewsletter float64 = 0.05
)
