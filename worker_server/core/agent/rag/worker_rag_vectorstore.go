package rag

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

type VectorStore struct {
	db *pgxpool.Pool
}

func NewVectorStore(db *pgxpool.Pool) *VectorStore {
	return &VectorStore{db: db}
}

type VectorRecord struct {
	ID        int64
	EmailID   int64
	UserID    string
	Direction string // inbound, outbound
	Embedding []float32
	Content   string
	Metadata  map[string]any
}

// Store stores embedding directly in the emails table
func (s *VectorStore) Store(ctx context.Context, record *VectorRecord) error {
	query := `
		UPDATE emails
		SET embedding = $1,
			updated_at = NOW()
		WHERE id = $2 AND user_id = $3
	`

	_, err := s.db.Exec(ctx, query,
		pgVector(record.Embedding),
		record.EmailID,
		record.UserID,
	)
	return err
}

// StoreBatch stores multiple embeddings
func (s *VectorStore) StoreBatch(ctx context.Context, records []*VectorRecord) error {
	for _, record := range records {
		if err := s.Store(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

type SearchOptions struct {
	UserID       string
	SentOnly     bool // outbound only (for style learning)
	ReceivedOnly bool // inbound only
	AllEmails    bool // both directions
	Limit        int
	MinScore     float64
}

type SearchResult struct {
	EmailID  int64
	Score    float64
	Content  string
	Metadata map[string]any
}

// Search performs vector similarity search on emails table
func (s *VectorStore) Search(ctx context.Context, embedding []float32, opts *SearchOptions) ([]*SearchResult, error) {
	if opts.Limit == 0 {
		opts.Limit = 10
	}

	query := `
		SELECT id, 1 - (embedding <=> $1) as score, subject, snippet
		FROM emails
		WHERE user_id = $2
		AND embedding IS NOT NULL
	`

	if opts.SentOnly {
		query += ` AND folder = 'sent'`
	} else if opts.ReceivedOnly {
		query += ` AND folder != 'sent'`
	}

	if opts.MinScore > 0 {
		query += ` AND 1 - (embedding <=> $1) >= ` + strconv.FormatFloat(opts.MinScore, 'f', 2, 64)
	}

	query += ` ORDER BY embedding <=> $1 LIMIT $3`

	rows, err := s.db.Query(ctx, query, pgVector(embedding), opts.UserID, opts.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*SearchResult
	for rows.Next() {
		var r SearchResult
		var subject, snippet string
		if err := rows.Scan(&r.EmailID, &r.Score, &subject, &snippet); err != nil {
			return nil, err
		}
		// Use subject + snippet as content for RAG context
		r.Content = subject + "\n" + snippet
		r.Metadata = map[string]any{
			"subject": subject,
			"snippet": snippet,
		}
		results = append(results, &r)
	}

	return results, nil
}

// Delete removes embedding from an email
func (s *VectorStore) Delete(ctx context.Context, emailID int64) error {
	_, err := s.db.Exec(ctx, `UPDATE emails SET embedding = NULL WHERE id = $1`, emailID)
	return err
}

// HasEmbedding checks if an email already has an embedding
func (s *VectorStore) HasEmbedding(ctx context.Context, emailID int64) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx,
		`SELECT embedding IS NOT NULL FROM emails WHERE id = $1`,
		emailID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// pgVector converts float32 slice to pgvector format string
// Optimized version using []byte buffer
func pgVector(v []float32) string {
	if len(v) == 0 {
		return "[0]"
	}

	// Pre-allocate buffer: '[' + (number with ~12 chars + ',') * len + ']'
	buf := make([]byte, 0, len(v)*13+2)
	buf = append(buf, '[')

	for i, f := range v {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = strconv.AppendFloat(buf, float64(f), 'f', 6, 32)
	}

	buf = append(buf, ']')
	return string(buf)
}
