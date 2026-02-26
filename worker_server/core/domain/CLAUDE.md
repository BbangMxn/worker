# Domain Layer

## 목적

순수 비즈니스 모델 정의. 외부 의존성 없음.

## 설계 방향

- **Entity**: ID를 가진 도메인 객체 (Email, User, Calendar)
- **Value Object**: 불변 값 (Folder, Category, Priority)
- **Aggregate**: 관련 엔티티 묶음 (Email + Attachment)

## 구현 완료

- [x] `mail.go` - Email, Thread, Folder, Category, Priority
- [x] `calendar.go` - Event, Recurrence
- [x] `contact.go` - Contact, ContactGroup
- [x] `user.go` - User
- [x] `classification.go` - AIClassification
- [x] `settings.go` - UserSettings, ClassificationRules

## 구현 필요

- [ ] `sync.go` - SyncState, SyncJob, HistoryId
- [ ] `notification.go` - RealtimeEvent, PushMessage
- [ ] Email에 sync 관련 필드 추가 (history_id, sync_status)
