// Package classification implements the score-based email classification pipeline.
package classification

import (
	"context"
	"strings"

	"worker_server/core/domain"
)

// =============================================================================
// Domain Score Classifier
// =============================================================================

// DomainScoreClassifier performs classification based on sender domain.
// This classifier recognizes well-known service domains and categorizes emails accordingly.
type DomainScoreClassifier struct {
	developerDomains    map[string]domainConfig
	financeDomains      map[string]domainConfig
	shoppingDomains     map[string]domainConfig
	travelDomains       map[string]domainConfig
	socialDomains       map[string]domainConfig
	productivityDomains map[string]domainConfig
}

type domainConfig struct {
	category    domain.EmailCategory
	subCategory domain.EmailSubCategory
	priority    domain.Priority
	score       float64
	source      string
}

// NewDomainScoreClassifier creates a new domain-based score classifier.
func NewDomainScoreClassifier() *DomainScoreClassifier {
	c := &DomainScoreClassifier{}
	c.initDeveloperDomains()
	c.initFinanceDomains()
	c.initShoppingDomains()
	c.initTravelDomains()
	c.initSocialDomains()
	c.initProductivityDomains()
	return c
}

// Name returns the classifier name.
func (c *DomainScoreClassifier) Name() string {
	return "domain"
}

// Stage returns the pipeline stage number.
func (c *DomainScoreClassifier) Stage() int {
	return 0 // Same stage as RFC (both are header-based)
}

// Classify performs domain-based classification.
func (c *DomainScoreClassifier) Classify(ctx context.Context, input *ScoreClassifierInput) (*ScoreClassifierResult, error) {
	if input.Email == nil || input.Email.FromEmail == "" {
		return nil, nil
	}

	// Extract domain from email
	emailDomain := extractDomain(input.Email.FromEmail)
	if emailDomain == "" {
		return nil, nil
	}

	// Check each domain category
	if result := c.checkDeveloperDomains(emailDomain); result != nil {
		return result, nil
	}
	if result := c.checkFinanceDomains(emailDomain); result != nil {
		return result, nil
	}
	if result := c.checkShoppingDomains(emailDomain); result != nil {
		return result, nil
	}
	if result := c.checkTravelDomains(emailDomain); result != nil {
		return result, nil
	}
	if result := c.checkSocialDomains(emailDomain); result != nil {
		return result, nil
	}
	if result := c.checkProductivityDomains(emailDomain); result != nil {
		return result, nil
	}

	return nil, nil
}

// checkDomainMap checks if the domain matches any in the map
// Priority is calculated using: DomainBaseScore + CategoryBonus
func (c *DomainScoreClassifier) checkDomainMap(emailDomain string, domainMap map[string]domainConfig) *ScoreClassifierResult {
	// Direct match
	if cfg, ok := domainMap[emailDomain]; ok {
		subCat := cfg.subCategory
		// Calculate priority using domain base score
		domainBaseScore := GetDomainScore(emailDomain)
		categoryBonus := getCategoryBonus(cfg.category)
		priority := CalculatePriority(domainBaseScore, categoryBonus, 0, 0)

		return &ScoreClassifierResult{
			Category:    cfg.category,
			SubCategory: &subCat,
			Priority:    domain.Priority(priority),
			Score:       cfg.score,
			Source:      cfg.source,
			Signals:     []string{"domain:" + emailDomain},
			LLMUsed:     false,
		}
	}

	// Subdomain match (e.g., mail.github.com -> github.com)
	for knownDomain, cfg := range domainMap {
		if strings.HasSuffix(emailDomain, "."+knownDomain) {
			subCat := cfg.subCategory
			// Calculate priority with slightly lower score for subdomain
			domainBaseScore := GetDomainScore(knownDomain) * 0.95
			categoryBonus := getCategoryBonus(cfg.category)
			priority := CalculatePriority(domainBaseScore, categoryBonus, 0, 0)

			return &ScoreClassifierResult{
				Category:    cfg.category,
				SubCategory: &subCat,
				Priority:    domain.Priority(priority),
				Score:       cfg.score * 0.95,
				Source:      cfg.source,
				Signals:     []string{"domain:" + emailDomain, "parent:" + knownDomain},
				LLMUsed:     false,
			}
		}
	}

	return nil
}

