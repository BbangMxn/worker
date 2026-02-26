package worker

import (
	"worker_server/pkg/logger"
	"context"

	"github.com/goccy/go-json"
)

type Handler struct {
	mailProcessor     *MailProcessor
	aiProcessor       *AIProcessor
	ragProcessor      *RAGProcessor
	calendarProcessor *CalendarProcessor
	webhookProcessor  *WebhookProcessor
}

func NewHandler(
	mailProcessor *MailProcessor,
	aiProcessor *AIProcessor,
	ragProcessor *RAGProcessor,
	calendarProcessor *CalendarProcessor,
	webhookProcessor *WebhookProcessor,
) *Handler {
	return &Handler{
		mailProcessor:     mailProcessor,
		aiProcessor:       aiProcessor,
		ragProcessor:      ragProcessor,
		calendarProcessor: calendarProcessor,
		webhookProcessor:  webhookProcessor,
	}
}

func (h *Handler) Process(ctx context.Context, msg *Message) error {
	logger.Debug("Processing message: %s", msg.Type)

	switch msg.Type {
	// Mail jobs
	case JobMailSync:
		return h.mailProcessor.ProcessSync(ctx, msg)
	case JobMailSend:
		return h.mailProcessor.ProcessSend(ctx, msg)
	case JobMailReply:
		return h.mailProcessor.ProcessReply(ctx, msg)
	case JobMailSave:
		return h.mailProcessor.ProcessSave(ctx, msg)
	case JobMailModify:
		return h.mailProcessor.ProcessModify(ctx, msg)

	// AI jobs
	case JobAIClassify:
		return h.aiProcessor.ProcessClassify(ctx, msg)
	case JobAISummarize:
		return h.aiProcessor.ProcessSummarize(ctx, msg)
	case JobAIReply:
		return h.aiProcessor.ProcessGenerateReply(ctx, msg)

	// RAG jobs
	case JobRAGIndex:
		return h.ragProcessor.ProcessIndex(ctx, msg)
	case JobRAGBatchIndex:
		return h.ragProcessor.ProcessBatchIndex(ctx, msg)

	// Calendar jobs
	case JobCalendarSync:
		return h.calendarProcessor.ProcessSync(ctx, msg)

	// Webhook jobs
	case JobWebhookRenew:
		return h.webhookProcessor.ProcessRenew(ctx, msg)

	default:
		logger.Warn("Unknown job type: %s", msg.Type)
		return nil
	}
}

func ParsePayload[T any](msg *Message) (*T, error) {
	var payload T
	data, err := json.Marshal(msg.Payload)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}
