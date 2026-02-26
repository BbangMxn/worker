package messaging

import (
	"context"
	"fmt"
	"time"

	"github.com/goccy/go-json"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// JobHandler processes jobs from streams.
type JobHandler interface {
	Handle(ctx context.Context, stream string, data []byte) error
}

// Consumer consumes messages from Redis Streams.
type Consumer struct {
	client   *redis.Client
	group    string
	consumer string
	streams  []string
	handler  JobHandler
	log      zerolog.Logger

	// Pending 메시지 재처리 설정
	pendingCheckInterval time.Duration // Pending 메시지 체크 간격
	pendingIdleTime      time.Duration // 이 시간 이상 pending이면 재처리
	maxRetries           int           // 최대 재시도 횟수
}

// ConsumerConfig holds consumer configuration.
type ConsumerConfig struct {
	Group    string
	Consumer string
	Streams  []string
	Handler  JobHandler
	Logger   zerolog.Logger

	// Optional: Pending 설정 (기본값 사용 가능)
	PendingCheckInterval time.Duration
	PendingIdleTime      time.Duration
	MaxRetries           int
}

// NewConsumer creates a new Consumer.
func NewConsumer(client *redis.Client, cfg *ConsumerConfig) *Consumer {
	// 기본값 설정 (최적화: 체크 간격 30초, idle 2분으로 단축)
	pendingCheckInterval := cfg.PendingCheckInterval
	if pendingCheckInterval == 0 {
		pendingCheckInterval = 30 * time.Second // 30초마다 체크 (기존 60초에서 단축)
	}

	pendingIdleTime := cfg.PendingIdleTime
	if pendingIdleTime == 0 {
		pendingIdleTime = 2 * time.Minute // 2분 이상 pending이면 재처리 (기존 5분에서 단축)
	}

	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	return &Consumer{
		client:               client,
		group:                cfg.Group,
		consumer:             cfg.Consumer,
		streams:              cfg.Streams,
		handler:              cfg.Handler,
		log:                  cfg.Logger,
		pendingCheckInterval: pendingCheckInterval,
		pendingIdleTime:      pendingIdleTime,
		maxRetries:           maxRetries,
	}
}

// Run starts consuming messages.
func (c *Consumer) Run(ctx context.Context) error {
	c.log.Info().
		Str("group", c.group).
		Str("consumer", c.consumer).
		Strs("streams", c.streams).
		Msg("starting consumer")

	// Ensure consumer groups exist
	for _, stream := range c.streams {
		c.createConsumerGroup(ctx, stream)
	}

	// Pending 메시지 재처리 고루틴 시작
	go c.processPendingMessages(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		result, err := c.readMessages(ctx)
		if err != nil {
			if err == redis.Nil {
				continue // No messages
			}
			c.log.Error().Err(err).Msg("error reading from streams")
			time.Sleep(time.Second)
			continue
		}

		// Process messages
		for _, stream := range result {
			for _, msg := range stream.Messages {
				if err := c.processMessage(ctx, stream.Stream, msg); err != nil {
					c.log.Error().
						Err(err).
						Str("stream", stream.Stream).
						Str("id", msg.ID).
						Msg("error processing message")
					continue
				}

				// Acknowledge message
				if err := c.client.XAck(ctx, stream.Stream, c.group, msg.ID).Err(); err != nil {
					c.log.Error().
						Err(err).
						Str("stream", stream.Stream).
						Str("id", msg.ID).
						Msg("error acknowledging message")
				}
			}
		}
	}
}

// processPendingMessages periodically checks and reprocesses stuck pending messages.
func (c *Consumer) processPendingMessages(ctx context.Context) {
	ticker := time.NewTicker(c.pendingCheckInterval)
	defer ticker.Stop()

	c.log.Info().
		Dur("check_interval", c.pendingCheckInterval).
		Dur("idle_time", c.pendingIdleTime).
		Int("max_retries", c.maxRetries).
		Msg("starting pending message processor")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.claimAndProcessPending(ctx)
		}
	}
}

