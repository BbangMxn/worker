package http

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"
	"golang.org/x/oauth2"

	"worker_server/adapter/out/provider"
	"worker_server/core/agent/rag"
	"worker_server/core/domain"
	"worker_server/core/port/in"
	"worker_server/core/port/out"
	"worker_server/core/service/auth"
	"worker_server/core/service/search"
	"worker_server/pkg/logger"
	"worker_server/pkg/ratelimit"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// cidPattern matches CID references in HTML: src="cid:xxx", src='cid:xxx', src=cid:xxx, src="cid:<xxx>"
// Compiled once at package level for performance
var cidPattern = regexp.MustCompile(`src=["']?cid:<?([^"'>\s]+)>?["']?`)

type EmailHandler struct {
	emailService     in.EmailService
	oauthService    *auth.OAuthService
	gmailProvider   *provider.GmailAdapter
	outlookProvider *provider.OutlookAdapter
	emailRepo        out.EmailRepository
	attachmentRepo  out.AttachmentRepository
	messageProducer out.MessageProducer
	unifiedProvider *provider.UnifiedMailProvider
	syncStateRepo   out.SyncStateRepository
	apiProtector    *ratelimit.APIProtector
	emailCache      *ratelimit.EmailListCache
	searchService   *search.Service
}

func NewMailHandler(emailService in.EmailService) *EmailHandler {
	return &EmailHandler{emailService: emailService}
}

func NewMailHandlerWithProvider(
	emailService in.EmailService,
	oauthService *auth.OAuthService,
	gmailProvider *provider.GmailAdapter,
	outlookProvider *provider.OutlookAdapter,
	emailRepo out.EmailRepository,
	attachmentRepo out.AttachmentRepository,
	messageProducer out.MessageProducer,
	syncStateRepo out.SyncStateRepository,
	redisClient *redis.Client,
	vectorStore *rag.VectorStore,
	embedder *rag.Embedder,
) *EmailHandler {
	// Create unified provider
	var unifiedProvider *provider.UnifiedMailProvider
	if oauthService != nil && emailRepo != nil {
		unifiedProvider = provider.NewUnifiedMailProvider(oauthService, emailRepo, syncStateRepo)
		if gmailProvider != nil {
			unifiedProvider.RegisterProvider("gmail", gmailProvider)
			unifiedProvider.RegisterProvider("google", gmailProvider)
		}
		if outlookProvider != nil {
			unifiedProvider.RegisterProvider("outlook", outlookProvider)
			unifiedProvider.RegisterProvider("microsoft", outlookProvider)
		}
	}

	// API 보호 레이어 초기화
	apiProtector := ratelimit.NewAPIProtector(redisClient, &ratelimit.Config{
		MaxConcurrent:     100, // 최대 동시 요청
		RequestsPerSecond: 10,  // Gmail API 제한 고려
		BurstSize:         20,  // 버스트 허용
		DebounceDuration:  30 * time.Second,
		MaxPayloadSize:    50, // 응답 최대 개수
	})

	// 이메일 목록 캐시 초기화
	emailCache := ratelimit.NewEmailListCache(redisClient, &ratelimit.CacheConfig{
		L1MaxSize:          1000,
		L1TTL:              30 * time.Second,
		L2TTL:              1 * time.Minute,
		MaxCacheableOffset: 100, // offset 100 이상은 캐시 안 함
	})

	// 통합 검색 서비스 초기화
	var searchService *search.Service
	if vectorStore != nil && embedder != nil {
		searchService = search.NewService(emailRepo, vectorStore, embedder)
	}

	return &EmailHandler{
		emailService:     emailService,
		oauthService:    oauthService,
		gmailProvider:   gmailProvider,
		outlookProvider: outlookProvider,
		emailRepo:        emailRepo,
		attachmentRepo:  attachmentRepo,
		messageProducer: messageProducer,
		unifiedProvider: unifiedProvider,
		syncStateRepo:   syncStateRepo,
		apiProtector:    apiProtector,
		emailCache:      emailCache,
		searchService:   searchService,
	}
}

func (h *EmailHandler) Register(app fiber.Router) {
	mail := app.Group("/email")

	// =========================================================================
	// 메일 목록 API
	// =========================================================================
	// Inbox: primary, work, personal 카테고리만 (중요 메일)
	// TODO: Inbox + 우선순위 정렬 (처리해야 할 메일)
	// Feed: notification, newsletter, marketing 등 자동 메일은 /category/:category로 조회
	// =========================================================================
	mail.Get("/", h.ListEmails)                       // All Mail (전체 메일)
	mail.Get("/inbox", h.ListInbox)                   // Inbox (primary, work, personal)
	mail.Get("/inbox/todo", h.ListTodo)               // TODO (Inbox + 우선순위 DESC 정렬)
	mail.Get("/category/:category", h.ListByCategory) // 카테고리별 (notification, newsletter, finance 등)

	// =========================================================================
	// 폴더별 목록
	// =========================================================================
	mail.Get("/sent", h.ListSent)       // Sent (보낸 메일)
	mail.Get("/drafts", h.ListDrafts)   // Drafts (임시 보관함)
	mail.Get("/trash", h.ListTrash)     // Trash (휴지통)
	mail.Get("/spam", h.ListSpam)       // Spam (스팸)
	mail.Get("/archive", h.ListArchive) // Archive (보관함)

	// =========================================================================
	// 검색 API
	// =========================================================================
	mail.Get("/search", h.SearchEmails)      // 검색 (DB + Provider)
	mail.Get("/search/v2", h.SearchEmailsV2) // 통합 검색 (DB + Vector + Provider)

	// =========================================================================
	// 동기화 API
	// =========================================================================
	mail.Get("/unified", h.ListEmailsUnified)        // 통합 목록 (커서 기반 페이징)
	mail.Get("/fetch", h.FetchFromProvider)          // Provider에서 직접 가져오기
	mail.Get("/fetch/body", h.FetchBodyFromProvider) // Provider에서 본문 가져오기
	mail.Post("/sync", h.TriggerSync)                // 동기화 트리거
	mail.Post("/resync", h.ResyncEmails)             // 재동기화 (첨부파일 갱신)
	mail.Post("/reclassify", h.ReclassifyEmails)     // 미분류 메일 재분류

	// =========================================================================
	// 첨부파일 API
	// =========================================================================
	mail.Get("/attachments", h.ListAllAttachments)                              // 전체 첨부파일 목록
	mail.Get("/attachments/stats", h.GetAttachmentStats)                        // 첨부파일 통계
	mail.Get("/attachments/search", h.SearchAttachments)                        // 첨부파일 검색
	mail.Post("/attachments/upload/session", h.CreateUploadSession)             // 업로드 세션 생성
	mail.Get("/attachments/upload/:sessionId/status", h.GetUploadSessionStatus) // 업로드 상태
	mail.Delete("/attachments/upload/:sessionId", h.CancelUploadSession)        // 업로드 취소

	// =========================================================================
	// 단일 메일 API
	// =========================================================================
	mail.Get("/:id", h.GetEmail)                                              // 메일 상세
	mail.Get("/:id/body", h.GetEmailBody)                                     // 메일 본문
	mail.Get("/:id/attachments", h.GetAttachments)                            // 첨부파일 목록
	mail.Get("/:id/attachments/:attachmentId", h.GetAttachment)               // 첨부파일 상세
	mail.Get("/:id/attachments/:attachmentId/download", h.DownloadAttachment) // 첨부파일 다운로드
	mail.Get("/:id/cid/:contentId", h.GetInlineAttachment)                    // 인라인 첨부파일
	mail.Get("/:id/attachments/download/all", h.DownloadAllAttachments)       // 전체 첨부파일 ZIP
	mail.Post("/:id/resync", h.ResyncSingleEmail)                             // 단일 재동기화

	// =========================================================================
	// 메일 작성 API
	// =========================================================================
	mail.Post("/", h.SendEmail)               // 메일 전송
	mail.Post("/:id/reply", h.ReplyEmail)     // 답장
	mail.Post("/:id/forward", h.ForwardEmail) // 전달

	// =========================================================================
	// 배치 작업 API (여러 메일 동시 처리)
	// =========================================================================
	mail.Post("/read", h.MarkAsRead)                 // 읽음 처리
	mail.Post("/unread", h.MarkAsUnread)             // 안읽음 처리
	mail.Post("/star", h.Star)                       // 별표
	mail.Post("/unstar", h.Unstar)                   // 별표 해제
	mail.Post("/archive", h.Archive)                 // 보관
	mail.Post("/trash", h.Trash)                     // 휴지통
	mail.Post("/delete", h.DeleteEmails)             // 영구 삭제
	mail.Post("/move", h.MoveToFolder)               // 폴더 이동
	mail.Post("/snooze", h.Snooze)                   // 스누즈
	mail.Post("/unsnooze", h.Unsnooze)               // 스누즈 해제
	mail.Post("/labels/add", h.BatchAddLabels)       // 라벨 추가
	mail.Post("/labels/remove", h.BatchRemoveLabels) // 라벨 제거
	mail.Post("/workflow", h.UpdateWorkflowStatus)   // Workflow 상태 (todo/done)
}

// FetchFromProvider fetches emails directly from Gmail API (실시간)
func (h *EmailHandler) FetchFromProvider(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	connectionID := c.QueryInt("connection_id", 0)
	if connectionID == 0 {
		return ErrorResponse(c, 400, "connection_id required")
	}

	limit := c.QueryInt("limit", 20)
	if limit > 100 {
		limit = 100
	}

	pageToken := c.Query("page_token", "")

	if h.oauthService == nil || h.gmailProvider == nil {
		return ErrorResponse(c, 500, "provider not configured")
	}

	logger.Info("[EmailHandler.FetchFromProvider] Fetching emails for user %s, connection %d, pageToken: %s", userID, connectionID, pageToken)

	emails, nextPageToken, err := h.fetchFromProviderWithToken(c, userID, int64(connectionID), limit, pageToken)
	if err != nil {
		return InternalErrorResponse(c, err, "fetch emails from provider")
	}

	logger.Info("[EmailHandler.FetchFromProvider] Fetched %d emails, nextPageToken: %s", len(emails), nextPageToken)

	return c.JSON(fiber.Map{
		"emails":          emails,
		"total":           len(emails),
		"source":          "gmail_api",
		"next_page_token": nextPageToken,
		"has_more":        nextPageToken != "",
	})
}

// TriggerSync triggers background mail sync
func (h *EmailHandler) TriggerSync(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req struct {
		ConnectionID int64 `json:"connection_id"`
		FullSync     bool  `json:"full_sync"`
	}
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request")
	}

	logger.Info("[EmailHandler.TriggerSync] User %s, Connection %d, FullSync %v", userID, req.ConnectionID, req.FullSync)

	// Publish sync job to Redis
	if h.messageProducer != nil {
		job := &out.MailSyncJob{
			UserID:       userID.String(),
			ConnectionID: req.ConnectionID,
			Provider:     "google",
			FullSync:     req.FullSync,
		}
		if err := h.messageProducer.PublishMailSync(c.Context(), job); err != nil {
			logger.Error("[EmailHandler.TriggerSync] Failed to publish sync job: %v", err)
			return ErrorResponse(c, 500, "failed to queue sync job")
		}
		logger.Info("[EmailHandler.TriggerSync] Sync job published to Redis")
	} else {
		logger.Warn("[EmailHandler.TriggerSync] MessageProducer not configured")
	}

	return c.JSON(fiber.Map{
		"status":  "ok",
		"message": "Sync job queued",
	})
}

// ResyncEmails resyncs emails to update attachment information
// POST /email/resync
// Body: { "connection_id": 123, "email_ids": [1, 2, 3] } or { "connection_id": 123, "all": true }
func (h *EmailHandler) ResyncEmails(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req struct {
		ConnectionID      int64   `json:"connection_id"`
		EmailIDs          []int64 `json:"email_ids"`          // 특정 이메일만 재동기화
		All               bool    `json:"all"`                // 모든 이메일 재동기화 (첨부파일 있는 것만)
		ResyncAttachments bool    `json:"resync_attachments"` // 첨부파일 정보 재동기화 (DB에 첨부파일 없는 이메일)
		Limit             int     `json:"limit"`              // 재동기화할 이메일 수 제한
	}
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request")
	}

	if req.ConnectionID == 0 {
		return ErrorResponse(c, 400, "connection_id required")
	}

	logger.Info("[EmailHandler.ResyncEmails] User %s, Connection %d, EmailIDs: %v, All: %v",
		userID, req.ConnectionID, req.EmailIDs, req.All)

	// OAuth 토큰 가져오기
	token, err := h.oauthService.GetOAuth2Token(c.Context(), req.ConnectionID)
	if err != nil {
		logger.Error("[EmailHandler.ResyncEmails] Failed to get OAuth token: %v", err)
		return ErrorResponse(c, 500, "failed to get oauth token")
	}

	var emailsToResync []*out.MailEntity

	if req.All {
		// 첨부파일이 있는 이메일 중 pending ID가 있는 것 조회
		emails, err := h.emailRepo.GetEmailsWithPendingAttachments(c.Context(), userID, req.ConnectionID)
		if err != nil {
			logger.Error("[EmailHandler.ResyncEmails] Failed to get emails with pending attachments: %v", err)
			return ErrorResponse(c, 500, "failed to get emails")
		}
		emailsToResync = emails
	} else if req.ResyncAttachments {
		// Gmail has:attachment 쿼리로 실제 첨부파일 있는 메시지만 조회
		limit := req.Limit
		if limit <= 0 {
			limit = 200
		}

		// 1. Gmail API로 첨부파일 있는 메시지 ID 조회
		attachmentMsgIDs, err := h.gmailProvider.GetAttachmentMessageIDs(c.Context(), token, limit)
		if err != nil {
			logger.Error("[EmailHandler.ResyncEmails] Failed to get attachment message IDs: %v", err)
			return ErrorResponse(c, 500, "failed to get attachment messages from Gmail")
		}
		logger.Info("[EmailHandler.ResyncEmails] Gmail returned %d messages with attachments", len(attachmentMsgIDs))

		// 2. DB에서 해당 external_id를 가진 이메일 중 attachment 정보 없는 것 조회
		emails, err := h.emailRepo.GetEmailsByExternalIDsNeedingAttachments(c.Context(), userID, req.ConnectionID, attachmentMsgIDs)
		if err != nil {
			logger.Error("[EmailHandler.ResyncEmails] Failed to get emails needing attachment resync: %v", err)
			return ErrorResponse(c, 500, "failed to get emails")
		}
		logger.Info("[EmailHandler.ResyncEmails] Found %d emails needing attachment sync", len(emails))
		emailsToResync = emails
	} else if len(req.EmailIDs) > 0 {
		// 특정 이메일 ID로 조회
		for _, emailID := range req.EmailIDs {
			email, err := h.emailRepo.GetByID(c.Context(), emailID)
			if err != nil || email == nil {
				continue
			}
			// 권한 체크
			if email.UserID != userID || email.ConnectionID != req.ConnectionID {
				continue
			}
			emailsToResync = append(emailsToResync, email)
		}
	} else {
		return ErrorResponse(c, 400, "email_ids or all required")
	}

	if len(emailsToResync) == 0 {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"message": "No emails to resync",
			"count":   0,
		})
	}

	// 백그라운드로 재동기화 실행
	go h.resyncEmailsBackground(userID, req.ConnectionID, token, emailsToResync)

	return c.JSON(fiber.Map{
		"status":  "ok",
		"message": "Resync started",
		"count":   len(emailsToResync),
	})
}

