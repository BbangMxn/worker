package tools

import (
	"context"
	"fmt"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

// SearchEmailTool performs semantic search across emails using RAG
type SearchEmailTool struct {
	emailRepo domain.EmailRepository
	ragSearch domain.RAGSearcher // Interface for RAG search
}

func NewSearchEmailTool(emailRepo domain.EmailRepository, ragSearch domain.RAGSearcher) *SearchEmailTool {
	return &SearchEmailTool{
		emailRepo: emailRepo,
		ragSearch: ragSearch,
	}
}

func (t *SearchEmailTool) Name() string           { return "search.email" }
func (t *SearchEmailTool) Category() ToolCategory { return CategorySearch }

func (t *SearchEmailTool) Description() string {
	return "Semantic search across emails to find relevant messages by meaning, not just keywords."
}

func (t *SearchEmailTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "query", Type: "string", Description: "Search query in natural language", Required: true},
		{Name: "limit", Type: "number", Description: "Maximum results", Default: 5},
		{Name: "include_sent", Type: "boolean", Description: "Include sent emails", Default: true},
		{Name: "include_received", Type: "boolean", Description: "Include received emails", Default: true},
	}
}

func (t *SearchEmailTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	query := getStringArg(args, "query", "")
	if query == "" {
		return &ToolResult{Success: false, Error: "query is required"}, nil
	}

	limit := getIntArg(args, "limit", 5)
	includeSent := getBoolArg(args, "include_sent", true)
	includeReceived := getBoolArg(args, "include_received", true)

	// Use RAG for semantic search
	results, err := t.ragSearch.SearchEmails(ctx, userID, query, limit, includeSent, includeReceived)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	// Format results
	summaries := make([]map[string]any, len(results))
	for i, r := range results {
		summaries[i] = map[string]any{
			"email_id":   r.EmailID,
			"subject":    r.Subject,
			"from":       r.From,
			"date":       r.Date,
			"snippet":    r.Snippet,
			"relevance":  r.Score,
			"ai_summary": r.Summary,
		}
	}

	return &ToolResult{
		Success: true,
		Data:    summaries,
		Message: fmt.Sprintf("Found %d relevant emails for '%s'", len(results), query),
	}, nil
}

// SearchCalendarTool searches calendar events
type SearchCalendarTool struct {
	calendarRepo domain.CalendarRepository
}

func NewSearchCalendarTool(calendarRepo domain.CalendarRepository) *SearchCalendarTool {
	return &SearchCalendarTool{calendarRepo: calendarRepo}
}

func (t *SearchCalendarTool) Name() string           { return "search.calendar" }
func (t *SearchCalendarTool) Category() ToolCategory { return CategorySearch }

func (t *SearchCalendarTool) Description() string {
	return "Search calendar events by keywords, attendees, or location."
}

func (t *SearchCalendarTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "query", Type: "string", Description: "Search keywords", Required: true},
		{Name: "attendee", Type: "string", Description: "Filter by attendee email"},
		{Name: "location", Type: "string", Description: "Filter by location"},
		{Name: "limit", Type: "number", Description: "Maximum results", Default: 10},
	}
}

func (t *SearchCalendarTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	query := getStringArg(args, "query", "")
	attendee := getStringArg(args, "attendee", "")
	location := getStringArg(args, "location", "")
	limit := getIntArg(args, "limit", 10)

	filter := &domain.CalendarEventFilter{
		UserID: userID,
		Search: &query,
		Limit:  limit,
	}

	if attendee != "" {
		filter.Attendee = &attendee
	}
	if location != "" {
		filter.Location = &location
	}

	events, total, err := t.calendarRepo.ListEvents(filter)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	summaries := make([]map[string]any, len(events))
	for i, e := range events {
		summaries[i] = map[string]any{
			"id":         e.ID,
			"title":      e.Title,
			"start_time": e.StartTime.Format("2006-01-02 15:04"),
			"end_time":   e.EndTime.Format("2006-01-02 15:04"),
			"location":   e.Location,
			"attendees":  e.Attendees,
		}
	}

	return &ToolResult{
		Success: true,
		Data:    summaries,
		Message: fmt.Sprintf("Found %d events matching '%s' (total %d)", len(events), query, total),
	}, nil
}

// SearchContactTool searches contacts
type SearchContactTool struct {
	contactRepo domain.ContactRepository
}

func NewSearchContactTool(contactRepo domain.ContactRepository) *SearchContactTool {
	return &SearchContactTool{contactRepo: contactRepo}
}

func (t *SearchContactTool) Name() string           { return "search.contact" }
func (t *SearchContactTool) Category() ToolCategory { return CategorySearch }

func (t *SearchContactTool) Description() string {
	return "Search contacts by name, email, company, or other details."
}

func (t *SearchContactTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "query", Type: "string", Description: "Search query", Required: true},
		{Name: "company", Type: "string", Description: "Filter by company"},
		{Name: "limit", Type: "number", Description: "Maximum results", Default: 10},
	}
}

func (t *SearchContactTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	query := getStringArg(args, "query", "")
	company := getStringArg(args, "company", "")
	limit := getIntArg(args, "limit", 10)

	filter := &domain.ContactFilter{
		UserID: userID,
		Search: &query,
		Limit:  limit,
	}

	if company != "" {
		filter.Company = &company
	}

	contacts, total, err := t.contactRepo.List(filter)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	summaries := make([]map[string]any, len(contacts))
	for i, c := range contacts {
		summaries[i] = map[string]any{
			"id":        c.ID,
			"name":      c.Name,
			"email":     c.Email,
			"phone":     c.Phone,
			"company":   c.Company,
			"job_title": c.JobTitle,
		}
	}

	return &ToolResult{
		Success: true,
		Data:    summaries,
		Message: fmt.Sprintf("Found %d contacts matching '%s' (total %d)", len(contacts), query, total),
	}, nil
}
