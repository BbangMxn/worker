package todo

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/in"
	"worker_server/core/port/out"
	"worker_server/pkg/snowflake"

	"github.com/google/uuid"
)

var (
	ErrTodoNotFound    = errors.New("todo not found")
	ErrProjectNotFound = errors.New("project not found")
	ErrUnauthorized    = errors.New("unauthorized access")
)

// Service implements in.TodoService
type Service struct {
	todoRepo     out.TodoRepository
	reminderRepo out.ReminderRepository
}

// NewService creates a new TodoService
func NewService(todoRepo out.TodoRepository, reminderRepo out.ReminderRepository) in.TodoService {
	return &Service{
		todoRepo:     todoRepo,
		reminderRepo: reminderRepo,
	}
}

// =============================================================================
// Todo CRUD
// =============================================================================

func (s *Service) GetTodo(ctx context.Context, userID uuid.UUID, todoID int64) (*domain.Todo, error) {
	todo, err := s.todoRepo.GetTodo(ctx, todoID)
	if err != nil {
		return nil, fmt.Errorf("get todo: %w", err)
	}
	if todo == nil {
		return nil, ErrTodoNotFound
	}
	if todo.UserID != userID {
		return nil, ErrUnauthorized
	}
	return todo, nil
}

func (s *Service) ListTodos(ctx context.Context, filter *domain.TodoFilter) (*in.TodoListResponse, error) {
	todos, total, err := s.todoRepo.ListTodos(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list todos: %w", err)
	}
	return &in.TodoListResponse{
		Todos: todos,
		Total: total,
	}, nil
}

