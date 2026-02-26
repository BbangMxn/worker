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
// Neo4j Classification Pattern Store Adapter
// =============================================================================

// ClassificationAdapter implements out.ClassificationPatternStore using Neo4j.
type ClassificationAdapter struct {
	driver neo4j.DriverWithContext
	dbName string
}

// NewClassificationAdapter creates a new Neo4j classification adapter.
func NewClassificationAdapter(driver neo4j.DriverWithContext, dbName string) *ClassificationAdapter {
	return &ClassificationAdapter{
		driver: driver,
		dbName: dbName,
	}
}

// EnsureIndexes creates necessary indexes for classification patterns.
func (a *ClassificationAdapter) EnsureIndexes(ctx context.Context) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	queries := []string{
		// Vector index for pattern similarity search
		"CREATE VECTOR INDEX pattern_embedding_index IF NOT EXISTS " +
			"FOR (p:ClassificationPattern) " +
			"ON (p.embedding) " +
			"OPTIONS {indexConfig: {`vector.dimensions`: 768, `vector.similarity_function`: 'cosine'}}",
		// Regular indexes
		`CREATE INDEX pattern_user_idx IF NOT EXISTS FOR (p:ClassificationPattern) ON (p.user_id)`,
		`CREATE INDEX pattern_category_idx IF NOT EXISTS FOR (p:ClassificationPattern) ON (p.category)`,
		`CREATE INDEX pattern_email_idx IF NOT EXISTS FOR (p:ClassificationPattern) ON (p.email_id)`,
		`CREATE CONSTRAINT pattern_unique IF NOT EXISTS FOR (p:ClassificationPattern) REQUIRE (p.user_id, p.email_id) IS UNIQUE`,
	}

	for _, query := range queries {
		_, err := session.Run(ctx, query, nil)
		if err != nil {
			// Ignore errors for existing indexes
			continue
		}
	}

	return nil
}

// =============================================================================
// Store Operations
// =============================================================================

