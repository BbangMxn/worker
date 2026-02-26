# AI Agent 메일 분류 시스템

> 메일 구분 및 AI 분류 전체 메커니즘 문서

---

## 1. 메일 구분 체계

### 1.1 Folder (위치)
메일의 물리적 위치를 나타냄. Gmail Label과 매핑됨.

| Folder | Gmail Label | 설명 |
|--------|-------------|------|
| `inbox` | INBOX | 받은편지함 |
| `sent` | SENT | 보낸편지함 |
| `drafts` | DRAFT | 임시보관함 |
| `trash` | TRASH | 휴지통 |
| `spam` | SPAM | 스팸 |
| `archive` | (INBOX 제거) | 보관됨 |

### 1.2 Labels (태그)
Gmail 라벨과 동기화. 한 메일에 여러 라벨 가능 (N:N 관계).

```
시스템 라벨: INBOX, SENT, IMPORTANT, STARRED, CATEGORY_SOCIAL, CATEGORY_PROMOTIONS...
사용자 라벨: 프로젝트A, 중요, 나중에읽기...
```

### 1.3 AI Category (분류)
AI가 자동으로 분류한 카테고리.

| Category | 설명 | 기본 Priority |
|----------|------|---------------|
| `work` | 업무 | high |
| `personal` | 개인 | normal |
| `finance` | 금융/결제 | high |
| `newsletter` | 뉴스레터 | low |
| `marketing` | 마케팅/광고 | low |
| `notification` | 알림 | normal |
| `social` | SNS | low |
| `travel` | 여행 | normal |
| `shopping` | 쇼핑 | normal |
| `spam` | 스팸 | lowest |
| `other` | 기타 | normal |

### 1.4 SubCategory (세부 분류)

| SubCategory | 설명 |
|-------------|------|
| `receipt` | 영수증 |
| `invoice` | 청구서 |
| `shipping` | 배송 알림 |
| `order` | 주문 확인 |
| `calendar` | 일정 초대 |
| `account` | 계정 알림 |
| `security` | 보안 알림 |
| `sns` | SNS 알림 |

### 1.5 Priority (우선순위)

| Priority | 값 | 설명 |
|----------|---|------|
| `urgent` | 5 | 긴급 |
| `high` | 4 | 높음 |
| `normal` | 3 | 보통 |
| `low` | 2 | 낮음 |
| `lowest` | 1 | 최저 |

### 1.6 Workflow Status (워크플로우)

| Status | 설명 |
|--------|------|
| `none` | 기본 상태 |
| `todo` | 할 일 |
| `done` | 완료 |
| `snoozed` | 스누즈 (나중에 알림) |

---

## 2. DB 스키마

### emails 테이블 주요 컬럼

```sql
-- 기본 정보
id                  BIGSERIAL PRIMARY KEY
external_id         VARCHAR         -- Gmail Message ID
user_id             UUID
connection_id       BIGINT          -- OAuth 연결 ID

-- 위치/조직
folder              VARCHAR         -- inbox, sent, drafts, trash, spam, archive
labels              TEXT[]          -- Gmail 라벨 ID 배열
tags                TEXT[]          -- 내부 태그 (starred, action_required)

-- AI 분류 결과
ai_status           VARCHAR         -- pending, completed, failed
ai_category         VARCHAR         -- work, personal, newsletter...
ai_sub_category     VARCHAR         -- receipt, invoice, shipping...
ai_priority         VARCHAR         -- urgent, high, normal, low, lowest
ai_summary          TEXT            -- AI 생성 요약
ai_sentiment        FLOAT           -- 감정 분석 점수 (-1.0 ~ 1.0)
ai_tags             TEXT[]          -- AI 추출 태그

-- 워크플로우
workflow_status     VARCHAR         -- none, todo, done, snoozed
snooze_until        TIMESTAMP       -- 스누즈 해제 시간

-- 상태 플래그
is_read             BOOLEAN
is_draft            BOOLEAN
has_attachment      BOOLEAN
is_replied          BOOLEAN
is_forwarded        BOOLEAN
```

---

## 3. AI 분류 파이프라인 (4단계)

### 전체 흐름

