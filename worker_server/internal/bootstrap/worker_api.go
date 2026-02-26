package bootstrap

import (
	"context"
	"os"
	"strings"

	"worker_server/adapter/in/http"
	"worker_server/adapter/out/persistence"
	"worker_server/config"
	"worker_server/infra/middleware"
	"worker_server/pkg/logger"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/rs/zerolog"
)

func NewAPI(cfg *config.Config) (*fiber.App, func(), error) {
	// Initialize logger
	logLevel := logger.LevelInfo
	if cfg.IsDevelopment() {
		logLevel = logger.LevelDebug
	}
	logger.Init(logger.Config{
		Level:   logLevel,
		Service: "bridgify-api",
	})

	// Initialize JWKS for Supabase ES256/RS256 JWT verification
	middleware.InitJWKS(cfg.SupabaseURL)

	// Note: Token blacklist and audit logger are initialized after deps are created

	deps, cleanup, err := NewDependencies(cfg)
	if err != nil {
		logger.WithError(err).Error("Failed to initialize dependencies")
		return nil, nil, err
	}

	// Initialize security components with Redis
	middleware.InitTokenBlacklist(deps.Redis)
	middleware.InitAuditLogger(deps.Redis)

	app := fiber.New(fiber.Config{
		ErrorHandler:          middleware.ErrorHandler(),
		DisableStartupMessage: cfg.IsProduction(),
		Prefork:               false,
		StrictRouting:         false,
		CaseSensitive:         false,

		// =============================================================================
		// 성능 최적화 설정
		// =============================================================================

		// Buffer sizes (메모리 vs 성능 트레이드오프)
		ReadBufferSize:  16384, // 16KB - 큰 요청 처리 최적화
		WriteBufferSize: 16384, // 16KB - 큰 응답 처리 최적화

		// go-json: 표준 encoding/json 대비 2~3배 빠른 JSON 직렬화
		JSONEncoder: json.Marshal,
		JSONDecoder: json.Unmarshal,

		// Body 제한 (메모리 보호)
		BodyLimit: 10 * 1024 * 1024, // 10MB

		// Concurrency 설정
		Concurrency: 256 * 1024, // 동시 연결 수

		// 서버 헤더 비활성화 (보안 + 미세한 성능 향상)
		ServerHeader:             "",
		DisableDefaultDate:       true,
		DisableHeaderNormalizing: false,

		// Keep-alive (연결 재사용)
		DisableKeepalive: false,

		// Streaming 최적화
		StreamRequestBody:            true,
		DisablePreParseMultipartForm: true,
	})

	// Global middleware stack (order matters)
	app.Use(middleware.Recover())              // 1. Panic recovery
	app.Use(middleware.RequestID())            // 2. Request ID
	app.Use(middleware.SecurityHeaders())      // 3. Security headers
	app.Use(middleware.PreventPathTraversal()) // 4. Path traversal protection
	app.Use(middleware.InputSanitizer())       // 5. Input sanitization
	app.Use(middleware.RequestLogger())        // 6. Request logging

	// Response compression (gzip/brotli) - reduces response size by ~70%
	app.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed, // 빠른 압축 (CPU vs 압축률 균형)
	}))

	// ETag 미들웨어 - 304 Not Modified 응답으로 대역폭 절약
	app.Use(middleware.ETag())

	// CORS - Security hardened configuration
	// AllowCredentials:true requires explicit origins (not "*")
	allowOrigins := strings.Join(cfg.AllowedOrigins, ",")
	allowCredentials := true
	if allowOrigins == "" || allowOrigins == "*" {
		// In production, never allow "*" with credentials
		if cfg.IsProduction() {
			allowOrigins = "" // Block all if not configured properly
			allowCredentials = false
		} else {
			// Development: allow localhost only
			allowOrigins = "http://localhost:3000,http://localhost:5173"
		}
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins:     allowOrigins,
		AllowMethods:     "GET,POST,PUT,DELETE,PATCH,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-Request-ID",
		ExposeHeaders:    "X-Request-ID,X-RateLimit-Limit,X-RateLimit-Remaining,X-RateLimit-Reset",
		AllowCredentials: allowCredentials,
		MaxAge:           86400, // 24 hours
	}))

	// Health check (no auth required)
	healthHandler := http.NewHealthHandlerWithDeps(deps.DB, deps.Redis)
	healthHandler.Register(app)

	// Development-only test endpoints (no auth, hardcoded test user)
	if cfg.IsDevelopment() {
		testUserID := "76b3b1fb-04fe-4b9f-8919-a431a8e3ddb1" // jixso6484@gmail.com
		RegisterDevTestRoutes(app, deps, testUserID)
		logger.Info("Development test routes enabled for user: %s", testUserID)
	}

	// OAuth state store for CSRF protection
	oauthStateStore := persistence.NewRedisOAuthStateStore(deps.Redis)

	// OAuth callback (no auth required - Google redirects here)
	oauthHandler := http.NewOAuthHandlerWithStateStore(deps.OAuthService, oauthStateStore)
	app.Get("/api/v1/oauth/callback/:provider", oauthHandler.Callback)
	app.Get("/api/v1/oauth/:provider/callback", oauthHandler.Callback)

	// Webhook handlers (no auth required - called by Google/Microsoft)
	webhookHandler := http.NewWebhookHandler(
		deps.OAuthService,
		deps.WebhookService,
		deps.MailSyncService,
		deps.MessageProducer,
		deps.RealtimeAdapter,
		deps.SyncStateRepo,
		deps.Redis,
	)
	// Set calendar sync repo for calendar webhook handling
	if deps.CalendarSyncRepo != nil {
		webhookHandler.SetCalendarSyncRepo(deps.CalendarSyncRepo)
	}
	webhookHandler.Register(app)

	// SSE Handler (using new RealtimePort-based SSEHub)
	zlog := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	sseHandler := http.NewSSEHandler(deps.SSEHub, zlog)

	// API routes (with auth and rate limiting)
	api := app.Group("/api/v1")

	// Apply advanced rate limiting
	rateLimiter := middleware.NewAdvancedRateLimiter(middleware.DefaultRateLimitConfig())
	api.Use(rateLimiter.Handler())

	api.Use(middleware.JWTAuth(cfg.JWTSecret))

	// Audit logging for sensitive actions
	api.Use(middleware.AuditMiddleware())

	// Register handlers
	sseHandler.Register(api)

	// OAuth handler (connect, connections, disconnect - requires auth)
	oauthHandler.Register(api)

	// Mail handler with provider for direct Gmail/Outlook API access
	// API 보호 레이어: Semaphore + Rate Limiter + Debounce + Cache
	// 통합 검색 서비스: DB + Vector + Provider
	emailHandler := http.NewMailHandlerWithProvider(
		deps.EmailService,
		deps.OAuthService,
		deps.GmailProvider,
		deps.OutlookProvider,
		deps.MailRepo,
		deps.AttachmentRepo,
		deps.MessageProducer,
		deps.SyncStateRepo,
		deps.Redis,       // API 보호 및 캐시용 Redis 클라이언트
		deps.VectorStore, // 통합 검색용 벡터 스토어
		deps.Embedder,    // 통합 검색용 임베더
	)
	emailHandler.Register(api)

	// Category handler (category metadata & stats)
	categoryHandler := http.NewCategoryHandler(deps.MailRepo)
	categoryHandler.Register(api)

	// Calendar handler
	calendarHandler := http.NewCalendarHandler(deps.CalendarService)
	calendarHandler.Register(api)

	// AI handler with Orchestrator for proposal management
	aiHandler := http.NewAIHandlerFull(deps.AIService, deps.Orchestrator)
	aiHandler.Register(api)

	// Contact handler
	contactHandler := http.NewContactHandler(deps.ContactService)
	contactHandler.Register(api)

	// Label handler
	labelHandler := http.NewLabelHandler(deps.LabelRepo)
	labelHandler.Register(api)

	// Folder and Smart Folder handler
	if deps.FolderRepo != nil && deps.SmartFolderRepo != nil {
		folderHandler := http.NewFolderHandler(deps.FolderRepo, deps.SmartFolderRepo)
		folderHandler.RegisterRoutes(api)
		logger.Info("Folder and SmartFolder handlers registered")
	}

	// Sender Profile handler
	if deps.SenderProfileRepo != nil {
		senderProfileHandler := http.NewSenderProfileHandler(deps.SenderProfileRepo)
		senderProfileHandler.RegisterRoutes(api)
		logger.Info("SenderProfile handler registered")
	}

	// Settings handler (with Redis caching and shortcuts for batch endpoint)
	settingsHandler := http.NewSettingsHandlerFull(deps.SettingsService, deps.ShortcutRepo, deps.Redis)
	settingsHandler.Register(api)

	// Notification handler
	notificationHandler := http.NewNotificationHandler(deps.NotificationService)
	notificationHandler.Register(api)

	// Report handler
	reportHandler := http.NewReportHandler(deps.ReportService)
	reportHandler.Register(api)

	// Shortcut handler (authenticated routes)
	shortcutHandler := http.NewShortcutHandler(deps.ShortcutRepo)
	shortcutHandler.Register(api)

	// Public shortcut routes (no auth required - presets, definitions, defaults)
	publicAPI := app.Group("/api/v1")
	shortcutHandler.RegisterPublic(publicAPI)

	// Template handler
	if deps.TemplateService != nil {
		templateHandler := http.NewTemplateHandler(deps.TemplateService)
		templateHandler.Register(api)
	}

	// Image handler (DALL-E image generation)
	if deps.ImageService != nil {
		imageHandler := http.NewImageHandler(deps.ImageService)
		imageHandler.Register(api)
		logger.Info("Image handler registered")
	}

	// Webhook management handler (authenticated)
	webhookHandler.RegisterManagement(api)

	// Setup webhooks for all existing connections on startup (async)
	if deps.WebhookService != nil {
		go func() {
			logger.Info("Setting up webhooks for existing connections...")
			success, failed, err := deps.WebhookService.SetupAllConnections(context.Background())
			if err != nil {
				logger.Error("Failed to setup webhooks: %v", err)
			} else {
				logger.Info("Webhook setup completed: %d success, %d failed", success, failed)
			}
		}()
	}

	logger.Info("API server initialized successfully")

	return app, cleanup, nil
}
