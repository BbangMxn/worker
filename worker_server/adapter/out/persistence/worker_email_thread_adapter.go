package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"worker_server/core/port/out"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// =============================================================================
// Thread Row Mapping
// =============================================================================

type threadRow struct {
	ID               int64           `db:"id"`
	UserID           uuid.UUID       `db:"user_id"`
	ConnectionID     int64           `db:"connection_id"`
	Provider         string          `db:"provider"`
	AccountEmail     string          `db:"account_email"`
	ExternalThreadID string          `db:"external_thread_id"`
	Subject          string          `db:"subject"`
	Snippet          string          `db:"snippet"`
	Participants     pq.StringArray  `db:"participants"`
	HasUnread        bool            `db:"has_unread"`
	HasStarred       bool            `db:"has_starred"`
	HasAttachment    bool            `db:"has_attachment"`
	WorkflowStatus   string          `db:"workflow_status"`
	SnoozedUntil     sql.NullTime    `db:"snooze_until"`
	AIStatus         string          `db:"ai_status"`
	AICategory       sql.NullString  `db:"ai_category"`
	AIPriority       sql.NullFloat64 `db:"ai_priority"`
	AISummary        sql.NullString  `db:"ai_summary"`
	MessageCount     int             `db:"message_count"`
	LatestDate       time.Time       `db:"latest_date"`
	CreatedAt        time.Time       `db:"created_at"`
	UpdatedAt        time.Time       `db:"updated_at"`
}

func (r *threadRow) toEntity() *out.MailThreadEntity {
	entity := &out.MailThreadEntity{
		ID:               r.ID,
		UserID:           r.UserID,
		ConnectionID:     r.ConnectionID,
		Provider:         r.Provider,
		AccountEmail:     r.AccountEmail,
		ExternalThreadID: r.ExternalThreadID,
		Subject:          r.Subject,
		Snippet:          r.Snippet,
		Participants:     r.Participants,
		HasUnread:        r.HasUnread,
		HasStarred:       r.HasStarred,
		HasAttachment:    r.HasAttachment,
		WorkflowStatus:   r.WorkflowStatus,
		AIStatus:         r.AIStatus,
		MessageCount:     r.MessageCount,
		LatestAt:         r.LatestDate,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}

	if r.SnoozedUntil.Valid {
		entity.SnoozedUntil = &r.SnoozedUntil.Time
	}
	if r.AICategory.Valid {
		entity.Category = r.AICategory.String
	}
	if r.AIPriority.Valid {
		entity.Priority = r.AIPriority.Float64
	}
	if r.AISummary.Valid {
		entity.Summary = r.AISummary.String
	}

	return entity
}

// =============================================================================
// Thread Operations
// =============================================================================

// GetThreadMessages gets all messages in a thread.
func (a *MailAdapter) GetThreadMessages(ctx context.Context, threadID int64) ([]*out.MailEntity, error) {
	query := fmt.Sprintf(`
		SELECT %s,
			c.name as contact_name,
			c.company as contact_company,
			c.photo_url as contact_photo
		FROM emails e
		LEFT JOIN contacts c ON c.user_id = e.user_id AND c.email = e.from_email
		WHERE e.thread_id = $1
		ORDER BY e.email_date ASC`, mailSelectColumns)

	rows, err := a.db.QueryxContext(ctx, query, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []*out.MailEntity
	for rows.Next() {
		var row mailRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		emails = append(emails, row.toEntity())
	}

	return emails, nil
}

// GetThreadByID gets a thread by ID.
func (a *MailAdapter) GetThreadByID(ctx context.Context, threadID int64) (*out.MailThreadEntity, error) {
	var row threadRow
	err := a.db.QueryRowxContext(ctx, "SELECT * FROM email_threads WHERE id = $1", threadID).StructScan(&row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("thread not found")
		}
		return nil, err
	}
	return row.toEntity(), nil
}

