package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// Registry manages all available tools
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates a new tool registry
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry
func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

// RegisterAll registers multiple tools
func (r *Registry) RegisterAll(tools ...Tool) {
	for _, tool := range tools {
		r.Register(tool)
	}
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return tool, nil
}

// List returns all registered tools
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// ListByCategory returns tools filtered by category
func (r *Registry) ListByCategory(category ToolCategory) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tools []Tool
	for _, tool := range r.tools {
		if tool.Category() == category {
			tools = append(tools, tool)
		}
	}
	return tools
}

// ListNames returns all tool names
func (r *Registry) ListNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// GetDefinitions returns tool definitions for LLM
func (r *Registry) GetDefinitions() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		defs = append(defs, ConvertToDefinition(tool))
	}
	return defs
}

// Execute runs a tool by name with given arguments
func (r *Registry) Execute(ctx context.Context, userID uuid.UUID, name string, args map[string]any) (*ToolResult, error) {
	tool, err := r.Get(name)
	if err != nil {
		return nil, err
	}

	// Validate required parameters
	for _, param := range tool.Parameters() {
		if param.Required {
			if _, ok := args[param.Name]; !ok {
				return &ToolResult{
					Success: false,
					Error:   fmt.Sprintf("missing required parameter: %s", param.Name),
				}, nil
			}
		}
	}

	return tool.Execute(ctx, userID, args)
}
