# MongoDB Adapters

> 이메일 본문(HTML/Text) 저장을 위한 MongoDB 어댑터

## 역할

MongoDB는 **대용량 본문 데이터**만 저장합니다:

| 어댑터 | 컬렉션 | 용도 |
|--------|--------|------|
| `MailBodyAdapter` | mail_bodies | 이메일 본문 (HTML/Text), 첨부파일 메타데이터 |

**메타데이터는 PostgreSQL**, **벡터는 pgvector**, **분석은 Neo4j**에 저장

## 파일 구조

```
mongodb/
├── client.go           # MongoDB 클라이언트 연결
└── mail_body_adapter.go # 본문 저장/조회/압축
```

## 최적화 기능

### 1. Gzip 압축

```go
const compressionThreshold = 1024 // 1KB 이상만 압축

// 저장 시 자동 압축
if originalSize > compressionThreshold {
    compressedHTML = gzip.Compress(htmlBytes)
    compressedText = gzip.Compress(textBytes)
    isCompressed = true
}

// 조회 시 자동 해제
if doc.IsCompressed {
    htmlBytes = gzip.Decompress(doc.HTML)
    textBytes = gzip.Decompress(doc.Text)
}
```

### 2. TTL 자동 만료

```go
// TTL 인덱스 - 자동 삭제
{
    Keys:    bson.D{{Key: "expires_at", Value: 1}},
    Options: options.Index().SetExpireAfterSeconds(0),
}

// 본문 저장 시 TTL 설정
doc := &mailBodyDocument{
    ExpiresAt: time.Now().AddDate(0, 0, ttlDays),
    TTLDays:   30,  // 기본 30일
}
```

### 3. 배치 처리

```go
// 개별 저장 대신 BulkWrite 사용
func BulkSaveBody(ctx, bodies []*MailBodyEntity) error {
    models := make([]mongo.WriteModel, len(bodies))
    for i, body := range bodies {
        models[i] = mongo.NewReplaceOneModel().
            SetFilter(bson.M{"email_id": body.EmailID}).
            SetReplacement(doc).
            SetUpsert(true)
    }
    collection.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
}
```

## Document 스키마

```go
type mailBodyDocument struct {
    EmailID      int64  `bson:"email_id"`       // PostgreSQL emails.id
    ConnectionID int64  `bson:"connection_id"`
    ExternalID   string `bson:"external_id"`    // Provider 메시지 ID

    // 본문 (압축 가능)
    HTML         []byte `bson:"html"`
    Text         []byte `bson:"text"`
    IsCompressed bool   `bson:"is_compressed"`

    // 첨부파일 메타데이터 (실제 파일은 Storage에)
    Attachments []attachmentDocument `bson:"attachments,omitempty"`

    // 사이즈 정보
    OriginalSize   int64 `bson:"original_size"`
    CompressedSize int64 `bson:"compressed_size"`

    // TTL
    CachedAt  time.Time `bson:"cached_at"`
    ExpiresAt time.Time `bson:"expires_at"`
    TTLDays   int       `bson:"ttl_days"`
}
```

## 인덱스

```javascript
// email_id로 빠른 조회 (Unique)
{ "email_id": 1 }  // unique: true

// connection 삭제 시 일괄 삭제
{ "connection_id": 1 }

// TTL 자동 삭제
{ "expires_at": 1 }  // expireAfterSeconds: 0

// 캐시 시간 조회
{ "cached_at": 1 }
```

## 인터페이스: `out.MailBodyRepository`

```go
// 단일 작업
SaveBody(ctx, *MailBodyEntity) error
GetBody(ctx, emailID int64) (*MailBodyEntity, error)
DeleteBody(ctx, emailID int64) error
ExistsBody(ctx, emailID int64) (bool, error)
IsCached(ctx, emailID int64) (bool, error)

// 배치 작업
BulkSaveBody(ctx, []*MailBodyEntity) error
BulkGetBody(ctx, emailIDs []int64) (map[int64]*MailBodyEntity, error)
BulkDeleteBody(ctx, emailIDs []int64) error

// 정리
DeleteExpired(ctx) (int64, error)
DeleteByConnectionID(ctx, connectionID int64) (int64, error)
DeleteOlderThan(ctx, before time.Time) (int64, error)

// 통계
GetStorageStats(ctx) (*BodyStorageStats, error)
GetCompressionStats(ctx) (*CompressionStats, error)
```

## 데이터 흐름

```
메일 동기화
    │
    ├─→ MailAdapter.BulkUpsert()     → PostgreSQL (메타데이터)
    │
    └─→ MailBodyAdapter.BulkSaveBody() → MongoDB (본문)
            │
            ├─→ 1KB 이상? → Gzip 압축
            ├─→ TTL 설정 (기본 30일)
            └─→ Upsert (email_id 기준)

메일 조회
    │
    ├─→ MailAdapter.GetByID()   → PostgreSQL (메타/AI결과)
    │
    └─→ MailBodyAdapter.GetBody() → MongoDB (본문)
            │
            └─→ 압축된 경우 → Gzip 해제
```

## 클라이언트 설정

```go
clientOpts := options.Client().
    ApplyURI(url).
    SetMaxPoolSize(100).      // 최대 연결 100개
    SetMinPoolSize(10).       // 최소 연결 10개
    SetMaxConnIdleTime(30 * time.Second)
```

## 주의사항

1. **본문만 저장**: 메타데이터(subject, from, to 등)는 PostgreSQL에
2. **압축 임계값**: 1KB 미만은 압축하지 않음 (오히려 커질 수 있음)
3. **TTL 필수**: 저장 시 반드시 `expires_at` 설정
4. **배치 사용**: 동기화 시 `BulkSaveBody` 사용으로 성능 최적화
5. **email_id 필수**: PostgreSQL의 emails.id와 1:1 매핑
