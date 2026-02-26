package http

import (
	"strconv"
	"strings"
	"time"

	"worker_server/core/domain"
	in "worker_server/core/port/in"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// TodoHandler handles HTTP requests for todo operations
type TodoHandler struct {
	service in.TodoService
}

// NewTodoHandler creates a new TodoHandler
func NewTodoHandler(service in.TodoService) *TodoHandler {
	return &TodoHandler{service: service}
}

// Register registers todo routes
func (h *TodoHandler) Register(router fiber.Router) {
	todos := router.Group("/todos")

	// CRUD
	todos.Get("/", h.List)
	todos.Post("/", h.Create)
	todos.Get("/:id", h.Get)
	todos.Put("/:id", h.Update)
	todos.Delete("/:id", h.Delete)

	// Status operations
	todos.Post("/:id/complete", h.Complete)
	todos.Post("/:id/reopen", h.Reopen)
	todos.Put("/:id/status", h.UpdateStatus)

	// Batch operations
	todos.Post("/batch/complete", h.BatchComplete)
	todos.Delete("/batch", h.BatchDelete)

	// Views
	todos.Get("/view/inbox", h.GetInbox)
	todos.Get("/view/today", h.GetToday)
	todos.Get("/view/upcoming", h.GetUpcoming)
	todos.Get("/view/overdue", h.GetOverdue)
	todos.Get("/view/date-range", h.GetByDateRange)

	// Source-based creation
	todos.Post("/from-email", h.CreateFromEmail)
	todos.Post("/from-calendar", h.CreateFromCalendar)
	todos.Post("/from-agent", h.CreateFromAgent)

	// Subtasks
	todos.Get("/:id/subtasks", h.GetSubtasks)
	todos.Post("/:id/subtasks", h.AddSubtask)

	// Stats
	todos.Get("/stats", h.GetStats)

	// Projects
	projects := router.Group("/projects")
	projects.Get("/", h.ListProjects)
	projects.Post("/", h.CreateProject)
	projects.Get("/:id", h.GetProject)
	projects.Put("/:id", h.UpdateProject)
	projects.Delete("/:id", h.DeleteProject)
	projects.Get("/:id/todos", h.GetProjectTodos)
}

// =============================================================================
// Todo CRUD
// =============================================================================

// List lists todos with filters
// @Summary List todos
// @Tags Todos
// @Produce json
// @Param status query string false "Filter by status"
// @Param priority query int false "Filter by priority"
// @Param project_id query int false "Filter by project"
// @Param area query string false "Filter by area"
// @Param source_type query string false "Filter by source type"
// @Param has_due_date query bool false "Filter has due date"
// @Param limit query int false "Limit (default 50)"
// @Param offset query int false "Offset"
// @Success 200 {object} in.TodoListResponse
// @Router /api/v1/todos [get]
func (h *TodoHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	filter := &domain.TodoFilter{
		UserID: userID,
		Limit:  c.QueryInt("limit", 50),
		Offset: c.QueryInt("offset", 0),
	}

	if status := c.Query("status"); status != "" {
		s := domain.TodoStatus(status)
		filter.Status = &s
	}
	if priority := c.Query("priority"); priority != "" {
		p, _ := strconv.Atoi(priority)
		pr := domain.TodoPriority(p)
		filter.Priority = &pr
	}
	if projectID := c.Query("project_id"); projectID != "" {
		id, _ := strconv.ParseInt(projectID, 10, 64)
		filter.ProjectID = &id
	}
	if area := c.Query("area"); area != "" {
		filter.Area = &area
	}
	if sourceType := c.Query("source_type"); sourceType != "" {
		st := domain.TodoSourceType(sourceType)
		filter.SourceType = &st
	}
	// Note: HasDueDate filter removed - use DueDateFrom/DueDateTo instead

	resp, err := h.service.ListTodos(c.Context(), filter)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"todos":  toTodoResponses(resp.Todos),
		"total":  resp.Total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

// Create creates a new todo
// @Summary Create a new todo
// @Tags Todos
// @Accept json
// @Produce json
// @Param request body in.CreateTodoRequest true "Todo data"
// @Success 201 {object} TodoResponse
// @Router /api/v1/todos [post]
func (h *TodoHandler) Create(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	var req in.CreateTodoRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Title == "" {
		return c.Status(400).JSON(fiber.Map{"error": "title is required"})
	}

	todo, err := h.service.CreateTodo(c.Context(), userID, &req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(toTodoResponse(todo))
}

// Get retrieves a todo by ID
// @Summary Get a todo by ID
// @Tags Todos
// @Produce json
// @Param id path int true "Todo ID"
// @Success 200 {object} TodoResponse
// @Router /api/v1/todos/{id} [get]
func (h *TodoHandler) Get(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid todo ID"})
	}

	todo, err := h.service.GetTodo(c.Context(), userID, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": "todo not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(toTodoResponse(todo))
}

// Update updates a todo
// @Summary Update a todo
// @Tags Todos
// @Accept json
// @Produce json
// @Param id path int true "Todo ID"
// @Param request body in.UpdateTodoRequest true "Todo data"
// @Success 200 {object} TodoResponse
// @Router /api/v1/todos/{id} [put]
func (h *TodoHandler) Update(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid todo ID"})
	}

	var req in.UpdateTodoRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	todo, err := h.service.UpdateTodo(c.Context(), userID, id, &req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": "todo not found"})
		}
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(toTodoResponse(todo))
}

// Delete deletes a todo
// @Summary Delete a todo
// @Tags Todos
// @Param id path int true "Todo ID"
// @Success 204
// @Router /api/v1/todos/{id} [delete]
func (h *TodoHandler) Delete(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid todo ID"})
	}

	if err := h.service.DeleteTodo(c.Context(), userID, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": "todo not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(204)
}

// =============================================================================
// Status Operations
// =============================================================================

// Complete marks a todo as completed
// @Summary Complete a todo
// @Tags Todos
// @Param id path int true "Todo ID"
// @Success 200
// @Router /api/v1/todos/{id}/complete [post]
func (h *TodoHandler) Complete(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid todo ID"})
	}

	if err := h.service.CompleteTodo(c.Context(), userID, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": "todo not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "todo completed"})
}

