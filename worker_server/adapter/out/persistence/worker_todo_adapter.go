package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/pkg/snowflake"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// TodoRepository implements out.TodoRepository
type TodoRepository struct {
	db *sqlx.DB
}

// NewTodoRepository creates a new TodoRepository
func NewTodoRepository(db *sqlx.DB) out.TodoRepository {
	return &TodoRepository{db: db}
}

// =============================================================================
// Todo CRUD
// =============================================================================

func (r *TodoRepository) GetTodo(ctx context.Context, id int64) (*domain.Todo, error) {
	query := `
		SELECT id, user_id, project_id, area, parent_id, title, description,
		       status, priority, due_date, due_datetime, start_date,
		       source_type, source_id, source_url, source_metadata,
		       related_email_id, related_event_id, tags, sort_order,
		       completed_at, created_at, updated_at
		FROM todos
		WHERE id = $1`

	var row todoRow
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get todo: %w", err)
	}

	return row.toDomain(), nil
}

func (r *TodoRepository) GetTodosByIDs(ctx context.Context, ids []int64) ([]*domain.Todo, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	query := `
		SELECT id, user_id, project_id, area, parent_id, title, description,
		       status, priority, due_date, due_datetime, start_date,
		       source_type, source_id, source_url, source_metadata,
		       related_email_id, related_event_id, tags, sort_order,
		       completed_at, created_at, updated_at
		FROM todos
		WHERE id = ANY($1)`

	var rows []todoRow
	if err := r.db.SelectContext(ctx, &rows, query, pq.Array(ids)); err != nil {
		return nil, fmt.Errorf("get todos by ids: %w", err)
	}

	todos := make([]*domain.Todo, len(rows))
	for i, row := range rows {
		todos[i] = row.toDomain()
	}
	return todos, nil
}

func (r *TodoRepository) ListTodos(ctx context.Context, filter *domain.TodoFilter) ([]*domain.Todo, int, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIdx))
	args = append(args, filter.UserID)
	argIdx++

	// View type shortcuts
	switch filter.ViewType {
	case "inbox":
		conditions = append(conditions, "project_id IS NULL")
		conditions = append(conditions, "area IS NULL")
		conditions = append(conditions, "status = 'inbox'")
	case "today":
		conditions = append(conditions, "due_date = CURRENT_DATE")
		conditions = append(conditions, "status NOT IN ('completed', 'cancelled')")
	case "upcoming":
		conditions = append(conditions, "due_date IS NOT NULL")
		conditions = append(conditions, "due_date > CURRENT_DATE")
		conditions = append(conditions, "status NOT IN ('completed', 'cancelled')")
	case "completed":
		conditions = append(conditions, "status = 'completed'")
	}

	if filter.ProjectID != nil {
		conditions = append(conditions, fmt.Sprintf("project_id = $%d", argIdx))
		args = append(args, *filter.ProjectID)
		argIdx++
	}

	if filter.Area != nil {
		conditions = append(conditions, fmt.Sprintf("area = $%d", argIdx))
		args = append(args, *filter.Area)
		argIdx++
	}

	if filter.ParentID != nil {
		conditions = append(conditions, fmt.Sprintf("parent_id = $%d", argIdx))
		args = append(args, *filter.ParentID)
		argIdx++
	}

	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *filter.Status)
		argIdx++
	}

	if len(filter.Statuses) > 0 {
		conditions = append(conditions, fmt.Sprintf("status = ANY($%d)", argIdx))
		args = append(args, pq.Array(filter.Statuses))
		argIdx++
	}

	if filter.Priority != nil {
		conditions = append(conditions, fmt.Sprintf("priority = $%d", argIdx))
		args = append(args, *filter.Priority)
		argIdx++
	}

	if filter.MinPriority != nil {
		conditions = append(conditions, fmt.Sprintf("priority <= $%d", argIdx))
		args = append(args, *filter.MinPriority)
		argIdx++
	}

	if filter.DueDateFrom != nil {
		conditions = append(conditions, fmt.Sprintf("due_date >= $%d", argIdx))
		args = append(args, *filter.DueDateFrom)
		argIdx++
	}

	if filter.DueDateTo != nil {
		conditions = append(conditions, fmt.Sprintf("due_date <= $%d", argIdx))
		args = append(args, *filter.DueDateTo)
		argIdx++
	}

	if filter.DueToday {
		conditions = append(conditions, "due_date = CURRENT_DATE")
	}

	if filter.Overdue {
		conditions = append(conditions, "due_date < CURRENT_DATE")
		conditions = append(conditions, "status NOT IN ('completed', 'cancelled')")
	}

	if filter.SourceType != nil {
		conditions = append(conditions, fmt.Sprintf("source_type = $%d", argIdx))
		args = append(args, *filter.SourceType)
		argIdx++
	}

	if filter.RelatedEmailID != nil {
		conditions = append(conditions, fmt.Sprintf("related_email_id = $%d", argIdx))
		args = append(args, *filter.RelatedEmailID)
		argIdx++
	}

	if filter.RelatedEventID != nil {
		conditions = append(conditions, fmt.Sprintf("related_event_id = $%d", argIdx))
		args = append(args, *filter.RelatedEventID)
		argIdx++
	}

	if filter.Search != nil && *filter.Search != "" {
		conditions = append(conditions, fmt.Sprintf("title ILIKE $%d", argIdx))
		args = append(args, "%"+*filter.Search+"%")
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count query
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM todos WHERE %s", whereClause)
	var total int
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("count todos: %w", err)
	}

	// Sort
	orderBy := "created_at DESC"
	switch filter.SortBy {
	case "due_date":
		orderBy = "due_date NULLS LAST, due_datetime NULLS LAST"
	case "priority":
		orderBy = "priority ASC, due_date NULLS LAST"
	case "sort_order":
		orderBy = "sort_order ASC, created_at DESC"
	case "created_at":
		if filter.SortOrder == "asc" {
			orderBy = "created_at ASC"
		}
	}

	// Data query
	query := fmt.Sprintf(`
		SELECT id, user_id, project_id, area, parent_id, title, description,
		       status, priority, due_date, due_datetime, start_date,
		       source_type, source_id, source_url, source_metadata,
		       related_email_id, related_event_id, tags, sort_order,
		       completed_at, created_at, updated_at
		FROM todos
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d`,
		whereClause, orderBy, argIdx, argIdx+1)

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit, filter.Offset)

	var rows []todoRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, 0, fmt.Errorf("list todos: %w", err)
	}

	todos := make([]*domain.Todo, len(rows))
	for i, row := range rows {
		todos[i] = row.toDomain()
	}

	return todos, total, nil
}

