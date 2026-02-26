// Package common provides shared utilities for services.
package common

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/goccy/go-json"

	"worker_server/core/domain"
	"worker_server/core/port/out"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
	"golang.org/x/sync/singleflight"
)

// =============================================================================
// Cache Configuration
// =============================================================================

// CacheConfig holds cache TTL configurations
type CacheConfig struct {
	// Redis TTLs
	BodyTTL     time.Duration // 메일 본문 캐시 TTL (기본 10분)
	ListTTL     time.Duration // 메일 목록 캐시 TTL (기본 5분)
	MetaTTL     time.Duration // 메타데이터 캐시 TTL (기본 10분)
	AIResultTTL time.Duration // AI 결과 캐시 TTL (기본 1시간)

	// MongoDB TTL
	MongoBodyTTLDays int // MongoDB 본문 TTL (기본 30일)

	// Compression
	CompressionThreshold int // 압축 임계값 (기본 1KB)
}

// DefaultCacheConfig returns default cache configuration
// 최적화: TTL 증가로 캐시 히트율 향상
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		BodyTTL:              30 * time.Minute, // 10분 → 30분 (본문은 잘 변하지 않음)
		ListTTL:              15 * time.Minute, // 5분 → 15분 (목록 캐시 유지)
		MetaTTL:              20 * time.Minute, // 10분 → 20분
		AIResultTTL:          2 * time.Hour,    // 1시간 → 2시간 (AI 결과는 고정)
		MongoBodyTTLDays:     30,
		CompressionThreshold: 2048, // 1KB → 2KB (작은 데이터는 압축 오버헤드가 더 큼)
	}
}

// =============================================================================
// Cache Keys
// =============================================================================

const (
	keyPrefixBody     = "body:"     // body:{email_id}
	keyPrefixList     = "list:"     // list:{user_id}:{folder}:{page}
	keyPrefixMeta     = "meta:"     // meta:{email_id}
	keyPrefixAI       = "ai:"       // ai:{email_id}
	keyPrefixPrefetch = "prefetch:" // prefetch:{user_id}
)

func bodyKey(emailID int64) string {
	return fmt.Sprintf("%s%d", keyPrefixBody, emailID)
}

func listKey(userID string, folder string, page int) string {
	return fmt.Sprintf("%s%s:%s:%d", keyPrefixList, userID, folder, page)
}

func metaKey(emailID int64) string {
	return fmt.Sprintf("%s%d", keyPrefixMeta, emailID)
}

func aiKey(emailID int64) string {
	return fmt.Sprintf("%s%d", keyPrefixAI, emailID)
}

// =============================================================================
// Cache Service - 3-Tier Caching
// =============================================================================

// OAuthTokenProvider interface for getting OAuth tokens (to avoid circular imports)
type OAuthTokenProvider interface {
	GetOAuth2Token(ctx context.Context, connectionID int64) (*oauth2.Token, error)
}

// EmailListFetcher interface for fetching email lists (to avoid circular imports)
type EmailListFetcher interface {
	ListEmails(ctx context.Context, filter *domain.EmailFilter) ([]*domain.Email, int, error)
}

// CacheService provides 3-tier caching: Redis -> MongoDB -> Provider
type CacheService struct {
	redis          *redis.Client
	mongoRepo      out.EmailBodyRepository
	emailRepo       out.EmailRepository
	attachmentRepo out.AttachmentRepository // Phase 3: 첨부파일 lazy 복구용
	provider       out.EmailProviderPort
	oauthService   OAuthTokenProvider
	emailFetcher   EmailListFetcher // for prefetching email lists
	config         *CacheConfig

	// Singleflight for deduplicating concurrent requests
	// 최적화: Tier 2 (MongoDB), Tier 3 (Provider) 각각 적용
	mongoFlight singleflight.Group // MongoDB cache miss 중복 방지
	bodyFlight  singleflight.Group // Provider fetch 중복 방지

	// Metrics
	metrics *CacheMetrics
}

// CacheMetrics tracks cache hit/miss statistics
type CacheMetrics struct {
	RedisHits    int64
	RedisMisses  int64
	MongoHits    int64
	MongoMisses  int64
	ProviderHits int64
}

