package http

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/goccy/go-json"
	"github.com/redis/go-redis/v9"

	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/core/service/auth"
	"worker_server/core/service/email"
	"worker_server/core/service/notification"
	"worker_server/pkg/logger"

	"github.com/gofiber/fiber/v2"
)

const (
	IdempotencyTTL = 5 * time.Minute
	SyncLockTTL    = 2 * time.Minute
)

type WebhookMetrics struct {
	Processed  int64
	Duplicates int64
	Errors     int64
	Queued     int64
	Direct     int64
}

type WebhookHandler struct {
	oauthService     *auth.OAuthService
	webhookService   *notification.WebhookService
	mailSyncService  *mail.SyncService
	messageProducer  out.MessageProducer
	realtime         out.RealtimePort
	syncRepo         out.SyncStateRepository
	calendarSyncRepo out.CalendarSyncRepository
	redis            *redis.Client
	metrics          WebhookMetrics
}

func NewWebhookHandler(
	oauthService *auth.OAuthService,
	webhookService *notification.WebhookService,
	mailSyncService *mail.SyncService,
	messageProducer out.MessageProducer,
	realtime out.RealtimePort,
	syncRepo out.SyncStateRepository,
	redisClient *redis.Client,
) *WebhookHandler {
	return &WebhookHandler{
		oauthService:    oauthService,
		webhookService:  webhookService,
		mailSyncService: mailSyncService,
		messageProducer: messageProducer,
		realtime:        realtime,
		syncRepo:        syncRepo,
		redis:           redisClient,
	}
}

func (h *WebhookHandler) SetCalendarSyncRepo(repo out.CalendarSyncRepository) {
	h.calendarSyncRepo = repo
}

func (h *WebhookHandler) GetMetrics() WebhookMetrics {
	return WebhookMetrics{
		Processed:  atomic.LoadInt64(&h.metrics.Processed),
		Duplicates: atomic.LoadInt64(&h.metrics.Duplicates),
		Errors:     atomic.LoadInt64(&h.metrics.Errors),
		Queued:     atomic.LoadInt64(&h.metrics.Queued),
		Direct:     atomic.LoadInt64(&h.metrics.Direct),
	}
}

func (h *WebhookHandler) Register(app *fiber.App) {
	app.Post("/webhook/gmail", h.GmailWebhook)
	app.Post("/webhooks/gmail", h.GmailWebhook)
	app.Post("/api/v1/webhook/gmail", h.GmailWebhook)
	app.Post("/api/v1/webhooks/gmail", h.GmailWebhook)
	app.Post("/webhook/google-calendar", h.GoogleCalendarWebhook)
	app.Post("/webhooks/google-calendar", h.GoogleCalendarWebhook)
	app.Post("/api/v1/webhook/google-calendar", h.GoogleCalendarWebhook)
	app.Post("/api/v1/webhooks/google-calendar", h.GoogleCalendarWebhook)
	app.Post("/webhook/outlook", h.OutlookWebhook)
	app.Post("/webhooks/outlook", h.OutlookWebhook)
	app.Post("/api/v1/webhook/outlook", h.OutlookWebhook)
	app.Post("/api/v1/webhooks/outlook", h.OutlookWebhook)
	app.Post("/webhook/outlook-calendar", h.OutlookCalendarWebhook)
	app.Post("/webhooks/outlook-calendar", h.OutlookCalendarWebhook)
	app.Post("/api/v1/webhook/outlook-calendar", h.OutlookCalendarWebhook)
	app.Post("/api/v1/webhooks/outlook-calendar", h.OutlookCalendarWebhook)
	app.Get("/webhook/outlook", h.OutlookValidation)
	app.Get("/webhooks/outlook", h.OutlookValidation)
	app.Get("/api/v1/webhook/outlook", h.OutlookValidation)
	app.Get("/api/v1/webhooks/outlook", h.OutlookValidation)
	app.Get("/webhook/outlook-calendar", h.OutlookValidation)
	app.Get("/webhooks/outlook-calendar", h.OutlookValidation)
	app.Get("/api/v1/webhook/outlook-calendar", h.OutlookValidation)
	app.Get("/api/v1/webhooks/outlook-calendar", h.OutlookValidation)
}

