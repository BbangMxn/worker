// Package persistence provides database adapters implementing outbound ports.
package persistence

import (
	"database/sql"
	"fmt"
	"time"

	"worker_server/core/domain"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// LabelAdapter implements domain.LabelRepository using PostgreSQL.
type LabelAdapter struct {
	db *sqlx.DB
}

// NewLabelAdapter creates a new LabelAdapter.
func NewLabelAdapter(db *sqlx.DB) *LabelAdapter {
	return &LabelAdapter{db: db}
}

// labelRow represents the database row for labels.
type labelRow struct {
	ID           int64          `db:"id"`
	UserID       uuid.UUID      `db:"user_id"`
	ConnectionID sql.NullInt64  `db:"connection_id"`
	ProviderID   sql.NullString `db:"provider_id"`
	Name         string         `db:"name"`
	Color        sql.NullString `db:"color"`
	IsSystem     bool           `db:"is_system"`
	IsVisible    bool           `db:"is_visible"`
	EmailCount   int            `db:"email_count"`
	UnreadCount  int            `db:"unread_count"`
	Position     int            `db:"position"`
	CreatedAt    time.Time      `db:"created_at"`
	UpdatedAt    time.Time      `db:"updated_at"`
}

func (r *labelRow) toEntity() *domain.Label {
	label := &domain.Label{
		ID:          r.ID,
		UserID:      r.UserID,
		Name:        r.Name,
		IsSystem:    r.IsSystem,
		IsVisible:   r.IsVisible,
		EmailCount:  r.EmailCount,
		UnreadCount: r.UnreadCount,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}

	if r.ConnectionID.Valid {
		connID := r.ConnectionID.Int64
		label.ConnectionID = &connID
	}
	if r.ProviderID.Valid {
		providerID := r.ProviderID.String
		label.ProviderID = &providerID
	}
	if r.Color.Valid {
		color := r.Color.String
		label.Color = &color
	}

	return label
}

// GetByID retrieves a label by its ID.
func (a *LabelAdapter) GetByID(id int64) (*domain.Label, error) {
	var row labelRow
	query := `SELECT * FROM labels WHERE id = $1`

	if err := a.db.Get(&row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("label not found: %d", id)
		}
		return nil, fmt.Errorf("failed to get label: %w", err)
	}

	return row.toEntity(), nil
}

// GetByProviderID retrieves a label by connection and provider ID.
func (a *LabelAdapter) GetByProviderID(connectionID int64, providerID string) (*domain.Label, error) {
	var row labelRow
	query := `SELECT * FROM labels WHERE connection_id = $1 AND provider_id = $2`

	if err := a.db.Get(&row, query, connectionID, providerID); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("label not found")
		}
		return nil, fmt.Errorf("failed to get label: %w", err)
	}

	return row.toEntity(), nil
}

