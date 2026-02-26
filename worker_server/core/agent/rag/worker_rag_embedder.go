package rag

import (
	"context"

	"worker_server/core/agent/llm"
)

type Embedder struct {
	client *llm.Client
}

func NewEmbedder(client *llm.Client) *Embedder {
	return &Embedder{client: client}
}

func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return e.client.Embedding(ctx, text)
}

func (e *Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return e.client.EmbeddingBatch(ctx, texts)
}

// PrepareText preprocesses text for embedding
func (e *Embedder) PrepareText(subject, body string, maxLen int) string {
	text := subject + "\n\n" + body
	if len(text) > maxLen {
		return text[:maxLen]
	}
	return text
}
