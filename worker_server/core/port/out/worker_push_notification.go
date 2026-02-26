package out

import (
	"context"
	"time"

	"golang.org/x/oauth2"
)

// PushNotificationPort - Gmail/Outlook Push Notification 관리
type PushNotificationPort interface {
	// Gmail watch 등록 (7일 유효)
	WatchMailbox(ctx context.Context, token *oauth2.Token, labelIDs []string) (*WatchResponse, error)

	// Watch 중지
	StopWatch(ctx context.Context, token *oauth2.Token) error

	// Watch 갱신
	RenewWatch(ctx context.Context, token *oauth2.Token, labelIDs []string) (*WatchResponse, error)
}

// WatchResponse - Watch 등록 결과
type WatchResponse struct {
	HistoryID  uint64    `json:"historyId"`
	Expiration time.Time `json:"expiration"`
	ResourceID string    `json:"resourceId,omitempty"`
}
