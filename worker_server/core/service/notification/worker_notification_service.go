package notification

import (
	"context"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

// Service handles notification operations.
type Service struct {
	notificationRepo         domain.NotificationRepository
	notificationSettingsRepo domain.NotificationSettingsRepository // 알림 설정 확인
	sseHub                   SSEHub                                // Interface for SSE push
}

// SSEHub interface for pushing real-time notifications.
type SSEHub interface {
	SendToUser(userID string, event string, data any)
}

// NewService creates a new notification service.
func NewService(notificationRepo domain.NotificationRepository, sseHub SSEHub) *Service {
	return &Service{
		notificationRepo: notificationRepo,
		sseHub:           sseHub,
	}
}

// NewServiceWithSettings creates a notification service with settings support.
func NewServiceWithSettings(notificationRepo domain.NotificationRepository, notificationSettingsRepo domain.NotificationSettingsRepository, sseHub SSEHub) *Service {
	return &Service{
		notificationRepo:         notificationRepo,
		notificationSettingsRepo: notificationSettingsRepo,
		sseHub:                   sseHub,
	}
}

// Send creates and pushes a notification.
func (s *Service) Send(ctx context.Context, notification *domain.Notification) error {
	if s.notificationRepo == nil {
		return nil
	}

	// Save to database
	if err := s.notificationRepo.Create(notification); err != nil {
		return err
	}

	// Push via SSE if hub is available
	if s.sseHub != nil {
		s.sseHub.SendToUser(notification.UserID.String(), "notification", notification)
	}

	return nil
}

// SendWithCheck creates and pushes a notification after checking user settings.
// senderEmail and threadID are optional (pass empty string and 0 if not applicable).
func (s *Service) SendWithCheck(ctx context.Context, notification *domain.Notification, senderEmail string, threadID int64) error {
	if s.notificationRepo == nil {
		return nil
	}

	// 사용자 알림 설정 확인
	if s.notificationSettingsRepo != nil {
		settings, err := s.notificationSettingsRepo.Get(ctx, notification.UserID)
		if err == nil && settings != nil {
			// 설정에 따라 알림을 보낼지 결정
			if !settings.ShouldNotify(notification.Type, notification.Priority, senderEmail, threadID) {
				return nil // 설정에 의해 알림 차단됨
			}
		}
	}

	// Save to database
	if err := s.notificationRepo.Create(notification); err != nil {
		return err
	}

	// Push via SSE if hub is available
	if s.sseHub != nil {
		s.sseHub.SendToUser(notification.UserID.String(), "notification", notification)
	}

	return nil
}

// SendEmailNotification sends an email-related notification.
func (s *Service) SendEmailNotification(ctx context.Context, userID uuid.UUID, emailID int64, title, body string, priority domain.NotificationPriority) error {
	notification := &domain.Notification{
		UserID:     userID,
		Type:       domain.NotificationTypeEmail,
		Title:      title,
		Body:       body,
		EntityType: "email",
		EntityID:   emailID,
		Priority:   priority,
	}
	return s.Send(ctx, notification)
}

// SendEmailNotificationWithSender sends an email notification with sender check.
func (s *Service) SendEmailNotificationWithSender(ctx context.Context, userID uuid.UUID, emailID int64, title, body string, priority domain.NotificationPriority, senderEmail string, threadID int64) error {
	notification := &domain.Notification{
		UserID:     userID,
		Type:       domain.NotificationTypeEmail,
		Title:      title,
		Body:       body,
		EntityType: "email",
		EntityID:   emailID,
		Priority:   priority,
	}
	return s.SendWithCheck(ctx, notification, senderEmail, threadID)
}

// SendCalendarNotification sends a calendar-related notification.
func (s *Service) SendCalendarNotification(ctx context.Context, userID uuid.UUID, eventID int64, title, body string) error {
	notification := &domain.Notification{
		UserID:     userID,
		Type:       domain.NotificationTypeCalendar,
		Title:      title,
		Body:       body,
		EntityType: "calendar",
		EntityID:   eventID,
		Priority:   domain.NotificationPriorityNormal,
	}
	return s.Send(ctx, notification)
}

// SendSystemNotification sends a system notification.
func (s *Service) SendSystemNotification(ctx context.Context, userID uuid.UUID, title, body string) error {
	notification := &domain.Notification{
		UserID:   userID,
		Type:     domain.NotificationTypeSystem,
		Title:    title,
		Body:     body,
		Priority: domain.NotificationPriorityNormal,
	}
	return s.Send(ctx, notification)
}

// List returns notifications for a user.
func (s *Service) List(ctx context.Context, userID uuid.UUID, filter *domain.NotificationFilter) ([]*domain.Notification, int, error) {
	if s.notificationRepo == nil {
		return []*domain.Notification{}, 0, nil
	}

	filter.UserID = userID
	return s.notificationRepo.List(filter)
}

// GetUnreadCount returns the count of unread notifications.
func (s *Service) GetUnreadCount(ctx context.Context, userID uuid.UUID) (int64, error) {
	if s.notificationRepo == nil {
		return 0, nil
	}
	return s.notificationRepo.CountUnread(userID)
}

// MarkAsRead marks specific notifications as read.
func (s *Service) MarkAsRead(ctx context.Context, userID uuid.UUID, notificationIDs []int64) error {
	if s.notificationRepo == nil {
		return nil
	}
	for _, id := range notificationIDs {
		if err := s.notificationRepo.MarkAsRead(id); err != nil {
			return err
		}
	}
	return nil
}

// MarkAllAsRead marks all notifications as read for a user.
func (s *Service) MarkAllAsRead(ctx context.Context, userID uuid.UUID) error {
	if s.notificationRepo == nil {
		return nil
	}
	return s.notificationRepo.MarkAllAsRead(userID)
}

// Delete deletes a notification.
func (s *Service) Delete(ctx context.Context, notificationID int64) error {
	if s.notificationRepo == nil {
		return nil
	}
	return s.notificationRepo.Delete(notificationID)
}

// DeleteAll deletes all notifications for a user.
func (s *Service) DeleteAll(ctx context.Context, userID uuid.UUID) error {
	if s.notificationRepo == nil {
		return nil
	}
	return s.notificationRepo.DeleteByUserID(userID)
}
