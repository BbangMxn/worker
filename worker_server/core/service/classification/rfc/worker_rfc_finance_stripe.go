// Package rfc implements domain-specific email RFC parsers for SaaS tools.
package rfc

import (
	"regexp"
	"strings"

	"worker_server/core/port/out"
)

// =============================================================================
// Stripe Parser
// =============================================================================
//
// Stripe email patterns:
//   - From: *@stripe.com (default)
//   - From: card-issuing-notices@stripe.com (Card Issuing)
//
// Subject patterns:
//   - "Receipt from [Business Name]"
//   - "Your receipt from [Business Name]"
//   - "Invoice #[number] from [Business Name]"
//   - "Invoice for [Service] - Due [Date]"
//   - Dispute notifications
//   - Payout failure notifications
//   - Card expiration warnings
//
// Stripe ID prefixes:
//   - pi_: Payment Intent
//   - ch_: Charge
//   - in_: Invoice
//   - sub_: Subscription
//   - cus_: Customer
//   - pm_: Payment Method
//   - txn_: Balance Transaction
//
// NOTE: No documented X-Stripe-* custom email headers.

// StripeParser parses Stripe notification emails.
type StripeParser struct {
	*FinanceBaseParser
}

// NewStripeParser creates a new Stripe parser.
func NewStripeParser() *StripeParser {
	return &StripeParser{
		FinanceBaseParser: NewFinanceBaseParser(ServiceStripe),
	}
}

// Stripe-specific regex patterns
var (
	// Stripe ID patterns
	stripePaymentIntentPattern = regexp.MustCompile(`pi_[a-zA-Z0-9]{24}`)
	stripeChargePattern        = regexp.MustCompile(`ch_[a-zA-Z0-9]{24}`)
	stripeInvoicePattern       = regexp.MustCompile(`in_[a-zA-Z0-9]{24}`)
	stripeSubscriptionPattern  = regexp.MustCompile(`sub_[a-zA-Z0-9]{24}`)
	stripeCustomerPattern      = regexp.MustCompile(`cus_[a-zA-Z0-9]{14,24}`)

	// Subject patterns
	stripeReceiptPattern        = regexp.MustCompile(`(?i)receipt\s+from\s+(.+)`)
	stripeInvoiceSubjectPattern = regexp.MustCompile(`(?i)invoice\s+(?:#?\d+\s+)?from\s+(.+)`)
	stripePaymentFailedSubject  = regexp.MustCompile(`(?i)(?:payment|charge)\s+(?:failed|declined)`)
	stripeDisputeSubject        = regexp.MustCompile(`(?i)dispute|chargeback`)
	stripeRefundSubject         = regexp.MustCompile(`(?i)refund`)
	stripeTrialEndingSubject    = regexp.MustCompile(`(?i)trial\s+(?:ending|ends)`)
	stripePayoutSubject         = regexp.MustCompile(`(?i)payout`)
	stripeCardExpiringSubject   = regexp.MustCompile(`(?i)card\s+(?:expir|expiry)`)

	// URL pattern
	stripeURLPattern = regexp.MustCompile(`https://(?:dashboard\.)?stripe\.com/[^\s"<>]+`)
)

// CanParse checks if this parser can handle the email.
func (p *StripeParser) CanParse(headers *out.ProviderClassificationHeaders, fromEmail string, rawHeaders map[string]string) bool {
	fromLower := strings.ToLower(fromEmail)

	return strings.Contains(fromLower, "@stripe.com")
}

// Parse extracts structured data from Stripe emails.
func (p *StripeParser) Parse(input *ParserInput) (*ParsedEmail, error) {
	subject := ""
	bodyText := ""
	if input.Message != nil {
		subject = input.Message.Subject
	}
	if input.Body != nil {
		bodyText = input.Body.Text
	}

	// Detect event
	event := p.detectStripeEvent(subject, bodyText)

	// Extract data
	data := p.extractData(input)

	// Calculate priority
	eventScore := p.GetEventScoreForEvent(event)
	isUrgent := event == FinEventDisputeCreated || event == FinEventPaymentFailed

	priority, score := p.CalculateFinancePriority(FinancePriorityConfig{
		DomainScore: 0.35, // Stripe is critical for revenue
		EventScore:  eventScore,
		IsUrgent:    isUrgent,
	})

	// Determine category
	category, subCat := p.DetermineFinanceCategory(event)

	// Generate action items
	actionItems := p.GenerateFinanceActionItems(event, data)

	// Generate entities
	entities := p.generateStripeEntities(data)

	return &ParsedEmail{
		Category:      CategoryFinance,
		Service:       ServiceStripe,
		Event:         string(event),
		EmailCategory: category,
		SubCategory:   subCat,
		Priority:      priority,
		Score:         score,
		Source:        "rfc:stripe:" + string(event),
		Data:          data,
		ActionItems:   actionItems,
		Entities:      entities,
		Signals:       []string{"stripe", "event:" + string(event)},
	}, nil
}