```
새 메일 도착
    │
    ▼
┌─────────────────────────────────────────┐
│ Stage 0: User Rules (~10%)              │
│ 사용자 정의 규칙 우선 적용              │
└─────────────────────────────────────────┘
    │ (매칭 실패 시)
    ▼
┌─────────────────────────────────────────┐
│ Stage 1: Headers (~35%)                 │
│ 이메일 헤더 기반 분류                   │
└─────────────────────────────────────────┘
    │ (매칭 실패 시)
    ▼
┌─────────────────────────────────────────┐
│ Stage 2: Domain/Profile (~30%)          │
│ 발신자 도메인/프로필 기반 분류          │
└─────────────────────────────────────────┘
    │ (매칭 실패 시)
    ▼
┌─────────────────────────────────────────┐
│ Stage 3: LLM (~25%)                     │
│ OpenAI GPT로 분류 (fallback)            │
└─────────────────────────────────────────┘
    │
    ▼
분류 결과 저장 (ai_category, ai_priority, ai_summary)
```

### Stage 0: User Rules (사용자 정의 규칙)

사용자가 직접 설정한 분류 규칙. 최우선 적용.

```go
type ClassificationRules struct {
    // 높은 우선순위로 분류
    ImportantDomains  []string  // "@company.com" → work + high
    ImportantKeywords []string  // "긴급", "마감" → work + high
    
    // 낮은 우선순위로 분류
    IgnoreSenders     []string  // "noreply@" → other + low
    IgnoreKeywords    []string  // "광고", "unsubscribe" → other + low
    
    // 자연어 규칙 (LLM에 전달)
    HighPriorityRules string    // "CEO 메일은 항상 긴급"
    LowPriorityRules  string    // "뉴스레터는 나중에"
    CategoryRules     string    // "HR팀은 work 카테고리"
}
```

**분류 로직:**
1. IgnoreSenders 체크 → 매칭 시 `other` + `low`
2. IgnoreKeywords 체크 → 매칭 시 `other` + `low`
3. ImportantDomains 체크 → 매칭 시 `work` + `high`
4. ImportantKeywords 체크 → 매칭 시 `work` + `high`

### Stage 1: Headers (헤더 기반 분류)

이메일 헤더를 분석하여 자동 분류.

| 헤더 | 값 | 분류 결과 |
|------|---|----------|
| `List-Unsubscribe` | 존재 | newsletter + low |
| `Precedence` | bulk, list, junk | marketing + lowest |
| `X-Mailchimp-ID` | 존재 | marketing + low |
| `X-Campaign` | 존재 | marketing + low |

```go
type EmailHeaders struct {
    ListUnsubscribe     string  // 뉴스레터 구독 취소 링크
    ListUnsubscribePost string
    Precedence          string  // bulk, list, junk
    XMailer             string
    XCampaign           string  // 마케팅 캠페인 ID
    XMailchimpID        string  // Mailchimp 캠페인 ID
}
```

### Stage 2: Domain/Profile (도메인 기반 분류)

#### SenderProfile (발신자 프로필)
사용자별 발신자 학습 데이터.

```go
type SenderProfile struct {
    Email              string
    Domain             string
    LearnedCategory    *EmailCategory    // 학습된 카테고리
    LearnedSubCategory *EmailSubCategory
    IsVIP              bool              // VIP 발신자 → high priority
    IsMuted            bool              // 뮤트 → lowest priority
    ReadRate           float64           // 읽음 비율 (0.0 ~ 1.0)
    ReplyRate          float64           // 답장 비율 (0.0 ~ 1.0)
}
```

**Priority 결정 로직:**
- IsVIP=true → `high`
- IsMuted=true → `lowest`
- ReplyRate > 0.5 → `high`
- ReadRate < 0.2 → `low`
- 그 외 → `normal`

#### KnownDomain (알려진 도메인)
사전 정의된 도메인 분류 데이터.

```go
type KnownDomain struct {
    Domain      string         // "github.com"
    Category    EmailCategory  // notification
    SubCategory *EmailSubCategory
    Confidence  float64        // 0.0 ~ 1.0
    Source      string         // "system" | "user" | "learned"
}
```

### Stage 3: LLM (OpenAI 분류)

위 단계에서 분류되지 않은 메일만 LLM 호출.
전체 메일의 약 25%만 LLM 사용 → **~75% 비용 절감**

**입력:**
- Subject
- Snippet (미리보기 텍스트)
- From Email
- Body (일부)