// ReclassifyEmails triggers reclassification for unclassified emails.
// POST /email/reclassify
// Body: { "connection_id": 123, "limit": 100 }
func (h *EmailHandler) ReclassifyEmails(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req struct {
		ConnectionID int64 `json:"connection_id"`
		Limit        int   `json:"limit"`
	}
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request")
	}

	if req.ConnectionID == 0 {
		return ErrorResponse(c, 400, "connection_id required")
	}

	if req.Limit <= 0 || req.Limit > 500 {
		req.Limit = 100
	}

	// 분류되지 않은 이메일 개수 확인
	count, err := h.emailRepo.CountUnclassified(c.Context(), req.ConnectionID)
	if err != nil {
		logger.Error("[EmailHandler.ReclassifyEmails] Failed to count unclassified: %v", err)
		return ErrorResponse(c, 500, "failed to count unclassified emails")
	}

	if count == 0 {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"message": "No unclassified emails",
			"count":   0,
		})
	}

	// 분류되지 않은 이메일 조회
	unclassified, err := h.emailRepo.ListUnclassifiedByConnection(c.Context(), req.ConnectionID, req.Limit)
	if err != nil {
		logger.Error("[EmailHandler.ReclassifyEmails] Failed to list unclassified: %v", err)
		return ErrorResponse(c, 500, "failed to list unclassified emails")
	}

	logger.Info("[EmailHandler.ReclassifyEmails] User %s, Connection %d, Found %d unclassified (total: %d)",
		userID, req.ConnectionID, len(unclassified), count)

	// ai.classify 작업 발행
	if h.messageProducer != nil {
		for _, email := range unclassified {
			h.messageProducer.PublishAIClassify(c.Context(), &out.AIClassifyJob{
				UserID:  userID.String(),
				EmailID: email.ID,
			})
		}
	}

	return c.JSON(fiber.Map{
		"status":             "ok",
		"message":            "Reclassification started",
		"queued":             len(unclassified),
		"total_unclassified": count,
	})
}

// ResyncSingleEmail resyncs a single email to update attachment information
// POST /email/:id/resync
func (h *EmailHandler) ResyncSingleEmail(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	emailID, err := c.ParamsInt("id")
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	// 이메일 조회
	email, err := h.emailRepo.GetByID(c.Context(), int64(emailID))
	if err != nil || email == nil {
		return ErrorResponse(c, 404, "email not found")
	}

	// 권한 체크
	if email.UserID != userID {
		return ErrorResponse(c, 403, "forbidden")
	}

	logger.Info("[EmailHandler.ResyncSingleEmail] User %s, Email %d, ExternalID %s",
		userID, emailID, email.ExternalID)

	// OAuth 토큰 가져오기
	token, err := h.oauthService.GetOAuth2Token(c.Context(), email.ConnectionID)
	if err != nil {
		logger.Error("[EmailHandler.ResyncSingleEmail] Failed to get OAuth token: %v", err)
		return ErrorResponse(c, 500, "failed to get oauth token")
	}

	// Provider에서 본문 + 첨부파일 정보 가져오기
	body, err := h.gmailProvider.GetMessageBody(c.Context(), token, email.ExternalID)
	if err != nil {
		logger.Error("[EmailHandler.ResyncSingleEmail] Failed to get message body: %v", err)
		return ErrorResponse(c, 500, "failed to fetch from provider")
	}

	// URL 기반 방식: 첨부파일 메타데이터는 DB에 저장하지 않음
	// has_attachment 플래그만 업데이트
	if len(body.Attachments) > 0 {
		if err := h.emailRepo.UpdateHasAttachment(c.Context(), int64(emailID), true); err != nil {
			logger.Warn("[EmailHandler.ResyncSingleEmail] Failed to update has_attachment: %v", err)
		}
	}

	return c.JSON(fiber.Map{
		"status":      "ok",
		"message":     "Email resynced",
		"attachments": len(body.Attachments),
	})
}

// resyncEmailsBackground performs email resync in background
// URL 기반 방식: 첨부파일 메타데이터는 DB에 저장하지 않음
// has_attachment 플래그만 업데이트
func (h *EmailHandler) resyncEmailsBackground(userID uuid.UUID, connectionID int64, token *oauth2.Token, emails []*out.MailEntity) {
	// Use timeout context instead of Background to prevent zombie goroutines
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	successCount := 0
	errorCount := 0

	for _, email := range emails {
		// Check context before each iteration
		if ctx.Err() != nil {
			logger.Warn("[EmailHandler.resyncEmailsBackground] Context cancelled, stopping. Processed: %d success, %d errors", successCount, errorCount)
			return
		}

		// Provider에서 본문 + 첨부파일 정보 가져오기
		body, err := h.gmailProvider.GetMessageBody(ctx, token, email.ExternalID)
		if err != nil {
			if ctx.Err() != nil {
				return // Context cancelled, stop silently
			}
			logger.Error("[EmailHandler.resyncEmailsBackground] Failed to get body for email %d: %v", email.ID, err)
			errorCount++
			continue
		}

		// has_attachment 플래그만 업데이트 (첨부파일 메타데이터는 DB에 저장하지 않음)
		if len(body.Attachments) > 0 {
			h.emailRepo.UpdateHasAttachment(ctx, email.ID, true)
		}

		successCount++
	}

	logger.Info("[EmailHandler.resyncEmailsBackground] Completed: %d success, %d errors", successCount, errorCount)
}

func (h *EmailHandler) ListEmails(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	filter := &domain.EmailFilter{
		UserID:       userID,
		ConnectionID: GetConnectionID(c),
		Limit:        20,
		Offset:       0,
	}

	// Parse all filter params
	filter.Folder = queryFolder(c, "folder")
	filter.FolderID = QueryInt64(c, "folder_id")
	filter.Category = queryCategory(c, "category")
	filter.SubCategory = querySubCategory(c, "sub_category")
	filter.Priority = queryPriority(c, "priority")
	filter.IsRead = QueryBool(c, "is_read")
	filter.IsStarred = QueryBool(c, "is_starred")
	filter.Search = QueryString(c, "search")
	filter.FromEmail = QueryString(c, "from_email")
	filter.FromDomain = QueryString(c, "from_domain")
	filter.WorkflowStatus = queryWorkflowStatus(c, "workflow_status")
	filter.LabelIDs = queryInt64Array(c, "label_ids")
	filter.DateFrom = queryTime(c, "date_from")
	filter.DateTo = queryTime(c, "date_to")
	filter.WorkflowStatus = queryWorkflowStatus(c, "workflow_status")

	// Pagination
	pagination := GetPaginationParams(c, 20)
	filter.Limit = pagination.Limit
	filter.Offset = pagination.Offset

	// 최대 payload 크기 제한 (메모리 보호)
	if h.apiProtector != nil && filter.Limit > h.apiProtector.MaxPayloadSize() {
		filter.Limit = h.apiProtector.MaxPayloadSize()
	}

	// =============================================================================
	// 1단계: 캐시 확인 (최신 메일만 캐시)
	// =============================================================================
	cacheKey := h.buildCacheKey(userID, filter)
	if h.emailCache != nil && h.emailCache.ShouldCache(filter.Offset) {
		if cachedData, found := h.emailCache.GetByString(c.Context(), cacheKey, filter.Offset); found {
			var cachedEmails []*domain.Email
			if err := json.Unmarshal(cachedData, &cachedEmails); err == nil {
				logger.Debug("[EmailHandler] Cache hit for %s", cacheKey)
				return c.JSON(fiber.Map{
					"emails":      cachedEmails,
					"total":       len(cachedEmails),
					"has_more":    len(cachedEmails) >= filter.Limit,
					"sync_status": "synced",
					"source":      "cache",
				})
			}
		}
	}

	// =============================================================================
	// 2단계: DB 조회 (항상 수행)
	// =============================================================================
	emails, total, err := h.emailService.ListEmails(c.Context(), filter)
	if err != nil {
		return InternalErrorResponse(c, err, "list emails")
	}

	hasMore := filter.Offset+len(emails) < total
	syncStatus := "synced"
	source := "db"

	// =============================================================================
	// 3단계: DB 부족 시 API 보충 (보호 레이어 적용)
	// 조건: DB 결과가 요청 개수보다 적고, connectionID가 있고, offset이 작을 때
	// 주의: AI 전용 필터 사용 시 API 호출 스킵 (Gmail/Outlook은 AI 분류를 모름)
	// =============================================================================
	if len(emails) < filter.Limit && filter.ConnectionID != nil {
		needed := filter.Limit - len(emails)

		// AI 전용 필터 체크: category, sub_category, priority, workflow_status, label_ids
		// 이 필터들은 우리 시스템에서만 존재하므로 API 호출해도 의미 없음
		providerOpts := BuildProviderFilterOptions(filter, needed, "")

		if providerOpts.SkipAPICall {
			// AI 필터 사용 시 DB 결과만 반환
			logger.Debug("[EmailHandler] Skipping API call: %s", providerOpts.SkipReason)
		} else {
			// offset이 작을 때만 API 호출 (오래된 메일은 API로 직접 조회)
			// offset이 크면 사용자가 스크롤 중 → 비동기 sync로 처리
			shouldCallAPI := filter.Offset < 100

			if shouldCallAPI && h.apiProtector != nil && h.oauthService != nil {
				// 보호 레이어 체크: Semaphore → Debounce → Rate Limiter
				protectKey := fmt.Sprintf("mail:list:%s:%d", userID.String(), *filter.ConnectionID)
				result, release := h.apiProtector.AcquireWithWait(c.Context(), protectKey, 2*time.Second)

				if result.Allowed && release != nil {
					defer release()

					// API 호출 (보호 레이어 통과) - 필터 옵션 전달
					apiEmails, apiHasMore, apiErr := h.fetchMoreFromProviderWithFilter(c, userID, *filter.ConnectionID, providerOpts, len(emails))
					if apiErr != nil {
						logger.Warn("[EmailHandler] API fetch failed: %v", apiErr)
						// API 실패해도 DB 결과는 반환
					} else if len(apiEmails) > 0 {
						// DB 결과에 API 결과 병합 (중복 제거)
						existingIDs := make(map[string]bool)
						for _, e := range emails {
							existingIDs[e.ProviderID] = true
						}
						for _, e := range apiEmails {
							if !existingIDs[e.ProviderID] {
								emails = append(emails, e)
								existingIDs[e.ProviderID] = true
							}
						}
						hasMore = apiHasMore
						source = "db+api"
						logger.Info("[EmailHandler] Supplemented %d emails from API (query: %s)", len(apiEmails), providerOpts.GmailQuery)
					}
				} else {
					// 보호 레이어에서 차단됨 → 비동기 동기화 요청
					logger.Info("[EmailHandler] API blocked (%s), requesting background sync", result.Reason)
					h.requestBackgroundSync(c, userID, *filter.ConnectionID)
					syncStatus = "syncing"
					hasMore = true
				}
			} else if filter.Offset >= 100 {
				// offset이 큰 경우 → 비동기 동기화만 요청
				if h.messageProducer != nil && filter.ConnectionID != nil {
					h.requestBackgroundSync(c, userID, *filter.ConnectionID)
					syncStatus = "syncing"
					hasMore = true
				}
			}
		}
	}

	// =============================================================================
	// 4단계: 캐시 저장 (최신 메일만)
	// =============================================================================
	if h.emailCache != nil && h.emailCache.ShouldCache(filter.Offset) && len(emails) > 0 {
		if cacheData, err := json.Marshal(emails); err == nil {
			h.emailCache.SetByString(c.Context(), cacheKey, filter.Offset, cacheData)
		}
	}

	return c.JSON(fiber.Map{
		"emails":      emails,
		"total":       total,
		"has_more":    hasMore,
		"sync_status": syncStatus,
		"source":      source,
	})
}

// buildCacheKey builds cache key for email list.
func (h *EmailHandler) buildCacheKey(userID uuid.UUID, filter *domain.EmailFilter) string {
	key := fmt.Sprintf("emails:%s", userID.String())
	if filter.ConnectionID != nil {
		key = fmt.Sprintf("%s:conn:%d", key, *filter.ConnectionID)
	}
	if filter.Folder != nil {
		key = fmt.Sprintf("%s:folder:%s", key, *filter.Folder)
	}
	key = fmt.Sprintf("%s:limit:%d:offset:%d", key, filter.Limit, filter.Offset)
	return key
}

// requestBackgroundSync requests background sync via Redis Stream.
func (h *EmailHandler) requestBackgroundSync(c *fiber.Ctx, userID uuid.UUID, connectionID int64) {
	if h.messageProducer == nil {
		return
	}

	job := &out.MailSyncJob{
		UserID:       userID.String(),
		ConnectionID: connectionID,
		Provider:     "google", // TODO: connection에서 provider 확인
		FullSync:     false,
	}
	if err := h.messageProducer.PublishMailSync(c.Context(), job); err != nil {
		logger.Warn("[EmailHandler] Failed to publish sync job: %v", err)
	}
}