// NewCacheService creates a new cache service
func NewCacheService(
	redis *redis.Client,
	mongoRepo out.EmailBodyRepository,
	emailRepo out.EmailRepository,
	attachmentRepo out.AttachmentRepository,
	provider out.EmailProviderPort,
	oauthService OAuthTokenProvider,
	emailFetcher EmailListFetcher,
	config *CacheConfig,
) *CacheService {
	if config == nil {
		config = DefaultCacheConfig()
	}
	return &CacheService{
		redis:          redis,
		mongoRepo:      mongoRepo,
		emailRepo:       emailRepo,
		attachmentRepo: attachmentRepo,
		provider:       provider,
		oauthService:   oauthService,
		emailFetcher:   emailFetcher,
		config:         config,
		metrics:        &CacheMetrics{},
	}
}

// SetEmailFetcher sets the email fetcher (for late binding to avoid circular deps)
func (s *CacheService) SetEmailFetcher(fetcher EmailListFetcher) {
	s.emailFetcher = fetcher
}

// SetAttachmentRepo sets the attachment repository (for late binding - Phase 3: lazy 복구)
func (s *CacheService) SetAttachmentRepo(repo out.AttachmentRepository) {
	s.attachmentRepo = repo
}

// SetMongoRepo sets the MongoDB repository (for late binding)
func (s *CacheService) SetMongoRepo(repo out.EmailBodyRepository) {
	s.mongoRepo = repo
}

// SetMailRepo sets the mail repository (for late binding)
func (s *CacheService) SetMailRepo(repo out.EmailRepository) {
	s.emailRepo = repo
}

// SetProvider sets the mail provider (for late binding)
func (s *CacheService) SetProvider(provider out.EmailProviderPort) {
	s.provider = provider
}

// SetOAuthService sets the OAuth service (for late binding)
func (s *CacheService) SetOAuthService(oauthService OAuthTokenProvider) {
	s.oauthService = oauthService
}

// GetAttachments retrieves attachment metadata for an email
func (s *CacheService) GetAttachments(ctx context.Context, emailID int64) ([]*out.EmailAttachmentEntity, error) {
	if s.attachmentRepo == nil {
		return nil, nil
	}
	return s.attachmentRepo.GetByEmailID(ctx, emailID)
}

// =============================================================================
// Body Cache - 3 Tier
// =============================================================================

