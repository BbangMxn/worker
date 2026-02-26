package search

import (
	"fmt"
	"strings"
)

// QueryTransformer converts parsed queries to provider-specific formats.
type QueryTransformer struct{}

// NewQueryTransformer creates a new query transformer.
func NewQueryTransformer() *QueryTransformer {
	return &QueryTransformer{}
}

// ToGmailQuery converts a parsed query to Gmail search syntax.
// Gmail supports: from:, to:, subject:, has:attachment, is:unread, after:, before:, in:, label:
func (t *QueryTransformer) ToGmailQuery(parsed *ParsedQuery) string {
	var parts []string

	// Add keywords (search in body and subject)
	for _, kw := range parsed.Keywords {
		// Quote if contains spaces
		if strings.Contains(kw, " ") {
			parts = append(parts, fmt.Sprintf(`"%s"`, kw))
		} else {
			parts = append(parts, kw)
		}
	}

	// Add structured filters
	if parsed.From != nil {
		parts = append(parts, fmt.Sprintf("from:%s", *parsed.From))
	}

	if parsed.To != nil {
		parts = append(parts, fmt.Sprintf("to:%s", *parsed.To))
	}

	if parsed.Subject != nil {
		parts = append(parts, fmt.Sprintf("subject:%s", quoteIfNeeded(*parsed.Subject)))
	}

	if parsed.HasAttachment != nil && *parsed.HasAttachment {
		parts = append(parts, "has:attachment")
	}

	if parsed.IsRead != nil {
		if *parsed.IsRead {
			parts = append(parts, "is:read")
		} else {
			parts = append(parts, "is:unread")
		}
	}

	if parsed.IsStarred != nil && *parsed.IsStarred {
		parts = append(parts, "is:starred")
	}

	if parsed.DateFrom != nil {
		parts = append(parts, fmt.Sprintf("after:%s", parsed.DateFrom.Format("2006/01/02")))
	}

	if parsed.DateTo != nil {
		parts = append(parts, fmt.Sprintf("before:%s", parsed.DateTo.Format("2006/01/02")))
	}

	if parsed.Folder != nil {
		parts = append(parts, fmt.Sprintf("in:%s", *parsed.Folder))
	}

	return strings.Join(parts, " ")
}

// ToOutlookQuery converts a parsed query to Outlook search syntax (KQL).
// Outlook $search supports: from:, subject:, hasAttachments:, etc.
// Outlook $filter supports: isRead, receivedDateTime, etc.
func (t *QueryTransformer) ToOutlookQuery(parsed *ParsedQuery) (search string, filter string) {
	var searchParts []string
	var filterParts []string

	// Keywords go to $search
	if len(parsed.Keywords) > 0 {
		// Outlook KQL: AND between terms
		searchParts = append(searchParts, strings.Join(parsed.Keywords, " AND "))
	}

	// From: can be in $search or $filter
	if parsed.From != nil {
		searchParts = append(searchParts, fmt.Sprintf(`from:"%s"`, *parsed.From))
	}

	// To: in $search
	if parsed.To != nil {
		searchParts = append(searchParts, fmt.Sprintf(`to:"%s"`, *parsed.To))
	}

	// Subject: in $search
	if parsed.Subject != nil {
		searchParts = append(searchParts, fmt.Sprintf(`subject:"%s"`, *parsed.Subject))
	}

	// HasAttachment: in $filter
	if parsed.HasAttachment != nil {
		filterParts = append(filterParts, fmt.Sprintf("hasAttachments eq %t", *parsed.HasAttachment))
	}

	// IsRead: in $filter
	if parsed.IsRead != nil {
		filterParts = append(filterParts, fmt.Sprintf("isRead eq %t", *parsed.IsRead))
	}

	// Date filters in $filter
	if parsed.DateFrom != nil {
		filterParts = append(filterParts, fmt.Sprintf("receivedDateTime ge %s", parsed.DateFrom.Format("2006-01-02T00:00:00Z")))
	}

	if parsed.DateTo != nil {
		filterParts = append(filterParts, fmt.Sprintf("receivedDateTime lt %s", parsed.DateTo.Format("2006-01-02T00:00:00Z")))
	}

	search = strings.Join(searchParts, " AND ")
	filter = strings.Join(filterParts, " and ")

	return search, filter
}