// ListInbox returns only personal emails (primary, work, personal categories).
// This is the main view for users who want to see important emails only.
// GET /email/inbox?connection_id=1&limit=20&offset=0&is_read=false
func (h *EmailHandler) ListInbox(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	// Build filter with inbox view type
	filter := &domain.EmailFilter{
		UserID:       userID,
		ConnectionID: GetConnectionID(c),
		ViewType:     stringPtr("inbox"), // Inbox view: primary, work, personal only
		Limit:        20,
		Offset:       0,
	}

	// Optional filters
	filter.IsRead = QueryBool(c, "is_read")
	filter.IsStarred = QueryBool(c, "is_starred")
	filter.Priority = queryPriority(c, "min_priority") // minimum priority filter
	filter.Search = QueryString(c, "search")
	filter.DateFrom = queryTime(c, "date_from")
	filter.DateTo = queryTime(c, "date_to")
	filter.WorkflowStatus = queryWorkflowStatus(c, "workflow_status")

	// Pagination
	pagination := GetPaginationParams(c, 20)
	filter.Limit = pagination.Limit
	filter.Offset = pagination.Offset

	// Max payload size
	if h.apiProtector != nil && filter.Limit > h.apiProtector.MaxPayloadSize() {
		filter.Limit = h.apiProtector.MaxPayloadSize()
	}

	// Cache check
	cacheKey := fmt.Sprintf("inbox:%s:conn:%v:wf:%v:limit:%d:offset:%d",
		userID.String(), filter.ConnectionID, filter.WorkflowStatus, filter.Limit, filter.Offset)
	if h.emailCache != nil && h.emailCache.ShouldCache(filter.Offset) {
		if cachedData, found := h.emailCache.GetByString(c.Context(), cacheKey, filter.Offset); found {
			var cachedEmails []*domain.Email
			if err := json.Unmarshal(cachedData, &cachedEmails); err == nil {
				logger.Debug("[EmailHandler.ListInbox] Cache hit")
				return c.JSON(fiber.Map{
					"emails":   cachedEmails,
					"total":    len(cachedEmails),
					"has_more": len(cachedEmails) >= filter.Limit,
					"view":     "inbox",
					"source":   "cache",
				})
			}
		}
	}

	// DB query
	emails, total, err := h.emailService.ListEmails(c.Context(), filter)
	if err != nil {
		return InternalErrorResponse(c, err, "list inbox")
	}

	hasMore := filter.Offset+len(emails) < total

	// Cache store
	if h.emailCache != nil && h.emailCache.ShouldCache(filter.Offset) && len(emails) > 0 {
		if cacheData, err := json.Marshal(emails); err == nil {
			h.emailCache.SetByString(c.Context(), cacheKey, filter.Offset, cacheData)
		}
	}

	return c.JSON(fiber.Map{
		"emails":   emails,
		"total":    total,
		"has_more": hasMore,
		"view":     "inbox",
		"source":   "db",
	})
}

// ListByCategory returns emails filtered by a specific category.
// Supported categories: newsletter, notification, marketing, social, finance, travel, shopping, spam, other
// GET /email/category/newsletter?connection_id=1&limit=20&offset=0
func (h *EmailHandler) ListByCategory(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	// Get category from URL parameter
	category := c.Params("category")
	if category == "" {
		return ErrorResponse(c, 400, "category is required")
	}

	// Validate category
	validCategories := map[string]bool{
		"primary": true, "work": true, "personal": true,
		"newsletter": true, "notification": true, "marketing": true,
		"social": true, "finance": true, "travel": true,
		"shopping": true, "spam": true, "other": true,
	}
	if !validCategories[category] {
		return ErrorResponse(c, 400, "invalid category: "+category)
	}

	// Build filter
	cat := domain.EmailCategory(category)
	filter := &domain.EmailFilter{
		UserID:       userID,
		ConnectionID: GetConnectionID(c),
		Category:     &cat,
		Limit:        20,
		Offset:       0,
	}

	// Optional sub-category filter (e.g., /category/notification?sub_category=shipping)
	filter.SubCategory = querySubCategory(c, "sub_category")
	filter.IsRead = QueryBool(c, "is_read")
	filter.IsStarred = QueryBool(c, "is_starred")
	filter.Search = QueryString(c, "search")
	filter.DateFrom = queryTime(c, "date_from")
	filter.DateTo = queryTime(c, "date_to")
	filter.WorkflowStatus = queryWorkflowStatus(c, "workflow_status")

	// Pagination
	pagination := GetPaginationParams(c, 20)
	filter.Limit = pagination.Limit
	filter.Offset = pagination.Offset

	// Max payload size
	if h.apiProtector != nil && filter.Limit > h.apiProtector.MaxPayloadSize() {
		filter.Limit = h.apiProtector.MaxPayloadSize()
	}

	// Cache check
	cacheKey := fmt.Sprintf("category:%s:%s:conn:%v:wf:%v:limit:%d:offset:%d",
		category, userID.String(), filter.ConnectionID, filter.WorkflowStatus, filter.Limit, filter.Offset)
	if h.emailCache != nil && h.emailCache.ShouldCache(filter.Offset) {
		if cachedData, found := h.emailCache.GetByString(c.Context(), cacheKey, filter.Offset); found {
			var cachedEmails []*domain.Email
			if err := json.Unmarshal(cachedData, &cachedEmails); err == nil {
				logger.Debug("[EmailHandler.ListByCategory] Cache hit for %s", category)
				return c.JSON(fiber.Map{
					"emails":   cachedEmails,
					"total":    len(cachedEmails),
					"has_more": len(cachedEmails) >= filter.Limit,
					"category": category,
					"source":   "cache",
				})
			}
		}
	}

	// DB query
	emails, total, err := h.emailService.ListEmails(c.Context(), filter)
	if err != nil {
		return InternalErrorResponse(c, err, "list by category")
	}

	hasMore := filter.Offset+len(emails) < total

	// Cache store
	if h.emailCache != nil && h.emailCache.ShouldCache(filter.Offset) && len(emails) > 0 {
		if cacheData, err := json.Marshal(emails); err == nil {
			h.emailCache.SetByString(c.Context(), cacheKey, filter.Offset, cacheData)
		}
	}

	return c.JSON(fiber.Map{
		"emails":   emails,
		"total":    total,
		"has_more": hasMore,
		"category": category,
		"source":   "db",
	})
}

// stringPtr returns a pointer to a string.
func stringPtr(s string) *string {
	return &s
}

// ListTodo returns Inbox emails sorted by priority (highest first).
// This is the TODO view - same filter as Inbox but sorted by ai_priority DESC.
// GET /email/inbox/todo?connection_id=1&limit=20&offset=0
func (h *EmailHandler) ListTodo(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	filter := &domain.EmailFilter{
		UserID:       userID,
		ConnectionID: GetConnectionID(c),
		ViewType:     stringPtr("inbox"),
		SortBy:       "priority", // Sort by ai_priority DESC
		Limit:        20,
		Offset:       0,
	}

	filter.IsRead = QueryBool(c, "is_read")
	filter.IsStarred = QueryBool(c, "is_starred")
	filter.Search = QueryString(c, "search")

	pagination := GetPaginationParams(c, 20)
	filter.Limit = pagination.Limit
	filter.Offset = pagination.Offset

	if h.apiProtector != nil && filter.Limit > h.apiProtector.MaxPayloadSize() {
		filter.Limit = h.apiProtector.MaxPayloadSize()
	}

	emails, total, err := h.emailService.ListEmails(c.Context(), filter)
	if err != nil {
		return InternalErrorResponse(c, err, "list todo")
	}

	return c.JSON(fiber.Map{
		"emails":   emails,
		"total":    total,
		"has_more": filter.Offset+len(emails) < total,
		"view":     "todo",
		"source":   "db",
	})
}

// ListSent returns sent emails.
// GET /email/sent?connection_id=1&limit=20&offset=0
func (h *EmailHandler) ListSent(c *fiber.Ctx) error {
	return h.listByFolder(c, "sent")
}

// ListDrafts returns draft emails.
// GET /email/drafts?connection_id=1&limit=20&offset=0
func (h *EmailHandler) ListDrafts(c *fiber.Ctx) error {
	return h.listByFolder(c, "drafts")
}

// ListTrash returns trashed emails.
// GET /email/trash?connection_id=1&limit=20&offset=0
func (h *EmailHandler) ListTrash(c *fiber.Ctx) error {
	return h.listByFolder(c, "trash")
}

// ListSpam returns spam emails.
// GET /email/spam?connection_id=1&limit=20&offset=0
func (h *EmailHandler) ListSpam(c *fiber.Ctx) error {
	return h.listByFolder(c, "spam")
}

// ListArchive returns archived emails.
// GET /email/archive?connection_id=1&limit=20&offset=0
func (h *EmailHandler) ListArchive(c *fiber.Ctx) error {
	return h.listByFolder(c, "archive")
}

// listByFolder is a helper to list emails by folder.
func (h *EmailHandler) listByFolder(c *fiber.Ctx, folder string) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	folderVal := domain.LegacyFolder(folder)
	filter := &domain.EmailFilter{
		UserID:       userID,
		ConnectionID: GetConnectionID(c),
		Folder:       &folderVal,
		Limit:        20,
		Offset:       0,
	}

	filter.IsRead = QueryBool(c, "is_read")
	filter.IsStarred = QueryBool(c, "is_starred")
	filter.Search = QueryString(c, "search")
	filter.Category = queryCategory(c, "category")
	filter.DateFrom = queryTime(c, "date_from")
	filter.DateTo = queryTime(c, "date_to")

	pagination := GetPaginationParams(c, 20)
	filter.Limit = pagination.Limit
	filter.Offset = pagination.Offset

	if h.apiProtector != nil && filter.Limit > h.apiProtector.MaxPayloadSize() {
		filter.Limit = h.apiProtector.MaxPayloadSize()
	}

	emails, total, err := h.emailService.ListEmails(c.Context(), filter)
	if err != nil {
		return InternalErrorResponse(c, err, "list "+folder)
	}

	return c.JSON(fiber.Map{
		"emails":   emails,
		"total":    total,
		"has_more": filter.Offset+len(emails) < total,
		"folder":   folder,
		"source":   "db",
	})
}

// ListEmailsUnified lists emails using unified provider with cursor-based pagination.
// 모든 연결된 계정(Gmail, Outlook)을 통합 조회하고, 커서 기반 페이징을 지원합니다.
func (h *EmailHandler) ListEmailsUnified(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	if h.unifiedProvider == nil {
		return ErrorResponse(c, 500, "unified provider not configured")
	}

	// Parse options
	limit := c.QueryInt("limit", 30)
	if limit > 100 {
		limit = 100
	}

	var folder *string
	if f := c.Query("folder"); f != "" {
		folder = &f
	}

	var search *string
	if s := c.Query("search"); s != "" {
		search = &s
	}

	// Decode cursor
	var cursor *provider.UnifiedCursor
	if cursorStr := c.Query("cursor"); cursorStr != "" {
		cursor = provider.DecodeCursor(cursorStr)
	}

	// Call unified provider
	result, err := h.unifiedProvider.ListAll(c.Context(), &provider.UnifiedListOptions{
		UserID: userID,
		Limit:  limit,
		Cursor: cursor,
		Folder: folder,
		Search: search,
	})
	if err != nil {
		return InternalErrorResponse(c, err, "list emails unified")
	}

	// Encode next cursor
	var nextCursor string
	if result.NextCursor != nil {
		nextCursor = provider.EncodeCursor(result.NextCursor)
	}

	return c.JSON(fiber.Map{
		"emails":      result.Emails,
		"total":       result.Total,
		"has_more":    result.HasMore,
		"next_cursor": nextCursor,
	})
}

// SearchEmails searches emails using Gmail API directly (전체 메일 검색)
func (h *EmailHandler) SearchEmails(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	query := c.Query("q")
	if query == "" {
		return ErrorResponse(c, 400, "query parameter 'q' is required")
	}

	connectionID := c.QueryInt("connection_id", 0)
	if connectionID == 0 {
		return ErrorResponse(c, 400, "connection_id required")
	}

	limit := c.QueryInt("limit", 20)
	if limit > 100 {
		limit = 100
	}

	// DB 검색 또는 Gmail API 검색 선택
	source := c.Query("source", "all") // "db", "gmail", "all"

	var emails []*domain.Email
	var total int
	var hasMore bool

	// DB 검색
	if source == "db" || source == "all" {
		filter := &domain.EmailFilter{
			UserID:       userID,
			ConnectionID: &[]int64{int64(connectionID)}[0],
			Limit:        limit,
			Search:       &query,
		}
		dbEmails, dbTotal, err := h.emailService.ListEmails(c.Context(), filter)
		if err != nil {
			logger.WithError(err).Warn("[EmailHandler.SearchEmails] DB search failed")
		} else {
			emails = append(emails, dbEmails...)
			total = dbTotal
		}
	}

	// Provider API 검색 (source가 provider 또는 all이고, DB 결과가 부족할 때)
	if (source == "gmail" || source == "outlook" || source == "provider" || source == "all") && len(emails) < limit {
		if h.oauthService != nil {
			providerEmails, providerHasMore, err := h.searchViaProvider(c, userID, int64(connectionID), query, limit-len(emails))
			if err != nil {
				logger.WithError(err).Warn("[EmailHandler.SearchEmails] Provider search failed")
			} else {
				// 중복 제거하고 추가
				existingIDs := make(map[string]bool)
				for _, e := range emails {
					existingIDs[e.ProviderID] = true
				}
				for _, e := range providerEmails {
					if !existingIDs[e.ProviderID] {
						emails = append(emails, e)
					}
				}
				hasMore = providerHasMore
			}
		}
	}

	return c.JSON(fiber.Map{
		"emails":   emails,
		"total":    total,
		"has_more": hasMore,
		"source":   source,
		"query":    query,
	})
}