// claimAndProcessPending claims stuck pending messages and reprocesses them.
func (c *Consumer) claimAndProcessPending(ctx context.Context) {
	for _, stream := range c.streams {
		// Get pending messages for this stream
		pending, err := c.client.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream: stream,
			Group:  c.group,
			Start:  "-",
			End:    "+",
			Count:  100,
		}).Result()
		if err != nil {
			if err != redis.Nil {
				c.log.Error().Err(err).Str("stream", stream).Msg("error getting pending messages")
			}
			continue
		}

		for _, p := range pending {
			// Skip if not idle long enough
			if p.Idle < c.pendingIdleTime {
				continue
			}

			// Check retry count - move to DLQ if exceeded
			if int(p.RetryCount) >= c.maxRetries {
				c.log.Warn().
					Str("stream", stream).
					Str("id", p.ID).
					Int64("retries", p.RetryCount).
					Msg("message exceeded max retries, moving to DLQ")

				// Move to Dead Letter Queue stream before acknowledging
				if err := c.moveToDeadLetterQueue(ctx, stream, p.ID); err != nil {
					c.log.Error().Err(err).Str("id", p.ID).Msg("error moving message to DLQ")
				}

				// Acknowledge to remove from pending
				c.client.XAck(ctx, stream, c.group, p.ID)
				continue
			}

			c.log.Info().
				Str("stream", stream).
				Str("id", p.ID).
				Str("consumer", p.Consumer).
				Dur("idle", p.Idle).
				Int64("retries", p.RetryCount).
				Msg("claiming stuck pending message")

			// Claim the message
			claimed, err := c.client.XClaim(ctx, &redis.XClaimArgs{
				Stream:   stream,
				Group:    c.group,
				Consumer: c.consumer,
				MinIdle:  c.pendingIdleTime,
				Messages: []string{p.ID},
			}).Result()
			if err != nil {
				c.log.Error().Err(err).Str("id", p.ID).Msg("error claiming message")
				continue
			}

			// Process claimed messages
			for _, msg := range claimed {
				if err := c.processMessage(ctx, stream, msg); err != nil {
					c.log.Error().
						Err(err).
						Str("stream", stream).
						Str("id", msg.ID).
						Msg("error reprocessing pending message")
					continue
				}

				// Acknowledge on success
				if err := c.client.XAck(ctx, stream, c.group, msg.ID).Err(); err != nil {
					c.log.Error().Err(err).Str("id", msg.ID).Msg("error acknowledging reprocessed message")
				} else {
					c.log.Info().Str("stream", stream).Str("id", msg.ID).Msg("successfully reprocessed pending message")
				}
			}
		}
	}
}

// createConsumerGroup creates a consumer group if it doesn't exist.
func (c *Consumer) createConsumerGroup(ctx context.Context, stream string) {
	err := c.client.XGroupCreateMkStream(ctx, stream, c.group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		c.log.Warn().Err(err).Str("stream", stream).Msg("error creating consumer group")
	}
}

// readMessages reads messages from all streams using XREADGROUP.
func (c *Consumer) readMessages(ctx context.Context) ([]redis.XStream, error) {
	if len(c.streams) == 0 {
		return nil, nil
	}

	// Build streams and IDs for XREADGROUP
	args := make([]string, len(c.streams)*2)
	for i, stream := range c.streams {
		args[i] = stream
		args[len(c.streams)+i] = ">"
	}

	result, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.group,
		Consumer: c.consumer,
		Streams:  args,
		Count:    10,
		Block:    5 * time.Second,
	}).Result()
	if err != nil {
		if err != redis.Nil {
			c.log.Debug().Err(err).Msg("XReadGroup returned error")
		}
		return nil, err
	}

	if len(result) > 0 {
		c.log.Info().Int("streams_with_messages", len(result)).Msg("XReadGroup received messages")
	}

	return result, nil
}

// processMessage processes a single message.
func (c *Consumer) processMessage(ctx context.Context, stream string, msg redis.XMessage) error {
	data, ok := msg.Values["data"]
	if !ok {
		return fmt.Errorf("invalid message format: missing data field")
	}

	dataStr, ok := data.(string)
	if !ok {
		return fmt.Errorf("invalid message format: data is not a string")
	}

	return c.handler.Handle(ctx, stream, []byte(dataStr))
}

// Message represents a parsed message.
type Message struct {
	ID     string
	Stream string
	Data   json.RawMessage
}

// moveToDeadLetterQueue moves a failed message to a Dead Letter Queue stream.
// DLQ stream name format: dlq:{original_stream_name}
func (c *Consumer) moveToDeadLetterQueue(ctx context.Context, stream string, msgID string) error {
	// Read the original message to get its data
	messages, err := c.client.XRange(ctx, stream, msgID, msgID).Result()
	if err != nil {
		return fmt.Errorf("failed to read message for DLQ: %w", err)
	}

	if len(messages) == 0 {
		return fmt.Errorf("message %s not found in stream %s", msgID, stream)
	}

	msg := messages[0]
	dlqStream := "dlq:" + stream

	// Add message to DLQ with metadata
	dlqData := map[string]interface{}{
		"original_stream": stream,
		"original_id":     msgID,
		"failed_at":       time.Now().UTC().Format(time.RFC3339),
		"consumer":        c.consumer,
		"group":           c.group,
	}

	// Copy original data
	for k, v := range msg.Values {
		dlqData["original_"+k] = v
	}

	_, err = c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: dlqStream,
		Values: dlqData,
	}).Result()
	if err != nil {
		return fmt.Errorf("failed to add message to DLQ: %w", err)
	}

	c.log.Info().
		Str("dlq_stream", dlqStream).
		Str("original_stream", stream).
		Str("original_id", msgID).
		Msg("message moved to DLQ")

	return nil
}