// Reopen reopens a completed todo
// @Summary Reopen a todo
// @Tags Todos
// @Param id path int true "Todo ID"
// @Success 200
// @Router /api/v1/todos/{id}/reopen [post]
func (h *TodoHandler) Reopen(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid todo ID"})
	}

	if err := h.service.ReopenTodo(c.Context(), userID, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": "todo not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "todo reopened"})
}

// UpdateStatus updates the status of a todo
// @Summary Update todo status
// @Tags Todos
// @Accept json
// @Param id path int true "Todo ID"
// @Param request body map[string]string true "Status"
// @Success 200
// @Router /api/v1/todos/{id}/status [put]
func (h *TodoHandler) UpdateStatus(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid todo ID"})
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := h.service.UpdateStatus(c.Context(), userID, id, domain.TodoStatus(req.Status)); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": "todo not found"})
		}
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "status updated"})
}

// =============================================================================
// Batch Operations
// =============================================================================

// BatchComplete completes multiple todos
// @Summary Complete multiple todos
// @Tags Todos
// @Accept json
// @Param request body map[string][]int64 true "Todo IDs"
// @Success 200
// @Router /api/v1/todos/batch/complete [post]
func (h *TodoHandler) BatchComplete(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := h.service.CompleteTodos(c.Context(), userID, req.IDs); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "todos completed", "count": len(req.IDs)})
}

// BatchDelete deletes multiple todos
// @Summary Delete multiple todos
// @Tags Todos
// @Accept json
// @Param request body map[string][]int64 true "Todo IDs"
// @Success 204
// @Router /api/v1/todos/batch [delete]
func (h *TodoHandler) BatchDelete(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := h.service.DeleteTodos(c.Context(), userID, req.IDs); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(204)
}

// =============================================================================
// Views
// =============================================================================

