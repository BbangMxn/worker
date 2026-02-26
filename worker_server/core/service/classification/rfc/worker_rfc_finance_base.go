// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/domain"
)

// =============================================================================
// Finance Base Parser
// =============================================================================
//
// Common patterns for Stripe, PayPal, and other payment services:
//   - Payment: Succeeded, Failed, Refunded
//   - Invoice: Created, Paid, Overdue
//   - Subscription: New, Cancelled, Renewed
//   - Dispute: Created, Resolved
//   - Payout: Paid, Failed

// FinanceBaseParser provides common functionality for finance tool parsers.
type FinanceBaseParser struct {
	service  SaaSService
	category SaaSCategory
}

// NewFinanceBaseParser creates a new base parser.
func NewFinanceBaseParser(service SaaSService) *FinanceBaseParser {
	return &FinanceBaseParser{
		service:  service,
		category: CategoryFinance,
	}
}

// Service returns the service.
func (p *FinanceBaseParser) Service() SaaSService {
	return p.service
}

// Category returns the category.
func (p *FinanceBaseParser) Category() SaaSCategory {
	return p.category
}

// =============================================================================
// Common Regex Patterns
// =============================================================================

var (
	// Amount patterns (supports $, €, £, and numeric amounts)
	financeAmountPattern   = regexp.MustCompile(`[\$\u00A3\u20AC]\s?([0-9,]+(?:\.[0-9]{2})?)`)
	financeCurrencyPattern = regexp.MustCompile(`\b(USD|EUR|GBP|CAD|AUD|JPY|CHF|CNY|HKD|SGD|KRW)\b`)

	// Transaction ID patterns
	financeTransactionPattern = regexp.MustCompile(`(?i)transaction\s*(?:id|#)?[:\s]*([A-Z0-9-]+)`)
	financeInvoicePattern     = regexp.MustCompile(`(?i)invoice\s*(?:#|number)?[:\s]*([A-Z0-9-]+)`)

	// Event patterns
	financePaymentSucceededPattern = regexp.MustCompile(`(?i)(?:payment|charge)\s+(?:succeeded|successful|completed|received)`)
	financePaymentFailedPattern    = regexp.MustCompile(`(?i)(?:payment|charge)\s+(?:failed|declined|unsuccessful)`)
	financeRefundPattern           = regexp.MustCompile(`(?i)refund`)
	financeInvoicePaidPattern      = regexp.MustCompile(`(?i)invoice\s+(?:paid|payment)`)
	financeInvoiceCreatedPattern   = regexp.MustCompile(`(?i)(?:new\s+)?invoice`)
	financeSubscriptionPattern     = regexp.MustCompile(`(?i)subscription`)
	financeDisputePattern          = regexp.MustCompile(`(?i)dispute|chargeback`)
	financePayoutPattern           = regexp.MustCompile(`(?i)payout`)

	// Customer patterns
	financeCustomerEmailPattern = regexp.MustCompile(`(?i)(?:customer|buyer|from)[:\s]*([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`)
)

// =============================================================================
// Common Extraction Methods
// =============================================================================

// ExtractAmount extracts amount from text.
func (p *FinanceBaseParser) ExtractAmount(text string) string {
	if matches := financeAmountPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return matches[0] // Include currency symbol
	}
	return ""
}

// ExtractCurrency extracts currency code from text.
func (p *FinanceBaseParser) ExtractCurrency(text string) string {
	// First try explicit currency code
	if matches := financeCurrencyPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return matches[1]
	}

	// Infer from symbol
	if strings.Contains(text, "$") {
		return "USD"
	}
	if strings.Contains(text, "€") || strings.Contains(text, "\u20AC") {
		return "EUR"
	}
	if strings.Contains(text, "£") || strings.Contains(text, "\u00A3") {
		return "GBP"
	}

	return ""
}

