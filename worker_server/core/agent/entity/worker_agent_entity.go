package entity

import (
	"time"

	"github.com/google/uuid"
)

type Agent struct {
	ID        string    `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Name      string    `json:"name"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	ID        string         `json:"id"`
	AgentID   string         `json:"agent_id"`
	UserID    uuid.UUID      `json:"user_id"`
	Messages  []Message      `json:"messages"`
	Context   map[string]any `json:"context"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type Message struct {
	ID        string     `json:"id"`
	Role      string     `json:"role"` // user, assistant, system, tool
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type ToolCall struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Args   map[string]any `json:"args"`
	Result any            `json:"result,omitempty"`
	Error  string         `json:"error,omitempty"`
}

type AgentConfig struct {
	Model        string   `json:"model"`
	Temperature  float64  `json:"temperature"`
	MaxTokens    int      `json:"max_tokens"`
	Tools        []string `json:"tools"`
	SystemPrompt string   `json:"system_prompt"`
}

func DefaultAgentConfig() *AgentConfig {
	return &AgentConfig{
		Model:       "gpt-4-turbo-preview",
		Temperature: 0.7,
		MaxTokens:   4096,
		Tools:       []string{"mail.read", "mail.send", "calendar.list", "calendar.create"},
		SystemPrompt: `You are a helpful email and calendar assistant.
You can help users manage their emails and calendar events.
Always be concise and helpful.`,
	}
}