// GetBody retrieves email body with 3-tier caching
// 30일 정책: 30일 이내 메일만 MongoDB에 저장, 오래된 메일은 API에서만 조회
// 최적화: MongoDB와 Provider 모두 singleflight 적용으로 중복 요청 방지
func (s *CacheService) GetBody(ctx context.Context, emailID int64, connectionID int64) (*domain.EmailBody, error) {
	// Tier 1: Redis (단기 캐시 - 모든 메일) - 락 없이 빠른 확인
	body, err := s.getBodyFromRedis(ctx, emailID)
	if err == nil && body != nil && s.hasBodyContent(body) {
		s.metrics.RedisHits++
		return cleanEmptyBodyMarker(body), nil
	}
	s.metrics.RedisMisses++

	// Tier 2 + 3: MongoDB → Provider (singleflight로 중복 요청 통합)
	key := fmt.Sprintf("%d:%d", emailID, connectionID)
	result, err, _ := s.mongoFlight.Do(key, func() (interface{}, error) {
		// Double-check Redis (다른 요청이 이미 저장했을 수 있음)
		if cachedBody, _ := s.getBodyFromRedis(ctx, emailID); cachedBody != nil && s.hasBodyContent(cachedBody) {
			return cachedBody, nil
		}

		// Tier 2: MongoDB 조회
		mongoBody, mongoErr := s.getBodyFromMongo(ctx, emailID)
		if mongoErr == nil && mongoBody != nil && s.hasBodyContent(mongoBody) {
			s.metrics.MongoHits++
			// Cache to Redis (동기 - 다음 요청 빠르게)
			s.cacheBodyToRedis(ctx, emailID, mongoBody)
			return mongoBody, nil
		}
		s.metrics.MongoMisses++

		// Tier 3: Provider API - 별도 singleflight로 Provider 호출 중복 방지
		providerResult, providerErr, _ := s.bodyFlight.Do(key, func() (interface{}, error) {
			// Double-check Redis again
			if cachedBody, _ := s.getBodyFromRedis(ctx, emailID); cachedBody != nil && s.hasBodyContent(cachedBody) {
				return cachedBody, nil
			}

			// Provider에서 fetch
			fetchedBody, fetchErr := s.fetchBodyFromProvider(ctx, emailID, connectionID)
			if fetchErr != nil {
				return nil, fetchErr
			}

			// 30일 정책 적용
			shouldSaveToMongo := true
			if s.emailRepo != nil {
				if entity, _ := s.emailRepo.GetByID(ctx, emailID); entity != nil {
					thirtyDaysAgo := time.Now().AddDate(0, 0, -s.config.MongoBodyTTLDays)
					if entity.ReceivedAt.Before(thirtyDaysAgo) {
						shouldSaveToMongo = false
					}
				}
			}

			// Redis 저장 (동기)
			s.cacheBodyToRedis(ctx, emailID, fetchedBody)

			// MongoDB 저장 (비동기)
			if shouldSaveToMongo {
				go s.cacheBodyToMongo(context.Background(), emailID, connectionID, fetchedBody)
			}

			s.metrics.ProviderHits++
			return fetchedBody, nil
		})

		if providerErr != nil {
			return nil, providerErr
		}
		return providerResult, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to fetch body: %w", err)
	}

	return cleanEmptyBodyMarker(result.(*domain.EmailBody)), nil
}

// getBodyFromRedis retrieves body from Redis cache
func (s *CacheService) getBodyFromRedis(ctx context.Context, emailID int64) (*domain.EmailBody, error) {
	if s.redis == nil {
		return nil, nil
	}

	key := bodyKey(emailID)
	data, err := s.redis.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Cache miss
		}
		return nil, err
	}

	// Decompress
	decompressed, err := decompress(data)
	if err != nil {
		return nil, err
	}

	var body domain.EmailBody
	if err := json.Unmarshal(decompressed, &body); err != nil {
		return nil, err
	}

	return &body, nil
}

// cacheBodyToRedis stores body in Redis cache
func (s *CacheService) cacheBodyToRedis(ctx context.Context, emailID int64, body *domain.EmailBody) error {
	if s.redis == nil {
		return nil
	}

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	// Compress if large enough
	if len(data) > s.config.CompressionThreshold {
		data, err = compress(data)
		if err != nil {
			return err
		}
	}

	key := bodyKey(emailID)
	return s.redis.Set(ctx, key, data, s.config.BodyTTL).Err()
}

// getBodyFromMongo retrieves body from MongoDB
func (s *CacheService) getBodyFromMongo(ctx context.Context, emailID int64) (*domain.EmailBody, error) {
	if s.mongoRepo == nil {
		return nil, nil
	}

	entity, err := s.mongoRepo.GetBody(ctx, emailID)
	if err != nil || entity == nil {
		return nil, err
	}

	return &domain.EmailBody{
		EmailID:  entity.EmailID,
		TextBody: entity.Text,
		HTMLBody: entity.HTML,
	}, nil
}

// cacheBodyToMongo stores body in MongoDB
func (s *CacheService) cacheBodyToMongo(ctx context.Context, emailID int64, connectionID int64, body *domain.EmailBody) error {
	if s.mongoRepo == nil {
		return nil
	}

	// Get external ID from mail repo
	externalID := ""
	if s.emailRepo != nil {
		if entity, err := s.emailRepo.GetByID(ctx, emailID); err == nil && entity != nil {
			externalID = entity.ExternalID
		}
	}

	entity := &out.MailBodyEntity{
		EmailID:      emailID,
		ConnectionID: connectionID,
		ExternalID:   externalID,
		HTML:         body.HTMLBody,
		Text:         body.TextBody,
		CachedAt:     time.Now(),
		ExpiresAt:    time.Now().AddDate(0, 0, s.config.MongoBodyTTLDays),
		TTLDays:      s.config.MongoBodyTTLDays,
	}

	return s.mongoRepo.SaveBody(ctx, entity)
}

