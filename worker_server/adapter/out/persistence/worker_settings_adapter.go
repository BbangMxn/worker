// Package persistence provides database adapters implementing outbound ports.
package persistence

import (
	"context"
	"database/sql"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/out"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// SettingsAdapter implements out.SettingsRepository using PostgreSQL.
type SettingsAdapter struct {
	db *sqlx.DB
}

// NewSettingsAdapter creates a new SettingsAdapter.
func NewSettingsAdapter(db *sqlx.DB) *SettingsAdapter {
	return &SettingsAdapter{db: db}
}

// =============================================================================
// Classification Rules Operations
// =============================================================================

// classificationRulesRow represents the database row for classification rules.
type classificationRulesRow struct {
	ID                int64          `db:"id"`
	UserID            uuid.UUID      `db:"user_id"`
	ImportantDomains  pq.StringArray `db:"important_domains"`
	ImportantKeywords pq.StringArray `db:"important_keywords"`
	IgnoreSenders     pq.StringArray `db:"ignore_senders"`
	IgnoreKeywords    pq.StringArray `db:"ignore_keywords"`
	HighPriorityRules sql.NullString `db:"high_priority_rules"`
	LowPriorityRules  sql.NullString `db:"low_priority_rules"`
	CategoryRules     sql.NullString `db:"category_rules"`
	CreatedAt         time.Time      `db:"created_at"`
	UpdatedAt         time.Time      `db:"updated_at"`
}

func (r *classificationRulesRow) toEntity() *out.ClassificationRulesEntity {
	entity := &out.ClassificationRulesEntity{
		UserID:            r.UserID.String(),
		ImportantDomains:  r.ImportantDomains,
		ImportantKeywords: r.ImportantKeywords,
		IgnoreSenders:     r.IgnoreSenders,
		IgnoreKeywords:    r.IgnoreKeywords,
	}

	if r.HighPriorityRules.Valid {
		entity.HighPriorityRules = r.HighPriorityRules.String
	}
	if r.LowPriorityRules.Valid {
		entity.LowPriorityRules = r.LowPriorityRules.String
	}
	if r.CategoryRules.Valid {
		entity.CategoryRules = r.CategoryRules.String
	}

	return entity
}

// GetClassificationRules retrieves user's classification rules.
func (a *SettingsAdapter) GetClassificationRules(ctx context.Context, userID uuid.UUID) (*out.ClassificationRulesEntity, error) {
	const query = `
		SELECT id, user_id, important_domains, important_keywords,
		       ignore_senders, ignore_keywords, high_priority_rules,
		       low_priority_rules, category_rules, created_at, updated_at
		FROM classification_rules
		WHERE user_id = $1
	`

	var row classificationRulesRow
	if err := a.db.GetContext(ctx, &row, query, userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return row.toEntity(), nil
}

// UpsertClassificationRules creates or updates classification rules.
func (a *SettingsAdapter) UpsertClassificationRules(ctx context.Context, rules *out.ClassificationRulesEntity) error {
	userID, err := uuid.Parse(rules.UserID)
	if err != nil {
		return err
	}

	const query = `
		INSERT INTO classification_rules (
			user_id, important_domains, important_keywords,
			ignore_senders, ignore_keywords, high_priority_rules,
			low_priority_rules, category_rules, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW()
		)
		ON CONFLICT (user_id) DO UPDATE SET
			important_domains = EXCLUDED.important_domains,
			important_keywords = EXCLUDED.important_keywords,
			ignore_senders = EXCLUDED.ignore_senders,
			ignore_keywords = EXCLUDED.ignore_keywords,
			high_priority_rules = EXCLUDED.high_priority_rules,
			low_priority_rules = EXCLUDED.low_priority_rules,
			category_rules = EXCLUDED.category_rules,
			updated_at = NOW()
	`

	_, err = a.db.ExecContext(ctx, query,
		userID,
		pq.Array(rules.ImportantDomains),
		pq.Array(rules.ImportantKeywords),
		pq.Array(rules.IgnoreSenders),
		pq.Array(rules.IgnoreKeywords),
		nullString(rules.HighPriorityRules),
		nullString(rules.LowPriorityRules),
		nullString(rules.CategoryRules),
	)

	return err
}

// nullString converts empty string to sql.NullString.
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// =============================================================================
// UserSettings Operations (for domain.SettingsRepository)
// =============================================================================

// userSettingsRow represents the database row for user settings.
type userSettingsRow struct {
	ID               int64          `db:"id"`
	UserID           uuid.UUID      `db:"user_id"`
	DefaultSignature sql.NullString `db:"default_signature"`
	AutoReplyEnabled bool           `db:"auto_reply_enabled"`
	AutoReplyMessage sql.NullString `db:"auto_reply_message"`
	AIEnabled        bool           `db:"ai_enabled"`
	AIAutoClassify   bool           `db:"ai_auto_classify"`
	AITone           string         `db:"ai_tone"`
	Theme            string         `db:"theme"`
	Language         string         `db:"language"`
	Timezone         string         `db:"timezone"`
	CreatedAt        time.Time      `db:"created_at"`
	UpdatedAt        time.Time      `db:"updated_at"`
}

func (r *userSettingsRow) toDomain() *domain.UserSettings {
	settings := &domain.UserSettings{
		ID:               r.ID,
		UserID:           r.UserID,
		AutoReplyEnabled: r.AutoReplyEnabled,
		AIEnabled:        r.AIEnabled,
		AIAutoClassify:   r.AIAutoClassify,
		AITone:           r.AITone,
		Theme:            r.Theme,
		Language:         r.Language,
		Timezone:         r.Timezone,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}

	if r.DefaultSignature.Valid {
		settings.DefaultSignature = &r.DefaultSignature.String
	}
	if r.AutoReplyMessage.Valid {
		settings.AutoReplyMessage = &r.AutoReplyMessage.String
	}

	return settings
}

// GetByUserID retrieves user settings by user ID.
func (a *SettingsAdapter) GetByUserID(userID uuid.UUID) (*domain.UserSettings, error) {
	const query = `
		SELECT id, user_id, default_signature, auto_reply_enabled, auto_reply_message,
		       ai_enabled, ai_auto_classify, ai_tone,
		       theme, language, timezone, created_at, updated_at
		FROM user_settings
		WHERE user_id = $1
	`

	var row userSettingsRow
	if err := a.db.Get(&row, query, userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}

	return row.toDomain(), nil
}

// Create creates new user settings.
func (a *SettingsAdapter) Create(settings *domain.UserSettings) error {
	const query = `
		INSERT INTO user_settings (
			user_id, default_signature, auto_reply_enabled, auto_reply_message,
			ai_enabled, ai_auto_classify, ai_tone,
			theme, language, timezone
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
		RETURNING id, created_at, updated_at
	`

	var sig, autoReply sql.NullString
	if settings.DefaultSignature != nil {
		sig = sql.NullString{String: *settings.DefaultSignature, Valid: true}
	}
	if settings.AutoReplyMessage != nil {
		autoReply = sql.NullString{String: *settings.AutoReplyMessage, Valid: true}
	}

	return a.db.QueryRowx(query,
		settings.UserID,
		sig,
		settings.AutoReplyEnabled,
		autoReply,
		settings.AIEnabled,
		settings.AIAutoClassify,
		settings.AITone,
		settings.Theme,
		settings.Language,
		settings.Timezone,
	).Scan(&settings.ID, &settings.CreatedAt, &settings.UpdatedAt)
}

// Update updates existing user settings.
func (a *SettingsAdapter) Update(settings *domain.UserSettings) error {
	const query = `
		UPDATE user_settings SET
			default_signature = $1,
			auto_reply_enabled = $2,
			auto_reply_message = $3,
			ai_enabled = $4,
			ai_auto_classify = $5,
			ai_tone = $6,
			theme = $7,
			language = $8,
			timezone = $9,
			updated_at = NOW()
		WHERE user_id = $10
	`

	var sig, autoReply sql.NullString
	if settings.DefaultSignature != nil {
		sig = sql.NullString{String: *settings.DefaultSignature, Valid: true}
	}
	if settings.AutoReplyMessage != nil {
		autoReply = sql.NullString{String: *settings.AutoReplyMessage, Valid: true}
	}

	_, err := a.db.Exec(query,
		sig,
		settings.AutoReplyEnabled,
		autoReply,
		settings.AIEnabled,
		settings.AIAutoClassify,
		settings.AITone,
		settings.Theme,
		settings.Language,
		settings.Timezone,
		settings.UserID,
	)

	return err
}

// DomainGetClassificationRules retrieves classification rules as domain type.
func (a *SettingsAdapter) DomainGetClassificationRules(ctx context.Context, userID uuid.UUID) (*domain.ClassificationRules, error) {
	const query = `
		SELECT id, user_id, important_domains, important_keywords,
		       ignore_senders, ignore_keywords, high_priority_rules,
		       low_priority_rules, category_rules, created_at, updated_at
		FROM classification_rules
		WHERE user_id = $1
	`

	var row classificationRulesRow
	if err := a.db.GetContext(ctx, &row, query, userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	rules := &domain.ClassificationRules{
		ID:                int64(row.ID),
		UserID:            row.UserID,
		ImportantDomains:  row.ImportantDomains,
		ImportantKeywords: row.ImportantKeywords,
		IgnoreSenders:     row.IgnoreSenders,
		IgnoreKeywords:    row.IgnoreKeywords,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}

	if row.HighPriorityRules.Valid {
		rules.HighPriorityRules = row.HighPriorityRules.String
	}
	if row.LowPriorityRules.Valid {
		rules.LowPriorityRules = row.LowPriorityRules.String
	}
	if row.CategoryRules.Valid {
		rules.CategoryRules = row.CategoryRules.String
	}

	return rules, nil
}

// DomainSaveClassificationRules saves classification rules from domain type.
func (a *SettingsAdapter) DomainSaveClassificationRules(ctx context.Context, rules *domain.ClassificationRules) error {
	const query = `
		INSERT INTO classification_rules (
			user_id, important_domains, important_keywords,
			ignore_senders, ignore_keywords, high_priority_rules,
			low_priority_rules, category_rules, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW()
		)
		ON CONFLICT (user_id) DO UPDATE SET
			important_domains = EXCLUDED.important_domains,
			important_keywords = EXCLUDED.important_keywords,
			ignore_senders = EXCLUDED.ignore_senders,
			ignore_keywords = EXCLUDED.ignore_keywords,
			high_priority_rules = EXCLUDED.high_priority_rules,
			low_priority_rules = EXCLUDED.low_priority_rules,
			category_rules = EXCLUDED.category_rules,
			updated_at = NOW()
	`

	_, err := a.db.ExecContext(ctx, query,
		rules.UserID,
		pq.Array(rules.ImportantDomains),
		pq.Array(rules.ImportantKeywords),
		pq.Array(rules.IgnoreSenders),
		pq.Array(rules.IgnoreKeywords),
		nullString(rules.HighPriorityRules),
		nullString(rules.LowPriorityRules),
		nullString(rules.CategoryRules),
	)

	return err
}

// Ensure SettingsAdapter implements out.SettingsRepository
var _ out.SettingsRepository = (*SettingsAdapter)(nil)
