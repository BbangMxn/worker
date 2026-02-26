package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/pkg/snowflake"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// ReminderRepository implements out.ReminderRepository
type ReminderRepository struct {
	db *sqlx.DB
}

// NewReminderRepository creates a new ReminderRepository
func NewReminderRepository(db *sqlx.DB) out.ReminderRepository {
	return &ReminderRepository{db: db}
}

// =============================================================================
// Reminder CRUD
// =============================================================================

func (r *ReminderRepository) GetReminder(ctx context.Context, id int64) (*domain.Reminder, error) {
	query := `
		SELECT id, user_id, rule_id, source_type, source_id,
		       title, message, url, remind_at, timezone, channels,
		       status, sent_at, snoozed_until, metadata, created_at, updated_at
		FROM reminders
		WHERE id = $1`

	var row reminderRow
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get reminder: %w", err)
	}

	return row.toDomain(), nil
}

func (r *ReminderRepository) ListReminders(ctx context.Context, filter *domain.ReminderFilter) ([]*domain.Reminder, int, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIdx))
	args = append(args, filter.UserID)
	argIdx++

	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *filter.Status)
		argIdx++
	}

	if len(filter.Statuses) > 0 {
		conditions = append(conditions, fmt.Sprintf("status = ANY($%d)", argIdx))
		args = append(args, pq.Array(filter.Statuses))
		argIdx++
	}

	if filter.SourceType != nil {
		conditions = append(conditions, fmt.Sprintf("source_type = $%d", argIdx))
		args = append(args, *filter.SourceType)
		argIdx++
	}

	if filter.SourceID != nil {
		conditions = append(conditions, fmt.Sprintf("source_id = $%d", argIdx))
		args = append(args, *filter.SourceID)
		argIdx++
	}

	if filter.RuleID != nil {
		conditions = append(conditions, fmt.Sprintf("rule_id = $%d", argIdx))
		args = append(args, *filter.RuleID)
		argIdx++
	}

	if filter.RemindAtFrom != nil {
		conditions = append(conditions, fmt.Sprintf("remind_at >= $%d", argIdx))
		args = append(args, *filter.RemindAtFrom)
		argIdx++
	}

	if filter.RemindAtTo != nil {
		conditions = append(conditions, fmt.Sprintf("remind_at <= $%d", argIdx))
		args = append(args, *filter.RemindAtTo)
		argIdx++
	}

	if filter.Pending {
		conditions = append(conditions, "status = 'pending'")
		conditions = append(conditions, "remind_at <= NOW()")
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM reminders WHERE %s", whereClause)
	var total int
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("count reminders: %w", err)
	}

	// Sort
	orderBy := "remind_at ASC"
	if filter.SortBy == "created_at" {
		orderBy = "created_at"
		if filter.SortOrder == "desc" {
			orderBy += " DESC"
		}
	}

	// Data
	query := fmt.Sprintf(`
		SELECT id, user_id, rule_id, source_type, source_id,
		       title, message, url, remind_at, timezone, channels,
		       status, sent_at, snoozed_until, metadata, created_at, updated_at
		FROM reminders
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d`,
		whereClause, orderBy, argIdx, argIdx+1)

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit, filter.Offset)

	var rows []reminderRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, 0, fmt.Errorf("list reminders: %w", err)
	}

	reminders := make([]*domain.Reminder, len(rows))
	for i, row := range rows {
		reminders[i] = row.toDomain()
	}

	return reminders, total, nil
}