**출력:**
```json
{
  "category": "work",
  "sub_category": "meeting",
  "priority": "high",
  "summary": "내일 오후 2시 팀 미팅 일정",
  "confidence": 0.92
}
```

---

## 4. 분류 결과 활용

### 4.1 쿼리 필터

```go
type MailListQuery struct {
    // 위치 필터
    Folder         string    // inbox, sent, drafts...
    
    // AI 분류 필터
    Category       string    // work, personal, newsletter...
    SubCategory    string    // receipt, invoice, shipping...
    Priority       *int      // 1=lowest ~ 5=urgent
    
    // 라벨/태그 필터
    Labels         []string  // Gmail 라벨
    Tags           []string  // 내부 태그
    LabelIDs       []int64   // 라벨 테이블 ID
    
    // 상태 필터
    IsRead         *bool
    IsStarred      *bool
    HasAttachment  *bool
    WorkflowStatus string    // todo, done, snoozed
    
    // 발신자 필터
    FromEmail      string
    FromDomain     string
    ContactID      *int64
}
```

### 4.2 Smart Folder (스마트 폴더)

AI 분류 결과를 활용한 동적 폴더.

```go
type SmartFolder struct {
    Name       string
    Conditions []FilterCondition  // AND 조건
}

// 예시: "중요 업무 메일"
{
    "name": "Important Work",
    "conditions": [
        {"field": "ai_category", "op": "eq", "value": "work"},
        {"field": "ai_priority", "op": "gte", "value": 4}
    ]
}
```

### 4.3 자동 액션

분류 결과에 따른 자동 처리.

| 조건 | 액션 |
|------|------|
| category=spam | trash로 이동 |
| category=newsletter + 읽지않음 3개 이상 | 자동 아카이브 제안 |
| priority=urgent | 푸시 알림 |
| subcategory=calendar | 캘린더 추출 제안 |

---

## 5. 데이터 흐름

### 새 메일 도착 시

```
Gmail Pub/Sub Webhook
    │
    ▼
DeltaSync (증분 동기화)
    │
    ├─→ 메일 메타데이터 저장 (PostgreSQL)
    │
    ├─→ 본문 저장 (MongoDB, 30일 TTL)
    │
    └─→ AI 작업 발행 (Redis Stream)
            │
            ├─→ ai:classify (분류)
            │       │
            │       ▼
            │   4단계 파이프라인 실행
            │       │
            │       ▼
            │   결과 저장 (ai_category, ai_priority...)
            │
            └─→ rag:index (RAG 인덱싱)
                    │
                    ▼
                pgvector 임베딩 저장
```

### 분류 결과 업데이트

```sql
UPDATE emails SET
    ai_status = 'completed',
    ai_category = 'work',
    ai_sub_category = 'meeting',
    ai_priority = 'high',
    ai_summary = '내일 오후 2시 팀 미팅',
    updated_at = NOW()
WHERE id = ?
```

---

## 6. 최적화 포인트

### 6.1 현재 구현된 최적화

- **4단계 파이프라인**: LLM 호출 ~75% 절감
- **규칙 정규화**: 소문자 변환 1회만 수행
- **배치 분류**: AI Processor에서 10개씩 묶어서 처리
- **Singleflight**: 동일 메일 중복 분류 방지

### 6.2 추가 최적화 필요

| 항목 | 현재 | 개선안 |
|------|------|--------|
| User Rules | 매번 DB 조회 | Redis 캐싱 (5분 TTL) |
| KnownDomain | 매번 DB 조회 | 인메모리 캐싱 |
| SenderProfile | 매번 업데이트 | 배치 업데이트 (1분 간격) |
| 배치 타임아웃 | 고정 3초 | 큐 깊이 기반 동적 조정 |

---

## 7. 관련 파일

| 파일 | 역할 |
|------|------|
| `core/domain/classification.go` | 분류 도메인 모델 |
| `core/domain/settings.go` | ClassificationRules 정의 |
| `core/domain/sender_profile.go` | SenderProfile, KnownDomain |
| `core/service/classification/pipeline.go` | 4단계 파이프라인 구현 |
| `core/service/ai/service.go` | AI 서비스 (분류 호출) |
| `adapter/in/worker/ai_processor.go` | 배치 분류 처리 |
| `adapter/out/persistence/mail_adapter.go` | DB 저장/조회 |