// SearchEmailsV2 performs unified search across DB, Vector, and Provider.
// GET /email/search/v2?q=query&connection_id=123&strategy=balanced&limit=20
//
// Strategies:
//   - fast: DB only (fastest, ~50ms)
//   - balanced: DB + Vector, Provider fallback (default, ~150ms)
//   - complete: All sources (slowest, ~1-2s)
//   - semantic: Vector only (for natural language queries)
//   - provider: Provider API only
func (h *EmailHandler) SearchEmailsV2(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	query := c.Query("q")
	if query == "" {
		return ErrorResponse(c, 400, "query parameter 'q' is required")
	}

	connectionID := c.QueryInt("connection_id", 0)
	if connectionID == 0 {
		return ErrorResponse(c, 400, "connection_id required")
	}

	// Check if search service is available
	if h.searchService == nil {
		// Fallback to legacy search
		return h.SearchEmails(c)
	}

	// Parse parameters
	limit := c.QueryInt("limit", 20)
	if limit > 100 {
		limit = 100
	}
	offset := c.QueryInt("offset", 0)

	// Parse strategy
	strategyStr := c.Query("strategy", "balanced")
	strategy := search.SearchStrategy(strategyStr)

	// Build search request
	req := &search.SearchRequest{
		UserID:       userID,
		ConnectionID: int64(connectionID),
		Query:        query,
		Strategy:     strategy,
		Limit:        limit,
		Offset:       offset,
	}

	// Parse optional filters
	if from := c.Query("from"); from != "" {
		if req.Filters == nil {
			req.Filters = &search.SearchFilters{}
		}
		req.Filters.From = &from
	}
	if category := c.Query("category"); category != "" {
		if req.Filters == nil {
			req.Filters = &search.SearchFilters{}
		}
		req.Filters.Category = &category
	}

	// Create provider search function
	providerSearchFunc := h.createProviderSearchFunc(c, int64(connectionID))

	// Get OAuth token for provider search
	var token *oauth2.Token
	if h.oauthService != nil {
		token, _ = h.oauthService.GetOAuth2Token(c.Context(), int64(connectionID))
	}

	// Execute unified search
	response, err := h.searchService.Search(c.Context(), req, providerSearchFunc, token)
	if err != nil {
		return InternalErrorResponse(c, err, "search failed")
	}

	// Convert to API response format
	return c.JSON(fiber.Map{
		"results":        response.Results,
		"total":          response.Total,
		"has_more":       response.HasMore,
		"next_cursor":    response.NextCursor,
		"sources":        response.Sources,
		"time_ms":        response.TimeTaken,
		"strategy":       response.Strategy,
		"intent":         response.Intent,
		"query":          query,
		"db_count":       response.DBCount,
		"vector_count":   response.VectorCount,
		"provider_count": response.ProviderCount,
	})
}

// createProviderSearchFunc creates a provider-specific search function.
func (h *EmailHandler) createProviderSearchFunc(c *fiber.Ctx, connectionID int64) search.ProviderSearchFunc {
	return func(ctx context.Context, token *oauth2.Token, query string, limit int) ([]*search.SearchResult, error) {
		if h.oauthService == nil {
			return nil, nil
		}

		conn, err := h.oauthService.GetConnection(ctx, connectionID)
		if err != nil {
			return nil, err
		}

		var result *out.ProviderListResult

		switch conn.Provider {
		case "gmail":
			if h.gmailProvider == nil {
				return nil, nil
			}
			result, err = h.gmailProvider.ListMessages(ctx, token, &out.ProviderListOptions{
				MaxResults: limit,
				Query:      query,
			})
		case "outlook":
			if h.outlookProvider == nil {
				return nil, nil
			}
			result, err = h.outlookProvider.ListMessages(ctx, token, &out.ProviderListOptions{
				MaxResults: limit,
				Query:      query,
			})
		default:
			return nil, nil
		}

		if err != nil {
			return nil, err
		}

		// Convert provider results to SearchResult
		results := make([]*search.SearchResult, 0, len(result.Messages))
		for _, msg := range result.Messages {
			from := ""
			if msg.From.Email != "" {
				from = msg.From.Email
			}

			to := make([]string, 0, len(msg.To))
			for _, t := range msg.To {
				to = append(to, t.Email)
			}

			results = append(results, &search.SearchResult{
				ProviderID: msg.ExternalID,
				Subject:    msg.Subject,
				Snippet:    msg.Snippet,
				From:       from,
				To:         to,
				Date:       msg.Date,
				IsRead:     msg.IsRead,
				HasAttach:  msg.HasAttachment,
				Folder:     msg.Folder,
				Source:     search.SourceProvider,
				Score:      1.0,
			})
		}

		return results, nil
	}
}

// searchViaProvider searches emails using Gmail or Outlook API
func (h *EmailHandler) searchViaProvider(c *fiber.Ctx, userID uuid.UUID, connectionID int64, query string, limit int) ([]*domain.Email, bool, error) {
	ctx := c.Context()

	token, err := h.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return nil, false, err
	}

	conn, err := h.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return nil, false, err
	}

	// Provider에 따라 API 선택
	var result *out.ProviderListResult
	switch conn.Provider {
	case "gmail":
		if h.gmailProvider == nil {
			return nil, false, nil
		}
		result, err = h.gmailProvider.ListMessages(ctx, token, &out.ProviderListOptions{
			MaxResults: limit,
			Query:      query, // Gmail 검색 문법 지원 (from:, to:, subject:, has:attachment 등)
		})
	case "outlook":
		if h.outlookProvider == nil {
			return nil, false, nil
		}
		result, err = h.outlookProvider.ListMessages(ctx, token, &out.ProviderListOptions{
			MaxResults: limit,
			Query:      query, // Outlook은 $search 파라미터 사용
		})
	default:
		return nil, false, nil
	}

	if err != nil {
		return nil, false, err
	}

	// 1. ExternalID 목록 추출
	externalIDs := make([]string, len(result.Messages))
	for i, msg := range result.Messages {
		externalIDs[i] = msg.ExternalID
	}

	// 2. 배치 조회로 기존 메일 확인
	var existingMap map[string]*out.MailEntity
	if h.emailRepo != nil && len(externalIDs) > 0 {
		var err error
		existingMap, err = h.emailRepo.GetByExternalIDs(ctx, connectionID, externalIDs)
		if err != nil {
			logger.WithFields(map[string]any{
				"connection_id":      connectionID,
				"external_ids_count": len(externalIDs),
			}).WithError(err).Warn("[EmailHandler] Failed to check existing emails, treating as new")
		}
	}
	if existingMap == nil {
		existingMap = make(map[string]*out.MailEntity)
	}

	// 3. 결과 변환 (DB 저장은 Worker에서)
	var emails []*domain.Email
	var saveEmails []out.MailSaveEmail

	for _, msg := range result.Messages {
		email := h.convertProviderMessage(msg, userID, connectionID, conn.Email, string(conn.Provider))

		// 기존 메일이면 ID 설정
		if existing, ok := existingMap[msg.ExternalID]; ok {
			email.ID = existing.ID
		} else {
			// 새 메일은 Worker용 데이터에 추가
			saveEmails = append(saveEmails, out.MailSaveEmail{
				ExternalID: msg.ExternalID,
				ThreadID:   msg.ExternalThreadID,
				Subject:    msg.Subject,
				FromEmail:  msg.From.Email,
				FromName:   msg.From.Name,
				ToEmails:   extractEmails(msg.To),
				CcEmails:   extractEmails(msg.CC),
				Snippet:    msg.Snippet,
				IsRead:     msg.IsRead,
				HasAttach:  msg.HasAttachment,
				Folder:     msg.Folder,
				Labels:     msg.Labels,
				ReceivedAt: msg.ReceivedAt,
			})
		}

		emails = append(emails, email)
	}

	// 4. Redis Stream으로 DB 저장 작업 발행 (Worker에서 처리)
	if h.messageProducer != nil && len(saveEmails) > 0 {
		job := &out.MailSaveJob{
			UserID:       userID.String(),
			ConnectionID: connectionID,
			AccountEmail: conn.Email,
			Provider:     string(conn.Provider),
			Emails:       saveEmails,
		}
		if err := h.messageProducer.PublishMailSave(ctx, job); err != nil {
			logger.Warn("[EmailHandler] Failed to publish mail save job: %v", err)
		}
	}

	hasMore := result.NextPageToken != ""
	return emails, hasMore, nil
}

// fetchMoreFromProviderWithFilter fetches emails from provider API with filter support.
// Gmail: uses q parameter for search query
// Outlook: uses $filter OData parameter
func (h *EmailHandler) fetchMoreFromProviderWithFilter(c *fiber.Ctx, userID uuid.UUID, connectionID int64, opts *ProviderFilterOptions, offset int) ([]*domain.Email, bool, error) {
	ctx := c.Context()

	// Get OAuth token
	token, err := h.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return nil, false, err
	}

	// Get connection info
	conn, err := h.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return nil, false, err
	}

	// Gmail API는 offset을 지원하지 않으므로 더 많이 가져와서 스킵
	fetchLimit := opts.MaxResults + offset
	if fetchLimit > 500 {
		fetchLimit = 500
	}

	var listResult *out.ProviderListResult

	// Provider별 API 호출
	switch conn.Provider {
	case "google", "gmail":
		if h.gmailProvider == nil {
			return nil, false, fmt.Errorf("gmail provider not configured")
		}
		// Gmail: q 파라미터로 필터 전달
		listResult, err = h.gmailProvider.ListMessages(ctx, token, &out.ProviderListOptions{
			MaxResults: fetchLimit,
			Query:      opts.GmailQuery, // "in:inbox is:unread from:x@y.com after:2024/01/01"
			PageToken:  opts.PageToken,
		})
	case "outlook", "microsoft":
		if h.outlookProvider == nil {
			return nil, false, fmt.Errorf("outlook provider not configured")
		}
		// Outlook: $filter 파라미터로 필터 전달
		listResult, err = h.outlookProvider.ListMessages(ctx, token, &out.ProviderListOptions{
			MaxResults: fetchLimit,
			Query:      opts.OutlookFilter, // "isRead eq false and from/emailAddress/address eq 'x@y.com'"
			PageToken:  opts.PageToken,
			// Outlook folder는 별도 경로로 처리 (TODO: OutlookFolder 지원 필요시 추가)
		})
	default:
		return nil, false, fmt.Errorf("unsupported provider: %s", conn.Provider)
	}

	if err != nil {
		return nil, false, err
	}

	// ExternalID 목록 추출 (배치 조회용)
	externalIDs := make([]string, len(listResult.Messages))
	for i, msg := range listResult.Messages {
		externalIDs[i] = msg.ExternalID
	}

	// 배치 조회로 기존 메일 확인
	var existingMap map[string]*out.MailEntity
	if h.emailRepo != nil && len(externalIDs) > 0 {
		var err error
		existingMap, err = h.emailRepo.GetByExternalIDs(ctx, connectionID, externalIDs)
		if err != nil {
			logger.WithFields(map[string]any{
				"connection_id":      connectionID,
				"external_ids_count": len(externalIDs),
			}).WithError(err).Warn("[EmailHandler] Failed to check existing emails, treating as new")
		}
	}
	if existingMap == nil {
		existingMap = make(map[string]*out.MailEntity)
	}

	// 메시지 변환 (DB 저장은 Worker에서)
	var emails []*domain.Email
	var saveEmails []out.MailSaveEmail

	for i, msg := range listResult.Messages {
		if i < offset {
			continue
		}

		email := h.convertProviderMessage(msg, userID, connectionID, conn.Email)

		// 기존 메일이면 ID 설정
		if existing, ok := existingMap[msg.ExternalID]; ok {
			email.ID = existing.ID
		} else {
			// 새 메일은 Worker용 데이터에 추가
			saveEmails = append(saveEmails, out.MailSaveEmail{
				ExternalID: msg.ExternalID,
				ThreadID:   msg.ExternalThreadID,
				Subject:    msg.Subject,
				FromEmail:  msg.From.Email,
				FromName:   msg.From.Name,
				ToEmails:   extractEmails(msg.To),
				CcEmails:   extractEmails(msg.CC),
				Snippet:    msg.Snippet,
				IsRead:     msg.IsRead,
				HasAttach:  msg.HasAttachment,
				Folder:     msg.Folder,
				Labels:     msg.Labels,
				ReceivedAt: msg.ReceivedAt,
			})
		}

		emails = append(emails, email)

		if len(emails) >= opts.MaxResults {
			break
		}
	}

	// Redis Stream으로 DB 저장 작업 발행 (Worker에서 처리)
	if h.messageProducer != nil && len(saveEmails) > 0 {
		job := &out.MailSaveJob{
			UserID:       userID.String(),
			ConnectionID: connectionID,
			AccountEmail: conn.Email,
			Provider:     string(conn.Provider),
			Emails:       saveEmails,
		}
		if err := h.messageProducer.PublishMailSave(ctx, job); err != nil {
			logger.Warn("[EmailHandler] Failed to publish mail save job: %v", err)
		}
	}

	hasMore := listResult.NextPageToken != ""
	return emails, hasMore, nil
}

// fetchMoreFromProvider fetches more emails from provider API (하이브리드 무한 스크롤용)
// Deprecated: Use fetchMoreFromProviderWithFilter instead
func (h *EmailHandler) fetchMoreFromProvider(c *fiber.Ctx, userID uuid.UUID, connectionID int64, limit int, offset int) ([]*domain.Email, bool, error) {
	ctx := c.Context()

	// Get OAuth token
	token, err := h.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return nil, false, err
	}

	// Get connection info
	conn, err := h.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return nil, false, err
	}

	// Gmail API는 offset을 지원하지 않으므로 더 많이 가져와서 스킵
	// 실제로는 pageToken 기반이지만, 간단하게 구현
	fetchLimit := limit + offset
	if fetchLimit > 500 {
		fetchLimit = 500 // 최대 제한
	}

	// Fetch from Gmail using ListMessages (더 효율적)
	listResult, err := h.gmailProvider.ListMessages(ctx, token, &out.ProviderListOptions{
		MaxResults: fetchLimit,
	})
	if err != nil {
		return nil, false, err
	}

	// 1. ExternalID 목록 추출 (배치 조회용)
	externalIDs := make([]string, len(listResult.Messages))
	for i, msg := range listResult.Messages {
		externalIDs[i] = msg.ExternalID
	}

	// 2. 배치 조회로 기존 메일 확인
	var existingMap map[string]*out.MailEntity
	if h.emailRepo != nil && len(externalIDs) > 0 {
		var err error
		existingMap, err = h.emailRepo.GetByExternalIDs(ctx, connectionID, externalIDs)
		if err != nil {
			logger.WithFields(map[string]any{
				"connection_id":      connectionID,
				"external_ids_count": len(externalIDs),
			}).WithError(err).Warn("[EmailHandler] Failed to check existing emails, treating as new")
		}
	}
	if existingMap == nil {
		existingMap = make(map[string]*out.MailEntity)
	}

	// 3. 메시지 변환 (DB 저장은 Worker에서)
	var emails []*domain.Email
	var saveEmails []out.MailSaveEmail

	for i, msg := range listResult.Messages {
		if i < offset {
			continue
		}

		email := h.convertProviderMessage(msg, userID, connectionID, conn.Email)

		// 기존 메일이면 ID 설정
		if existing, ok := existingMap[msg.ExternalID]; ok {
			email.ID = existing.ID
		} else {
			// 새 메일은 Worker용 데이터에 추가
			saveEmails = append(saveEmails, out.MailSaveEmail{
				ExternalID: msg.ExternalID,
				ThreadID:   msg.ExternalThreadID,
				Subject:    msg.Subject,
				FromEmail:  msg.From.Email,
				FromName:   msg.From.Name,
				ToEmails:   extractEmails(msg.To),
				CcEmails:   extractEmails(msg.CC),
				Snippet:    msg.Snippet,
				IsRead:     msg.IsRead,
				HasAttach:  msg.HasAttachment,
				Folder:     msg.Folder,
				Labels:     msg.Labels,
				ReceivedAt: msg.ReceivedAt,
			})
		}

		emails = append(emails, email)

		if len(emails) >= limit {
			break
		}
	}

	// 4. Redis Stream으로 DB 저장 작업 발행 (Worker에서 처리)
	if h.messageProducer != nil && len(saveEmails) > 0 {
		job := &out.MailSaveJob{
			UserID:       userID.String(),
			ConnectionID: connectionID,
			AccountEmail: conn.Email,
			Provider:     string(conn.Provider),
			Emails:       saveEmails,
		}
		if err := h.messageProducer.PublishMailSave(ctx, job); err != nil {
			logger.Warn("[EmailHandler] Failed to publish mail save job: %v", err)
		}
	}

	hasMore := listResult.NextPageToken != ""
	return emails, hasMore, nil
}

