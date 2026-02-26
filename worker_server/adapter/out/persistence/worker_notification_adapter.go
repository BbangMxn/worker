package persistence

import (
	"database/sql"
	"time"

	"github.com/goccy/go-json"

	"worker_server/core/domain"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// NotificationAdapter implements domain.NotificationRepository using PostgreSQL.
type NotificationAdapter struct {
	db *sqlx.DB
}

// NewNotificationAdapter creates a new notification adapter.
func NewNotificationAdapter(db *sqlx.DB) *NotificationAdapter {
	return &NotificationAdapter{db: db}
}

// notificationRow represents the database row.
type notificationRow struct {
	ID         int64          `db:"id"`
	UserID     uuid.UUID      `db:"user_id"`
	Type       string         `db:"type"`
	Title      string         `db:"title"`
	Body       sql.NullString `db:"body"`
	Data       []byte         `db:"data"`
	EntityType sql.NullString `db:"entity_type"`
	EntityID   sql.NullInt64  `db:"entity_id"`
	IsRead     bool           `db:"is_read"`
	ReadAt     sql.NullTime   `db:"read_at"`
	Priority   string         `db:"priority"`
	CreatedAt  time.Time      `db:"created_at"`
	ExpiresAt  sql.NullTime   `db:"expires_at"`
}

func (r *notificationRow) toDomain() *domain.Notification {
	n := &domain.Notification{
		ID:        r.ID,
		UserID:    r.UserID,
		Type:      domain.NotificationType(r.Type),
		Title:     r.Title,
		IsRead:    r.IsRead,
		Priority:  domain.NotificationPriority(r.Priority),
		CreatedAt: r.CreatedAt,
	}

	if r.Body.Valid {
		n.Body = r.Body.String
	}
	if r.EntityType.Valid {
		n.EntityType = r.EntityType.String
	}
	if r.EntityID.Valid {
		n.EntityID = r.EntityID.Int64
	}
	if r.ReadAt.Valid {
		n.ReadAt = &r.ReadAt.Time
	}
	if r.ExpiresAt.Valid {
		n.ExpiresAt = &r.ExpiresAt.Time
	}
	if len(r.Data) > 0 {
		json.Unmarshal(r.Data, &n.Data)
	}

	return n
}

// Create creates a new notification.
func (a *NotificationAdapter) Create(notification *domain.Notification) error {
	var dataBytes []byte
	if notification.Data != nil {
		dataBytes, _ = json.Marshal(notification.Data)
	}

	query := `
		INSERT INTO notifications (user_id, type, title, body, data, entity_type, entity_id, is_read, priority, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		RETURNING id, created_at
	`

	var body, entityType sql.NullString
	var entityID sql.NullInt64
	var expiresAt sql.NullTime

	if notification.Body != "" {
		body = sql.NullString{String: notification.Body, Valid: true}
	}
	if notification.EntityType != "" {
		entityType = sql.NullString{String: notification.EntityType, Valid: true}
	}
	if notification.EntityID != 0 {
		entityID = sql.NullInt64{Int64: notification.EntityID, Valid: true}
	}
	if notification.ExpiresAt != nil {
		expiresAt = sql.NullTime{Time: *notification.ExpiresAt, Valid: true}
	}

	priority := string(notification.Priority)
	if priority == "" {
		priority = string(domain.NotificationPriorityNormal)
	}

	return a.db.QueryRow(
		query,
		notification.UserID,
		string(notification.Type),
		notification.Title,
		body,
		dataBytes,
		entityType,
		entityID,
		notification.IsRead,
		priority,
		expiresAt,
	).Scan(&notification.ID, &notification.CreatedAt)
}

// GetByID retrieves a notification by ID.
func (a *NotificationAdapter) GetByID(id int64) (*domain.Notification, error) {
	query := `SELECT * FROM notifications WHERE id = $1`

	var row notificationRow
	if err := a.db.Get(&row, query, id); err != nil {
		return nil, err
	}

	return row.toDomain(), nil
}

// List lists notifications with filter.
func (a *NotificationAdapter) List(filter *domain.NotificationFilter) ([]*domain.Notification, int, error) {
	baseQuery := `FROM notifications WHERE user_id = $1`
	args := []any{filter.UserID}
	argIndex := 2

	if filter.Type != nil {
		baseQuery += ` AND type = $` + string(rune('0'+argIndex))
		args = append(args, string(*filter.Type))
		argIndex++
	}
	if filter.IsRead != nil {
		baseQuery += ` AND is_read = $` + string(rune('0'+argIndex))
		args = append(args, *filter.IsRead)
		argIndex++
	}
	if filter.Priority != nil {
		baseQuery += ` AND priority = $` + string(rune('0'+argIndex))
		args = append(args, string(*filter.Priority))
		argIndex++
	}

	// Count total
	var total int
	countQuery := `SELECT COUNT(*) ` + baseQuery
	if err := a.db.Get(&total, countQuery, args...); err != nil {
		return nil, 0, err
	}

	// Get results
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset

	selectQuery := `SELECT * ` + baseQuery + ` ORDER BY created_at DESC LIMIT $` + string(rune('0'+argIndex)) + ` OFFSET $` + string(rune('0'+argIndex+1))
	args = append(args, limit, offset)

	var rows []notificationRow
	if err := a.db.Select(&rows, selectQuery, args...); err != nil {
		return nil, 0, err
	}

	notifications := make([]*domain.Notification, len(rows))
	for i, row := range rows {
		notifications[i] = row.toDomain()
	}

	return notifications, total, nil
}

// Delete deletes a notification.
func (a *NotificationAdapter) Delete(id int64) error {
	_, err := a.db.Exec(`DELETE FROM notifications WHERE id = $1`, id)
	return err
}

// DeleteByUserID deletes all notifications for a user.
func (a *NotificationAdapter) DeleteByUserID(userID uuid.UUID) error {
	_, err := a.db.Exec(`DELETE FROM notifications WHERE user_id = $1`, userID)
	return err
}

// MarkAsRead marks a notification as read.
func (a *NotificationAdapter) MarkAsRead(id int64) error {
	_, err := a.db.Exec(`UPDATE notifications SET is_read = true, read_at = NOW() WHERE id = $1`, id)
	return err
}

// MarkAsReadBatch marks multiple notifications as read.
func (a *NotificationAdapter) MarkAsReadBatch(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	query, args, err := sqlx.In(`UPDATE notifications SET is_read = true, read_at = NOW() WHERE id IN (?)`, ids)
	if err != nil {
		return err
	}

	query = a.db.Rebind(query)
	_, err = a.db.Exec(query, args...)
	return err
}

// MarkAllAsRead marks all notifications as read for a user.
func (a *NotificationAdapter) MarkAllAsRead(userID uuid.UUID) error {
	_, err := a.db.Exec(`UPDATE notifications SET is_read = true, read_at = NOW() WHERE user_id = $1 AND is_read = false`, userID)
	return err
}

// GetUnreadCount returns the count of unread notifications.
func (a *NotificationAdapter) GetUnreadCount(userID uuid.UUID) (int, error) {
	var count int
	err := a.db.Get(&count, `SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false`, userID)
	return count, err
}

// CountUnread returns the count of unread notifications (interface compliance).
func (a *NotificationAdapter) CountUnread(userID uuid.UUID) (int64, error) {
	var count int64
	err := a.db.Get(&count, `SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false`, userID)
	return count, err
}

// DeleteExpired deletes expired notifications.
func (a *NotificationAdapter) DeleteExpired() (int64, error) {
	result, err := a.db.Exec(`DELETE FROM notifications WHERE expires_at IS NOT NULL AND expires_at < NOW()`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteOlderThan deletes notifications older than specified time.
func (a *NotificationAdapter) DeleteOlderThan(before time.Time) (int64, error) {
	result, err := a.db.Exec(`DELETE FROM notifications WHERE created_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Ensure interface compliance
var _ domain.NotificationRepository = (*NotificationAdapter)(nil)
