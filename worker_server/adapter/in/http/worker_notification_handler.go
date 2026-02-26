package http

import (
	"strconv"

	"worker_server/core/domain"
	"worker_server/core/service/notification"

	"github.com/gofiber/fiber/v2"
)

// NotificationHandler handles notification requests.
type NotificationHandler struct {
	notificationService *notification.Service
}

// NewNotificationHandler creates a new notification handler.
func NewNotificationHandler(notificationService *notification.Service) *NotificationHandler {
	return &NotificationHandler{
		notificationService: notificationService,
	}
}

// Register registers notification routes.
func (h *NotificationHandler) Register(router fiber.Router) {
	notifications := router.Group("/notifications")

	notifications.Get("/", h.ListNotifications)
	notifications.Get("/unread-count", h.GetUnreadCount)
	notifications.Post("/mark-read", h.MarkAsRead)
	notifications.Post("/mark-all-read", h.MarkAllAsRead)
	notifications.Delete("/:id", h.DeleteNotification)
	notifications.Delete("/", h.DeleteAllNotifications)
}

// =============================================================================
// Handlers
// =============================================================================

// ListNotifications returns a list of notifications.
func (h *NotificationHandler) ListNotifications(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.notificationService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Notification service not available")
	}

	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	unreadOnly := c.Query("unread_only") == "true"
	notifType := c.Query("type")

	filter := &domain.NotificationFilter{
		Limit:  limit,
		Offset: offset,
	}

	if unreadOnly {
		isRead := false
		filter.IsRead = &isRead
	}

	if notifType != "" {
		t := domain.NotificationType(notifType)
		filter.Type = &t
	}

	notifications, total, err := h.notificationService.List(c.Context(), userID, filter)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{
		"notifications": notifications,
		"total":         total,
		"limit":         limit,
		"offset":        offset,
	})
}

// GetUnreadCount returns the count of unread notifications.
func (h *NotificationHandler) GetUnreadCount(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.notificationService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Notification service not available")
	}

	count, err := h.notificationService.GetUnreadCount(c.Context(), userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{
		"unread_count": count,
	})
}

// MarkAsReadRequest represents mark as read request.
type MarkAsReadRequest struct {
	NotificationIDs []int64 `json:"notification_ids"`
}

// MarkAsRead marks notifications as read.
func (h *NotificationHandler) MarkAsRead(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.notificationService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Notification service not available")
	}

	var req MarkAsReadRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if len(req.NotificationIDs) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "notification_ids is required")
	}

	if err := h.notificationService.MarkAsRead(c.Context(), userID, req.NotificationIDs); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{
		"success": true,
		"marked":  len(req.NotificationIDs),
	})
}

// MarkAllAsRead marks all notifications as read.
func (h *NotificationHandler) MarkAllAsRead(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.notificationService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Notification service not available")
	}

	if err := h.notificationService.MarkAllAsRead(c.Context(), userID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "All notifications marked as read",
	})
}

// DeleteNotification deletes a notification.
func (h *NotificationHandler) DeleteNotification(c *fiber.Ctx) error {
	if h.notificationService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Notification service not available")
	}

	notificationID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid notification ID")
	}

	if err := h.notificationService.Delete(c.Context(), notificationID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// DeleteAllNotifications deletes all notifications for the user.
func (h *NotificationHandler) DeleteAllNotifications(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if h.notificationService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Notification service not available")
	}

	if err := h.notificationService.DeleteAll(c.Context(), userID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "All notifications deleted",
	})
}
