package domain

import (
	"time"

	"github.com/google/uuid"
)

// TodoStatus represents the status of a todo item
type TodoStatus string

const (
	TodoStatusInbox      TodoStatus = "inbox"
	TodoStatusPending    TodoStatus = "pending"
	TodoStatusInProgress TodoStatus = "in_progress"
	TodoStatusWaiting    TodoStatus = "waiting"
	TodoStatusCompleted  TodoStatus = "completed"
	TodoStatusCancelled  TodoStatus = "cancelled"
)

// TodoPriority represents priority levels (1=urgent, 4=low)
type TodoPriority int

const (
	TodoPriorityUrgent TodoPriority = 1
	TodoPriorityHigh   TodoPriority = 2
	TodoPriorityNormal TodoPriority = 3
	TodoPriorityLow    TodoPriority = 4
)

// TodoSourceType represents where the todo was created from
type TodoSourceType string

const (
	TodoSourceManual   TodoSourceType = "manual"
	TodoSourceEmail    TodoSourceType = "email"
	TodoSourceCalendar TodoSourceType = "calendar"
	TodoSourceAgent    TodoSourceType = "agent"
	TodoSourceJira     TodoSourceType = "jira"
	TodoSourceGitHub   TodoSourceType = "github"
	TodoSourceLinear   TodoSourceType = "linear"
)

// Todo represents a todo item
type Todo struct {
	ID     int64     `json:"id"`
	UserID uuid.UUID `json:"user_id"`

	// Hierarchy (all optional)
	ProjectID *int64  `json:"project_id,omitempty"`
	Area      *string `json:"area,omitempty"`
	ParentID  *int64  `json:"parent_id,omitempty"`

	// Basic info
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`

	// Status & Priority
	Status   TodoStatus   `json:"status"`
	Priority TodoPriority `json:"priority"`

	// Schedule
	DueDate     *time.Time `json:"due_date,omitempty"`
	DueDatetime *time.Time `json:"due_datetime,omitempty"`
	StartDate   *time.Time `json:"start_date,omitempty"`

	// Source linking
	SourceType     *TodoSourceType        `json:"source_type,omitempty"`
	SourceID       *string                `json:"source_id,omitempty"`
	SourceURL      *string                `json:"source_url,omitempty"`
	SourceMetadata map[string]interface{} `json:"source_metadata,omitempty"`

	// Related entities
	RelatedEmailID *int64 `json:"related_email_id,omitempty"`
	RelatedEventID *int64 `json:"related_event_id,omitempty"`

	// Classification
	Tags []string `json:"tags,omitempty"`

	// Ordering
	SortOrder int `json:"sort_order"`

	// Completion
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TodoProject represents a project that groups todos
type TodoProject struct {
	ID     int64     `json:"id"`
	UserID uuid.UUID `json:"user_id"`

	// Basic info
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`

	// Organization
	Area  *string `json:"area,omitempty"`
	Color *string `json:"color,omitempty"`
	Icon  *string `json:"icon,omitempty"`

	// Status
	Status string `json:"status"` // active, completed, archived

	// Ordering
	SortOrder int `json:"sort_order"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TodoFilter represents filter options for listing todos
type TodoFilter struct {
	UserID      uuid.UUID
	ProjectID   *int64
	Area        *string
	ParentID    *int64
	Status      *TodoStatus
	Statuses    []TodoStatus // Multiple statuses (OR)
	Priority    *TodoPriority
	MinPriority *TodoPriority // Priority <= this value (1=urgent is highest)

	// Date filters
	DueDateFrom *time.Time
	DueDateTo   *time.Time
	DueToday    bool
	Overdue     bool

	// Source filters
	SourceType     *TodoSourceType
	RelatedEmailID *int64
	RelatedEventID *int64

	// Search
	Search *string

	// View types
	ViewType string // inbox, today, upcoming, completed

	// Pagination
	Limit  int
	Offset int

	// Sorting
	SortBy    string // created_at, due_date, priority, sort_order
	SortOrder string // asc, desc
}

// TodoProjectFilter represents filter options for listing projects
type TodoProjectFilter struct {
	UserID uuid.UUID
	Area   *string
	Status *string

	Limit  int
	Offset int
}

// IsCompleted returns true if the todo is completed
func (t *Todo) IsCompleted() bool {
	return t.Status == TodoStatusCompleted
}

// IsOverdue returns true if the todo is past due date
func (t *Todo) IsOverdue() bool {
	if t.DueDate == nil && t.DueDatetime == nil {
		return false
	}
	if t.IsCompleted() {
		return false
	}

	now := time.Now()
	if t.DueDatetime != nil {
		return t.DueDatetime.Before(now)
	}
	if t.DueDate != nil {
		// Compare date only
		dueEnd := time.Date(t.DueDate.Year(), t.DueDate.Month(), t.DueDate.Day(), 23, 59, 59, 0, t.DueDate.Location())
		return dueEnd.Before(now)
	}
	return false
}

// Complete marks the todo as completed
func (t *Todo) Complete() {
	now := time.Now()
	t.Status = TodoStatusCompleted
	t.CompletedAt = &now
	t.UpdatedAt = now
}

// Reopen marks the todo as pending (uncomplete)
func (t *Todo) Reopen() {
	t.Status = TodoStatusPending
	t.CompletedAt = nil
	t.UpdatedAt = time.Now()
}
