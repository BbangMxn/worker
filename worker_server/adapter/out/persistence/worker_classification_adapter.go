// Package persistence provides database adapters implementing outbound ports.
package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"worker_server/core/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// =============================================================================
// Classification Rule Adapter (v2, score-based)
// =============================================================================

// ClassificationRuleAdapter implements domain.ClassificationRuleRepository.
type ClassificationRuleAdapter struct {
	db *sqlx.DB
}

// NewClassificationRuleAdapter creates a new ClassificationRuleAdapter.
func NewClassificationRuleAdapter(db *sqlx.DB) *ClassificationRuleAdapter {
	return &ClassificationRuleAdapter{db: db}
}

// classificationRuleRow represents the database row.
type classificationRuleRow struct {
	ID        int64        `db:"id"`
	UserID    uuid.UUID    `db:"user_id"`
	Type      string       `db:"type"`
	Pattern   string       `db:"pattern"`
	Action    string       `db:"action"`
	Value     string       `db:"value"`
	Score     float64      `db:"score"`
	Position  int          `db:"position"`
	IsActive  bool         `db:"is_active"`
	HitCount  int          `db:"hit_count"`
	LastHitAt sql.NullTime `db:"last_hit_at"`
	CreatedAt time.Time    `db:"created_at"`
	UpdatedAt time.Time    `db:"updated_at"`
}

func (r *classificationRuleRow) toEntity() *domain.ScoreClassificationRule {
	rule := &domain.ScoreClassificationRule{
		ID:        r.ID,
		UserID:    r.UserID,
		Type:      domain.RuleType(r.Type),
		Pattern:   r.Pattern,
		Action:    domain.ScoreRuleAction(r.Action),
		Value:     r.Value,
		Score:     r.Score,
		Position:  r.Position,
		IsActive:  r.IsActive,
		HitCount:  r.HitCount,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	if r.LastHitAt.Valid {
		rule.LastHitAt = &r.LastHitAt.Time
	}
	return rule
}

// GetByID retrieves a rule by ID.
func (a *ClassificationRuleAdapter) GetByID(ctx context.Context, id int64) (*domain.ScoreClassificationRule, error) {
	var row classificationRuleRow
	query := `SELECT * FROM classification_rules_v2 WHERE id = $1`

	if err := a.db.GetContext(ctx, &row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get classification rule: %w", err)
	}

	return row.toEntity(), nil
}

// ListByUser retrieves all rules for a user.
func (a *ClassificationRuleAdapter) ListByUser(ctx context.Context, userID uuid.UUID) ([]*domain.ScoreClassificationRule, error) {
	var rows []classificationRuleRow
	query := `SELECT * FROM classification_rules_v2 WHERE user_id = $1 ORDER BY type, position`

	if err := a.db.SelectContext(ctx, &rows, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list classification rules: %w", err)
	}

	rules := make([]*domain.ScoreClassificationRule, len(rows))
	for i, row := range rows {
		rules[i] = row.toEntity()
	}

	return rules, nil
}

// ListByUserAndType retrieves rules by user and type.
func (a *ClassificationRuleAdapter) ListByUserAndType(ctx context.Context, userID uuid.UUID, ruleType domain.RuleType) ([]*domain.ScoreClassificationRule, error) {
	var rows []classificationRuleRow
	query := `SELECT * FROM classification_rules_v2 WHERE user_id = $1 AND type = $2 ORDER BY position`

	if err := a.db.SelectContext(ctx, &rows, query, userID, string(ruleType)); err != nil {
		return nil, fmt.Errorf("failed to list classification rules by type: %w", err)
	}

	rules := make([]*domain.ScoreClassificationRule, len(rows))
	for i, row := range rows {
		rules[i] = row.toEntity()
	}

	return rules, nil
}

// ListActiveByUser retrieves active rules for a user.
func (a *ClassificationRuleAdapter) ListActiveByUser(ctx context.Context, userID uuid.UUID) ([]*domain.ScoreClassificationRule, error) {
	var rows []classificationRuleRow
	query := `SELECT * FROM classification_rules_v2 WHERE user_id = $1 AND is_active = TRUE ORDER BY type, position`

	if err := a.db.SelectContext(ctx, &rows, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list active classification rules: %w", err)
	}

	rules := make([]*domain.ScoreClassificationRule, len(rows))
	for i, row := range rows {
		rules[i] = row.toEntity()
	}

	return rules, nil
}

// Create creates a new rule.
func (a *ClassificationRuleAdapter) Create(ctx context.Context, rule *domain.ScoreClassificationRule) error {
	query := `
		INSERT INTO classification_rules_v2 (user_id, type, pattern, action, value, score, position, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`

	err := a.db.QueryRowContext(ctx, query,
		rule.UserID, string(rule.Type), rule.Pattern, string(rule.Action),
		rule.Value, rule.Score, rule.Position, rule.IsActive,
	).Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create classification rule: %w", err)
	}

	return nil
}

