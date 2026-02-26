package http

import (
	"strconv"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/in"
	"worker_server/core/service/common"
	"worker_server/core/service/email"
	"worker_server/pkg/logger"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// =============================================================================
// Optimized Mail Handler
// =============================================================================

// OptimizedMailHandler handles mail requests with caching and prefetch
type OptimizedMailHandler struct {
	emailService  in.EmailService
	cacheService *common.CacheService
	syncService  *mail.SyncService
}

// NewOptimizedMailHandler creates a new optimized mail handler
func NewOptimizedMailHandler(
	emailService in.EmailService,
	cacheService *common.CacheService,
	syncService *mail.SyncService,
) *OptimizedMailHandler {
	return &OptimizedMailHandler{
		emailService:  emailService,
		cacheService: cacheService,
		syncService:  syncService,
	}
}

// Register registers optimized mail routes
func (h *OptimizedMailHandler) Register(app fiber.Router) {
	mail := app.Group("/v2/email")
	mail.Get("/", h.ListEmails)
	mail.Get("/:id", h.GetEmail)
	mail.Get("/:id/body", h.GetEmailBody)
}

// =============================================================================
// List Emails - With Caching
// =============================================================================

// ListEmailsResponse represents the list response
type ListEmailsResponse struct {
	Emails     []*domain.Email `json:"emails"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	HasMore    bool            `json:"has_more"`
	CacheHit   bool            `json:"cache_hit,omitempty"`
	ResponseMs int64           `json:"response_ms"`
}

// ListEmails returns paginated email list with caching
func (h *OptimizedMailHandler) ListEmails(c *fiber.Ctx) error {
	start := time.Now()

	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	// Parse query params
	folder := c.Query("folder", "inbox")
	limit := c.QueryInt("limit", 20)
	page := c.QueryInt("page", 0)
	offset := page * limit

	// Check cache first
	cacheHit := false
	if h.cacheService != nil {
		cached, err := h.cacheService.GetList(c.Context(), userID.String(), folder, page, limit)
		if err == nil && cached != nil {
			cacheHit = true

			// Prefetch next page in background
			h.cacheService.PrefetchNextPage(c.Context(), userID.String(), folder, page)

			return c.JSON(ListEmailsResponse{
				Emails:     cached.Emails,
				Total:      cached.Total,
				Page:       page,
				HasMore:    len(cached.Emails) == limit,
				CacheHit:   true,
				ResponseMs: time.Since(start).Milliseconds(),
			})
		}
	}

	// Cache miss - query database
	filter := &domain.EmailFilter{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	}

	if folder != "" {
		f := domain.LegacyFolder(folder)
		filter.Folder = &f
	}

	emails, total, err := h.emailService.ListEmails(c.Context(), filter)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	// Cache the result
	if h.cacheService != nil && len(emails) > 0 {
		go h.cacheService.CacheList(c.Context(), userID.String(), folder, page, emails, total)

		// Prefetch next page
		h.cacheService.PrefetchNextPage(c.Context(), userID.String(), folder, page)

		// Prefetch first 5 email bodies
		if len(emails) > 0 {
			connectionID := getConnectionIDFromEmails(emails)
			prefetchIDs := make([]int64, 0, 5)
			for i, e := range emails {
				if i >= 5 {
					break
				}
				prefetchIDs = append(prefetchIDs, e.ID)
			}
			h.cacheService.PrefetchBodies(c.Context(), prefetchIDs, connectionID)
		}
	}

	return c.JSON(ListEmailsResponse{
		Emails:     emails,
		Total:      total,
		Page:       page,
		HasMore:    len(emails) == limit,
		CacheHit:   cacheHit,
		ResponseMs: time.Since(start).Milliseconds(),
	})
}

// =============================================================================
// Get Email - With Metadata Cache
// =============================================================================

// GetEmail returns a single email
func (h *OptimizedMailHandler) GetEmail(c *fiber.Ctx) error {
	start := time.Now()

	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	emailID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	email, err := h.emailService.GetEmail(c.Context(), userID, emailID)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	// Prefetch next 2 email bodies
	if h.cacheService != nil {
		go func() {
			// This would need access to the email list to get next IDs
			// For now, just prefetch the body of the current email
			h.cacheService.PrefetchBodies(c.Context(), []int64{emailID}, email.ConnectionID)
		}()
	}

	return c.JSON(fiber.Map{
		"email":       email,
		"response_ms": time.Since(start).Milliseconds(),
	})
}

// =============================================================================
// Get Email Body - 3-Tier Cache
// =============================================================================

// AttachmentInfo represents attachment metadata in body response
type AttachmentInfo struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	MimeType  string `json:"mime_type"`
	Size      int64  `json:"size"`
	IsInline  bool   `json:"is_inline"`
	ContentID string `json:"content_id,omitempty"`
}

// GetEmailBodyResponse represents the body response
type GetEmailBodyResponse struct {
	EmailID     int64            `json:"email_id"`
	HTML        string           `json:"html,omitempty"`
	Text        string           `json:"text,omitempty"`
	Attachments []AttachmentInfo `json:"attachments,omitempty"` // 첨부파일 메타 포함
	Source      string           `json:"source"`                // "redis", "mongodb", "provider"
	ResponseMs  int64            `json:"response_ms"`
}

// GetEmailBody returns email body with 3-tier caching
func (h *OptimizedMailHandler) GetEmailBody(c *fiber.Ctx) error {
	start := time.Now()

	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	emailID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	// Get email first to verify ownership and get connectionID
	email, err := h.emailService.GetEmail(c.Context(), userID, emailID)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	// 첨부파일 조회 (DB에서)
	var attachments []AttachmentInfo
	if h.cacheService != nil {
		attEntities, _ := h.cacheService.GetAttachments(c.Context(), emailID)
		for _, att := range attEntities {
			contentID := ""
			if att.ContentID != nil {
				contentID = *att.ContentID
			}
			attachments = append(attachments, AttachmentInfo{
				ID:        att.ExternalID,
				Filename:  att.Filename,
				MimeType:  att.MimeType,
				Size:      att.Size,
				IsInline:  att.IsInline,
				ContentID: contentID,
			})
		}
	}

	// Use cache service for 3-tier lookup
	if h.cacheService != nil {
		body, err := h.cacheService.GetBody(c.Context(), emailID, email.ConnectionID)
		if err != nil {
			logger.WithError(err).Warn("[OptimizedMailHandler.GetEmailBody] Cache error")
		}
		if body != nil {
			// Determine source based on metrics
			source := determineSource(h.cacheService)

			return c.JSON(GetEmailBodyResponse{
				EmailID:     emailID,
				HTML:        body.HTMLBody,
				Text:        body.TextBody,
				Attachments: attachments,
				Source:      source,
				ResponseMs:  time.Since(start).Milliseconds(),
			})
		}
	}

	// Fallback to direct service call
	body, err := h.emailService.GetEmailBody(c.Context(), emailID)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(GetEmailBodyResponse{
		EmailID:     emailID,
		HTML:        body.HTMLBody,
		Text:        body.TextBody,
		Attachments: attachments,
		Source:      "service",
		ResponseMs:  time.Since(start).Milliseconds(),
	})
}

// =============================================================================
// Helpers
// =============================================================================

func getConnectionIDFromEmails(emails []*domain.Email) int64 {
	if len(emails) > 0 {
		return emails[0].ConnectionID
	}
	return 0
}

func determineSource(cache *common.CacheService) string {
	if cache == nil {
		return "unknown"
	}
	metrics := cache.GetMetrics()
	if metrics.RedisHits > 0 {
		return "redis"
	}
	if metrics.MongoHits > 0 {
		return "mongodb"
	}
	if metrics.ProviderHits > 0 {
		return "provider"
	}
	return "unknown"
}

// =============================================================================
// Batch Operations with Cache Invalidation
// =============================================================================

// BatchMarkAsRead marks multiple emails as read
func (h *OptimizedMailHandler) BatchMarkAsRead(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req struct {
		IDs    []int64 `json:"ids"`
		Folder string  `json:"folder,omitempty"`
	}
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request")
	}

	if err := h.emailService.MarkAsRead(c.Context(), userID, req.IDs); err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	// Invalidate list cache for this folder
	if h.cacheService != nil && req.Folder != "" {
		go h.cacheService.InvalidateList(c.Context(), userID.String(), req.Folder)
	}

	return c.JSON(fiber.Map{
		"status":  "ok",
		"updated": len(req.IDs),
	})
}

// BatchArchive archives multiple emails
func (h *OptimizedMailHandler) BatchArchive(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req struct {
		IDs    []int64 `json:"ids"`
		Folder string  `json:"folder,omitempty"`
	}
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request")
	}

	if err := h.emailService.Archive(c.Context(), userID, req.IDs); err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	// Invalidate cache for both source folder and archive
	if h.cacheService != nil {
		go func() {
			if req.Folder != "" {
				h.cacheService.InvalidateList(c.Context(), userID.String(), req.Folder)
			}
			h.cacheService.InvalidateList(c.Context(), userID.String(), "archive")
		}()
	}

	return c.JSON(fiber.Map{
		"status":   "ok",
		"archived": len(req.IDs),
	})
}

// =============================================================================
// Cache Stats Endpoint
// =============================================================================

// GetCacheStats returns cache statistics
func (h *OptimizedMailHandler) GetCacheStats(c *fiber.Ctx) error {
	if h.cacheService == nil {
		return c.JSON(fiber.Map{
			"enabled": false,
		})
	}

	metrics := h.cacheService.GetMetrics()
	hitRate := h.cacheService.GetHitRate()

	return c.JSON(fiber.Map{
		"enabled":       true,
		"redis_hits":    metrics.RedisHits,
		"redis_misses":  metrics.RedisMisses,
		"mongo_hits":    metrics.MongoHits,
		"mongo_misses":  metrics.MongoMisses,
		"provider_hits": metrics.ProviderHits,
		"hit_rate":      hitRate,
	})
}

// GetUserID helper that handles uuid parsing
func getUserID(c *fiber.Ctx) (uuid.UUID, error) {
	// This would be extracted from JWT or context
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		return uuid.Nil, fiber.ErrUnauthorized
	}

	switch v := userIDStr.(type) {
	case string:
		return uuid.Parse(v)
	case uuid.UUID:
		return v, nil
	default:
		return uuid.Nil, fiber.ErrUnauthorized
	}
}
