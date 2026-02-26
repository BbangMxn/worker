// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/port/out"
)

// =============================================================================
// PayPal Parser
// =============================================================================
//
// PayPal email patterns:
//   - From: service@paypal.com (primary)
//   - From: service@paypal.co.uk (UK)
//   - From: member@paypal.com (account)
//   - From: billing@paypal.com (billing)
//   - From: no-reply@paypal.com
//
// Subject patterns:
//   - "Notification of Payment Received"
//   - "[item#] - Notification of an instant payment from [buyer]"
//   - "Invoice from [Name] (#[number])"
//   - "Receipt for your payment to [recipient]"
//   - Dispute/claim notifications
//   - Chargeback notifications
//
// Transaction ID format: 12-19 alphanumeric characters
//
// Payment status values:
//   - Completed, Pending, Failed, Denied, Refunded, Reversed
//   - Expired, Voided, Processed, Canceled_Reversal
//
// Legitimate PayPal email characteristics:
//   - Always addresses by full name (not "Dear user")
//   - Links use paypal.com domain
//   - Never asks for passwords via email

// PayPalParser parses PayPal notification emails.
type PayPalParser struct {
	*FinanceBaseParser
}

// NewPayPalParser creates a new PayPal parser.
func NewPayPalParser() *PayPalParser {
	return &PayPalParser{
		FinanceBaseParser: NewFinanceBaseParser(ServicePayPal),
	}
}

// PayPal-specific regex patterns
var (
	// Transaction ID pattern (12-19 alphanumeric)
	paypalTransactionPattern = regexp.MustCompile(`(?i)Transaction\s*(?:ID)?[:\s]*([A-Z0-9]{12,19})`)

	// Subject patterns
	paypalPaymentReceivedPattern = regexp.MustCompile(`(?i)(?:notification\s+of\s+)?(?:payment|money)\s+received`)
	paypalInvoiceSubjectPattern  = regexp.MustCompile(`(?i)invoice\s+from\s+(.+?)\s*(?:\(#?(\d+)\))?`)
	paypalPaymentSentPattern     = regexp.MustCompile(`(?i)receipt\s+for\s+your\s+payment\s+to\s+(.+)`)
	paypalDisputePattern         = regexp.MustCompile(`(?i)dispute|claim|case`)
	paypalChargebackPattern      = regexp.MustCompile(`(?i)chargeback`)
	paypalRefundPattern          = regexp.MustCompile(`(?i)refund`)
	paypalSubscriptionPattern    = regexp.MustCompile(`(?i)(?:automatic\s+payment|subscription|recurring)`)

	// Amount pattern (PayPal format)
	paypalAmountPattern = regexp.MustCompile(`[\$\u00A3\u20AC]\s?([0-9,]+(?:\.[0-9]{2})?)(?:\s*([A-Z]{3}))?`)

	// URL pattern
	paypalURLPattern = regexp.MustCompile(`https://(?:www\.)?paypal\.com/[^\s"<>]+`)

	// Merchant/Buyer pattern
	paypalMerchantPattern = regexp.MustCompile(`(?i)(?:to|from|merchant|seller|buyer)[:\s]*([^<\n]+?)(?:<|$|\n)`)
)

// CanParse checks if this parser can handle the email.
func (p *PayPalParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)

	return strings.Contains(fromLower, "@paypal.com") ||
		strings.Contains(fromLower, "@paypal.co.uk") ||
		strings.Contains(fromLower, "@paypal.") // Other regional domains
}