// Update updates a rule.
func (a *ClassificationRuleAdapter) Update(ctx context.Context, rule *domain.ScoreClassificationRule) error {
	query := `
		UPDATE classification_rules_v2
		SET type = $2, pattern = $3, action = $4, value = $5, score = $6, position = $7, is_active = $8, updated_at = NOW()
		WHERE id = $1`

	result, err := a.db.ExecContext(ctx, query,
		rule.ID, string(rule.Type), rule.Pattern, string(rule.Action),
		rule.Value, rule.Score, rule.Position, rule.IsActive,
	)
	if err != nil {
		return fmt.Errorf("failed to update classification rule: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("classification rule not found: %d", rule.ID)
	}

	return nil
}

// Delete deletes a rule.
func (a *ClassificationRuleAdapter) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM classification_rules_v2 WHERE id = $1`

	result, err := a.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete classification rule: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("classification rule not found: %d", id)
	}

	return nil
}

// DeleteByUser deletes all rules for a user.
func (a *ClassificationRuleAdapter) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM classification_rules_v2 WHERE user_id = $1`

	_, err := a.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete classification rules: %w", err)
	}

	return nil
}

// UpdatePositions updates the positions of rules.
func (a *ClassificationRuleAdapter) UpdatePositions(ctx context.Context, userID uuid.UUID, ruleIDs []int64) error {
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for i, id := range ruleIDs {
		_, err := tx.ExecContext(ctx,
			`UPDATE classification_rules_v2 SET position = $1, updated_at = NOW() WHERE id = $2 AND user_id = $3`,
			i, id, userID,
		)
		if err != nil {
			return fmt.Errorf("failed to update position: %w", err)
		}
	}

	return tx.Commit()
}

// IncrementHitCount increments the hit count for a rule.
func (a *ClassificationRuleAdapter) IncrementHitCount(ctx context.Context, id int64) error {
	query := `UPDATE classification_rules_v2 SET hit_count = hit_count + 1, last_hit_at = NOW() WHERE id = $1`

	_, err := a.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to increment hit count: %w", err)
	}

	return nil
}

// =============================================================================
// Label Rule Adapter (Auto Labeling)
// =============================================================================

// LabelRuleAdapter implements domain.LabelRuleRepository.
type LabelRuleAdapter struct {
	db *sqlx.DB
}

// NewLabelRuleAdapter creates a new LabelRuleAdapter.
func NewLabelRuleAdapter(db *sqlx.DB) *LabelRuleAdapter {
	return &LabelRuleAdapter{db: db}
}

// labelRuleRow represents the database row.
type labelRuleRow struct {
	ID            int64        `db:"id"`
	UserID        uuid.UUID    `db:"user_id"`
	LabelID       int64        `db:"label_id"`
	Type          string       `db:"type"`
	Pattern       string       `db:"pattern"`
	Score         float64      `db:"score"`
	IsAutoCreated bool         `db:"is_auto_created"`
	IsActive      bool         `db:"is_active"`
	HitCount      int          `db:"hit_count"`
	LastHitAt     sql.NullTime `db:"last_hit_at"`
	CreatedAt     time.Time    `db:"created_at"`
	UpdatedAt     time.Time    `db:"updated_at"`
}