func (r *ReminderRepository) CreateReminder(ctx context.Context, reminder *domain.Reminder) error {
	if reminder.ID == 0 {
		reminder.ID = snowflake.ID()
	}
	if reminder.CreatedAt.IsZero() {
		reminder.CreatedAt = time.Now()
	}
	reminder.UpdatedAt = time.Now()

	metadata, _ := json.Marshal(reminder.Metadata)
	channels := make([]string, len(reminder.Channels))
	for i, c := range reminder.Channels {
		channels[i] = string(c)
	}

	query := `
		INSERT INTO reminders (
			id, user_id, rule_id, source_type, source_id,
			title, message, url, remind_at, timezone, channels,
			status, sent_at, snoozed_until, metadata, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16, $17
		)`

	_, err := r.db.ExecContext(ctx, query,
		reminder.ID, reminder.UserID, reminder.RuleID, reminder.SourceType, reminder.SourceID,
		reminder.Title, reminder.Message, reminder.URL, reminder.RemindAt, reminder.Timezone,
		pq.Array(channels), reminder.Status, reminder.SentAt, reminder.SnoozedUntil,
		metadata, reminder.CreatedAt, reminder.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create reminder: %w", err)
	}

	return nil
}

func (r *ReminderRepository) UpdateReminder(ctx context.Context, reminder *domain.Reminder) error {
	reminder.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(reminder.Metadata)
	channels := make([]string, len(reminder.Channels))
	for i, c := range reminder.Channels {
		channels[i] = string(c)
	}

	query := `
		UPDATE reminders SET
			rule_id = $2, source_type = $3, source_id = $4, title = $5,
			message = $6, url = $7, remind_at = $8, timezone = $9, channels = $10,
			status = $11, sent_at = $12, snoozed_until = $13, metadata = $14, updated_at = $15
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		reminder.ID, reminder.RuleID, reminder.SourceType, reminder.SourceID, reminder.Title,
		reminder.Message, reminder.URL, reminder.RemindAt, reminder.Timezone, pq.Array(channels),
		reminder.Status, reminder.SentAt, reminder.SnoozedUntil, metadata, reminder.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update reminder: %w", err)
	}

	return nil
}

func (r *ReminderRepository) DeleteReminder(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM reminders WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete reminder: %w", err)
	}
	return nil
}

// =============================================================================
// Batch Operations
// =============================================================================

func (r *ReminderRepository) CreateReminders(ctx context.Context, reminders []*domain.Reminder) error {
	if len(reminders) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, reminder := range reminders {
		if reminder.ID == 0 {
			reminder.ID = snowflake.ID()
		}
		if reminder.CreatedAt.IsZero() {
			reminder.CreatedAt = time.Now()
		}
		reminder.UpdatedAt = time.Now()

		metadata, _ := json.Marshal(reminder.Metadata)
		channels := make([]string, len(reminder.Channels))
		for i, c := range reminder.Channels {
			channels[i] = string(c)
		}

		query := `
			INSERT INTO reminders (
				id, user_id, rule_id, source_type, source_id,
				title, message, url, remind_at, timezone, channels,
				status, sent_at, snoozed_until, metadata, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
				$12, $13, $14, $15, $16, $17
			)`

		_, err := tx.ExecContext(ctx, query,
			reminder.ID, reminder.UserID, reminder.RuleID, reminder.SourceType, reminder.SourceID,
			reminder.Title, reminder.Message, reminder.URL, reminder.RemindAt, reminder.Timezone,
			pq.Array(channels), reminder.Status, reminder.SentAt, reminder.SnoozedUntil,
			metadata, reminder.CreatedAt, reminder.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("create reminder batch: %w", err)
		}
	}

	return tx.Commit()
}

// =============================================================================
// Status Updates
// =============================================================================

func (r *ReminderRepository) MarkReminderSent(ctx context.Context, id int64) error {
	query := `UPDATE reminders SET status = 'sent', sent_at = NOW(), updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("mark sent: %w", err)
	}
	return nil
}

func (r *ReminderRepository) CancelReminder(ctx context.Context, id int64) error {
	query := `UPDATE reminders SET status = 'cancelled', updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("cancel reminder: %w", err)
	}
	return nil
}

func (r *ReminderRepository) SnoozeReminder(ctx context.Context, id int64, until time.Time) error {
	query := `UPDATE reminders SET status = 'snoozed', snoozed_until = $2, remind_at = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, until)
	if err != nil {
		return fmt.Errorf("snooze reminder: %w", err)
	}
	return nil
}