func (r *TodoRepository) CreateTodo(ctx context.Context, todo *domain.Todo) error {
	if todo.ID == 0 {
		todo.ID = snowflake.ID()
	}
	if todo.CreatedAt.IsZero() {
		todo.CreatedAt = time.Now()
	}
	todo.UpdatedAt = time.Now()

	metadata, _ := json.Marshal(todo.SourceMetadata)

	query := `
		INSERT INTO todos (
			id, user_id, project_id, area, parent_id, title, description,
			status, priority, due_date, due_datetime, start_date,
			source_type, source_id, source_url, source_metadata,
			related_email_id, related_event_id, tags, sort_order,
			completed_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23
		)`

	_, err := r.db.ExecContext(ctx, query,
		todo.ID, todo.UserID, todo.ProjectID, todo.Area, todo.ParentID,
		todo.Title, todo.Description, todo.Status, todo.Priority,
		todo.DueDate, todo.DueDatetime, todo.StartDate,
		todo.SourceType, todo.SourceID, todo.SourceURL, metadata,
		todo.RelatedEmailID, todo.RelatedEventID, pq.Array(todo.Tags),
		todo.SortOrder, todo.CompletedAt, todo.CreatedAt, todo.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create todo: %w", err)
	}

	return nil
}

func (r *TodoRepository) UpdateTodo(ctx context.Context, todo *domain.Todo) error {
	todo.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(todo.SourceMetadata)

	query := `
		UPDATE todos SET
			project_id = $2, area = $3, parent_id = $4, title = $5,
			description = $6, status = $7, priority = $8, due_date = $9,
			due_datetime = $10, start_date = $11, source_type = $12,
			source_id = $13, source_url = $14, source_metadata = $15,
			related_email_id = $16, related_event_id = $17, tags = $18,
			sort_order = $19, completed_at = $20, updated_at = $21
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		todo.ID, todo.ProjectID, todo.Area, todo.ParentID, todo.Title,
		todo.Description, todo.Status, todo.Priority, todo.DueDate,
		todo.DueDatetime, todo.StartDate, todo.SourceType,
		todo.SourceID, todo.SourceURL, metadata,
		todo.RelatedEmailID, todo.RelatedEventID, pq.Array(todo.Tags),
		todo.SortOrder, todo.CompletedAt, todo.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update todo: %w", err)
	}

	return nil
}

func (r *TodoRepository) DeleteTodo(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM todos WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete todo: %w", err)
	}
	return nil
}

// =============================================================================
// Batch & Status Operations
// =============================================================================

func (r *TodoRepository) CreateTodos(ctx context.Context, todos []*domain.Todo) error {
	if len(todos) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, todo := range todos {
		if todo.ID == 0 {
			todo.ID = snowflake.ID()
		}
		if todo.CreatedAt.IsZero() {
			todo.CreatedAt = time.Now()
		}
		todo.UpdatedAt = time.Now()

		metadata, _ := json.Marshal(todo.SourceMetadata)

		query := `
			INSERT INTO todos (
				id, user_id, project_id, area, parent_id, title, description,
				status, priority, due_date, due_datetime, start_date,
				source_type, source_id, source_url, source_metadata,
				related_email_id, related_event_id, tags, sort_order,
				completed_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
				$13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23
			)`

		_, err := tx.ExecContext(ctx, query,
			todo.ID, todo.UserID, todo.ProjectID, todo.Area, todo.ParentID,
			todo.Title, todo.Description, todo.Status, todo.Priority,
			todo.DueDate, todo.DueDatetime, todo.StartDate,
			todo.SourceType, todo.SourceID, todo.SourceURL, metadata,
			todo.RelatedEmailID, todo.RelatedEventID, pq.Array(todo.Tags),
			todo.SortOrder, todo.CompletedAt, todo.CreatedAt, todo.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("create todo batch: %w", err)
		}
	}

	return tx.Commit()
}

func (r *TodoRepository) UpdateTodoStatus(ctx context.Context, id int64, status domain.TodoStatus) error {
	query := `UPDATE todos SET status = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return fmt.Errorf("update todo status: %w", err)
	}
	return nil
}

