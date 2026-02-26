# Realtime Push - 코드 가이드

## 개요

SSE(Server-Sent Events)를 통해 프론트엔드에 실시간 이벤트 푸시.

---

## 1. 이벤트 타입

```go
const (
    EventNewEmail        = "email.new"        // 새 메일 도착
    EventEmailClassified = "email.classified" // AI 분류 완료
    EventEmailUpdated    = "email.updated"    // 메일 상태 변경
    EventSyncStarted     = "sync.started"     // 동기화 시작
    EventSyncCompleted   = "sync.completed"   // 동기화 완료
    EventSyncError       = "sync.error"       // 동기화 에러
)
```

---

## 2. 도메인 모델

### core/domain/notification.go

```go
package domain

type RealtimeEvent struct {
    Type      string      `json:"type"`
    UserID    string      `json:"-"`           // 전송 대상
    Data      interface{} `json:"data"`
    Timestamp time.Time   `json:"timestamp"`
}

// 새 메일 이벤트
type NewEmailData struct {
    EmailID   int64  `json:"email_id"`
    Subject   string `json:"subject"`
    From      string `json:"from"`
    Snippet   string `json:"snippet"`
    Folder    string `json:"folder"`
}

// 분류 완료 이벤트
type ClassifiedData struct {
    EmailID   int64  `json:"email_id"`
    Category  string `json:"category"`
    Priority  string `json:"priority"`
    Summary   string `json:"summary,omitempty"`
}
```

---

## 3. Port 인터페이스

### core/port/out/realtime.go

```go
package out

type RealtimePort interface {
    // 사용자 채널 구독
    Subscribe(userID string) <-chan *domain.RealtimeEvent
    
    // 구독 해제
    Unsubscribe(userID string, ch <-chan *domain.RealtimeEvent)
    
    // 이벤트 발행 (특정 사용자)
    Push(ctx context.Context, userID string, event *domain.RealtimeEvent) error
    
    // 브로드캐스트 (모든 사용자)
    Broadcast(ctx context.Context, event *domain.RealtimeEvent) error
    
    // 연결된 사용자 수
    ConnectedCount() int
}
```

---

## 4. SSE Adapter 구현

### adapter/out/realtime/sse_adapter.go

```go
package realtime

type SSEAdapter struct {
    channels map[string][]chan *domain.RealtimeEvent
    mu       sync.RWMutex
}

func NewSSEAdapter() *SSEAdapter {
    return &SSEAdapter{
        channels: make(map[string][]chan *domain.RealtimeEvent),
    }
}

func (a *SSEAdapter) Subscribe(userID string) <-chan *domain.RealtimeEvent {
    a.mu.Lock()
    defer a.mu.Unlock()
    
    ch := make(chan *domain.RealtimeEvent, 100) // 버퍼
    a.channels[userID] = append(a.channels[userID], ch)
    
    return ch
}

func (a *SSEAdapter) Unsubscribe(userID string, ch <-chan *domain.RealtimeEvent) {
    a.mu.Lock()
    defer a.mu.Unlock()
    
    channels := a.channels[userID]
    for i, c := range channels {
        if c == ch {
            a.channels[userID] = append(channels[:i], channels[i+1:]...)
            close(c)
            break
        }
    }
    
    if len(a.channels[userID]) == 0 {
        delete(a.channels, userID)
    }
}

func (a *SSEAdapter) Push(ctx context.Context, userID string, event *domain.RealtimeEvent) error {
    a.mu.RLock()
    channels := a.channels[userID]
    a.mu.RUnlock()
    
    for _, ch := range channels {
        select {
        case ch <- event:
        default:
            // 버퍼 가득 참 → 드롭 (또는 오래된 메시지 제거)
        }
    }
    
    return nil
}

func (a *SSEAdapter) Broadcast(ctx context.Context, event *domain.RealtimeEvent) error {
    a.mu.RLock()
    defer a.mu.RUnlock()
    
    for _, channels := range a.channels {
        for _, ch := range channels {
            select {
            case ch <- event:
            default:
            }
        }
    }
    
    return nil
}
```

---

## 5. SSE Handler

### adapter/in/http/sse.go

