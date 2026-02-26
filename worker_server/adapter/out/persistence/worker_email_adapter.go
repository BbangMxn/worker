// Package persistence provides database adapters implementing outbound ports.
package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"worker_server/core/port/out"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// =============================================================================
// Mail Adapter (Supabase/PostgreSQL)
// =============================================================================

// MailAdapter implements out.EmailRepository using PostgreSQL.
type MailAdapter struct {
	db *sqlx.DB
}

// NewMailAdapter creates a new MailAdapter.
func NewMailAdapter(db *sqlx.DB) *MailAdapter {
	return &MailAdapter{db: db}
}

// =============================================================================
// Database Row Mapping
// =============================================================================

// mailSelectColumns contains explicit column names for SELECT queries.
// We exclude the 'embedding' column (vector type) as it's handled separately by RAG.
const mailSelectColumns = `
	e.id, e.external_id, e.external_thread_id, e.thread_id, e.connection_id, e.user_id,
	e.provider, e.account_email, e.message_id, e.in_reply_to, e.references,
	e.from_email, e.from_name, e.to_emails, e.cc_emails, e.bcc_emails,
	e.subject, e.snippet, e.direction, e.is_read, e.is_draft, e.has_attachment, e.is_replied, e.is_forwarded,
	e.folder, e.labels, e.tags, e.workflow_status, e.snooze_until,
	e.ai_status, e.ai_category, e.ai_priority, e.ai_summary, e.ai_intent, e.ai_is_urgent,
	e.ai_due_date, e.ai_action_item, e.ai_sentiment, e.ai_tags,
	e.contact_id, e.email_date, e.created_at, e.updated_at`

// mailRow represents the database row for emails.
type mailRow struct {
	ID               int64          `db:"id"`
	ExternalID       string         `db:"external_id"`
	ExternalThreadID sql.NullString `db:"external_thread_id"`
	ThreadID         sql.NullInt64  `db:"thread_id"`
	ConnectionID     int64          `db:"connection_id"`
	UserID           uuid.UUID      `db:"user_id"`
	Provider         string         `db:"provider"`
	AccountEmail     string         `db:"account_email"`

	// Message IDs
	MessageID  sql.NullString `db:"message_id"`
	InReplyTo  sql.NullString `db:"in_reply_to"`
	References pq.StringArray `db:"references"`

	// Sender/Recipients
	FromEmail string         `db:"from_email"`
	FromName  sql.NullString `db:"from_name"`
	ToEmails  pq.StringArray `db:"to_emails"`
	CcEmails  pq.StringArray `db:"cc_emails"`
	BccEmails pq.StringArray `db:"bcc_emails"`

	// Content
	Subject string `db:"subject"`
	Snippet string `db:"snippet"`

	// Status
	Direction     string `db:"direction"`
	IsRead        bool   `db:"is_read"`
	IsDraft       bool   `db:"is_draft"`
	HasAttachment bool   `db:"has_attachment"`
	IsReplied     bool   `db:"is_replied"`
	IsForwarded   bool   `db:"is_forwarded"`

	// Organization
	Folder string         `db:"folder"`
	Labels pq.StringArray `db:"labels"`
	Tags   pq.StringArray `db:"tags"`

	// Workflow
	WorkflowStatus string       `db:"workflow_status"`
	SnoozedUntil   sql.NullTime `db:"snooze_until"`

	// AI
	AIStatus   string          `db:"ai_status"`
	AICategory sql.NullString  `db:"ai_category"`
	AIPriority sql.NullFloat64 `db:"ai_priority"` // 0.0 ~ 1.0 priority score
	AISummary  sql.NullString  `db:"ai_summary"`
	AIIntent   sql.NullString  `db:"ai_intent"`
	AIIsUrgent sql.NullBool    `db:"ai_is_urgent"`
	AIDueDate  sql.NullTime    `db:"ai_due_date"`
	ActionItem sql.NullString  `db:"ai_action_item"`
	Sentiment  sql.NullFloat64 `db:"ai_sentiment"`
	AITags     pq.StringArray  `db:"ai_tags"`

	// Contact
	ContactID sql.NullInt64 `db:"contact_id"`

	// Embedding (vector)
	// Note: embedding column은 별도 쿼리로 처리 (pgvector)

	// Timestamps
	ReceivedAt time.Time `db:"email_date"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`

	// Join fields (optional)
	ContactName    sql.NullString `db:"contact_name"`
	ContactCompany sql.NullString `db:"contact_company"`
	ContactPhoto   sql.NullString `db:"contact_photo"`
}

// mailRowWithCount extends mailRow with total count for optimized queries
// COUNT(*) OVER() 윈도우 함수 결과를 저장
type mailRowWithCount struct {
	mailRow
	TotalCount int `db:"total_count"`
}

