package out

import (
	"context"

	"worker_server/core/domain"
)

// RealtimePort - 실시간 이벤트 푸시
type RealtimePort interface {
	// 사용자 채널 구독
	Subscribe(userID string) <-chan *domain.RealtimeEvent

	// 구독 해제
	Unsubscribe(userID string, ch <-chan *domain.RealtimeEvent)

	// 특정 사용자에게 이벤트 전송
	Push(ctx context.Context, userID string, event *domain.RealtimeEvent) error

	// 모든 사용자에게 브로드캐스트
	Broadcast(ctx context.Context, event *domain.RealtimeEvent) error

	// 연결된 사용자 수
	ConnectedCount() int

	// 특정 사용자 연결 여부
	IsConnected(userID string) bool
}
