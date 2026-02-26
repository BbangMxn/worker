package search

import (
	"regexp"
	"strings"
	"time"
	"unicode"
)

// QueryAnalyzer analyzes search queries to detect intent and extract components.
type QueryAnalyzer struct {
	// Patterns for detection
	emailPattern    *regexp.Regexp
	datePattern     *regexp.Regexp
	operatorPattern *regexp.Regexp
	filterPattern   *regexp.Regexp
}

// NewQueryAnalyzer creates a new query analyzer.
func NewQueryAnalyzer() *QueryAnalyzer {
	return &QueryAnalyzer{
		emailPattern:    regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		datePattern:     regexp.MustCompile(`\d{4}[-/]\d{2}[-/]\d{2}`),
		operatorPattern: regexp.MustCompile(`\b(AND|OR|NOT)\b`),
		filterPattern:   regexp.MustCompile(`(from|to|subject|has|is|before|after|in|label):[\w@."'-]+`),
	}
}

// Analyze parses and analyzes a search query.
func (a *QueryAnalyzer) Analyze(query string) *ParsedQuery {
	query = strings.TrimSpace(query)

	parsed := &ParsedQuery{
		RawQuery:   query,
		Complexity: 1,
	}

	// Extract structured filters (from:, to:, has:, etc.)
	a.extractFilters(parsed)

	// Extract keywords (remaining text after filter extraction)
	a.extractKeywords(parsed)

	// Detect intent
	parsed.Intent = a.detectIntent(parsed)

	// Calculate complexity
	parsed.Complexity = a.calculateComplexity(parsed)

	// Prepare semantic query (clean text for embedding)
	parsed.SemanticQuery = a.prepareSemanticQuery(parsed)

	return parsed
}

// extractFilters extracts structured filters from the query.
func (a *QueryAnalyzer) extractFilters(parsed *ParsedQuery) {
	query := parsed.RawQuery

	// Extract from:
	if match := regexp.MustCompile(`from:["']?([^"'\s]+)["']?`).FindStringSubmatch(query); len(match) > 1 {
		parsed.From = &match[1]
		query = strings.Replace(query, match[0], "", 1)
	}

	// Extract to:
	if match := regexp.MustCompile(`to:["']?([^"'\s]+)["']?`).FindStringSubmatch(query); len(match) > 1 {
		parsed.To = &match[1]
		query = strings.Replace(query, match[0], "", 1)
	}

	// Extract subject:
	if match := regexp.MustCompile(`subject:["']([^"']+)["']`).FindStringSubmatch(query); len(match) > 1 {
		parsed.Subject = &match[1]
		query = strings.Replace(query, match[0], "", 1)
	} else if match := regexp.MustCompile(`subject:(\S+)`).FindStringSubmatch(query); len(match) > 1 {
		parsed.Subject = &match[1]
		query = strings.Replace(query, match[0], "", 1)
	}

	// Extract has:attachment
	if strings.Contains(query, "has:attachment") {
		hasAttach := true
		parsed.HasAttachment = &hasAttach
		query = strings.Replace(query, "has:attachment", "", 1)
	}

	// Extract is:read / is:unread
	if strings.Contains(query, "is:unread") {
		isRead := false
		parsed.IsRead = &isRead
		query = strings.Replace(query, "is:unread", "", 1)
	} else if strings.Contains(query, "is:read") {
		isRead := true
		parsed.IsRead = &isRead
		query = strings.Replace(query, "is:read", "", 1)
	}

	// Extract is:starred
	if strings.Contains(query, "is:starred") {
		isStarred := true
		parsed.IsStarred = &isStarred
		query = strings.Replace(query, "is:starred", "", 1)
	}

	// Extract after: (date)
	if match := regexp.MustCompile(`after:(\d{4}[-/]\d{2}[-/]\d{2})`).FindStringSubmatch(query); len(match) > 1 {
		if t, err := time.Parse("2006-01-02", strings.ReplaceAll(match[1], "/", "-")); err == nil {
			parsed.DateFrom = &t
		}
		query = strings.Replace(query, match[0], "", 1)
	}

	// Extract before: (date)
	if match := regexp.MustCompile(`before:(\d{4}[-/]\d{2}[-/]\d{2})`).FindStringSubmatch(query); len(match) > 1 {
		if t, err := time.Parse("2006-01-02", strings.ReplaceAll(match[1], "/", "-")); err == nil {
			parsed.DateTo = &t
		}
		query = strings.Replace(query, match[0], "", 1)
	}

	// Extract in: (folder)
	if match := regexp.MustCompile(`in:(\w+)`).FindStringSubmatch(query); len(match) > 1 {
		parsed.Folder = &match[1]
		query = strings.Replace(query, match[0], "", 1)
	}

	// Store cleaned query back (will be used for keywords)
	parsed.RawQuery = strings.TrimSpace(query)
}

