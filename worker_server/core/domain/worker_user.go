package domain

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID         uuid.UUID  `json:"id"`
	Email      string     `json:"email"`
	Name       *string    `json:"name,omitempty"`
	AvatarURL  *string    `json:"avatar_url,omitempty"`
	Country    *string    `json:"country,omitempty"`    // 국적 (KR, US, JP 등)
	Occupation *string    `json:"occupation,omitempty"` // 직업
	Company    *string    `json:"company,omitempty"`    // 회사명
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
}

type UserRepository interface {
	GetByID(id uuid.UUID) (*User, error)
	GetByEmail(email string) (*User, error)
	Create(user *User) error
	Update(user *User) error
	Delete(id uuid.UUID) error
}
