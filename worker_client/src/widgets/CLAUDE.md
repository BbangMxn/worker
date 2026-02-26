# Widgets 폴더

> 페이지 구성 위젯 (복합 컴포넌트)

## 구조

```
widgets/
├── mail-list/           # 메일 목록
│   ├── MailList.tsx
│   ├── MailItem.tsx
│   ├── MailFilters.tsx
│   └── index.ts
│
├── mail-detail/         # 메일 상세
│   ├── MailDetail.tsx
│   ├── MailHeader.tsx
│   ├── MailBody.tsx
│   ├── MailActions.tsx
│   └── index.ts
│
├── mail-compose/        # 메일 작성
│   ├── ComposeModal.tsx
│   ├── ComposeForm.tsx
│   ├── RecipientInput.tsx
│   └── index.ts
│
├── sidebar/             # 사이드바
│   ├── Sidebar.tsx
│   ├── NavItem.tsx
│   ├── AccountSwitcher.tsx
│   └── index.ts
│
├── calendar-view/       # 캘린더 뷰
│   ├── CalendarView.tsx
│   ├── DayCell.tsx
│   ├── EventCard.tsx
│   └── index.ts
│
└── command-palette/     # 명령 팔레트
    ├── CommandPalette.tsx
    ├── CommandItem.tsx
    └── index.ts
```

## Widget vs Component

| 구분 | Widget | Component (shared/ui) |
|------|--------|----------------------|
| 복잡도 | 높음 | 낮음 |
| 상태 | 있음 | 없거나 최소 |
| 비즈니스 로직 | 있음 | 없음 |
| 재사용 | 페이지 간 | 앱 전체 |

## 위젯 예시

```typescript
// mail-list/MailList.tsx
'use client';

import { useMails, useMarkRead } from '@/entities/mail';
import { MailItem } from './MailItem';
import { MailFilters } from './MailFilters';

export function MailList({ folder }: { folder: string }) {
  const [filters, setFilters] = useState<Filters>({});
  const { data, isLoading } = useMails({ folder, ...filters });
  const markRead = useMarkRead();
  
  const handleSelect = (mail: Mail) => {
    if (!mail.isRead) {
      markRead.mutate({ id: mail.id, isRead: true });
    }
    router.push(`/mail/${mail.id}`);
  };
  
  return (
    <div className="flex flex-col h-full">
      <MailFilters value={filters} onChange={setFilters} />
      
      <div className="flex-1 overflow-auto">
        {isLoading ? (
          <MailListSkeleton />
        ) : (
          data?.emails.map(mail => (
            <MailItem 
              key={mail.id} 
              mail={mail} 
              onSelect={handleSelect}
            />
          ))
        )}
      </div>
    </div>
  );
}
```

```typescript
// sidebar/Sidebar.tsx
'use client';

import { usePathname } from 'next/navigation';
import { NavItem } from './NavItem';
import { AccountSwitcher } from './AccountSwitcher';

const navItems = [
  { href: '/mail', icon: Mail, label: '메일' },
  { href: '/calendar', icon: Calendar, label: '캘린더' },
  { href: '/contacts', icon: Users, label: '연락처' },
];

export function Sidebar() {
  const pathname = usePathname();
  
  return (
    <aside className="w-64 border-r bg-gray-50">
      <div className="p-4">
        <AccountSwitcher />
      </div>
      
      <nav className="space-y-1 px-2">
        {navItems.map(item => (
          <NavItem
            key={item.href}
            {...item}
            active={pathname.startsWith(item.href)}
          />
        ))}
      </nav>
    </aside>
  );
}
```

## 키보드 단축키 통합

```typescript
// mail-list/MailList.tsx
import { useKeyboard } from '@/shared/hooks';

export function MailList() {
  const [selectedIndex, setSelectedIndex] = useState(0);
  
  useKeyboard({
    'j': () => setSelectedIndex(i => Math.min(i + 1, emails.length - 1)),
    'k': () => setSelectedIndex(i => Math.max(i - 1, 0)),
    'o': () => handleSelect(emails[selectedIndex]),
    'e': () => handleArchive(emails[selectedIndex]),
  });
  
  // ...
}
```

## Export

```typescript
// mail-list/index.ts
export { MailList } from './MailList';

// 사용
import { MailList } from '@/widgets/mail-list';
```

## 주의사항
- 위젯은 'use client' 필수 (상태 있음)
- 하위 컴포넌트는 같은 폴더에
- 엔티티 훅 적극 활용
- 스타일은 Tailwind로 통일