// GetInbox returns todos in inbox (no project, no area)
// @Summary Get inbox todos
// @Tags Todos
// @Produce json
// @Param limit query int false "Limit (default 50)"
// @Param offset query int false "Offset"
// @Success 200 {object} in.TodoListResponse
// @Router /api/v1/todos/view/inbox [get]
func (h *TodoHandler) GetInbox(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)

	resp, err := h.service.GetInbox(c.Context(), userID, limit, offset)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"todos":  toTodoResponses(resp.Todos),
		"total":  resp.Total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetToday returns todos due today
// @Summary Get today's todos
// @Tags Todos
// @Produce json
// @Success 200 {object} in.TodoListResponse
// @Router /api/v1/todos/view/today [get]
func (h *TodoHandler) GetToday(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	resp, err := h.service.GetToday(c.Context(), userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"todos": toTodoResponses(resp.Todos),
		"total": resp.Total,
	})
}

// GetUpcoming returns upcoming todos
// @Summary Get upcoming todos
// @Tags Todos
// @Produce json
// @Param days query int false "Number of days (default 7)"
// @Success 200 {object} in.TodoListResponse
// @Router /api/v1/todos/view/upcoming [get]
func (h *TodoHandler) GetUpcoming(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	days := c.QueryInt("days", 7)

	resp, err := h.service.GetUpcoming(c.Context(), userID, days)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"todos": toTodoResponses(resp.Todos),
		"total": resp.Total,
		"days":  days,
	})
}

// GetOverdue returns overdue todos
// @Summary Get overdue todos
// @Tags Todos
// @Produce json
// @Success 200 {object} in.TodoListResponse
// @Router /api/v1/todos/view/overdue [get]
func (h *TodoHandler) GetOverdue(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	resp, err := h.service.GetOverdue(c.Context(), userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"todos": toTodoResponses(resp.Todos),
		"total": resp.Total,
	})
}

// GetByDateRange returns todos within a date range
// @Summary Get todos by date range
// @Tags Todos
// @Produce json
// @Param start query string true "Start date (RFC3339)"
// @Param end query string true "End date (RFC3339)"
// @Success 200 {object} in.TodoListResponse
// @Router /api/v1/todos/view/date-range [get]
func (h *TodoHandler) GetByDateRange(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	startStr := c.Query("start")
	endStr := c.Query("end")

	if startStr == "" || endStr == "" {
		return c.Status(400).JSON(fiber.Map{"error": "start and end dates are required"})
	}

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid start date format"})
	}

	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid end date format"})
	}

	resp, err := h.service.GetByDateRange(c.Context(), userID, start, end)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"todos": toTodoResponses(resp.Todos),
		"total": resp.Total,
		"start": start.Format(time.RFC3339),
		"end":   end.Format(time.RFC3339),
	})
}

// =============================================================================
// Source-based Creation
// =============================================================================

