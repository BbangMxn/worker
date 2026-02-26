// Package persistence provides database adapters implementing outbound ports.
package persistence

import (
	"context"
	"fmt"
	"time"

	"worker_server/core/port/out"
	"worker_server/pkg/cache"

	"github.com/google/uuid"
)

// CachedContactAdapter wraps ContactAdapter with Redis caching.
type CachedContactAdapter struct {
	delegate *ContactAdapter
	cache    *cache.RedisCache
	ttl      time.Duration
}

// NewCachedContactAdapter creates a new cached contact adapter.
func NewCachedContactAdapter(delegate *ContactAdapter, redisCache *cache.RedisCache) *CachedContactAdapter {
	return &CachedContactAdapter{
		delegate: delegate,
		cache:    redisCache,
		ttl:      30 * time.Minute, // 연락처는 자주 변경되지 않음
	}
}

// contactCacheKey generates cache key for contact email lookup.
func contactCacheKey(userID uuid.UUID, email string) string {
	return fmt.Sprintf("contact:%s:%s", userID.String(), email)
}

// =============================================================================
// MailContactRepository Implementation (with caching)
// =============================================================================

// GetContactByEmail gets contact info by email with caching.
func (a *CachedContactAdapter) GetContactByEmail(ctx context.Context, userID uuid.UUID, email string) (*out.MailContactInfo, error) {
	key := contactCacheKey(userID, email)

	// 캐시 확인
	var info out.MailContactInfo
	found, err := a.cache.GetJSON(ctx, key, &info)
	if err == nil && found {
		return &info, nil
	}

	// DB 조회
	result, err := a.delegate.GetContactByEmail(ctx, userID, email)
	if err != nil {
		return nil, err
	}

	// 캐시 저장 (nil도 캐싱 - negative caching)
	if result != nil {
		_ = a.cache.SetJSON(ctx, key, result, a.ttl)
	} else {
		// nil 결과도 짧게 캐싱 (존재하지 않는 연락처 반복 조회 방지)
		_ = a.cache.SetJSON(ctx, key, &out.MailContactInfo{}, 5*time.Minute)
	}

	return result, nil
}

// BulkGetContactsByEmail gets multiple contacts with caching.
func (a *CachedContactAdapter) BulkGetContactsByEmail(ctx context.Context, userID uuid.UUID, emails []string) (map[string]*out.MailContactInfo, error) {
	if len(emails) == 0 {
		return make(map[string]*out.MailContactInfo), nil
	}

	result := make(map[string]*out.MailContactInfo, len(emails))
	var missingEmails []string

	// 캐시에서 먼저 조회
	for _, email := range emails {
		key := contactCacheKey(userID, email)
		var info out.MailContactInfo
		found, err := a.cache.GetJSON(ctx, key, &info)
		if err == nil && found {
			// ContactID가 0이면 negative cache (존재하지 않는 연락처)
			if info.ContactID != 0 {
				result[email] = &info
			}
		} else {
			missingEmails = append(missingEmails, email)
		}
	}

	// 캐시 미스된 이메일들은 DB에서 조회
	if len(missingEmails) > 0 {
		dbResult, err := a.delegate.BulkGetContactsByEmail(ctx, userID, missingEmails)
		if err != nil {
			return nil, err
		}

		// 결과 병합 및 캐시 저장
		for _, email := range missingEmails {
			key := contactCacheKey(userID, email)
			if info, ok := dbResult[email]; ok {
				result[email] = info
				_ = a.cache.SetJSON(ctx, key, info, a.ttl)
			} else {
				// Negative caching
				_ = a.cache.SetJSON(ctx, key, &out.MailContactInfo{}, 5*time.Minute)
			}
		}
	}

	return result, nil
}

// LinkMailToContact delegates to underlying adapter.
func (a *CachedContactAdapter) LinkMailToContact(ctx context.Context, mailID int64, contactID int64) error {
	return a.delegate.LinkMailToContact(ctx, mailID, contactID)
}

// UpdateContactInteraction updates interaction and invalidates cache.
func (a *CachedContactAdapter) UpdateContactInteraction(ctx context.Context, userID uuid.UUID, email string) error {
	// 캐시 무효화
	key := contactCacheKey(userID, email)
	_ = a.cache.Delete(ctx, key)

	return a.delegate.UpdateContactInteraction(ctx, userID, email)
}

// =============================================================================
// ContactRepository Implementation (delegate)
// =============================================================================

func (a *CachedContactAdapter) Create(ctx context.Context, contact *out.ContactEntity) error {
	return a.delegate.Create(ctx, contact)
}

func (a *CachedContactAdapter) Update(ctx context.Context, contact *out.ContactEntity) error {
	// 업데이트 시 캐시 무효화
	if contact.Email != "" {
		key := contactCacheKey(contact.UserID, contact.Email)
		_ = a.cache.Delete(ctx, key)
	}
	return a.delegate.Update(ctx, contact)
}

func (a *CachedContactAdapter) Delete(ctx context.Context, userID uuid.UUID, id int64) error {
	// 삭제 전 이메일 조회해서 캐시 무효화
	entity, err := a.delegate.GetByID(ctx, userID, id)
	if err == nil && entity != nil && entity.Email != "" {
		key := contactCacheKey(userID, entity.Email)
		_ = a.cache.Delete(ctx, key)
	}
	return a.delegate.Delete(ctx, userID, id)
}

func (a *CachedContactAdapter) GetByID(ctx context.Context, userID uuid.UUID, id int64) (*out.ContactEntity, error) {
	return a.delegate.GetByID(ctx, userID, id)
}

func (a *CachedContactAdapter) GetByEmail(ctx context.Context, userID uuid.UUID, email string) (*out.ContactEntity, error) {
	return a.delegate.GetByEmail(ctx, userID, email)
}

func (a *CachedContactAdapter) List(ctx context.Context, userID uuid.UUID, query *out.ContactListQuery) ([]*out.ContactEntity, int, error) {
	return a.delegate.List(ctx, userID, query)
}

func (a *CachedContactAdapter) UpdateInteraction(ctx context.Context, userID uuid.UUID, contactID int64) error {
	return a.delegate.UpdateInteraction(ctx, userID, contactID)
}

func (a *CachedContactAdapter) UpdateRelationshipScore(ctx context.Context, userID uuid.UUID, contactID int64, score int16) error {
	return a.delegate.UpdateRelationshipScore(ctx, userID, contactID, score)
}

func (a *CachedContactAdapter) Upsert(ctx context.Context, contact *out.ContactEntity) error {
	// Upsert 시 캐시 무효화
	if contact.Email != "" {
		key := contactCacheKey(contact.UserID, contact.Email)
		_ = a.cache.Delete(ctx, key)
	}
	return a.delegate.Upsert(ctx, contact)
}

// Ensure CachedContactAdapter implements both interfaces
var _ out.ContactRepository = (*CachedContactAdapter)(nil)
var _ out.MailContactRepository = (*CachedContactAdapter)(nil)
