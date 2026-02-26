package http

import (
	"fmt"
	"strings"

	"worker_server/core/domain"
)

// =============================================================================
// AI-Only Filter Detection
// =============================================================================

// hasAIOnlyFilters checks if the filter contains AI-specific filters
// that are not supported by Gmail/Outlook APIs.
// If true, we should skip API calls and return DB results only.
func hasAIOnlyFilters(filter *domain.EmailFilter) bool {
	if filter == nil {
		return false
	}

	// AI classification filters (not supported by providers)
	if filter.Category != nil {
		return true
	}
	if filter.SubCategory != nil {
		return true
	}
	if filter.Priority != nil {
		return true
	}

	// Our system-specific filters
	if filter.WorkflowStatus != nil {
		return true
	}
	if len(filter.LabelIDs) > 0 {
		return true
	}

	// FolderID is our internal folder system (different from Gmail labels)
	if filter.FolderID != nil {
		return true
	}

	return false
}

// =============================================================================
// Gmail Query Builder
// =============================================================================

// buildGmailQuery converts EmailFilter to Gmail search query string.
// Gmail uses q parameter with search operators like "in:inbox is:unread from:x@y.com"
//
// Supported operators:
//   - in:inbox, in:sent, in:drafts, in:trash, in:spam (folder)
//   - is:read, is:unread (read status)
//   - is:starred (starred)
//   - has:attachment (attachments)
//   - from:email@domain.com (sender)
//   - after:YYYY/MM/DD, before:YYYY/MM/DD (date range)
//   - keyword (search text)
func buildGmailQuery(filter *domain.EmailFilter) string {
	if filter == nil {
		return ""
	}

	var parts []string

	// Folder filter
	if filter.Folder != nil {
		folder := strings.ToLower(string(*filter.Folder))
		switch folder {
		case "inbox":
			parts = append(parts, "in:inbox")
		case "sent":
			parts = append(parts, "in:sent")
		case "drafts":
			parts = append(parts, "in:drafts")
		case "trash":
			parts = append(parts, "in:trash")
		case "spam":
			parts = append(parts, "in:spam")
		case "archive":
			parts = append(parts, "-in:inbox -in:spam -in:trash")
		default:
			// Custom label
			parts = append(parts, fmt.Sprintf("label:%s", folder))
		}
	}

	// Read status
	if filter.IsRead != nil {
		if *filter.IsRead {
			parts = append(parts, "is:read")
		} else {
			parts = append(parts, "is:unread")
		}
	}

	// Starred
	if filter.IsStarred != nil && *filter.IsStarred {
		parts = append(parts, "is:starred")
	}

	// From email
	if filter.FromEmail != nil && *filter.FromEmail != "" {
		parts = append(parts, fmt.Sprintf("from:%s", *filter.FromEmail))
	}

	// From domain
	if filter.FromDomain != nil && *filter.FromDomain != "" {
		parts = append(parts, fmt.Sprintf("from:@%s", *filter.FromDomain))
	}

	// Date range (Gmail format: YYYY/MM/DD)
	if filter.DateFrom != nil {
		parts = append(parts, fmt.Sprintf("after:%s", filter.DateFrom.Format("2006/01/02")))
	}
	if filter.DateTo != nil {
		parts = append(parts, fmt.Sprintf("before:%s", filter.DateTo.Format("2006/01/02")))
	}

	// Has attachment
	if filter.HasAttachment != nil && *filter.HasAttachment {
		parts = append(parts, "has:attachment")
	}

	// Search text
	if filter.Search != nil && *filter.Search != "" {
		// Gmail treats plain text as keyword search
		parts = append(parts, *filter.Search)
	}

	return strings.Join(parts, " ")
}

// =============================================================================
// Outlook Filter Builder
// =============================================================================