// =============================================================================
// By Source
// =============================================================================

func (r *ReminderRepository) GetRemindersBySource(ctx context.Context, sourceType domain.ReminderSourceType, sourceID string) ([]*domain.Reminder, error) {
	query := `
		SELECT id, user_id, rule_id, source_type, source_id,
		       title, message, url, remind_at, timezone, channels,
		       status, sent_at, snoozed_until, metadata, created_at, updated_at
		FROM reminders
		WHERE source_type = $1 AND source_id = $2
		ORDER BY remind_at`

	var rows []reminderRow
	if err := r.db.SelectContext(ctx, &rows, query, sourceType, sourceID); err != nil {
		return nil, fmt.Errorf("get by source: %w", err)
	}

	reminders := make([]*domain.Reminder, len(rows))
	for i, row := range rows {
		reminders[i] = row.toDomain()
	}
	return reminders, nil
}

func (r *ReminderRepository) DeleteRemindersBySource(ctx context.Context, sourceType domain.ReminderSourceType, sourceID string) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM reminders WHERE source_type = $1 AND source_id = $2",
		sourceType, sourceID)
	if err != nil {
		return fmt.Errorf("delete by source: %w", err)
	}
	return nil
}

// =============================================================================
// Scheduling
// =============================================================================

func (r *ReminderRepository) GetPendingReminders(ctx context.Context, until time.Time, limit int) ([]*domain.Reminder, error) {
	query := `
		SELECT id, user_id, rule_id, source_type, source_id,
		       title, message, url, remind_at, timezone, channels,
		       status, sent_at, snoozed_until, metadata, created_at, updated_at
		FROM reminders
		WHERE status = 'pending' AND remind_at <= $1
		ORDER BY remind_at
		LIMIT $2`

	var rows []reminderRow
	if err := r.db.SelectContext(ctx, &rows, query, until, limit); err != nil {
		return nil, fmt.Errorf("get pending: %w", err)
	}

	reminders := make([]*domain.Reminder, len(rows))
	for i, row := range rows {
		reminders[i] = row.toDomain()
	}
	return reminders, nil
}

func (r *ReminderRepository) GetSnoozedReminders(ctx context.Context, until time.Time) ([]*domain.Reminder, error) {
	query := `
		SELECT id, user_id, rule_id, source_type, source_id,
		       title, message, url, remind_at, timezone, channels,
		       status, sent_at, snoozed_until, metadata, created_at, updated_at
		FROM reminders
		WHERE status = 'snoozed' AND snoozed_until <= $1
		ORDER BY snoozed_until`

	var rows []reminderRow
	if err := r.db.SelectContext(ctx, &rows, query, until); err != nil {
		return nil, fmt.Errorf("get snoozed: %w", err)
	}

	reminders := make([]*domain.Reminder, len(rows))
	for i, row := range rows {
		reminders[i] = row.toDomain()
	}
	return reminders, nil
}

// =============================================================================
// ReminderRule CRUD
// =============================================================================

func (r *ReminderRepository) GetRule(ctx context.Context, id int64) (*domain.ReminderRule, error) {
	query := `
		SELECT id, user_id, name, is_enabled, target_type, conditions,
		       trigger_type, offset_minutes, trigger_time, channels,
		       sort_order, created_at, updated_at
		FROM reminder_rules
		WHERE id = $1`

	var row ruleRow
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get rule: %w", err)
	}

	return row.toDomain(), nil
}

