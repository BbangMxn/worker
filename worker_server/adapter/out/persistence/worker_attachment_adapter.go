// Package persistence provides database adapters implementing outbound ports.
package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"worker_server/core/port/out"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// =============================================================================
// Attachment Adapter (PostgreSQL)
// =============================================================================

// AttachmentAdapter implements out.AttachmentRepository using PostgreSQL.
type AttachmentAdapter struct {
	db *sqlx.DB
}

// NewAttachmentAdapter creates a new AttachmentAdapter.
func NewAttachmentAdapter(db *sqlx.DB) *AttachmentAdapter {
	return &AttachmentAdapter{db: db}
}

// =============================================================================
// Database Row Mapping
// =============================================================================

type attachmentRow struct {
	ID         int64          `db:"id"`
	EmailID    int64          `db:"email_id"`
	ExternalID string         `db:"external_id"`
	Filename   string         `db:"filename"`
	MimeType   string         `db:"mime_type"`
	Size       int64          `db:"size"`
	ContentID  sql.NullString `db:"content_id"`
	IsInline   bool           `db:"is_inline"`
	CreatedAt  time.Time      `db:"created_at"`
}

func (r *attachmentRow) toEntity() *out.EmailAttachmentEntity {
	var contentID *string
	if r.ContentID.Valid {
		contentID = &r.ContentID.String
	}

	return &out.EmailAttachmentEntity{
		ID:         r.ID,
		EmailID:    r.EmailID,
		ExternalID: r.ExternalID,
		Filename:   r.Filename,
		MimeType:   r.MimeType,
		Size:       r.Size,
		ContentID:  contentID,
		IsInline:   r.IsInline,
		CreatedAt:  r.CreatedAt,
	}
}

// =============================================================================
// CRUD Operations
// =============================================================================

// Create creates a new attachment record.
func (a *AttachmentAdapter) Create(ctx context.Context, attachment *out.EmailAttachmentEntity) error {
	query := `
		INSERT INTO email_attachments (email_id, external_id, filename, mime_type, size, content_id, is_inline)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`

	return a.db.QueryRowContext(ctx, query,
		attachment.EmailID,
		attachment.ExternalID,
		attachment.Filename,
		attachment.MimeType,
		attachment.Size,
		attachment.ContentID,
		attachment.IsInline,
	).Scan(&attachment.ID, &attachment.CreatedAt)
}