// fetchFromProviderWithToken fetches emails from Gmail API with pageToken support
// 철학: "메일은 즉시 반환, DB 저장은 Worker에서"
func (h *EmailHandler) fetchFromProviderWithToken(c *fiber.Ctx, userID uuid.UUID, connectionID int64, limit int, pageToken string) ([]*domain.Email, string, error) {
	ctx := c.Context()

	// Get OAuth token
	token, err := h.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return nil, "", err
	}

	// Get connection info
	conn, err := h.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return nil, "", err
	}

	// Fetch from Gmail with pageToken
	listResult, err := h.gmailProvider.ListMessages(ctx, token, &out.ProviderListOptions{
		MaxResults: limit,
		PageToken:  pageToken,
	})
	if err != nil {
		return nil, "", err
	}

	// 1. ExternalID 목록 추출
	externalIDs := make([]string, len(listResult.Messages))
	for i, msg := range listResult.Messages {
		externalIDs[i] = msg.ExternalID
	}

	// 2. 배치 조회 (1번의 DB 쿼리로 N+1 문제 해결)
	var existingMap map[string]*out.MailEntity
	if h.emailRepo != nil && len(externalIDs) > 0 {
		var err error
		existingMap, err = h.emailRepo.GetByExternalIDs(ctx, connectionID, externalIDs)
		if err != nil {
			logger.WithFields(map[string]any{
				"connection_id":      connectionID,
				"external_ids_count": len(externalIDs),
			}).WithError(err).Warn("[EmailHandler] Failed to check existing emails, treating as new")
		}
	}
	if existingMap == nil {
		existingMap = make(map[string]*out.MailEntity)
	}

	// 3. 메시지 변환 및 새 메일 필터링
	emails := make([]*domain.Email, 0, len(listResult.Messages))
	saveEmails := make([]out.MailSaveEmail, 0, len(listResult.Messages))

	for _, msg := range listResult.Messages {
		email := h.convertProviderMessage(msg, userID, connectionID, conn.Email)

		// 기존 메일이면 ID 설정
		if existing, ok := existingMap[msg.ExternalID]; ok {
			email.ID = existing.ID
		}

		emails = append(emails, email)

		// 새 메일만 Worker용 데이터에 추가
		if _, exists := existingMap[msg.ExternalID]; !exists {
			saveEmails = append(saveEmails, out.MailSaveEmail{
				ExternalID: msg.ExternalID,
				ThreadID:   msg.ExternalThreadID,
				Subject:    msg.Subject,
				FromEmail:  msg.From.Email,
				FromName:   msg.From.Name,
				ToEmails:   extractEmails(msg.To),
				CcEmails:   extractEmails(msg.CC),
				Snippet:    msg.Snippet,
				IsRead:     msg.IsRead,
				HasAttach:  msg.HasAttachment,
				Folder:     msg.Folder,
				Labels:     msg.Labels,
				ReceivedAt: msg.ReceivedAt,
			})
		}
	}

	// Redis Stream으로 DB 저장 작업 발행 (새 메일만, Worker에서 처리)
	if h.messageProducer != nil && len(saveEmails) > 0 {
		logger.Info("[EmailHandler] Publishing %d new emails to save (skipped %d existing)", len(saveEmails), len(emails)-len(saveEmails))
		job := &out.MailSaveJob{
			UserID:       userID.String(),
			ConnectionID: connectionID,
			AccountEmail: conn.Email,
			Provider:     string(conn.Provider),
			Emails:       saveEmails,
		}
		if err := h.messageProducer.PublishMailSave(ctx, job); err != nil {
			logger.Warn("[EmailHandler] Failed to publish mail save job: %v", err)
		}
	}

	return emails, listResult.NextPageToken, nil
}

// extractEmails extracts email addresses from ProviderEmailAddress slice
func extractEmails(addrs []out.ProviderEmailAddress) []string {
	result := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		result = append(result, addr.Email)
	}
	return result
}

// fetchFromProvider fetches emails directly from Gmail API (DB 저장은 Worker에서)
func (h *EmailHandler) fetchFromProvider(c *fiber.Ctx, userID uuid.UUID, connectionID int64, limit int) ([]*domain.Email, error) {
	ctx := c.Context()

	// Get OAuth token
	token, err := h.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	// Get connection info
	conn, err := h.oauthService.GetConnection(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	// Fetch from Gmail
	syncResult, err := h.gmailProvider.InitialSync(ctx, token, &out.ProviderSyncOptions{
		MaxResults: limit,
	})
	if err != nil {
		return nil, err
	}

	// 1. ExternalID 목록 추출
	externalIDs := make([]string, len(syncResult.Messages))
	for i, msg := range syncResult.Messages {
		externalIDs[i] = msg.ExternalID
	}

	// 2. 배치 조회로 기존 메일 확인
	var existingMap map[string]*out.MailEntity
	if h.emailRepo != nil && len(externalIDs) > 0 {
		var err error
		existingMap, err = h.emailRepo.GetByExternalIDs(ctx, connectionID, externalIDs)
		if err != nil {
			logger.WithFields(map[string]any{
				"connection_id":      connectionID,
				"external_ids_count": len(externalIDs),
			}).WithError(err).Warn("[EmailHandler] Failed to check existing emails, treating as new")
		}
	}
	if existingMap == nil {
		existingMap = make(map[string]*out.MailEntity)
	}

	// 3. 메시지 변환 (DB 저장은 Worker에서)
	var emails []*domain.Email
	var saveEmails []out.MailSaveEmail

	for _, msg := range syncResult.Messages {
		email := h.convertProviderMessage(msg, userID, connectionID, conn.Email)

		// 기존 메일이면 ID 설정
		if existing, ok := existingMap[msg.ExternalID]; ok {
			email.ID = existing.ID
		} else {
			// 새 메일은 Worker용 데이터에 추가
			saveEmails = append(saveEmails, out.MailSaveEmail{
				ExternalID: msg.ExternalID,
				ThreadID:   msg.ExternalThreadID,
				Subject:    msg.Subject,
				FromEmail:  msg.From.Email,
				FromName:   msg.From.Name,
				ToEmails:   extractEmails(msg.To),
				CcEmails:   extractEmails(msg.CC),
				Snippet:    msg.Snippet,
				IsRead:     msg.IsRead,
				HasAttach:  msg.HasAttachment,
				Folder:     msg.Folder,
				Labels:     msg.Labels,
				ReceivedAt: msg.ReceivedAt,
			})
		}

		emails = append(emails, email)
	}

	// 4. Redis Stream으로 DB 저장 작업 발행 (Worker에서 처리)
	if h.messageProducer != nil && len(saveEmails) > 0 {
		job := &out.MailSaveJob{
			UserID:       userID.String(),
			ConnectionID: connectionID,
			AccountEmail: conn.Email,
			Provider:     string(conn.Provider),
			Emails:       saveEmails,
		}
		if err := h.messageProducer.PublishMailSave(ctx, job); err != nil {
			logger.Warn("[EmailHandler] Failed to publish mail save job: %v", err)
		}
	}

	return emails, nil
}

func (h *EmailHandler) convertProviderMessage(msg out.ProviderMailMessage, userID uuid.UUID, connectionID int64, accountEmail string, providerType ...string) *domain.Email {
	var fromName *string
	if msg.From.Name != "" {
		fromName = &msg.From.Name
	}

	// 기본값 google, 파라미터로 받으면 해당 값 사용
	emailProvider := domain.MailProviderGmail
	if len(providerType) > 0 && providerType[0] == "outlook" {
		emailProvider = domain.MailProviderOutlook
	}

	email := &domain.Email{
		UserID:       userID,
		ConnectionID: connectionID,
		Provider:     emailProvider,
		ProviderID:   msg.ExternalID,
		ThreadID:     msg.ExternalThreadID,
		Subject:      msg.Subject,
		FromEmail:    msg.From.Email,
		FromName:     fromName,
		Snippet:      msg.Snippet,
		IsRead:       msg.IsRead,
		HasAttach:    msg.HasAttachment,
		Folder:       domain.LegacyFolder(msg.Folder),
		Labels:       msg.Labels,
		ReceivedAt:   msg.ReceivedAt,
		Date:         msg.ReceivedAt,
	}

	for _, to := range msg.To {
		email.ToEmails = append(email.ToEmails, to.Email)
	}
	for _, cc := range msg.CC {
		email.CcEmails = append(email.CcEmails, cc.Email)
	}

	return email
}

func (h *EmailHandler) domainToEntity(d *domain.Email, connectionID int64, accountEmail string) *out.MailEntity {
	var fromName string
	if d.FromName != nil {
		fromName = *d.FromName
	}

	return &out.MailEntity{
		UserID:       d.UserID,
		ConnectionID: connectionID,
		Provider:     string(d.Provider),
		AccountEmail: accountEmail,
		ExternalID:   d.ProviderID,
		FromEmail:    d.FromEmail,
		FromName:     fromName,
		ToEmails:     d.ToEmails,
		CcEmails:     d.CcEmails,
		Subject:      d.Subject,
		IsRead:       d.IsRead,
		Folder:       string(d.Folder),
		Labels:       d.Labels,
		ReceivedAt:   d.ReceivedAt,
		AIStatus:     "pending",
	}
}

func (h *EmailHandler) GetEmail(c *fiber.Ctx) error {
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
		return InternalErrorResponse(c, err, "get email")
	}

	return c.JSON(email)
}

func (h *EmailHandler) GetEmailBody(c *fiber.Ctx) error {
	emailID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	body, err := h.emailService.GetEmailBody(c.Context(), emailID)
	if err != nil {
		return InternalErrorResponse(c, err, "get email body")
	}

	// 첨부파일 조회해서 body에 포함
	if h.attachmentRepo != nil {
		attEntities, err := h.attachmentRepo.GetByEmailID(c.Context(), emailID)
		if err != nil {
			logger.Warn("[GetEmailBody] Failed to get attachments for email %d: %v", emailID, err)
		}
		logger.Info("[GetEmailBody] Found %d attachments in DB for email %d", len(attEntities), emailID)
		for _, att := range attEntities {
			contentID := ""
			if att.ContentID != nil {
				contentID = *att.ContentID
			}
			body.Attachments = append(body.Attachments, &domain.Attachment{
				ID:         att.ID,
				EmailID:    att.EmailID,
				ExternalID: att.ExternalID,
				Filename:   att.Filename,
				MimeType:   att.MimeType,
				Size:       att.Size,
				ContentID:  contentID,
				IsInline:   att.IsInline,
			})
		}
	} else {
		logger.Warn("[GetEmailBody] attachmentRepo is nil, cannot fetch attachments")
	}

	// Replace CID references with Base64 data URLs (inline images)
	// This avoids authentication issues with <img src="..."> requests
	replaceCID := c.QueryBool("replace_cid", true) // default: true
	if replaceCID && body != nil && body.HTMLBody != "" && h.attachmentRepo != nil {
		body.HTMLBody = h.replaceCIDWithBase64(c.Context(), emailID, body.HTMLBody)
	}

	return c.JSON(body)
}

