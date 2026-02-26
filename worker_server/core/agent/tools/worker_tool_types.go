package tools

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Tool represents a tool that AI Agent can use
type Tool interface {
	Name() string
	Description() string
	Category() ToolCategory
	Parameters() []ParameterSpec
	Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error)
}

// ToolCategory categorizes tools
type ToolCategory string

const (
	CategoryMail     ToolCategory = "mail"
	CategoryCalendar ToolCategory = "calendar"
	CategoryContact  ToolCategory = "contact"
	CategorySearch   ToolCategory = "search"
	CategoryAnalysis ToolCategory = "analysis"
)

// ParameterSpec defines a tool parameter
type ParameterSpec struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // string, number, boolean, array, object
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Enum        []string `json:"enum,omitempty"` // allowed values
	Default     any      `json:"default,omitempty"`
}

// ToolResult represents the result of tool execution
type ToolResult struct {
	Success  bool            `json:"success"`
	Data     any             `json:"data,omitempty"`
	Message  string          `json:"message,omitempty"`
	Error    string          `json:"error,omitempty"`
	Proposal *ActionProposal `json:"proposal,omitempty"` // for actions requiring confirmation
}

// ActionProposal represents a proposed action for user confirmation
type ActionProposal struct {
	ID          string         `json:"id"`
	Action      string         `json:"action"`
	Description string         `json:"description"`
	Data        map[string]any `json:"data"`
	ExpiresAt   time.Time      `json:"expires_at"`
}

// ToolDefinition for LLM function calling
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Category    ToolCategory   `json:"category"`
	Parameters  ToolParameters `json:"parameters"`
}

// ToolParameters for OpenAI function calling format
type ToolParameters struct {
	Type       string                       `json:"type"`
	Properties map[string]ParameterProperty `json:"properties"`
	Required   []string                     `json:"required"`
}

// ParameterProperty for OpenAI format
type ParameterProperty struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
}

// ToolCall represents a tool call from LLM
type ToolCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// ConvertToDefinition converts Tool to ToolDefinition for LLM
func ConvertToDefinition(t Tool) ToolDefinition {
	params := t.Parameters()
	properties := make(map[string]ParameterProperty)
	required := []string{}

	for _, p := range params {
		properties[p.Name] = ParameterProperty{
			Type:        p.Type,
			Description: p.Description,
			Enum:        p.Enum,
		}
		if p.Required {
			required = append(required, p.Name)
		}
	}

	return ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Category:    t.Category(),
		Parameters: ToolParameters{
			Type:       "object",
			Properties: properties,
			Required:   required,
		},
	}
}
