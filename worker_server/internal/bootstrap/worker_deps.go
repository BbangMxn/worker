package bootstrap

import (
	"context"
	"strings"
	"time"

	"worker_server/adapter/out/graph"
	"worker_server/adapter/out/messaging"
	"worker_server/adapter/out/mongodb"
	"worker_server/adapter/out/persistence"
	"worker_server/adapter/out/provider"
	"worker_server/adapter/out/realtime"
	"worker_server/config"
	"worker_server/core/agent"
	"worker_server/core/agent/llm"
	"worker_server/core/agent/rag"
	agentservice "worker_server/core/agent/service"
	"worker_server/core/agent/tools"
	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/core/service"
	"worker_server/core/service/ai"
	"worker_server/core/service/auth"
	"worker_server/core/service/calendar"
	"worker_server/core/service/classification"
	"worker_server/core/service/common"
	"worker_server/core/service/contact"
	imageservice "worker_server/core/service/image"
	"worker_server/core/service/email"
	"worker_server/core/service/notification"
	"worker_server/core/service/report"
	"worker_server/infra/database"
	"worker_server/pkg/logger"
	"worker_server/pkg/metrics"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver for database/sql
	"github.com/jmoiron/sqlx"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/oauth2"
)

type Dependencies struct {
	Config  *config.Config
	DB      *pgxpool.Pool
	SQLDB   *sqlx.DB
	Redis   *redis.Client
	MongoDB *mongo.Client
	Neo4j   neo4j.DriverWithContext

	// Repositories
	MailRepo           out.EmailRepository
	AttachmentRepo     out.AttachmentRepository
	OAuthRepo          out.OAuthRepository
	CalendarRepo       out.CalendarRepository
	CalendarSyncRepo   out.CalendarSyncRepository
	MailBodyRepo       out.EmailBodyRepository
	SyncStateRepo      out.SyncStateRepository
	ContactRepo        *persistence.ContactAdapter
	LabelRepo          *persistence.LabelAdapter
	SettingsRepo       *persistence.SettingsAdapter
	SettingsDomainRepo *persistence.SettingsDomainWrapper
	NotificationRepo   *persistence.NotificationAdapter
	WebhookRepo        *persistence.WebhookAdapter
	ShortcutRepo       *persistence.ShortcutAdapter
	TemplateRepo       *persistence.TemplateAdapter
	FolderRepo         *persistence.FolderAdapter
	SmartFolderRepo    *persistence.SmartFolderAdapter
	SenderProfileRepo  *persistence.SenderProfileAdapter
	KnownDomainRepo    *persistence.KnownDomainAdapter

	// Neo4j Adapters (Personalization)
	PersonalizationRepo out.ExtendedPersonalizationStore

	// Providers
	GmailProvider          *provider.GmailAdapter
	OutlookProvider        *provider.OutlookAdapter
	GoogleCalendarProvider *provider.GoogleCalendarAdapter

	// Messaging
	MessageProducer out.MessageProducer

	// Realtime
	RealtimeAdapter *realtime.SSEAdapter
	SSEHub          *realtime.SSEHub

	// Cache
	CacheService *common.CacheService
	// Note: EmailListCache is created directly in EmailHandler for optimistic updates

	// Services
	EmailService            *mail.Service
	MailSyncService        *mail.SyncService
	OAuthService           *auth.OAuthService
	CalendarService        *calendar.Service
	CalendarSyncService    *calendar.SyncService
	ContactService         *contact.Service
	AIService              *ai.Service
	SettingsService        *auth.SettingsService
	NotificationService    *notification.Service
	WebhookService         *notification.WebhookService
	ReportService          *report.Service
	TemplateService        *service.TemplateService
	ClassificationPipeline *classification.Pipeline

	// Agent
	LLMClient     *llm.Client
	ImageClient   *llm.ImageClient
	RAGRetriever  *rag.Retriever
	RAGIndexer    *rag.IndexerService
	StyleAnalyzer *rag.StyleAnalyzer
	AgentService  *agentservice.AgentService
	Orchestrator  *agent.Orchestrator

	// Image Generation
	ImageService *imageservice.Service

	// RAG Components (for unified search)
	VectorStore *rag.VectorStore
	Embedder    *rag.Embedder
}

