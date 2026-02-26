package in

import (
	"context"
	"time"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

// TodoService defines the interface for todo operations
type TodoService interface {
	// === Todo CRUD ===
	GetTodo(ctx context.Context, userID uuid.UUID, todoID int64) (*domain.Todo, error)
	ListTodos(ctx context.Context, filter *domain.TodoFilter) (*TodoListResponse, error)
	CreateTodo(ctx context.Context, userID uuid.UUID, req *CreateTodoRequest) (*domain.Todo, error)
	UpdateTodo(ctx context.Context, userID uuid.UUID, todoID int64, req *UpdateTodoRequest) (*domain.Todo, error)
	DeleteTodo(ctx context.Context, userID uuid.UUID, todoID int64) error

	// === Status Operations ===
	CompleteTodo(ctx context.Context, userID uuid.UUID, todoID int64) error
	ReopenTodo(ctx context.Context, userID uuid.UUID, todoID int64) error
	UpdateStatus(ctx context.Context, userID uuid.UUID, todoID int64, status domain.TodoStatus) error

	// === Batch Operations ===
	CompleteTodos(ctx context.Context, userID uuid.UUID, todoIDs []int64) error
	DeleteTodos(ctx context.Context, userID uuid.UUID, todoIDs []int64) error

	// === Source-based Creation ===
	CreateFromEmail(ctx context.Context, userID uuid.UUID, req *CreateTodoFromEmailRequest) (*domain.Todo, error)
	CreateFromCalendar(ctx context.Context, userID uuid.UUID, req *CreateTodoFromCalendarRequest) (*domain.Todo, error)
	CreateFromAgent(ctx context.Context, userID uuid.UUID, req *CreateTodoFromAgentRequest) (*domain.Todo, error)

	// === Subtasks ===
	GetSubtasks(ctx context.Context, userID uuid.UUID, parentID int64) ([]*domain.Todo, error)
	AddSubtask(ctx context.Context, userID uuid.UUID, parentID int64, req *CreateTodoRequest) (*domain.Todo, error)

	// === Views ===
	GetInbox(ctx context.Context, userID uuid.UUID, limit, offset int) (*TodoListResponse, error)
	GetToday(ctx context.Context, userID uuid.UUID) (*TodoListResponse, error)
	GetUpcoming(ctx context.Context, userID uuid.UUID, days int) (*TodoListResponse, error)
	GetOverdue(ctx context.Context, userID uuid.UUID) (*TodoListResponse, error)
	GetByDateRange(ctx context.Context, userID uuid.UUID, start, end time.Time) (*TodoListResponse, error)

	// === Project Operations ===
	GetProject(ctx context.Context, userID uuid.UUID, projectID int64) (*domain.TodoProject, error)
	ListProjects(ctx context.Context, userID uuid.UUID) ([]*domain.TodoProject, error)
	CreateProject(ctx context.Context, userID uuid.UUID, req *CreateProjectRequest) (*domain.TodoProject, error)
	UpdateProject(ctx context.Context, userID uuid.UUID, projectID int64, req *UpdateProjectRequest) (*domain.TodoProject, error)
	DeleteProject(ctx context.Context, userID uuid.UUID, projectID int64) error
	GetProjectTodos(ctx context.Context, userID uuid.UUID, projectID int64) (*TodoListResponse, error)

	// === Stats ===
	GetStats(ctx context.Context, userID uuid.UUID) (*TodoStats, error)

	// === Related Queries (for Calendar integration) ===
	GetTodosByEmail(ctx context.Context, emailID int64) ([]*domain.Todo, error)
	GetTodosByEvent(ctx context.Context, eventID int64) ([]*domain.Todo, error)
}

// =============================================================================
// Request/Response Types
// =============================================================================

type TodoListResponse struct {
	Todos []*domain.Todo `json:"todos"`
	Total int            `json:"total"`
}

type CreateTodoRequest struct {
	Title       string               `json:"title" validate:"required,max=500"`
	Description string               `json:"description,omitempty"`
	ProjectID   *int64               `json:"project_id,omitempty"`
	Area        *string              `json:"area,omitempty"`
	ParentID    *int64               `json:"parent_id,omitempty"`
	Priority    *domain.TodoPriority `json:"priority,omitempty"`
	DueDate     *time.Time           `json:"due_date,omitempty"`
	DueDatetime *time.Time           `json:"due_datetime,omitempty"`
	StartDate   *time.Time           `json:"start_date,omitempty"`
	Tags        []string             `json:"tags,omitempty"`
}

type UpdateTodoRequest struct {
	Title       *string              `json:"title,omitempty"`
	Description *string              `json:"description,omitempty"`
	ProjectID   *int64               `json:"project_id,omitempty"`
	Area        *string              `json:"area,omitempty"`
	Priority    *domain.TodoPriority `json:"priority,omitempty"`
	Status      *domain.TodoStatus   `json:"status,omitempty"`
	DueDate     *time.Time           `json:"due_date,omitempty"`
	DueDatetime *time.Time           `json:"due_datetime,omitempty"`
	StartDate   *time.Time           `json:"start_date,omitempty"`
	Tags        []string             `json:"tags,omitempty"`
	SortOrder   *int                 `json:"sort_order,omitempty"`
}

// =============================================================================
// Source-based Creation Requests
// =============================================================================

// CreateTodoFromEmailRequest creates a todo from an email action item
type CreateTodoFromEmailRequest struct {
	EmailID     int64                `json:"email_id" validate:"required"`
	Title       string               `json:"title" validate:"required"`
	Description string               `json:"description,omitempty"`
	Priority    *domain.TodoPriority `json:"priority,omitempty"`
	DueDate     *time.Time           `json:"due_date,omitempty"`
	ActionType  string               `json:"action_type,omitempty"` // review, respond, fix, etc.
	SourceURL   string               `json:"source_url,omitempty"`
}

// CreateTodoFromCalendarRequest creates a todo from a calendar event
type CreateTodoFromCalendarRequest struct {
	EventID     int64                `json:"event_id" validate:"required"`
	Title       string               `json:"title" validate:"required"`
	Description string               `json:"description,omitempty"`
	Priority    *domain.TodoPriority `json:"priority,omitempty"`
	DueDate     *time.Time           `json:"due_date,omitempty"`  // defaults to event start
	PrepTime    *int                 `json:"prep_time,omitempty"` // minutes before event
}

// CreateTodoFromAgentRequest creates a todo extracted from AI agent conversation
type CreateTodoFromAgentRequest struct {
	Title         string               `json:"title" validate:"required"`
	Description   string               `json:"description,omitempty"`
	Priority      *domain.TodoPriority `json:"priority,omitempty"`
	DueDate       *time.Time           `json:"due_date,omitempty"`
	DueDatetime   *time.Time           `json:"due_datetime,omitempty"`
	ExtractedFrom string               `json:"extracted_from,omitempty"` // original text
	Confidence    float64              `json:"confidence,omitempty"`
}

// =============================================================================
// Project Requests
// =============================================================================

type CreateProjectRequest struct {
	Name        string  `json:"name" validate:"required,max=200"`
	Description *string `json:"description,omitempty"`
	Area        *string `json:"area,omitempty"`
	Color       *string `json:"color,omitempty"`
	Icon        *string `json:"icon,omitempty"`
}

type UpdateProjectRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Area        *string `json:"area,omitempty"`
	Color       *string `json:"color,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	Status      *string `json:"status,omitempty"`
	SortOrder   *int    `json:"sort_order,omitempty"`
}

// =============================================================================
// Stats
// =============================================================================

type TodoStats struct {
	Total      int            `json:"total"`
	Inbox      int            `json:"inbox"`
	Today      int            `json:"today"`
	Upcoming   int            `json:"upcoming"`
	Overdue    int            `json:"overdue"`
	Completed  int            `json:"completed"`
	ByStatus   map[string]int `json:"by_status"`
	ByPriority map[int]int    `json:"by_priority"`
	BySource   map[string]int `json:"by_source"`
}
