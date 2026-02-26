package out

import (
	"context"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

// TodoRepository defines the interface for todo persistence
type TodoRepository interface {
	// Todo CRUD
	GetTodo(ctx context.Context, id int64) (*domain.Todo, error)
	GetTodosByIDs(ctx context.Context, ids []int64) ([]*domain.Todo, error)
	ListTodos(ctx context.Context, filter *domain.TodoFilter) ([]*domain.Todo, int, error)
	CreateTodo(ctx context.Context, todo *domain.Todo) error
	UpdateTodo(ctx context.Context, todo *domain.Todo) error
	DeleteTodo(ctx context.Context, id int64) error

	// Batch operations
	CreateTodos(ctx context.Context, todos []*domain.Todo) error
	UpdateTodoStatus(ctx context.Context, id int64, status domain.TodoStatus) error
	CompleteTodo(ctx context.Context, id int64) error
	ReopenTodo(ctx context.Context, id int64) error

	// Subtasks
	GetSubtasks(ctx context.Context, parentID int64) ([]*domain.Todo, error)

	// By source
	GetTodoBySource(ctx context.Context, userID uuid.UUID, sourceType domain.TodoSourceType, sourceID string) (*domain.Todo, error)
	GetTodosByEmail(ctx context.Context, emailID int64) ([]*domain.Todo, error)
	GetTodosByEvent(ctx context.Context, eventID int64) ([]*domain.Todo, error)

	// Project CRUD
	GetProject(ctx context.Context, id int64) (*domain.TodoProject, error)
	ListProjects(ctx context.Context, filter *domain.TodoProjectFilter) ([]*domain.TodoProject, int, error)
	CreateProject(ctx context.Context, project *domain.TodoProject) error
	UpdateProject(ctx context.Context, project *domain.TodoProject) error
	DeleteProject(ctx context.Context, id int64) error

	// Stats
	CountTodosByStatus(ctx context.Context, userID uuid.UUID) (map[domain.TodoStatus]int, error)
	CountTodosInProject(ctx context.Context, projectID int64) (int, error)
	CountOverdueTodos(ctx context.Context, userID uuid.UUID) (int, error)
}