// CreateFromEmail creates a todo from an email
// @Summary Create todo from email
// @Tags Todos
// @Accept json
// @Produce json
// @Param request body in.CreateTodoFromEmailRequest true "Email todo data"
// @Success 201 {object} TodoResponse
// @Router /api/v1/todos/from-email [post]
func (h *TodoHandler) CreateFromEmail(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	var req in.CreateTodoFromEmailRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.EmailID == 0 || req.Title == "" {
		return c.Status(400).JSON(fiber.Map{"error": "email_id and title are required"})
	}

	todo, err := h.service.CreateFromEmail(c.Context(), userID, &req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(toTodoResponse(todo))
}

// CreateFromCalendar creates a todo from a calendar event
// @Summary Create todo from calendar event
// @Tags Todos
// @Accept json
// @Produce json
// @Param request body in.CreateTodoFromCalendarRequest true "Calendar todo data"
// @Success 201 {object} TodoResponse
// @Router /api/v1/todos/from-calendar [post]
func (h *TodoHandler) CreateFromCalendar(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	var req in.CreateTodoFromCalendarRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.EventID == 0 || req.Title == "" {
		return c.Status(400).JSON(fiber.Map{"error": "event_id and title are required"})
	}

	todo, err := h.service.CreateFromCalendar(c.Context(), userID, &req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(toTodoResponse(todo))
}

// CreateFromAgent creates a todo from AI agent extraction
// @Summary Create todo from AI agent
// @Tags Todos
// @Accept json
// @Produce json
// @Param request body in.CreateTodoFromAgentRequest true "Agent todo data"
// @Success 201 {object} TodoResponse
// @Router /api/v1/todos/from-agent [post]
func (h *TodoHandler) CreateFromAgent(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	var req in.CreateTodoFromAgentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Title == "" {
		return c.Status(400).JSON(fiber.Map{"error": "title is required"})
	}

	todo, err := h.service.CreateFromAgent(c.Context(), userID, &req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(toTodoResponse(todo))
}

// =============================================================================
// Subtasks
// =============================================================================

// GetSubtasks returns subtasks of a todo
// @Summary Get subtasks
// @Tags Todos
// @Produce json
// @Param id path int true "Parent Todo ID"
// @Success 200 {array} TodoResponse
// @Router /api/v1/todos/{id}/subtasks [get]
func (h *TodoHandler) GetSubtasks(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid todo ID"})
	}

	subtasks, err := h.service.GetSubtasks(c.Context(), userID, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"subtasks": toTodoResponses(subtasks)})
}

// AddSubtask adds a subtask to a todo
// @Summary Add subtask
// @Tags Todos
// @Accept json
// @Produce json
// @Param id path int true "Parent Todo ID"
// @Param request body in.CreateTodoRequest true "Subtask data"
// @Success 201 {object} TodoResponse
// @Router /api/v1/todos/{id}/subtasks [post]
func (h *TodoHandler) AddSubtask(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid todo ID"})
	}

	var req in.CreateTodoRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Title == "" {
		return c.Status(400).JSON(fiber.Map{"error": "title is required"})
	}

	subtask, err := h.service.AddSubtask(c.Context(), userID, id, &req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(toTodoResponse(subtask))
}

// =============================================================================
// Stats
// =============================================================================

// GetStats returns todo statistics
// @Summary Get todo statistics
// @Tags Todos
// @Produce json
// @Success 200 {object} in.TodoStats
// @Router /api/v1/todos/stats [get]
func (h *TodoHandler) GetStats(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	stats, err := h.service.GetStats(c.Context(), userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(stats)
}

// =============================================================================
// Projects
// =============================================================================

// ListProjects lists all projects
// @Summary List projects
// @Tags Projects
// @Produce json
// @Success 200 {array} ProjectResponse
// @Router /api/v1/projects [get]
func (h *TodoHandler) ListProjects(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	projects, err := h.service.ListProjects(c.Context(), userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"projects": toProjectResponses(projects)})
}

// CreateProject creates a new project
// @Summary Create a project
// @Tags Projects
// @Accept json
// @Produce json
// @Param request body in.CreateProjectRequest true "Project data"
// @Success 201 {object} ProjectResponse
// @Router /api/v1/projects [post]
func (h *TodoHandler) CreateProject(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	var req in.CreateProjectRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}

	project, err := h.service.CreateProject(c.Context(), userID, &req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(toProjectResponse(project))
}

// GetProject retrieves a project by ID
// @Summary Get a project
// @Tags Projects
// @Produce json
// @Param id path int true "Project ID"
// @Success 200 {object} ProjectResponse
// @Router /api/v1/projects/{id} [get]
func (h *TodoHandler) GetProject(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid project ID"})
	}

	project, err := h.service.GetProject(c.Context(), userID, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": "project not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(toProjectResponse(project))
}

// UpdateProject updates a project
// @Summary Update a project
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path int true "Project ID"
// @Param request body in.UpdateProjectRequest true "Project data"
// @Success 200 {object} ProjectResponse
// @Router /api/v1/projects/{id} [put]
func (h *TodoHandler) UpdateProject(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid project ID"})
	}

	var req in.UpdateProjectRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	project, err := h.service.UpdateProject(c.Context(), userID, id, &req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": "project not found"})
		}
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(toProjectResponse(project))
}