// CreateBatch creates multiple attachment records in a single transaction.
func (a *AttachmentAdapter) CreateBatch(ctx context.Context, attachments []*out.EmailAttachmentEntity) error {
	if len(attachments) == 0 {
		return nil
	}

	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO email_attachments (email_id, external_id, filename, mime_type, size, content_id, is_inline)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (email_id, external_id) DO UPDATE SET
			filename = EXCLUDED.filename,
			mime_type = EXCLUDED.mime_type,
			size = EXCLUDED.size,
			content_id = EXCLUDED.content_id,
			is_inline = EXCLUDED.is_inline
		RETURNING id, created_at`

	for _, att := range attachments {
		err := tx.QueryRowContext(ctx, query,
			att.EmailID,
			att.ExternalID,
			att.Filename,
			att.MimeType,
			att.Size,
			att.ContentID,
			att.IsInline,
		).Scan(&att.ID, &att.CreatedAt)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetByID retrieves an attachment by ID.
func (a *AttachmentAdapter) GetByID(ctx context.Context, id int64) (*out.EmailAttachmentEntity, error) {
	var row attachmentRow
	query := `
		SELECT id, email_id, external_id, filename, mime_type, size, content_id, is_inline, created_at
		FROM email_attachments
		WHERE id = $1`

	err := a.db.GetContext(ctx, &row, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return row.toEntity(), nil
}

// GetByEmailID retrieves all attachments for an email.
func (a *AttachmentAdapter) GetByEmailID(ctx context.Context, emailID int64) ([]*out.EmailAttachmentEntity, error) {
	var rows []attachmentRow
	query := `
		SELECT id, email_id, external_id, filename, mime_type, size, content_id, is_inline, created_at
		FROM email_attachments
		WHERE email_id = $1
		ORDER BY id`

	err := a.db.SelectContext(ctx, &rows, query, emailID)
	if err != nil {
		return nil, err
	}

	result := make([]*out.EmailAttachmentEntity, len(rows))
	for i, row := range rows {
		result[i] = row.toEntity()
	}
	return result, nil
}

// GetByExternalID retrieves an attachment by email ID and external ID.
func (a *AttachmentAdapter) GetByExternalID(ctx context.Context, emailID int64, externalID string) (*out.EmailAttachmentEntity, error) {
	var row attachmentRow
	query := `
		SELECT id, email_id, external_id, filename, mime_type, size, content_id, is_inline, created_at
		FROM email_attachments
		WHERE email_id = $1 AND external_id = $2`

	err := a.db.GetContext(ctx, &row, query, emailID, externalID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return row.toEntity(), nil
}

// GetByContentID retrieves an attachment by email ID and Content-ID (for inline attachments).
func (a *AttachmentAdapter) GetByContentID(ctx context.Context, emailID int64, contentID string) (*out.EmailAttachmentEntity, error) {
	var row attachmentRow
	query := `
		SELECT id, email_id, external_id, filename, mime_type, size, content_id, is_inline, created_at
		FROM email_attachments
		WHERE email_id = $1 AND content_id = $2`

	err := a.db.GetContext(ctx, &row, query, emailID, contentID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return row.toEntity(), nil
}

// Delete deletes an attachment by ID.
func (a *AttachmentAdapter) Delete(ctx context.Context, id int64) error {
	_, err := a.db.ExecContext(ctx, "DELETE FROM email_attachments WHERE id = $1", id)
	return err
}

// DeleteByEmailID deletes all attachments for an email.
func (a *AttachmentAdapter) DeleteByEmailID(ctx context.Context, emailID int64) error {
	_, err := a.db.ExecContext(ctx, "DELETE FROM email_attachments WHERE email_id = $1", emailID)
	return err
}

// =============================================================================
// Query Operations
// =============================================================================

// ListByEmails retrieves attachments for multiple emails (batch query).
func (a *AttachmentAdapter) ListByEmails(ctx context.Context, emailIDs []int64) (map[int64][]*out.EmailAttachmentEntity, error) {
	if len(emailIDs) == 0 {
		return make(map[int64][]*out.EmailAttachmentEntity), nil
	}

	query := `
		SELECT id, email_id, external_id, filename, mime_type, size, content_id, is_inline, created_at
		FROM email_attachments
		WHERE email_id = ANY($1)
		ORDER BY email_id, id`

	var rows []attachmentRow
	err := a.db.SelectContext(ctx, &rows, query, emailIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[int64][]*out.EmailAttachmentEntity)
	for _, row := range rows {
		entity := row.toEntity()
		result[row.EmailID] = append(result[row.EmailID], entity)
	}

	return result, nil
}

// CountByEmailID counts attachments for an email.
func (a *AttachmentAdapter) CountByEmailID(ctx context.Context, emailID int64) (int, error) {
	var count int
	err := a.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM email_attachments WHERE email_id = $1", emailID)
	return count, err
}

// GetInlineByEmailID retrieves only inline attachments for an email.
func (a *AttachmentAdapter) GetInlineByEmailID(ctx context.Context, emailID int64) ([]*out.EmailAttachmentEntity, error) {
	var rows []attachmentRow
	query := `
		SELECT id, email_id, external_id, filename, mime_type, size, content_id, is_inline, created_at
		FROM email_attachments
		WHERE email_id = $1 AND is_inline = true AND content_id IS NOT NULL
		ORDER BY created_at`

	if err := a.db.SelectContext(ctx, &rows, query, emailID); err != nil {
		return nil, err
	}

	result := make([]*out.EmailAttachmentEntity, len(rows))
	for i, row := range rows {
		result[i] = row.toEntity()
	}
	return result, nil
}

// GetPendingByEmailID retrieves attachments with pending external IDs.
func (a *AttachmentAdapter) GetPendingByEmailID(ctx context.Context, emailID int64) ([]*out.EmailAttachmentEntity, error) {
	var rows []attachmentRow
	query := `
		SELECT id, email_id, external_id, filename, mime_type, size, content_id, is_inline, created_at
		FROM email_attachments
		WHERE email_id = $1 AND external_id LIKE 'pending_%'
		ORDER BY id`

	if err := a.db.SelectContext(ctx, &rows, query, emailID); err != nil {
		return nil, err
	}

	result := make([]*out.EmailAttachmentEntity, len(rows))
	for i, row := range rows {
		result[i] = row.toEntity()
	}
	return result, nil
}

// UpdateExternalIDs updates pending external IDs to actual attachment IDs.
func (a *AttachmentAdapter) UpdateExternalIDs(ctx context.Context, emailID int64, updates []out.AttachmentExternalIDUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 파일명으로 매칭하여 업데이트 (pending ID는 순서가 다를 수 있음)
	query := `
		UPDATE email_attachments
		SET external_id = $1
		WHERE email_id = $2 AND filename = $3 AND external_id LIKE 'pending_%'`

	for _, update := range updates {
		_, err := tx.ExecContext(ctx, query, update.NewExternalID, emailID, update.Filename)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// =============================================================================
// User-Scoped Queries (모아보기)
// =============================================================================

// attachmentWithEmailRow is the database row for attachment with email JOIN.
type attachmentWithEmailRow struct {
	ID              int64          `db:"id"`
	EmailID         int64          `db:"email_id"`
	ExternalID      string         `db:"external_id"`
	Filename        string         `db:"filename"`
	MimeType        string         `db:"mime_type"`
	Size            int64          `db:"size"`
	ContentID       sql.NullString `db:"content_id"`
	IsInline        bool           `db:"is_inline"`
	CreatedAt       time.Time      `db:"created_at"`
	EmailSubject    string         `db:"email_subject"`
	EmailFrom       string         `db:"email_from"`
	EmailDate       time.Time      `db:"email_date"`
	ConnectionID    int64          `db:"connection_id"`
	EmailProvider   string         `db:"email_provider"`
	EmailExternalID string         `db:"email_external_id"`
}

func (r *attachmentWithEmailRow) toModel() *out.AttachmentWithEmail {
	return &out.AttachmentWithEmail{
		ID:              r.ID,
		EmailID:         r.EmailID,
		ExternalID:      r.ExternalID,
		Filename:        r.Filename,
		MimeType:        r.MimeType,
		Size:            r.Size,
		IsInline:        r.IsInline,
		CreatedAt:       r.CreatedAt,
		EmailSubject:    r.EmailSubject,
		EmailFrom:       r.EmailFrom,
		EmailDate:       r.EmailDate,
		ConnectionID:    r.ConnectionID,
		EmailProvider:   r.EmailProvider,
		EmailExternalID: r.EmailExternalID,
	}
}

// ListByUser retrieves attachments for a user with filters (모아보기).
func (a *AttachmentAdapter) ListByUser(ctx context.Context, userID uuid.UUID, query *out.AttachmentListQuery) ([]*out.AttachmentWithEmail, int, error) {
	// Set defaults
	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	if query.SortBy == "" {
		query.SortBy = "created_at"
	}
	if query.SortOrder == "" {
		query.SortOrder = "desc"
	}

	// Validate sort fields
	validSortFields := map[string]string{
		"created_at": "a.created_at",
		"size":       "a.size",
		"filename":   "a.filename",
	}
	sortField, ok := validSortFields[query.SortBy]
	if !ok {
		sortField = "a.created_at"
	}
	sortOrder := "DESC"
	if query.SortOrder == "asc" {
		sortOrder = "ASC"
	}

	// Build WHERE clause
	whereClause := "e.user_id = $1 AND a.is_inline = false"
	args := []interface{}{userID}
	argIdx := 2

	if query.ConnectionID != nil {
		whereClause += " AND e.connection_id = $" + itoa(argIdx)
		args = append(args, *query.ConnectionID)
		argIdx++
	}

	if len(query.MimeTypes) > 0 {
		whereClause += " AND ("
		for i, mt := range query.MimeTypes {
			if i > 0 {
				whereClause += " OR "
			}
			if mt[len(mt)-1] == '*' {
				// Wildcard match (e.g., "image/*")
				whereClause += "a.mime_type LIKE $" + itoa(argIdx)
				args = append(args, mt[:len(mt)-1]+"%")
			} else {
				whereClause += "a.mime_type = $" + itoa(argIdx)
				args = append(args, mt)
			}
			argIdx++
		}
		whereClause += ")"
	}

	if query.MinSize != nil {
		whereClause += " AND a.size >= $" + itoa(argIdx)
		args = append(args, *query.MinSize)
		argIdx++
	}

	if query.MaxSize != nil {
		whereClause += " AND a.size <= $" + itoa(argIdx)
		args = append(args, *query.MaxSize)
		argIdx++
	}

	if query.StartDate != nil {
		whereClause += " AND a.created_at >= $" + itoa(argIdx)
		args = append(args, *query.StartDate)
		argIdx++
	}

	if query.EndDate != nil {
		whereClause += " AND a.created_at <= $" + itoa(argIdx)
		args = append(args, *query.EndDate)
		argIdx++
	}

	// Count query
	countQuery := `
		SELECT COUNT(*)
		FROM email_attachments a
		JOIN emails e ON a.email_id = e.id
		WHERE ` + whereClause

	var total int
	if err := a.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, err
	}

	// Data query
	dataQuery := `
		SELECT
			a.id, a.email_id, a.external_id, a.filename, a.mime_type, a.size,
			a.content_id, a.is_inline, a.created_at,
			e.subject as email_subject, e.from_email as email_from,
			e.email_date as email_date, e.connection_id, e.provider as email_provider,
			e.external_id as email_external_id
		FROM email_attachments a
		JOIN emails e ON a.email_id = e.id
		WHERE ` + whereClause + `
		ORDER BY ` + sortField + ` ` + sortOrder + `
		LIMIT $` + itoa(argIdx) + ` OFFSET $` + itoa(argIdx+1)

	args = append(args, query.Limit, query.Offset)

	var rows []attachmentWithEmailRow
	if err := a.db.SelectContext(ctx, &rows, dataQuery, args...); err != nil {
		return nil, 0, err
	}

	result := make([]*out.AttachmentWithEmail, len(rows))
	for i, row := range rows {
		result[i] = row.toModel()
	}

	return result, total, nil
}

// GetStatsByUser returns attachment statistics for a user.
func (a *AttachmentAdapter) GetStatsByUser(ctx context.Context, userID uuid.UUID) (*out.AttachmentStats, error) {
	// Total count and size
	var totalCount int
	var totalSize int64

	summaryQuery := `
		SELECT COUNT(*), COALESCE(SUM(a.size), 0)
		FROM email_attachments a
		JOIN emails e ON a.email_id = e.id
		WHERE e.user_id = $1 AND a.is_inline = false`

	if err := a.db.QueryRowContext(ctx, summaryQuery, userID).Scan(&totalCount, &totalSize); err != nil {
		return nil, err
	}

	// Group by mime type (simplified categories)
	typeQuery := `
		SELECT
			CASE
				WHEN a.mime_type LIKE 'image/%' THEN 'image'
				WHEN a.mime_type LIKE 'video/%' THEN 'video'
				WHEN a.mime_type LIKE 'audio/%' THEN 'audio'
				WHEN a.mime_type = 'application/pdf' THEN 'pdf'
				WHEN a.mime_type LIKE 'application/vnd.ms-excel%' OR a.mime_type LIKE 'application/vnd.openxmlformats-officedocument.spreadsheet%' THEN 'spreadsheet'
				WHEN a.mime_type LIKE 'application/vnd.ms-word%' OR a.mime_type LIKE 'application/vnd.openxmlformats-officedocument.wordprocessing%' THEN 'document'
				WHEN a.mime_type LIKE 'application/vnd.ms-powerpoint%' OR a.mime_type LIKE 'application/vnd.openxmlformats-officedocument.presentation%' THEN 'presentation'
				WHEN a.mime_type LIKE 'application/zip%' OR a.mime_type LIKE 'application/x-rar%' OR a.mime_type LIKE 'application/x-7z%' OR a.mime_type LIKE 'application/gzip%' THEN 'archive'
				WHEN a.mime_type LIKE 'text/%' THEN 'text'
				ELSE 'other'
			END as category,
			COUNT(*) as count,
			COALESCE(SUM(a.size), 0) as size
		FROM email_attachments a
		JOIN emails e ON a.email_id = e.id
		WHERE e.user_id = $1 AND a.is_inline = false
		GROUP BY category`

	rows, err := a.db.QueryContext(ctx, typeQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	countByType := make(map[string]int)
	sizeByType := make(map[string]int64)

	for rows.Next() {
		var category string
		var count int
		var size int64
		if err := rows.Scan(&category, &count, &size); err != nil {
			return nil, err
		}
		countByType[category] = count
		sizeByType[category] = size
	}

	return &out.AttachmentStats{
		TotalCount:      totalCount,
		TotalSize:       totalSize,
		CountByMimeType: countByType,
		SizeByMimeType:  sizeByType,
	}, nil
}

// SearchByUser searches attachments by filename.
func (a *AttachmentAdapter) SearchByUser(ctx context.Context, userID uuid.UUID, filename string, limit, offset int) ([]*out.AttachmentWithEmail, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	whereClause := "e.user_id = $1 AND a.is_inline = false AND a.filename ILIKE $2"
	searchPattern := "%" + filename + "%"

	// Count
	var total int
	countQuery := `
		SELECT COUNT(*)
		FROM email_attachments a
		JOIN emails e ON a.email_id = e.id
		WHERE ` + whereClause

	if err := a.db.GetContext(ctx, &total, countQuery, userID, searchPattern); err != nil {
		return nil, 0, err
	}

	// Data
	dataQuery := `
		SELECT
			a.id, a.email_id, a.external_id, a.filename, a.mime_type, a.size,
			a.content_id, a.is_inline, a.created_at,
			e.subject as email_subject, e.from_email as email_from,
			e.email_date as email_date, e.connection_id, e.provider as email_provider,
			e.external_id as email_external_id
		FROM email_attachments a
		JOIN emails e ON a.email_id = e.id
		WHERE ` + whereClause + `
		ORDER BY a.created_at DESC
		LIMIT $3 OFFSET $4`

	var rows []attachmentWithEmailRow
	if err := a.db.SelectContext(ctx, &rows, dataQuery, userID, searchPattern, limit, offset); err != nil {
		return nil, 0, err
	}

	result := make([]*out.AttachmentWithEmail, len(rows))
	for i, row := range rows {
		result[i] = row.toModel()
	}

	return result, total, nil
}

// itoa converts int to string for query building.
func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

// Ensure AttachmentAdapter implements out.AttachmentRepository
var _ out.AttachmentRepository = (*AttachmentAdapter)(nil)