// getCategoryBonus returns a bonus score based on category importance
func getCategoryBonus(cat domain.EmailCategory) float64 {
	switch cat {
	case domain.CategoryFinance:
		return 0.15 // Financial emails are important
	case domain.CategoryWork:
		return 0.10
	case domain.CategoryTravel:
		return 0.08
	case domain.CategoryShopping:
		return 0.05
	case domain.CategorySocial:
		return 0.02
	case domain.CategoryNewsletter:
		return 0.00
	case domain.CategoryMarketing:
		return -0.05 // Lower priority for marketing
	case domain.CategorySpam:
		return -0.10
	default:
		return 0.00
	}
}

func (c *DomainScoreClassifier) checkDeveloperDomains(emailDomain string) *ScoreClassifierResult {
	return c.checkDomainMap(emailDomain, c.developerDomains)
}

func (c *DomainScoreClassifier) checkFinanceDomains(emailDomain string) *ScoreClassifierResult {
	return c.checkDomainMap(emailDomain, c.financeDomains)
}

func (c *DomainScoreClassifier) checkShoppingDomains(emailDomain string) *ScoreClassifierResult {
	return c.checkDomainMap(emailDomain, c.shoppingDomains)
}

func (c *DomainScoreClassifier) checkTravelDomains(emailDomain string) *ScoreClassifierResult {
	return c.checkDomainMap(emailDomain, c.travelDomains)
}

func (c *DomainScoreClassifier) checkSocialDomains(emailDomain string) *ScoreClassifierResult {
	return c.checkDomainMap(emailDomain, c.socialDomains)
}

func (c *DomainScoreClassifier) checkProductivityDomains(emailDomain string) *ScoreClassifierResult {
	return c.checkDomainMap(emailDomain, c.productivityDomains)
}

// =============================================================================
// Domain Initialization
// =============================================================================

func (c *DomainScoreClassifier) initDeveloperDomains() {
	c.developerDomains = map[string]domainConfig{
		// === Version Control & Code Hosting ===
		"github.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:github",
		},
		"gitlab.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:gitlab",
		},
		"bitbucket.org": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:bitbucket",
		},

		// === CI/CD & Deployment ===
		"vercel.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:vercel",
		},
		"netlify.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:netlify",
		},
		"railway.app": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:railway",
		},
		"render.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:render",
		},
		"fly.io": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:fly",
		},
		"heroku.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:heroku",
		},
		"circleci.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:circleci",
		},
		"travis-ci.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:travis",
		},

		// === Cloud Providers ===
		"amazonaws.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:aws",
		},
		"cloud.google.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:gcp",
		},
		"azure.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:azure",
		},
		"digitalocean.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:digitalocean",
		},

		// === Monitoring & Error Tracking ===
		"sentry.io": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityHigh,
			score:       0.92,
			source:      "domain:sentry",
		},
		"datadoghq.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:datadog",
		},
		"newrelic.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:newrelic",
		},
		"pagerduty.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:pagerduty",
		},
		"opsgenie.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:opsgenie",
		},

		// === Project Management ===
		"atlassian.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:atlassian",
		},
		"atlassian.net": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:atlassian",
		},
		"linear.app": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:linear",
		},
		"asana.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:asana",
		},
		"monday.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:monday",
		},

		// === Database & Backend Services ===
		"supabase.io": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:supabase",
		},
		"supabase.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:supabase",
		},
		"firebase.google.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:firebase",
		},
		"mongodb.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:mongodb",
		},

		// === Package Registries ===
		"npmjs.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityLow,
			score:       0.85,
			source:      "domain:npm",
		},
		"pypi.org": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityLow,
			score:       0.85,
			source:      "domain:pypi",
		},

		// === API & Developer Tools ===
		"postman.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:postman",
		},
		"docker.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:docker",
		},

		// === Uptime & Status Monitoring ===
		"grafana.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.92,
			source:      "domain:grafana",
		},
		"grafana.net": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.92,
			source:      "domain:grafana",
		},
		"uptimerobot.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:uptimerobot",
		},
		"pingdom.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:pingdom",
		},
		"statuspage.io": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:statuspage",
		},
		"betteruptime.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.92,
			source:      "domain:betteruptime",
		},

		// === CDN & Infrastructure ===
		"cloudflare.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:cloudflare",
		},
		"fastly.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:fastly",
		},
		"akamai.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:akamai",
		},

		// === Security ===
		"snyk.io": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityHigh,
			score:       0.92,
			source:      "domain:snyk",
		},
		"dependabot.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryAlert,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:dependabot",
		},
		"sonarcloud.io": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:sonarcloud",
		},
		"codecov.io": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:codecov",
		},

		// === AI/ML Platforms ===
		"openai.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:openai",
		},
		"anthropic.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:anthropic",
		},
		"huggingface.co": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:huggingface",
		},
		"replicate.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryDeveloper,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:replicate",
		},
	}
}

