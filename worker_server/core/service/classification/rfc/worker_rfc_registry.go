// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

// =============================================================================
// Default Registry Setup
// =============================================================================

// NewDefaultRegistry creates a registry with all default parsers registered.
func NewDefaultRegistry() *Registry {
	r := NewRegistry()

	// === Developer Tools ===
	r.Register(NewGitHubParser())
	r.Register(NewGitLabParser())
	r.Register(NewBitbucketParser())

	// === Project Management ===
	r.Register(NewJiraParser())
	r.Register(NewLinearParser())
	r.Register(NewAsanaParser())
	r.Register(NewTrelloParser())

	// === Communication ===
	r.Register(NewSlackParser())
	r.Register(NewTeamsParser())
	r.Register(NewDiscordParser())

	// === Documentation ===
	r.Register(NewConfluenceParser())
	r.Register(NewNotionParser())

	// === Monitoring ===
	r.Register(NewSentryParser())
	r.Register(NewPagerDutyParser())
	r.Register(NewDatadogParser())
	r.Register(NewOpsGenieParser())

	// === Deployment ===
	r.Register(NewVercelParser())
	r.Register(NewNetlifyParser())
	r.Register(NewAWSParser())

	// === Finance ===
	r.Register(NewStripeParser())
	r.Register(NewPayPalParser())

	return r
}

// =============================================================================
// Parser Factory Functions
// =============================================================================

// GetParserForService returns a parser for the given service.
func GetParserForService(service SaaSService) Parser {
	switch service {
	// Developer Tools
	case ServiceGitHub:
		return NewGitHubParser()
	case ServiceGitLab:
		return NewGitLabParser()
	case ServiceBitbucket:
		return NewBitbucketParser()

	// Project Management
	case ServiceJira:
		return NewJiraParser()
	case ServiceLinear:
		return NewLinearParser()
	case ServiceAsana:
		return NewAsanaParser()
	case ServiceTrello:
		return NewTrelloParser()

	// Communication
	case ServiceSlack:
		return NewSlackParser()
	case ServiceTeams:
		return NewTeamsParser()
	case ServiceDiscord:
		return NewDiscordParser()

	// Documentation
	case ServiceConfluence:
		return NewConfluenceParser()
	case ServiceNotion:
		return NewNotionParser()

	// Monitoring
	case ServiceSentry:
		return NewSentryParser()
	case ServicePagerDuty:
		return NewPagerDutyParser()
	case ServiceDatadog:
		return NewDatadogParser()
	case ServiceOpsGenie:
		return NewOpsGenieParser()

	// Deployment
	case ServiceVercel:
		return NewVercelParser()
	case ServiceNetlify:
		return NewNetlifyParser()
	case ServiceAWS:
		return NewAWSParser()

	// Finance
	case ServiceStripe:
		return NewStripeParser()
	case ServicePayPal:
		return NewPayPalParser()

	default:
		return nil
	}
}

// GetCategoryForService returns the category for a given service.
func GetCategoryForService(service SaaSService) SaaSCategory {
	switch service {
	case ServiceGitHub, ServiceGitLab, ServiceBitbucket:
		return CategoryDevTools
	case ServiceJira, ServiceLinear, ServiceAsana, ServiceTrello, ServiceMonday:
		return CategoryProjectMgmt
	case ServiceSlack, ServiceTeams, ServiceDiscord:
		return CategoryCommunication
	case ServiceConfluence, ServiceNotion, ServiceGoogleDocs:
		return CategoryDocumentation
	case ServiceSentry, ServicePagerDuty, ServiceOpsGenie, ServiceDatadog, ServiceNewRelic, ServiceGrafana:
		return CategoryMonitoring
	case ServiceVercel, ServiceNetlify, ServiceAWS, ServiceGCP, ServiceAzure, ServiceCloudflare, ServiceHeroku:
		return CategoryDeployment
	case ServiceStripe, ServicePayPal:
		return CategoryFinance
	default:
		return CategoryUnknown
	}
}

// =============================================================================
// Service Detection Helpers
// =============================================================================

// KnownDomains maps email domains to SaaS services.
var KnownDomains = map[string]SaaSService{
	// Developer Tools
	"github.com":         ServiceGitHub,
	"noreply.github.com": ServiceGitHub,
	"gitlab.com":         ServiceGitLab,
	"bitbucket.org":      ServiceBitbucket,

	// Project Management
	"atlassian.com":     ServiceJira,
	"atlassian.net":     ServiceJira,
	"jira.com":          ServiceJira,
	"am.atlassian.com":  ServiceJira, // Atlassian shared services
	"linear.app":        ServiceLinear,
	"asana.com":         ServiceAsana,
	"mail.asana.com":    ServiceAsana,
	"trello.com":        ServiceTrello,
	"trellobutler.com":  ServiceTrello,
	"boards.trello.com": ServiceTrello,
	"monday.com":        ServiceMonday,

	// Communication
	"slack.com":                 ServiceSlack,
	"teams.microsoft.com":       ServiceTeams,
	"email.teams.microsoft.com": ServiceTeams,
	"teams.mail.microsoft":      ServiceTeams, // New Teams domain
	"discord.com":               ServiceDiscord,
	"discordapp.com":            ServiceDiscord, // Legacy domain
	"m.discord.com":             ServiceDiscord, // Marketing subdomain

	// Documentation
	"confluence.com":  ServiceConfluence,
	"notion.so":       ServiceNotion,
	"mail.notion.so":  ServiceNotion,
	"makenotion.com":  ServiceNotion, // Notion support
	"docs.google.com": ServiceGoogleDocs,

	// Monitoring
	"sentry.io":     ServiceSentry,
	"getsentry.com": ServiceSentry,
	"pagerduty.com": ServicePagerDuty,
	"opsgenie.com":  ServiceOpsGenie,
	"opsgenie.net":  ServiceOpsGenie,
	"datadoghq.com": ServiceDatadog,
	"dtdg.co":       ServiceDatadog, // Datadog synthetic testing
	"newrelic.com":  ServiceNewRelic,
	"grafana.com":   ServiceGrafana,
	"grafana.net":   ServiceGrafana,

	// Deployment
	"vercel.com":       ServiceVercel,
	"netlify.com":      ServiceNetlify,
	"amazonaws.com":    ServiceAWS,
	"aws.amazon.com":   ServiceAWS,
	"amazon.com":       ServiceAWS,
	"marketplace.aws":  ServiceAWS,
	"cloud.google.com": ServiceGCP,
	"azure.com":        ServiceAzure,
	"microsoft.com":    ServiceAzure,
	"cloudflare.com":   ServiceCloudflare,
	"heroku.com":       ServiceHeroku,

	// Finance
	"stripe.com":   ServiceStripe,
	"paypal.com":   ServicePayPal,
	"paypal.co.uk": ServicePayPal,
}

// DetectServiceFromDomain detects service from email domain.
func DetectServiceFromDomain(domain string) SaaSService {
	// Direct lookup
	if service, ok := KnownDomains[domain]; ok {
		return service
	}

	// Check for subdomain matches
	for knownDomain, service := range KnownDomains {
		if len(domain) > len(knownDomain) {
			suffix := domain[len(domain)-len(knownDomain):]
			if suffix == knownDomain && domain[len(domain)-len(knownDomain)-1] == '.' {
				return service
			}
		}
	}

	return ServiceUnknown
}