// replaceCIDWithBase64 replaces cid: references in HTML with Base64 data URLs.
// This solves the authentication issue where <img src="..."> requests don't include auth headers.
// e.g., src="cid:image001" -> src="data:image/png;base64,iVBORw0KGgo..."
func (h *EmailHandler) replaceCIDWithBase64(ctx context.Context, emailID int64, html string) string {
	if h.attachmentRepo == nil || h.emailRepo == nil {
		return html
	}

	// Get inline attachments for this email
	inlineAttachments, err := h.attachmentRepo.GetInlineByEmailID(ctx, emailID)
	if err != nil || len(inlineAttachments) == 0 {
		return html
	}

	// Get email to find provider info
	email, err := h.emailRepo.GetByID(ctx, emailID)
	if err != nil || email == nil {
		logger.WithError(err).WithField("email_id", emailID).Warn("[replaceCIDWithBase64] Failed to get email")
		return html
	}

	// Get OAuth token
	token, err := h.oauthService.GetOAuth2Token(ctx, email.ConnectionID)
	if err != nil {
		logger.WithError(err).WithField("connection_id", email.ConnectionID).Warn("[replaceCIDWithBase64] Failed to get OAuth token")
		return html
	}

	// Filter valid attachments first
	var validAttachments []*out.EmailAttachmentEntity
	for _, att := range inlineAttachments {
		if att.ContentID != nil && *att.ContentID != "" {
			validAttachments = append(validAttachments, att)
		}
	}
	if len(validAttachments) == 0 {
		return html
	}

	// Prepare attachment download results
	type cidResult struct {
		cid     string
		dataURL string
	}

	// Create timeout context for downloads (10 seconds max)
	downloadCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Bounded channel and semaphore for controlled concurrency
	const maxConcurrent = 5
	resultChan := make(chan cidResult, len(validAttachments))
	sem := make(chan struct{}, maxConcurrent)

	var wg sync.WaitGroup

	for _, att := range validAttachments {
		wg.Add(1)
		go func(att *out.EmailAttachmentEntity) {
			defer wg.Done()

			// Acquire semaphore (limit concurrency)
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-downloadCtx.Done():
				return // Context cancelled, exit early
			}

			cid := *att.ContentID

			// Download attachment data from provider
			var data []byte
			var mimeType string
			var downloadErr error

			switch email.Provider {
			case "google", "gmail":
				if h.gmailProvider != nil {
					data, mimeType, downloadErr = h.gmailProvider.GetAttachment(downloadCtx, token, email.ExternalID, att.ExternalID)
				}
			case "outlook", "microsoft":
				if h.outlookProvider != nil {
					data, mimeType, downloadErr = h.outlookProvider.GetAttachment(downloadCtx, token, email.ExternalID, att.ExternalID)
				}
			}

			if downloadErr != nil || len(data) == 0 {
				if downloadCtx.Err() == nil { // Only log if not cancelled
					logger.WithError(downloadErr).WithFields(map[string]any{
						"email_id": emailID,
						"cid":      cid,
					}).Warn("[replaceCIDWithBase64] Failed to download inline attachment")
				}
				resultChan <- cidResult{cid: cid, dataURL: ""}
				return
			}

			// Use stored MIME type if provider didn't return one
			if mimeType == "" {
				mimeType = att.MimeType
			}
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}

			// Convert to Base64 data URL
			base64Data := base64.StdEncoding.EncodeToString(data)
			dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)

			resultChan <- cidResult{cid: cid, dataURL: dataURL}
		}(att)
	}

	// Wait for all goroutines to complete in a separate goroutine
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results with timeout
	cidMap := make(map[string]string, len(validAttachments))
	for result := range resultChan {
		if result.dataURL != "" {
			cidMap[result.cid] = result.dataURL
		}
	}

	// Replace CID references with data URLs using single regex pass
	if len(cidMap) == 0 {
		return html
	}

	// Use pre-compiled pattern for performance
	html = cidPattern.ReplaceAllStringFunc(html, func(match string) string {
		// Extract CID from match
		submatches := cidPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		cid := submatches[1]

		// Look up data URL for this CID
		if dataURL, ok := cidMap[cid]; ok {
			return fmt.Sprintf(`src="%s"`, dataURL)
		}
		return match
	})

	return html
}

// FetchBodyFromProvider fetches email body directly from Gmail/Outlook API using provider_id
func (h *EmailHandler) FetchBodyFromProvider(c *fiber.Ctx) error {
	_, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	providerID := c.Query("provider_id")
	if providerID == "" {
		return ErrorResponse(c, 400, "provider_id required")
	}

	connectionID := c.QueryInt("connection_id", 0)
	if connectionID == 0 {
		return ErrorResponse(c, 400, "connection_id required")
	}

	if h.oauthService == nil {
		return ErrorResponse(c, 500, "oauth service not configured")
	}

	ctx := c.Context()

	// Get OAuth token
	token, err := h.oauthService.GetOAuth2Token(ctx, int64(connectionID))
	if err != nil {
		logger.WithError(err).Error("[EmailHandler.FetchBodyFromProvider] Failed to get OAuth token")
		return ErrorResponse(c, 500, "failed to get oauth token")
	}

	// Get connection info to determine provider type
	conn, err := h.oauthService.GetConnection(ctx, int64(connectionID))
	if err != nil {
		logger.WithError(err).Error("[EmailHandler.FetchBodyFromProvider] Failed to get connection")
		return ErrorResponse(c, 500, "failed to get connection info")
	}

	var body *out.ProviderMessageBody

	// Fetch body based on provider type
	switch conn.Provider {
	case "google", "gmail":
		if h.gmailProvider == nil {
			return ErrorResponse(c, 500, "gmail provider not configured")
		}
		body, err = h.gmailProvider.GetMessageBody(ctx, token, providerID)
	case "outlook", "microsoft":
		if h.outlookProvider == nil {
			return ErrorResponse(c, 500, "outlook provider not configured")
		}
		body, err = h.outlookProvider.GetMessageBody(ctx, token, providerID)
	default:
		return ErrorResponse(c, 400, "unsupported provider: "+string(conn.Provider))
	}

	if err != nil {
		return InternalErrorResponse(c, err, "fetch body from provider")
	}

	return c.JSON(fiber.Map{
		"html_body": body.HTML,
		"text_body": body.Text,
	})
}

func (h *EmailHandler) SendEmail(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req in.SendEmailRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	// Use connection_id from request or query
	if req.ConnectionID == 0 {
		if connID := GetConnectionID(c); connID != nil {
			req.ConnectionID = *connID
		}
	}

	email, err := h.emailService.SendEmail(c.Context(), userID, &req)
	if err != nil {
		return InternalErrorResponse(c, err, "send email")
	}

	return c.Status(201).JSON(email)
}

func (h *EmailHandler) ReplyEmail(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	emailID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	var req in.ReplyEmailRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	email, err := h.emailService.ReplyEmail(c.Context(), userID, emailID, &req)
	if err != nil {
		return InternalErrorResponse(c, err, "reply email")
	}

	return c.Status(201).JSON(email)
}

func (h *EmailHandler) ForwardEmail(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	emailID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	var req in.ForwardEmailRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	email, err := h.emailService.ForwardEmail(c.Context(), userID, emailID, &req)
	if err != nil {
		return InternalErrorResponse(c, err, "forward email")
	}

	return c.Status(201).JSON(email)
}

type EmailIDsRequest struct {
	IDs          []int64 `json:"ids"`
	ConnectionID int64   `json:"connection_id,omitempty"`
}

func (h *EmailHandler) MarkAsRead(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req EmailIDsRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if err := h.emailService.MarkAsRead(c.Context(), userID, req.IDs); err != nil {
		return InternalErrorResponse(c, err, "mark as read")
	}

	// Patch cache instead of full invalidation (Optimistic Update)
	if h.emailCache != nil {
		h.emailCache.PatchReadStatus(c.Context(), userID.String(), req.IDs, true)
	}

	return c.JSON(fiber.Map{"status": "ok", "ids": req.IDs})
}

func (h *EmailHandler) MarkAsUnread(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req EmailIDsRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if err := h.emailService.MarkAsUnread(c.Context(), userID, req.IDs); err != nil {
		return InternalErrorResponse(c, err, "mark as unread")
	}

	// Patch cache instead of full invalidation (Optimistic Update)
	if h.emailCache != nil {
		h.emailCache.PatchReadStatus(c.Context(), userID.String(), req.IDs, false)
	}

	return c.JSON(fiber.Map{"status": "ok", "ids": req.IDs})
}

func (h *EmailHandler) Star(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req EmailIDsRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if err := h.emailService.Star(c.Context(), userID, req.IDs); err != nil {
		return InternalErrorResponse(c, err, "star emails")
	}

	// Patch cache instead of full invalidation (Optimistic Update)
	if h.emailCache != nil {
		h.emailCache.PatchStarStatus(c.Context(), userID.String(), req.IDs, true)
	}

	return c.JSON(fiber.Map{"status": "ok", "ids": req.IDs})
}

func (h *EmailHandler) Unstar(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req EmailIDsRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if err := h.emailService.Unstar(c.Context(), userID, req.IDs); err != nil {
		return InternalErrorResponse(c, err, "unstar emails")
	}

	// Patch cache instead of full invalidation (Optimistic Update)
	if h.emailCache != nil {
		h.emailCache.PatchStarStatus(c.Context(), userID.String(), req.IDs, false)
	}

	return c.JSON(fiber.Map{"status": "ok", "ids": req.IDs})
}

func (h *EmailHandler) Archive(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req EmailIDsRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if err := h.emailService.Archive(c.Context(), userID, req.IDs); err != nil {
		return InternalErrorResponse(c, err, "archive emails")
	}

	// Remove from inbox cache and patch folder (Optimistic Update)
	if h.emailCache != nil {
		h.emailCache.PatchFolder(c.Context(), userID.String(), req.IDs, "archive")
	}

	return c.JSON(fiber.Map{"status": "ok", "ids": req.IDs})
}

func (h *EmailHandler) Trash(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req EmailIDsRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if err := h.emailService.Trash(c.Context(), userID, req.IDs); err != nil {
		return InternalErrorResponse(c, err, "trash emails")
	}

	// Remove from current folder cache and patch folder (Optimistic Update)
	if h.emailCache != nil {
		h.emailCache.PatchFolder(c.Context(), userID.String(), req.IDs, "trash")
	}

	return c.JSON(fiber.Map{"status": "ok", "ids": req.IDs})
}

// DeleteEmails permanently deletes emails.
func (h *EmailHandler) DeleteEmails(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req EmailIDsRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if err := h.emailService.Delete(c.Context(), userID, req.IDs); err != nil {
		return InternalErrorResponse(c, err, "delete emails")
	}

	// Remove from cache (Optimistic Update)
	if h.emailCache != nil {
		h.emailCache.RemoveFromCache(c.Context(), userID.String(), req.IDs)
	}

	return c.JSON(fiber.Map{"status": "ok", "deleted": len(req.IDs), "ids": req.IDs})
}

// MoveToFolderRequest represents move to folder request.
type MoveToFolderRequest struct {
	IDs    []int64 `json:"ids"`
	Folder string  `json:"folder"`
}

// MoveToFolder moves emails to a specific folder.
func (h *EmailHandler) MoveToFolder(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req MoveToFolderRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if req.Folder == "" {
		return ErrorResponse(c, 400, "folder is required")
	}

	if err := h.emailService.MoveToFolder(c.Context(), userID, req.IDs, req.Folder); err != nil {
		return InternalErrorResponse(c, err, "move to folder")
	}

	// Patch folder in cache (Optimistic Update)
	if h.emailCache != nil {
		h.emailCache.PatchFolder(c.Context(), userID.String(), req.IDs, req.Folder)
	}

	return c.JSON(fiber.Map{"status": "ok", "moved": len(req.IDs), "folder": req.Folder, "ids": req.IDs})
}

// SnoozeRequest represents snooze request.
type SnoozeRequest struct {
	IDs   []int64   `json:"ids"`
	Until time.Time `json:"until"`
}

// Snooze snoozes emails until a specific time.
func (h *EmailHandler) Snooze(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req SnoozeRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if req.Until.IsZero() {
		return ErrorResponse(c, 400, "until time is required")
	}

	if err := h.emailService.Snooze(c.Context(), userID, req.IDs, req.Until); err != nil {
		return InternalErrorResponse(c, err, "snooze emails")
	}

	// Snooze changes workflow_status, invalidate cache (no direct patch field)
	if h.emailCache != nil {
		h.emailCache.InvalidateByUser(c.Context(), userID.String())
	}

	return c.JSON(fiber.Map{"status": "ok", "snoozed": len(req.IDs), "until": req.Until, "ids": req.IDs})
}

// Unsnooze removes snooze from emails.
func (h *EmailHandler) Unsnooze(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req EmailIDsRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if err := h.emailService.Unsnooze(c.Context(), userID, req.IDs); err != nil {
		return InternalErrorResponse(c, err, "unsnooze emails")
	}

	// Unsnooze changes workflow_status, invalidate cache (no direct patch field)
	if h.emailCache != nil {
		h.emailCache.InvalidateByUser(c.Context(), userID.String())
	}

	return c.JSON(fiber.Map{"status": "ok", "unsnoozed": len(req.IDs), "ids": req.IDs})
}

// WorkflowStatusRequest represents workflow status update request.
type WorkflowStatusRequest struct {
	IDs    []int64 `json:"email_ids"`
	Status string  `json:"status"` // "todo", "done", "none"
}

// UpdateWorkflowStatus changes the workflow status of emails.
func (h *EmailHandler) UpdateWorkflowStatus(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req WorkflowStatusRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if len(req.IDs) == 0 {
		return ErrorResponse(c, 400, "email_ids are required")
	}

	// Validate status
	validStatuses := map[string]bool{"todo": true, "done": true, "none": true}
	if !validStatuses[req.Status] {
		return ErrorResponse(c, 400, "invalid status: must be 'todo', 'done', or 'none'")
	}

	if err := h.emailService.UpdateWorkflowStatus(c.Context(), userID, req.IDs, req.Status); err != nil {
		return InternalErrorResponse(c, err, "update workflow status")
	}

	// Invalidate cache
	if h.emailCache != nil {
		h.emailCache.InvalidateByUser(c.Context(), userID.String())
	}

	return c.JSON(fiber.Map{"status": "ok", "updated": len(req.IDs), "workflow_status": req.Status, "ids": req.IDs})
}

// BatchLabelsRequest represents batch labels request.
type BatchLabelsRequest struct {
	IDs    []int64  `json:"ids"`
	Labels []string `json:"labels"`
}

// BatchAddLabels adds labels to multiple emails.
func (h *EmailHandler) BatchAddLabels(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req BatchLabelsRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if len(req.Labels) == 0 {
		return ErrorResponse(c, 400, "labels are required")
	}

	if err := h.emailService.BatchAddLabels(c.Context(), userID, req.IDs, req.Labels); err != nil {
		return InternalErrorResponse(c, err, "add labels")
	}

	// Labels not directly cached, invalidate cache
	if h.emailCache != nil {
		h.emailCache.InvalidateByUser(c.Context(), userID.String())
	}

	return c.JSON(fiber.Map{"status": "ok", "updated": len(req.IDs), "added_labels": req.Labels, "ids": req.IDs})
}

// BatchRemoveLabels removes labels from multiple emails.
func (h *EmailHandler) BatchRemoveLabels(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req BatchLabelsRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if len(req.Labels) == 0 {
		return ErrorResponse(c, 400, "labels are required")
	}

	if err := h.emailService.BatchRemoveLabels(c.Context(), userID, req.IDs, req.Labels); err != nil {
		return InternalErrorResponse(c, err, "remove labels")
	}

	// Labels not directly cached, invalidate cache
	if h.emailCache != nil {
		h.emailCache.InvalidateByUser(c.Context(), userID.String())
	}

	return c.JSON(fiber.Map{"status": "ok", "updated": len(req.IDs), "removed_labels": req.Labels, "ids": req.IDs})
}

// =============================================================================
// Attachment Handlers (모아보기)
// =============================================================================

