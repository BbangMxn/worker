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

// FolderAdapter implements domain.FolderRepository using PostgreSQL.
type FolderAdapter struct {
	db *sqlx.DB
}

// NewFolderAdapter creates a new FolderAdapter.
func NewFolderAdapter(db *sqlx.DB) *FolderAdapter {
	return &FolderAdapter{db: db}
}

// folderRow represents the database row for folders.
type folderRow struct {
	ID          int64          `db:"id"`
	UserID      uuid.UUID      `db:"user_id"`
	Name        string         `db:"name"`
	Type        string         `db:"type"`
	SystemKey   sql.NullString `db:"system_key"`
	Color       sql.NullString `db:"color"`
	Icon        sql.NullString `db:"icon"`
	Position    int            `db:"position"`
	TotalCount  int            `db:"total_count"`
	UnreadCount int            `db:"unread_count"`
	CreatedAt   time.Time      `db:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at"`
	DeletedAt   sql.NullTime   `db:"deleted_at"`
}

func (r *folderRow) toEntity() *domain.EmailFolder {
	folder := &domain.EmailFolder{
		ID:          r.ID,
		UserID:      r.UserID,
		Name:        r.Name,
		Type:        domain.FolderType(r.Type),
		Position:    r.Position,
		TotalCount:  r.TotalCount,
		UnreadCount: r.UnreadCount,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}

	if r.SystemKey.Valid {
		key := domain.SystemFolderKey(r.SystemKey.String)
		folder.SystemKey = &key
	}
	if r.Color.Valid {
		folder.Color = &r.Color.String
	}
	if r.Icon.Valid {
		folder.Icon = &r.Icon.String
	}
	if r.DeletedAt.Valid {
		folder.DeletedAt = &r.DeletedAt.Time
	}

	return folder
}

// GetByID retrieves a folder by its ID.
func (a *FolderAdapter) GetByID(id int64) (*domain.EmailFolder, error) {
	var row folderRow
	query := `SELECT * FROM folders WHERE id = $1 AND deleted_at IS NULL`

	if err := a.db.Get(&row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("folder not found: %d", id)
		}
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}

	return row.toEntity(), nil
}

// GetByUserID retrieves all folders for a user.
func (a *FolderAdapter) GetByUserID(userID uuid.UUID) ([]*domain.EmailFolder, error) {
	var rows []folderRow
	query := `SELECT * FROM folders WHERE user_id = $1 AND deleted_at IS NULL ORDER BY position ASC`

	if err := a.db.Select(&rows, query, userID); err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}

	folders := make([]*domain.EmailFolder, len(rows))
	for i, row := range rows {
		folders[i] = row.toEntity()
	}

	return folders, nil
}

// GetSystemFolder retrieves a system folder by its key.
func (a *FolderAdapter) GetSystemFolder(userID uuid.UUID, systemKey domain.SystemFolderKey) (*domain.EmailFolder, error) {
	var row folderRow
	query := `SELECT * FROM folders WHERE user_id = $1 AND system_key = $2 AND deleted_at IS NULL`

	if err := a.db.Get(&row, query, userID, string(systemKey)); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("system folder not found: %s", systemKey)
		}
		return nil, fmt.Errorf("failed to get system folder: %w", err)
	}

	return row.toEntity(), nil
}

// Create creates a new folder.
func (a *FolderAdapter) Create(folder *domain.EmailFolder) error {
	query := `
		INSERT INTO folders (user_id, name, type, system_key, color, icon, position)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	var systemKey sql.NullString
	if folder.SystemKey != nil {
		systemKey = sql.NullString{String: string(*folder.SystemKey), Valid: true}
	}

	var color sql.NullString
	if folder.Color != nil {
		color = sql.NullString{String: *folder.Color, Valid: true}
	}

	var icon sql.NullString
	if folder.Icon != nil {
		icon = sql.NullString{String: *folder.Icon, Valid: true}
	}

	err := a.db.QueryRow(
		query,
		folder.UserID,
		folder.Name,
		string(folder.Type),
		systemKey,
		color,
		icon,
		folder.Position,
	).Scan(&folder.ID, &folder.CreatedAt, &folder.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create folder: %w", err)
	}

	return nil
}

// Update updates a folder.
func (a *FolderAdapter) Update(folder *domain.EmailFolder) error {
	query := `
		UPDATE folders
		SET name = $2, color = $3, icon = $4, position = $5, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`

	var color sql.NullString
	if folder.Color != nil {
		color = sql.NullString{String: *folder.Color, Valid: true}
	}

	var icon sql.NullString
	if folder.Icon != nil {
		icon = sql.NullString{String: *folder.Icon, Valid: true}
	}

	result, err := a.db.Exec(query, folder.ID, folder.Name, color, icon, folder.Position)
	if err != nil {
		return fmt.Errorf("failed to update folder: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("folder not found: %d", folder.ID)
	}

	return nil
}

// Delete soft-deletes a folder.
func (a *FolderAdapter) Delete(id int64) error {
	query := `UPDATE folders SET deleted_at = NOW() WHERE id = $1 AND type = 'user' AND deleted_at IS NULL`

	result, err := a.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete folder: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("folder not found or is system folder: %d", id)
	}

	return nil
}

// folderProviderMappingRow represents the database row for folder provider mappings.
type folderProviderMappingRow struct {
	ID           int64          `db:"id"`
	FolderID     int64          `db:"folder_id"`
	ConnectionID int64          `db:"connection_id"`
	Provider     string         `db:"provider"`
	ExternalID   sql.NullString `db:"external_id"`
	MappingType  string         `db:"mapping_type"`
	CreatedAt    time.Time      `db:"created_at"`
	UpdatedAt    time.Time      `db:"updated_at"`
}

func (r *folderProviderMappingRow) toEntity() *domain.FolderProviderMapping {
	mapping := &domain.FolderProviderMapping{
		ID:           r.ID,
		FolderID:     r.FolderID,
		ConnectionID: r.ConnectionID,
		Provider:     domain.Provider(r.Provider),
		MappingType:  domain.ProviderMappingType(r.MappingType),
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
	if r.ExternalID.Valid {
		mapping.ExternalID = &r.ExternalID.String
	}
	return mapping
}

// GetMapping retrieves a folder provider mapping.
func (a *FolderAdapter) GetMapping(folderID, connectionID int64) (*domain.FolderProviderMapping, error) {
	var row folderProviderMappingRow
	query := `SELECT * FROM folder_provider_mappings WHERE folder_id = $1 AND connection_id = $2`

	if err := a.db.Get(&row, query, folderID, connectionID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get folder mapping: %w", err)
	}

	return row.toEntity(), nil
}

// GetMappingByExternalID retrieves a mapping by external ID.
func (a *FolderAdapter) GetMappingByExternalID(connectionID int64, externalID string) (*domain.FolderProviderMapping, error) {
	var row folderProviderMappingRow
	query := `SELECT * FROM folder_provider_mappings WHERE connection_id = $1 AND external_id = $2`

	if err := a.db.Get(&row, query, connectionID, externalID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get folder mapping: %w", err)
	}

	return row.toEntity(), nil
}

// CreateMapping creates a new folder provider mapping.
func (a *FolderAdapter) CreateMapping(mapping *domain.FolderProviderMapping) error {
	query := `
		INSERT INTO folder_provider_mappings (folder_id, connection_id, provider, external_id, mapping_type)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	var externalID sql.NullString
	if mapping.ExternalID != nil {
		externalID = sql.NullString{String: *mapping.ExternalID, Valid: true}
	}

	err := a.db.QueryRow(
		query,
		mapping.FolderID,
		mapping.ConnectionID,
		string(mapping.Provider),
		externalID,
		string(mapping.MappingType),
	).Scan(&mapping.ID, &mapping.CreatedAt, &mapping.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create folder mapping: %w", err)
	}

	return nil
}

