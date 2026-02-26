// Package persistence provides database adapters implementing outbound ports.
package persistence

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"worker_server/core/domain"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// SmartFolderAdapter implements domain.SmartFolderRepository using PostgreSQL.
type SmartFolderAdapter struct {
	db *sqlx.DB
}

// NewSmartFolderAdapter creates a new SmartFolderAdapter.
func NewSmartFolderAdapter(db *sqlx.DB) *SmartFolderAdapter {
	return &SmartFolderAdapter{db: db}
}

// smartFolderRow represents the database row for smart folders.
type smartFolderRow struct {
	ID               int64          `db:"id"`
	UserID           uuid.UUID      `db:"user_id"`
	Name             string         `db:"name"`
	Icon             sql.NullString `db:"icon"`
	Color            sql.NullString `db:"color"`
	Query            []byte         `db:"query"` // JSONB
	SortBy           sql.NullString `db:"sort_by"`
	SortOrder        sql.NullString `db:"sort_order"`
	IsSystem         bool           `db:"is_system"`
	IsVisible        bool           `db:"is_visible"`
	Position         int            `db:"position"`
	TotalCount       int            `db:"total_count"`
	UnreadCount      int            `db:"unread_count"`
	LastCalculatedAt sql.NullTime   `db:"last_calculated_at"`
	CreatedAt        time.Time      `db:"created_at"`
	UpdatedAt        time.Time      `db:"updated_at"`
}

func (r *smartFolderRow) toEntity() (*domain.SmartFolder, error) {
	folder := &domain.SmartFolder{
		ID:        r.ID,
		UserID:    r.UserID,
		Name:      r.Name,
		IsSystem:  r.IsSystem,
		Position:  r.Position,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}

	if r.Icon.Valid {
		folder.Icon = &r.Icon.String
	}
	if r.Color.Valid {
		folder.Color = &r.Color.String
	}

	// Parse JSONB query
	if len(r.Query) > 0 {
		if err := json.Unmarshal(r.Query, &folder.Query); err != nil {
			return nil, fmt.Errorf("failed to parse smart folder query: %w", err)
		}
	}

	return folder, nil
}

// GetByID retrieves a smart folder by its ID.
func (a *SmartFolderAdapter) GetByID(id int64) (*domain.SmartFolder, error) {
	var row smartFolderRow
	query := `SELECT * FROM smart_folders WHERE id = $1`

	if err := a.db.Get(&row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("smart folder not found: %d", id)
		}
		return nil, fmt.Errorf("failed to get smart folder: %w", err)
	}

	return row.toEntity()
}

// GetByUserID retrieves all smart folders for a user.
func (a *SmartFolderAdapter) GetByUserID(userID uuid.UUID) ([]*domain.SmartFolder, error) {
	var rows []smartFolderRow
	query := `SELECT * FROM smart_folders WHERE user_id = $1 ORDER BY position ASC`

	if err := a.db.Select(&rows, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list smart folders: %w", err)
	}

	folders := make([]*domain.SmartFolder, 0, len(rows))
	for _, row := range rows {
		folder, err := row.toEntity()
		if err != nil {
			return nil, err
		}
		folders = append(folders, folder)
	}

	return folders, nil
}

