package domain

import (
	"math"
	"time"

	"github.com/google/uuid"
)

// SenderProfile represents learned classification for a sender
type SenderProfile struct {
	ID     int64     `json:"id"`
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Domain string    `json:"domain"`

	// Learned classification
	LearnedCategory    *EmailCategory    `json:"learned_category,omitempty"`
	LearnedSubCategory *EmailSubCategory `json:"learned_sub_category,omitempty"`

	// User preferences
	IsVIP   bool `json:"is_vip"`   // Important sender
	IsMuted bool `json:"is_muted"` // Muted sender (still received but deprioritized)

	// Stats for learning
	EmailCount int     `json:"email_count"`
	ReadRate   float64 `json:"read_rate"`   // 0.0 - 1.0
	ReplyRate  float64 `json:"reply_rate"`  // 0.0 - 1.0
	DeleteRate float64 `json:"delete_rate"` // 0.0 - 1.0 (new)

	// Engagement tracking (new)
	IsContact        bool       `json:"is_contact"`         // 연락처에 있는지
	InteractionCount int        `json:"interaction_count"`  // 총 상호작용 수 (읽기+답장+클릭)
	LastInteractedAt *time.Time `json:"last_interacted_at"` // 마지막 상호작용 시간

	// Calculated score (new)
	ImportanceScore float64 `json:"importance_score"` // 0.0 - 1.0 (캐시된 중요도 점수)

	// Labels (new)
	ConfirmedLabels []int64 `json:"confirmed_labels"` // 사용자가 확정한 라벨 ID들

	// Optional display info
	DisplayName *string `json:"display_name,omitempty"`
	AvatarURL   *string `json:"avatar_url,omitempty"`

	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CalculateImportanceScore calculates the importance score based on engagement signals.
//
// Based on research from Gmail Priority Inbox, Superhuman, and Eisenhower Matrix:
// - Google Research: Social features (reply rate, read rate) are strongest predictors
// - SIGIR: Historical interaction count is most important for reply prediction
// - Eisenhower: Urgent vs Important distinction
//
// Formula (0.0 - 1.0 scale):
//
//  1. Engagement Score (40%): Reply rate is strongest signal
//     - ReplyRate * 0.20 (답장률 - 가장 강력한 시그널)
//     - ReadRate * 0.12 (읽음률)
//     - (1-DeleteRate) * 0.08 (삭제하지 않음)
//
//  2. Relationship Score (25%):
//     - IsContact: +0.12 (연락처)
//     - High interaction (>20): +0.08 (자주 소통)
//     - Medium interaction (>5): +0.05
//
//  3. Recency Score (20%):
//     - 7일 내: +0.20
//     - 30일 내: +0.12
//     - 90일 내: +0.06
//
//  4. Activity Score (15%):
//     - Recent interaction (<24h): +0.10
//     - Recent interaction (<72h): +0.05
//     - Frequent sender (>50 emails): +0.05
//
//  5. Special Cases:
//     - VIP: 0.98 고정
//     - Muted: 0.05 고정
func (p *SenderProfile) CalculateImportanceScore() float64 {
	// VIP는 최고 점수 (Superhuman style)
	if p.IsVIP {
		return 0.98
	}

	// Muted는 최저 점수
	if p.IsMuted {
		return 0.05
	}

	score := 0.0

	// === 1. Engagement Score (40%) ===
	// Google Research: "Social features are based on the degree of interaction"
	// Reply rate is the strongest signal (SIGIR research)
	score += p.ReplyRate * 0.20
	score += p.ReadRate * 0.12
	score += (1 - p.DeleteRate) * 0.08

	// === 2. Relationship Score (25%) ===
	// Contact bonus (known person)
	if p.IsContact {
		score += 0.12
	}

	// Interaction frequency bonus (historical interaction is key predictor)
	if p.InteractionCount > 20 {
		score += 0.08 // High engagement
	} else if p.InteractionCount > 5 {
		score += 0.05 // Medium engagement
	} else if p.InteractionCount > 0 {
		score += 0.02 // Some engagement
	}

	// === 3. Recency Score (20%) ===
	// Recent senders are more likely to be important
	if !p.LastSeenAt.IsZero() {
		daysSinceLastEmail := time.Since(p.LastSeenAt).Hours() / 24
		if daysSinceLastEmail < 7 {
			score += 0.20 // Very recent
		} else if daysSinceLastEmail < 30 {
			score += 0.12 // Recent
		} else if daysSinceLastEmail < 90 {
			score += 0.06 // Somewhat recent
		}
		// 90일 이상: 보너스 없음
	}

	// === 4. Activity Score (15%) ===
	// Recent interaction indicates active relationship
	if p.LastInteractedAt != nil && !p.LastInteractedAt.IsZero() {
		hoursSinceInteraction := time.Since(*p.LastInteractedAt).Hours()
		if hoursSinceInteraction < 24 {
			score += 0.10 // Very active
		} else if hoursSinceInteraction < 72 {
			score += 0.05 // Active
		}
	}

	// Frequent sender bonus (established relationship)
	if p.EmailCount > 50 {
		score += 0.05
	} else if p.EmailCount > 20 {
		score += 0.03
	}

	// Cap at 0.95 (VIP gets 0.98)
	return math.Min(score, 0.95)
}

// IsImportant returns true if the sender is considered important.
func (p *SenderProfile) IsImportant() bool {
	return p.IsVIP || p.ImportanceScore >= 0.70 || p.ReplyRate > 0.5
}

// ShouldDeprioritize returns true if the sender should be deprioritized.
func (p *SenderProfile) ShouldDeprioritize() bool {
	return p.IsMuted || p.ImportanceScore < 0.30 || p.DeleteRate > 0.7
}

// SenderProfileRepository interface for sender profile operations
type SenderProfileRepository interface {
	// Basic CRUD
	GetByID(id int64) (*SenderProfile, error)
	GetByEmail(userID uuid.UUID, email string) (*SenderProfile, error)
	GetByDomain(userID uuid.UUID, domain string) ([]*SenderProfile, error)
	GetByUserID(userID uuid.UUID, limit, offset int) ([]*SenderProfile, error)
	Create(profile *SenderProfile) error
	Update(profile *SenderProfile) error
	Delete(id int64) error

	// Bulk operations
	GetVIPSenders(userID uuid.UUID) ([]*SenderProfile, error)
	GetMutedSenders(userID uuid.UUID) ([]*SenderProfile, error)

	// Important senders by score (new)
	GetTopSenders(userID uuid.UUID, minScore float64, limit int) ([]*SenderProfile, error)
	GetContactSenders(userID uuid.UUID) ([]*SenderProfile, error)

	// Stats update
	IncrementEmailCount(id int64) error
	UpdateReadRate(id int64, newRate float64) error
	UpdateReplyRate(id int64, newRate float64) error
	UpdateDeleteRate(id int64, newRate float64) error
	UpdateLastSeen(id int64, lastSeenAt time.Time) error

	// Engagement update (new)
	UpdateIsContact(id int64, isContact bool) error
	IncrementInteractionCount(id int64) error
	UpdateLastInteraction(id int64, at time.Time) error
	AddConfirmedLabel(id int64, labelID int64) error
	RemoveConfirmedLabel(id int64, labelID int64) error
}

// KnownDomain represents a pre-classified domain for auto-classification
type KnownDomain struct {
	ID          int               `json:"id"`
	Domain      string            `json:"domain"`
	Category    EmailCategory     `json:"category"`
	SubCategory *EmailSubCategory `json:"sub_category,omitempty"`
	Confidence  float64           `json:"confidence"` // 0.0 - 1.0
	Source      string            `json:"source"`     // "system" | "user" | "learned"
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// KnownDomainRepository interface for known domain operations
type KnownDomainRepository interface {
	GetByDomain(domain string) (*KnownDomain, error)
	List() ([]*KnownDomain, error)
	Create(domain *KnownDomain) error
	Update(domain *KnownDomain) error
	Delete(id int) error
}
