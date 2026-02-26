package rag

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type IndexerService struct {
	embedder    *Embedder
	vectorStore *VectorStore
}

func NewIndexerService(embedder *Embedder, vectorStore *VectorStore) *IndexerService {
	return &IndexerService{
		embedder:    embedder,
		vectorStore: vectorStore,
	}
}

type EmailIndexRequest struct {
	EmailID    int64
	UserID     uuid.UUID
	Subject    string
	Body       string
	FromEmail  string
	Direction  string // inbound, outbound
	ReceivedAt time.Time
	Folder     string
}

// IndexEmail indexes a single email for RAG search
func (s *IndexerService) IndexEmail(ctx context.Context, req *EmailIndexRequest) error {
	// Prepare text for embedding
	text := s.embedder.PrepareText(req.Subject, req.Body, 8000)

	// Generate embedding
	embedding, err := s.embedder.Embed(ctx, text)
	if err != nil {
		return err
	}

	// Store in vector database
	record := &VectorRecord{
		EmailID:   req.EmailID,
		UserID:    req.UserID.String(),
		Direction: req.Direction,
		Embedding: embedding,
		Content:   text,
		Metadata: map[string]any{
			"subject":     req.Subject,
			"from":        req.FromEmail,
			"received_at": req.ReceivedAt,
			"folder":      req.Folder,
		},
	}

	return s.vectorStore.Store(ctx, record)
}

// IndexBatch indexes multiple emails in batch
func (s *IndexerService) IndexBatch(ctx context.Context, requests []*EmailIndexRequest) error {
	if len(requests) == 0 {
		return nil
	}

	// Prepare texts
	texts := make([]string, len(requests))
	for i, req := range requests {
		texts[i] = s.embedder.PrepareText(req.Subject, req.Body, 8000)
	}

	// Batch embed
	embeddings, err := s.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return err
	}

	// Prepare records
	records := make([]*VectorRecord, len(requests))
	for i, req := range requests {
		records[i] = &VectorRecord{
			EmailID:   req.EmailID,
			UserID:    req.UserID.String(),
			Direction: req.Direction,
			Embedding: embeddings[i],
			Content:   texts[i],
			Metadata: map[string]any{
				"subject":     req.Subject,
				"from":        req.FromEmail,
				"received_at": req.ReceivedAt,
				"folder":      req.Folder,
			},
		}
	}

	return s.vectorStore.StoreBatch(ctx, records)
}

// DeleteEmail removes an email from the index
func (s *IndexerService) DeleteEmail(ctx context.Context, emailID int64) error {
	return s.vectorStore.Delete(ctx, emailID)
}

// SentEmailIndexer indexes sent emails for style learning
type SentEmailIndexer struct {
	indexer *IndexerService
}

func NewSentEmailIndexer(indexer *IndexerService) *SentEmailIndexer {
	return &SentEmailIndexer{indexer: indexer}
}

func (s *SentEmailIndexer) Index(ctx context.Context, req *EmailIndexRequest) error {
	req.Direction = "outbound"
	return s.indexer.IndexEmail(ctx, req)
}

// ReceivedEmailIndexer indexes received emails for context search
type ReceivedEmailIndexer struct {
	indexer *IndexerService
}

func NewReceivedEmailIndexer(indexer *IndexerService) *ReceivedEmailIndexer {
	return &ReceivedEmailIndexer{indexer: indexer}
}

func (s *ReceivedEmailIndexer) Index(ctx context.Context, req *EmailIndexRequest) error {
	req.Direction = "inbound"
	return s.indexer.IndexEmail(ctx, req)
}
