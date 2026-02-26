package rag

import (
	"context"

	"github.com/google/uuid"
)

type Retriever struct {
	embedder    *Embedder
	vectorStore *VectorStore
}

func NewRetriever(embedder *Embedder, vectorStore *VectorStore) *Retriever {
	return &Retriever{
		embedder:    embedder,
		vectorStore: vectorStore,
	}
}

type RetrievalRequest struct {
	Query        string
	UserID       uuid.UUID
	SentOnly     bool // For style learning
	ReceivedOnly bool // For context search
	Limit        int
	MinScore     float64
}

type RetrievalResult struct {
	EmailID  int64          `json:"email_id"`
	Score    float64        `json:"score"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata"`
}

func (r *Retriever) Retrieve(ctx context.Context, req *RetrievalRequest) ([]*RetrievalResult, error) {
	// Embed the query
	embedding, err := r.embedder.Embed(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	// Search vector store
	opts := &SearchOptions{
		UserID:       req.UserID.String(),
		SentOnly:     req.SentOnly,
		ReceivedOnly: req.ReceivedOnly,
		AllEmails:    !req.SentOnly && !req.ReceivedOnly,
		Limit:        req.Limit,
		MinScore:     req.MinScore,
	}

	if opts.Limit == 0 {
		opts.Limit = 5
	}

	results, err := r.vectorStore.Search(ctx, embedding, opts)
	if err != nil {
		return nil, err
	}

	// Convert to retrieval results
	retrievalResults := make([]*RetrievalResult, len(results))
	for i, r := range results {
		retrievalResults[i] = &RetrievalResult{
			EmailID:  r.EmailID,
			Score:    r.Score,
			Content:  r.Content,
			Metadata: r.Metadata,
		}
	}

	return retrievalResults, nil
}

// RetrieveForStyle retrieves sent emails for style learning
func (r *Retriever) RetrieveForStyle(ctx context.Context, userID uuid.UUID, query string, limit int) ([]*RetrievalResult, error) {
	return r.Retrieve(ctx, &RetrievalRequest{
		Query:    query,
		UserID:   userID,
		SentOnly: true,
		Limit:    limit,
	})
}

// RetrieveForContext retrieves all emails for context
func (r *Retriever) RetrieveForContext(ctx context.Context, userID uuid.UUID, query string, limit int) ([]*RetrievalResult, error) {
	return r.Retrieve(ctx, &RetrievalRequest{
		Query:  query,
		UserID: userID,
		Limit:  limit,
	})
}
