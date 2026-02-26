# App 폴더 (Next.js App Router)

> 페이지 및 라우팅

## 구조

```
app/
├── layout.tsx           # 루트 레이아웃
├── page.tsx             # 홈 (리다이렉트)
├── globals.css          # 전역 스타일
│
├── (workspace)/         # 워크스페이스 그룹
│   ├── layout.tsx       # 사이드바 포함 레이아웃
│   ├── mail/            # 메일
│   ├── calendar/        # 캘린더
│   └── contacts/        # 연락처
│
├── auth/                # 인증
│   ├── login/
│   └── callback/
│
└── api/                 # API Routes (필요시)
```

## 라우트 그룹

```typescript
// (workspace) - 괄호는 URL에 포함 안 됨
// /mail, /calendar, /contacts 모두 같은 레이아웃 공유

// (workspace)/layout.tsx
export default function WorkspaceLayout({ children }) {
  return (
    <div className="flex h-screen">
      <Sidebar />
      <main className="flex-1">{children}</main>
    </div>
  );
}
```

## 페이지 컴포넌트

```typescript
// 서버 컴포넌트 (기본)
export default async function MailPage() {
  const emails = await fetchEmails();
  return <MailList emails={emails} />;
}

// 클라이언트 컴포넌트 (필요시)
'use client';
export default function MailDetailPage() {
  const [email, setEmail] = useState(null);
  // ...
}
```

## 동적 라우트

```
mail/[id]/page.tsx      → /mail/123
calendar/[date]/page.tsx → /calendar/2025-01-10
```

## 로딩 & 에러

```typescript
// loading.tsx - 로딩 UI
export default function Loading() {
  return <Skeleton />;
}

// error.tsx - 에러 UI
'use client';
export default function Error({ error, reset }) {
  return <ErrorMessage error={error} onRetry={reset} />;
}

// not-found.tsx - 404
export default function NotFound() {
  return <div>페이지를 찾을 수 없습니다</div>;
}
```

## 메타데이터

```typescript
// 정적 메타데이터
export const metadata = {
  title: 'Bridgify - 메일',
  description: '스마트 이메일 관리',
};

// 동적 메타데이터
export async function generateMetadata({ params }) {
  const email = await getEmail(params.id);
  return { title: email.subject };
}
```

## 주의사항
- 서버 컴포넌트 우선
- 'use client'는 필요한 컴포넌트만
- 데이터 페칭은 서버에서
- Suspense로 스트리밍
