# Entities 폴더

> 도메인 엔티티 및 비즈니스 로직

## 구조

```
entities/
├── mail/
│   ├── model/           # 타입 정의
│   │   └── types.ts
│   ├── api/             # API 함수
│   │   └── mailApi.ts
│   ├── hooks/           # React Query 훅
│   │   └── useMails.ts
│   └── index.ts
│
├── calendar/
│   ├── model/
│   ├── api/
│   └── hooks/
│
├── user/
│   ├── model/
│   ├── api/
│   └── hooks/
│
└── contact/
    ├── model/
    ├── api/
    └── hooks/
```

## Mail Entity

```typescript
// mail/model/types.ts
export interface Mail {
  id: number;
  from: string;
  fromName?: string;
  to: string[];
  subject: string;
  snippet: string;
  body?: string;
  folder: Folder;
  isRead: boolean;
  isStarred: boolean;
  hasAttachment: boolean;
  workflowStatus: WorkflowStatus;
  aiCategory?: Category;
  aiPriority?: number;
  emailDate: string;
}

export type Folder = 'inbox' | 'sent' | 'draft' | 'trash' | 'spam' | 'archive';
export type WorkflowStatus = 'inbox' | 'todo' | 'done' | 'snoozed';
export type Category = 'work' | 'personal' | 'promo' | 'social';
```

```typescript
// mail/api/mailApi.ts
export const mailApi = {
  list: async (params: ListParams): Promise<MailListResponse> => {
    return api.get('/api/mail', { params });
  },
  
  getById: async (id: number): Promise<Mail> => {
    return api.get(`/api/mail/${id}`);
  },
  
  send: async (data: SendMailRequest): Promise<Mail> => {
    return api.post('/api/mail', data);
  },
  
  markRead: async (id: number, isRead: boolean): Promise<void> => {
    return api.patch(`/api/mail/${id}/read`, { isRead });
  },
  
  updateWorkflow: async (id: number, status: WorkflowStatus): Promise<void> => {
    return api.patch(`/api/mail/${id}/workflow`, { status });
  },
  
  archive: async (ids: number[]): Promise<void> => {
    return api.post('/api/mail/batch/archive', { ids });
  },
};
```

```typescript
// mail/hooks/useMails.ts
export function useMails(params: ListParams) {
  return useQuery({
    queryKey: ['mails', params],
    queryFn: () => mailApi.list(params),
    staleTime: 30 * 1000,  // 30초
  });
}

export function useMail(id: number) {
  return useQuery({
    queryKey: ['mail', id],
    queryFn: () => mailApi.getById(id),
    enabled: !!id,
  });
}

export function useSendMail() {
  const queryClient = useQueryClient();
  
  return useMutation({
    mutationFn: mailApi.send,
    onSuccess: () => {
      queryClient.invalidateQueries(['mails']);
    },
  });
}

export function useMarkRead() {
  const queryClient = useQueryClient();
  
  return useMutation({
    mutationFn: ({ id, isRead }: { id: number; isRead: boolean }) =>
      mailApi.markRead(id, isRead),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries(['mails']);
      queryClient.invalidateQueries(['mail', id]);
    },
  });
}
```

## Export 패턴

```typescript
// mail/index.ts
export * from './model/types';
export * from './api/mailApi';
export * from './hooks/useMails';

// 사용
import { Mail, useMails, mailApi } from '@/entities/mail';
```

## 규칙

1. **model**: 타입 정의만
2. **api**: API 호출 함수
3. **hooks**: React Query 래퍼
4. **UI 없음**: 엔티티에 컴포넌트 없음

## 주의사항
- 엔티티 간 의존성 최소화
- API 응답 타입 정확히 정의
- 낙관적 업데이트 적용
