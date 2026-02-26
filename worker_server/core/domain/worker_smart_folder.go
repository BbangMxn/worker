package domain

import (
	"time"

	"github.com/google/uuid"
)

// SmartFolderQuery represents the query conditions for a smart folder
type SmartFolderQuery struct {
	// Category filtering
	Categories    []EmailCategory    `json:"categories,omitempty"`
	SubCategories []EmailSubCategory `json:"sub_categories,omitempty"`

	// Priority filtering
	Priorities []Priority `json:"priorities,omitempty"`

	// Read status
	IsRead *bool `json:"is_read,omitempty"`

	// Star status
	IsStarred *bool `json:"is_starred,omitempty"`

	// Date range (relative)
	DateRange string `json:"date_range,omitempty"` // "7_days", "30_days", "90_days"

	// Sender filtering
	FromDomains []string `json:"from_domains,omitempty"`
	FromEmails  []string `json:"from_emails,omitempty"`

	// Label filtering
	LabelIDs []int64 `json:"label_ids,omitempty"`

	// VIP/Muted sender filtering
	IsVIP   *bool `json:"is_vip,omitempty"`
	IsMuted *bool `json:"is_muted,omitempty"`

	// Full text search
	SearchQuery string `json:"search_query,omitempty"`
}

// SmartFolder represents a virtual query-based folder
type SmartFolder struct {
	ID       int64            `json:"id"`
	UserID   uuid.UUID        `json:"user_id"`
	Name     string           `json:"name"`
	Icon     *string          `json:"icon,omitempty"`
	Color    *string          `json:"color,omitempty"`
	Query    SmartFolderQuery `json:"query"`
	IsSystem bool             `json:"is_system"` // System smart folders cannot be deleted
	Position int              `json:"position"`

	// Stats (calculated on-demand or cached)
	TotalCount  int `json:"total_count"`
	UnreadCount int `json:"unread_count"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SmartFolderRepository interface for smart folder operations
type SmartFolderRepository interface {
	// Basic CRUD
	GetByID(id int64) (*SmartFolder, error)
	GetByUserID(userID uuid.UUID) ([]*SmartFolder, error)
	Create(folder *SmartFolder) error
	Update(folder *SmartFolder) error
	Delete(id int64) error

	// Query execution
	CountEmails(folderID int64) (total int, unread int, err error)
	GetEmailIDs(folderID int64, limit, offset int) ([]int64, error)
}