func (c *DomainScoreClassifier) initFinanceDomains() {
	c.financeDomains = map[string]domainConfig{
		// === Global Fintech ===
		"stripe.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryInvoice,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:stripe",
		},
		"paypal.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryReceipt,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:paypal",
		},
		"plaid.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:plaid",
		},
		"brex.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:brex",
		},
		"ramp.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:ramp",
		},
		"mercury.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:mercury",
		},
		"wise.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:wise",
		},
		"revolut.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:revolut",
		},
		"n26.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:n26",
		},

		// === Crypto ===
		"coinbase.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:coinbase",
		},
		"binance.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:binance",
		},

		// === Korean Finance ===
		"toss.im": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:toss",
		},
		"kakaopay.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryReceipt,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:kakaopay",
		},
		"naverpay.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryReceipt,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:naverpay",
		},

		// === Korean Banks ===
		"kbstar.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:kbbank",
		},
		"shinhan.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:shinhan",
		},
		"wooribank.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:woori",
		},
		"hanabank.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:hana",
		},
		"ibk.co.kr": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:ibk",
		},
		"nhbank.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:nhbank",
		},
		"kakaobank.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:kakaobank",
		},
		"kbanknow.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryAccount,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:kbank",
		},

		// === Credit Cards ===
		"samsungcard.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryInvoice,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:samsungcard",
		},
		"shinhancard.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryInvoice,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:shinhancard",
		},
		"lottemembers.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryInvoice,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:lottecard",
		},
		"hyundaicard.com": {
			category:    domain.CategoryFinance,
			subCategory: domain.SubCategoryInvoice,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:hyundaicard",
		},
	}
}

func (c *DomainScoreClassifier) initShoppingDomains() {
	c.shoppingDomains = map[string]domainConfig{
		// === Global E-commerce ===
		"amazon.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryOrder,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:amazon",
		},
		"shopify.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryOrder,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:shopify",
		},
		"etsy.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryOrder,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:etsy",
		},
		"ebay.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryOrder,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:ebay",
		},
		"aliexpress.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryOrder,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:aliexpress",
		},

		// === Korean E-commerce ===
		"coupang.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryOrder,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:coupang",
		},
		"gmarket.co.kr": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryOrder,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:gmarket",
		},
		"11st.co.kr": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryOrder,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:11st",
		},

		// === Food Delivery ===
		"doordash.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryOrder,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:doordash",
		},
		"ubereats.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryOrder,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:ubereats",
		},
		"grubhub.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryOrder,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:grubhub",
		},
		"baemin.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryOrder,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:baemin",
		},

		// === Global Shipping ===
		"fedex.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryShipping,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:fedex",
		},
		"ups.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryShipping,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:ups",
		},
		"dhl.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryShipping,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:dhl",
		},
		"usps.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryShipping,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:usps",
		},

		// === Korean Shipping ===
		"cjlogistics.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryShipping,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:cj",
		},
		"hanjin.co.kr": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryShipping,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:hanjin",
		},
		"logen.co.kr": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryShipping,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:logen",
		},
		"epost.go.kr": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryShipping,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:epost",
		},
		"kunyoung.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryShipping,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:kunyoung",
		},
		"goodsflow.com": {
			category:    domain.CategoryShopping,
			subCategory: domain.SubCategoryShipping,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:goodsflow",
		},
	}
}