// fetchBodyFromProvider fetches body from Gmail/Outlook API
func (s *CacheService) fetchBodyFromProvider(ctx context.Context, emailID int64, connectionID int64) (*domain.EmailBody, error) {
	log.Printf("[CacheService.fetchBodyFromProvider] Starting for emailID=%d, connectionID=%d", emailID, connectionID)

	if s.provider == nil || s.oauthService == nil {
		log.Printf("[CacheService.fetchBodyFromProvider] ERROR: provider=%v, oauthService=%v", s.provider != nil, s.oauthService != nil)
		return nil, fmt.Errorf("provider or oauth service not configured")
	}

	// Get email external ID
	if s.emailRepo == nil {
		log.Printf("[CacheService.fetchBodyFromProvider] ERROR: emailRepo is nil")
		return nil, fmt.Errorf("mail repository not configured")
	}

	entity, err := s.emailRepo.GetByID(ctx, emailID)
	if err != nil {
		log.Printf("[CacheService.fetchBodyFromProvider] ERROR: failed to get email %d: %v", emailID, err)
		return nil, fmt.Errorf("failed to get email: %w", err)
	}
	log.Printf("[CacheService.fetchBodyFromProvider] Found email: ID=%d, ExternalID=%s, ConnectionID=%d", entity.ID, entity.ExternalID, entity.ConnectionID)

	// Get OAuth token
	token, err := s.oauthService.GetOAuth2Token(ctx, connectionID)
	if err != nil {
		log.Printf("[CacheService.fetchBodyFromProvider] ERROR: failed to get oauth token for connection %d: %v", connectionID, err)
		return nil, fmt.Errorf("failed to get oauth token: %w", err)
	}
	log.Printf("[CacheService.fetchBodyFromProvider] Got OAuth token, expiry=%v", token.Expiry)

	// Fetch from provider
	log.Printf("[CacheService.fetchBodyFromProvider] Calling provider.GetMessageBody for externalID=%s", entity.ExternalID)
	providerBody, err := s.provider.GetMessageBody(ctx, token, entity.ExternalID)
	if err != nil {
		log.Printf("[CacheService.fetchBodyFromProvider] ERROR: provider.GetMessageBody failed: %v", err)
		return nil, fmt.Errorf("failed to fetch from provider: %w", err)
	}
	log.Printf("[CacheService.fetchBodyFromProvider] Provider returned: text=%d bytes, html=%d bytes, attachments=%d",
		len(providerBody.Text), len(providerBody.HTML), len(providerBody.Attachments))

	// 첨부파일 메타데이터는 DB에 저장하지 않음 (URL 기반 방식)
	// Provider 응답에서 직접 반환하고, 다운로드는 Gmail API 직접 호출

	// 첨부파일을 body 응답에 포함 (DB 저장 완료 대기 없이 즉시 반환)
	var attachments []*domain.Attachment
	for _, att := range providerBody.Attachments {
		attachments = append(attachments, &domain.Attachment{
			EmailID:    emailID,
			ExternalID: att.ID,
			Filename:   att.Filename,
			MimeType:   att.MimeType,
			Size:       att.Size,
			ContentID:  att.ContentID,
			IsInline:   att.IsInline,
		})
	}

	// If body is actually empty, use special marker to distinguish from "not fetched"
	textBody := providerBody.Text
	htmlBody := providerBody.HTML
	if textBody == "" && htmlBody == "" {
		textBody = EmptyBodyMarker
		log.Printf("[CacheService] Email %d has no body content, marking with EmptyBodyMarker", emailID)
	}

	return &domain.EmailBody{
		EmailID:     emailID,
		TextBody:    textBody,
		HTMLBody:    htmlBody,
		Attachments: attachments,
	}, nil
}

// saveAttachmentsAsync is removed - URL 기반 방식으로 변경
// 첨부파일 메타데이터는 DB에 저장하지 않고 Provider에서 직접 가져옴

// =============================================================================
// List Cache
// =============================================================================

// CachedEmailList represents cached email list
type CachedEmailList struct {
	Emails   []*domain.Email `json:"emails"`
	Total    int             `json:"total"`
	CachedAt time.Time       `json:"cached_at"`
}

// GetList retrieves email list with caching
func (s *CacheService) GetList(ctx context.Context, userID string, folder string, page int, limit int) (*CachedEmailList, error) {
	// Try Redis first
	cached, err := s.getListFromRedis(ctx, userID, folder, page)
	if err == nil && cached != nil {
		return cached, nil
	}

	// Cache miss - return nil, caller should query DB
	return nil, nil
}

