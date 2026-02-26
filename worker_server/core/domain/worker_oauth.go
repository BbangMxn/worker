package domain

import (
	"time"

	"github.com/google/uuid"
)

type OAuthProvider string

const (
	ProviderGoogle         OAuthProvider = "google"
	ProviderGmail          OAuthProvider = "google" // alias for backward compatibility
	ProviderOutlook        OAuthProvider = "outlook"
	ProviderGoogleCalendar OAuthProvider = "google_calendar"
)

type OAuthConnection struct {
	ID           int64         `json:"id"`
	UserID       uuid.UUID     `json:"user_id"`
	Provider     OAuthProvider `json:"provider"`
	Email        string        `json:"email"`
	AccessToken  string        `json:"-"`
	RefreshToken string        `json:"-"`
	ExpiresAt    time.Time     `json:"expires_at"`
	IsConnected  bool          `json:"is_connected"`
	IsDefault    bool          `json:"is_default"`          // 기본 발송 계정 여부
	Signature    *string       `json:"signature,omitempty"` // 계정별 이메일 서명
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type OAuthRepository interface {
	GetByID(id int64) (*OAuthConnection, error)
	GetByUserAndProvider(userID uuid.UUID, provider OAuthProvider) (*OAuthConnection, error)
	GetAllByUser(userID uuid.UUID) ([]*OAuthConnection, error)
	Create(conn *OAuthConnection) error
	Update(conn *OAuthConnection) error
	Delete(id int64) error
}