func NewDependencies(cfg *config.Config) (*Dependencies, func(), error) {
	deps := &Dependencies{Config: cfg}
	var cleanups []func()

	// Database (pgxpool)
	db, err := database.NewPostgres(cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	deps.DB = db
	cleanups = append(cleanups, func() { db.Close() })

	// Database (sqlx for adapters that need it)
	// Add statement_cache_mode=describe to avoid prepared statement conflicts with PgBouncer
	logger.Debug("Connecting to database via sqlx...")
	sqlxURL := cfg.DatabaseURL
	if strings.Contains(sqlxURL, "?") {
		sqlxURL += "&default_query_exec_mode=simple_protocol"
	} else {
		sqlxURL += "?default_query_exec_mode=simple_protocol"
	}
	sqlDB, err := sqlx.Connect("pgx", sqlxURL)
	if err != nil {
		logger.Error("sqlx connection failed: %v", err)
		logger.Debug("DATABASE_URL length: %d", len(cfg.DatabaseURL))
	} else {
		// Optimize connection pool settings for production
		// Based on typical backend workload patterns
		sqlDB.SetMaxOpenConns(25)                  // Max concurrent connections
		sqlDB.SetMaxIdleConns(10)                  // Keep idle connections ready
		sqlDB.SetConnMaxLifetime(30 * time.Minute) // Recycle connections periodically
		sqlDB.SetConnMaxIdleTime(5 * time.Minute)  // Close idle connections after 5min

		deps.SQLDB = sqlDB
		cleanups = append(cleanups, func() { sqlDB.Close() })

		// Register with global pool monitor
		metrics.RegisterPool("postgres", sqlDB.DB)

		logger.Info("sqlx database connection successful (pool: max=%d, idle=%d)", 25, 10)
	}

	// Redis
	redisClient, err := database.NewRedis(cfg.RedisURL)
	if err != nil {
		logger.Warn("Redis connection failed: %v", err)
	} else {
		deps.Redis = redisClient
		cleanups = append(cleanups, func() { redisClient.Close() })

		// Initialize Cache Service (L2 - Redis) - other deps added later
		deps.CacheService = common.NewCacheService(
			redisClient,
			nil, // mongoRepo - added after MongoDB init
			nil, // emailRepo - added after repo init
			nil, // attachmentRepo - added after repo init (Phase 3: lazy 복구)
			nil, // provider - added after provider init
			nil, // oauthService - added after service init
			nil, // emailFetcher - added via SetEmailFetcher
			common.DefaultCacheConfig(),
		)
		logger.Info("CacheService (L2 Redis) initialized")
		// Note: HybridCache removed - EmailListCache in EmailHandler handles L1+L2 with optimistic updates
	}

	// MongoDB
	if cfg.MongoDBURL != "" {
		mongoClient, err := mongodb.NewClient(cfg.MongoDBURL, cfg.MongoDBName)
		if err != nil {
			logger.Warn("MongoDB connection failed: %v", err)
		} else {
			deps.MongoDB = mongoClient
			cleanups = append(cleanups, func() {
				mongoClient.Disconnect(context.Background())
			})

			// Mail Body Repository (MongoDB)
			mongoDB := mongoClient.Database(cfg.MongoDBName)
			deps.MailBodyRepo = mongodb.NewMailBodyAdapter(mongoDB)

			// CacheService에 MongoRepo 주입
			if deps.CacheService != nil {
				deps.CacheService.SetMongoRepo(deps.MailBodyRepo)
			}
		}
	}

	// Neo4j (Personalization - 스타일, 톤, 선호도)
	if cfg.Neo4jURL != "" {
		neo4jDriver, err := graph.NewDriver(cfg.Neo4jURL, cfg.Neo4jUsername, cfg.Neo4jPassword)
		if err != nil {
			logger.Warn("Neo4j connection failed: %v", err)
		} else {
			deps.Neo4j = neo4jDriver
			cleanups = append(cleanups, func() {
				neo4jDriver.Close(context.Background())
			})

			// Personalization Repository (Neo4j)
			personalizationAdapter := graph.NewPersonalizationAdapter(neo4jDriver, "neo4j")
			deps.PersonalizationRepo = personalizationAdapter

			// Ensure indexes
			if err := personalizationAdapter.EnsureIndexes(context.Background()); err != nil {
				logger.Warn("Failed to ensure Neo4j indexes: %v", err)
			}
			if err := personalizationAdapter.EnsureExtendedIndexes(context.Background()); err != nil {
				logger.Warn("Failed to ensure Neo4j extended indexes: %v", err)
			}
			logger.Info("Neo4j PersonalizationAdapter initialized")
		}
	}

	// Message Producer (Redis Streams)
	if deps.Redis != nil {
		deps.MessageProducer = messaging.NewRedisProducer(deps.Redis)
	}

	// Repositories
	if deps.SQLDB != nil {
		deps.MailRepo = persistence.NewMailAdapter(deps.SQLDB)
		deps.AttachmentRepo = persistence.NewAttachmentAdapter(deps.SQLDB)
		deps.OAuthRepo = persistence.NewOAuthAdapter(deps.SQLDB)
		deps.CalendarRepo = persistence.NewCalendarAdapter(deps.SQLDB)
		deps.CalendarSyncRepo = persistence.NewCalendarSyncAdapter(deps.SQLDB)
		deps.SyncStateRepo = persistence.NewSyncStateAdapter(deps.SQLDB)
		deps.ContactRepo = persistence.NewContactAdapter(deps.SQLDB)
		deps.LabelRepo = persistence.NewLabelAdapter(deps.SQLDB)
		deps.SettingsRepo = persistence.NewSettingsAdapter(deps.SQLDB)
		deps.SettingsDomainRepo = persistence.NewSettingsDomainWrapper(deps.SettingsRepo)
		deps.NotificationRepo = persistence.NewNotificationAdapter(deps.SQLDB)
		deps.WebhookRepo = persistence.NewWebhookAdapter(deps.SQLDB)
		deps.ShortcutRepo = persistence.NewShortcutAdapter(deps.SQLDB)
		deps.TemplateRepo = persistence.NewTemplateAdapter(deps.SQLDB)
		deps.FolderRepo = persistence.NewFolderAdapter(deps.SQLDB)
		deps.SmartFolderRepo = persistence.NewSmartFolderAdapter(deps.SQLDB)
		deps.SenderProfileRepo = persistence.NewSenderProfileAdapter(deps.SQLDB)
		deps.KnownDomainRepo = persistence.NewKnownDomainAdapter(deps.SQLDB)
		logger.Info("Folder, SmartFolder, SenderProfile, KnownDomain repositories initialized")

		// CacheService에 필요한 Repository 주입
		if deps.CacheService != nil {
			deps.CacheService.SetAttachmentRepo(deps.AttachmentRepo)
			deps.CacheService.SetMailRepo(deps.MailRepo)
		}
	}

	// Classification Pipeline (3-stage: Header -> Domain -> LLM)
	// Will be fully initialized after LLM client is created
	// Placeholder - initialized later when LLMClient is available

	// Realtime (SSE)
	zlog := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()
	deps.RealtimeAdapter = realtime.NewSSEAdapter(zlog)
	deps.SSEHub = realtime.NewSSEHub(deps.RealtimeAdapter, zlog)

	// Gmail Provider
	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		deps.GmailProvider = provider.NewGmailAdapter(&provider.GmailConfig{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  cfg.GoogleRedirectURL,
			ProjectID:    cfg.GoogleProjectID,
		})

		// Google Calendar Provider (uses same OAuth config)
		oauthConfig := &oauth2.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  cfg.GoogleRedirectURL,
			Scopes: []string{
				"https://www.googleapis.com/auth/calendar",
				"https://www.googleapis.com/auth/calendar.events",
			},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://accounts.google.com/o/oauth2/auth",
				TokenURL: "https://oauth2.googleapis.com/token",
			},
		}
		deps.GoogleCalendarProvider = provider.NewGoogleCalendarAdapter(oauthConfig, "")
		logger.Info("Google Calendar Provider initialized")
	}

	// Outlook Provider
	if cfg.MicrosoftClientID != "" && cfg.MicrosoftClientSecret != "" {
		deps.OutlookProvider = provider.NewOutlookAdapterWithConfig(&provider.OutlookConfig{
			ClientID:     cfg.MicrosoftClientID,
			ClientSecret: cfg.MicrosoftClientSecret,
			TenantID:     cfg.MicrosoftTenantID,
			RedirectURL:  cfg.MicrosoftRedirectURL,
		})
		logger.Info("Outlook Provider initialized")
	}

	// LLM Client with config
	if cfg.OpenAIAPIKey != "" {
		deps.LLMClient = llm.NewClientWithConfig(llm.ClientConfig{
			APIKey:      cfg.OpenAIAPIKey,
			Model:       cfg.LLMModel,
			MaxTokens:   cfg.LLMMaxTokens,
			Temperature: cfg.LLMTemperature,
		})

		// Image Client (DALL-E)
		deps.ImageClient = llm.NewImageClient(cfg.OpenAIAPIKey)
		logger.Info("Image Client (DALL-E) initialized")

		// Image Service (repositories are optional, nil = in-memory only)
		deps.ImageService = imageservice.NewService(deps.ImageClient, nil, nil)
		logger.Info("Image Service initialized")

		// Initialize Classification Pipeline with LLM client
		if deps.KnownDomainRepo != nil && deps.SenderProfileRepo != nil {
			deps.ClassificationPipeline = classification.NewPipeline(
				deps.KnownDomainRepo,
				deps.SenderProfileRepo,
				deps.SettingsDomainRepo,
				deps.LLMClient,
			)
			logger.Info("Classification Pipeline initialized (UserRules -> Header -> Domain -> LLM)")
		}
	}

	// RAG Components
	if deps.LLMClient != nil {
		deps.Embedder = rag.NewEmbedder(deps.LLMClient)
		deps.VectorStore = rag.NewVectorStore(db)
		deps.RAGRetriever = rag.NewRetriever(deps.Embedder, deps.VectorStore)
		deps.RAGIndexer = rag.NewIndexerService(deps.Embedder, deps.VectorStore)

		// StyleAnalyzer with Neo4j PersonalizationStore
		if deps.PersonalizationRepo != nil {
			deps.StyleAnalyzer = rag.NewStyleAnalyzer(deps.Embedder, deps.PersonalizationRepo, deps.VectorStore)
			logger.Info("StyleAnalyzer initialized with Neo4j PersonalizationStore")
		}
	}

	// Agent Service with Tools
	if deps.LLMClient != nil && deps.RAGRetriever != nil {
		// Create tool registry
		toolRegistry := tools.NewRegistry()

		// Register mail tools if mail repo is available
		if deps.MailRepo != nil {
			// Create domain wrapper for mail repository
			mailAdapter, ok := deps.MailRepo.(*persistence.MailAdapter)
			if ok {
				mailDomainRepo := persistence.NewMailDomainWrapper(mailAdapter, deps.MailBodyRepo)
				toolRegistry.RegisterAll(
					tools.NewMailListTool(mailDomainRepo),
					tools.NewMailReadTool(mailDomainRepo),
					tools.NewMailSearchTool(mailDomainRepo),
					tools.NewMailReplyTool(mailDomainRepo),
					// New action tools
					tools.NewMailDeleteTool(mailDomainRepo),
					tools.NewMailArchiveTool(mailDomainRepo),
					tools.NewMailMarkReadTool(mailDomainRepo),
					tools.NewMailStarTool(mailDomainRepo),
				)

				// Register LLM-based tools if LLM client is available
				if deps.LLMClient != nil {
					toolRegistry.Register(tools.NewMailTranslateTool(mailDomainRepo, deps.LLMClient))
					toolRegistry.Register(tools.NewMailSummarizeTool(mailDomainRepo, deps.SettingsDomainRepo, deps.LLMClient))
				}
			}
		}

		// Register mail send tool (no repo needed, uses proposal)
		toolRegistry.Register(tools.NewMailSendTool())

		// Register contact tools if contact repo is available
		if deps.ContactRepo != nil {
			contactDomainRepo := persistence.NewContactDomainWrapper(deps.ContactRepo)
			toolRegistry.RegisterAll(
				tools.NewContactListTool(contactDomainRepo),
				tools.NewContactGetTool(contactDomainRepo),
				tools.NewContactSearchTool(contactDomainRepo),
			)
		}

		// Register calendar tools
		if deps.CalendarRepo != nil {
			calendarAdapter, ok := deps.CalendarRepo.(*persistence.CalendarAdapter)
			if ok {
				calendarDomainRepo := persistence.NewCalendarDomainWrapper(calendarAdapter)
				toolRegistry.RegisterAll(
					tools.NewCalendarListTool(calendarDomainRepo),
					tools.NewCalendarCreateTool(calendarDomainRepo),
					tools.NewCalendarFindFreeTool(calendarDomainRepo),
					// New calendar tools
					tools.NewCalendarGetTool(calendarDomainRepo),
					tools.NewCalendarDeleteTool(calendarDomainRepo),
					tools.NewCalendarUpdateTool(calendarDomainRepo),
				)
				logger.Info("Calendar tools registered")
			}
		}

		// Register label tools
		if deps.LabelRepo != nil && deps.MailRepo != nil {
			mailAdapter, ok := deps.MailRepo.(*persistence.MailAdapter)
			if ok {
				mailDomainRepo := persistence.NewMailDomainWrapper(mailAdapter, deps.MailBodyRepo)
				toolRegistry.RegisterAll(
					tools.NewLabelListTool(deps.LabelRepo),
					tools.NewLabelAddTool(mailDomainRepo, deps.LabelRepo),
					tools.NewLabelRemoveTool(mailDomainRepo, deps.LabelRepo),
					tools.NewLabelCreateTool(deps.LabelRepo),
				)
				logger.Info("Label tools registered")
			}
		}

		// Create agent service with tools
		deps.AgentService = agentservice.NewAgentServiceWithTools(deps.LLMClient, deps.RAGRetriever, toolRegistry)
		logger.Info("Agent Service initialized with %d tools", len(toolRegistry.ListNames()))

		// Create Orchestrator (will be fully configured after OAuthService is created)
		deps.Orchestrator = agent.NewOrchestrator(deps.LLMClient, deps.RAGRetriever, toolRegistry)
		logger.Info("Orchestrator initialized")
	}

	// OAuth Service with Producer for triggering sync
	if deps.SQLDB == nil {
		logger.Warn("SQLDB is nil, OAuth connections will NOT be saved!")
	} else {
		logger.Debug("SQLDB initialized successfully for OAuth")
	}
	deps.OAuthService = auth.NewOAuthServiceWithConfig(
		cfg.GoogleClientID,
		cfg.GoogleClientSecret,
		cfg.GoogleRedirectURL,
		cfg.MicrosoftClientID,
		cfg.MicrosoftClientSecret,
		cfg.MicrosoftRedirectURL,
		deps.SQLDB,
	)
	// Set message producer for triggering sync after OAuth
	if deps.MessageProducer != nil {
		deps.OAuthService.SetMessageProducer(deps.MessageProducer)
	}

	// CacheService에 Provider와 OAuthService 주입 (본문 lazy loading용)
	if deps.CacheService != nil {
		if deps.GmailProvider != nil {
			deps.CacheService.SetProvider(deps.GmailProvider)
		}
		deps.CacheService.SetOAuthService(deps.OAuthService)
		logger.Info("CacheService: Provider and OAuthService configured for body fetching")
	}

	// Configure Orchestrator with providers for proposal execution
	if deps.Orchestrator != nil {
		if deps.GmailProvider != nil {
			deps.Orchestrator.SetMailProvider(deps.GmailProvider)
			logger.Debug("Orchestrator: Mail provider configured")
		}
		if deps.GoogleCalendarProvider != nil {
			deps.Orchestrator.SetCalendarProvider(deps.GoogleCalendarProvider)
			logger.Debug("Orchestrator: Calendar provider configured")
		}
		if deps.OAuthService != nil {
			deps.Orchestrator.SetOAuthProvider(deps.OAuthService)
			logger.Debug("Orchestrator: OAuth provider configured")
		}
		if deps.LabelRepo != nil {
			deps.Orchestrator.SetLabelRepository(deps.LabelRepo)
			logger.Debug("Orchestrator: Label repository configured")
		}
	}

	// Configure AgentService with providers for proposal execution
	if deps.AgentService != nil {
		if deps.GmailProvider != nil {
			deps.AgentService.SetMailProvider(deps.GmailProvider)
			logger.Debug("AgentService: Mail provider configured")
		}
		if deps.GoogleCalendarProvider != nil {
			deps.AgentService.SetCalendarProvider(deps.GoogleCalendarProvider)
			logger.Debug("AgentService: Calendar provider configured")
		}
		if deps.OAuthService != nil {
			deps.AgentService.SetOAuthProvider(deps.OAuthService)
			logger.Debug("AgentService: OAuth provider configured")
		}
	}

	// Mail Service with repository and provider
	if deps.MailRepo != nil {
		mailAdapter, ok := deps.MailRepo.(*persistence.MailAdapter)
		if ok {
			mailDomainRepo := persistence.NewMailDomainWrapper(mailAdapter, deps.MailBodyRepo)
			deps.EmailService = mail.NewServiceFull(
				mailDomainRepo,
				deps.LabelRepo, // LabelRepo for label operations
				deps.MailRepo,
				deps.CacheService, // L2 Redis cache
				deps.GmailProvider,
				deps.OAuthService,
				deps.MessageProducer, // async provider sync
			)
			logger.Info("EmailService initialized with CacheService and LabelRepo")
		} else {
			deps.EmailService = mail.NewService(nil, nil)
		}
	} else {
		deps.EmailService = mail.NewService(nil, nil)
	}

	// Mail Sync Service (새로운 Pub/Sub 기반 동기화)
	if deps.MailRepo != nil && deps.SyncStateRepo != nil && deps.GmailProvider != nil {
		deps.MailSyncService = mail.NewSyncService(
			deps.MailRepo,
			deps.MailBodyRepo,
			deps.AttachmentRepo,
			deps.SyncStateRepo,
			deps.GmailProvider,
			deps.OAuthService,
			deps.MessageProducer,
			deps.RealtimeAdapter,
		)
		logger.Info("MailSyncService initialized")
	}

	// Calendar Service - with domain wrapper and provider support
	if deps.CalendarRepo != nil {
		calendarAdapter, ok := deps.CalendarRepo.(*persistence.CalendarAdapter)
		if ok {
			calendarDomainRepo := persistence.NewCalendarDomainWrapper(calendarAdapter)
			// Initialize with providers for direct API access
			deps.CalendarService = calendar.NewServiceWithProviders(
				calendarDomainRepo,
				deps.OAuthService,
				deps.GoogleCalendarProvider,
				nil, // Outlook provider - TODO: add when ready
			)
			logger.Info("CalendarService initialized with repository and providers")
		}
	}
	if deps.CalendarService == nil {
		deps.CalendarService = calendar.NewService(nil)
		logger.Warn("CalendarService initialized without repository")
	}

	// Calendar Sync Service (real-time sync with Watch/Webhook)
	if deps.CalendarRepo != nil && deps.CalendarSyncRepo != nil {
		calendarAdapter, ok := deps.CalendarRepo.(*persistence.CalendarAdapter)
		if ok {
			calendarDomainRepo := persistence.NewCalendarDomainWrapper(calendarAdapter)
			deps.CalendarSyncService = calendar.NewSyncService(
				calendarDomainRepo,
				deps.CalendarSyncRepo,
				deps.GoogleCalendarProvider, // Primary provider (Google)
				deps.OAuthService,
				deps.MessageProducer,
				deps.RealtimeAdapter,
			)
			logger.Info("CalendarSyncService initialized with all dependencies")
		}
	}
	if deps.CalendarSyncService == nil {
		deps.CalendarSyncService = calendar.NewSyncService(
			nil, nil, nil, deps.OAuthService, deps.MessageProducer, deps.RealtimeAdapter,
		)
		logger.Warn("CalendarSyncService initialized without repositories")
	}

	// Contact Service - using domain wrapper for type alignment
	if deps.ContactRepo != nil {
		contactDomainRepo := persistence.NewContactDomainWrapper(deps.ContactRepo)
		deps.ContactService = contact.NewService(contactDomainRepo)
	}

	// Settings Service - using domain wrapper for type alignment
	if deps.SettingsRepo != nil {
		settingsDomainRepo := persistence.NewSettingsDomainWrapper(deps.SettingsRepo)
		deps.SettingsService = auth.NewSettingsService(settingsDomainRepo)
	}

	// Notification Service
	deps.NotificationService = notification.NewService(deps.NotificationRepo, nil) // SSE hub added later

	// Webhook Service
	deps.WebhookService = notification.NewWebhookService(deps.WebhookRepo, deps.OAuthService, deps.GmailProvider)

	// Connect webhook auto-subscription to OAuth callback
	if deps.WebhookService != nil && deps.OAuthService != nil {
		deps.OAuthService.SetWebhookSetup(func(ctx context.Context, connectionID int64) error {
			_, err := deps.WebhookService.SetupWatch(ctx, connectionID)
			return err
		})
		logger.Info("Webhook auto-subscription configured for OAuth")
	}

	// AI Service
	// Create domain wrapper for AI service if mail repo is available
	var aiEmailRepo domain.EmailRepository
	if deps.MailRepo != nil {
		if mailAdapter, ok := deps.MailRepo.(*persistence.MailAdapter); ok {
			aiEmailRepo = persistence.NewMailDomainWrapper(mailAdapter, deps.MailBodyRepo)
		}
	}
	deps.AIService = ai.NewService(aiEmailRepo, nil, deps.LLMClient, deps.RAGRetriever, deps.RAGIndexer, nil)

	// Connect Classification Pipeline to AI Service (4-stage classification)
	if deps.ClassificationPipeline != nil {
		deps.AIService.SetClassificationPipeline(deps.ClassificationPipeline)
	}

	// Report Service
	deps.ReportService = report.NewService(nil, nil, deps.LLMClient) // Email/Report repos added later

	// Template Service
	if deps.TemplateRepo != nil {
		deps.TemplateService = service.NewTemplateService(deps.TemplateRepo)
	}

	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	return deps, cleanup, nil
}

func (d *Dependencies) HealthCheck(ctx context.Context) error {
	// Check database
	if err := d.DB.Ping(ctx); err != nil {
		return err
	}

	// Check Redis
	if d.Redis != nil {
		if err := d.Redis.Ping(ctx).Err(); err != nil {
			return err
		}
	}

	return nil
}