func (r *TodoRepository) CompleteTodo(ctx context.Context, id int64) error {
	query := `UPDATE todos SET status = 'completed', completed_at = NOW(), updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("complete todo: %w", err)
	}
	return nil
}

func (r *TodoRepository) ReopenTodo(ctx context.Context, id int64) error {
	query := `UPDATE todos SET status = 'pending', completed_at = NULL, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("reopen todo: %w", err)
	}
	return nil
}

// =============================================================================
// Subtasks & Source
// =============================================================================

func (r *TodoRepository) GetSubtasks(ctx context.Context, parentID int64) ([]*domain.Todo, error) {
	query := `
		SELECT id, user_id, project_id, area, parent_id, title, description,
		       status, priority, due_date, due_datetime, start_date,
		       source_type, source_id, source_url, source_metadata,
		       related_email_id, related_event_id, tags, sort_order,
		       completed_at, created_at, updated_at
		FROM todos
		WHERE parent_id = $1
		ORDER BY sort_order, created_at`

	var rows []todoRow
	if err := r.db.SelectContext(ctx, &rows, query, parentID); err != nil {
		return nil, fmt.Errorf("get subtasks: %w", err)
	}

	todos := make([]*domain.Todo, len(rows))
	for i, row := range rows {
		todos[i] = row.toDomain()
	}
	return todos, nil
}

func (r *TodoRepository) GetTodoBySource(ctx context.Context, userID uuid.UUID, sourceType domain.TodoSourceType, sourceID string) (*domain.Todo, error) {
	query := `
		SELECT id, user_id, project_id, area, parent_id, title, description,
		       status, priority, due_date, due_datetime, start_date,
		       source_type, source_id, source_url, source_metadata,
		       related_email_id, related_event_id, tags, sort_order,
		       completed_at, created_at, updated_at
		FROM todos
		WHERE user_id = $1 AND source_type = $2 AND source_id = $3`

	var row todoRow
	if err := r.db.GetContext(ctx, &row, query, userID, sourceType, sourceID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get todo by source: %w", err)
	}

	return row.toDomain(), nil
}

func (r *TodoRepository) GetTodosByEmail(ctx context.Context, emailID int64) ([]*domain.Todo, error) {
	query := `
		SELECT id, user_id, project_id, area, parent_id, title, description,
		       status, priority, due_date, due_datetime, start_date,
		       source_type, source_id, source_url, source_metadata,
		       related_email_id, related_event_id, tags, sort_order,
		       completed_at, created_at, updated_at
		FROM todos
		WHERE related_email_id = $1
		ORDER BY created_at`

	var rows []todoRow
	if err := r.db.SelectContext(ctx, &rows, query, emailID); err != nil {
		return nil, fmt.Errorf("get todos by email: %w", err)
	}

	todos := make([]*domain.Todo, len(rows))
	for i, row := range rows {
		todos[i] = row.toDomain()
	}
	return todos, nil
}