// threadRowWithCount extends threadRow with total count for window function optimization
type threadRowWithCount struct {
	threadRow
	TotalCount int `db:"total_count"`
}

// ListThreads lists email threads using window function for single query optimization.
// Combines COUNT + SELECT into 1 query for ~50% performance improvement.
func (a *MailAdapter) ListThreads(ctx context.Context, userID uuid.UUID, req *out.MailListQuery) ([]*out.MailThreadEntity, int, error) {
	if req == nil {
		req = &out.MailListQuery{}
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 50
	}

	where, args := a.buildThreadWhereClause(userID, req)

	// Single query with COUNT(*) OVER() window function
	argIdx := len(args) + 1
	selectQuery := fmt.Sprintf(`
		SELECT t.*, COUNT(*) OVER() as total_count
		FROM email_threads t
		WHERE %s
		ORDER BY t.latest_date DESC
		LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1)
	args = append(args, req.Limit, req.Offset)

	rows, err := a.db.QueryxContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var threads []*out.MailThreadEntity
	var total int
	for rows.Next() {
		var row threadRowWithCount
		if err := rows.StructScan(&row); err != nil {
			return nil, 0, err
		}
		threads = append(threads, row.threadRow.toEntity())
		total = row.TotalCount
	}

	return threads, total, nil
}

// GetOrCreateThread gets or creates a thread for an email.
func (a *MailAdapter) GetOrCreateThread(ctx context.Context, mail *out.MailEntity) (int64, error) {
	if mail.ThreadID == nil || *mail.ThreadID == 0 {
		return 0, nil
	}

	// Try to find existing thread
	var threadID int64
	err := a.db.QueryRowxContext(ctx, `
		SELECT id FROM email_threads
		WHERE connection_id = $1 AND external_thread_id = $2`,
		mail.ConnectionID, mail.ExternalID).Scan(&threadID)

	if err == nil {
		return threadID, nil
	}

	if err != sql.ErrNoRows {
		return 0, err
	}

	// Create new thread
	participants := collectParticipants(mail)
	err = a.db.QueryRowxContext(ctx, `
		INSERT INTO email_threads (
			user_id, connection_id, provider, account_email, external_thread_id,
			subject, snippet, participants,
			has_unread, has_starred, has_attachment,
			workflow_status, ai_status,
			message_count, latest_date
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 1, $14)
		ON CONFLICT (connection_id, external_thread_id) DO UPDATE SET updated_at = NOW()
		RETURNING id`,
		mail.UserID, mail.ConnectionID, mail.Provider, mail.AccountEmail, mail.ExternalID,
		mail.Subject, mail.Snippet, pq.Array(participants),
		!mail.IsRead, isStarred(mail.Tags), mail.HasAttachment,
		"inbox", "none", mail.ReceivedAt,
	).Scan(&threadID)

	return threadID, err
}

// UpdateThreadStats updates thread statistics.
func (a *MailAdapter) UpdateThreadStats(ctx context.Context, threadID int64) error {
	_, err := a.db.ExecContext(ctx, `
		UPDATE email_threads t SET
			subject = COALESCE((
				SELECT subject FROM emails WHERE thread_id = t.id ORDER BY email_date ASC LIMIT 1
			), t.subject),
			snippet = COALESCE((
				SELECT snippet FROM emails WHERE thread_id = t.id ORDER BY email_date DESC LIMIT 1
			), t.snippet),
			participants = COALESCE((
				SELECT array_agg(DISTINCT e) FROM (
					SELECT unnest(array_cat(ARRAY[from_email], array_cat(to_emails, cc_emails))) as e
					FROM emails WHERE thread_id = t.id
				) sub
			), t.participants),
			has_unread = EXISTS(SELECT 1 FROM emails WHERE thread_id = t.id AND is_read = false),
			has_starred = EXISTS(SELECT 1 FROM emails WHERE thread_id = t.id AND 'starred' = ANY(tags)),
			has_attachment = EXISTS(SELECT 1 FROM emails WHERE thread_id = t.id AND has_attachment = true),
			message_count = (SELECT COUNT(*) FROM emails WHERE thread_id = t.id),
			latest_date = COALESCE((SELECT MAX(email_date) FROM emails WHERE thread_id = t.id), t.latest_date),
			updated_at = NOW()
		WHERE t.id = $1`, threadID)
	return err
}

// UpdateThreadAIResult updates thread AI result.
func (a *MailAdapter) UpdateThreadAIResult(ctx context.Context, threadID int64, result *out.MailAIResult) error {
	_, err := a.db.ExecContext(ctx, `
		UPDATE email_threads SET
			ai_status = COALESCE(NULLIF($1, '')::ai_status, ai_status), ai_category = $2, ai_priority = $3, ai_summary = $4,
			updated_at = NOW()
		WHERE id = $5`,
		result.Status, result.Category, nullFloat64(result.Priority), result.Summary, threadID)
	return err
}

// =============================================================================
// Snooze Operations
// =============================================================================

// GetSnoozedToWake gets snoozed emails ready to wake.
func (a *MailAdapter) GetSnoozedToWake(ctx context.Context) ([]*out.MailEntity, error) {
	query := fmt.Sprintf(`
		SELECT %s,
			c.name as contact_name,
			c.company as contact_company,
			c.photo_url as contact_photo
		FROM emails e
		LEFT JOIN contacts c ON c.user_id = e.user_id AND c.email = e.from_email
		WHERE e.workflow_status = 'snoozed' AND e.snooze_until <= NOW()`, mailSelectColumns)

	rows, err := a.db.QueryxContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []*out.MailEntity
	for rows.Next() {
		var row mailRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		emails = append(emails, row.toEntity())
	}

	return emails, nil
}

// UnsnoozeExpired unsnoozes expired emails.
func (a *MailAdapter) UnsnoozeExpired(ctx context.Context) (int, error) {
	result, err := a.db.ExecContext(ctx, `
		UPDATE emails SET
			workflow_status = 'inbox',
			snooze_until = NULL,
			updated_at = NOW()
		WHERE workflow_status = 'snoozed' AND snooze_until <= NOW()`)
	if err != nil {
		return 0, err
	}
	rows, _ := result.RowsAffected()
	return int(rows), nil
}

// =============================================================================
// Resync
// =============================================================================

// GetEmailsWithPendingAttachments returns emails that have pending attachment IDs.
func (a *MailAdapter) GetEmailsWithPendingAttachments(ctx context.Context, userID uuid.UUID, connectionID int64) ([]*out.MailEntity, error) {
	query := fmt.Sprintf(`
		SELECT DISTINCT %s,
			NULL as contact_name,
			NULL as contact_company,
			NULL as contact_photo
		FROM emails e
		INNER JOIN email_attachments ea ON e.id = ea.email_id
		WHERE e.user_id = $1 AND e.connection_id = $2 AND ea.external_id LIKE 'pending_%%'
		ORDER BY e.email_date DESC
		LIMIT 500`, mailSelectColumns)

	var rows []mailRow
	if err := a.db.SelectContext(ctx, &rows, query, userID, connectionID); err != nil {
		return nil, err
	}

	result := make([]*out.MailEntity, len(rows))
	for i, row := range rows {
		result[i] = row.toEntity()
	}
	return result, nil
}

// GetEmailsNeedingAttachmentResync returns emails that may have attachments but have none recorded.
// This catches emails synced with metadata format that didn't fetch attachment details.
func (a *MailAdapter) GetEmailsNeedingAttachmentResync(ctx context.Context, userID uuid.UUID, connectionID int64, limit int) ([]*out.MailEntity, error) {
	if limit <= 0 {
		limit = 200
	}
	query := fmt.Sprintf(`
		SELECT %s,
			NULL as contact_name,
			NULL as contact_company,
			NULL as contact_photo
		FROM emails e
		WHERE e.user_id = $1 AND e.connection_id = $2
		AND NOT EXISTS (SELECT 1 FROM email_attachments ea WHERE ea.email_id = e.id)
		ORDER BY e.email_date DESC
		LIMIT $3`, mailSelectColumns)

	var rows []mailRow
	if err := a.db.SelectContext(ctx, &rows, query, userID, connectionID, limit); err != nil {
		return nil, err
	}

	result := make([]*out.MailEntity, len(rows))
	for i, row := range rows {
		result[i] = row.toEntity()
	}
	return result, nil
}

// GetEmailsByExternalIDsNeedingAttachments returns emails with given external IDs that don't have attachments recorded.
func (a *MailAdapter) GetEmailsByExternalIDsNeedingAttachments(ctx context.Context, userID uuid.UUID, connectionID int64, externalIDs []string) ([]*out.MailEntity, error) {
	if len(externalIDs) == 0 {
		return nil, nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(externalIDs))
	args := make([]interface{}, len(externalIDs)+2)
	args[0] = userID
	args[1] = connectionID
	for i, id := range externalIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+3)
		args[i+2] = id
	}

	query := fmt.Sprintf(`
		SELECT %s,
			NULL as contact_name,
			NULL as contact_company,
			NULL as contact_photo
		FROM emails e
		WHERE e.user_id = $1 AND e.connection_id = $2
		AND e.external_id IN (%s)
		AND NOT EXISTS (SELECT 1 FROM email_attachments ea WHERE ea.email_id = e.id)
		ORDER BY e.email_date DESC`, mailSelectColumns, strings.Join(placeholders, ","))

	var rows []mailRow
	if err := a.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}

	result := make([]*out.MailEntity, len(rows))
	for i, row := range rows {
		result[i] = row.toEntity()
	}
	return result, nil
}

// =============================================================================
// AI Pending
// =============================================================================

// ListPendingAI lists emails pending AI processing.
// This includes emails with ai_status='pending' or ai_category IS NULL
func (a *MailAdapter) ListPendingAI(ctx context.Context, userID uuid.UUID, limit int) ([]*out.MailEntity, error) {
	if limit <= 0 {
		limit = 50
	}

	query := fmt.Sprintf(`
		SELECT %s,
			c.name as contact_name,
			c.company as contact_company,
			c.photo_url as contact_photo
		FROM emails e
		LEFT JOIN contacts c ON c.user_id = e.user_id AND c.email = e.from_email
		WHERE e.user_id = $1 AND (e.ai_status IN ('none', 'pending') OR e.ai_category IS NULL)
		ORDER BY e.email_date DESC
		LIMIT $2`, mailSelectColumns)

	rows, err := a.db.QueryxContext(ctx, query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []*out.MailEntity
	for rows.Next() {
		var row mailRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		emails = append(emails, row.toEntity())
	}

	return emails, nil
}

// CountUnclassified counts emails that need classification for a specific connection.
func (a *MailAdapter) CountUnclassified(ctx context.Context, connectionID int64) (int, error) {
	var count int
	err := a.db.QueryRowxContext(ctx, `
		SELECT COUNT(*) FROM emails
		WHERE connection_id = $1 AND (ai_status IN ('none', 'pending') OR ai_category IS NULL)`,
		connectionID).Scan(&count)
	return count, err
}

// ListUnclassifiedByConnection lists emails that need classification for a specific connection.
// Used for reclassification after OAuth reconnect or classification errors.
func (a *MailAdapter) ListUnclassifiedByConnection(ctx context.Context, connectionID int64, limit int) ([]*out.MailEntity, error) {
	if limit <= 0 {
		limit = 100
	}

	query := fmt.Sprintf(`
		SELECT %s,
			c.name as contact_name,
			c.company as contact_company,
			c.photo_url as contact_photo
		FROM emails e
		LEFT JOIN contacts c ON c.user_id = e.user_id AND c.email = e.from_email
		WHERE e.connection_id = $1 AND (e.ai_status IN ('none', 'pending') OR e.ai_category IS NULL)
		ORDER BY e.email_date DESC
		LIMIT $2`, mailSelectColumns)

	rows, err := a.db.QueryxContext(ctx, query, connectionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []*out.MailEntity
	for rows.Next() {
		var row mailRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		emails = append(emails, row.toEntity())
	}

	return emails, nil
}

// =============================================================================
// Bulk Operations
// =============================================================================

// BulkUpsert bulk upserts emails.
func (a *MailAdapter) BulkUpsert(ctx context.Context, userID uuid.UUID, connectionID int64, mails []*out.MailEntity) error {
	if len(mails) == 0 {
		return nil
	}

	// Process in batches of 100
	const batchSize = 100
	for i := 0; i < len(mails); i += batchSize {
		end := i + batchSize
		if end > len(mails) {
			end = len(mails)
		}
		batch := mails[i:end]

		if err := a.bulkUpsertBatch(ctx, userID, connectionID, batch); err != nil {
			return err
		}
	}

	return nil
}

// bulkUpsertColumns defines the columns for bulk upsert (order matters, must match values)
var bulkUpsertColumns = []string{
	"user_id", "connection_id", "provider", "account_email", "external_id", "message_id",
	"from_email", "from_name", "to_emails", "cc_emails", "bcc_emails",
	"subject", "snippet", "direction", "folder", "labels",
	"is_read", "is_draft", "has_attachment", "is_replied", "is_forwarded",
	"tags", "workflow_status", "ai_status", "email_date",
}

// buildPlaceholders generates ($1, $2, ..., $N, NOW()) for a single row
func buildPlaceholders(rowIndex, paramsPerRow int) string {
	placeholders := make([]string, paramsPerRow)
	base := rowIndex * paramsPerRow
	for i := 0; i < paramsPerRow; i++ {
		placeholders[i] = fmt.Sprintf("$%d", base+i+1)
	}
	return "(" + strings.Join(placeholders, ", ") + ", NOW())"
}

// buildMailValues extracts values from MailEntity in column order
func buildMailValues(userID uuid.UUID, connectionID int64, mail *out.MailEntity) []interface{} {
	direction := mail.Direction
	if direction == "" {
		direction = "inbound"
		if mail.Folder == "sent" || mail.Folder == "drafts" {
			direction = "outbound"
		}
	}

	// workflow_status: 빈 문자열은 enum 오류 발생 → 기본값 "none" 사용
	workflowStatus := mail.WorkflowStatus
	if workflowStatus == "" {
		workflowStatus = "none"
	}

	// ai_status: 빈 문자열은 기본값 "pending" 사용
	aiStatus := mail.AIStatus
	if aiStatus == "" {
		aiStatus = "pending"
	}

	return []interface{}{
		userID, connectionID, mail.Provider, mail.AccountEmail, mail.ExternalID, nullStr(mail.MessageID),
		mail.FromEmail, nullStr(mail.FromName),
		pq.Array(mail.ToEmails), pq.Array(mail.CcEmails), pq.Array(mail.BccEmails),
		mail.Subject, mail.Snippet, direction, mail.Folder, pq.Array(mail.Labels),
		mail.IsRead, mail.IsDraft, mail.HasAttachment, mail.IsReplied, mail.IsForwarded,
		pq.Array(mail.Tags), workflowStatus, aiStatus,
		mail.ReceivedAt,
	}
}

func (a *MailAdapter) bulkUpsertBatch(ctx context.Context, userID uuid.UUID, connectionID int64, mails []*out.MailEntity) error {
	paramsPerRow := len(bulkUpsertColumns)
	valueStrings := make([]string, 0, len(mails))
	valueArgs := make([]interface{}, 0, len(mails)*paramsPerRow)

	for i, mail := range mails {
		valueStrings = append(valueStrings, buildPlaceholders(i, paramsPerRow))
		valueArgs = append(valueArgs, buildMailValues(userID, connectionID, mail)...)
	}

	// Build column list with updated_at appended
	columnList := strings.Join(bulkUpsertColumns, ", ") + ", updated_at"

	query := fmt.Sprintf(`
		INSERT INTO emails (%s) VALUES %s
		ON CONFLICT (user_id, connection_id, external_id)
		DO UPDATE SET
			from_email = EXCLUDED.from_email, from_name = EXCLUDED.from_name,
			to_emails = EXCLUDED.to_emails, cc_emails = EXCLUDED.cc_emails,
			subject = EXCLUDED.subject, snippet = EXCLUDED.snippet,
			direction = EXCLUDED.direction, folder = EXCLUDED.folder, labels = EXCLUDED.labels,
			is_read = EXCLUDED.is_read, tags = EXCLUDED.tags,
			has_attachment = EXCLUDED.has_attachment,
			updated_at = NOW()`,
		columnList, strings.Join(valueStrings, ", "))

	_, err := a.db.ExecContext(ctx, query, valueArgs...)
	return err
}

// DeleteByExternalIDs deletes emails by external IDs.
func (a *MailAdapter) DeleteByExternalIDs(ctx context.Context, connectionID int64, externalIDs []string) error {
	if len(externalIDs) == 0 {
		return nil
	}
	_, err := a.db.ExecContext(ctx,
		"DELETE FROM emails WHERE connection_id = $1 AND external_id = ANY($2)",
		connectionID, pq.Array(externalIDs))
	return err
}

// =============================================================================
// Translation
// =============================================================================

// SaveTranslation saves a translation.
func (a *MailAdapter) SaveTranslation(ctx context.Context, result *out.MailTranslation) error {
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO email_translations (email_id, target_language, translated_subject, translated_body)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email_id, target_language)
		DO UPDATE SET translated_subject = $3, translated_body = $4`,
		result.EmailID, result.TargetLang, result.Subject, result.Body)
	return err
}

