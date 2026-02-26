# Worker Pool - 코드 가이드

## 개요

Redis Stream 기반 백그라운드 작업 처리 시스템.
지능형 스케일링, Rate Limiting, Backpressure 처리.

---

## 1. Redis Stream 구조

### Stream 목록

```
mail:sync           # 메일 동기화
mail:sync:priority  # 긴급 동기화
ai:classify         # AI 분류
ai:classify:batch   # 배치 분류
rag:index           # RAG 인덱싱
dlq:mail            # Dead Letter Queue
```

### Consumer Group

```bash
# Consumer Group 생성
XGROUP CREATE mail:sync mail-workers $ MKSTREAM

# 메시지 발행
XADD mail:sync * type mail.sync.delta connection_id 3 history_id 12345

# 메시지 소비
XREADGROUP GROUP mail-workers worker-1 COUNT 10 BLOCK 5000 STREAMS mail:sync >

# ACK
XACK mail:sync mail-workers 1234567890-0

# Pending 조회
XPENDING mail:sync mail-workers
```

---

## 2. 도메인 모델

### core/domain/worker.go

```go
package domain

type Priority int

const (
    PriorityLow    Priority = 1
    PriorityNormal Priority = 5
    PriorityHigh   Priority = 10
    PriorityUrgent Priority = 20
)

type WorkerMetrics struct {
    QueueLengths      map[string]int64  // stream -> pending count
    ProcessingRate    float64           // jobs/sec
    ErrorRate         float64
    AvgLatency        time.Duration
    WorkerUtilization float64           // 0.0 ~ 1.0
    ActiveWorkers     int
}
```

---

## 3. Port 인터페이스

### core/port/out/message_queue.go

```go
package out

type MessageQueuePort interface {
    // 메시지 발행
    Publish(ctx context.Context, stream string, job interface{}) error
    
    // 우선순위 발행
    PublishWithPriority(ctx context.Context, stream string, job interface{}, priority Priority) error
    
    // Consumer Group 생성
    CreateConsumerGroup(ctx context.Context, stream, group string) error
    
    // 메시지 소비 (blocking)
    Consume(ctx context.Context, stream, group, consumer string, count int) ([]Message, error)
    
    // ACK
    Ack(ctx context.Context, stream, group string, ids ...string) error
    
    // Pending 메시지 조회
    GetPending(ctx context.Context, stream, group string) ([]PendingMessage, error)
    
    // Pending 메시지 재처리 (claim)
    ClaimPending(ctx context.Context, stream, group, consumer string, minIdle time.Duration, count int) ([]Message, error)
    
    // DLQ로 이동
    MoveToDLQ(ctx context.Context, originalStream string, msg Message, reason string) error
    
    // 큐 길이 조회
    GetQueueLength(ctx context.Context, stream string) (int64, error)
}

type Message struct {
    ID      string
    Stream  string
    Payload map[string]interface{}
}

type PendingMessage struct {
    ID          string
    Consumer    string
    IdleTime    time.Duration
    DeliveryCount int
}
```

---

## 4. Worker Pool 구현

### adapter/in/worker/pool.go

```go
package worker

type WorkerPool struct {
    minWorkers     int
    maxWorkers     int
    currentWorkers int
    
    messageQueue   out.MessageQueuePort
    processors     map[domain.JobType]Processor
    
    metrics        *Metrics
    scaler         *AutoScaler
    rateLimiter    *RateLimiter
    
    ctx            context.Context
    cancel         context.CancelFunc
    wg             sync.WaitGroup
    mu             sync.RWMutex
}

type Processor interface {
    Process(ctx context.Context, job *domain.SyncJob) error
    JobType() domain.JobType
}

func NewWorkerPool(cfg *Config, mq out.MessageQueuePort) *WorkerPool {
    ctx, cancel := context.WithCancel(context.Background())
    
    pool := &WorkerPool{
        minWorkers:   cfg.MinWorkers,
        maxWorkers:   cfg.MaxWorkers,
        messageQueue: mq,
        processors:   make(map[domain.JobType]Processor),
        metrics:      NewMetrics(),
        ctx:          ctx,
        cancel:       cancel,
    }
    
    pool.scaler = NewAutoScaler(pool)
    pool.rateLimiter = NewRateLimiter(cfg.RateLimit)
    
    return pool
}

func (p *WorkerPool) Start() {
    // 최소 워커 수만큼 시작
    for i := 0; i < p.minWorkers; i++ {
        p.spawnWorker()
    }
    
    // 스케일링 고루틴
    go p.scaler.Run(p.ctx)
    
    // Pending 메시지 복구 고루틴
    go p.recoverPending(p.ctx)
    
    // 메트릭 수집 고루틴
    go p.collectMetrics(p.ctx)
}

func (p *WorkerPool) spawnWorker() {
    p.mu.Lock()
    if p.currentWorkers >= p.maxWorkers {
        p.mu.Unlock()
        return
    }
    p.currentWorkers++
    workerID := p.currentWorkers
    p.mu.Unlock()
    
    p.wg.Add(1)
    go func() {
        defer p.wg.Done()
        p.workerLoop(workerID)
    }()
}

func (p *WorkerPool) workerLoop(id int) {
    consumerName := fmt.Sprintf("worker-%d", id)
    streams := []string{"mail:sync:priority", "mail:sync", "ai:classify"}
    
    for {
        select {
        case <-p.ctx.Done():
            return
        default:
        }
        
        // 우선순위 순서대로 폴링
        for _, stream := range streams {
            messages, err := p.messageQueue.Consume(p.ctx, stream, "mail-workers", consumerName, 1)
            if err != nil || len(messages) == 0 {
                continue
            }
            
            for _, msg := range messages {
                p.processMessage(stream, msg)
            }
        }
    }
}

func (p *WorkerPool) processMessage(stream string, msg Message) {
    job := parseJob(msg)
    
    // Rate Limit 체크
    if !p.rateLimiter.Allow(job.UserID) {
        // 나중에 재처리
        time.Sleep(100 * time.Millisecond)
        return
    }
    
    processor := p.processors[job.Type]
    if processor == nil {
        log.Printf("Unknown job type: %s", job.Type)
        p.messageQueue.Ack(p.ctx, stream, "mail-workers", msg.ID)
        return
    }
    
    start := time.Now()
    err := processor.Process(p.ctx, job)
    duration := time.Since(start)
    
    if err != nil {
        p.handleError(stream, msg, job, err)
    } else {
        p.messageQueue.Ack(p.ctx, stream, "mail-workers", msg.ID)
        p.metrics.RecordSuccess(duration)
    }
}

func (p *WorkerPool) handleError(stream string, msg Message, job *domain.SyncJob, err error) {
    job.RetryCount++
    
    if job.RetryCount >= 3 {
        // DLQ로 이동
        p.messageQueue.MoveToDLQ(p.ctx, stream, msg, err.Error())
        p.messageQueue.Ack(p.ctx, stream, "mail-workers", msg.ID)
        p.metrics.RecordDLQ()
    } else {
        // 재시도 (ACK 안 함 → 자동 재처리)
        p.metrics.RecordRetry()
    }
    
    p.metrics.RecordError()
}

func (p *WorkerPool) recoverPending(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // 5분 이상 처리 안 된 메시지 claim
            streams := []string{"mail:sync", "ai:classify"}
            for _, stream := range streams {
                messages, _ := p.messageQueue.ClaimPending(ctx, stream, "mail-workers", "recovery", 5*time.Minute, 10)
                for _, msg := range messages {
                    p.processMessage(stream, msg)
                }
            }
        }
    }
}
```