func (r *mailRowWithCount) toEntity() *out.MailEntity {
	return r.mailRow.toEntity()
}

// mailRowWithScoreCount includes search score for BM25-style ranking.
type mailRowWithScoreCount struct {
	mailRow
	TotalCount  int     `db:"total_count"`
	SearchScore float64 `db:"search_score"`
}

func (r *mailRowWithScoreCount) toEntity() *out.MailEntity {
	return r.mailRow.toEntity()
}

func (r *mailRow) toEntity() *out.MailEntity {
	entity := &out.MailEntity{
		ID:             r.ID,
		ExternalID:     r.ExternalID,
		ConnectionID:   r.ConnectionID,
		UserID:         r.UserID,
		Provider:       r.Provider,
		AccountEmail:   r.AccountEmail,
		FromEmail:      r.FromEmail,
		ToEmails:       r.ToEmails,
		CcEmails:       r.CcEmails,
		BccEmails:      r.BccEmails,
		Subject:        r.Subject,
		Snippet:        r.Snippet,
		Direction:      r.Direction,
		IsRead:         r.IsRead,
		IsDraft:        r.IsDraft,
		HasAttachment:  r.HasAttachment,
		IsReplied:      r.IsReplied,
		IsForwarded:    r.IsForwarded,
		Folder:         r.Folder,
		Labels:         r.Labels,
		Tags:           r.Tags,
		WorkflowStatus: r.WorkflowStatus,
		AIStatus:       r.AIStatus,
		ReceivedAt:     r.ReceivedAt,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}

	if r.ThreadID.Valid {
		entity.ThreadID = &r.ThreadID.Int64
	}
	if r.MessageID.Valid {
		entity.MessageID = r.MessageID.String
	}
	if r.InReplyTo.Valid {
		entity.InReplyTo = r.InReplyTo.String
	}
	if r.References != nil {
		entity.References = r.References
	}
	if r.FromName.Valid {
		entity.FromName = r.FromName.String
	}
	if r.SnoozedUntil.Valid {
		entity.SnoozedUntil = &r.SnoozedUntil.Time
	}
	if r.AICategory.Valid {
		entity.Category = r.AICategory.String
	}
	if r.AIPriority.Valid {
		// Priority is now stored as float64 (0.0 ~ 1.0)
		entity.Priority = r.AIPriority.Float64
	}
	if r.AISummary.Valid {
		entity.Summary = r.AISummary.String
	}
	if r.Sentiment.Valid {
		entity.Sentiment = r.Sentiment.Float64
	}
	if r.ActionItem.Valid {
		entity.ActionItem = r.ActionItem.String
	}
	if r.ContactID.Valid {
		entity.ContactID = &r.ContactID.Int64
	}

	return entity
}

// =============================================================================
// CRUD Operations
// =============================================================================

// Create creates a new email.
func (a *MailAdapter) Create(ctx context.Context, mail *out.MailEntity) error {
	query := `
		INSERT INTO emails (
			user_id, connection_id, provider, account_email,
			external_id, thread_id, message_id, in_reply_to, "references",
			from_email, from_name, to_emails, cc_emails, bcc_emails,
			subject, snippet, folder, labels, tags,
			is_read, is_draft, has_attachment, is_replied, is_forwarded,
			workflow_status, snooze_until,
			ai_status, ai_category, ai_priority, ai_summary, ai_sentiment, ai_action_item,
			contact_id, email_date
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26,
			$27, $28, $29, $30, $31, $32, $33, $34
		)
		ON CONFLICT (user_id, connection_id, external_id) DO UPDATE SET
			labels = EXCLUDED.labels,
			is_read = EXCLUDED.is_read,
			folder = EXCLUDED.folder,
			updated_at = NOW()
		RETURNING id, created_at, updated_at`

	return a.db.QueryRowxContext(ctx, query,
		mail.UserID, mail.ConnectionID, mail.Provider, mail.AccountEmail,
		mail.ExternalID, mail.ThreadID, nullStr(mail.MessageID), nullStr(mail.InReplyTo), pq.Array(mail.References),
		mail.FromEmail, nullStr(mail.FromName), pq.Array(mail.ToEmails), pq.Array(mail.CcEmails), pq.Array(mail.BccEmails),
		mail.Subject, mail.Snippet, mail.Folder, pq.Array(mail.Labels), pq.Array(mail.Tags),
		mail.IsRead, mail.IsDraft, mail.HasAttachment, mail.IsReplied, mail.IsForwarded,
		mail.WorkflowStatus, mail.SnoozedUntil,
		mail.AIStatus, nullStr(mail.Category), nullFloat64(mail.Priority), nullStr(mail.Summary), mail.Sentiment, nullStr(mail.ActionItem),
		mail.ContactID, mail.ReceivedAt,
	).Scan(&mail.ID, &mail.CreatedAt, &mail.UpdatedAt)
}