// Parse extracts structured data from PayPal emails.
func (p *PayPalParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	subject := ""
	bodyText := ""
	if input.Message != nil {
		subject = input.Message.Subject
	}
	if input.Body != nil {
		bodyText = input.Body.Text
	}

	// Detect event
	event := p.detectPayPalEvent(subject, bodyText)

	// Extract data
	data := p.extractData(input)

	// Calculate priority
	eventScore := p.GetEventScoreForEvent(event)
	isUrgent := event == FinEventDisputeCreated || event == FinEventPaymentFailed

	priority, score := p.CalculateFinancePriority(FinancePriorityConfig{
		DomainScore: 0.32, // PayPal is important for payments
		EventScore:  eventScore,
		IsUrgent:    isUrgent,
	})

	// Determine category
	category, subCat := p.DetermineFinanceCategory(event)

	// Generate action items
	actionItems := p.GenerateFinanceActionItems(event, data)

	// Generate entities
	entities := p.GenerateFinanceEntities(data)

	return &ParsedEmail{
		Category:      CategoryFinance,
		Service:       ServicePayPal,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:paypal:" + string(event),
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"paypal", "event:" + string(event)},
	}, nil
}

// detectPayPalEvent detects the PayPal event from subject and body.
func (p *PayPalParser) detectPayPalEvent(subject, body string) FinanceEventType {
	subjectLower := strings.ToLower(subject)

	// Chargeback (highest priority)
	if paypalChargebackPattern.MatchString(subject) {
		return FinEventDisputeCreated
	}

	// Dispute/Claim
	if paypalDisputePattern.MatchString(subject) {
		return FinEventDisputeCreated
	}

	// Refund
	if paypalRefundPattern.MatchString(subject) {
		return FinEventRefundIssued
	}

	// Payment received
	if paypalPaymentReceivedPattern.MatchString(subject) {
		return FinEventPaymentSucceeded
	}

	// Payment sent (receipt)
	if paypalPaymentSentPattern.MatchString(subject) {
		return FinEventPaymentSucceeded
	}

	// Invoice
	if paypalInvoiceSubjectPattern.MatchString(subject) {
		if strings.Contains(subjectLower, "paid") {
			return FinEventInvoicePaid
		}
		return FinEventInvoiceCreated
	}

	// Subscription/Automatic payment
	if paypalSubscriptionPattern.MatchString(subject) {
		if strings.Contains(subjectLower, "no longer active") || strings.Contains(subjectLower, "cancel") {
			return FinEventSubscriptionEnd
		}
		return FinEventSubscriptionNew
	}

	// Payment failed
	if strings.Contains(subjectLower, "failed") || strings.Contains(subjectLower, "declined") {
		return FinEventPaymentFailed
	}

	// Fallback
	return p.DetectFinanceEvent(subject, body)
}

// extractData extracts structured data from the email.
func (p *PayPalParser) extractData(input *ParserInput) *ExtractedData {
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

	// Extract transaction ID
	if matches := paypalTransactionPattern.FindStringSubmatch(combined); len(matches) >= 2 {
		data.TransactionID = matches[1]
	}

	// Extract amount and currency
	if matches := paypalAmountPattern.FindStringSubmatch(combined); len(matches) >= 2 {
		data.Amount = matches[0]
		if len(matches) >= 3 && matches[2] != "" {
			data.Currency = matches[2]
		}
	}
	if data.Currency == "" {
		data.Currency = p.ExtractCurrency(combined)
	}

	// Extract customer/merchant email
	data.CustomerEmail = p.ExtractCustomerEmail(combined)

	// Extract invoice number from subject
	if matches := paypalInvoiceSubjectPattern.FindStringSubmatch(subject); len(matches) >= 3 && matches[2] != "" {
		data.InvoiceNumber = matches[2]
	}

	// Extract URL
	if matches := paypalURLPattern.FindString(combined); matches != "" {
		data.URL = matches
	}

	// Extract merchant/recipient name
	data.Title = p.extractPayPalTitle(subject)

	return data
}

// extractPayPalTitle extracts merchant or title from subject.
func (p *PayPalParser) extractPayPalTitle(subject string) string {
	// Try invoice pattern
	if matches := paypalInvoiceSubjectPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		return "Invoice from " + strings.TrimSpace(matches[1])
	}

	// Try payment sent pattern
	if matches := paypalPaymentSentPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		return "Payment to " + strings.TrimSpace(matches[1])
	}

	// Try payment received pattern
	if paypalPaymentReceivedPattern.MatchString(subject) {
		return "Payment Received"
	}

	return subject
}