// detectStripeEvent detects the Stripe event from subject and body.
func (p *StripeParser) detectStripeEvent(subject, body string) FinanceEventType {
	// Dispute (highest priority)
	if stripeDisputeSubject.MatchString(subject) {
		return FinEventDisputeCreated
	}

	// Payment failed
	if stripePaymentFailedSubject.MatchString(subject) {
		return FinEventPaymentFailed
	}

	// Refund
	if stripeRefundSubject.MatchString(subject) {
		return FinEventRefundIssued
	}

	// Payout
	if stripePayoutSubject.MatchString(subject) {
		if strings.Contains(strings.ToLower(subject), "failed") {
			return FinEventPaymentFailed
		}
		return FinEventPayoutPaid
	}

	// Trial ending
	if stripeTrialEndingSubject.MatchString(subject) {
		return FinEventSubscriptionEnd
	}

	// Card expiring (treat as warning)
	if stripeCardExpiringSubject.MatchString(subject) {
		return FinEventPaymentFailed
	}

	// Invoice
	if stripeInvoiceSubjectPattern.MatchString(subject) {
		if strings.Contains(strings.ToLower(subject+body), "paid") {
			return FinEventInvoicePaid
		}
		return FinEventInvoiceCreated
	}

	// Receipt (successful payment)
	if stripeReceiptPattern.MatchString(subject) {
		return FinEventPaymentSucceeded
	}

	// Subscription
	if strings.Contains(strings.ToLower(subject), "subscription") {
		if strings.Contains(strings.ToLower(subject), "cancel") {
			return FinEventSubscriptionEnd
		}
		return FinEventSubscriptionNew
	}

	// Fallback
	return p.DetectFinanceEvent(subject, body)
}

// extractData extracts structured data from the email.
func (p *StripeParser) extractData(input *ParserInput) *ExtractedData {
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

	// Extract Stripe IDs
	p.extractStripeIDs(combined, data)

	// Extract amount and currency
	data.Amount = p.ExtractAmount(combined)
	data.Currency = p.ExtractCurrency(combined)

	// Extract customer email
	data.CustomerEmail = p.ExtractCustomerEmail(combined)

	// Extract invoice number (Stripe format)
	if matches := stripeInvoicePattern.FindString(combined); matches != "" {
		data.InvoiceNumber = matches
	}

	// Extract URL
	if matches := stripeURLPattern.FindString(combined); matches != "" {
		data.URL = matches
	}

	// Extract business name from subject
	data.Title = p.extractStripeTitle(subject)

	return data
}

// extractStripeIDs extracts Stripe-specific IDs from text.
func (p *StripeParser) extractStripeIDs(text string, data *ExtractedData) {
	// Payment Intent
	if matches := stripePaymentIntentPattern.FindString(text); matches != "" {
		data.TransactionID = matches
		data.Extra["payment_intent"] = matches
	}

	// Charge
	if matches := stripeChargePattern.FindString(text); matches != "" {
		if data.TransactionID == "" {
			data.TransactionID = matches
		}
		data.Extra["charge_id"] = matches
	}

	// Invoice
	if matches := stripeInvoicePattern.FindString(text); matches != "" {
		data.Extra["invoice_id"] = matches
	}

	// Subscription
	if matches := stripeSubscriptionPattern.FindString(text); matches != "" {
		data.Extra["subscription_id"] = matches
	}

	// Customer
	if matches := stripeCustomerPattern.FindString(text); matches != "" {
		data.CustomerID = matches
	}
}

// extractStripeTitle extracts business name or title from subject.
func (p *StripeParser) extractStripeTitle(subject string) string {
	// Try receipt pattern
	if matches := stripeReceiptPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		return "Receipt from " + strings.TrimSpace(matches[1])
	}

	// Try invoice pattern
	if matches := stripeInvoiceSubjectPattern.FindStringSubmatch(subject); len(matches) >= 2 {
		return "Invoice from " + strings.TrimSpace(matches[1])
	}

	return subject
}

// generateStripeEntities generates Stripe-specific entities.
func (p *StripeParser) generateStripeEntities(data *ExtractedData) []Entity {
	entities := p.GenerateFinanceEntities(data)

	// Add Stripe-specific entities
	if subID, ok := data.Extra["subscription_id"].(string); ok && subID != "" {
		entities = append(entities, Entity{
			Type: EntityService,
			ID:   subID,
			Name: "Subscription " + subID,
		})
	}

	return entities
}
