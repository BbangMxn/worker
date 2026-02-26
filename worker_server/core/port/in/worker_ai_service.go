package in

import (
	"context"
	"time"

	"worker_server/core/agent/tools"
	"worker_server/core/domain"

	"github.com/google/uuid"
)

type AIService interface {
	// Email classification
	ClassifyEmail(ctx context.Context, emailID int64) (*domain.ClassificationResult, error)
	ClassifyEmailBatch(ctx context.Context, emailIDs []int64) ([]*domain.ClassificationResult, error)

	// Email summarization
	// force=true: API 요청 시 길이 관계없이 AI 실행
	// force=false: 자동 처리 시 200자 미만 스킵
	SummarizeEmail(ctx context.Context, emailID int64, force bool) (string, error)
	SummarizeEmailDirect(ctx context.Context, subject, body string, force bool) (string, error)
	SummarizeThread(ctx context.Context, threadID string) (string, error)
	SummarizeEmailWithLang(ctx context.Context, emailID int64, language string) (string, error)
	SummarizeThreadWithLang(ctx context.Context, threadID string, language string) (string, error)

	// Translation
	TranslateEmail(ctx context.Context, emailID int64, targetLang string) (*TranslateEmailResult, error)
	TranslateEmailDirect(ctx context.Context, subject, body, targetLang string) (*TranslateEmailResult, error)
	TranslateText(ctx context.Context, text, targetLang string) (*TranslateTextResult, error)
	DetectLanguage(ctx context.Context, text string) (string, float64, error)

	// Reply generation
	GenerateReply(ctx context.Context, emailID int64, options *ReplyOptions) (string, error)

	// Meeting extraction
	ExtractMeetingInfo(ctx context.Context, emailID int64) (*MeetingInfo, error)

	// Chat (Agent)
	Chat(ctx context.Context, userID uuid.UUID, req *ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, userID uuid.UUID, req *ChatRequest, handler StreamHandler) error

	// Tool execution
	ExecuteTool(ctx context.Context, userID uuid.UUID, toolName string, args map[string]any) (*tools.ToolResult, error)
	ConfirmProposal(ctx context.Context, userID uuid.UUID, proposalID string) (*tools.ToolResult, error)
	ListTools() []tools.ToolDefinition
}

type ReplyOptions struct {
	Tone      string `json:"tone"`   // professional, casual, friendly
	Intent    string `json:"intent"` // accept, decline, question, inform
	Length    string `json:"length"` // short, medium, long
	Context   string `json:"context,omitempty"`
	MaxLength int    `json:"max_length,omitempty"`
}

type MeetingInfo struct {
	Title       string   `json:"title"`
	StartTime   string   `json:"start_time,omitempty"`
	EndTime     string   `json:"end_time,omitempty"`
	Location    string   `json:"location,omitempty"`
	Attendees   []string `json:"attendees,omitempty"`
	Description string   `json:"description,omitempty"`
	MeetingURL  string   `json:"meeting_url,omitempty"`
	HasMeeting  bool     `json:"has_meeting"`
}

type ChatRequest struct {
	Message   string         `json:"message"`
	SessionID string         `json:"session_id,omitempty"`
	Context   map[string]any `json:"context,omitempty"`
}

type ChatResponse struct {
	Message     string              `json:"message"`
	SessionID   string              `json:"session_id"`
	ToolResults []*tools.ToolResult `json:"tool_results,omitempty"`
	Proposals   []ProposalInfo      `json:"proposals,omitempty"`
	Suggestions []string            `json:"suggestions,omitempty"`
	Data        any                 `json:"data,omitempty"`
	TokensUsed  int                 `json:"tokens_used"`
}

type ProposalInfo struct {
	ID          string    `json:"id"`
	Action      string    `json:"action"`
	Description string    `json:"description"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type StreamHandler func(chunk string) error

// =============================================================================
// Translation Types
// =============================================================================

// TranslateEmailResult contains the translated email.
type TranslateEmailResult struct {
	EmailID        int64  `json:"email_id,omitempty"`
	Subject        string `json:"subject"`
	Body           string `json:"body"`
	SourceLanguage string `json:"source_language"`
	TargetLanguage string `json:"target_language"`
}

// TranslateTextResult contains the translated text.
type TranslateTextResult struct {
	TranslatedText string  `json:"translated_text"`
	SourceLanguage string  `json:"source_language"`
	TargetLanguage string  `json:"target_language"`
	Confidence     float64 `json:"confidence"`
}

// SupportedLanguages lists all supported languages for translation.
var SupportedLanguages = map[string]string{
	"ko": "Korean",
	"en": "English",
	"ja": "Japanese",
	"zh": "Chinese",
	"es": "Spanish",
	"fr": "French",
	"de": "German",
	"pt": "Portuguese",
	"it": "Italian",
	"ru": "Russian",
	"ar": "Arabic",
	"vi": "Vietnamese",
	"th": "Thai",
	"id": "Indonesian",
}