func (r *labelRuleRow) toEntity() *domain.LabelRule {
	rule := &domain.LabelRule{
		ID:            r.ID,
		UserID:        r.UserID,
		LabelID:       r.LabelID,
		Type:          domain.LabelRuleType(r.Type),
		Pattern:       r.Pattern,
		Score:         r.Score,
		IsAutoCreated: r.IsAutoCreated,
		IsActive:      r.IsActive,
		HitCount:      r.HitCount,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
	if r.LastHitAt.Valid {
		rule.LastHitAt = &r.LastHitAt.Time
	}
	return rule
}

// GetByID retrieves a label rule by ID.
func (a *LabelRuleAdapter) GetByID(ctx context.Context, id int64) (*domain.LabelRule, error) {
	var row labelRuleRow
	query := `SELECT * FROM label_rules WHERE id = $1`

	if err := a.db.GetContext(ctx, &row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get label rule: %w", err)
	}

	return row.toEntity(), nil
}

// ListByUser retrieves all label rules for a user.
func (a *LabelRuleAdapter) ListByUser(ctx context.Context, userID uuid.UUID) ([]*domain.LabelRule, error) {
	var rows []labelRuleRow
	query := `SELECT * FROM label_rules WHERE user_id = $1 ORDER BY label_id, type`

	if err := a.db.SelectContext(ctx, &rows, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list label rules: %w", err)
	}

	rules := make([]*domain.LabelRule, len(rows))
	for i, row := range rows {
		rules[i] = row.toEntity()
	}

	return rules, nil
}

// ListByLabel retrieves rules for a specific label.
func (a *LabelRuleAdapter) ListByLabel(ctx context.Context, labelID int64) ([]*domain.LabelRule, error) {
	var rows []labelRuleRow
	query := `SELECT * FROM label_rules WHERE label_id = $1 ORDER BY type`

	if err := a.db.SelectContext(ctx, &rows, query, labelID); err != nil {
		return nil, fmt.Errorf("failed to list label rules: %w", err)
	}

	rules := make([]*domain.LabelRule, len(rows))
	for i, row := range rows {
		rules[i] = row.toEntity()
	}

	return rules, nil
}

// ListByUserAndType retrieves label rules by user and type.
func (a *LabelRuleAdapter) ListByUserAndType(ctx context.Context, userID uuid.UUID, ruleType domain.LabelRuleType) ([]*domain.LabelRule, error) {
	var rows []labelRuleRow
	query := `SELECT * FROM label_rules WHERE user_id = $1 AND type = $2 ORDER BY label_id`

	if err := a.db.SelectContext(ctx, &rows, query, userID, string(ruleType)); err != nil {
		return nil, fmt.Errorf("failed to list label rules by type: %w", err)
	}

	rules := make([]*domain.LabelRule, len(rows))
	for i, row := range rows {
		rules[i] = row.toEntity()
	}

	return rules, nil
}

// ListActiveByUser retrieves active label rules for a user.
func (a *LabelRuleAdapter) ListActiveByUser(ctx context.Context, userID uuid.UUID) ([]*domain.LabelRule, error) {
	var rows []labelRuleRow
	query := `SELECT * FROM label_rules WHERE user_id = $1 AND is_active = TRUE ORDER BY label_id, type`

	if err := a.db.SelectContext(ctx, &rows, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list active label rules: %w", err)
	}

	rules := make([]*domain.LabelRule, len(rows))
	for i, row := range rows {
		rules[i] = row.toEntity()
	}

	return rules, nil
}

// FindByPattern finds a rule by pattern.
func (a *LabelRuleAdapter) FindByPattern(ctx context.Context, userID uuid.UUID, labelID int64, ruleType domain.LabelRuleType, pattern string) (*domain.LabelRule, error) {
	var row labelRuleRow
	query := `SELECT * FROM label_rules WHERE user_id = $1 AND label_id = $2 AND type = $3 AND pattern = $4`

	if err := a.db.GetContext(ctx, &row, query, userID, labelID, string(ruleType), pattern); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find label rule: %w", err)
	}

	return row.toEntity(), nil
}