// ToDBQuery converts a parsed query to PostgreSQL full-text search format.
func (t *QueryTransformer) ToDBQuery(parsed *ParsedQuery) string {
	if len(parsed.Keywords) == 0 {
		return ""
	}

	// Convert keywords to tsquery format
	// "vacation planning" → "vacation & planning"
	terms := make([]string, 0, len(parsed.Keywords))
	for _, kw := range parsed.Keywords {
		// Escape special characters
		kw = strings.ReplaceAll(kw, "'", "''")
		kw = strings.ReplaceAll(kw, "&", "")
		kw = strings.ReplaceAll(kw, "|", "")
		kw = strings.ReplaceAll(kw, "!", "")

		if kw != "" {
			terms = append(terms, kw)
		}
	}

	if len(terms) == 0 {
		return ""
	}

	// Join with AND operator
	return strings.Join(terms, " & ")
}

// FiltersToSQL converts SearchFilters to SQL WHERE clauses.
func (t *QueryTransformer) FiltersToSQL(filters *SearchFilters, parsed *ParsedQuery) (conditions []string, args []interface{}, argIndex int) {
	argIndex = 1
	conditions = make([]string, 0)
	args = make([]interface{}, 0)

	// Merge parsed query filters with explicit filters
	if parsed != nil {
		if parsed.From != nil {
			conditions = append(conditions, fmt.Sprintf("from_email ILIKE $%d", argIndex))
			args = append(args, "%"+*parsed.From+"%")
			argIndex++
		}

		if parsed.To != nil {
			conditions = append(conditions, fmt.Sprintf("to_email ILIKE $%d", argIndex))
			args = append(args, "%"+*parsed.To+"%")
			argIndex++
		}

		if parsed.IsRead != nil {
			conditions = append(conditions, fmt.Sprintf("is_read = $%d", argIndex))
			args = append(args, *parsed.IsRead)
			argIndex++
		}

		if parsed.IsStarred != nil {
			conditions = append(conditions, fmt.Sprintf("is_starred = $%d", argIndex))
			args = append(args, *parsed.IsStarred)
			argIndex++
		}

		if parsed.HasAttachment != nil {
			conditions = append(conditions, fmt.Sprintf("has_attachment = $%d", argIndex))
			args = append(args, *parsed.HasAttachment)
			argIndex++
		}

		if parsed.DateFrom != nil {
			conditions = append(conditions, fmt.Sprintf("email_date >= $%d", argIndex))
			args = append(args, *parsed.DateFrom)
			argIndex++
		}

		if parsed.DateTo != nil {
			conditions = append(conditions, fmt.Sprintf("email_date < $%d", argIndex))
			args = append(args, *parsed.DateTo)
			argIndex++
		}

		if parsed.Folder != nil {
			conditions = append(conditions, fmt.Sprintf("folder = $%d", argIndex))
			args = append(args, *parsed.Folder)
			argIndex++
		}
	}

	// Add explicit filters (override parsed if both present)
	if filters != nil {
		if filters.From != nil && (parsed == nil || parsed.From == nil) {
			conditions = append(conditions, fmt.Sprintf("from_email ILIKE $%d", argIndex))
			args = append(args, "%"+*filters.From+"%")
			argIndex++
		}

		if filters.Category != nil {
			conditions = append(conditions, fmt.Sprintf("ai_category = $%d", argIndex))
			args = append(args, *filters.Category)
			argIndex++
		}

		if filters.Priority != nil {
			conditions = append(conditions, fmt.Sprintf("ai_priority = $%d", argIndex))
			args = append(args, *filters.Priority)
			argIndex++
		}
	}

	return conditions, args, argIndex
}

// quoteIfNeeded wraps a string in quotes if it contains spaces.
func quoteIfNeeded(s string) string {
	if strings.Contains(s, " ") {
		return fmt.Sprintf(`"%s"`, s)
	}
	return s
}

// BuildProviderSearchQuery creates a ProviderSearchQuery from parsed query.
func (t *QueryTransformer) BuildProviderSearchQuery(parsed *ParsedQuery, limit int) *ProviderSearchQuery {
	gmailQuery := t.ToGmailQuery(parsed)
	outlookSearch, _ := t.ToOutlookQuery(parsed)

	return &ProviderSearchQuery{
		GmailQuery:   gmailQuery,
		OutlookQuery: outlookSearch,
		Limit:        limit,
	}
}