func (r *TodoRepository) GetTodosByEvent(ctx context.Context, eventID int64) ([]*domain.Todo, error) {
	query := `
		SELECT id, user_id, project_id, area, parent_id, title, description,
		       status, priority, due_date, due_datetime, start_date,
		       source_type, source_id, source_url, source_metadata,
		       related_email_id, related_event_id, tags, sort_order,
		       completed_at, created_at, updated_at
		FROM todos
		WHERE related_event_id = $1
		ORDER BY created_at`

	var rows []todoRow
	if err := r.db.SelectContext(ctx, &rows, query, eventID); err != nil {
		return nil, fmt.Errorf("get todos by event: %w", err)
	}

	todos := make([]*domain.Todo, len(rows))
	for i, row := range rows {
		todos[i] = row.toDomain()
	}
	return todos, nil
}

// =============================================================================
// Project CRUD
// =============================================================================

func (r *TodoRepository) GetProject(ctx context.Context, id int64) (*domain.TodoProject, error) {
	query := `
		SELECT id, user_id, name, description, area, color, icon,
		       status, sort_order, created_at, updated_at
		FROM todo_projects
		WHERE id = $1`

	var row projectRow
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get project: %w", err)
	}

	return row.toDomain(), nil
}

func (r *TodoRepository) ListProjects(ctx context.Context, filter *domain.TodoProjectFilter) ([]*domain.TodoProject, int, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIdx))
	args = append(args, filter.UserID)
	argIdx++

	if filter.Area != nil {
		conditions = append(conditions, fmt.Sprintf("area = $%d", argIdx))
		args = append(args, *filter.Area)
		argIdx++
	}

	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *filter.Status)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM todo_projects WHERE %s", whereClause)
	var total int
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("count projects: %w", err)
	}

	// Data
	query := fmt.Sprintf(`
		SELECT id, user_id, name, description, area, color, icon,
		       status, sort_order, created_at, updated_at
		FROM todo_projects
		WHERE %s
		ORDER BY sort_order, created_at
		LIMIT $%d OFFSET $%d`,
		whereClause, argIdx, argIdx+1)

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit, filter.Offset)

	var rows []projectRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, 0, fmt.Errorf("list projects: %w", err)
	}

	projects := make([]*domain.TodoProject, len(rows))
	for i, row := range rows {
		projects[i] = row.toDomain()
	}

	return projects, total, nil
}

func (r *TodoRepository) CreateProject(ctx context.Context, project *domain.TodoProject) error {
	if project.ID == 0 {
		project.ID = snowflake.ID()
	}
	if project.CreatedAt.IsZero() {
		project.CreatedAt = time.Now()
	}
	project.UpdatedAt = time.Now()

	query := `
		INSERT INTO todo_projects (
			id, user_id, name, description, area, color, icon,
			status, sort_order, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err := r.db.ExecContext(ctx, query,
		project.ID, project.UserID, project.Name, project.Description,
		project.Area, project.Color, project.Icon, project.Status,
		project.SortOrder, project.CreatedAt, project.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}

	return nil
}

func (r *TodoRepository) UpdateProject(ctx context.Context, project *domain.TodoProject) error {
	project.UpdatedAt = time.Now()

	query := `
		UPDATE todo_projects SET
			name = $2, description = $3, area = $4, color = $5, icon = $6,
			status = $7, sort_order = $8, updated_at = $9
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		project.ID, project.Name, project.Description, project.Area,
		project.Color, project.Icon, project.Status, project.SortOrder,
		project.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}

	return nil
}

func (r *TodoRepository) DeleteProject(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM todo_projects WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}

// =============================================================================
// Stats
// =============================================================================

func (r *TodoRepository) CountTodosByStatus(ctx context.Context, userID uuid.UUID) (map[domain.TodoStatus]int, error) {
	query := `
		SELECT status, COUNT(*) as count
		FROM todos
		WHERE user_id = $1
		GROUP BY status`

	type statusCount struct {
		Status string `db:"status"`
		Count  int    `db:"count"`
	}

	var rows []statusCount
	if err := r.db.SelectContext(ctx, &rows, query, userID); err != nil {
		return nil, fmt.Errorf("count by status: %w", err)
	}

	result := make(map[domain.TodoStatus]int)
	for _, row := range rows {
		result[domain.TodoStatus(row.Status)] = row.Count
	}

	return result, nil
}

func (r *TodoRepository) CountTodosInProject(ctx context.Context, projectID int64) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count,
		"SELECT COUNT(*) FROM todos WHERE project_id = $1 AND status NOT IN ('completed', 'cancelled')",
		projectID)
	if err != nil {
		return 0, fmt.Errorf("count in project: %w", err)
	}
	return count, nil
}