func (s *Service) CreateTodo(ctx context.Context, userID uuid.UUID, req *in.CreateTodoRequest) (*domain.Todo, error) {
	priority := domain.TodoPriorityNormal
	if req.Priority != nil {
		priority = *req.Priority
	}

	todo := &domain.Todo{
		ID:          snowflake.ID(),
		UserID:      userID,
		ProjectID:   req.ProjectID,
		Area:        req.Area,
		ParentID:    req.ParentID,
		Title:       req.Title,
		Description: req.Description,
		Status:      domain.TodoStatusInbox,
		Priority:    priority,
		DueDate:     req.DueDate,
		DueDatetime: req.DueDatetime,
		StartDate:   req.StartDate,
		Tags:        req.Tags,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// If has project or area, set to pending instead of inbox
	if req.ProjectID != nil || req.Area != nil {
		todo.Status = domain.TodoStatusPending
	}

	if err := s.todoRepo.CreateTodo(ctx, todo); err != nil {
		return nil, fmt.Errorf("create todo: %w", err)
	}

	// Create reminders based on rules
	if err := s.createRemindersForTodo(ctx, todo); err != nil {
		// Log but don't fail
		fmt.Printf("failed to create reminders for todo %d: %v\n", todo.ID, err)
	}

	return todo, nil
}

func (s *Service) UpdateTodo(ctx context.Context, userID uuid.UUID, todoID int64, req *in.UpdateTodoRequest) (*domain.Todo, error) {
	todo, err := s.GetTodo(ctx, userID, todoID)
	if err != nil {
		return nil, err
	}

	if req.Title != nil {
		todo.Title = *req.Title
	}
	if req.Description != nil {
		todo.Description = *req.Description
	}
	if req.ProjectID != nil {
		todo.ProjectID = req.ProjectID
	}
	if req.Area != nil {
		todo.Area = req.Area
	}
	if req.Priority != nil {
		todo.Priority = *req.Priority
	}
	if req.Status != nil {
		todo.Status = *req.Status
	}
	if req.DueDate != nil {
		todo.DueDate = req.DueDate
	}
	if req.DueDatetime != nil {
		todo.DueDatetime = req.DueDatetime
	}
	if req.StartDate != nil {
		todo.StartDate = req.StartDate
	}
	if req.Tags != nil {
		todo.Tags = req.Tags
	}
	if req.SortOrder != nil {
		todo.SortOrder = *req.SortOrder
	}

	todo.UpdatedAt = time.Now()

	if err := s.todoRepo.UpdateTodo(ctx, todo); err != nil {
		return nil, fmt.Errorf("update todo: %w", err)
	}

	return todo, nil
}

func (s *Service) DeleteTodo(ctx context.Context, userID uuid.UUID, todoID int64) error {
	todo, err := s.GetTodo(ctx, userID, todoID)
	if err != nil {
		return err
	}

	// Delete related reminders
	sourceID := strconv.FormatInt(todo.ID, 10)
	if err := s.reminderRepo.DeleteRemindersBySource(ctx, domain.ReminderSourceTodo, sourceID); err != nil {
		fmt.Printf("failed to delete reminders for todo %d: %v\n", todoID, err)
	}

	return s.todoRepo.DeleteTodo(ctx, todoID)
}

// =============================================================================
// Status Operations
// =============================================================================

func (s *Service) CompleteTodo(ctx context.Context, userID uuid.UUID, todoID int64) error {
	if _, err := s.GetTodo(ctx, userID, todoID); err != nil {
		return err
	}

	if err := s.todoRepo.CompleteTodo(ctx, todoID); err != nil {
		return fmt.Errorf("complete todo: %w", err)
	}

	// Cancel pending reminders
	sourceID := strconv.FormatInt(todoID, 10)
	reminders, _ := s.reminderRepo.GetRemindersBySource(ctx, domain.ReminderSourceTodo, sourceID)
	for _, r := range reminders {
		if r.Status == domain.ReminderStatusPending {
			s.reminderRepo.CancelReminder(ctx, r.ID)
		}
	}

	return nil
}

func (s *Service) ReopenTodo(ctx context.Context, userID uuid.UUID, todoID int64) error {
	if _, err := s.GetTodo(ctx, userID, todoID); err != nil {
		return err
	}
	return s.todoRepo.ReopenTodo(ctx, todoID)
}

func (s *Service) UpdateStatus(ctx context.Context, userID uuid.UUID, todoID int64, status domain.TodoStatus) error {
	if _, err := s.GetTodo(ctx, userID, todoID); err != nil {
		return err
	}
	return s.todoRepo.UpdateTodoStatus(ctx, todoID, status)
}

// =============================================================================
// Batch Operations
// =============================================================================

func (s *Service) CompleteTodos(ctx context.Context, userID uuid.UUID, todoIDs []int64) error {
	for _, id := range todoIDs {
		if err := s.CompleteTodo(ctx, userID, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) DeleteTodos(ctx context.Context, userID uuid.UUID, todoIDs []int64) error {
	for _, id := range todoIDs {
		if err := s.DeleteTodo(ctx, userID, id); err != nil {
			return err
		}
	}
	return nil
}

// =============================================================================
// Source-based Creation
// =============================================================================

func (s *Service) CreateFromEmail(ctx context.Context, userID uuid.UUID, req *in.CreateTodoFromEmailRequest) (*domain.Todo, error) {
	priority := domain.TodoPriorityNormal
	if req.Priority != nil {
		priority = *req.Priority
	}

	sourceType := domain.TodoSourceEmail
	sourceID := strconv.FormatInt(req.EmailID, 10)

	todo := &domain.Todo{
		ID:             snowflake.ID(),
		UserID:         userID,
		Title:          req.Title,
		Description:    req.Description,
		Status:         domain.TodoStatusPending,
		Priority:       priority,
		DueDate:        req.DueDate,
		SourceType:     &sourceType,
		SourceID:       &sourceID,
		SourceURL:      &req.SourceURL,
		RelatedEmailID: &req.EmailID,
		SourceMetadata: map[string]interface{}{
			"action_type": req.ActionType,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.todoRepo.CreateTodo(ctx, todo); err != nil {
		return nil, fmt.Errorf("create todo from email: %w", err)
	}

	s.createRemindersForTodo(ctx, todo)
	return todo, nil
}

func (s *Service) CreateFromCalendar(ctx context.Context, userID uuid.UUID, req *in.CreateTodoFromCalendarRequest) (*domain.Todo, error) {
	priority := domain.TodoPriorityNormal
	if req.Priority != nil {
		priority = *req.Priority
	}

	sourceType := domain.TodoSourceCalendar
	sourceID := strconv.FormatInt(req.EventID, 10)

	todo := &domain.Todo{
		ID:             snowflake.ID(),
		UserID:         userID,
		Title:          req.Title,
		Description:    req.Description,
		Status:         domain.TodoStatusPending,
		Priority:       priority,
		DueDate:        req.DueDate,
		SourceType:     &sourceType,
		SourceID:       &sourceID,
		RelatedEventID: &req.EventID,
		SourceMetadata: map[string]interface{}{
			"prep_time": req.PrepTime,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.todoRepo.CreateTodo(ctx, todo); err != nil {
		return nil, fmt.Errorf("create todo from calendar: %w", err)
	}

	s.createRemindersForTodo(ctx, todo)
	return todo, nil
}

func (s *Service) CreateFromAgent(ctx context.Context, userID uuid.UUID, req *in.CreateTodoFromAgentRequest) (*domain.Todo, error) {
	priority := domain.TodoPriorityNormal
	if req.Priority != nil {
		priority = *req.Priority
	}

	sourceType := domain.TodoSourceAgent

	todo := &domain.Todo{
		ID:          snowflake.ID(),
		UserID:      userID,
		Title:       req.Title,
		Description: req.Description,
		Status:      domain.TodoStatusPending,
		Priority:    priority,
		DueDate:     req.DueDate,
		DueDatetime: req.DueDatetime,
		SourceType:  &sourceType,
		SourceMetadata: map[string]interface{}{
			"extracted_from": req.ExtractedFrom,
			"confidence":     req.Confidence,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.todoRepo.CreateTodo(ctx, todo); err != nil {
		return nil, fmt.Errorf("create todo from agent: %w", err)
	}

	s.createRemindersForTodo(ctx, todo)
	return todo, nil
}

// =============================================================================
// Subtasks
// =============================================================================

func (s *Service) GetSubtasks(ctx context.Context, userID uuid.UUID, parentID int64) ([]*domain.Todo, error) {
	// Verify parent ownership
	if _, err := s.GetTodo(ctx, userID, parentID); err != nil {
		return nil, err
	}
	return s.todoRepo.GetSubtasks(ctx, parentID)
}

func (s *Service) AddSubtask(ctx context.Context, userID uuid.UUID, parentID int64, req *in.CreateTodoRequest) (*domain.Todo, error) {
	// Verify parent ownership
	parent, err := s.GetTodo(ctx, userID, parentID)
	if err != nil {
		return nil, err
	}

	// Inherit project and area from parent
	req.ParentID = &parentID
	if req.ProjectID == nil {
		req.ProjectID = parent.ProjectID
	}
	if req.Area == nil {
		req.Area = parent.Area
	}

	return s.CreateTodo(ctx, userID, req)
}

// =============================================================================
// Views
// =============================================================================

func (s *Service) GetInbox(ctx context.Context, userID uuid.UUID, limit, offset int) (*in.TodoListResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.ListTodos(ctx, &domain.TodoFilter{
		UserID:   userID,
		ViewType: "inbox",
		Limit:    limit,
		Offset:   offset,
	})
}

func (s *Service) GetToday(ctx context.Context, userID uuid.UUID) (*in.TodoListResponse, error) {
	return s.ListTodos(ctx, &domain.TodoFilter{
		UserID:   userID,
		ViewType: "today",
		SortBy:   "priority",
		Limit:    100,
	})
}

func (s *Service) GetUpcoming(ctx context.Context, userID uuid.UUID, days int) (*in.TodoListResponse, error) {
	if days <= 0 {
		days = 7
	}
	end := time.Now().AddDate(0, 0, days)
	return s.ListTodos(ctx, &domain.TodoFilter{
		UserID:    userID,
		ViewType:  "upcoming",
		DueDateTo: &end,
		SortBy:    "due_date",
		Limit:     100,
	})
}

func (s *Service) GetOverdue(ctx context.Context, userID uuid.UUID) (*in.TodoListResponse, error) {
	return s.ListTodos(ctx, &domain.TodoFilter{
		UserID:  userID,
		Overdue: true,
		SortBy:  "due_date",
		Limit:   100,
	})
}

func (s *Service) GetByDateRange(ctx context.Context, userID uuid.UUID, start, end time.Time) (*in.TodoListResponse, error) {
	return s.ListTodos(ctx, &domain.TodoFilter{
		UserID:      userID,
		DueDateFrom: &start,
		DueDateTo:   &end,
		Statuses: []domain.TodoStatus{
			domain.TodoStatusPending,
			domain.TodoStatusInProgress,
			domain.TodoStatusWaiting,
		},
		SortBy: "due_date",
		Limit:  200,
	})
}

// =============================================================================
// Project Operations
// =============================================================================

func (s *Service) GetProject(ctx context.Context, userID uuid.UUID, projectID int64) (*domain.TodoProject, error) {
	project, err := s.todoRepo.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if project == nil {
		return nil, ErrProjectNotFound
	}
	if project.UserID != userID {
		return nil, ErrUnauthorized
	}
	return project, nil
}

func (s *Service) ListProjects(ctx context.Context, userID uuid.UUID) ([]*domain.TodoProject, error) {
	projects, _, err := s.todoRepo.ListProjects(ctx, &domain.TodoProjectFilter{
		UserID: userID,
		Limit:  100,
	})
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return projects, nil
}

func (s *Service) CreateProject(ctx context.Context, userID uuid.UUID, req *in.CreateProjectRequest) (*domain.TodoProject, error) {
	project := &domain.TodoProject{
		ID:          snowflake.ID(),
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		Area:        req.Area,
		Color:       req.Color,
		Icon:        req.Icon,
		Status:      "active",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.todoRepo.CreateProject(ctx, project); err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}

	return project, nil
}

func (s *Service) UpdateProject(ctx context.Context, userID uuid.UUID, projectID int64, req *in.UpdateProjectRequest) (*domain.TodoProject, error) {
	project, err := s.GetProject(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		project.Name = *req.Name
	}
	if req.Description != nil {
		project.Description = req.Description
	}
	if req.Area != nil {
		project.Area = req.Area
	}
	if req.Color != nil {
		project.Color = req.Color
	}
	if req.Icon != nil {
		project.Icon = req.Icon
	}
	if req.Status != nil {
		project.Status = *req.Status
	}
	if req.SortOrder != nil {
		project.SortOrder = *req.SortOrder
	}

	project.UpdatedAt = time.Now()

	if err := s.todoRepo.UpdateProject(ctx, project); err != nil {
		return nil, fmt.Errorf("update project: %w", err)
	}

	return project, nil
}

func (s *Service) DeleteProject(ctx context.Context, userID uuid.UUID, projectID int64) error {
	if _, err := s.GetProject(ctx, userID, projectID); err != nil {
		return err
	}
	return s.todoRepo.DeleteProject(ctx, projectID)
}

func (s *Service) GetProjectTodos(ctx context.Context, userID uuid.UUID, projectID int64) (*in.TodoListResponse, error) {
	// Verify ownership
	if _, err := s.GetProject(ctx, userID, projectID); err != nil {
		return nil, err
	}

	return s.ListTodos(ctx, &domain.TodoFilter{
		UserID:    userID,
		ProjectID: &projectID,
		Statuses: []domain.TodoStatus{
			domain.TodoStatusPending,
			domain.TodoStatusInProgress,
			domain.TodoStatusWaiting,
		},
		SortBy: "sort_order",
		Limit:  200,
	})
}

// =============================================================================
// Stats
// =============================================================================

func (s *Service) GetStats(ctx context.Context, userID uuid.UUID) (*in.TodoStats, error) {
	statusCounts, err := s.todoRepo.CountTodosByStatus(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("count by status: %w", err)
	}

	overdueCount, err := s.todoRepo.CountOverdueTodos(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("count overdue: %w", err)
	}

	// Calculate today count
	todayResp, _ := s.GetToday(ctx, userID)
	todayCount := 0
	if todayResp != nil {
		todayCount = todayResp.Total
	}

	// Calculate upcoming count
	upcomingResp, _ := s.GetUpcoming(ctx, userID, 7)
	upcomingCount := 0
	if upcomingResp != nil {
		upcomingCount = upcomingResp.Total
	}

	// Calculate inbox count
	inboxResp, _ := s.GetInbox(ctx, userID, 1, 0)
	inboxCount := 0
	if inboxResp != nil {
		inboxCount = inboxResp.Total
	}

	total := 0
	byStatus := make(map[string]int)
	for status, count := range statusCounts {
		byStatus[string(status)] = count
		total += count
	}

	return &in.TodoStats{
		Total:     total,
		Inbox:     inboxCount,
		Today:     todayCount,
		Upcoming:  upcomingCount,
		Overdue:   overdueCount,
		Completed: byStatus["completed"],
		ByStatus:  byStatus,
	}, nil
}

// =============================================================================
// Related Queries (for Calendar integration)
// =============================================================================

func (s *Service) GetTodosByEmail(ctx context.Context, emailID int64) ([]*domain.Todo, error) {
	return s.todoRepo.GetTodosByEmail(ctx, emailID)
}

func (s *Service) GetTodosByEvent(ctx context.Context, eventID int64) ([]*domain.Todo, error) {
	return s.todoRepo.GetTodosByEvent(ctx, eventID)
}

// =============================================================================
// Internal Helpers
// =============================================================================

func (s *Service) createRemindersForTodo(ctx context.Context, todo *domain.Todo) error {
	if s.reminderRepo == nil {
		return nil
	}

	// Skip if no due date
	if todo.DueDate == nil && todo.DueDatetime == nil {
		return nil
	}

	// Get enabled rules for this user
	rules, err := s.reminderRepo.GetEnabledRules(ctx, todo.UserID, domain.ReminderSourceTodo)
	if err != nil {
		return err
	}

	sourceID := strconv.FormatInt(todo.ID, 10)

	for _, rule := range rules {
		// Check if rule matches this todo
		if !rule.MatchesConditions(todo) {
			continue
		}

		// Calculate remind_at based on trigger type
		var remindAt time.Time
		switch rule.TriggerType {
		case domain.ReminderTriggerBeforeDue:
			if rule.OffsetMinutes == nil {
				continue
			}
			dueTime := todo.DueDatetime
			if dueTime == nil && todo.DueDate != nil {
				// Use end of due date
				t := time.Date(todo.DueDate.Year(), todo.DueDate.Month(), todo.DueDate.Day(), 23, 59, 0, 0, todo.DueDate.Location())
				dueTime = &t
			}
			if dueTime == nil {
				continue
			}
			remindAt = dueTime.Add(-time.Duration(*rule.OffsetMinutes) * time.Minute)

		default:
			continue
		}

		// Skip if remind time is in the past
		if remindAt.Before(time.Now()) {
			continue
		}

		reminder := &domain.Reminder{
			ID:         snowflake.ID(),
			UserID:     todo.UserID,
			RuleID:     &rule.ID,
			SourceType: domain.ReminderSourceTodo,
			SourceID:   &sourceID,
			Title:      todo.Title,
			RemindAt:   remindAt,
			Timezone:   "UTC",
			Channels:   rule.Channels,
			Status:     domain.ReminderStatusPending,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := s.reminderRepo.CreateReminder(ctx, reminder); err != nil {
			return err
		}
	}

	return nil
}
