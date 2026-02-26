package domain

import (
	"time"

	"github.com/google/uuid"
)

// FolderType represents the type of folder
type FolderType string

const (
	FolderTypeSystem FolderType = "system"
	FolderTypeUser   FolderType = "user"
)

// SystemFolderKey represents system folder identifiers
type SystemFolderKey string

const (
	SystemFolderInbox   SystemFolderKey = "inbox"
	SystemFolderSent    SystemFolderKey = "sent"
	SystemFolderDrafts  SystemFolderKey = "drafts"
	SystemFolderTrash   SystemFolderKey = "trash"
	SystemFolderSpam    SystemFolderKey = "spam"
	SystemFolderArchive SystemFolderKey = "archive"
)

// EmailFolder represents a user's email folder (both system and custom)
type EmailFolder struct {
	ID        int64            `json:"id"`
	UserID    uuid.UUID        `json:"user_id"`
	Name      string           `json:"name"`
	Type      FolderType       `json:"type"`
	SystemKey *SystemFolderKey `json:"system_key,omitempty"` // Only for system folders
	Color     *string          `json:"color,omitempty"`
	Icon      *string          `json:"icon,omitempty"`
	Position  int              `json:"position"`

	// Stats (cached, updated via triggers/jobs)
	TotalCount  int `json:"total_count"`
	UnreadCount int `json:"unread_count"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// IsSystem returns true if this is a system folder
func (f *EmailFolder) IsSystem() bool {
	return f.Type == FolderTypeSystem
}

// ProviderMappingType represents how folder maps to provider
type ProviderMappingType string

const (
	MappingTypeLabel    ProviderMappingType = "label"    // Gmail Labels
	MappingTypeFolder   ProviderMappingType = "folder"   // Outlook Folders
	MappingTypeCategory ProviderMappingType = "category" // Outlook Categories
)

// FolderProviderMapping maps workspace folders to provider-specific folders/labels
type FolderProviderMapping struct {
	ID           int64               `json:"id"`
	FolderID     int64               `json:"folder_id"`
	ConnectionID int64               `json:"connection_id"`
	Provider     Provider            `json:"provider"`
	ExternalID   *string             `json:"external_id,omitempty"` // Provider's folder/label ID (nil = not yet created)
	MappingType  ProviderMappingType `json:"mapping_type"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

// EmailFolderWithMappings represents a folder with its provider mappings
type EmailFolderWithMappings struct {
	Folder   *EmailFolder             `json:"folder"`
	Mappings []*FolderProviderMapping `json:"mappings,omitempty"`
}

// FolderRepository interface for folder operations
type FolderRepository interface {
	// Basic CRUD
	GetByID(id int64) (*EmailFolder, error)
	GetByUserID(userID uuid.UUID) ([]*EmailFolder, error)
	GetSystemFolder(userID uuid.UUID, systemKey SystemFolderKey) (*EmailFolder, error)
	Create(folder *EmailFolder) error
	Update(folder *EmailFolder) error
	Delete(id int64) error

	// Provider mappings
	GetMapping(folderID, connectionID int64) (*FolderProviderMapping, error)
	GetMappingByExternalID(connectionID int64, externalID string) (*FolderProviderMapping, error)
	CreateMapping(mapping *FolderProviderMapping) error
	UpdateMapping(mapping *FolderProviderMapping) error
	DeleteMapping(id int64) error
	GetMappingsByFolder(folderID int64) ([]*FolderProviderMapping, error)
	GetMappingsByConnection(connectionID int64) ([]*FolderProviderMapping, error)

	// Stats
	UpdateCounts(folderID int64, totalDelta, unreadDelta int) error
	RecalculateCounts(folderID int64) error
}
