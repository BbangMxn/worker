package stream

import (
	"context"
	"log"

	"github.com/goccy/go-json"

	"worker_server/adapter/in/worker"
)

type Consumer struct {
	stream  *RedisStream
	handler *worker.Handler
	name    string
}

func NewConsumer(stream *RedisStream, handler *worker.Handler, name string) *Consumer {
	return &Consumer{
		stream:  stream,
		handler: handler,
		name:    name,
	}
}

func (c *Consumer) Start(ctx context.Context) {
	// Create consumer groups
	streams := []string{StreamMailSync, StreamMailSend, StreamAI, StreamRAG, StreamCalendar}
	for _, s := range streams {
		if err := c.stream.CreateGroup(ctx, s); err != nil {
			log.Printf("Failed to create group for %s: %v", s, err)
		}
	}

	// Start consumers for each stream
	go c.consume(ctx, StreamMailSync)
	go c.consume(ctx, StreamMailSend)
	go c.consume(ctx, StreamAI)
	go c.consume(ctx, StreamRAG)
	go c.consume(ctx, StreamCalendar)
}

func (c *Consumer) consume(ctx context.Context, stream string) {
	c.stream.Consume(ctx, stream, c.name, func(id string, data []byte) error {
		var job Job
		if err := json.Unmarshal(data, &job); err != nil {
			log.Printf("Failed to unmarshal job: %v", err)
			return err
		}

		msg := &worker.Message{
			ID:        job.ID,
			Type:      job.Type,
			Payload:   job.Payload,
			CreatedAt: job.CreatedAt,
		}

		return c.handler.Process(ctx, msg)
	})
}