// ListAllAttachments returns all attachments for a user with filters.
// GET /email/attachments?connection_id=1&type=image&min_size=1024&max_size=10485760&sort_by=size&sort_order=desc&limit=50&offset=0
func (h *EmailHandler) ListAllAttachments(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	if h.attachmentRepo == nil {
		return ErrorResponse(c, 500, "attachment repository not configured")
	}

	// Parse query parameters
	query := &out.AttachmentListQuery{
		Limit:     c.QueryInt("limit", 50),
		Offset:    c.QueryInt("offset", 0),
		SortBy:    c.Query("sort_by", "created_at"),
		SortOrder: c.Query("sort_order", "desc"),
	}

	// Connection filter
	if connID := c.QueryInt("connection_id", 0); connID > 0 {
		id := int64(connID)
		query.ConnectionID = &id
	}

	// File type filter (simplified categories)
	if fileType := c.Query("type"); fileType != "" {
		switch fileType {
		case "image":
			query.MimeTypes = []string{"image/*"}
		case "video":
			query.MimeTypes = []string{"video/*"}
		case "audio":
			query.MimeTypes = []string{"audio/*"}
		case "pdf":
			query.MimeTypes = []string{"application/pdf"}
		case "document":
			query.MimeTypes = []string{
				"application/msword",
				"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
				"application/vnd.ms-word*",
				"application/vnd.openxmlformats-officedocument.wordprocessingml*",
			}
		case "spreadsheet":
			query.MimeTypes = []string{
				"application/vnd.ms-excel",
				"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
				"application/vnd.ms-excel*",
				"application/vnd.openxmlformats-officedocument.spreadsheetml*",
			}
		case "presentation":
			query.MimeTypes = []string{
				"application/vnd.ms-powerpoint",
				"application/vnd.openxmlformats-officedocument.presentationml.presentation",
				"application/vnd.ms-powerpoint*",
				"application/vnd.openxmlformats-officedocument.presentationml*",
			}
		case "archive":
			query.MimeTypes = []string{
				"application/zip",
				"application/x-rar-compressed",
				"application/x-7z-compressed",
				"application/gzip",
			}
		case "text":
			query.MimeTypes = []string{"text/*"}
		}
	}

	// Custom mime types (comma-separated)
	if mimeTypes := c.Query("mime_types"); mimeTypes != "" {
		query.MimeTypes = splitAndTrim(mimeTypes, ",")
	}

	// Size filters
	if minSize := c.QueryInt("min_size", 0); minSize > 0 {
		size := int64(minSize)
		query.MinSize = &size
	}
	if maxSize := c.QueryInt("max_size", 0); maxSize > 0 {
		size := int64(maxSize)
		query.MaxSize = &size
	}

	// Date filters
	if startDate := c.Query("start_date"); startDate != "" {
		if t, err := time.Parse(time.RFC3339, startDate); err == nil {
			query.StartDate = &t
		}
	}
	if endDate := c.Query("end_date"); endDate != "" {
		if t, err := time.Parse(time.RFC3339, endDate); err == nil {
			query.EndDate = &t
		}
	}

	// Execute query
	attachments, total, err := h.attachmentRepo.ListByUser(c.Context(), userID, query)
	if err != nil {
		return InternalErrorResponse(c, err, "list attachments")
	}

	// Convert to response format
	result := make([]fiber.Map, len(attachments))
	for i, att := range attachments {
		result[i] = fiber.Map{
			"id":                att.ID,
			"email_id":          att.EmailID,
			"external_id":       att.ExternalID,
			"filename":          att.Filename,
			"mime_type":         att.MimeType,
			"size":              att.Size,
			"is_inline":         att.IsInline,
			"created_at":        att.CreatedAt,
			"email_subject":     att.EmailSubject,
			"email_from":        att.EmailFrom,
			"email_date":        att.EmailDate,
			"connection_id":     att.ConnectionID,
			"email_provider":    att.EmailProvider,
			"email_external_id": att.EmailExternalID,
		}
	}

	hasMore := query.Offset+len(attachments) < total

	return c.JSON(fiber.Map{
		"attachments": result,
		"total":       total,
		"has_more":    hasMore,
		"limit":       query.Limit,
		"offset":      query.Offset,
	})
}

// GetAttachmentStats returns attachment statistics for a user.
// GET /email/attachments/stats
func (h *EmailHandler) GetAttachmentStats(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	if h.attachmentRepo == nil {
		return ErrorResponse(c, 500, "attachment repository not configured")
	}

	stats, err := h.attachmentRepo.GetStatsByUser(c.Context(), userID)
	if err != nil {
		return InternalErrorResponse(c, err, "get attachment stats")
	}

	return c.JSON(fiber.Map{
		"total_count":        stats.TotalCount,
		"total_size":         stats.TotalSize,
		"total_size_display": formatFileSize(stats.TotalSize),
		"count_by_type":      stats.CountByMimeType,
		"size_by_type":       stats.SizeByMimeType,
	})
}

// SearchAttachments searches attachments by filename.
// GET /email/attachments/search?q=report&limit=50&offset=0
func (h *EmailHandler) SearchAttachments(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	query := c.Query("q")
	if query == "" {
		return ErrorResponse(c, 400, "query parameter 'q' is required")
	}

	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)

	if h.attachmentRepo == nil {
		return ErrorResponse(c, 500, "attachment repository not configured")
	}

	attachments, total, err := h.attachmentRepo.SearchByUser(c.Context(), userID, query, limit, offset)
	if err != nil {
		return InternalErrorResponse(c, err, "search attachments")
	}

	// Convert to response format
	result := make([]fiber.Map, len(attachments))
	for i, att := range attachments {
		result[i] = fiber.Map{
			"id":                att.ID,
			"email_id":          att.EmailID,
			"external_id":       att.ExternalID,
			"filename":          att.Filename,
			"mime_type":         att.MimeType,
			"size":              att.Size,
			"is_inline":         att.IsInline,
			"created_at":        att.CreatedAt,
			"email_subject":     att.EmailSubject,
			"email_from":        att.EmailFrom,
			"email_date":        att.EmailDate,
			"connection_id":     att.ConnectionID,
			"email_provider":    att.EmailProvider,
			"email_external_id": att.EmailExternalID,
		}
	}

	hasMore := offset+len(attachments) < total

	return c.JSON(fiber.Map{
		"attachments": result,
		"total":       total,
		"has_more":    hasMore,
		"query":       query,
	})
}