func (r *ReminderRepository) ListRules(ctx context.Context, filter *domain.ReminderRuleFilter) ([]*domain.ReminderRule, int, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIdx))
	args = append(args, filter.UserID)
	argIdx++

	if filter.TargetType != nil {
		conditions = append(conditions, fmt.Sprintf("target_type = $%d", argIdx))
		args = append(args, *filter.TargetType)
		argIdx++
	}

	if filter.IsEnabled != nil {
		conditions = append(conditions, fmt.Sprintf("is_enabled = $%d", argIdx))
		args = append(args, *filter.IsEnabled)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM reminder_rules WHERE %s", whereClause)
	var total int
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("count rules: %w", err)
	}

	// Data
	query := fmt.Sprintf(`
		SELECT id, user_id, name, is_enabled, target_type, conditions,
		       trigger_type, offset_minutes, trigger_time, channels,
		       sort_order, created_at, updated_at
		FROM reminder_rules
		WHERE %s
		ORDER BY sort_order, created_at
		LIMIT $%d OFFSET $%d`,
		whereClause, argIdx, argIdx+1)

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit, filter.Offset)

	var rows []ruleRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, 0, fmt.Errorf("list rules: %w", err)
	}

	rules := make([]*domain.ReminderRule, len(rows))
	for i, row := range rows {
		rules[i] = row.toDomain()
	}

	return rules, total, nil
}

func (r *ReminderRepository) CreateRule(ctx context.Context, rule *domain.ReminderRule) error {
	if rule.ID == 0 {
		rule.ID = snowflake.ID()
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now()
	}
	rule.UpdatedAt = time.Now()

	conditions, _ := json.Marshal(rule.Conditions)
	channels := make([]string, len(rule.Channels))
	for i, c := range rule.Channels {
		channels[i] = string(c)
	}

	query := `
		INSERT INTO reminder_rules (
			id, user_id, name, is_enabled, target_type, conditions,
			trigger_type, offset_minutes, trigger_time, channels,
			sort_order, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)`

	_, err := r.db.ExecContext(ctx, query,
		rule.ID, rule.UserID, rule.Name, rule.IsEnabled, rule.TargetType, conditions,
		rule.TriggerType, rule.OffsetMinutes, rule.TriggerTime, pq.Array(channels),
		rule.SortOrder, rule.CreatedAt, rule.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create rule: %w", err)
	}

	return nil
}

func (r *ReminderRepository) UpdateRule(ctx context.Context, rule *domain.ReminderRule) error {
	rule.UpdatedAt = time.Now()
	conditions, _ := json.Marshal(rule.Conditions)
	channels := make([]string, len(rule.Channels))
	for i, c := range rule.Channels {
		channels[i] = string(c)
	}

	query := `
		UPDATE reminder_rules SET
			name = $2, is_enabled = $3, target_type = $4, conditions = $5,
			trigger_type = $6, offset_minutes = $7, trigger_time = $8, channels = $9,
			sort_order = $10, updated_at = $11
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		rule.ID, rule.Name, rule.IsEnabled, rule.TargetType, conditions,
		rule.TriggerType, rule.OffsetMinutes, rule.TriggerTime, pq.Array(channels),
		rule.SortOrder, rule.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update rule: %w", err)
	}

	return nil
}

func (r *ReminderRepository) DeleteRule(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM reminder_rules WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	return nil
}

// =============================================================================
// Rule Operations
// =============================================================================

func (r *ReminderRepository) GetEnabledRules(ctx context.Context, userID uuid.UUID, targetType domain.ReminderSourceType) ([]*domain.ReminderRule, error) {
	query := `
		SELECT id, user_id, name, is_enabled, target_type, conditions,
		       trigger_type, offset_minutes, trigger_time, channels,
		       sort_order, created_at, updated_at
		FROM reminder_rules
		WHERE user_id = $1 AND target_type = $2 AND is_enabled = true
		ORDER BY sort_order`

	var rows []ruleRow
	if err := r.db.SelectContext(ctx, &rows, query, userID, targetType); err != nil {
		return nil, fmt.Errorf("get enabled rules: %w", err)
	}

	rules := make([]*domain.ReminderRule, len(rows))
	for i, row := range rows {
		rules[i] = row.toDomain()
	}
	return rules, nil
}

func (r *ReminderRepository) ToggleRule(ctx context.Context, id int64, enabled bool) error {
	query := `UPDATE reminder_rules SET is_enabled = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, enabled)
	if err != nil {
		return fmt.Errorf("toggle rule: %w", err)
	}
	return nil
}

