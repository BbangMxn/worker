package stream

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Producer struct {
	stream *RedisStream
}

func NewProducer(stream *RedisStream) *Producer {
	return &Producer{stream: stream}
}

type Job struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

func (p *Producer) PublishMailSync(ctx context.Context, connectionID int64, userID uuid.UUID, fullSync bool) (string, error) {
	job := &Job{
		ID:   uuid.New().String(),
		Type: "mail.sync",
		Payload: map[string]any{
			"connection_id": connectionID,
			"user_id":       userID.String(),
			"full_sync":     fullSync,
		},
		CreatedAt: time.Now(),
	}
	return p.stream.Publish(ctx, StreamMailSync, job)
}

func (p *Producer) PublishMailSend(ctx context.Context, connectionID int64, userID uuid.UUID, to []string, subject, body string) (string, error) {
	job := &Job{
		ID:   uuid.New().String(),
		Type: "mail.send",
		Payload: map[string]any{
			"connection_id": connectionID,
			"user_id":       userID.String(),
			"to":            to,
			"subject":       subject,
			"body":          body,
		},
		CreatedAt: time.Now(),
	}
	return p.stream.Publish(ctx, StreamMailSend, job)
}

func (p *Producer) PublishAIClassify(ctx context.Context, emailID int64, userID uuid.UUID) (string, error) {
	job := &Job{
		ID:   uuid.New().String(),
		Type: "ai.classify",
		Payload: map[string]any{
			"email_id": emailID,
			"user_id":  userID.String(),
		},
		CreatedAt: time.Now(),
	}
	return p.stream.Publish(ctx, StreamAI, job)
}

func (p *Producer) PublishRAGIndex(ctx context.Context, emailID int64, userID uuid.UUID, subject, body, from, direction, folder string) (string, error) {
	job := &Job{
		ID:   uuid.New().String(),
		Type: "rag.index",
		Payload: map[string]any{
			"email_id":   emailID,
			"user_id":    userID.String(),
			"subject":    subject,
			"body":       body,
			"from_email": from,
			"direction":  direction,
			"folder":     folder,
		},
		CreatedAt: time.Now(),
	}
	return p.stream.Publish(ctx, StreamRAG, job)
}

func (p *Producer) PublishRAGBatchIndex(ctx context.Context, emails []map[string]any) (string, error) {
	job := &Job{
		ID:   uuid.New().String(),
		Type: "rag.batch_index",
		Payload: map[string]any{
			"emails": emails,
		},
		CreatedAt: time.Now(),
	}
	return p.stream.Publish(ctx, StreamRAG, job)
}

func (p *Producer) PublishCalendarSync(ctx context.Context, connectionID int64, userID uuid.UUID) (string, error) {
	job := &Job{
		ID:   uuid.New().String(),
		Type: "calendar.sync",
		Payload: map[string]any{
			"connection_id": connectionID,
			"user_id":       userID.String(),
		},
		CreatedAt: time.Now(),
	}
	return p.stream.Publish(ctx, StreamCalendar, job)
}