// splitAndTrim splits a string by separator and trims whitespace from each part.
func splitAndTrim(s, sep string) []string {
	parts := make([]string, 0)
	for _, p := range stringsSplit(s, sep) {
		if trimmed := stringsTrim(p); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func stringsSplit(s, sep string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if string(s[i]) == sep {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func stringsTrim(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// formatFileSize formats file size in human-readable format.
func formatFileSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

// =============================================================================
// Email-specific Attachment Handlers
// =============================================================================

// GetAttachments returns all attachments for an email.
func (h *EmailHandler) GetAttachments(c *fiber.Ctx) error {
	_, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	emailID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	// 1단계: DB에서 먼저 조회
	var result []fiber.Map
	if h.attachmentRepo != nil {
		attachments, err := h.attachmentRepo.GetByEmailID(c.Context(), emailID)
		if err == nil && len(attachments) > 0 {
			result = make([]fiber.Map, len(attachments))
			for i, att := range attachments {
				result[i] = fiber.Map{
					"id":          att.ID,
					"external_id": att.ExternalID,
					"filename":    att.Filename,
					"mime_type":   att.MimeType,
					"size":        att.Size,
					"is_inline":   att.IsInline,
					"content_id":  att.ContentID,
				}
			}
			return c.JSON(fiber.Map{
				"attachments": result,
				"count":       len(result),
				"source":      "db",
			})
		}
	}

	// 2단계: DB에 없으면 Provider API에서 직접 가져오기
	if h.emailRepo == nil || h.oauthService == nil {
		return c.JSON(fiber.Map{
			"attachments": []fiber.Map{},
			"count":       0,
			"source":      "none",
		})
	}

	// 이메일 정보 조회
	email, err := h.emailRepo.GetByID(c.Context(), emailID)
	if err != nil || email == nil {
		return c.JSON(fiber.Map{
			"attachments": []fiber.Map{},
			"count":       0,
			"source":      "none",
		})
	}

	// OAuth 토큰 획득
	token, err := h.oauthService.GetOAuth2Token(c.Context(), email.ConnectionID)
	if err != nil {
		logger.WithError(err).Error("[EmailHandler.GetAttachments] Failed to get OAuth token")
		return c.JSON(fiber.Map{
			"attachments": []fiber.Map{},
			"count":       0,
			"source":      "none",
		})
	}

	// Provider에서 메시지 본문 + 첨부파일 정보 가져오기
	var body *out.ProviderMessageBody

	switch email.Provider {
	case "google", "gmail":
		if h.gmailProvider != nil {
			body, err = h.gmailProvider.GetMessageBody(c.Context(), token, email.ExternalID)
		}
	case "outlook", "microsoft":
		if h.outlookProvider != nil {
			body, err = h.outlookProvider.GetMessageBody(c.Context(), token, email.ExternalID)
		}
	}

	if err != nil || body == nil {
		logger.WithError(err).Warn("[EmailHandler.GetAttachments] Failed to get message body from provider")
		return c.JSON(fiber.Map{
			"attachments": []fiber.Map{},
			"count":       0,
			"source":      "none",
		})
	}

	// Provider 응답을 결과로 변환
	result = make([]fiber.Map, len(body.Attachments))
	for i, att := range body.Attachments {
		result[i] = fiber.Map{
			"id":          0, // DB ID 없음
			"external_id": att.ID,
			"filename":    att.Filename,
			"mime_type":   att.MimeType,
			"size":        att.Size,
			"is_inline":   att.IsInline,
			"content_id":  att.ContentID,
		}
	}

	// URL 기반 방식: 첨부파일 메타데이터는 DB에 저장하지 않음
	// Provider 응답을 그대로 반환

	return c.JSON(fiber.Map{
		"attachments": result,
		"count":       len(result),
		"source":      "provider",
	})
}

// saveAttachmentsAsync is removed - URL 기반 방식으로 변경
// 첨부파일 메타데이터는 DB에 저장하지 않고 Provider에서 직접 가져옴

// GetAttachment returns a single attachment metadata.
func (h *EmailHandler) GetAttachment(c *fiber.Ctx) error {
	_, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	emailID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	attachmentID, err := strconv.ParseInt(c.Params("attachmentId"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid attachment id")
	}

	if h.attachmentRepo == nil {
		return ErrorResponse(c, 500, "attachment repository not configured")
	}

	attachment, err := h.attachmentRepo.GetByID(c.Context(), attachmentID)
	if err != nil {
		return InternalErrorResponse(c, err, "get attachment")
	}
	if attachment == nil || attachment.EmailID != emailID {
		return ErrorResponse(c, 404, "attachment not found")
	}

	return c.JSON(fiber.Map{
		"id":          attachment.ID,
		"external_id": attachment.ExternalID,
		"filename":    attachment.Filename,
		"mime_type":   attachment.MimeType,
		"size":        attachment.Size,
		"is_inline":   attachment.IsInline,
		"content_id":  attachment.ContentID,
	})
}

// DownloadAttachment downloads an attachment from provider API.
func (h *EmailHandler) DownloadAttachment(c *fiber.Ctx) error {
	_, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	emailID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	// attachmentId는 DB ID 또는 external_id (Provider ID)
	attachmentParam := c.Params("attachmentId")
	if attachmentParam == "" {
		return ErrorResponse(c, 400, "attachment id required")
	}

	// Get email to find connection and provider info
	if h.emailRepo == nil {
		return ErrorResponse(c, 500, "mail repository not configured")
	}

	email, err := h.emailRepo.GetByID(c.Context(), emailID)
	if err != nil {
		return InternalErrorResponse(c, err, "get email")
	}
	if email == nil {
		return ErrorResponse(c, 404, "email not found")
	}

	// Get OAuth token
	if h.oauthService == nil {
		return ErrorResponse(c, 500, "oauth service not configured")
	}

	token, err := h.oauthService.GetOAuth2Token(c.Context(), email.ConnectionID)
	if err != nil {
		logger.WithError(err).Error("[EmailHandler.DownloadAttachment] Failed to get OAuth token")
		return ErrorResponse(c, 500, "failed to get oauth token")
	}

	// attachmentId 확인: DB ID인지 external_id인지 판별
	var attachmentExternalID string
	var attachmentFilename string
	var attachmentMimeType string

	// 먼저 숫자인지 확인 (DB ID)
	if attachmentID, parseErr := strconv.ParseInt(attachmentParam, 10, 64); parseErr == nil && h.attachmentRepo != nil {
		// DB에서 조회 시도
		attachment, dbErr := h.attachmentRepo.GetByID(c.Context(), attachmentID)
		if dbErr == nil && attachment != nil && attachment.EmailID == emailID {
			attachmentExternalID = attachment.ExternalID
			attachmentFilename = attachment.Filename
			attachmentMimeType = attachment.MimeType
		}
	}

	// DB에서 못 찾으면 attachmentParam을 external_id로 사용
	if attachmentExternalID == "" {
		attachmentExternalID = attachmentParam
	}

	// Download from provider
	var data []byte
	var mimeType string

	switch email.Provider {
	case "google", "gmail":
		if h.gmailProvider == nil {
			return ErrorResponse(c, 500, "gmail provider not configured")
		}
		data, mimeType, err = h.gmailProvider.GetAttachment(c.Context(), token, email.ExternalID, attachmentExternalID)
	case "outlook", "microsoft":
		if h.outlookProvider == nil {
			return ErrorResponse(c, 500, "outlook provider not configured")
		}
		data, mimeType, err = h.outlookProvider.GetAttachment(c.Context(), token, email.ExternalID, attachmentExternalID)
	default:
		return ErrorResponse(c, 400, "unsupported provider: "+email.Provider)
	}

	if err != nil {
		logger.WithError(err).Error("[EmailHandler.DownloadAttachment] Failed to download attachment")
		return ErrorResponse(c, 500, "failed to download attachment")
	}

	// Use stored mime type if provider didn't return one
	if mimeType == "" && attachmentMimeType != "" {
		mimeType = attachmentMimeType
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// 파일명 결정 (DB에서 가져왔거나, 기본값 사용)
	filename := attachmentFilename
	if filename == "" {
		filename = "attachment"
	}

	// Set headers for file download
	c.Set("Content-Type", mimeType)
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Set("Content-Length", strconv.FormatInt(int64(len(data)), 10))

	return c.Send(data)
}

// GetInlineAttachment serves an inline attachment by Content-ID.
// GET /email/:id/cid/:contentId
func (h *EmailHandler) GetInlineAttachment(c *fiber.Ctx) error {
	_, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	emailID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	contentID := c.Params("contentId")
	if contentID == "" {
		return ErrorResponse(c, 400, "content id required")
	}

	if h.attachmentRepo == nil {
		return ErrorResponse(c, 500, "attachment repository not configured")
	}

	// Find attachment by Content-ID
	attachment, err := h.attachmentRepo.GetByContentID(c.Context(), emailID, contentID)
	if err != nil {
		return InternalErrorResponse(c, err, "get inline attachment")
	}
	if attachment == nil {
		return ErrorResponse(c, 404, "inline attachment not found")
	}

	// Get email to find provider info
	if h.emailRepo == nil {
		return ErrorResponse(c, 500, "mail repository not configured")
	}

	email, err := h.emailRepo.GetByID(c.Context(), emailID)
	if err != nil {
		return InternalErrorResponse(c, err, "get email")
	}
	if email == nil {
		return ErrorResponse(c, 404, "email not found")
	}

	// Get OAuth token
	token, err := h.oauthService.GetOAuth2Token(c.Context(), email.ConnectionID)
	if err != nil {
		return ErrorResponse(c, 500, "failed to get oauth token")
	}

	// Download from provider
	var data []byte
	var mimeType string

	switch email.Provider {
	case "google", "gmail":
		if h.gmailProvider == nil {
			return ErrorResponse(c, 500, "gmail provider not configured")
		}
		data, mimeType, err = h.gmailProvider.GetAttachment(c.Context(), token, email.ExternalID, attachment.ExternalID)
	case "outlook", "microsoft":
		if h.outlookProvider == nil {
			return ErrorResponse(c, 500, "outlook provider not configured")
		}
		data, mimeType, err = h.outlookProvider.GetAttachment(c.Context(), token, email.ExternalID, attachment.ExternalID)
	default:
		return ErrorResponse(c, 400, "unsupported provider: "+email.Provider)
	}

	if err != nil {
		return ErrorResponse(c, 500, "failed to download attachment")
	}

	if mimeType == "" {
		mimeType = attachment.MimeType
	}

	// For inline images, set cache headers
	c.Set("Content-Type", mimeType)
	c.Set("Cache-Control", "private, max-age=86400") // 24h cache
	c.Set("Content-Length", strconv.FormatInt(int64(len(data)), 10))

	return c.Send(data)
}

// DownloadAllAttachments downloads all attachments as a ZIP file.
// Uses streaming to reduce memory usage for large attachments.
// GET /email/:id/attachments/download/all
func (h *EmailHandler) DownloadAllAttachments(c *fiber.Ctx) error {
	_, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	emailID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	if h.attachmentRepo == nil {
		return ErrorResponse(c, 500, "attachment repository not configured")
	}

	// Get all attachments for the email
	attachments, err := h.attachmentRepo.GetByEmailID(c.Context(), emailID)
	if err != nil {
		return InternalErrorResponse(c, err, "get attachments")
	}
	if len(attachments) == 0 {
		return ErrorResponse(c, 404, "no attachments found")
	}

	// Get email for provider info
	email, err := h.emailRepo.GetByID(c.Context(), emailID)
	if err != nil {
		return InternalErrorResponse(c, err, "get email")
	}
	if email == nil {
		return ErrorResponse(c, 404, "email not found")
	}

	// Get OAuth token
	token, err := h.oauthService.GetOAuth2Token(c.Context(), email.ConnectionID)
	if err != nil {
		return ErrorResponse(c, 500, "failed to get oauth token")
	}

	// Filter non-inline attachments
	var downloadAttachments []*out.EmailAttachmentEntity
	for _, att := range attachments {
		if !att.IsInline {
			downloadAttachments = append(downloadAttachments, att)
		}
	}
	if len(downloadAttachments) == 0 {
		return ErrorResponse(c, 404, "no downloadable attachments found")
	}

	// Set headers for streaming ZIP download
	c.Set("Content-Type", "application/zip")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="attachments_%d.zip"`, emailID))
	c.Set("Transfer-Encoding", "chunked")

	// Stream ZIP directly to response
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		zipWriter := zip.NewWriter(w)
		defer zipWriter.Close()

		// Limit concurrent downloads
		const maxConcurrent = 3
		sem := make(chan struct{}, maxConcurrent)
		type downloadResult struct {
			filename string
			data     []byte
			err      error
		}
		results := make(chan downloadResult, len(downloadAttachments))

		// Download attachments concurrently
		var wg sync.WaitGroup
		for _, att := range downloadAttachments {
			wg.Add(1)
			go func(att *out.EmailAttachmentEntity) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				var data []byte
				var dlErr error
				switch email.Provider {
				case "google", "gmail":
					data, _, dlErr = h.gmailProvider.GetAttachment(c.Context(), token, email.ExternalID, att.ExternalID)
				case "outlook", "microsoft":
					data, _, dlErr = h.outlookProvider.GetAttachment(c.Context(), token, email.ExternalID, att.ExternalID)
				}
				results <- downloadResult{filename: att.Filename, data: data, err: dlErr}
			}(att)
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		// Write to ZIP as downloads complete
		for result := range results {
			if result.err != nil {
				logger.WithError(result.err).Warn("[EmailHandler.DownloadAllAttachments] Failed to download: %s", result.filename)
				continue
			}
			if zw, err := zipWriter.Create(result.filename); err == nil {
				zw.Write(result.data)
			}
		}
	})

	return nil
}

// =============================================================================
// Upload Session API (Provider Delegation for Large Attachments)
// =============================================================================

// CreateUploadSessionRequest represents the request body for creating an upload session.
type CreateUploadSessionRequest struct {
	ConnectionID int64  `json:"connection_id"`
	MessageID    string `json:"message_id,omitempty"` // For attaching to existing draft
	Filename     string `json:"filename"`
	Size         int64  `json:"size"`
	MimeType     string `json:"mime_type"`
	IsInline     bool   `json:"is_inline,omitempty"`
	ContentID    string `json:"content_id,omitempty"`
}

// CreateUploadSession creates an upload session for large attachments.
// The frontend will receive an uploadUrl to directly upload chunks to Gmail/Outlook.
// POST /email/attachments/upload/session
func (h *EmailHandler) CreateUploadSession(c *fiber.Ctx) error {
	_, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req CreateUploadSessionRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	// Validation
	if req.ConnectionID == 0 {
		return ErrorResponse(c, 400, "connection_id required")
	}
	if req.Filename == "" {
		return ErrorResponse(c, 400, "filename required")
	}
	if req.Size <= 0 {
		return ErrorResponse(c, 400, "size must be positive")
	}
	if req.MimeType == "" {
		req.MimeType = "application/octet-stream"
	}

	// Get connection info
	conn, err := h.oauthService.GetConnection(c.Context(), req.ConnectionID)
	if err != nil {
		return ErrorResponse(c, 500, "failed to get connection")
	}

	// Get OAuth token
	token, err := h.oauthService.GetOAuth2Token(c.Context(), req.ConnectionID)
	if err != nil {
		return ErrorResponse(c, 500, "failed to get oauth token")
	}

	// Build upload session request
	uploadReq := &out.UploadSessionRequest{
		Filename:  req.Filename,
		Size:      req.Size,
		MimeType:  req.MimeType,
		IsInline:  req.IsInline,
		ContentID: req.ContentID,
	}

	var resp *out.UploadSessionResponse

	switch conn.Provider {
	case "google", "gmail":
		if h.gmailProvider == nil {
			return ErrorResponse(c, 500, "gmail provider not configured")
		}
		resp, err = h.gmailProvider.CreateUploadSession(c.Context(), token, req.MessageID, uploadReq)
	case "outlook", "microsoft":
		if h.outlookProvider == nil {
			return ErrorResponse(c, 500, "outlook provider not configured")
		}
		if req.MessageID == "" {
			return ErrorResponse(c, 400, "message_id required for Outlook attachments")
		}
		resp, err = h.outlookProvider.CreateUploadSession(c.Context(), token, req.MessageID, uploadReq)
	default:
		return ErrorResponse(c, 400, "unsupported provider: "+string(conn.Provider))
	}

	if err != nil {
		logger.WithError(err).Error("[EmailHandler.CreateUploadSession] Failed")
		return ErrorResponse(c, 500, "failed to create upload session: "+err.Error())
	}

	return c.Status(201).JSON(fiber.Map{
		"session_id":     resp.SessionID,
		"upload_url":     resp.UploadURL,
		"expires_at":     resp.ExpiresAt,
		"chunk_size":     resp.ChunkSize,
		"max_chunk_size": resp.MaxChunkSize,
		"provider":       resp.Provider,
	})
}

// GetUploadSessionStatus checks the status of an upload session.
// GET /email/attachments/upload/:sessionId/status
func (h *EmailHandler) GetUploadSessionStatus(c *fiber.Ctx) error {
	_, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	uploadURL := c.Query("upload_url")
	if uploadURL == "" {
		return ErrorResponse(c, 400, "upload_url query parameter required")
	}

	connectionID := c.QueryInt("connection_id", 0)
	if connectionID == 0 {
		return ErrorResponse(c, 400, "connection_id required")
	}

	// Get connection info
	conn, err := h.oauthService.GetConnection(c.Context(), int64(connectionID))
	if err != nil {
		return ErrorResponse(c, 500, "failed to get connection")
	}

	// Get OAuth token
	token, err := h.oauthService.GetOAuth2Token(c.Context(), int64(connectionID))
	if err != nil {
		return ErrorResponse(c, 500, "failed to get oauth token")
	}

	var status *out.UploadSessionStatus

	switch conn.Provider {
	case "google", "gmail":
		if h.gmailProvider == nil {
			return ErrorResponse(c, 500, "gmail provider not configured")
		}
		status, err = h.gmailProvider.GetUploadSessionStatus(c.Context(), token, uploadURL)
	case "outlook", "microsoft":
		if h.outlookProvider == nil {
			return ErrorResponse(c, 500, "outlook provider not configured")
		}
		status, err = h.outlookProvider.GetUploadSessionStatus(c.Context(), token, uploadURL)
	default:
		return ErrorResponse(c, 400, "unsupported provider: "+string(conn.Provider))
	}

	if err != nil {
		logger.WithError(err).Error("[EmailHandler.GetUploadSessionStatus] Failed")
		return ErrorResponse(c, 500, "failed to get upload status: "+err.Error())
	}

	return c.JSON(fiber.Map{
		"session_id":       status.SessionID,
		"bytes_uploaded":   status.BytesUploaded,
		"total_bytes":      status.TotalBytes,
		"is_complete":      status.IsComplete,
		"attachment_id":    status.AttachmentID,
		"next_range_start": status.NextRangeStart,
	})
}

// CancelUploadSession cancels an upload session.
// DELETE /email/attachments/upload/:sessionId
func (h *EmailHandler) CancelUploadSession(c *fiber.Ctx) error {
	_, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	uploadURL := c.Query("upload_url")
	if uploadURL == "" {
		return ErrorResponse(c, 400, "upload_url query parameter required")
	}

	connectionID := c.QueryInt("connection_id", 0)
	if connectionID == 0 {
		return ErrorResponse(c, 400, "connection_id required")
	}

	// Get connection info
	conn, err := h.oauthService.GetConnection(c.Context(), int64(connectionID))
	if err != nil {
		return ErrorResponse(c, 500, "failed to get connection")
	}

	// Get OAuth token
	token, err := h.oauthService.GetOAuth2Token(c.Context(), int64(connectionID))
	if err != nil {
		return ErrorResponse(c, 500, "failed to get oauth token")
	}

	switch conn.Provider {
	case "google", "gmail":
		if h.gmailProvider == nil {
			return ErrorResponse(c, 500, "gmail provider not configured")
		}
		err = h.gmailProvider.CancelUploadSession(c.Context(), token, uploadURL)
	case "outlook", "microsoft":
		if h.outlookProvider == nil {
			return ErrorResponse(c, 500, "outlook provider not configured")
		}
		err = h.outlookProvider.CancelUploadSession(c.Context(), token, uploadURL)
	default:
		return ErrorResponse(c, 400, "unsupported provider: "+string(conn.Provider))
	}

	if err != nil {
		logger.WithError(err).Error("[EmailHandler.CancelUploadSession] Failed")
		return ErrorResponse(c, 500, "failed to cancel upload: "+err.Error())
	}

	return c.SendStatus(204)
}

// =============================================================================
// Query Parameter Helpers for Domain Types
// =============================================================================

// queryFolder parses folder query parameter
func queryFolder(c *fiber.Ctx, key string) *domain.LegacyFolder {
	val := c.Query(key)
	if val == "" {
		return nil
	}
	f := domain.LegacyFolder(val)
	return &f
}

// queryCategory parses AI category query parameter
func queryCategory(c *fiber.Ctx, key string) *domain.EmailCategory {
	val := c.Query(key)
	if val == "" {
		return nil
	}
	cat := domain.EmailCategory(val)
	return &cat
}

// querySubCategory parses AI sub-category query parameter
func querySubCategory(c *fiber.Ctx, key string) *domain.EmailSubCategory {
	val := c.Query(key)
	if val == "" {
		return nil
	}
	subCat := domain.EmailSubCategory(val)
	return &subCat
}

// queryPriority parses priority query parameter
func queryPriority(c *fiber.Ctx, key string) *domain.Priority {
	val := c.QueryInt(key, 0)
	if val == 0 {
		return nil
	}
	pri := domain.Priority(val)
	return &pri
}

// queryWorkflowStatus parses workflow status query parameter
func queryWorkflowStatus(c *fiber.Ctx, key string) *domain.WorkflowStatus {
	val := c.Query(key)
	if val == "" {
		return nil
	}
	status := domain.WorkflowStatus(val)
	return &status
}

// queryInt64Array parses comma-separated int64 array (e.g., "1,2,3")
func queryInt64Array(c *fiber.Ctx, key string) []int64 {
	val := c.Query(key)
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	result := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if id, err := strconv.ParseInt(p, 10, 64); err == nil {
			result = append(result, id)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// queryTime parses ISO8601/RFC3339 date string (e.g., "2024-01-15" or "2024-01-15T10:30:00Z")
func queryTime(c *fiber.Ctx, key string) *time.Time {
	val := c.Query(key)
	if val == "" {
		return nil
	}
	// Try RFC3339 first
	if t, err := time.Parse(time.RFC3339, val); err == nil {
		return &t
	}
	// Try date only format
	if t, err := time.Parse("2006-01-02", val); err == nil {
		return &t
	}
	return nil
}