// Store stores a classification pattern with its embedding.
func (a *ClassificationAdapter) Store(ctx context.Context, pattern *out.ClassificationPattern, embedding []float32) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MERGE (p:ClassificationPattern {user_id: $userID, email_id: $emailID})
		SET p.from_addr = $from,
			p.subject = $subject,
			p.snippet = $snippet,
			p.category = $category,
			p.priority = $priority,
			p.tags = $tags,
			p.intent = $intent,
			p.is_manual = $isManual,
			p.embedding = $embedding,
			p.created_at = $createdAt,
			p.updated_at = timestamp()
	`

	params := map[string]interface{}{
		"userID":    pattern.UserID,
		"emailID":   pattern.EmailID,
		"from":      pattern.From,
		"subject":   pattern.Subject,
		"snippet":   pattern.Snippet,
		"category":  pattern.Category,
		"priority":  pattern.Priority,
		"tags":      pattern.Tags,
		"intent":    pattern.Intent,
		"isManual":  pattern.IsManual,
		"embedding": embedding,
		"createdAt": pattern.CreatedAt.Unix(),
	}

	_, err := session.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to store classification pattern: %w", err)
	}

	// Link to User node
	linkQuery := `
		MATCH (u:User {user_id: $userID}), (p:ClassificationPattern {user_id: $userID, email_id: $emailID})
		MERGE (u)-[:HAS_PATTERN]->(p)
	`

	_, err = session.Run(ctx, linkQuery, map[string]interface{}{
		"userID":  pattern.UserID,
		"emailID": pattern.EmailID,
	})
	if err != nil {
		// Non-fatal error, user might not exist yet
		return nil
	}

	return nil
}

// =============================================================================
// Search Operations
// =============================================================================

// Search finds similar classification patterns using vector similarity.
func (a *ClassificationAdapter) Search(ctx context.Context, userID string, embedding []float32, topK int) ([]*out.ClassificationPattern, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		CALL db.index.vector.queryNodes('pattern_embedding_index', $topK * 2, $embedding)
		YIELD node, score
		WHERE node.user_id = $userID
		RETURN node.email_id AS email_id, node.from_addr AS from_addr,
			   node.subject AS subject, node.snippet AS snippet,
			   node.category AS category, node.priority AS priority,
			   node.tags AS tags, node.intent AS intent,
			   node.is_manual AS is_manual, node.created_at AS created_at,
			   score
		ORDER BY score DESC
		LIMIT $topK
	`

	params := map[string]interface{}{
		"userID":    userID,
		"embedding": embedding,
		"topK":      topK,
	}

	result, err := session.Run(ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("failed to search patterns: %w", err)
	}

	var patterns []*out.ClassificationPattern
	for result.Next(ctx) {
		record := result.Record()

		var createdAt time.Time
		if ts, ok := record.Get("created_at"); ok && ts != nil {
			if tsInt, ok := ts.(int64); ok {
				createdAt = time.Unix(tsInt, 0)
			}
		}

		var tags []string
		if t, ok := record.Get("tags"); ok && t != nil {
			if tagArr, ok := t.([]interface{}); ok {
				for _, tag := range tagArr {
					if s, ok := tag.(string); ok {
						tags = append(tags, s)
					}
				}
			}
		}

		pattern := &out.ClassificationPattern{
			UserID:    userID,
			EmailID:   getInt64Value(record, "email_id"),
			From:      getStringValue(record, "from_addr"),
			Subject:   getStringValue(record, "subject"),
			Snippet:   getStringValue(record, "snippet"),
			Category:  getStringValue(record, "category"),
			Priority:  getFloatValue(record, "priority"),
			Tags:      tags,
			Intent:    getStringValue(record, "intent"),
			IsManual:  getBoolValue(record, "is_manual"),
			CreatedAt: createdAt,
		}

		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// GetByCategory retrieves patterns by category.
func (a *ClassificationAdapter) GetByCategory(ctx context.Context, userID, category string, limit int) ([]*out.ClassificationPattern, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (p:ClassificationPattern {user_id: $userID, category: $category})
		RETURN p.email_id AS email_id, p.from_addr AS from_addr,
			   p.subject AS subject, p.snippet AS snippet,
			   p.category AS category, p.priority AS priority,
			   p.tags AS tags, p.intent AS intent,
			   p.is_manual AS is_manual, p.created_at AS created_at
		ORDER BY p.created_at DESC
		LIMIT $limit
	`

	params := map[string]interface{}{
		"userID":   userID,
		"category": category,
		"limit":    limit,
	}

	result, err := session.Run(ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get patterns by category: %w", err)
	}

	var patterns []*out.ClassificationPattern
	for result.Next(ctx) {
		record := result.Record()

		var createdAt time.Time
		if ts, ok := record.Get("created_at"); ok && ts != nil {
			if tsInt, ok := ts.(int64); ok {
				createdAt = time.Unix(tsInt, 0)
			}
		}

		var tags []string
		if t, ok := record.Get("tags"); ok && t != nil {
			if tagArr, ok := t.([]interface{}); ok {
				for _, tag := range tagArr {
					if s, ok := tag.(string); ok {
						tags = append(tags, s)
					}
				}
			}
		}

		pattern := &out.ClassificationPattern{
			UserID:    userID,
			EmailID:   getInt64Value(record, "email_id"),
			From:      getStringValue(record, "from_addr"),
			Subject:   getStringValue(record, "subject"),
			Snippet:   getStringValue(record, "snippet"),
			Category:  category,
			Priority:  getFloatValue(record, "priority"),
			Tags:      tags,
			Intent:    getStringValue(record, "intent"),
			IsManual:  getBoolValue(record, "is_manual"),
			CreatedAt: createdAt,
		}

		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// =============================================================================
// Delete Operations
// =============================================================================

// Delete removes a classification pattern.
func (a *ClassificationAdapter) Delete(ctx context.Context, userID string, emailID int64) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (p:ClassificationPattern {user_id: $userID, email_id: $emailID})
		DETACH DELETE p
	`

	params := map[string]interface{}{
		"userID":  userID,
		"emailID": emailID,
	}

	_, err := session.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to delete pattern: %w", err)
	}

	return nil
}