// Create creates a new label rule.
func (a *LabelRuleAdapter) Create(ctx context.Context, rule *domain.LabelRule) error {
	query := `
		INSERT INTO label_rules (user_id, label_id, type, pattern, score, is_auto_created, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, label_id, type, pattern) DO NOTHING
		RETURNING id, created_at, updated_at`

	err := a.db.QueryRowContext(ctx, query,
		rule.UserID, rule.LabelID, string(rule.Type), rule.Pattern,
		rule.Score, rule.IsAutoCreated, rule.IsActive,
	).Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			// Conflict, rule already exists
			return nil
		}
		return fmt.Errorf("failed to create label rule: %w", err)
	}

	return nil
}

// Update updates a label rule.
func (a *LabelRuleAdapter) Update(ctx context.Context, rule *domain.LabelRule) error {
	query := `
		UPDATE label_rules
		SET type = $2, pattern = $3, score = $4, is_active = $5, updated_at = NOW()
		WHERE id = $1`

	result, err := a.db.ExecContext(ctx, query, rule.ID, string(rule.Type), rule.Pattern, rule.Score, rule.IsActive)
	if err != nil {
		return fmt.Errorf("failed to update label rule: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("label rule not found: %d", rule.ID)
	}

	return nil
}

// Delete deletes a label rule.
func (a *LabelRuleAdapter) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM label_rules WHERE id = $1`

	result, err := a.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete label rule: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("label rule not found: %d", id)
	}

	return nil
}

// DeleteByLabel deletes all rules for a label.
func (a *LabelRuleAdapter) DeleteByLabel(ctx context.Context, labelID int64) error {
	query := `DELETE FROM label_rules WHERE label_id = $1`

	_, err := a.db.ExecContext(ctx, query, labelID)
	if err != nil {
		return fmt.Errorf("failed to delete label rules: %w", err)
	}

	return nil
}

// DeleteAutoCreatedByLabel deletes auto-created rules for a label.
func (a *LabelRuleAdapter) DeleteAutoCreatedByLabel(ctx context.Context, labelID int64) error {
	query := `DELETE FROM label_rules WHERE label_id = $1 AND is_auto_created = TRUE`

	_, err := a.db.ExecContext(ctx, query, labelID)
	if err != nil {
		return fmt.Errorf("failed to delete auto-created label rules: %w", err)
	}

	return nil
}

// IncrementHitCount increments the hit count for a label rule.
func (a *LabelRuleAdapter) IncrementHitCount(ctx context.Context, id int64) error {
	query := `UPDATE label_rules SET hit_count = hit_count + 1, last_hit_at = NOW() WHERE id = $1`

	_, err := a.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to increment hit count: %w", err)
	}

	return nil
}

// =============================================================================
// Classification Cache Adapter (pgvector)
// =============================================================================

// ClassificationCacheAdapter implements domain.ClassificationCacheRepository using pgvector.
type ClassificationCacheAdapter struct {
	db *pgxpool.Pool // Using pgx for pgvector support
}

// NewClassificationCacheAdapter creates a new ClassificationCacheAdapter.
func NewClassificationCacheAdapter(db *pgxpool.Pool) *ClassificationCacheAdapter {
	return &ClassificationCacheAdapter{db: db}
}

// FindSimilar finds similar cached classifications using cosine similarity.
func (a *ClassificationCacheAdapter) FindSimilar(ctx context.Context, userID uuid.UUID, embedding []float32, minScore float64, limit int) ([]*domain.ClassificationCache, error) {
	query := `
		SELECT id, user_id, embedding, category, sub_category, priority, labels, score, usage_count, last_used_at, expires_at, created_at,
			   1 - (embedding <=> $1) as similarity
		FROM classification_cache
		WHERE user_id = $2
		  AND embedding IS NOT NULL
		  AND expires_at > NOW()
		  AND 1 - (embedding <=> $1) >= $3
		ORDER BY embedding <=> $1
		LIMIT $4`

	rows, err := a.db.Query(ctx, query, pgVector(embedding), userID, minScore, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to find similar cache: %w", err)
	}
	defer rows.Close()

	var results []*domain.ClassificationCache
	for rows.Next() {
		var cache domain.ClassificationCache
		var subCategory sql.NullString
		var similarity float64

		if err := rows.Scan(
			&cache.ID, &cache.UserID, &cache.Embedding,
			&cache.Category, &subCategory, &cache.Priority, pq.Array(&cache.Labels),
			&cache.Score, &cache.UsageCount, &cache.LastUsedAt, &cache.ExpiresAt, &cache.CreatedAt,
			&similarity,
		); err != nil {
			return nil, fmt.Errorf("failed to scan cache row: %w", err)
		}

		if subCategory.Valid {
			cache.SubCategory = &subCategory.String
		}

		// Override score with similarity for ranking
		cache.Score = similarity

		results = append(results, &cache)
	}

	return results, nil
}

// GetByID retrieves a cache entry by ID.
func (a *ClassificationCacheAdapter) GetByID(ctx context.Context, id int64) (*domain.ClassificationCache, error) {
	query := `SELECT id, user_id, embedding, category, sub_category, priority, labels, score, usage_count, last_used_at, expires_at, created_at
		FROM classification_cache WHERE id = $1`

	var cache domain.ClassificationCache
	var subCategory sql.NullString

	err := a.db.QueryRow(ctx, query, id).Scan(
		&cache.ID, &cache.UserID, &cache.Embedding,
		&cache.Category, &subCategory, &cache.Priority, pq.Array(&cache.Labels),
		&cache.Score, &cache.UsageCount, &cache.LastUsedAt, &cache.ExpiresAt, &cache.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get cache: %w", err)
	}

	if subCategory.Valid {
		cache.SubCategory = &subCategory.String
	}

	return &cache, nil
}

// Create creates a new cache entry.
func (a *ClassificationCacheAdapter) Create(ctx context.Context, cache *domain.ClassificationCache) error {
	query := `
		INSERT INTO classification_cache (user_id, embedding, category, sub_category, priority, labels, score)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, usage_count, last_used_at, expires_at, created_at`

	var subCategory sql.NullString
	if cache.SubCategory != nil {
		subCategory = sql.NullString{String: *cache.SubCategory, Valid: true}
	}

	err := a.db.QueryRow(ctx, query,
		cache.UserID, pgVector(cache.Embedding),
		cache.Category, subCategory, cache.Priority, pq.Array(cache.Labels), cache.Score,
	).Scan(&cache.ID, &cache.UsageCount, &cache.LastUsedAt, &cache.ExpiresAt, &cache.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create cache: %w", err)
	}

	return nil
}

// Delete deletes a cache entry.
func (a *ClassificationCacheAdapter) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM classification_cache WHERE id = $1`

	_, err := a.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete cache: %w", err)
	}

	return nil
}