```go
package http

type SSEHandler struct {
    realtime out.RealtimePort
}

// GET /api/v1/events/stream
func (h *SSEHandler) Stream(c *fiber.Ctx) error {
    userID := c.Locals("user_id").(string)
    
    // SSE 헤더 설정
    c.Set("Content-Type", "text/event-stream")
    c.Set("Cache-Control", "no-cache")
    c.Set("Connection", "keep-alive")
    c.Set("X-Accel-Buffering", "no") // nginx 버퍼링 비활성화
    
    // 채널 구독
    ch := h.realtime.Subscribe(userID)
    defer h.realtime.Unsubscribe(userID, ch)
    
    // Keep-alive ticker
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
        for {
            select {
            case event := <-ch:
                data, _ := json.Marshal(event)
                fmt.Fprintf(w, "event: %s\n", event.Type)
                fmt.Fprintf(w, "data: %s\n\n", data)
                w.Flush()
                
            case <-ticker.C:
                // Keep-alive (빈 코멘트)
                fmt.Fprintf(w, ": keepalive\n\n")
                w.Flush()
                
            case <-c.Context().Done():
                return
            }
        }
    })
    
    return nil
}
```

---

## 6. Service에서 이벤트 발행

### core/service/mail_sync.go

```go
func (s *MailSyncService) processNewMessage(ctx context.Context, connectionID int64, messageID string) error {
    // ... 메일 저장 로직 ...
    
    // 실시간 이벤트 발행
    s.realtime.Push(ctx, userID, &domain.RealtimeEvent{
        Type: "email.new",
        Data: &domain.NewEmailData{
            EmailID: email.ID,
            Subject: email.Subject,
            From:    email.FromEmail,
            Snippet: email.Snippet,
            Folder:  string(email.Folder),
        },
        Timestamp: time.Now(),
    })
    
    return nil
}
```

### adapter/in/worker/ai_processor.go

```go
func (p *AIProcessor) Process(ctx context.Context, job *domain.SyncJob) error {
    // ... AI 분류 로직 ...
    
    // 분류 완료 이벤트 발행
    p.realtime.Push(ctx, job.UserID, &domain.RealtimeEvent{
        Type: "email.classified",
        Data: &domain.ClassifiedData{
            EmailID:  emailID,
            Category: classification.Category,
            Priority: classification.Priority,
            Summary:  classification.Summary,
        },
        Timestamp: time.Now(),
    })
    
    return nil
}
```

---

## 7. 프론트엔드 연동

```typescript
// Frontend: shared/api/sse.ts
export function subscribeToEvents(onEvent: (event: RealtimeEvent) => void) {
  const token = api.getToken();
  const eventSource = new EventSource(
    `${API_URL}/api/v1/events/stream`,
    {
      headers: { Authorization: `Bearer ${token}` }
    }
  );
  
  eventSource.addEventListener('email.new', (e) => {
    onEvent({ type: 'email.new', data: JSON.parse(e.data) });
  });
  
  eventSource.addEventListener('email.classified', (e) => {
    onEvent({ type: 'email.classified', data: JSON.parse(e.data) });
  });
  
  eventSource.onerror = () => {
    // 재연결 로직
    setTimeout(() => subscribeToEvents(onEvent), 5000);
  };
  
  return () => eventSource.close();
}
```

---

## 8. 다중 서버 환경

단일 서버에서는 위 구현으로 충분하지만, 다중 서버 환경에서는 Redis Pub/Sub 필요.

```go
// adapter/out/realtime/redis_sse_adapter.go
type RedisSSEAdapter struct {
    local  *SSEAdapter      // 로컬 연결 관리
    redis  *redis.Client    // 서버 간 통신
    pubsub *redis.PubSub
}

func (a *RedisSSEAdapter) Push(ctx context.Context, userID string, event *domain.RealtimeEvent) error {
    // Redis Pub/Sub으로 발행 → 모든 서버에서 수신
    data, _ := json.Marshal(event)
    return a.redis.Publish(ctx, "realtime:"+userID, data).Err()
}

func (a *RedisSSEAdapter) subscribeRedis() {
    for msg := range a.pubsub.Channel() {
        var event domain.RealtimeEvent
        json.Unmarshal([]byte(msg.Payload), &event)
        
        // 로컬 연결에 전달
        a.local.Push(context.Background(), event.UserID, &event)
    }
}
```