// extractKeywords extracts keywords from the remaining query.
func (a *QueryAnalyzer) extractKeywords(parsed *ParsedQuery) {
	query := parsed.RawQuery

	// Remove operators
	query = a.operatorPattern.ReplaceAllString(query, " ")

	// Split into words
	words := strings.Fields(query)

	// Filter out empty and very short words
	keywords := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.Trim(w, `"'`)
		if len(w) >= 2 {
			keywords = append(keywords, w)
		}
	}

	parsed.Keywords = keywords
}

// detectIntent determines the search intent based on query characteristics.
func (a *QueryAnalyzer) detectIntent(parsed *ParsedQuery) SearchIntent {
	// If has structured filters, it's structured or hybrid
	hasFilters := parsed.From != nil || parsed.To != nil || parsed.Subject != nil ||
		parsed.HasAttachment != nil || parsed.IsRead != nil || parsed.DateFrom != nil

	hasKeywords := len(parsed.Keywords) > 0

	// Check for natural language characteristics
	isNaturalLanguage := a.isNaturalLanguage(parsed.Keywords)

	switch {
	case hasFilters && !hasKeywords:
		// Only filters, no keywords
		return IntentStructured

	case hasFilters && hasKeywords && isNaturalLanguage:
		// Filters + natural language keywords
		return IntentHybrid

	case hasFilters && hasKeywords && !isNaturalLanguage:
		// Filters + keyword-like terms
		return IntentStructured

	case !hasFilters && hasKeywords && isNaturalLanguage:
		// Pure natural language query
		return IntentSemantic

	case !hasFilters && hasKeywords && !isNaturalLanguage:
		// Simple keyword search
		return IntentKeyword

	default:
		return IntentKeyword
	}
}

// isNaturalLanguage checks if keywords appear to be natural language.
func (a *QueryAnalyzer) isNaturalLanguage(keywords []string) bool {
	if len(keywords) == 0 {
		return false
	}

	// Check for non-ASCII (Korean, Japanese, etc.)
	fullText := strings.Join(keywords, " ")
	for _, r := range fullText {
		if r > 127 && unicode.IsLetter(r) {
			return true
		}
	}

	// Check for sentence-like structure (3+ words)
	if len(keywords) >= 3 {
		return true
	}

	// Check for common natural language patterns
	naturalPatterns := []string{
		"about", "related", "regarding", "concerning",
		"with", "from", "like", "similar",
		"recent", "latest", "last", "this",
		"관련", "에게", "부터", "처럼", "같은",
	}

	for _, kw := range keywords {
		kwLower := strings.ToLower(kw)
		for _, pattern := range naturalPatterns {
			if strings.Contains(kwLower, pattern) {
				return true
			}
		}
	}

	// Check query length
	if len(fullText) > 30 {
		return true
	}

	return false
}

// calculateComplexity determines query complexity (1-5).
func (a *QueryAnalyzer) calculateComplexity(parsed *ParsedQuery) int {
	complexity := 1

	// Add complexity for each filter
	if parsed.From != nil {
		complexity++
	}
	if parsed.To != nil {
		complexity++
	}
	if parsed.Subject != nil {
		complexity++
	}
	if parsed.HasAttachment != nil {
		complexity++
	}
	if parsed.DateFrom != nil || parsed.DateTo != nil {
		complexity++
	}

	// Add complexity for keywords
	if len(parsed.Keywords) > 2 {
		complexity++
	}

	// Cap at 5
	if complexity > 5 {
		complexity = 5
	}

	return complexity
}

// prepareSemanticQuery creates a clean query string for embedding.
func (a *QueryAnalyzer) prepareSemanticQuery(parsed *ParsedQuery) string {
	parts := make([]string, 0)

	// Add keywords
	if len(parsed.Keywords) > 0 {
		parts = append(parts, strings.Join(parsed.Keywords, " "))
	}

	// Add subject if present (important for semantic meaning)
	if parsed.Subject != nil {
		parts = append(parts, *parsed.Subject)
	}

	return strings.Join(parts, " ")
}

// HasStructuredFilters checks if query has any structured filters.
func (parsed *ParsedQuery) HasStructuredFilters() bool {
	return parsed.From != nil || parsed.To != nil || parsed.Subject != nil ||
		parsed.HasAttachment != nil || parsed.IsRead != nil || parsed.IsStarred != nil ||
		parsed.DateFrom != nil || parsed.DateTo != nil || parsed.Folder != nil
}

// IsEmpty checks if the parsed query has no meaningful content.
func (parsed *ParsedQuery) IsEmpty() bool {
	return len(parsed.Keywords) == 0 && !parsed.HasStructuredFilters()
}