// DeleteProject deletes a project
// @Summary Delete a project
// @Tags Projects
// @Param id path int true "Project ID"
// @Success 204
// @Router /api/v1/projects/{id} [delete]
func (h *TodoHandler) DeleteProject(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid project ID"})
	}

	if err := h.service.DeleteProject(c.Context(), userID, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(404).JSON(fiber.Map{"error": "project not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(204)
}

// GetProjectTodos returns todos for a project
// @Summary Get project todos
// @Tags Projects
// @Produce json
// @Param id path int true "Project ID"
// @Success 200 {object} in.TodoListResponse
// @Router /api/v1/projects/{id}/todos [get]
func (h *TodoHandler) GetProjectTodos(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uuid.UUID)

	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid project ID"})
	}

	resp, err := h.service.GetProjectTodos(c.Context(), userID, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"todos": toTodoResponses(resp.Todos),
		"total": resp.Total,
	})
}

// =============================================================================
// Response Types
// =============================================================================

// TodoResponse represents the HTTP response for a todo
type TodoResponse struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Status      string   `json:"status"`
	Priority    int      `json:"priority"`
	ProjectID   *int64   `json:"project_id,omitempty"`
	Area        *string  `json:"area,omitempty"`
	ParentID    *int64   `json:"parent_id,omitempty"`
	DueDate     *string  `json:"due_date,omitempty"`
	DueDatetime *string  `json:"due_datetime,omitempty"`
	StartDate   *string  `json:"start_date,omitempty"`
	CompletedAt *string  `json:"completed_at,omitempty"`
	Tags        []string `json:"tags"`
	SourceType  *string  `json:"source_type,omitempty"`
	SourceID    *string  `json:"source_id,omitempty"`
	SourceURL   *string  `json:"source_url,omitempty"`
	SortOrder   int      `json:"sort_order"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// ProjectResponse represents the HTTP response for a project
type ProjectResponse struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Area        *string `json:"area,omitempty"`
	Color       *string `json:"color,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	Status      string  `json:"status"`
	SortOrder   int     `json:"sort_order"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// =============================================================================
// Helper Functions
// =============================================================================

func toTodoResponse(t *domain.Todo) TodoResponse {
	resp := TodoResponse{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Status:      string(t.Status),
		Priority:    int(t.Priority),
		ProjectID:   t.ProjectID,
		Area:        t.Area,
		ParentID:    t.ParentID,
		Tags:        t.Tags,
		SourceID:    t.SourceID,
		SourceURL:   t.SourceURL,
		SortOrder:   t.SortOrder,
		CreatedAt:   t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   t.UpdatedAt.Format(time.RFC3339),
	}

	if t.SourceType != nil {
		st := string(*t.SourceType)
		resp.SourceType = &st
	}

	if t.DueDate != nil {
		formatted := t.DueDate.Format("2006-01-02")
		resp.DueDate = &formatted
	}
	if t.DueDatetime != nil {
		formatted := t.DueDatetime.Format(time.RFC3339)
		resp.DueDatetime = &formatted
	}
	if t.StartDate != nil {
		formatted := t.StartDate.Format("2006-01-02")
		resp.StartDate = &formatted
	}
	if t.CompletedAt != nil {
		formatted := t.CompletedAt.Format(time.RFC3339)
		resp.CompletedAt = &formatted
	}

	if resp.Tags == nil {
		resp.Tags = []string{}
	}

	return resp
}

func toTodoResponses(todos []*domain.Todo) []TodoResponse {
	if todos == nil {
		return []TodoResponse{}
	}

	responses := make([]TodoResponse, len(todos))
	for i, t := range todos {
		responses[i] = toTodoResponse(t)
	}
	return responses
}

func toProjectResponse(p *domain.TodoProject) ProjectResponse {
	resp := ProjectResponse{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Area:        p.Area,
		Color:       p.Color,
		Icon:        p.Icon,
		Status:      p.Status,
		SortOrder:   p.SortOrder,
		CreatedAt:   p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   p.UpdatedAt.Format(time.RFC3339),
	}

	return resp
}

func toProjectResponses(projects []*domain.TodoProject) []ProjectResponse {
	if projects == nil {
		return []ProjectResponse{}
	}

	responses := make([]ProjectResponse, len(projects))
	for i, p := range projects {
		responses[i] = toProjectResponse(p)
	}
	return responses
}