func (h *WebhookHandler) RegisterManagement(router fiber.Router) {
	webhooks := router.Group("/webhooks")
	webhooks.Get("/", h.ListWebhooks)
	webhooks.Get("/:id", h.GetWebhook)
	webhooks.Post("/setup/:connection_id", h.SetupWebhook)
	webhooks.Delete("/:id", h.StopWebhook)
	webhooks.Post("/:id/enable", h.EnableWebhook)
	webhooks.Post("/:id/disable", h.DisableWebhook)
	webhooks.Get("/metrics", h.GetWebhookMetrics)
}

func (h *WebhookHandler) idempotencyKey(provider string, connID int64, historyID uint64) string {
	return fmt.Sprintf("webhook:idempotent:%s:%d:%d", provider, connID, historyID)
}

func (h *WebhookHandler) syncLockKey(connID int64) string {
	return fmt.Sprintf("webhook:synclock:%d", connID)
}

func (h *WebhookHandler) checkIdempotency(ctx context.Context, provider string, connID int64, historyID uint64) bool {
	if h.redis == nil {
		return false
	}
	key := h.idempotencyKey(provider, connID, historyID)
	ok, err := h.redis.SetNX(ctx, key, "1", IdempotencyTTL).Result()
	if err != nil || !ok {
		atomic.AddInt64(&h.metrics.Duplicates, 1)
		return true
	}
	return false
}

func (h *WebhookHandler) acquireSyncLock(ctx context.Context, connID int64) bool {
	if h.redis == nil {
		return true
	}
	key := h.syncLockKey(connID)
	ok, err := h.redis.SetNX(ctx, key, "1", SyncLockTTL).Result()
	return err == nil && ok
}

func (h *WebhookHandler) releaseSyncLock(ctx context.Context, connID int64) {
	if h.redis == nil {
		return
	}
	_ = h.redis.Del(ctx, h.syncLockKey(connID))
}

func (h *WebhookHandler) ListWebhooks(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}
	if h.webhookService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Webhook service not available")
	}
	webhooks, err := h.webhookService.ListUserWebhooks(c.Context(), userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(fiber.Map{"webhooks": webhooks, "total": len(webhooks)})
}

func (h *WebhookHandler) GetWebhook(c *fiber.Ctx) error {
	if h.webhookService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Webhook service not available")
	}
	webhookID, err := parseID(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	webhook, err := h.webhookService.GetWebhook(c.Context(), webhookID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Webhook not found")
	}
	return c.JSON(webhook)
}

func (h *WebhookHandler) SetupWebhook(c *fiber.Ctx) error {
	if h.webhookService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Webhook service not available")
	}
	connectionID, err := parseID(c.Params("connection_id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid connection ID")
	}
	webhook, err := h.webhookService.SetupWatch(c.Context(), connectionID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(webhook)
}

