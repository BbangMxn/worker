# Bridgify Frontend

> Next.js 기반 프론트엔드

## 기술 스택
- Next.js 14 (App Router)
- TypeScript
- Tailwind CSS
- React Query (TanStack Query)

## 폴더 구조

```
worker_front/
├── src/
│   ├── app/                 # Next.js App Router
│   │   ├── (workspace)/     # 메인 워크스페이스
│   │   │   ├── mail/        # 메일 페이지
│   │   │   ├── calendar/    # 캘린더 페이지
│   │   │   └── contacts/    # 연락처 페이지
│   │   ├── auth/            # 인증 페이지
│   │   └── layout.tsx       # 루트 레이아웃
│   │
│   ├── entities/            # 도메인 엔티티
│   │   ├── mail/
│   │   ├── calendar/
│   │   └── user/
│   │
│   ├── shared/              # 공유 컴포넌트
│   │   ├── ui/              # UI 컴포넌트
│   │   ├── lib/             # 유틸리티
│   │   └── api/             # API 클라이언트
│   │
│   └── widgets/             # 위젯
│       ├── mail-list/
│       ├── mail-detail/
│       └── sidebar/
│
├── public/                  # 정적 파일
├── tailwind.config.js
└── package.json
```

## 주요 페이지

### 메일 (`/mail`)
```
/mail                    # 받은편지함
/mail?folder=sent        # 보낸편지함
/mail?folder=draft       # 임시보관
/mail/:id                # 메일 상세
/mail/compose            # 메일 작성
```

### 캘린더 (`/calendar`)
```
/calendar                # 월간 뷰
/calendar?view=week      # 주간 뷰
/calendar?view=day       # 일간 뷰
```

### 연락처 (`/contacts`)
```
/contacts                # 연락처 목록
/contacts/:id            # 연락처 상세
```

## API 호출

```typescript
// api/mail.ts
export const mailApi = {
  list: (params: ListParams) => 
    api.get<MailListResponse>('/api/mail', { params }),
  
  getById: (id: number) => 
    api.get<Mail>(`/api/mail/${id}`),
  
  send: (data: SendMailRequest) => 
    api.post<Mail>('/api/mail', data),
  
  updateWorkflow: (id: number, status: WorkflowStatus) =>
    api.patch(`/api/mail/${id}/workflow`, { status }),
};
```

## 상태 관리

```typescript
// React Query 사용
const { data, isLoading } = useQuery({
  queryKey: ['mails', folder],
  queryFn: () => mailApi.list({ folder }),
});

// 뮤테이션
const mutation = useMutation({
  mutationFn: mailApi.send,
  onSuccess: () => {
    queryClient.invalidateQueries(['mails']);
  },
});
```

## 컴포넌트 규칙

```typescript
// 파일명: PascalCase
// MailList.tsx, MailItem.tsx

// 컴포넌트
export function MailList({ emails }: MailListProps) {
  return (
    <div className="space-y-2">
      {emails.map(email => (
        <MailItem key={email.id} email={email} />
      ))}
    </div>
  );
}
```

## 스타일 규칙

```typescript
// Tailwind CSS 사용
<div className="flex items-center gap-4 p-4 bg-white rounded-lg shadow">
  <span className="text-sm text-gray-600">
    {email.subject}
  </span>
</div>

// 조건부 스타일
<div className={cn(
  "p-4 rounded",
  isRead ? "bg-gray-50" : "bg-white font-semibold"
)}>
```

## 키보드 단축키

| 단축키 | 동작 |
|--------|------|
| `j/k` | 이전/다음 메일 |
| `o` | 메일 열기 |
| `e` | 보관 |
| `#` | 삭제 |
| `r` | 답장 |
| `c` | 새 메일 |

## 환경변수

```bash
NEXT_PUBLIC_API_URL=http://localhost:8080
NEXT_PUBLIC_WS_URL=ws://localhost:8080/ws
```

## 주의사항
- 서버 컴포넌트 우선 사용
- 클라이언트 상태 최소화
- 이미지 최적화 (next/image)
- 접근성 준수 (aria-*)
