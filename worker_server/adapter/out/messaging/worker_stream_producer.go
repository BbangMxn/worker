// Package messaging provides message queue adapters.
package messaging

import (
	"context"
	"fmt"

	"github.com/goccy/go-json"

	"worker_server/core/port/out"

	"github.com/redis/go-redis/v9"
)

// Stream names
const (
	StreamMailSend        = "mail:send"
	StreamMailSync        = "mail:sync"
	StreamMailBatch       = "mail:batch"
	StreamMailSave        = "mail:save"
	StreamMailModify      = "mail:modify"
	StreamCalendarSync    = "calendar:sync"
	StreamCalendarEvent   = "calendar:event"
	StreamAIClassify      = "ai:classify"
	StreamAISummarize     = "ai:summarize"
	StreamAITranslate     = "ai:translate"
	StreamAIAutocomplete  = "ai:autocomplete"
	StreamAIChat          = "ai:chat"
	StreamAIGenerateReply = "ai:generate_reply"
	StreamRAGIndex        = "rag:index"
	StreamRAGBatchIndex   = "rag:batch"
	StreamRAGSearch       = "rag:search"
	StreamProfile         = "profile:analyze"

	// Priority streams
	StreamMailPriority     = "mail:priority"
	StreamCalendarPriority = "calendar:priority"
	StreamAIPriority       = "ai:priority"
)

// RedisProducer implements out.MessageProducer using Redis Streams.
type RedisProducer struct {
	client *redis.Client
}

// NewRedisProducer creates a new RedisProducer.
func NewRedisProducer(client *redis.Client) *RedisProducer {
	return &RedisProducer{client: client}
}

// PublishMailSend publishes a mail send job.
func (p *RedisProducer) PublishMailSend(ctx context.Context, job *out.MailSendJob) error {
	return p.publish(ctx, StreamMailSend, job)
}

// PublishMailSync publishes a mail sync job.
func (p *RedisProducer) PublishMailSync(ctx context.Context, job *out.MailSyncJob) error {
	return p.publish(ctx, StreamMailSync, job)
}

// PublishMailBatch publishes a mail batch job.
func (p *RedisProducer) PublishMailBatch(ctx context.Context, job *out.MailBatchJob) error {
	return p.publish(ctx, StreamMailBatch, job)
}

// PublishMailSave publishes a mail save job (async metadata save).
func (p *RedisProducer) PublishMailSave(ctx context.Context, job *out.MailSaveJob) error {
	return p.publish(ctx, StreamMailSave, job)
}

// PublishMailModify publishes a mail modify job (async provider sync).
func (p *RedisProducer) PublishMailModify(ctx context.Context, job *out.MailModifyJob) error {
	return p.publish(ctx, StreamMailModify, job)
}

// PublishCalendarSync publishes a calendar sync job.
func (p *RedisProducer) PublishCalendarSync(ctx context.Context, job *out.CalendarSyncJob) error {
	return p.publish(ctx, StreamCalendarSync, job)
}

// PublishCalendarEvent publishes a calendar event job.
func (p *RedisProducer) PublishCalendarEvent(ctx context.Context, job *out.CalendarEventJob) error {
	return p.publish(ctx, StreamCalendarEvent, job)
}

// PublishAIClassify publishes an AI classify job.
func (p *RedisProducer) PublishAIClassify(ctx context.Context, job *out.AIClassifyJob) error {
	return p.publish(ctx, StreamAIClassify, job)
}

// PublishAIBatchClassify publishes an AI batch classify job.
func (p *RedisProducer) PublishAIBatchClassify(ctx context.Context, job *out.AIBatchClassifyJob) error {
	return p.publish(ctx, StreamAIClassify, job)
}

// PublishAISummarize publishes an AI summarize job.
func (p *RedisProducer) PublishAISummarize(ctx context.Context, job *out.AISummarizeJob) error {
	return p.publish(ctx, StreamAISummarize, job)
}

// PublishAITranslate publishes an AI translate job.
func (p *RedisProducer) PublishAITranslate(ctx context.Context, job *out.AITranslateJob) error {
	return p.publish(ctx, StreamAITranslate, job)
}

// PublishAIAutocomplete publishes an AI autocomplete job.
func (p *RedisProducer) PublishAIAutocomplete(ctx context.Context, job *out.AIAutocompleteJob) error {
	return p.publish(ctx, StreamAIAutocomplete, job)
}

// PublishAIChat publishes an AI chat job.
func (p *RedisProducer) PublishAIChat(ctx context.Context, job *out.AIChatJob) error {
	return p.publish(ctx, StreamAIChat, job)
}

// PublishAIReply publishes an AI reply job.
func (p *RedisProducer) PublishAIReply(ctx context.Context, job *out.AIReplyJob) error {
	return p.publish(ctx, StreamAIGenerateReply, job)
}

// PublishAIGenerateReply publishes an AI generate reply job.
func (p *RedisProducer) PublishAIGenerateReply(ctx context.Context, job *out.AIGenerateReplyJob) error {
	return p.publish(ctx, StreamAIGenerateReply, job)
}

