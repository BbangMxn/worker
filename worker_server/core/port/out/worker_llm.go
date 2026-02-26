package out

import (
	"context"

	"worker_server/core/domain"
)

// LLMClient LLM 클라이언트 인터페이스
type LLMClient interface {
	ExtractSchedules(ctx context.Context, emails []EmailForAnalysis) ([]domain.ScheduleSuggestion, error)
	ExtractActionItems(ctx context.Context, emails []EmailForAnalysis) ([]domain.ActionItem, error)
	SummarizeEmail(ctx context.Context, subject, body string) (string, error)
}

// EmailForAnalysis 분석용 이메일 정보
type EmailForAnalysis struct {
	ID         int64   `db:"id"`
	Subject    string  `db:"subject"`
	From       string  `db:"from_email"`
	FromName   string  `db:"from_name"`
	Body       string  `db:"body"`
	ReceivedAt string  `db:"received_at"`
	IsRead     bool    `db:"is_read"`
	IsStarred  bool    `db:"is_starred"`
	NeedsReply bool    `db:"needs_reply"`
	AICategory string  `db:"ai_category"`
	AIPriority float64 `db:"ai_priority"`
	AISummary  string  `db:"ai_summary"`
}