func (h *WebhookHandler) StopWebhook(c *fiber.Ctx) error {
	if h.webhookService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Webhook service not available")
	}
	webhookID, err := parseID(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	webhook, err := h.webhookService.GetWebhook(c.Context(), webhookID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Webhook not found")
	}
	if err := h.webhookService.StopWatch(c.Context(), webhook.ConnectionID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *WebhookHandler) EnableWebhook(c *fiber.Ctx) error {
	if h.webhookService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Webhook service not available")
	}
	webhookID, err := parseID(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	webhook, err := h.webhookService.EnableWebhook(c.Context(), webhookID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(webhook)
}

func (h *WebhookHandler) DisableWebhook(c *fiber.Ctx) error {
	if h.webhookService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Webhook service not available")
	}
	webhookID, err := parseID(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	if err := h.webhookService.DisableWebhook(c.Context(), webhookID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(fiber.Map{"success": true, "message": "Webhook disabled"})
}

func (h *WebhookHandler) GetWebhookMetrics(c *fiber.Ctx) error {
	m := h.GetMetrics()
	return c.JSON(fiber.Map{
		"processed": m.Processed, "duplicates": m.Duplicates,
		"errors": m.Errors, "queued": m.Queued, "direct": m.Direct,
	})
}

func parseID(s string) (int64, error) {
	var id int64
	_, err := fmt.Sscanf(s, "%d", &id)
	return id, err
}

// GmailPushNotification represents Gmail Pub/Sub push notification.
type GmailPushNotification struct {
	Message struct {
		Data        string `json:"data"`
		MessageID   string `json:"messageId"`
		PublishTime string `json:"publishTime"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// GmailNotificationData represents the decoded data from Gmail push notification.
type GmailNotificationData struct {
	EmailAddress string `json:"emailAddress"`
	HistoryID    uint64 `json:"historyId"`
}

func (h *WebhookHandler) GmailWebhook(c *fiber.Ctx) error {
	var notification GmailPushNotification
	if err := c.BodyParser(&notification); err != nil {
		logger.WithError(err).Warn("[GmailWebhook] Failed to parse notification")
		return c.SendStatus(fiber.StatusOK)
	}

	data, err := base64.StdEncoding.DecodeString(notification.Message.Data)
	if err != nil {
		logger.WithError(err).Warn("[GmailWebhook] Failed to decode data")
		return c.SendStatus(fiber.StatusOK)
	}

	var notificationData GmailNotificationData
	if err := json.Unmarshal(data, &notificationData); err != nil {
		logger.WithError(err).Warn("[GmailWebhook] Failed to unmarshal data")
		return c.SendStatus(fiber.StatusOK)
	}

	logger.Info("[GmailWebhook] Received: email=%s, historyId=%d",
		notificationData.EmailAddress, notificationData.HistoryID)

	ctx := c.Context()

	conn, err := h.oauthService.GetConnectionByEmail(ctx, notificationData.EmailAddress, "google")
	if err != nil {
		logger.WithError(err).Warn("[GmailWebhook] Failed to find connection: %s", notificationData.EmailAddress)
		return c.SendStatus(fiber.StatusOK)
	}
	if conn == nil {
		logger.Warn("[GmailWebhook] No connection for email: %s", notificationData.EmailAddress)
		return c.SendStatus(fiber.StatusOK)
	}

	if h.checkIdempotency(ctx, "google", conn.ID, notificationData.HistoryID) {
		logger.Debug("[GmailWebhook] Duplicate skipped: conn=%d, historyId=%d", conn.ID, notificationData.HistoryID)
		return c.SendStatus(fiber.StatusOK)
	}

	if !h.acquireSyncLock(ctx, conn.ID) {
		logger.Info("[GmailWebhook] Lock busy, queueing: conn=%d", conn.ID)
		h.processGmailQueued(ctx, conn.ID, conn.UserID.String(), notificationData.HistoryID)
		return c.SendStatus(fiber.StatusOK)
	}

	atomic.AddInt64(&h.metrics.Processed, 1)

	if h.realtime != nil {
		h.realtime.Push(ctx, conn.UserID.String(), &domain.RealtimeEvent{
			Type:      domain.EventSyncStarted,
			Timestamp: time.Now(),
			Data: &domain.SyncProgressData{
				ConnectionID: conn.ID,
				Status:       "syncing",
			},
		})
	}

	if h.mailSyncService != nil {
		h.processGmailDirect(ctx, conn.ID, conn.UserID.String(), notificationData.HistoryID)
	} else if h.messageProducer != nil {
		h.processGmailQueued(ctx, conn.ID, conn.UserID.String(), notificationData.HistoryID)
	} else {
		logger.Error("[GmailWebhook] No sync service or producer available")
		atomic.AddInt64(&h.metrics.Errors, 1)
		h.releaseSyncLock(ctx, conn.ID)
	}

	return c.SendStatus(fiber.StatusOK)
}

func (h *WebhookHandler) processGmailDirect(ctx context.Context, connID int64, userID string, historyID uint64) {
	atomic.AddInt64(&h.metrics.Direct, 1)

	go func() {
		defer h.releaseSyncLock(ctx, connID)

		syncCtx, cancel := context.WithTimeout(context.Background(), SyncLockTTL)
		defer cancel()

		if err := h.mailSyncService.DeltaSync(syncCtx, connID, historyID); err != nil {
			logger.WithError(err).Error("[GmailWebhook] DeltaSync failed: conn=%d", connID)
			atomic.AddInt64(&h.metrics.Errors, 1)
			if h.realtime != nil {
				h.realtime.Push(syncCtx, userID, &domain.RealtimeEvent{
					Type:      domain.EventSyncCompleted,
					Timestamp: time.Now(),
					Data: &domain.SyncProgressData{
						ConnectionID: connID,
						Status:       "error",
					},
				})
			}
		} else {
			logger.Info("[GmailWebhook] DeltaSync completed: conn=%d", connID)
			if h.realtime != nil {
				h.realtime.Push(syncCtx, userID, &domain.RealtimeEvent{
					Type:      domain.EventSyncCompleted,
					Timestamp: time.Now(),
					Data: &domain.SyncProgressData{
						ConnectionID: connID,
						Status:       "completed",
					},
				})
			}
		}
	}()
}

func (h *WebhookHandler) processGmailQueued(ctx context.Context, connID int64, userID string, historyID uint64) {
	atomic.AddInt64(&h.metrics.Queued, 1)

	syncJob := &out.MailSyncJob{
		UserID:       userID,
		ConnectionID: connID,
		Provider:     "google",
		FullSync:     false,
		HistoryID:    historyID,
	}

	if err := h.messageProducer.PublishMailSync(ctx, syncJob); err != nil {
		logger.WithError(err).Error("[GmailWebhook] Failed to publish: conn=%d", connID)
		atomic.AddInt64(&h.metrics.Errors, 1)
		h.releaseSyncLock(ctx, connID)
	} else {
		logger.Info("[GmailWebhook] Queued: conn=%d, historyId=%d", connID, historyID)
	}
}

// OutlookNotification represents Microsoft Graph change notification.
type OutlookNotification struct {
	Value []struct {
		SubscriptionID                 string `json:"subscriptionId"`
		SubscriptionExpirationDateTime string `json:"subscriptionExpirationDateTime"`
		ChangeType                     string `json:"changeType"`
		Resource                       string `json:"resource"`
		ClientState                    string `json:"clientState"`
		ResourceData                   struct {
			ID string `json:"id"`
		} `json:"resourceData"`
	} `json:"value"`
}

func (h *WebhookHandler) OutlookWebhook(c *fiber.Ctx) error {
	var notification OutlookNotification
	if err := c.BodyParser(&notification); err != nil {
		logger.WithError(err).Warn("[OutlookWebhook] Failed to parse notification")
		return c.SendStatus(fiber.StatusOK)
	}

	ctx := c.Context()

	for _, change := range notification.Value {
		logger.Info("[OutlookWebhook] Received: subscription=%s, changeType=%s",
			change.SubscriptionID, change.ChangeType)

		conn, err := h.oauthService.GetConnectionByWebhookID(ctx, change.SubscriptionID, "outlook")
		if err != nil || conn == nil {
			logger.Warn("[OutlookWebhook] No connection for subscription: %s", change.SubscriptionID)
			continue
		}

		var pseudoHistoryID uint64
		if change.ResourceData.ID != "" {
			for _, ch := range change.ResourceData.ID {
				pseudoHistoryID = pseudoHistoryID*31 + uint64(ch)
			}
		} else {
			pseudoHistoryID = uint64(time.Now().UnixNano())
		}

		if h.checkIdempotency(ctx, "outlook", conn.ID, pseudoHistoryID) {
			logger.Debug("[OutlookWebhook] Duplicate skipped: conn=%d", conn.ID)
			continue
		}

		if !h.acquireSyncLock(ctx, conn.ID) {
			logger.Info("[OutlookWebhook] Lock busy, queueing: conn=%d", conn.ID)
		}

		atomic.AddInt64(&h.metrics.Processed, 1)

		if h.realtime != nil {
			h.realtime.Push(ctx, conn.UserID.String(), &domain.RealtimeEvent{
				Type:      domain.EventSyncStarted,
				Timestamp: time.Now(),
				Data: &domain.SyncProgressData{
					ConnectionID: conn.ID,
					Status:       "syncing",
				},
			})
		}

		if h.messageProducer != nil {
			syncJob := &out.MailSyncJob{
				UserID:       conn.UserID.String(),
				ConnectionID: conn.ID,
				Provider:     "outlook",
				FullSync:     false,
			}
			if err := h.messageProducer.PublishMailSync(ctx, syncJob); err != nil {
				logger.WithError(err).Error("[OutlookWebhook] Failed to publish sync job")
				atomic.AddInt64(&h.metrics.Errors, 1)
				h.releaseSyncLock(ctx, conn.ID)
			} else {
				atomic.AddInt64(&h.metrics.Queued, 1)
			}
		} else {
			h.releaseSyncLock(ctx, conn.ID)
		}
	}

	return c.SendStatus(fiber.StatusOK)
}

func (h *WebhookHandler) OutlookValidation(c *fiber.Ctx) error {
	validationToken := c.Query("validationToken")
	if validationToken != "" {
		c.Set("Content-Type", "text/plain")
		return c.SendString(validationToken)
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *WebhookHandler) GoogleCalendarWebhook(c *fiber.Ctx) error {
	channelID := c.Get("X-Goog-Channel-ID")
	resourceID := c.Get("X-Goog-Resource-ID")
	resourceState := c.Get("X-Goog-Resource-State")

	logger.Info("[GoogleCalendarWebhook] Received: channel=%s, resource=%s, state=%s",
		channelID, resourceID, resourceState)

	if resourceState == "sync" {
		return c.SendStatus(fiber.StatusOK)
	}

	ctx := c.Context()

	if h.calendarSyncRepo == nil {
		logger.Warn("[GoogleCalendarWebhook] calendarSyncRepo not initialized")
		return c.SendStatus(fiber.StatusOK)
	}

	syncState, err := h.calendarSyncRepo.GetByWatchID(ctx, channelID)
	if err != nil || syncState == nil {
		logger.Warn("[GoogleCalendarWebhook] No sync state for channel: %s", channelID)
		return c.SendStatus(fiber.StatusOK)
	}

	var pseudoHistoryID uint64
	for _, ch := range resourceID {
		pseudoHistoryID = pseudoHistoryID*31 + uint64(ch)
	}

	if h.checkIdempotency(ctx, "google-calendar", syncState.ConnectionID, pseudoHistoryID) {
		return c.SendStatus(fiber.StatusOK)
	}

	atomic.AddInt64(&h.metrics.Processed, 1)

	if h.realtime != nil {
		h.realtime.Push(ctx, syncState.UserID, &domain.RealtimeEvent{
			Type:      domain.EventCalendarUpdated,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"connection_id": syncState.ConnectionID,
				"calendar_id":   syncState.CalendarID,
				"change_type":   resourceState,
			},
		})
	}

	if h.messageProducer != nil {
		syncJob := &out.CalendarSyncJob{
			UserID:       syncState.UserID,
			ConnectionID: syncState.ConnectionID,
			CalendarID:   syncState.CalendarID,
			FullSync:     false,
		}
		if err := h.messageProducer.PublishCalendarSync(ctx, syncJob); err != nil {
			logger.WithError(err).Error("[GoogleCalendarWebhook] Failed to publish")
			atomic.AddInt64(&h.metrics.Errors, 1)
		} else {
			atomic.AddInt64(&h.metrics.Queued, 1)
		}
	}

	return c.SendStatus(fiber.StatusOK)
}

func (h *WebhookHandler) OutlookCalendarWebhook(c *fiber.Ctx) error {
	var notification OutlookNotification
	if err := c.BodyParser(&notification); err != nil {
		logger.WithError(err).Warn("[OutlookCalendarWebhook] Failed to parse notification")
		return c.SendStatus(fiber.StatusOK)
	}

	ctx := c.Context()

	for _, change := range notification.Value {
		if change.ClientState != "calendar-watch" {
			continue
		}

		conn, err := h.oauthService.GetConnectionByWebhookID(ctx, change.SubscriptionID, "outlook")
		if err != nil || conn == nil {
			continue
		}

		var pseudoHistoryID uint64
		if change.ResourceData.ID != "" {
			for _, ch := range change.ResourceData.ID {
				pseudoHistoryID = pseudoHistoryID*31 + uint64(ch)
			}
		} else {
			pseudoHistoryID = uint64(time.Now().UnixNano())
		}

		if h.checkIdempotency(ctx, "outlook-calendar", conn.ID, pseudoHistoryID) {
			continue
		}

		atomic.AddInt64(&h.metrics.Processed, 1)

		if h.realtime != nil {
			h.realtime.Push(ctx, conn.UserID.String(), &domain.RealtimeEvent{
				Type:      domain.EventCalendarUpdated,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"connection_id": conn.ID,
					"change_type":   change.ChangeType,
				},
			})
		}

		if h.messageProducer != nil {
			syncJob := &out.CalendarSyncJob{
				UserID:       conn.UserID.String(),
				ConnectionID: conn.ID,
				FullSync:     false,
			}
			if err := h.messageProducer.PublishCalendarSync(ctx, syncJob); err != nil {
				atomic.AddInt64(&h.metrics.Errors, 1)
			} else {
				atomic.AddInt64(&h.metrics.Queued, 1)
			}
		}
	}

	return c.SendStatus(fiber.StatusOK)
}