func (r *TodoRepository) CountOverdueTodos(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM todos
		 WHERE user_id = $1 AND due_date < CURRENT_DATE
		 AND status NOT IN ('completed', 'cancelled')`,
		userID)
	if err != nil {
		return 0, fmt.Errorf("count overdue: %w", err)
	}
	return count, nil
}

// =============================================================================
// Row Types
// =============================================================================

type todoRow struct {
	ID             int64          `db:"id"`
	UserID         uuid.UUID      `db:"user_id"`
	ProjectID      sql.NullInt64  `db:"project_id"`
	Area           sql.NullString `db:"area"`
	ParentID       sql.NullInt64  `db:"parent_id"`
	Title          string         `db:"title"`
	Description    sql.NullString `db:"description"`
	Status         string         `db:"status"`
	Priority       int            `db:"priority"`
	DueDate        sql.NullTime   `db:"due_date"`
	DueDatetime    sql.NullTime   `db:"due_datetime"`
	StartDate      sql.NullTime   `db:"start_date"`
	SourceType     sql.NullString `db:"source_type"`
	SourceID       sql.NullString `db:"source_id"`
	SourceURL      sql.NullString `db:"source_url"`
	SourceMetadata []byte         `db:"source_metadata"`
	RelatedEmailID sql.NullInt64  `db:"related_email_id"`
	RelatedEventID sql.NullInt64  `db:"related_event_id"`
	Tags           pq.StringArray `db:"tags"`
	SortOrder      int            `db:"sort_order"`
	CompletedAt    sql.NullTime   `db:"completed_at"`
	CreatedAt      time.Time      `db:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at"`
}

func (r *todoRow) toDomain() *domain.Todo {
	todo := &domain.Todo{
		ID:          r.ID,
		UserID:      r.UserID,
		Title:       r.Title,
		Description: r.Description.String,
		Status:      domain.TodoStatus(r.Status),
		Priority:    domain.TodoPriority(r.Priority),
		Tags:        r.Tags,
		SortOrder:   r.SortOrder,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}

	if r.ProjectID.Valid {
		todo.ProjectID = &r.ProjectID.Int64
	}
	if r.Area.Valid {
		todo.Area = &r.Area.String
	}
	if r.ParentID.Valid {
		todo.ParentID = &r.ParentID.Int64
	}
	if r.DueDate.Valid {
		todo.DueDate = &r.DueDate.Time
	}
	if r.DueDatetime.Valid {
		todo.DueDatetime = &r.DueDatetime.Time
	}
	if r.StartDate.Valid {
		todo.StartDate = &r.StartDate.Time
	}
	if r.SourceType.Valid {
		st := domain.TodoSourceType(r.SourceType.String)
		todo.SourceType = &st
	}
	if r.SourceID.Valid {
		todo.SourceID = &r.SourceID.String
	}
	if r.SourceURL.Valid {
		todo.SourceURL = &r.SourceURL.String
	}
	if len(r.SourceMetadata) > 0 {
		json.Unmarshal(r.SourceMetadata, &todo.SourceMetadata)
	}
	if r.RelatedEmailID.Valid {
		todo.RelatedEmailID = &r.RelatedEmailID.Int64
	}
	if r.RelatedEventID.Valid {
		todo.RelatedEventID = &r.RelatedEventID.Int64
	}
	if r.CompletedAt.Valid {
		todo.CompletedAt = &r.CompletedAt.Time
	}

	return todo
}

type projectRow struct {
	ID          int64          `db:"id"`
	UserID      uuid.UUID      `db:"user_id"`
	Name        string         `db:"name"`
	Description sql.NullString `db:"description"`
	Area        sql.NullString `db:"area"`
	Color       sql.NullString `db:"color"`
	Icon        sql.NullString `db:"icon"`
	Status      string         `db:"status"`
	SortOrder   int            `db:"sort_order"`
	CreatedAt   time.Time      `db:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at"`
}

func (r *projectRow) toDomain() *domain.TodoProject {
	project := &domain.TodoProject{
		ID:        r.ID,
		UserID:    r.UserID,
		Name:      r.Name,
		Status:    r.Status,
		SortOrder: r.SortOrder,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}

	if r.Description.Valid {
		project.Description = &r.Description.String
	}
	if r.Area.Valid {
		project.Area = &r.Area.String
	}
	if r.Color.Valid {
		project.Color = &r.Color.String
	}
	if r.Icon.Valid {
		project.Icon = &r.Icon.String
	}

	return project
}
