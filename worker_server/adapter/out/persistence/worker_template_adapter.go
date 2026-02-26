package persistence

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/goccy/go-json"

	"worker_server/core/port/out"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// TemplateAdapter implements out.TemplateRepository using PostgreSQL.
type TemplateAdapter struct {
	db *sqlx.DB
}

// NewTemplateAdapter creates a new TemplateAdapter.
func NewTemplateAdapter(db *sqlx.DB) *TemplateAdapter {
	return &TemplateAdapter{db: db}
}

// templateRow represents the database row for email templates.
type templateRow struct {
	ID         int64          `db:"id"`
	UserID     uuid.UUID      `db:"user_id"`
	Name       string         `db:"name"`
	Category   string         `db:"category"`
	Subject    sql.NullString `db:"subject"`
	Body       string         `db:"body"`
	HTMLBody   sql.NullString `db:"html_body"`
	Variables  []byte         `db:"variables"`
	Tags       pq.StringArray `db:"tags"`
	IsDefault  bool           `db:"is_default"`
	IsArchived bool           `db:"is_archived"`
	UsageCount int            `db:"usage_count"`
	LastUsedAt sql.NullTime   `db:"last_used_at"`
	CreatedAt  sql.NullTime   `db:"created_at"`
	UpdatedAt  sql.NullTime   `db:"updated_at"`
}

func (r *templateRow) toEntity() *out.TemplateEntity {
	entity := &out.TemplateEntity{
		ID:         r.ID,
		UserID:     r.UserID,
		Name:       r.Name,
		Category:   r.Category,
		Body:       r.Body,
		Tags:       r.Tags,
		IsDefault:  r.IsDefault,
		IsArchived: r.IsArchived,
		UsageCount: r.UsageCount,
	}

	if r.Subject.Valid {
		entity.Subject = r.Subject.String
	}
	if r.HTMLBody.Valid {
		entity.HTMLBody = r.HTMLBody.String
	}
	if r.LastUsedAt.Valid {
		entity.LastUsedAt = &r.LastUsedAt.Time
	}
	if r.CreatedAt.Valid {
		entity.CreatedAt = r.CreatedAt.Time
	}
	if r.UpdatedAt.Valid {
		entity.UpdatedAt = r.UpdatedAt.Time
	}

	// Parse variables JSON
	if len(r.Variables) > 0 {
		var variables []out.TemplateVariableEntity
		if err := json.Unmarshal(r.Variables, &variables); err == nil {
			entity.Variables = variables
		}
	}

	return entity
}

// Create creates a new template.
func (a *TemplateAdapter) Create(ctx context.Context, template *out.TemplateEntity) error {
	variablesJSON, err := json.Marshal(template.Variables)
	if err != nil {
		variablesJSON = []byte("[]")
	}

	query := `
		INSERT INTO email_templates (
			user_id, name, category, subject, body, html_body,
			variables, tags, is_default, is_archived
		) VALUES (
			$1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''),
			$7, $8, $9, $10
		)
		RETURNING id, created_at, updated_at
	`

	return a.db.QueryRowxContext(ctx, query,
		template.UserID,
		template.Name,
		template.Category,
		template.Subject,
		template.Body,
		template.HTMLBody,
		variablesJSON,
		pq.Array(template.Tags),
		template.IsDefault,
		template.IsArchived,
	).Scan(&template.ID, &template.CreatedAt, &template.UpdatedAt)
}

// Update updates a template.
func (a *TemplateAdapter) Update(ctx context.Context, template *out.TemplateEntity) error {
	variablesJSON, err := json.Marshal(template.Variables)
	if err != nil {
		variablesJSON = []byte("[]")
	}

	query := `
		UPDATE email_templates SET
			name = $1,
			category = $2,
			subject = NULLIF($3, ''),
			body = $4,
			html_body = NULLIF($5, ''),
			variables = $6,
			tags = $7,
			is_default = $8,
			updated_at = NOW()
		WHERE id = $9 AND user_id = $10
	`

	result, err := a.db.ExecContext(ctx, query,
		template.Name,
		template.Category,
		template.Subject,
		template.Body,
		template.HTMLBody,
		variablesJSON,
		pq.Array(template.Tags),
		template.IsDefault,
		template.ID,
		template.UserID,
	)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("template not found")
	}
	return nil
}

// Delete deletes a template.
func (a *TemplateAdapter) Delete(ctx context.Context, userID uuid.UUID, id int64) error {
	query := `DELETE FROM email_templates WHERE id = $1 AND user_id = $2`

	result, err := a.db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("template not found")
	}
	return nil
}

// GetByID retrieves a template by ID.
func (a *TemplateAdapter) GetByID(ctx context.Context, userID uuid.UUID, id int64) (*out.TemplateEntity, error) {
	query := `SELECT * FROM email_templates WHERE id = $1 AND user_id = $2`

	var row templateRow
	err := a.db.QueryRowxContext(ctx, query, id, userID).StructScan(&row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("template not found")
		}
		return nil, err
	}

	return row.toEntity(), nil
}