// =============================================================================
// Row Types
// =============================================================================

type reminderRow struct {
	ID           int64          `db:"id"`
	UserID       uuid.UUID      `db:"user_id"`
	RuleID       sql.NullInt64  `db:"rule_id"`
	SourceType   string         `db:"source_type"`
	SourceID     sql.NullString `db:"source_id"`
	Title        string         `db:"title"`
	Message      sql.NullString `db:"message"`
	URL          sql.NullString `db:"url"`
	RemindAt     time.Time      `db:"remind_at"`
	Timezone     string         `db:"timezone"`
	Channels     pq.StringArray `db:"channels"`
	Status       string         `db:"status"`
	SentAt       sql.NullTime   `db:"sent_at"`
	SnoozedUntil sql.NullTime   `db:"snoozed_until"`
	Metadata     []byte         `db:"metadata"`
	CreatedAt    time.Time      `db:"created_at"`
	UpdatedAt    time.Time      `db:"updated_at"`
}

func (r *reminderRow) toDomain() *domain.Reminder {
	reminder := &domain.Reminder{
		ID:         r.ID,
		UserID:     r.UserID,
		SourceType: domain.ReminderSourceType(r.SourceType),
		Title:      r.Title,
		RemindAt:   r.RemindAt,
		Timezone:   r.Timezone,
		Status:     domain.ReminderStatus(r.Status),
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}

	if r.RuleID.Valid {
		reminder.RuleID = &r.RuleID.Int64
	}
	if r.SourceID.Valid {
		reminder.SourceID = &r.SourceID.String
	}
	if r.Message.Valid {
		reminder.Message = &r.Message.String
	}
	if r.URL.Valid {
		reminder.URL = &r.URL.String
	}
	if r.SentAt.Valid {
		reminder.SentAt = &r.SentAt.Time
	}
	if r.SnoozedUntil.Valid {
		reminder.SnoozedUntil = &r.SnoozedUntil.Time
	}
	if len(r.Metadata) > 0 {
		json.Unmarshal(r.Metadata, &reminder.Metadata)
	}

	reminder.Channels = make([]domain.ReminderChannel, len(r.Channels))
	for i, c := range r.Channels {
		reminder.Channels[i] = domain.ReminderChannel(c)
	}

	return reminder
}

type ruleRow struct {
	ID            int64          `db:"id"`
	UserID        uuid.UUID      `db:"user_id"`
	Name          string         `db:"name"`
	IsEnabled     bool           `db:"is_enabled"`
	TargetType    string         `db:"target_type"`
	Conditions    []byte         `db:"conditions"`
	TriggerType   string         `db:"trigger_type"`
	OffsetMinutes sql.NullInt32  `db:"offset_minutes"`
	TriggerTime   sql.NullString `db:"trigger_time"`
	Channels      pq.StringArray `db:"channels"`
	SortOrder     int            `db:"sort_order"`
	CreatedAt     time.Time      `db:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at"`
}

func (r *ruleRow) toDomain() *domain.ReminderRule {
	rule := &domain.ReminderRule{
		ID:          r.ID,
		UserID:      r.UserID,
		Name:        r.Name,
		IsEnabled:   r.IsEnabled,
		TargetType:  domain.ReminderSourceType(r.TargetType),
		TriggerType: domain.ReminderTriggerType(r.TriggerType),
		SortOrder:   r.SortOrder,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}

	if len(r.Conditions) > 0 {
		json.Unmarshal(r.Conditions, &rule.Conditions)
	}
	if r.OffsetMinutes.Valid {
		offset := int(r.OffsetMinutes.Int32)
		rule.OffsetMinutes = &offset
	}
	if r.TriggerTime.Valid {
		rule.TriggerTime = &r.TriggerTime.String
	}

	rule.Channels = make([]domain.ReminderChannel, len(r.Channels))
	for i, c := range r.Channels {
		rule.Channels[i] = domain.ReminderChannel(c)
	}

	return rule
}