// CacheList stores email list in Redis
func (s *CacheService) CacheList(ctx context.Context, userID string, folder string, page int, emails []*domain.Email, total int) error {
	if s.redis == nil {
		return nil
	}

	cached := &CachedEmailList{
		Emails:   emails,
		Total:    total,
		CachedAt: time.Now(),
	}

	data, err := json.Marshal(cached)
	if err != nil {
		return err
	}

	key := listKey(userID, folder, page)
	return s.redis.Set(ctx, key, data, s.config.ListTTL).Err()
}

// InvalidateList invalidates list cache for a user/folder
// Uses SCAN instead of KEYS to avoid blocking Redis on large keyspaces
func (s *CacheService) InvalidateList(ctx context.Context, userID string, folder string) error {
	if s.redis == nil {
		return nil
	}

	// Delete all pages for this folder using SCAN (non-blocking)
	pattern := fmt.Sprintf("%s%s:%s:*", keyPrefixList, userID, folder)

	var cursor uint64
	var keysToDelete []string

	for {
		var keys []string
		var err error
		keys, cursor, err = s.redis.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}

		keysToDelete = append(keysToDelete, keys...)

		if cursor == 0 {
			break
		}
	}

	if len(keysToDelete) > 0 {
		// Delete in batches to avoid huge DEL commands
		const batchSize = 100
		for i := 0; i < len(keysToDelete); i += batchSize {
			end := i + batchSize
			if end > len(keysToDelete) {
				end = len(keysToDelete)
			}
			if err := s.redis.Del(ctx, keysToDelete[i:end]...).Err(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *CacheService) getListFromRedis(ctx context.Context, userID string, folder string, page int) (*CachedEmailList, error) {
	if s.redis == nil {
		return nil, nil
	}

	key := listKey(userID, folder, page)
	data, err := s.redis.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var cached CachedEmailList
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, err
	}

	return &cached, nil
}

// =============================================================================
// AI Result Cache
// =============================================================================

// CachedAIResult represents cached AI classification result
type CachedAIResult struct {
	Category string    `json:"category,omitempty"`
	Priority float64   `json:"priority,omitempty"`
	Summary  string    `json:"summary,omitempty"`
	Tags     []string  `json:"tags,omitempty"`
	Intent   string    `json:"intent,omitempty"`
	Score    float64   `json:"score,omitempty"`
	CachedAt time.Time `json:"cached_at"`
}

// GetAIResult retrieves AI result from cache
func (s *CacheService) GetAIResult(ctx context.Context, emailID int64) (*CachedAIResult, error) {
	if s.redis == nil {
		return nil, nil
	}

	key := aiKey(emailID)
	data, err := s.redis.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var cached CachedAIResult
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, err
	}

	return &cached, nil
}

// CacheAIResult stores AI result in Redis
func (s *CacheService) CacheAIResult(ctx context.Context, emailID int64, result *CachedAIResult) error {
	if s.redis == nil {
		return nil
	}

	result.CachedAt = time.Now()
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	key := aiKey(emailID)
	return s.redis.Set(ctx, key, data, s.config.AIResultTTL).Err()
}

// =============================================================================
// Prefetch
// =============================================================================

// PrefetchBodies prefetches email bodies in background
func (s *CacheService) PrefetchBodies(ctx context.Context, emailIDs []int64, connectionID int64) {
	go func() {
		bgCtx := context.Background()
		for _, emailID := range emailIDs {
			// Check if already cached
			if cached, _ := s.getBodyFromRedis(bgCtx, emailID); cached != nil {
				continue
			}

			// Warm the cache
			if _, err := s.GetBody(bgCtx, emailID, connectionID); err != nil {
				log.Printf("[CacheService.PrefetchBodies] Failed to prefetch email %d: %v", emailID, err)
			}
		}
	}()
}

