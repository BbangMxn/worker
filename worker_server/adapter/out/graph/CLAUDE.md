# Neo4j Graph Adapters

> 사용자 개인화 및 패턴 학습을 위한 그래프 DB 어댑터

## 역할

Neo4j는 **관계 기반 데이터**를 저장하고 분석합니다:

| 어댑터 | 용도 | 그래프 관계 |
|--------|------|-------------|
| `PersonalizationAdapter` | 사용자 스타일/톤 분석 | User → Trait, WritingStyle, TonePreference, Phrase |
| `ClassificationAdapter` | 분류 패턴 학습 | User → ClassificationPattern |

**중요**: RAG 벡터 검색은 **pgvector** (Supabase)를 사용합니다. `vector_adapter.go`는 레거시입니다.

## 파일 구조

```
graph/
├── driver.go                  # Neo4j 드라이버 생성
├── personalization_adapter.go # 사용자 개인화 (out.PersonalizationStore)
├── classification_adapter.go  # 분류 패턴 (out.ClassificationPatternStore)
└── vector_adapter.go          # [레거시] pgvector로 대체됨
```

## 그래프 스키마

```
(:User {user_id, email, name, job_title, company})
    │
    ├─[:HAS_TRAIT]─→ (:Trait {name, score})
    │
    ├─[:HAS_WRITING_STYLE]─→ (:WritingStyle {
    │       embedding, avg_sentence_length, formality_score, emoji_frequency
    │   })
    │
    ├─[:HAS_TONE_PREF]─→ (:TonePreference {context, style, formality})
    │
    ├─[:USES_PHRASE]─→ (:Phrase {text, count, category, last_used})
    │
    ├─[:HAS_SIGNATURE]─→ (:Signature {id, text, is_default})
    │
    └─[:HAS_PATTERN]─→ (:ClassificationPattern {
            email_id, from_addr, subject, category, priority, intent, embedding
        })
```

## PersonalizationAdapter

사용자 작성 스타일과 톤 분석.

### 인터페이스: `out.PersonalizationStore`

```go
// 프로필
GetUserProfile(ctx, userID) (*UserProfile, error)
UpdateUserProfile(ctx, userID, *UserProfile) error

// 특성
GetUserTraits(ctx, userID) ([]*UserTrait, error)
UpdateUserTrait(ctx, userID, *UserTrait) error

// 작성 스타일
GetWritingStyle(ctx, userID) (*WritingStyle, error)
UpdateWritingStyle(ctx, userID, *WritingStyle) error

// 톤 설정 (수신자/컨텍스트별)
GetTonePreference(ctx, userID, context) (*TonePreference, error)
UpdateTonePreference(ctx, userID, *TonePreference) error

// 자주 쓰는 표현
GetFrequentPhrases(ctx, userID, limit) ([]*FrequentPhrase, error)
AddPhrase(ctx, userID, *FrequentPhrase) error
IncrementPhraseCount(ctx, userID, phraseText) error

// 서명
GetSignatures(ctx, userID) ([]*Signature, error)
SetDefaultSignature(ctx, userID, signatureID) error
```

### 사용 예시

```go
// 답장 생성 시 스타일 참조
style, _ := personStore.GetWritingStyle(ctx, userID)
tonePref, _ := personStore.GetTonePreference(ctx, userID, recipientDomain)

// 자동완성 시 자주 쓰는 표현 참조
phrases, _ := personStore.GetFrequentPhrases(ctx, userID, 10)
```

## ClassificationAdapter

이메일 분류 패턴 학습 및 검색.

### 인터페이스: `out.ClassificationPatternStore`

```go
// 패턴 저장 (분류 후 학습)
Store(ctx, *ClassificationPattern, embedding []float32) error
BatchStore(ctx, []*ClassificationPattern, [][]float32) error

// 유사 패턴 검색 (분류 전 참조)
Search(ctx, userID, embedding []float32, topK int) ([]*ClassificationPattern, error)
GetByCategory(ctx, userID, category, limit) ([]*ClassificationPattern, error)

// 삭제
Delete(ctx, userID, emailID) error
DeleteByUser(ctx, userID) error

// 통계
GetStats(ctx, userID) (*PatternStats, error)
```

### 분류 플로우

```
새 메일 수신
    ↓
임베딩 생성 (OpenAI)
    ↓
ClassificationAdapter.Search() → 유사 패턴 검색
    ↓
[패턴 있음] → 패턴 기반 분류 (빠름)
[패턴 없음] → LLM 분류 (정확)
    ↓
ClassificationAdapter.Store() → 패턴 학습
```

### 수동 분류 학습

사용자가 직접 분류하면 `is_manual=true`로 저장하여 우선순위 부여:

```go
pattern := &out.ClassificationPattern{
    UserID:   userID,
    EmailID:  emailID,
    Category: "work",
    Priority: 1,
    IsManual: true,  // 사용자 직접 분류
}
adapter.Store(ctx, pattern, embedding)
```

## 인덱스

```cypher
-- PersonalizationAdapter
CREATE CONSTRAINT user_id_unique FOR (u:User) REQUIRE u.user_id IS UNIQUE
CREATE INDEX user_email_idx FOR (u:User) ON (u.email)
CREATE INDEX trait_name_idx FOR (t:Trait) ON (t.name)
CREATE INDEX phrase_user_idx FOR (p:Phrase) ON (p.user_id)
CREATE INDEX tone_user_context_idx FOR (tp:TonePreference) ON (tp.user_id, tp.context)

-- ClassificationAdapter
CREATE VECTOR INDEX pattern_embedding_index FOR (p:ClassificationPattern) ON (p.embedding)
CREATE INDEX pattern_user_idx FOR (p:ClassificationPattern) ON (p.user_id)
CREATE INDEX pattern_category_idx FOR (p:ClassificationPattern) ON (p.category)
```

## 주의사항

1. **벡터 검색은 pgvector 사용**: `vector_adapter.go`는 레거시, RAG는 `core/agent/rag/vectorstore.go` 사용
2. **EnsureIndexes 호출 필수**: 앱 시작 시 인덱스 생성
3. **트랜잭션**: Neo4j 세션은 자동 커밋, 필요시 명시적 트랜잭션 사용
