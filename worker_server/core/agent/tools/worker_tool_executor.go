package tools

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Executor wraps the Registry and provides tool execution
type Executor struct {
	registry *Registry
}

// NewExecutor creates a new tool executor
func NewExecutor(registry *Registry) *Executor {
	return &Executor{registry: registry}
}

// Execute runs a single tool by name
func (e *Executor) Execute(ctx context.Context, userID uuid.UUID, call *ToolCall) (*ToolResult, error) {
	tool, err := e.registry.Get(call.Name)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool not found: %s", call.Name),
		}, nil
	}

	// Validate required parameters
	for _, param := range tool.Parameters() {
		if param.Required {
			if _, ok := call.Args[param.Name]; !ok {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("missing required parameter: %s", param.Name),
				}, nil
			}
		}
	}

	return tool.Execute(ctx, userID, call.Args)
}

// ExecuteMultiple runs multiple tool calls
func (e *Executor) ExecuteMultiple(ctx context.Context, userID uuid.UUID, calls []ToolCall) ([]*ToolResult, error) {
	results := make([]*ToolResult, len(calls))

	for i, call := range calls {
		result, err := e.Execute(ctx, userID, &call)
		if err != nil {
			results[i] = &ToolResult{
				Success: false,
				Error:   err.Error(),
			}
		} else {
			results[i] = result
		}
	}

	return results, nil
}

// GetAvailableTools returns all available tool definitions
func (e *Executor) GetAvailableTools() []ToolDefinition {
	return e.registry.GetDefinitions()
}

// GetToolsByCategory returns tools filtered by category
func (e *Executor) GetToolsByCategory(category ToolCategory) []Tool {
	return e.registry.ListByCategory(category)
}