// =============================================================================
// Bulk Operations (Additional)
// =============================================================================

// BatchStore stores multiple patterns.
func (a *ClassificationAdapter) BatchStore(ctx context.Context, patterns []*out.ClassificationPattern, embeddings [][]float32) error {
	if len(patterns) == 0 || len(patterns) != len(embeddings) {
		return nil
	}

	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		UNWIND $items AS item
		MERGE (p:ClassificationPattern {user_id: item.user_id, email_id: item.email_id})
		SET p.from_addr = item.from_addr,
			p.subject = item.subject,
			p.snippet = item.snippet,
			p.category = item.category,
			p.priority = item.priority,
			p.tags = item.tags,
			p.intent = item.intent,
			p.is_manual = item.is_manual,
			p.embedding = item.embedding,
			p.created_at = item.created_at,
			p.updated_at = timestamp()
	`

	items := make([]map[string]interface{}, len(patterns))
	for i, p := range patterns {
		items[i] = map[string]interface{}{
			"user_id":    p.UserID,
			"email_id":   p.EmailID,
			"from_addr":  p.From,
			"subject":    p.Subject,
			"snippet":    p.Snippet,
			"category":   p.Category,
			"priority":   p.Priority,
			"tags":       p.Tags,
			"intent":     p.Intent,
			"is_manual":  p.IsManual,
			"embedding":  embeddings[i],
			"created_at": p.CreatedAt.Unix(),
		}
	}

	_, err := session.Run(ctx, query, map[string]interface{}{"items": items})
	if err != nil {
		return fmt.Errorf("failed to batch store patterns: %w", err)
	}

	return nil
}

// DeleteByUser removes all patterns for a user.
func (a *ClassificationAdapter) DeleteByUser(ctx context.Context, userID string) error {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (p:ClassificationPattern {user_id: $userID})
		DETACH DELETE p
	`

	_, err := session.Run(ctx, query, map[string]interface{}{"userID": userID})
	if err != nil {
		return fmt.Errorf("failed to delete user patterns: %w", err)
	}

	return nil
}

// GetStats returns pattern statistics for a user.
func (a *ClassificationAdapter) GetStats(ctx context.Context, userID string) (*PatternStats, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: a.dbName})
	defer session.Close(ctx)

	query := `
		MATCH (p:ClassificationPattern {user_id: $userID})
		RETURN count(p) AS total,
			   count(CASE WHEN p.is_manual THEN 1 END) AS manual,
			   collect(DISTINCT p.category) AS categories
	`

	result, err := session.Run(ctx, query, map[string]interface{}{"userID": userID})
	if err != nil {
		return nil, fmt.Errorf("failed to get pattern stats: %w", err)
	}

	stats := &PatternStats{}
	if result.Next(ctx) {
		record := result.Record()
		stats.Total = int(getInt64Value(record, "total"))
		stats.Manual = int(getInt64Value(record, "manual"))
		stats.Auto = stats.Total - stats.Manual

		if cats, ok := record.Get("categories"); ok && cats != nil {
			if catArr, ok := cats.([]interface{}); ok {
				for _, cat := range catArr {
					if s, ok := cat.(string); ok {
						stats.Categories = append(stats.Categories, s)
					}
				}
			}
		}
	}

	return stats, nil
}

// PatternStats represents classification pattern statistics.
type PatternStats struct {
	Total      int      `json:"total"`
	Manual     int      `json:"manual"`
	Auto       int      `json:"auto"`
	Categories []string `json:"categories"`
}

// =============================================================================
// Helper Functions
// =============================================================================

func getInt64Value(record *neo4j.Record, key string) int64 {
	if val, ok := record.Get(key); ok && val != nil {
		switch v := val.(type) {
		case int64:
			return v
		case int:
			return int64(v)
		case float64:
			return int64(v)
		}
	}
	return 0
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ out.ClassificationPatternStore = (*ClassificationAdapter)(nil)
