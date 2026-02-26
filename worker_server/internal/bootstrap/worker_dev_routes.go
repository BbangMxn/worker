package bootstrap

import (
	"worker_server/adapter/in/http"
	"worker_server/core/agent"
	"worker_server/core/domain"
	"worker_server/core/port/in"
	"worker_server/pkg/logger"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// RegisterDevTestRoutes registers development-only test routes without authentication
// WARNING: Only enable in development environment!
func RegisterDevTestRoutes(app *fiber.App, deps *Dependencies, testUserID string) {
	userID, err := uuid.Parse(testUserID)
	if err != nil {
		logger.Error("[DevTest] Invalid test user ID: %s", testUserID)
		return
	}

	dev := app.Group("/dev")

	// Middleware to inject test user ID
	dev.Use(func(c *fiber.Ctx) error {
		c.Locals("userID", userID)
		return c.Next()
	})

	// Mail list
	dev.Get("/email", func(c *fiber.Ctx) error {
		limit := c.QueryInt("limit", 20)
		offset := c.QueryInt("offset", 0)
		folder := c.Query("folder", "inbox")

		logger.Info("[DevTest] ListEmails: user=%s, folder=%s, limit=%d", userID, folder, limit)

		folderVal := domain.LegacyFolder(folder)
		filter := &domain.EmailFilter{
			UserID: userID,
			Folder: &folderVal,
			Limit:  limit,
			Offset: offset,
		}

		emails, total, err := deps.EmailService.ListEmails(c.Context(), filter)
		if err != nil {
			return http.ErrorResponse(c, 500, err.Error())
		}

		return c.JSON(fiber.Map{
			"emails": emails,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	})

	// Fetch directly from Gmail API (no DB save) - MUST be before /email/:id
	dev.Get("/email/fetch", func(c *fiber.Ctx) error {
		connectionID := c.QueryInt("connection_id", 0)
		if connectionID == 0 {
			return http.ErrorResponse(c, 400, "connection_id required")
		}
		limit := c.QueryInt("limit", 20)

		logger.Info("[DevTest] FetchFromProvider: user=%s, connection=%d", userID, connectionID)

		emails, err := deps.MailSyncService.FetchFromProvider(c.Context(), userID.String(), int64(connectionID), limit)
		if err != nil {
			return http.ErrorResponse(c, 500, err.Error())
		}

		return c.JSON(fiber.Map{
			"emails": emails,
			"total":  len(emails),
			"source": "gmail_api",
		})
	})

	// Trigger sync (InitialSync) - MUST be before /email/:id
	dev.Post("/email/sync", func(c *fiber.Ctx) error {
		connectionID := c.QueryInt("connection_id", 0)
		if connectionID == 0 {
			return http.ErrorResponse(c, 400, "connection_id required")
		}

		logger.Info("[DevTest] InitialSync: user=%s, connection=%d", userID, connectionID)

		err := deps.MailSyncService.InitialSync(c.Context(), userID.String(), int64(connectionID))
		if err != nil {
			return http.ErrorResponse(c, 500, err.Error())
		}

		return c.JSON(fiber.Map{
			"status":  "sync_completed",
			"user_id": userID,
		})
	})

	// Get email detail
	dev.Get("/email/:id", func(c *fiber.Ctx) error {
		emailID, err := c.ParamsInt("id")
		if err != nil {
			return http.ErrorResponse(c, 400, "invalid email id")
		}
		logger.Info("[DevTest] GetEmail: user=%s, id=%d", userID, emailID)

		email, err := deps.EmailService.GetEmail(c.Context(), userID, int64(emailID))
		if err != nil {
			return http.ErrorResponse(c, 404, err.Error())
		}

		return c.JSON(email)
	})

	// Get email body
	dev.Get("/email/:id/body", func(c *fiber.Ctx) error {
		emailID, err := c.ParamsInt("id")
		if err != nil {
			return http.ErrorResponse(c, 400, "invalid email id")
		}
		logger.Info("[DevTest] GetEmailBody: user=%s, id=%d", userID, emailID)

		body, err := deps.EmailService.GetEmailBody(c.Context(), int64(emailID))
		if err != nil {
			return http.ErrorResponse(c, 404, err.Error())
		}

		return c.JSON(body)
	})

	// List OAuth connections
	dev.Get("/connections", func(c *fiber.Ctx) error {
		logger.Info("[DevTest] GetConnectionsByUser: user=%s", userID)

		connections, err := deps.OAuthService.GetConnectionsByUser(c.Context(), userID)
		if err != nil {
			return http.ErrorResponse(c, 500, err.Error())
		}

		return c.JSON(fiber.Map{
			"connections": connections,
		})
	})

	// Get specific connection
	dev.Get("/connections/:id", func(c *fiber.Ctx) error {
		connID, err := c.ParamsInt("id")
		if err != nil {
			return http.ErrorResponse(c, 400, "invalid connection id")
		}

		logger.Info("[DevTest] GetConnection: id=%d", connID)

		conn, err := deps.OAuthService.GetConnection(c.Context(), int64(connID))
		if err != nil {
			return http.ErrorResponse(c, 404, err.Error())
		}

		return c.JSON(conn)
	})

	// Calendar endpoints
	dev.Get("/calendar", func(c *fiber.Ctx) error {
		logger.Info("[DevTest] ListCalendarEvents: user=%s", userID)

		filter := &domain.CalendarEventFilter{
			UserID: userID,
			Limit:  50,
		}

		events, total, err := deps.CalendarService.ListEvents(c.Context(), filter)
		if err != nil {
			return http.ErrorResponse(c, 500, err.Error())
		}

		return c.JSON(fiber.Map{
			"events": events,
			"total":  total,
		})
	})

	// AI Chat
	dev.Post("/ai/chat", func(c *fiber.Ctx) error {
		var req struct {
			Message   string `json:"message"`
			SessionID string `json:"session_id"`
		}
		if err := c.BodyParser(&req); err != nil {
			return http.ErrorResponse(c, 400, "invalid request body")
		}

		logger.Info("[DevTest] AIChat: user=%s, message=%s", userID, req.Message)

		agentReq := &agent.AgentRequest{
			UserID:    userID,
			SessionID: req.SessionID,
			Message:   req.Message,
		}

		response, err := deps.Orchestrator.Process(c.Context(), agentReq)
		if err != nil {
			return http.ErrorResponse(c, 500, err.Error())
		}

		return c.JSON(response)
	})

	// AI Classify - 이메일 분류
	dev.Post("/ai/classify/:id", func(c *fiber.Ctx) error {
		emailID, err := c.ParamsInt("id")
		if err != nil {
			return http.ErrorResponse(c, 400, "invalid email id")
		}

		logger.Info("[DevTest] AIClassify: user=%s, email=%d", userID, emailID)

		result, err := deps.AIService.ClassifyEmail(c.Context(), int64(emailID))
		if err != nil {
			return http.ErrorResponse(c, 500, err.Error())
		}

		return c.JSON(result)
	})

	// AI Summarize - 이메일 요약
	dev.Post("/ai/summarize/:id", func(c *fiber.Ctx) error {
		emailID, err := c.ParamsInt("id")
		if err != nil {
			return http.ErrorResponse(c, 400, "invalid email id")
		}

		logger.Info("[DevTest] AISummarize: user=%s, email=%d", userID, emailID)

		summary, err := deps.AIService.SummarizeEmail(c.Context(), int64(emailID), true)
		if err != nil {
			return http.ErrorResponse(c, 500, err.Error())
		}

		return c.JSON(fiber.Map{
			"email_id": emailID,
			"summary":  summary,
		})
	})

	// AI Translate - 이메일 번역
	dev.Post("/ai/translate/:id", func(c *fiber.Ctx) error {
		emailID, err := c.ParamsInt("id")
		if err != nil {
			return http.ErrorResponse(c, 400, "invalid email id")
		}

		var req struct {
			TargetLang string `json:"target_lang"`
		}
		if err := c.BodyParser(&req); err != nil {
			req.TargetLang = "ko" // default Korean
		}
		if req.TargetLang == "" {
			req.TargetLang = "ko"
		}

		logger.Info("[DevTest] AITranslate: user=%s, email=%d, lang=%s", userID, emailID, req.TargetLang)

		result, err := deps.AIService.TranslateEmail(c.Context(), int64(emailID), req.TargetLang)
		if err != nil {
			return http.ErrorResponse(c, 500, err.Error())
		}

		return c.JSON(result)
	})

	// AI Reply - 답장 생성
	dev.Post("/ai/reply/:id", func(c *fiber.Ctx) error {
		emailID, err := c.ParamsInt("id")
		if err != nil {
			return http.ErrorResponse(c, 400, "invalid email id")
		}

		var req struct {
			Tone   string `json:"tone"`
			Intent string `json:"intent"`
		}
		if err := c.BodyParser(&req); err != nil {
			req.Tone = "professional"
			req.Intent = "inform"
		}
		if req.Tone == "" {
			req.Tone = "professional"
		}
		if req.Intent == "" {
			req.Intent = "inform"
		}

		logger.Info("[DevTest] AIReply: user=%s, email=%d, tone=%s", userID, emailID, req.Tone)

		options := &in.ReplyOptions{
			Tone:   req.Tone,
			Intent: req.Intent,
		}

		reply, err := deps.AIService.GenerateReply(c.Context(), int64(emailID), options)
		if err != nil {
			return http.ErrorResponse(c, 500, err.Error())
		}

		return c.JSON(fiber.Map{
			"email_id": emailID,
			"reply":    reply,
		})
	})

	// AI Extract Meeting - 미팅 정보 추출
	dev.Post("/ai/meeting/:id", func(c *fiber.Ctx) error {
		emailID, err := c.ParamsInt("id")
		if err != nil {
			return http.ErrorResponse(c, 400, "invalid email id")
		}

		logger.Info("[DevTest] AIExtractMeeting: user=%s, email=%d", userID, emailID)

		meeting, err := deps.AIService.ExtractMeetingInfo(c.Context(), int64(emailID))
		if err != nil {
			return http.ErrorResponse(c, 500, err.Error())
		}

		return c.JSON(meeting)
	})

	logger.Info("[DevTest] Development test routes registered at /dev/*")
}
