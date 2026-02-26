package stream

import (
	"context"
	"log"
	"time"

	"github.com/goccy/go-json"

	"github.com/redis/go-redis/v9"
)

const (
	StreamMailSync = "mail:sync"
	StreamMailSend = "mail:send"
	StreamAI       = "ai:jobs"
	StreamRAG      = "rag:jobs"
	StreamCalendar = "calendar:jobs"
)

type RedisStream struct {
	client *redis.Client
	group  string
}

func NewRedisStream(client *redis.Client, group string) *RedisStream {
	return &RedisStream{
		client: client,
		group:  group,
	}
}

func (s *RedisStream) CreateGroup(ctx context.Context, stream string) error {
	err := s.client.XGroupCreateMkStream(ctx, stream, s.group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

func (s *RedisStream) Publish(ctx context.Context, stream string, data any) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	return s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"data": jsonData},
	}).Result()
}

func (s *RedisStream) Consume(ctx context.Context, stream, consumer string, handler func(id string, data []byte) error) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := s.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    s.group,
			Consumer: consumer,
			Streams:  []string{stream, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()

		if err != nil {
			if err != redis.Nil {
				log.Printf("Stream read error: %v", err)
			}
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				data, ok := msg.Values["data"].(string)
				if !ok {
					continue
				}

				if err := handler(msg.ID, []byte(data)); err != nil {
					log.Printf("Handler error for %s: %v", msg.ID, err)
					continue
				}

				// Acknowledge message
				s.client.XAck(ctx, stream.Stream, s.group, msg.ID)
			}
		}
	}
}

func (s *RedisStream) Ack(ctx context.Context, stream, id string) error {
	return s.client.XAck(ctx, stream, s.group, id).Err()
}

func (s *RedisStream) Pending(ctx context.Context, stream string) (int64, error) {
	info, err := s.client.XPending(ctx, stream, s.group).Result()
	if err != nil {
		return 0, err
	}
	return info.Count, nil
}