// PrefetchNextPage prefetches next page of email list
// 최적화: 실제 DB 조회 + 캐시 저장 구현
func (s *CacheService) PrefetchNextPage(ctx context.Context, userID string, folder string, currentPage int) {
	if s.emailFetcher == nil {
		return // emailFetcher not configured
	}

	go func() {
		bgCtx := context.Background()
		nextPage := currentPage + 1
		limit := 20 // default page size

		// Check if already cached
		if cached, _ := s.getListFromRedis(bgCtx, userID, folder, nextPage); cached != nil {
			return
		}

		// Parse userID
		uid, err := uuid.Parse(userID)
		if err != nil {
			log.Printf("[CacheService.PrefetchNextPage] Invalid userID: %s", userID)
			return
		}

		// Build filter
		f := domain.LegacyFolder(folder)
		filter := &domain.EmailFilter{
			UserID: uid,
			Folder: &f,
			Limit:  limit,
			Offset: nextPage * limit,
		}

		// Query DB
		emails, total, err := s.emailFetcher.ListEmails(bgCtx, filter)
		if err != nil {
			log.Printf("[CacheService.PrefetchNextPage] Failed to fetch: %v", err)
			return
		}

		// Cache the result
		if len(emails) > 0 {
			s.CacheList(bgCtx, userID, folder, nextPage, emails, total)
			log.Printf("[CacheService.PrefetchNextPage] Cached page %d for user %s folder %s (%d emails)", nextPage, userID, folder, len(emails))
		}
	}()
}

// PrefetchEmailBodiesBatch prefetches multiple email bodies concurrently
// 최적화: 병렬 처리로 여러 본문 동시 로드
func (s *CacheService) PrefetchEmailBodiesBatch(ctx context.Context, emailIDs []int64, connectionID int64, concurrency int) {
	if len(emailIDs) == 0 {
		return
	}
	if concurrency <= 0 {
		concurrency = 5
	}

	go func() {
		bgCtx := context.Background()
		sem := make(chan struct{}, concurrency)

		for _, emailID := range emailIDs {
			sem <- struct{}{} // acquire
			go func(id int64) {
				defer func() { <-sem }() // release

				// Check if already cached
				if cached, _ := s.getBodyFromRedis(bgCtx, id); cached != nil {
					return
				}

				// Warm the cache
				if _, err := s.GetBody(bgCtx, id, connectionID); err != nil {
					log.Printf("[CacheService.PrefetchEmailBodiesBatch] Failed to prefetch email %d: %v", id, err)
				}
			}(emailID)
		}
	}()
}

// GetMetrics returns cache metrics
func (s *CacheService) GetMetrics() *CacheMetrics {
	return s.metrics
}

// GetHitRate returns cache hit rate
func (s *CacheService) GetHitRate() float64 {
	total := s.metrics.RedisHits + s.metrics.RedisMisses
	if total == 0 {
		return 0
	}
	return float64(s.metrics.RedisHits) / float64(total)
}

// =============================================================================
// Compression Helpers
// =============================================================================

func compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompress(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		// Data might not be compressed
		return data, nil
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// Special marker for emails with no body content
// This distinguishes "not fetched yet" from "actually empty"
const EmptyBodyMarker = "\x00__EMPTY_BODY__\x00"

// hasBodyContent checks if the email body has actual content
// Returns false if both text and html are empty or marked as empty
func (s *CacheService) hasBodyContent(body *domain.EmailBody) bool {
	if body == nil {
		return false
	}
	// Check for empty body marker
	if body.TextBody == EmptyBodyMarker || body.HTMLBody == EmptyBodyMarker {
		return true // Has content (the marker), so don't refetch
	}
	return body.TextBody != "" || body.HTMLBody != ""
}

// isEmptyBodyMarker checks if the body is marked as actually empty
func (s *CacheService) isEmptyBodyMarker(body *domain.EmailBody) bool {
	if body == nil {
		return false
	}
	return body.TextBody == EmptyBodyMarker || body.HTMLBody == EmptyBodyMarker
}

// cleanEmptyBodyMarker removes the empty body marker before returning to caller
func cleanEmptyBodyMarker(body *domain.EmailBody) *domain.EmailBody {
	if body == nil {
		return nil
	}
	if body.TextBody == EmptyBodyMarker {
		body.TextBody = ""
	}
	if body.HTMLBody == EmptyBodyMarker {
		body.HTMLBody = ""
	}
	return body
}