// Update updates an existing email.
func (a *MailAdapter) Update(ctx context.Context, mail *out.MailEntity) error {
	query := `
		UPDATE emails SET
			thread_id = $1, from_email = $2, from_name = $3,
			to_emails = $4, cc_emails = $5, bcc_emails = $6,
			subject = $7, snippet = $8, folder = $9, labels = $10, tags = $11,
			is_read = $12, is_draft = $13, has_attachment = $14, is_replied = $15, is_forwarded = $16,
			workflow_status = COALESCE(NULLIF($17, '')::workflow_status, workflow_status), snooze_until = $18,
			ai_status = COALESCE(NULLIF($19, '')::ai_status, ai_status), ai_category = $20, ai_sub_category = $21,
			ai_priority = COALESCE($22, ai_priority),
			ai_summary = $23, ai_sentiment = $24, ai_action_item = $25,
			ai_score = $26, classification_source = $27,
			contact_id = $28, updated_at = NOW()
		WHERE id = $29`

	result, err := a.db.ExecContext(ctx, query,
		mail.ThreadID, mail.FromEmail, nullStr(mail.FromName),
		pq.Array(mail.ToEmails), pq.Array(mail.CcEmails), pq.Array(mail.BccEmails),
		mail.Subject, mail.Snippet, mail.Folder, pq.Array(mail.Labels), pq.Array(mail.Tags),
		mail.IsRead, mail.IsDraft, mail.HasAttachment, mail.IsReplied, mail.IsForwarded,
		mail.WorkflowStatus, mail.SnoozedUntil,
		mail.AIStatus, nullStr(mail.Category), nullSubCategory(mail.SubCategory), nullFloat64(mail.Priority),
		nullStr(mail.Summary), mail.Sentiment, nullStr(mail.ActionItem),
		nullFloat64(mail.AIScore), nullStr(mail.ClassificationSource),
		mail.ContactID, mail.ID,
	)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("email not found")
	}
	return nil
}

// Delete deletes an email.
func (a *MailAdapter) Delete(ctx context.Context, id int64) error {
	result, err := a.db.ExecContext(ctx, "DELETE FROM emails WHERE id = $1", id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("email not found")
	}
	return nil
}

// GetByID gets an email by ID with contact info.
func (a *MailAdapter) GetByID(ctx context.Context, id int64) (*out.MailEntity, error) {
	query := fmt.Sprintf(`
		SELECT %s,
			c.name as contact_name,
			c.company as contact_company,
			c.photo_url as contact_photo
		FROM emails e
		LEFT JOIN contacts c ON c.user_id = e.user_id AND c.email = e.from_email
		WHERE e.id = $1`, mailSelectColumns)

	var row mailRow
	if err := a.db.QueryRowxContext(ctx, query, id).StructScan(&row); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("email not found")
		}
		return nil, err
	}

	return row.toEntity(), nil
}

// GetByExternalID gets an email by external ID.
func (a *MailAdapter) GetByExternalID(ctx context.Context, connectionID int64, externalID string) (*out.MailEntity, error) {
	query := fmt.Sprintf(`
		SELECT %s,
			c.name as contact_name,
			c.company as contact_company,
			c.photo_url as contact_photo
		FROM emails e
		LEFT JOIN contacts c ON c.user_id = e.user_id AND c.email = e.from_email
		WHERE e.connection_id = $1 AND e.external_id = $2`, mailSelectColumns)

	var row mailRow
	if err := a.db.QueryRowxContext(ctx, query, connectionID, externalID).StructScan(&row); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("email not found")
		}
		return nil, err
	}

	return row.toEntity(), nil
}