// Create creates a new smart folder.
func (a *SmartFolderAdapter) Create(folder *domain.SmartFolder) error {
	queryJSON, err := json.Marshal(folder.Query)
	if err != nil {
		return fmt.Errorf("failed to marshal smart folder query: %w", err)
	}

	query := `
		INSERT INTO smart_folders (user_id, name, icon, color, query, is_system, position)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	var icon sql.NullString
	if folder.Icon != nil {
		icon = sql.NullString{String: *folder.Icon, Valid: true}
	}

	var color sql.NullString
	if folder.Color != nil {
		color = sql.NullString{String: *folder.Color, Valid: true}
	}

	err = a.db.QueryRow(
		query,
		folder.UserID,
		folder.Name,
		icon,
		color,
		queryJSON,
		folder.IsSystem,
		folder.Position,
	).Scan(&folder.ID, &folder.CreatedAt, &folder.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create smart folder: %w", err)
	}

	return nil
}

// Update updates a smart folder.
func (a *SmartFolderAdapter) Update(folder *domain.SmartFolder) error {
	queryJSON, err := json.Marshal(folder.Query)
	if err != nil {
		return fmt.Errorf("failed to marshal smart folder query: %w", err)
	}

	query := `
		UPDATE smart_folders
		SET name = $2, icon = $3, color = $4, query = $5, position = $6, updated_at = NOW()
		WHERE id = $1`

	var icon sql.NullString
	if folder.Icon != nil {
		icon = sql.NullString{String: *folder.Icon, Valid: true}
	}

	var color sql.NullString
	if folder.Color != nil {
		color = sql.NullString{String: *folder.Color, Valid: true}
	}

	result, err := a.db.Exec(query, folder.ID, folder.Name, icon, color, queryJSON, folder.Position)
	if err != nil {
		return fmt.Errorf("failed to update smart folder: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("smart folder not found: %d", folder.ID)
	}

	return nil
}

// Delete deletes a smart folder.
func (a *SmartFolderAdapter) Delete(id int64) error {
	query := `DELETE FROM smart_folders WHERE id = $1 AND is_system = FALSE`

	result, err := a.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete smart folder: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("smart folder not found or is system folder: %d", id)
	}

	return nil
}

// CountEmails counts emails matching the smart folder query.
func (a *SmartFolderAdapter) CountEmails(folderID int64) (total int, unread int, err error) {
	// Get the smart folder to get its query
	folder, err := a.GetByID(folderID)
	if err != nil {
		return 0, 0, err
	}

	// Build dynamic query based on smart folder criteria
	baseQuery, args := a.buildSmartFolderQuery(folder)

	// Count total
	totalQuery := fmt.Sprintf("SELECT COUNT(*) FROM emails e WHERE %s", baseQuery)
	if err := a.db.Get(&total, totalQuery, args...); err != nil {
		return 0, 0, fmt.Errorf("failed to count total emails: %w", err)
	}

	// Count unread
	unreadQuery := fmt.Sprintf("SELECT COUNT(*) FROM emails e WHERE %s AND e.is_read = FALSE", baseQuery)
	if err := a.db.Get(&unread, unreadQuery, args...); err != nil {
		return 0, 0, fmt.Errorf("failed to count unread emails: %w", err)
	}

	return total, unread, nil
}

// GetEmailIDs retrieves email IDs matching the smart folder query.
func (a *SmartFolderAdapter) GetEmailIDs(folderID int64, limit, offset int) ([]int64, error) {
	folder, err := a.GetByID(folderID)
	if err != nil {
		return nil, err
	}

	baseQuery, args := a.buildSmartFolderQuery(folder)

	// Add pagination
	args = append(args, limit, offset)
	query := fmt.Sprintf(`
		SELECT e.id FROM emails e
		WHERE %s
		ORDER BY e.email_date DESC
		LIMIT $%d OFFSET $%d`, baseQuery, len(args)-1, len(args))

	var emailIDs []int64
	if err := a.db.Select(&emailIDs, query, args...); err != nil {
		return nil, fmt.Errorf("failed to get smart folder emails: %w", err)
	}

	return emailIDs, nil
}

// buildSmartFolderQuery builds a SQL WHERE clause from a SmartFolderQuery.
func (a *SmartFolderAdapter) buildSmartFolderQuery(folder *domain.SmartFolder) (string, []interface{}) {
	var conditions []string
	var args []interface{}
	argIndex := 1

	// User ID is required
	conditions = append(conditions, fmt.Sprintf("e.user_id = $%d", argIndex))
	args = append(args, folder.UserID)
	argIndex++

	// Deleted filter
	conditions = append(conditions, "e.deleted_at IS NULL")

	q := folder.Query

	// Categories filter
	if len(q.Categories) > 0 {
		placeholders := make([]string, len(q.Categories))
		for i, cat := range q.Categories {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, string(cat))
			argIndex++
		}
		conditions = append(conditions, fmt.Sprintf("e.ai_category IN (%s)", joinStrings(placeholders, ", ")))
	}

	// SubCategories filter
	if len(q.SubCategories) > 0 {
		placeholders := make([]string, len(q.SubCategories))
		for i, subCat := range q.SubCategories {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, string(subCat))
			argIndex++
		}
		conditions = append(conditions, fmt.Sprintf("e.ai_sub_category IN (%s)", joinStrings(placeholders, ", ")))
	}

	// Priorities filter
	if len(q.Priorities) > 0 {
		placeholders := make([]string, len(q.Priorities))
		for i, pri := range q.Priorities {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, int(pri))
			argIndex++
		}
		conditions = append(conditions, fmt.Sprintf("e.ai_priority IN (%s)", joinStrings(placeholders, ", ")))
	}

	// IsRead filter
	if q.IsRead != nil {
		conditions = append(conditions, fmt.Sprintf("e.is_read = $%d", argIndex))
		args = append(args, *q.IsRead)
		argIndex++
	}

	// IsStarred filter
	if q.IsStarred != nil {
		conditions = append(conditions, fmt.Sprintf("e.is_starred = $%d", argIndex))
		args = append(args, *q.IsStarred)
		argIndex++
	}

	// Date range filter
	if q.DateRange != "" {
		switch q.DateRange {
		case "7_days":
			conditions = append(conditions, "e.email_date >= NOW() - INTERVAL '7 days'")
		case "30_days":
			conditions = append(conditions, "e.email_date >= NOW() - INTERVAL '30 days'")
		case "90_days":
			conditions = append(conditions, "e.email_date >= NOW() - INTERVAL '90 days'")
		}
	}

	// From domains filter
	if len(q.FromDomains) > 0 {
		domainConditions := make([]string, len(q.FromDomains))
		for i, domain := range q.FromDomains {
			domainConditions[i] = fmt.Sprintf("e.from_email LIKE $%d", argIndex)
			args = append(args, "%@"+domain)
			argIndex++
		}
		conditions = append(conditions, "("+joinStrings(domainConditions, " OR ")+")")
	}

	// From emails filter
	if len(q.FromEmails) > 0 {
		placeholders := make([]string, len(q.FromEmails))
		for i, email := range q.FromEmails {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, email)
			argIndex++
		}
		conditions = append(conditions, fmt.Sprintf("e.from_email IN (%s)", joinStrings(placeholders, ", ")))
	}

	return joinStrings(conditions, " AND "), args
}

// joinStrings joins strings with a separator.
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