// buildOutlookFilter converts EmailFilter to Outlook OData $filter string.
// Outlook uses $filter parameter with OData syntax like "isRead eq false and from/emailAddress/address eq 'x@y.com'"
//
// Supported filters:
//   - isRead eq true/false (read status)
//   - flag/flagStatus eq 'flagged' (starred/flagged)
//   - hasAttachments eq true (attachments)
//   - from/emailAddress/address eq 'email' (sender)
//   - receivedDateTime ge/le 2024-01-01T00:00:00Z (date range)
//
// Note: Folder is handled via endpoint path, not $filter
func buildOutlookFilter(filter *domain.EmailFilter) string {
	if filter == nil {
		return ""
	}

	var conditions []string

	// Read status
	if filter.IsRead != nil {
		if *filter.IsRead {
			conditions = append(conditions, "isRead eq true")
		} else {
			conditions = append(conditions, "isRead eq false")
		}
	}

	// Starred (flagged in Outlook)
	if filter.IsStarred != nil && *filter.IsStarred {
		conditions = append(conditions, "flag/flagStatus eq 'flagged'")
	}

	// From email (exact match)
	if filter.FromEmail != nil && *filter.FromEmail != "" {
		// Use contains for partial match, eq for exact match
		if strings.Contains(*filter.FromEmail, "@") {
			conditions = append(conditions, fmt.Sprintf("from/emailAddress/address eq '%s'", *filter.FromEmail))
		} else {
			conditions = append(conditions, fmt.Sprintf("contains(from/emailAddress/address, '%s')", *filter.FromEmail))
		}
	}

	// From domain
	if filter.FromDomain != nil && *filter.FromDomain != "" {
		conditions = append(conditions, fmt.Sprintf("endswith(from/emailAddress/address, '@%s')", *filter.FromDomain))
	}

	// Date range (ISO8601 format)
	if filter.DateFrom != nil {
		conditions = append(conditions, fmt.Sprintf("receivedDateTime ge %s", filter.DateFrom.Format("2006-01-02T15:04:05Z")))
	}
	if filter.DateTo != nil {
		conditions = append(conditions, fmt.Sprintf("receivedDateTime le %s", filter.DateTo.Format("2006-01-02T15:04:05Z")))
	}

	// Has attachment
	if filter.HasAttachment != nil && *filter.HasAttachment {
		conditions = append(conditions, "hasAttachments eq true")
	}

	return strings.Join(conditions, " and ")
}

// buildOutlookSearch builds $search parameter for Outlook.
// Note: $filter and $search can be combined in some cases.
func buildOutlookSearch(filter *domain.EmailFilter) string {
	if filter == nil || filter.Search == nil || *filter.Search == "" {
		return ""
	}
	// Outlook $search uses KQL (Keyword Query Language)
	return fmt.Sprintf("\"%s\"", *filter.Search)
}

// getOutlookFolderPath returns the Outlook folder path for the filter.
// Outlook uses folder path in URL, not in $filter.
func getOutlookFolderPath(filter *domain.EmailFilter) string {
	if filter == nil || filter.Folder == nil {
		return "messages" // All messages
	}

	folder := strings.ToLower(string(*filter.Folder))
	switch folder {
	case "inbox":
		return "mailFolders/inbox/messages"
	case "sent":
		return "mailFolders/sentitems/messages"
	case "drafts":
		return "mailFolders/drafts/messages"
	case "trash":
		return "mailFolders/deleteditems/messages"
	case "spam":
		return "mailFolders/junkemail/messages"
	case "archive":
		return "mailFolders/archive/messages"
	default:
		return "messages"
	}
}

// =============================================================================
// Provider Filter Options
// =============================================================================

// ProviderFilterOptions contains filter options for provider API calls.
type ProviderFilterOptions struct {
	// Gmail
	GmailQuery string

	// Outlook
	OutlookFilter string
	OutlookSearch string
	OutlookFolder string

	// Common
	MaxResults int
	PageToken  string

	// Skip API call flag
	SkipAPICall bool
	SkipReason  string
}

// BuildProviderFilterOptions creates filter options for both Gmail and Outlook.
func BuildProviderFilterOptions(filter *domain.EmailFilter, limit int, pageToken string) *ProviderFilterOptions {
	opts := &ProviderFilterOptions{
		MaxResults: limit,
		PageToken:  pageToken,
	}

	// Check if we should skip API call
	if hasAIOnlyFilters(filter) {
		opts.SkipAPICall = true
		opts.SkipReason = "AI-only filters present (category, sub_category, priority, workflow_status, label_ids)"
		return opts
	}

	// Build Gmail query
	opts.GmailQuery = buildGmailQuery(filter)

	// Build Outlook filter
	opts.OutlookFilter = buildOutlookFilter(filter)
	opts.OutlookSearch = buildOutlookSearch(filter)
	opts.OutlookFolder = getOutlookFolderPath(filter)

	return opts
}