// GetByExternalIDs gets emails by external IDs (batch query - N+1 해결).
// 최적화: Contact JOIN 제거 - 동기화 시에는 contact 정보 불필요
func (a *MailAdapter) GetByExternalIDs(ctx context.Context, connectionID int64, externalIDs []string) (map[string]*out.MailEntity, error) {
	if len(externalIDs) == 0 {
		return make(map[string]*out.MailEntity), nil
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM emails e
		WHERE e.connection_id = $1 AND e.external_id = ANY($2)`, mailSelectColumns)

	rows, err := a.db.QueryxContext(ctx, query, connectionID, pq.Array(externalIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to query emails: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*out.MailEntity, len(externalIDs))
	var scanErrors int
	for rows.Next() {
		var row mailRow
		if err := rows.StructScan(&row); err != nil {
			scanErrors++
			// Log error but continue processing other rows
			continue
		}
		entity := row.toEntity()
		result[entity.ExternalID] = entity
	}

	// Check for iteration errors
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("error iterating rows: %w", err)
	}

	return result, nil
}

// =============================================================================
// Query Operations
// =============================================================================

// List lists emails with filters.
// List lists emails with filters.
// 최적화: COUNT(*) OVER() 윈도우 함수로 단일 쿼리 (2번 → 1번으로 감소)
// Contact JOIN 제거로 쿼리 단순화 (목록에서는 불필요)
func (a *MailAdapter) List(ctx context.Context, userID uuid.UUID, req *out.MailListQuery) ([]*out.MailEntity, int, error) {
	if req == nil {
		req = &out.MailListQuery{}
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 50
	}

	// TODO 목록은 우선순위 순으로 정렬 (높은 우선순위 먼저)
	// ai_priority: 5(Urgent) > 4(High) > 3(Normal) > 2(Low) > 1(Lowest)
	if req.WorkflowStatus == "todo" && req.OrderBy == "" {
		req.OrderBy = "ai_priority"
		req.Order = "desc"
	}

	if req.OrderBy == "" {
		req.OrderBy = "email_date"
	}
	if req.Order == "" {
		req.Order = "desc"
	}

	where, args := a.buildWhereClause(userID, req)

	// Validate order by (SQL injection 방지)
	validOrderBy := map[string]bool{
		"email_date": true, "created_at": true, "updated_at": true,
		"ai_priority": true, "from_email": true, "subject": true,
	}
	if !validOrderBy[req.OrderBy] {
		req.OrderBy = "email_date"
	}
	if req.Order != "asc" && req.Order != "desc" {
		req.Order = "desc"
	}

	// 최적화: 단일 쿼리로 데이터 + COUNT 조회 (윈도우 함수)
	// Contact JOIN 제거 - 목록 조회 시 불필요, 상세 조회 시에만 사용
	// embedding 컬럼 제외 - 벡터 타입은 별도 처리
	argIdx := len(args) + 1

	// ORDER BY 절 생성
	// TODO 목록: 우선순위 → 날짜 (같은 우선순위면 최신 먼저)
	orderClause := fmt.Sprintf("e.%s %s", req.OrderBy, req.Order)
	if req.WorkflowStatus == "todo" && req.OrderBy == "ai_priority" {
		// 2차 정렬: 같은 우선순위 내에서 최신 날짜 먼저
		orderClause = fmt.Sprintf("e.ai_priority %s, e.email_date DESC", req.Order)
	}

	selectQuery := fmt.Sprintf(`
		SELECT %s, COUNT(*) OVER() as total_count
		FROM emails e
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d`,
		mailSelectColumns, where, orderClause, argIdx, argIdx+1)
	args = append(args, req.Limit, req.Offset)

	rows, err := a.db.QueryxContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var emails []*out.MailEntity
	var total int
	for rows.Next() {
		var row mailRowWithCount
		if err := rows.StructScan(&row); err != nil {
			return nil, 0, err
		}
		emails = append(emails, row.toEntity())
		total = row.TotalCount // 모든 행에서 동일한 값
	}

	return emails, total, nil
}

// Search searches emails using PostgreSQL full-text search.
// 최적화: GIN 인덱스 활용 + 단일 쿼리 + 윈도우 함수
// 검색 우선순위: 1) Full-text (subject + snippet) 2) From email exact match
func (a *MailAdapter) Search(ctx context.Context, userID uuid.UUID, query string, limit, offset int) ([]*out.MailEntity, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Full-text search query 생성: "hello world" → "hello:* & world:*"
	tsQuery := buildTsQuery(query)

	// BM25 스타일 검색 점수 계산:
	// - 제목 매칭: 가중치 2.0 (제목이 더 중요)
	// - 본문 매칭: 가중치 1.0
	// - 발신자 매칭: 가중치 1.5 (발신자 검색도 중요)
	// - ts_rank 정규화: normalization 32 (문서 길이 정규화)
	selectSQL := fmt.Sprintf(`
		SELECT %s,
			COUNT(*) OVER() as total_count,
			(
				COALESCE(ts_rank(setweight(to_tsvector('english', e.subject), 'A'), to_tsquery('english', $2), 32), 0) * 2.0 +
				COALESCE(ts_rank(to_tsvector('english', e.snippet), to_tsquery('english', $2), 32), 0) * 1.0 +
				CASE WHEN e.from_email ILIKE $3 OR e.from_name ILIKE $3 THEN 0.5 ELSE 0 END
			) as search_score
		FROM emails e
		WHERE e.user_id = $1
		AND (
			to_tsvector('english', e.subject || ' ' || e.snippet) @@ to_tsquery('english', $2)
			OR e.from_email ILIKE $3
			OR e.from_name ILIKE $3
		)
		ORDER BY search_score DESC, e.email_date DESC
		LIMIT $4 OFFSET $5`, mailSelectColumns)

	likeQuery := "%" + query + "%"
	rows, err := a.db.QueryxContext(ctx, selectSQL, userID, tsQuery, likeQuery, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var emails []*out.MailEntity
	var total int
	for rows.Next() {
		var row mailRowWithScoreCount
		if err := rows.StructScan(&row); err != nil {
			return nil, 0, err
		}
		entity := row.toEntity()
		entity.SearchScore = row.SearchScore // BM25 스타일 점수 저장
		emails = append(emails, entity)
		total = row.TotalCount
	}

	return emails, total, nil
}

// buildTsQuery converts user input to PostgreSQL tsquery format.
// "hello world" → "hello:* & world:*" (prefix matching with AND)
func buildTsQuery(query string) string {
	words := strings.Fields(query)
	if len(words) == 0 {
		return ""
	}

	var parts []string
	for _, word := range words {
		// 특수문자 제거 (SQL injection 방지)
		clean := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r >= 0x80 { // 유니코드 허용
				return r
			}
			return -1
		}, word)
		if clean != "" {
			parts = append(parts, clean+":*")
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " & ")
}

// ListByContact lists emails by contact ID using window function optimization.
// Combines 3 queries (contact lookup + COUNT + SELECT) into 2 for ~33% improvement.
func (a *MailAdapter) ListByContact(ctx context.Context, userID uuid.UUID, contactID int64, limit, offset int) ([]*out.MailEntity, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Get contact email first (필수 - contact 존재 확인)
	var contactEmail string
	if err := a.db.QueryRowxContext(ctx, "SELECT email FROM contacts WHERE id = $1 AND user_id = $2", contactID, userID).Scan(&contactEmail); err != nil {
		if err == sql.ErrNoRows {
			return nil, 0, fmt.Errorf("contact not found")
		}
		return nil, 0, err
	}

	// Single query with COUNT(*) OVER() - removed unnecessary LEFT JOIN for list view
	selectSQL := fmt.Sprintf(`
		SELECT %s,
			NULL as contact_name,
			NULL as contact_company,
			NULL as contact_photo,
			COUNT(*) OVER() as total_count
		FROM emails e
		WHERE e.user_id = $1 AND (e.from_email = $2 OR $2 = ANY(e.to_emails))
		ORDER BY e.email_date DESC
		LIMIT $3 OFFSET $4`, mailSelectColumns)

	rows, err := a.db.QueryxContext(ctx, selectSQL, userID, contactEmail, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var emails []*out.MailEntity
	var total int
	for rows.Next() {
		var row mailRowWithCount
		if err := rows.StructScan(&row); err != nil {
			return nil, 0, err
		}
		emails = append(emails, row.toEntity())
		total = row.TotalCount
	}

	return emails, total, nil
}

// =============================================================================
// Status Updates
// =============================================================================

// UpdateReadStatus updates read status.
func (a *MailAdapter) UpdateReadStatus(ctx context.Context, id int64, isRead bool) error {
	_, err := a.db.ExecContext(ctx,
		"UPDATE emails SET is_read = $1, updated_at = NOW() WHERE id = $2",
		isRead, id)
	return err
}

// UpdateFolder updates folder.
func (a *MailAdapter) UpdateFolder(ctx context.Context, id int64, folder string) error {
	_, err := a.db.ExecContext(ctx,
		"UPDATE emails SET folder = $1, updated_at = NOW() WHERE id = $2",
		folder, id)
	return err
}

// UpdateTags updates tags.
func (a *MailAdapter) UpdateTags(ctx context.Context, id int64, tags []string) error {
	_, err := a.db.ExecContext(ctx,
		"UPDATE emails SET tags = $1, updated_at = NOW() WHERE id = $2",
		pq.Array(tags), id)
	return err
}

// UpdateWorkflowStatus updates workflow status.
func (a *MailAdapter) UpdateWorkflowStatus(ctx context.Context, id int64, status string, snoozeUntil *time.Time) error {
	_, err := a.db.ExecContext(ctx,
		"UPDATE emails SET workflow_status = $1, snooze_until = $2, updated_at = NOW() WHERE id = $3",
		status, snoozeUntil, id)
	return err
}

// UpdateHasAttachment updates has_attachment flag.
func (a *MailAdapter) UpdateHasAttachment(ctx context.Context, id int64, hasAttachment bool) error {
	_, err := a.db.ExecContext(ctx,
		"UPDATE emails SET has_attachment = $1, updated_at = NOW() WHERE id = $2",
		hasAttachment, id)
	return err
}

// AddLabel adds a label.
func (a *MailAdapter) AddLabel(ctx context.Context, id int64, label string) error {
	_, err := a.db.ExecContext(ctx,
		"UPDATE emails SET labels = array_append(labels, $1), updated_at = NOW() WHERE id = $2",
		label, id)
	return err
}

// RemoveLabel removes a label.
func (a *MailAdapter) RemoveLabel(ctx context.Context, id int64, label string) error {
	_, err := a.db.ExecContext(ctx,
		"UPDATE emails SET labels = array_remove(labels, $1), updated_at = NOW() WHERE id = $2",
		label, id)
	return err
}

// =============================================================================
// AI Results
// =============================================================================

// UpdateAIResult updates AI result.
func (a *MailAdapter) UpdateAIResult(ctx context.Context, id int64, result *out.MailAIResult) error {
	_, err := a.db.ExecContext(ctx, `
		UPDATE emails SET
			ai_status = COALESCE(NULLIF($1, '')::ai_status, ai_status), ai_category = $2, ai_priority = $3,
			ai_summary = $4, ai_sentiment = $5, ai_action_item = $6,
			tags = array_cat(tags, $7),
			updated_at = NOW()
		WHERE id = $8`,
		result.Status, result.Category, nullFloat64(result.Priority),
		result.Summary, result.Sentiment, result.ActionItem,
		pq.Array(result.Tags), id)
	return err
}

// =============================================================================
// Batch Operations
// =============================================================================

// BatchUpdateReadStatus batch updates read status.
func (a *MailAdapter) BatchUpdateReadStatus(ctx context.Context, ids []int64, isRead bool) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := a.db.ExecContext(ctx,
		"UPDATE emails SET is_read = $1, updated_at = NOW() WHERE id = ANY($2)",
		isRead, pq.Array(ids))
	return err
}

// BatchUpdateFolder batch updates folder.
func (a *MailAdapter) BatchUpdateFolder(ctx context.Context, ids []int64, folder string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := a.db.ExecContext(ctx,
		"UPDATE emails SET folder = $1, updated_at = NOW() WHERE id = ANY($2)",
		folder, pq.Array(ids))
	return err
}

// BatchUpdateTags batch updates tags.
func (a *MailAdapter) BatchUpdateTags(ctx context.Context, ids []int64, addTags, removeTags []string) error {
	if len(ids) == 0 {
		return nil
	}
	query := `
		UPDATE emails SET
			tags = (
				SELECT array_agg(DISTINCT t)
				FROM unnest(array_cat(tags, $1)) t
				WHERE t != ALL($2)
			),
			updated_at = NOW()
		WHERE id = ANY($3)`
	_, err := a.db.ExecContext(ctx, query, pq.Array(addTags), pq.Array(removeTags), pq.Array(ids))
	return err
}

// BatchDelete batch deletes emails.
func (a *MailAdapter) BatchDelete(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := a.db.ExecContext(ctx, "DELETE FROM emails WHERE id = ANY($1)", pq.Array(ids))
	return err
}

// BatchUpdateWorkflowStatus batch updates workflow status (snooze 등).
func (a *MailAdapter) BatchUpdateWorkflowStatus(ctx context.Context, ids []int64, status string, snoozeUntil *time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := a.db.ExecContext(ctx,
		"UPDATE emails SET workflow_status = $1, snooze_until = $2, updated_at = NOW() WHERE id = ANY($3)",
		status, snoozeUntil, pq.Array(ids))
	return err
}

// =============================================================================
// Statistics
// =============================================================================

// GetStats gets mail statistics using a single optimized query.
// Combines 3 separate queries into 1 for ~66% performance improvement.
func (a *MailAdapter) GetStats(ctx context.Context, userID uuid.UUID) (*out.MailStats, error) {
	stats := &out.MailStats{
		ByFolder:   make(map[string]int, 10),
		ByCategory: make(map[string]int, 12),
		ByPriority: make(map[string]int, 5),
	}

	// Single query with multiple aggregations using UNION ALL
	query := `
		WITH base AS (
			SELECT folder, ai_category, is_read, tags, workflow_status
			FROM emails WHERE user_id = $1
		)
		SELECT 'summary' as type, '' as key,
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE is_read = false) as unread,
			COUNT(*) FILTER (WHERE 'starred' = ANY(tags)) as starred,
			COUNT(*) FILTER (WHERE 'action_required' = ANY(tags)) as action,
			COUNT(*) FILTER (WHERE workflow_status = 'snoozed') as snoozed
		FROM base
		UNION ALL
		SELECT 'folder' as type, folder as key, COUNT(*) as total, 0, 0, 0, 0
		FROM base GROUP BY folder
		UNION ALL
		SELECT 'category' as type, ai_category as key, COUNT(*) as total, 0, 0, 0, 0
		FROM base WHERE ai_category IS NOT NULL GROUP BY ai_category`

	rows, err := a.db.QueryxContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var rowType, key string
		var total, unread, starred, action, snoozed int
		if err := rows.Scan(&rowType, &key, &total, &unread, &starred, &action, &snoozed); err != nil {
			return nil, err
		}

		switch rowType {
		case "summary":
			stats.TotalCount = total
			stats.UnreadCount = unread
			stats.StarredCount = starred
			stats.ActionCount = action
			stats.SnoozedCount = snoozed
		case "folder":
			stats.ByFolder[key] = total
		case "category":
			stats.ByCategory[key] = total
		}
	}

	return stats, nil
}

// CountUnread counts unread emails.
func (a *MailAdapter) CountUnread(ctx context.Context, userID uuid.UUID, connectionID *int64) (int, error) {
	var count int
	query := "SELECT COUNT(*) FROM emails WHERE user_id = $1 AND is_read = false"
	args := []interface{}{userID}

	if connectionID != nil {
		query += " AND connection_id = $2"
		args = append(args, *connectionID)
	}

	if err := a.db.QueryRowxContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// GetCategoryStats returns email counts per category.
func (a *MailAdapter) GetCategoryStats(ctx context.Context, userID uuid.UUID, connectionID *int64) (map[string]*out.CategoryStatItem, error) {
	query := `
		SELECT
			COALESCE(ai_category, 'other') as category,
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE is_read = false) as unread
		FROM emails
		WHERE user_id = $1`
	args := []interface{}{userID}

	if connectionID != nil {
		query += " AND connection_id = $2"
		args = append(args, *connectionID)
	}

	query += " GROUP BY ai_category"

	rows, err := a.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*out.CategoryStatItem)
	for rows.Next() {
		var category string
		var total, unread int
		if err := rows.Scan(&category, &total, &unread); err != nil {
			return nil, err
		}
		result[category] = &out.CategoryStatItem{
			Total:  total,
			Unread: unread,
		}
	}

	return result, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

func (a *MailAdapter) buildWhereClause(userID uuid.UUID, req *out.MailListQuery) (string, []interface{}) {
	conditions := []string{"e.user_id = $1"}
	args := []interface{}{userID}
	argIdx := 2

	if req.Folder != "" {
		conditions = append(conditions, fmt.Sprintf("e.folder = $%d", argIdx))
		args = append(args, req.Folder)
		argIdx++
	}

	if req.Category != "" {
		conditions = append(conditions, fmt.Sprintf("e.ai_category = $%d", argIdx))
		args = append(args, req.Category)
		argIdx++
	}

	if len(req.Labels) > 0 {
		conditions = append(conditions, fmt.Sprintf("e.labels && $%d", argIdx))
		args = append(args, pq.Array(req.Labels))
		argIdx++
	}

	if len(req.Tags) > 0 {
		conditions = append(conditions, fmt.Sprintf("e.tags && $%d", argIdx))
		args = append(args, pq.Array(req.Tags))
		argIdx++
	}

	if req.IsRead != nil {
		conditions = append(conditions, fmt.Sprintf("e.is_read = $%d", argIdx))
		args = append(args, *req.IsRead)
		argIdx++
	}

	// is_starred는 DB에 없음 - Gmail/Outlook API로 직접 조회
	// if req.IsStarred != nil { ... }

	if req.HasAttachment != nil {
		conditions = append(conditions, fmt.Sprintf("e.has_attachment = $%d", argIdx))
		args = append(args, *req.HasAttachment)
		argIdx++
	}

	if req.Priority != nil {
		conditions = append(conditions, fmt.Sprintf("e.ai_priority >= $%d", argIdx))
		args = append(args, *req.Priority)
		argIdx++
	}

	if req.ContactID != nil {
		conditions = append(conditions, fmt.Sprintf("e.contact_id = $%d", argIdx))
		args = append(args, *req.ContactID)
		argIdx++
	}

	// Connection ID filter
	if req.ConnectionID != nil {
		conditions = append(conditions, fmt.Sprintf("e.connection_id = $%d", argIdx))
		args = append(args, *req.ConnectionID)
		argIdx++
	}

	// Folder ID filter (new folder system)
	if req.FolderID != nil {
		conditions = append(conditions, fmt.Sprintf("e.folder_id = $%d", argIdx))
		args = append(args, *req.FolderID)
		argIdx++
	}

	// SubCategory filter
	if req.SubCategory != "" {
		conditions = append(conditions, fmt.Sprintf("e.ai_sub_category = $%d", argIdx))
		args = append(args, req.SubCategory)
		argIdx++
	}

	// Workflow status filter
	if req.WorkflowStatus != "" {
		conditions = append(conditions, fmt.Sprintf("e.workflow_status = $%d", argIdx))
		args = append(args, req.WorkflowStatus)
		argIdx++
	}

	// From email filter
	if req.FromEmail != "" {
		conditions = append(conditions, fmt.Sprintf("e.from_email ILIKE $%d", argIdx))
		args = append(args, "%"+req.FromEmail+"%")
		argIdx++
	}

	// From domain filter
	if req.FromDomain != "" {
		conditions = append(conditions, fmt.Sprintf("e.from_email LIKE $%d", argIdx))
		args = append(args, "%@"+req.FromDomain)
		argIdx++
	}

	// Label IDs filter
	if len(req.LabelIDs) > 0 {
		conditions = append(conditions, fmt.Sprintf("EXISTS (SELECT 1 FROM email_labels el WHERE el.email_id = e.id AND el.label_id = ANY($%d))", argIdx))
		args = append(args, pq.Array(req.LabelIDs))
		argIdx++
	}

	// === Inbox/Category View Filters ===

	// ViewType: predefined view filter
	// "inbox" = personal mail only (primary, work, personal) + folder = inbox
	if req.ViewType == "inbox" {
		// Inbox view: only personal categories in inbox folder
		conditions = append(conditions, "e.folder = 'inbox'")
		conditions = append(conditions, fmt.Sprintf("e.ai_category = ANY($%d)", argIdx))
		args = append(args, pq.Array([]string{"primary", "work", "personal"}))
		argIdx++
		// Exclude spam even if somehow classified as personal
		conditions = append(conditions, "e.ai_category != 'spam'")
	}

	// Categories: multiple category filter (OR logic)
	// Only apply if ViewType is not "inbox" (inbox has its own category filter)
	if len(req.Categories) > 0 && req.ViewType != "inbox" {
		conditions = append(conditions, fmt.Sprintf("e.ai_category = ANY($%d)", argIdx))
		args = append(args, pq.Array(req.Categories))
		argIdx++
	}

	// SubCategories: multiple sub-category filter (OR logic)
	if len(req.SubCategories) > 0 {
		conditions = append(conditions, fmt.Sprintf("e.ai_sub_category = ANY($%d)", argIdx))
		args = append(args, pq.Array(req.SubCategories))
		argIdx++
	}

	// ExcludeCategories: exclude specific categories
	if len(req.ExcludeCategories) > 0 {
		conditions = append(conditions, fmt.Sprintf("(e.ai_category IS NULL OR e.ai_category != ALL($%d))", argIdx))
		args = append(args, pq.Array(req.ExcludeCategories))
		argIdx++
	}

	_ = argIdx // suppress unused warning
	return strings.Join(conditions, " AND "), args
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// validSubCategories contains all valid email_sub_category enum values in the database.
// Must match exactly with the DB enum definition.
// Updated in migration 029_add_sub_category_enums.sql to include notification, alert, developer
var validSubCategories = map[string]bool{
	"receipt":      true,
	"invoice":      true,
	"shipping":     true,
	"order":        true,
	"travel":       true,
	"calendar":     true,
	"account":      true,
	"security":     true,
	"sns":          true,
	"comment":      true,
	"newsletter":   true,
	"marketing":    true,
	"deal":         true,
	"notification": true, // Added in migration 029
	"alert":        true, // Added in migration 029
	"developer":    true, // Added in migration 029
}

// nullSubCategory returns a valid sub_category or NULL if invalid.
// This prevents enum errors when LLM returns values like "other" that don't exist in DB.
func nullSubCategory(s string) sql.NullString {
	if s == "" || !validSubCategories[s] {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullInt(i int) sql.NullInt32 {
	if i == 0 {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(i), Valid: true}
}

func nullFloat64(f float64) sql.NullFloat64 {
	if f == 0 {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: f, Valid: true}
}

// =============================================================================
// Profile Analysis
// =============================================================================

// GetSentEmails returns sent emails for profile analysis.
func (a *MailAdapter) GetSentEmails(ctx context.Context, userID uuid.UUID, connectionID int64, limit int) ([]*out.MailEntity, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, external_id, thread_id, connection_id, user_id,
			provider, account_email, message_id, in_reply_to, "references",
			from_email, from_name, to_emails, cc_emails, bcc_emails,
			subject, snippet, is_read, is_draft, has_attachment, is_replied, is_forwarded,
			folder, labels, tags, workflow_status, snoozed_until,
			ai_status, ai_category, ai_priority, ai_sentiment, ai_summary, ai_action_item,
			contact_id, email_date, created_at, updated_at
		FROM emails
		WHERE user_id = $1
		  AND connection_id = $2
		  AND folder = 'sent'
		ORDER BY email_date DESC
		LIMIT $3`

	rows, err := a.db.QueryxContext(ctx, query, userID, connectionID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get sent emails: %w", err)
	}
	defer rows.Close()

	var results []*out.MailEntity
	for rows.Next() {
		row := &mailRow{}
		if err := rows.StructScan(row); err != nil {
			continue
		}
		results = append(results, row.toEntity())
	}

	return results, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

var _ out.EmailRepository = (*MailAdapter)(nil)