// UpdateMapping updates a folder provider mapping.
func (a *FolderAdapter) UpdateMapping(mapping *domain.FolderProviderMapping) error {
	query := `
		UPDATE folder_provider_mappings
		SET external_id = $2, updated_at = NOW()
		WHERE id = $1`

	var externalID sql.NullString
	if mapping.ExternalID != nil {
		externalID = sql.NullString{String: *mapping.ExternalID, Valid: true}
	}

	result, err := a.db.Exec(query, mapping.ID, externalID)
	if err != nil {
		return fmt.Errorf("failed to update folder mapping: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("folder mapping not found: %d", mapping.ID)
	}

	return nil
}

// DeleteMapping deletes a folder provider mapping.
func (a *FolderAdapter) DeleteMapping(id int64) error {
	query := `DELETE FROM folder_provider_mappings WHERE id = $1`

	result, err := a.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete folder mapping: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("folder mapping not found: %d", id)
	}

	return nil
}

// GetMappingsByFolder retrieves all mappings for a folder.
func (a *FolderAdapter) GetMappingsByFolder(folderID int64) ([]*domain.FolderProviderMapping, error) {
	var rows []folderProviderMappingRow
	query := `SELECT * FROM folder_provider_mappings WHERE folder_id = $1`

	if err := a.db.Select(&rows, query, folderID); err != nil {
		return nil, fmt.Errorf("failed to get folder mappings: %w", err)
	}

	mappings := make([]*domain.FolderProviderMapping, len(rows))
	for i, row := range rows {
		mappings[i] = row.toEntity()
	}

	return mappings, nil
}

// GetMappingsByConnection retrieves all mappings for a connection.
func (a *FolderAdapter) GetMappingsByConnection(connectionID int64) ([]*domain.FolderProviderMapping, error) {
	var rows []folderProviderMappingRow
	query := `SELECT * FROM folder_provider_mappings WHERE connection_id = $1`

	if err := a.db.Select(&rows, query, connectionID); err != nil {
		return nil, fmt.Errorf("failed to get folder mappings: %w", err)
	}

	mappings := make([]*domain.FolderProviderMapping, len(rows))
	for i, row := range rows {
		mappings[i] = row.toEntity()
	}

	return mappings, nil
}

// UpdateCounts updates the total and unread counts for a folder.
func (a *FolderAdapter) UpdateCounts(folderID int64, totalDelta, unreadDelta int) error {
	query := `
		UPDATE folders
		SET total_count = total_count + $2,
		    unread_count = unread_count + $3,
		    updated_at = NOW()
		WHERE id = $1`

	_, err := a.db.Exec(query, folderID, totalDelta, unreadDelta)
	if err != nil {
		return fmt.Errorf("failed to update folder counts: %w", err)
	}

	return nil
}

// RecalculateCounts recalculates the counts for a folder from emails table.
func (a *FolderAdapter) RecalculateCounts(folderID int64) error {
	query := `
		UPDATE folders f
		SET total_count = COALESCE((SELECT COUNT(*) FROM emails WHERE folder_id = f.id AND deleted_at IS NULL), 0),
		    unread_count = COALESCE((SELECT COUNT(*) FROM emails WHERE folder_id = f.id AND is_read = FALSE AND deleted_at IS NULL), 0),
		    updated_at = NOW()
		WHERE f.id = $1`

	_, err := a.db.Exec(query, folderID)
	if err != nil {
		return fmt.Errorf("failed to recalculate folder counts: %w", err)
	}

	return nil
}