// ListByUser retrieves all labels for a user.
func (a *LabelAdapter) ListByUser(userID uuid.UUID) ([]*domain.Label, error) {
	var rows []labelRow
	query := `SELECT * FROM labels WHERE user_id = $1 ORDER BY is_system DESC, name ASC`

	if err := a.db.Select(&rows, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	labels := make([]*domain.Label, len(rows))
	for i, row := range rows {
		labels[i] = row.toEntity()
	}

	return labels, nil
}

// ListByConnection retrieves all labels for a specific connection.
func (a *LabelAdapter) ListByConnection(connectionID int64) ([]*domain.Label, error) {
	var rows []labelRow
	query := `SELECT * FROM labels WHERE connection_id = $1 ORDER BY is_system DESC, name ASC`

	if err := a.db.Select(&rows, query, connectionID); err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	labels := make([]*domain.Label, len(rows))
	for i, row := range rows {
		labels[i] = row.toEntity()
	}

	return labels, nil
}

// Create creates a new label.
func (a *LabelAdapter) Create(label *domain.Label) error {
	query := `
		INSERT INTO labels (user_id, connection_id, provider_id, name, color, is_system, is_visible)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	var connID sql.NullInt64
	if label.ConnectionID != nil {
		connID = sql.NullInt64{Int64: *label.ConnectionID, Valid: true}
	}

	var providerID sql.NullString
	if label.ProviderID != nil {
		providerID = sql.NullString{String: *label.ProviderID, Valid: true}
	}

	var color sql.NullString
	if label.Color != nil {
		color = sql.NullString{String: *label.Color, Valid: true}
	}

	err := a.db.QueryRow(
		query,
		label.UserID,
		connID,
		providerID,
		label.Name,
		color,
		label.IsSystem,
		label.IsVisible,
	).Scan(&label.ID, &label.CreatedAt, &label.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create label: %w", err)
	}

	return nil
}

// Update updates a label.
func (a *LabelAdapter) Update(label *domain.Label) error {
	query := `
		UPDATE labels
		SET name = $2, color = $3, is_visible = $4, updated_at = NOW()
		WHERE id = $1`

	var color sql.NullString
	if label.Color != nil {
		color = sql.NullString{String: *label.Color, Valid: true}
	}

	result, err := a.db.Exec(query, label.ID, label.Name, color, label.IsVisible)
	if err != nil {
		return fmt.Errorf("failed to update label: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("label not found: %d", label.ID)
	}

	return nil
}

// Delete deletes a label.
func (a *LabelAdapter) Delete(id int64) error {
	query := `DELETE FROM labels WHERE id = $1 AND is_system = FALSE`

	result, err := a.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete label: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("label not found or is system label: %d", id)
	}

	return nil
}

// AddEmailLabel adds a label to an email.
func (a *LabelAdapter) AddEmailLabel(emailID, labelID int64) error {
	query := `
		INSERT INTO email_labels (email_id, label_id)
		VALUES ($1, $2)
		ON CONFLICT (email_id, label_id) DO NOTHING`

	_, err := a.db.Exec(query, emailID, labelID)
	if err != nil {
		return fmt.Errorf("failed to add label to email: %w", err)
	}

	// Update email count
	updateQuery := `
		UPDATE labels
		SET email_count = (SELECT COUNT(*) FROM email_labels WHERE label_id = $1),
		    updated_at = NOW()
		WHERE id = $1`
	_, _ = a.db.Exec(updateQuery, labelID)

	return nil
}

// RemoveEmailLabel removes a label from an email.
func (a *LabelAdapter) RemoveEmailLabel(emailID, labelID int64) error {
	query := `DELETE FROM email_labels WHERE email_id = $1 AND label_id = $2`

	result, err := a.db.Exec(query, emailID, labelID)
	if err != nil {
		return fmt.Errorf("failed to remove label from email: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("label association not found")
	}

	// Update email count
	updateQuery := `
		UPDATE labels
		SET email_count = (SELECT COUNT(*) FROM email_labels WHERE label_id = $1),
		    updated_at = NOW()
		WHERE id = $1`
	_, _ = a.db.Exec(updateQuery, labelID)

	return nil
}

// GetEmailLabels retrieves all labels for an email.
func (a *LabelAdapter) GetEmailLabels(emailID int64) ([]*domain.Label, error) {
	var rows []labelRow
	query := `
		SELECT l.* FROM labels l
		INNER JOIN email_labels el ON l.id = el.label_id
		WHERE el.email_id = $1
		ORDER BY l.name ASC`

	if err := a.db.Select(&rows, query, emailID); err != nil {
		return nil, fmt.Errorf("failed to get email labels: %w", err)
	}

	labels := make([]*domain.Label, len(rows))
	for i, row := range rows {
		labels[i] = row.toEntity()
	}

	return labels, nil
}

// UpdatePositions updates the display order of labels for a user.
func (a *LabelAdapter) UpdatePositions(userID uuid.UUID, labelIDs []int64) error {
	tx, err := a.db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for i, labelID := range labelIDs {
		query := `UPDATE labels SET position = $1, updated_at = NOW() WHERE id = $2 AND user_id = $3`
		_, err := tx.Exec(query, i, labelID, userID)
		if err != nil {
			return fmt.Errorf("failed to update label position: %w", err)
		}
	}

	return tx.Commit()
}

// GetEmailsByLabel retrieves email IDs that have a specific label.
func (a *LabelAdapter) GetEmailsByLabel(labelID int64, limit, offset int) ([]int64, error) {
	var emailIDs []int64
	query := `
		SELECT email_id FROM email_labels
		WHERE label_id = $1
		ORDER BY email_id DESC
		LIMIT $2 OFFSET $3`

	if err := a.db.Select(&emailIDs, query, labelID, limit, offset); err != nil {
		return nil, fmt.Errorf("failed to get emails by label: %w", err)
	}

	return emailIDs, nil
}

// labelProviderMappingRow represents the database row for label provider mappings.
type labelProviderMappingRow struct {
	ID           int64          `db:"id"`
	LabelID      int64          `db:"label_id"`
	ConnectionID int64          `db:"connection_id"`
	Provider     string         `db:"provider"`
	ExternalID   sql.NullString `db:"external_id"`
	MappingType  string         `db:"mapping_type"`
	CreatedAt    time.Time      `db:"created_at"`
	UpdatedAt    time.Time      `db:"updated_at"`
}

func (r *labelProviderMappingRow) toEntity() *domain.LabelProviderMapping {
	mapping := &domain.LabelProviderMapping{
		ID:           r.ID,
		LabelID:      r.LabelID,
		ConnectionID: r.ConnectionID,
		Provider:     domain.Provider(r.Provider),
		MappingType:  r.MappingType,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
	if r.ExternalID.Valid {
		extID := r.ExternalID.String
		mapping.ExternalID = &extID
	}
	return mapping
}

// GetMapping retrieves a label provider mapping.
func (a *LabelAdapter) GetMapping(labelID, connectionID int64) (*domain.LabelProviderMapping, error) {
	var row labelProviderMappingRow
	query := `SELECT * FROM label_provider_mappings WHERE label_id = $1 AND connection_id = $2`

	if err := a.db.Get(&row, query, labelID, connectionID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get label mapping: %w", err)
	}

	return row.toEntity(), nil
}

// GetMappingByExternalID retrieves a mapping by external ID.
func (a *LabelAdapter) GetMappingByExternalID(connectionID int64, externalID string) (*domain.LabelProviderMapping, error) {
	var row labelProviderMappingRow
	query := `SELECT * FROM label_provider_mappings WHERE connection_id = $1 AND external_id = $2`

	if err := a.db.Get(&row, query, connectionID, externalID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get label mapping: %w", err)
	}

	return row.toEntity(), nil
}

// CreateMapping creates a new label provider mapping.
func (a *LabelAdapter) CreateMapping(mapping *domain.LabelProviderMapping) error {
	query := `
		INSERT INTO label_provider_mappings (label_id, connection_id, provider, external_id, mapping_type)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	var externalID sql.NullString
	if mapping.ExternalID != nil {
		externalID = sql.NullString{String: *mapping.ExternalID, Valid: true}
	}

	err := a.db.QueryRow(
		query,
		mapping.LabelID,
		mapping.ConnectionID,
		string(mapping.Provider),
		externalID,
		mapping.MappingType,
	).Scan(&mapping.ID, &mapping.CreatedAt, &mapping.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create label mapping: %w", err)
	}

	return nil
}

// UpdateMapping updates a label provider mapping.
func (a *LabelAdapter) UpdateMapping(mapping *domain.LabelProviderMapping) error {
	query := `
		UPDATE label_provider_mappings
		SET external_id = $2, updated_at = NOW()
		WHERE id = $1`

	var externalID sql.NullString
	if mapping.ExternalID != nil {
		externalID = sql.NullString{String: *mapping.ExternalID, Valid: true}
	}

	result, err := a.db.Exec(query, mapping.ID, externalID)
	if err != nil {
		return fmt.Errorf("failed to update label mapping: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("label mapping not found: %d", mapping.ID)
	}

	return nil
}

// DeleteMapping deletes a label provider mapping.
func (a *LabelAdapter) DeleteMapping(id int64) error {
	query := `DELETE FROM label_provider_mappings WHERE id = $1`

	result, err := a.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete label mapping: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("label mapping not found: %d", id)
	}

	return nil
}