// PublishRAGIndex publishes a RAG index job.
func (p *RedisProducer) PublishRAGIndex(ctx context.Context, job *out.RAGIndexJob) error {
	return p.publish(ctx, StreamRAGIndex, job)
}

// PublishRAGBatchIndex publishes a RAG batch index job for initial sync.
func (p *RedisProducer) PublishRAGBatchIndex(ctx context.Context, job *out.RAGBatchIndexJob) error {
	return p.publish(ctx, StreamRAGBatchIndex, job)
}

// PublishRAGSearch publishes a RAG search job.
func (p *RedisProducer) PublishRAGSearch(ctx context.Context, job *out.RAGSearchJob) error {
	return p.publish(ctx, StreamRAGSearch, job)
}

// PublishProfileAnalyze publishes a user profile analysis job.
func (p *RedisProducer) PublishProfileAnalyze(ctx context.Context, job *out.ProfileAnalyzeJob) error {
	return p.publish(ctx, StreamProfile, job)
}

// PublishPriority publishes a priority job.
func (p *RedisProducer) PublishPriority(ctx context.Context, stream string, job interface{}) error {
	return p.publish(ctx, stream, job)
}

// PublishMailSyncInit publishes a mail sync init job for parallel sync.
func (p *RedisProducer) PublishMailSyncInit(ctx context.Context, job *out.MailSyncInitJob) error {
	return p.publish(ctx, StreamMailSync, job)
}

// PublishMailSyncPage publishes a mail sync page job for parallel sync.
func (p *RedisProducer) PublishMailSyncPage(ctx context.Context, job *out.MailSyncPageJob) error {
	return p.publish(ctx, StreamMailSync, job)
}

// =============================================================================
// Sync Status (Redis Hash)
// =============================================================================

const syncStatusKeyPrefix = "sync:status:"

// SetSyncStatus stores sync status in Redis.
func (p *RedisProducer) SetSyncStatus(ctx context.Context, connectionID int64, status *out.SyncStatus) error {
	key := fmt.Sprintf("%s%d", syncStatusKeyPrefix, connectionID)

	err := p.client.HSet(ctx, key,
		"phase", string(status.Phase),
		"status", status.Status,
		"total_pages", status.TotalPages,
		"synced_pages", status.SyncedPages,
		"total_emails", status.TotalEmails,
		"synced_emails", status.SyncedEmails,
	).Err()
	if err != nil {
		return fmt.Errorf("failed to set sync status: %w", err)
	}

	// Set expiry (24 hours)
	p.client.Expire(ctx, key, 86400)

	return nil
}

// GetSyncStatus retrieves sync status from Redis.
func (p *RedisProducer) GetSyncStatus(ctx context.Context, connectionID int64) (*out.SyncStatus, error) {
	key := fmt.Sprintf("%s%d", syncStatusKeyPrefix, connectionID)

	result, err := p.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get sync status: %w", err)
	}

	if len(result) == 0 {
		return nil, nil
	}

	status := &out.SyncStatus{
		ConnectionID: connectionID,
		Phase:        out.SyncPhase(result["phase"]),
		Status:       result["status"],
	}

	if v, ok := result["total_pages"]; ok {
		fmt.Sscanf(v, "%d", &status.TotalPages)
	}
	if v, ok := result["synced_pages"]; ok {
		fmt.Sscanf(v, "%d", &status.SyncedPages)
	}
	if v, ok := result["total_emails"]; ok {
		fmt.Sscanf(v, "%d", &status.TotalEmails)
	}
	if v, ok := result["synced_emails"]; ok {
		fmt.Sscanf(v, "%d", &status.SyncedEmails)
	}

	return status, nil
}

// IncrementSyncProgress atomically increments sync progress.
func (p *RedisProducer) IncrementSyncProgress(ctx context.Context, connectionID int64, emailCount int) error {
	key := fmt.Sprintf("%s%d", syncStatusKeyPrefix, connectionID)

	// Increment synced_pages by 1
	if err := p.client.HIncrBy(ctx, key, "synced_pages", 1).Err(); err != nil {
		return fmt.Errorf("failed to increment synced_pages: %w", err)
	}

	// Increment synced_emails by emailCount
	if err := p.client.HIncrBy(ctx, key, "synced_emails", int64(emailCount)).Err(); err != nil {
		return fmt.Errorf("failed to increment synced_emails: %w", err)
	}

	return nil
}

// publish publishes a job to a stream using go-redis.
func (p *RedisProducer) publish(ctx context.Context, stream string, job interface{}) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	err = p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		ID:     "*",
		Values: map[string]interface{}{
			"data": string(data),
		},
	}).Err()
	if err != nil {
		return fmt.Errorf("failed to publish to %s: %w", stream, err)
	}

	return nil
}

// Ensure RedisProducer implements out.MessageProducer
var _ out.MessageProducer = (*RedisProducer)(nil)