// GetTranslation gets a translation.
func (a *MailAdapter) GetTranslation(ctx context.Context, emailID int64, targetLang string) (*out.MailTranslation, error) {
	var result out.MailTranslation
	err := a.db.QueryRowxContext(ctx, `
		SELECT email_id, target_language, translated_subject as subject, translated_body as body, created_at
		FROM email_translations
		WHERE email_id = $1 AND target_language = $2`,
		emailID, targetLang).Scan(&result.EmailID, &result.TargetLang, &result.Subject, &result.Body, &result.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

func (a *MailAdapter) buildThreadWhereClause(userID uuid.UUID, req *out.MailListQuery) (string, []interface{}) {
	conditions := []string{"t.user_id = $1"}
	args := []interface{}{userID}
	argIdx := 2

	if req.Category != "" {
		conditions = append(conditions, fmt.Sprintf("t.ai_category = $%d", argIdx))
		args = append(args, req.Category)
		argIdx++
	}

	if req.IsRead != nil {
		if *req.IsRead {
			conditions = append(conditions, "t.has_unread = false")
		} else {
			conditions = append(conditions, "t.has_unread = true")
		}
	}

	return strings.Join(conditions, " AND "), args
}

func collectParticipants(mail *out.MailEntity) []string {
	seen := make(map[string]bool)
	var participants []string

	addIfNew := func(email string) {
		if email != "" && !seen[email] {
			seen[email] = true
			participants = append(participants, email)
		}
	}

	addIfNew(mail.FromEmail)
	for _, e := range mail.ToEmails {
		addIfNew(e)
	}
	for _, e := range mail.CcEmails {
		addIfNew(e)
	}

	return participants
}

func isStarred(tags []string) bool {
	for _, t := range tags {
		if t == "starred" {
			return true
		}
	}
	return false
}