// List lists templates with filters.
func (a *TemplateAdapter) List(ctx context.Context, userID uuid.UUID, query *out.TemplateListQuery) ([]*out.TemplateEntity, int, error) {
	if query == nil {
		query = &out.TemplateListQuery{}
	}
	if query.Limit <= 0 || query.Limit > 100 {
		query.Limit = 50
	}

	baseQuery := `FROM email_templates WHERE user_id = $1`
	args := []interface{}{userID}
	argIdx := 2

	// Filter: is_archived (default false)
	if query.IsArchived != nil {
		baseQuery += fmt.Sprintf(` AND is_archived = $%d`, argIdx)
		args = append(args, *query.IsArchived)
		argIdx++
	} else {
		baseQuery += ` AND is_archived = false`
	}

	// Filter: category
	if query.Category != nil && *query.Category != "" {
		baseQuery += fmt.Sprintf(` AND category = $%d`, argIdx)
		args = append(args, *query.Category)
		argIdx++
	}

	// Filter: is_default
	if query.IsDefault != nil {
		baseQuery += fmt.Sprintf(` AND is_default = $%d`, argIdx)
		args = append(args, *query.IsDefault)
		argIdx++
	}

	// Filter: tags (ANY match)
	if len(query.Tags) > 0 {
		baseQuery += fmt.Sprintf(` AND tags && $%d`, argIdx)
		args = append(args, pq.Array(query.Tags))
		argIdx++
	}

	// Filter: search (name or body)
	if query.Search != nil && *query.Search != "" {
		baseQuery += fmt.Sprintf(` AND (name ILIKE $%d OR body ILIKE $%d)`, argIdx, argIdx)
		args = append(args, "%"+*query.Search+"%")
		argIdx++
	}

	// Count total
	var total int
	countQuery := `SELECT COUNT(*) ` + baseQuery
	if err := a.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Order by validation
	validOrderBy := map[string]bool{
		"name": true, "category": true, "usage_count": true,
		"last_used_at": true, "created_at": true, "updated_at": true,
	}
	if !validOrderBy[query.OrderBy] {
		query.OrderBy = "updated_at"
	}
	if query.Order != "asc" && query.Order != "desc" {
		query.Order = "desc"
	}

	selectQuery := fmt.Sprintf(`SELECT * %s ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		baseQuery, query.OrderBy, query.Order, argIdx, argIdx+1)
	args = append(args, query.Limit, query.Offset)

	rows, err := a.db.QueryxContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var templates []*out.TemplateEntity
	for rows.Next() {
		var row templateRow
		if err := rows.StructScan(&row); err != nil {
			return nil, 0, err
		}
		templates = append(templates, row.toEntity())
	}

	return templates, total, nil
}

// GetDefault retrieves the default template for a category.
func (a *TemplateAdapter) GetDefault(ctx context.Context, userID uuid.UUID, category string) (*out.TemplateEntity, error) {
	query := `SELECT * FROM email_templates WHERE user_id = $1 AND category = $2 AND is_default = true AND is_archived = false`

	var row templateRow
	err := a.db.QueryRowxContext(ctx, query, userID, category).StructScan(&row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return row.toEntity(), nil
}

// GetByCategory retrieves all templates for a category.
func (a *TemplateAdapter) GetByCategory(ctx context.Context, userID uuid.UUID, category string) ([]*out.TemplateEntity, error) {
	query := `SELECT * FROM email_templates WHERE user_id = $1 AND category = $2 AND is_archived = false ORDER BY is_default DESC, name ASC`

	rows, err := a.db.QueryxContext(ctx, query, userID, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []*out.TemplateEntity
	for rows.Next() {
		var row templateRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		templates = append(templates, row.toEntity())
	}

	return templates, nil
}

// IncrementUsage increments the usage count and updates last_used_at.
func (a *TemplateAdapter) IncrementUsage(ctx context.Context, id int64) error {
	query := `
		UPDATE email_templates
		SET usage_count = usage_count + 1, last_used_at = NOW()
		WHERE id = $1
	`
	_, err := a.db.ExecContext(ctx, query, id)
	return err
}

// SetDefault sets a template as the default for its category.
func (a *TemplateAdapter) SetDefault(ctx context.Context, userID uuid.UUID, id int64) error {
	// First, get the template to find its category
	var category string
	err := a.db.QueryRowxContext(ctx,
		`SELECT category FROM email_templates WHERE id = $1 AND user_id = $2`,
		id, userID,
	).Scan(&category)
	if err != nil {
		return fmt.Errorf("template not found")
	}

	// Clear existing default for this category
	if err := a.ClearDefault(ctx, userID, category); err != nil {
		return err
	}

	// Set new default
	query := `UPDATE email_templates SET is_default = true WHERE id = $1 AND user_id = $2`
	_, err = a.db.ExecContext(ctx, query, id, userID)
	return err
}

// ClearDefault clears the default template for a category.
func (a *TemplateAdapter) ClearDefault(ctx context.Context, userID uuid.UUID, category string) error {
	query := `UPDATE email_templates SET is_default = false WHERE user_id = $1 AND category = $2 AND is_default = true`
	_, err := a.db.ExecContext(ctx, query, userID, category)
	return err
}

// Archive archives a template.
func (a *TemplateAdapter) Archive(ctx context.Context, userID uuid.UUID, id int64) error {
	query := `UPDATE email_templates SET is_archived = true, is_default = false WHERE id = $1 AND user_id = $2`
	result, err := a.db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("template not found")
	}
	return nil
}

// Restore restores an archived template.
func (a *TemplateAdapter) Restore(ctx context.Context, userID uuid.UUID, id int64) error {
	query := `UPDATE email_templates SET is_archived = false WHERE id = $1 AND user_id = $2`
	result, err := a.db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("template not found")
	}
	return nil
}

// DeleteBatch deletes multiple templates.
func (a *TemplateAdapter) DeleteBatch(ctx context.Context, userID uuid.UUID, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	query := `DELETE FROM email_templates WHERE user_id = $1 AND id = ANY($2)`
	_, err := a.db.ExecContext(ctx, query, userID, pq.Array(ids))
	return err
}

// Ensure TemplateAdapter implements out.TemplateRepository
var _ out.TemplateRepository = (*TemplateAdapter)(nil)