// DeleteExpired deletes expired cache entries.
func (a *ClassificationCacheAdapter) DeleteExpired(ctx context.Context) (int, error) {
	query := `DELETE FROM classification_cache WHERE expires_at < NOW()`

	result, err := a.db.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired cache: %w", err)
	}

	return int(result.RowsAffected()), nil
}

// DeleteByUser deletes all cache entries for a user.
func (a *ClassificationCacheAdapter) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM classification_cache WHERE user_id = $1`

	_, err := a.db.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user cache: %w", err)
	}

	return nil
}

// IncrementUsageCount increments the usage count for a cache entry.
func (a *ClassificationCacheAdapter) IncrementUsageCount(ctx context.Context, id int64) error {
	query := `UPDATE classification_cache SET usage_count = usage_count + 1, last_used_at = NOW() WHERE id = $1`

	_, err := a.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to increment usage count: %w", err)
	}

	return nil
}

// pgVector converts float32 slice to pgvector format string.
func pgVector(v []float32) string {
	if len(v) == 0 {
		return "[0]"
	}

	buf := make([]byte, 0, len(v)*13+2)
	buf = append(buf, '[')

	for i, f := range v {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = fmt.Appendf(buf, "%f", f)
	}

	buf = append(buf, ']')
	return string(buf)
}