// ExtractTransactionID extracts transaction ID from text.
func (p *FinanceBaseParser) ExtractTransactionID(text string) string {
	if matches := financeTransactionPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// ExtractInvoiceNumber extracts invoice number from text.
func (p *FinanceBaseParser) ExtractInvoiceNumber(text string) string {
	if matches := financeInvoicePattern.FindStringSubmatch(text); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// ExtractCustomerEmail extracts customer email from text.
func (p *FinanceBaseParser) ExtractCustomerEmail(text string) string {
	if matches := financeCustomerEmailPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// DetectFinanceEvent detects event type from subject/body.
func (p *FinanceBaseParser) DetectFinanceEvent(subject, body string) FinanceEventType {
	combined := subject + " " + body

	switch {
	// Disputes (highest priority)
	case financeDisputePattern.MatchString(combined):
		return FinEventDisputeCreated

	// Payment events
	case financePaymentFailedPattern.MatchString(combined):
		return FinEventPaymentFailed
	case financeRefundPattern.MatchString(combined):
		return FinEventRefundIssued
	case financePaymentSucceededPattern.MatchString(combined):
		return FinEventPaymentSucceeded

	// Invoice events
	case financeInvoicePaidPattern.MatchString(combined):
		return FinEventInvoicePaid
	case financeInvoiceCreatedPattern.MatchString(combined):
		return FinEventInvoiceCreated

	// Subscription events
	case financeSubscriptionPattern.MatchString(combined):
		if strings.Contains(strings.ToLower(combined), "cancel") {
			return FinEventSubscriptionEnd
		}
		return FinEventSubscriptionNew

	// Payout events
	case financePayoutPattern.MatchString(combined):
		return FinEventPayoutPaid
	}

	return FinEventPaymentSucceeded
}

// =============================================================================
// Common Priority Calculation
// =============================================================================

// FinancePriorityConfig holds priority calculation parameters.
type FinancePriorityConfig struct {
	DomainScore float64
	EventScore  float64
	IsUrgent    bool
}

// CalculateFinancePriority calculates priority for finance tools.
func (p *FinanceBaseParser) CalculateFinancePriority(config FinancePriorityConfig) (domain.Priority, float64) {
	// Weighted calculation
	score := config.DomainScore*0.3 + config.EventScore*0.7

	// Urgent flag boosts priority
	if config.IsUrgent {
		score = finMin(score*1.3, 1.0)
	}

	return p.ScoreToPriority(score), score
}

// GetEventScoreForEvent returns event score for finance events.
func (p *FinanceBaseParser) GetEventScoreForEvent(event FinanceEventType) float64 {
	switch event {
	// Critical - requires immediate action
	case FinEventDisputeCreated:
		return 0.95
	case FinEventPaymentFailed:
		return 0.85

	// Important - revenue impact
	case FinEventSubscriptionEnd:
		return 0.7
	case FinEventRefundIssued:
		return 0.65

	// Normal - informational
	case FinEventPaymentSucceeded:
		return 0.4
	case FinEventInvoicePaid:
		return 0.4
	case FinEventSubscriptionNew:
		return 0.5
	case FinEventPayoutPaid:
		return 0.35
	case FinEventInvoiceCreated:
		return 0.3

	default:
		return 0.3
	}
}

// ScoreToPriority converts score to Priority.
func (p *FinanceBaseParser) ScoreToPriority(score float64) domain.Priority {
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

// DetermineFinanceCategory determines category based on event type.
func (p *FinanceBaseParser) DetermineFinanceCategory(event FinanceEventType) (domain.EmailCategory, *domain.EmailSubCategory) {
	alertSubCat := domain.SubCategoryAlert
	disputeSubCat := domain.SubCategoryDispute
	paymentSubCat := domain.SubCategoryPayment
	invoiceSubCat := domain.SubCategoryInvoice
	subscriptionSubCat := domain.SubCategorySubscription
	payoutSubCat := domain.SubCategoryPayout
	refundSubCat := domain.SubCategoryRefund

	switch event {
	// Urgent action needed - disputes
	case FinEventDisputeCreated:
		return domain.CategoryFinance, &disputeSubCat

	// Payment failures need attention
	case FinEventPaymentFailed:
		return domain.CategoryWork, &alertSubCat

	// Payment events
	case FinEventPaymentSucceeded:
		return domain.CategoryFinance, &paymentSubCat

	// Invoice events
	case FinEventInvoicePaid, FinEventInvoiceCreated:
		return domain.CategoryFinance, &invoiceSubCat

	// Subscription events
	case FinEventSubscriptionNew, FinEventSubscriptionEnd:
		return domain.CategoryFinance, &subscriptionSubCat

	// Payout events
	case FinEventPayoutPaid:
		return domain.CategoryFinance, &payoutSubCat

	// Refund events
	case FinEventRefundIssued:
		return domain.CategoryFinance, &refundSubCat

	default:
		return domain.CategoryFinance, &paymentSubCat
	}
}

// =============================================================================
// Common Action Item Generation
// =============================================================================

// GenerateFinanceActionItems generates action items based on event type.
func (p *FinanceBaseParser) GenerateFinanceActionItems(event FinanceEventType, data *ExtractedData) []ActionItem {
	var items []ActionItem

	switch event {
	case FinEventDisputeCreated:
		items = append(items, ActionItem{
			Type:        ActionRespond,
			Title:       "Respond to dispute: " + data.TransactionID,
			Description: "Amount: " + data.Amount,
			URL:         data.URL,
			Priority:    "urgent",
		})

	case FinEventPaymentFailed:
		items = append(items, ActionItem{
			Type:        ActionFix,
			Title:       "Resolve payment failure",
			Description: "Customer: " + data.CustomerEmail,
			URL:         data.URL,
			Priority:    "high",
		})

	case FinEventInvoiceCreated:
		items = append(items, ActionItem{
			Type:     ActionPay,
			Title:    "Pay invoice: " + data.InvoiceNumber,
			URL:      data.URL,
			Priority: "medium",
		})

	case FinEventSubscriptionEnd:
		items = append(items, ActionItem{
			Type:        ActionInvestigate,
			Title:       "Review subscription cancellation",
			Description: "Customer: " + data.CustomerEmail,
			URL:         data.URL,
			Priority:    "medium",
		})
	}

	return items
}

// =============================================================================
// Common Entity Extraction
// =============================================================================

// GenerateFinanceEntities generates entities from extracted data.
func (p *FinanceBaseParser) GenerateFinanceEntities(data *ExtractedData) []Entity {
	var entities []Entity

	// Transaction entity
	if data.TransactionID != "" {
		entities = append(entities, Entity{
			Type: EntityInvoice, // Using Invoice type for transactions
			ID:   data.TransactionID,
			Name: "Transaction " + data.TransactionID,
			URL:  data.URL,
		})
	}

	// Invoice entity
	if data.InvoiceNumber != "" {
		entities = append(entities, Entity{
			Type: EntityInvoice,
			ID:   data.InvoiceNumber,
			Name: "Invoice " + data.InvoiceNumber,
			URL:  data.URL,
		})
	}

	// Customer entity
	if data.CustomerEmail != "" {
		entities = append(entities, Entity{
			Type: EntityUser,
			ID:   data.CustomerEmail,
			Name: data.CustomerEmail,
		})
	}

	return entities
}

// Helper function
func finMin(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