func (c *DomainScoreClassifier) initTravelDomains() {
	c.travelDomains = map[string]domainConfig{
		// === Booking ===
		"booking.com": {
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:booking",
		},
		"airbnb.com": {
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:airbnb",
		},
		"expedia.com": {
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:expedia",
		},
		"tripadvisor.com": {
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:tripadvisor",
		},
		"hotels.com": {
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:hotels",
		},

		// === Airlines ===
		"united.com": {
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:united",
		},
		"delta.com": {
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:delta",
		},
		"aa.com": {
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:american-airlines",
		},
		"koreanair.com": {
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:koreanair",
		},
		"asiana.com": {
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityHigh,
			score:       0.95,
			source:      "domain:asiana",
		},

		// === Ride Sharing ===
		"uber.com": {
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:uber",
		},
		"lyft.com": {
			category:    domain.CategoryTravel,
			subCategory: domain.SubCategoryTravel,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:lyft",
		},
	}
}

func (c *DomainScoreClassifier) initSocialDomains() {
	c.socialDomains = map[string]domainConfig{
		// === Social Media ===
		"facebook.com": {
			category:    domain.CategorySocial,
			subCategory: domain.SubCategorySNS,
			priority:    domain.PriorityLow,
			score:       0.90,
			source:      "domain:facebook",
		},
		"facebookmail.com": {
			category:    domain.CategorySocial,
			subCategory: domain.SubCategorySNS,
			priority:    domain.PriorityLow,
			score:       0.90,
			source:      "domain:facebook",
		},
		"instagram.com": {
			category:    domain.CategorySocial,
			subCategory: domain.SubCategorySNS,
			priority:    domain.PriorityLow,
			score:       0.90,
			source:      "domain:instagram",
		},
		"twitter.com": {
			category:    domain.CategorySocial,
			subCategory: domain.SubCategorySNS,
			priority:    domain.PriorityLow,
			score:       0.90,
			source:      "domain:twitter",
		},
		"x.com": {
			category:    domain.CategorySocial,
			subCategory: domain.SubCategorySNS,
			priority:    domain.PriorityLow,
			score:       0.90,
			source:      "domain:x",
		},
		"linkedin.com": {
			category:    domain.CategorySocial,
			subCategory: domain.SubCategorySNS,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:linkedin",
		},
		"tiktok.com": {
			category:    domain.CategorySocial,
			subCategory: domain.SubCategorySNS,
			priority:    domain.PriorityLow,
			score:       0.90,
			source:      "domain:tiktok",
		},
		"reddit.com": {
			category:    domain.CategorySocial,
			subCategory: domain.SubCategorySNS,
			priority:    domain.PriorityLow,
			score:       0.88,
			source:      "domain:reddit",
		},
		"discord.com": {
			category:    domain.CategorySocial,
			subCategory: domain.SubCategorySNS,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:discord",
		},

		// === Korean Social ===
		"kakaocorp.com": {
			category:    domain.CategorySocial,
			subCategory: domain.SubCategorySNS,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:kakao",
		},
		"naver.com": {
			category:    domain.CategorySocial,
			subCategory: domain.SubCategorySNS,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:naver",
		},
	}
}

func (c *DomainScoreClassifier) initProductivityDomains() {
	c.productivityDomains = map[string]domainConfig{
		// === Productivity & Collaboration ===
		"notion.so": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:notion",
		},
		"slack.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:slack",
		},
		"zoom.us": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryCalendar,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:zoom",
		},
		"calendly.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryCalendar,
			priority:    domain.PriorityNormal,
			score:       0.92,
			source:      "domain:calendly",
		},
		"loom.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:loom",
		},
		"miro.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:miro",
		},
		"figma.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:figma",
		},
		"canva.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityNormal,
			score:       0.85,
			source:      "domain:canva",
		},

		// === Customer Support ===
		"zendesk.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:zendesk",
		},
		"intercom.io": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:intercom",
		},
		"freshdesk.com": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityNormal,
			score:       0.88,
			source:      "domain:freshdesk",
		},

		// === HR & Recruiting ===
		"greenhouse.io": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:greenhouse",
		},
		"lever.co": {
			category:    domain.CategoryWork,
			subCategory: domain.SubCategoryNotification,
			priority:    domain.PriorityNormal,
			score:       0.90,
			source:      "domain:lever",
		},
	}
}