---

## 5. Auto Scaler

### adapter/in/worker/scaler.go

```go
package worker

type AutoScaler struct {
    pool *WorkerPool
    
    scaleUpThreshold   float64 // Queue/Workers > threshold → scale up
    scaleDownThreshold float64 // Utilization < threshold → scale down
    scaleUpCooldown    time.Duration
    scaleDownCooldown  time.Duration
    
    lastScaleUp   time.Time
    lastScaleDown time.Time
}

func (s *AutoScaler) Run(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.evaluate()
        }
    }
}

func (s *AutoScaler) evaluate() {
    metrics := s.pool.metrics.Get()
    
    totalQueue := int64(0)
    for _, length := range metrics.QueueLengths {
        totalQueue += length
    }
    
    queuePressure := float64(totalQueue) / float64(s.pool.currentWorkers)
    
    // Scale Up
    if queuePressure > s.scaleUpThreshold &&
       time.Since(s.lastScaleUp) > s.scaleUpCooldown &&
       s.pool.currentWorkers < s.pool.maxWorkers {
        
        s.pool.spawnWorker()
        s.lastScaleUp = time.Now()
        log.Printf("Scaled up to %d workers (queue pressure: %.2f)", s.pool.currentWorkers, queuePressure)
    }
    
    // Scale Down
    if metrics.WorkerUtilization < s.scaleDownThreshold &&
       time.Since(s.lastScaleDown) > s.scaleDownCooldown &&
       s.pool.currentWorkers > s.pool.minWorkers {
        
        s.pool.killWorker()
        s.lastScaleDown = time.Now()
        log.Printf("Scaled down to %d workers (utilization: %.2f)", s.pool.currentWorkers, metrics.WorkerUtilization)
    }
}
```

---

## 6. Rate Limiter

### adapter/in/worker/ratelimiter.go

```go
package worker

// Gmail API: 250 quota units/user/second
// messages.get = 5 units, history.list = 2 units

type RateLimiter struct {
    userBuckets map[string]*TokenBucket
    globalLimit *TokenBucket
    mu          sync.RWMutex
}

type TokenBucket struct {
    capacity   int64
    tokens     int64
    refillRate int64 // per second
    lastRefill time.Time
}

func (r *RateLimiter) Allow(userID string) bool {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    bucket := r.getOrCreateBucket(userID)
    bucket.refill()
    
    if bucket.tokens >= 5 { // messages.get cost
        bucket.tokens -= 5
        return true
    }
    return false
}

func (b *TokenBucket) refill() {
    now := time.Now()
    elapsed := now.Sub(b.lastRefill).Seconds()
    
    tokensToAdd := int64(elapsed * float64(b.refillRate))
    b.tokens = min(b.capacity, b.tokens + tokensToAdd)
    b.lastRefill = now
}
```

---

## 7. 환경변수

```env
# Worker Pool
WORKER_MIN=4
WORKER_MAX=32

# Auto Scaler
WORKER_SCALE_UP_THRESHOLD=10.0    # queue/workers
WORKER_SCALE_DOWN_THRESHOLD=0.3   # utilization
WORKER_SCALE_UP_COOLDOWN=30s
WORKER_SCALE_DOWN_COOLDOWN=60s

# Rate Limiting
GMAIL_QUOTA_PER_USER=250

# Redis Stream
REDIS_CONSUMER_GROUP=mail-workers
REDIS_PENDING_TIMEOUT=5m
REDIS_DLQ_MAX_RETRIES=3
```
