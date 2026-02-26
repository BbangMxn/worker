package tools

import (
	"context"
	"fmt"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

// ContactListTool lists contacts
type ContactListTool struct {
	contactRepo domain.ContactRepository
}

func NewContactListTool(contactRepo domain.ContactRepository) *ContactListTool {
	return &ContactListTool{contactRepo: contactRepo}
}

func (t *ContactListTool) Name() string           { return "contact.list" }
func (t *ContactListTool) Category() ToolCategory { return CategoryContact }

func (t *ContactListTool) Description() string {
	return "List contacts from address book. Can search by name or email."
}

func (t *ContactListTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "search", Type: "string", Description: "Search by name or email"},
		{Name: "group", Type: "string", Description: "Filter by contact group"},
		{Name: "limit", Type: "number", Description: "Maximum contacts to return", Default: 20},
	}
}

func (t *ContactListTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	search := getStringArg(args, "search", "")
	group := getStringArg(args, "group", "")
	limit := getIntArg(args, "limit", 20)

	filter := &domain.ContactFilter{
		UserID: userID,
		Limit:  limit,
	}

	if search != "" {
		filter.Search = &search
	}
	if group != "" {
		filter.Group = &group
	}

	contacts, total, err := t.contactRepo.List(filter)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	// Return summary
	summaries := make([]map[string]any, len(contacts))
	for i, c := range contacts {
		summaries[i] = map[string]any{
			"id":           c.ID,
			"name":         c.Name,
			"email":        c.Email,
			"phone":        c.Phone,
			"company":      c.Company,
			"job_title":    c.JobTitle,
			"last_contact": c.LastContactDate,
		}
	}

	return &ToolResult{
		Success: true,
		Data:    summaries,
		Message: fmt.Sprintf("Found %d contacts (showing %d)", total, len(contacts)),
	}, nil
}

// ContactGetTool retrieves a specific contact
type ContactGetTool struct {
	contactRepo domain.ContactRepository
}

func NewContactGetTool(contactRepo domain.ContactRepository) *ContactGetTool {
	return &ContactGetTool{contactRepo: contactRepo}
}

func (t *ContactGetTool) Name() string           { return "contact.get" }
func (t *ContactGetTool) Category() ToolCategory { return CategoryContact }

func (t *ContactGetTool) Description() string {
	return "Get detailed information about a specific contact."
}

func (t *ContactGetTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "contact_id", Type: "number", Description: "Contact ID", Required: true},
	}
}

func (t *ContactGetTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	contactID := int64(getIntArg(args, "contact_id", 0))
	if contactID == 0 {
		return &ToolResult{Success: false, Error: "contact_id is required"}, nil
	}

	contact, err := t.contactRepo.GetByID(contactID)
	if err != nil {
		return &ToolResult{Success: false, Error: "contact not found"}, nil
	}

	// Verify ownership
	if contact.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	result := map[string]any{
		"id":               contact.ID,
		"name":             contact.Name,
		"email":            contact.Email,
		"phone":            contact.Phone,
		"company":          contact.Company,
		"job_title":        contact.JobTitle,
		"notes":            contact.Notes,
		"last_contact":     contact.LastContactDate,
		"groups":           contact.Groups,
		"interaction_freq": contact.InteractionFrequency,
	}

	return &ToolResult{
		Success: true,
		Data:    result,
	}, nil
}

// ContactSearchTool searches contacts by email
type ContactSearchTool struct {
	contactRepo domain.ContactRepository
}

func NewContactSearchTool(contactRepo domain.ContactRepository) *ContactSearchTool {
	return &ContactSearchTool{contactRepo: contactRepo}
}

func (t *ContactSearchTool) Name() string           { return "contact.search" }
func (t *ContactSearchTool) Category() ToolCategory { return CategoryContact }

func (t *ContactSearchTool) Description() string {
	return "Search for contacts by email address to get their details."
}

func (t *ContactSearchTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "email", Type: "string", Description: "Email address to search", Required: true},
	}
}

func (t *ContactSearchTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	email := getStringArg(args, "email", "")
	if email == "" {
		return &ToolResult{Success: false, Error: "email is required"}, nil
	}

	contact, err := t.contactRepo.GetByEmail(userID, email)
	if err != nil {
		return &ToolResult{
			Success: true,
			Data:    nil,
			Message: fmt.Sprintf("No contact found for %s", email),
		}, nil
	}

	result := map[string]any{
		"id":               contact.ID,
		"name":             contact.Name,
		"email":            contact.Email,
		"phone":            contact.Phone,
		"company":          contact.Company,
		"job_title":        contact.JobTitle,
		"last_contact":     contact.LastContactDate,
		"interaction_freq": contact.InteractionFrequency,
	}

	return &ToolResult{
		Success: true,
		Data:    result,
		Message: fmt.Sprintf("Found contact: %s", contact.Name),
	}, nil
}
