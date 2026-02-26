// Package graph implements Neo4j adapters for the application.
package graph

import (
	"worker_server/core/port/out"
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// =============================================================================
// Neo4j Vector Store Adapter
//
// DEPRECATED: 이 어댑터는 레거시입니다.
// RAG 벡터 검색은 pgvector (Supabase)를 사용하세요.
// 새 구현: core/agent/rag/vectorstore.go → PgVectorStore
//
// Neo4j는 사용자 분석용으로만 사용됩니다:
// - PersonalizationAdapter: 사용자 스타일/톤 분석
// - ClassificationAdapter: 분류 패턴 학습
// =============================================================================

// VectorAdapter implements out.VectorStore using Neo4j.
//
// Deprecated: Use core/agent/rag.PgVectorStore instead.
// This adapter is kept for backward compatibility only.
type VectorAdapter struct {
	driver neo4j.DriverWithContext
	dbName string
}

// NewVectorAdapter creates a new Neo4j vector adapter.
func NewVectorAdapter(driver neo4j.DriverWithContext, dbName string) *VectorAdapter {
	return &VectorAdapter{
		driver: driver,
		dbName: dbName,
	}
}

// EnsureIndexes creates necessary indexes and constraints.
func (a *VectorAdapter) EnsureIndexes(ctx context.Context) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	queries := []string{
		// Vector index for similarity search
		"CREATE VECTOR INDEX email_embedding_index IF NOT EXISTS " +
			"FOR (e:Email) " +
			"ON (e.embedding) " +
			"OPTIONS {indexConfig: {`vector.dimensions`: 768, `vector.similarity_function`: 'cosine'}}",
		// Regular indexes
		`CREATE INDEX email_user_idx IF NOT EXISTS FOR (e:Email) ON (e.user_id)`,
		`CREATE INDEX email_recipient_idx IF NOT EXISTS FOR (e:Email) ON (e.recipient_email)`,
		`CREATE INDEX email_sent_at_idx IF NOT EXISTS FOR (e:Email) ON (e.sent_at)`,
	}

	for _, query := range queries {
		_, err := session.Run(ctx, query, nil)
		if err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// =============================================================================
// Search Operations
// =============================================================================

// Search performs a similarity search.
func (a *VectorAdapter) Search(ctx context.Context, embedding []float32, topK int) ([]out.VectorSearchResult, error) {
	return a.SearchWithFilter(ctx, embedding, topK, nil)
}

// SearchWithFilter performs a similarity search with filters.
func (a *VectorAdapter) SearchWithFilter(ctx context.Context, embedding []float32, topK int, opts *out.VectorSearchOptions) ([]out.VectorSearchResult, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	// Build query with optional filters
	query := `
		CALL db.index.vector.queryNodes('email_embedding_index', $topK, $embedding)
		YIELD node, score
	`

	whereClause := ""
	params := map[string]interface{}{
		"embedding": embedding,
		"topK":      topK,
	}

	if opts != nil {
		conditions := []string{}

		if opts.UserID != "" {
			conditions = append(conditions, "node.user_id = $userID")
			params["userID"] = opts.UserID
		}
		if opts.MinScore > 0 {
			conditions = append(conditions, "score >= $minScore")
			params["minScore"] = opts.MinScore
		}
		if opts.SentOnly {
			conditions = append(conditions, "node.is_sent = true")
		}
		if opts.RecipientEmail != "" {
			conditions = append(conditions, "node.recipient_email = $recipientEmail")
			params["recipientEmail"] = opts.RecipientEmail
		}
		if opts.RecipientType != "" {
			conditions = append(conditions, "node.recipient_type = $recipientType")
			params["recipientType"] = opts.RecipientType
		}
		if opts.Folder != "" {
			conditions = append(conditions, "node.folder = $folder")
			params["folder"] = opts.Folder
		}
		if opts.Category != "" {
			conditions = append(conditions, "node.category = $category")
			params["category"] = opts.Category
		}
		if opts.DateFrom != nil {
			conditions = append(conditions, "node.sent_at >= $dateFrom")
			params["dateFrom"] = opts.DateFrom.Unix()
		}
		if opts.DateTo != nil {
			conditions = append(conditions, "node.sent_at <= $dateTo")
			params["dateTo"] = opts.DateTo.Unix()
		}

		if len(conditions) > 0 {
			whereClause = " WHERE "
			for i, cond := range conditions {
				if i > 0 {
					whereClause += " AND "
				}
				whereClause += cond
			}
		}
	}

	query += whereClause + `
		RETURN node.id AS id, score, node.subject AS subject,
			   node.snippet AS snippet, node.recipient_email AS recipient_email,
			   node.sent_at AS sent_at, node.metadata AS metadata
		ORDER BY score DESC
		LIMIT $topK
	`

	result, err := session.Run(ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("failed to search vectors: %w", err)
	}

	var results []out.VectorSearchResult
	for result.Next(ctx) {
		record := result.Record()

		var sentAt time.Time
		if ts, ok := record.Get("sent_at"); ok && ts != nil {
			if tsInt, ok := ts.(int64); ok {
				sentAt = time.Unix(tsInt, 0)
			}
		}

		r := out.VectorSearchResult{
			ID:    getStringValue(record, "id"),
			Score: getFloatValue(record, "score"),
		}

		if subject, ok := record.Get("subject"); ok && subject != nil {
			r.Subject = subject.(string)
		}
		if snippet, ok := record.Get("snippet"); ok && snippet != nil {
			r.Snippet = snippet.(string)
		}
		if recipientEmail, ok := record.Get("recipient_email"); ok && recipientEmail != nil {
			r.RecipientEmail = recipientEmail.(string)
		}
		if !sentAt.IsZero() {
			r.SentAt = sentAt
		}
		if metadata, ok := record.Get("metadata"); ok && metadata != nil {
			if m, ok := metadata.(map[string]interface{}); ok {
				r.Metadata = m
			}
		}

		results = append(results, r)
	}

	return results, nil
}

// SearchByRecipient searches vectors by recipient email.
func (a *VectorAdapter) SearchByRecipient(ctx context.Context, userID, recipientEmail string, topK int) ([]out.VectorSearchResult, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (e:Email)
		WHERE e.user_id = $userID AND e.recipient_email = $recipientEmail
		RETURN e.id AS id, 1.0 AS score, e.subject AS subject,
			   e.snippet AS snippet, e.recipient_email AS recipient_email,
			   e.sent_at AS sent_at, e.metadata AS metadata
		ORDER BY e.sent_at DESC
		LIMIT $topK
	`

	params := map[string]interface{}{
		"userID":         userID,
		"recipientEmail": recipientEmail,
		"topK":           topK,
	}

	result, err := session.Run(ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("failed to search by recipient: %w", err)
	}

	var results []out.VectorSearchResult
	for result.Next(ctx) {
		record := result.Record()

		r := out.VectorSearchResult{
			ID:             getStringValue(record, "id"),
			Score:          getFloatValue(record, "score"),
			Subject:        getStringValue(record, "subject"),
			Snippet:        getStringValue(record, "snippet"),
			RecipientEmail: recipientEmail,
		}

		if sentAt, ok := record.Get("sent_at"); ok && sentAt != nil {
			if ts, ok := sentAt.(int64); ok {
				r.SentAt = time.Unix(ts, 0)
			}
		}

		results = append(results, r)
	}

	return results, nil
}

// =============================================================================
// CRUD Operations
// =============================================================================

// Store stores a vector with metadata.
func (a *VectorAdapter) Store(ctx context.Context, id string, embedding []float32, metadata map[string]interface{}) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MERGE (e:Email {id: $id})
		SET e.embedding = $embedding,
			e.metadata = $metadata,
			e.updated_at = timestamp()
	`

	// Add metadata fields to node properties
	if metadata != nil {
		for key := range metadata {
			query = fmt.Sprintf(`%s, e.%s = $metadata.%s`, query, key, key)
		}
	}

	params := map[string]interface{}{
		"id":        id,
		"embedding": embedding,
		"metadata":  metadata,
	}

	_, err := session.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to store vector: %w", err)
	}

	return nil
}

// Delete deletes a vector by ID.
func (a *VectorAdapter) Delete(ctx context.Context, id string) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (e:Email {id: $id})
		DELETE e
	`

	_, err := session.Run(ctx, query, map[string]interface{}{"id": id})
	if err != nil {
		return fmt.Errorf("failed to delete vector: %w", err)
	}

	return nil
}

// GetByID retrieves a vector item by ID.
func (a *VectorAdapter) GetByID(ctx context.Context, id string) (*out.VectorItem, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (e:Email {id: $id})
		RETURN e.embedding AS embedding, e.metadata AS metadata
	`

	result, err := session.Run(ctx, query, map[string]interface{}{"id": id})
	if err != nil {
		return nil, fmt.Errorf("failed to get vector: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()

		var embedding []float32
		if emb, ok := record.Get("embedding"); ok && emb != nil {
			if embArr, ok := emb.([]interface{}); ok {
				embedding = make([]float32, len(embArr))
				for i, v := range embArr {
					if f, ok := v.(float64); ok {
						embedding[i] = float32(f)
					}
				}
			}
		}

		var metadata map[string]interface{}
		if m, ok := record.Get("metadata"); ok && m != nil {
			metadata = m.(map[string]interface{})
		}

		return &out.VectorItem{
			ID:        id,
			Embedding: embedding,
			Metadata:  metadata,
		}, nil
	}

	return nil, nil
}

// =============================================================================
// Batch Operations
// =============================================================================

// BatchStore stores multiple vectors.
func (a *VectorAdapter) BatchStore(ctx context.Context, items []out.VectorItem) error {
	if len(items) == 0 {
		return nil
	}

	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		UNWIND $items AS item
		MERGE (e:Email {id: item.id})
		SET e.embedding = item.embedding,
			e.metadata = item.metadata,
			e.updated_at = timestamp()
	`

	itemsData := make([]map[string]interface{}, len(items))
	for i, item := range items {
		itemsData[i] = map[string]interface{}{
			"id":        item.ID,
			"embedding": item.Embedding,
			"metadata":  item.Metadata,
		}
	}

	_, err := session.Run(ctx, query, map[string]interface{}{"items": itemsData})
	if err != nil {
		return fmt.Errorf("failed to batch store vectors: %w", err)
	}

	return nil
}

// BatchDelete deletes multiple vectors.
func (a *VectorAdapter) BatchDelete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (e:Email)
		WHERE e.id IN $ids
		DELETE e
	`

	_, err := session.Run(ctx, query, map[string]interface{}{"ids": ids})
	if err != nil {
		return fmt.Errorf("failed to batch delete vectors: %w", err)
	}

	return nil
}

// Helper functions are defined in personalization_adapter.go

// =============================================================================
// Interface Compliance
// =============================================================================

var _ out.VectorStore = (*VectorAdapter)(nil)
