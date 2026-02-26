# Shared 폴더

> 공유 컴포넌트 및 유틸리티

## 구조

```
shared/
├── ui/                  # UI 컴포넌트
│   ├── Button.tsx
│   ├── Input.tsx
│   ├── Modal.tsx
│   ├── Dropdown.tsx
│   ├── Avatar.tsx
│   ├── Badge.tsx
│   ├── Skeleton.tsx
│   └── index.ts
│
├── lib/                 # 유틸리티
│   ├── api.ts           # API 클라이언트
│   ├── cn.ts            # 클래스 병합
│   ├── date.ts          # 날짜 포맷
│   └── storage.ts       # 로컬 스토리지
│
├── hooks/               # 커스텀 훅
│   ├── useDebounce.ts
│   ├── useKeyboard.ts
│   └── useLocalStorage.ts
│
└── types/               # 공통 타입
    └── index.ts
```

## UI 컴포넌트

```typescript
// Button.tsx
interface ButtonProps {
  variant?: 'primary' | 'secondary' | 'ghost';
  size?: 'sm' | 'md' | 'lg';
  loading?: boolean;
  children: React.ReactNode;
}

export function Button({ 
  variant = 'primary', 
  size = 'md',
  loading,
  children,
  ...props 
}: ButtonProps) {
  return (
    <button 
      className={cn(
        'rounded font-medium transition',
        variants[variant],
        sizes[size],
        loading && 'opacity-50 cursor-wait'
      )}
      disabled={loading}
      {...props}
    >
      {loading ? <Spinner /> : children}
    </button>
  );
}
```

## API 클라이언트

```typescript
// lib/api.ts
const api = {
  async get<T>(url: string, options?: RequestOptions): Promise<T> {
    const res = await fetch(`${API_URL}${url}`, {
      headers: { Authorization: `Bearer ${getToken()}` },
      ...options,
    });
    if (!res.ok) throw new ApiError(res);
    return res.json();
  },
  
  async post<T>(url: string, data: any): Promise<T> {
    const res = await fetch(`${API_URL}${url}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${getToken()}`,
      },
      body: JSON.stringify(data),
    });
    if (!res.ok) throw new ApiError(res);
    return res.json();
  },
};
```

## 유틸리티

```typescript
// lib/cn.ts - 클래스 병합
import { clsx } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// lib/date.ts - 날짜 포맷
export function formatRelative(date: Date): string {
  const now = new Date();
  const diff = now.getTime() - date.getTime();
  
  if (diff < 60000) return '방금 전';
  if (diff < 3600000) return `${Math.floor(diff / 60000)}분 전`;
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}시간 전`;
  return format(date, 'M월 d일');
}
```

## 커스텀 훅

```typescript
// hooks/useDebounce.ts
export function useDebounce<T>(value: T, delay: number): T {
  const [debouncedValue, setDebouncedValue] = useState(value);
  
  useEffect(() => {
    const timer = setTimeout(() => setDebouncedValue(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  
  return debouncedValue;
}

// hooks/useKeyboard.ts
export function useKeyboard(shortcuts: Record<string, () => void>) {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const key = e.key.toLowerCase();
      if (shortcuts[key]) {
        e.preventDefault();
        shortcuts[key]();
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [shortcuts]);
}
```

## Export 패턴

```typescript
// ui/index.ts
export { Button } from './Button';
export { Input } from './Input';
export { Modal } from './Modal';

// 사용
import { Button, Input, Modal } from '@/shared/ui';
```

## 주의사항
- 범용적인 것만 shared에
- 도메인 로직은 entities에
- 컴포넌트는 최대한 stateless
