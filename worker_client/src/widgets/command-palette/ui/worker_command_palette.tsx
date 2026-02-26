"use client";

import {
  useState,
  useEffect,
  useCallback,
  useRef,
  KeyboardEvent,
  useMemo,
} from "react";
import {
  Search,
  Mail,
  Users,
  FileText,
  Image as ImageIcon,
  Settings,
  Plus,
  Clock,
  Star,
  Archive,
  Trash2,
  Send,
  Calendar,
  Command,
} from "lucide-react";
import { cn } from "@/shared/lib";

interface CommandItem {
  id: string;
  label: string;
  description?: string;
  icon: React.ReactNode;
  shortcut?: string;
  group: string;
  action: () => void;
}

interface CommandPaletteProps {
  isOpen: boolean;
  onClose: () => void;
}

// Command groups and items
const createCommands = (onClose: () => void): CommandItem[] => [
  // Recent
  {
    id: "recent-1",
    label: "회의 안건 검토 요청",
    description: "김철수 · 2시간 전",
    icon: <Mail size={18} strokeWidth={1.5} />,
    group: "Recent",
    action: () => {
      console.log("Open email");
      onClose();
    },
  },
  {
    id: "recent-2",
    label: "프로젝트 기획안.docx",
    description: "문서 · 어제",
    icon: <FileText size={18} strokeWidth={1.5} />,
    group: "Recent",
    action: () => {
      console.log("Open document");
      onClose();
    },
  },

  // Navigation
  {
    id: "nav-inbox",
    label: "Inbox로 이동",
    icon: <Mail size={18} strokeWidth={1.5} />,
    shortcut: "G I",
    group: "Navigation",
    action: () => {
      console.log("Go to inbox");
      onClose();
    },
  },
  {
    id: "nav-contacts",
    label: "연락처로 이동",
    icon: <Users size={18} strokeWidth={1.5} />,
    shortcut: "G C",
    group: "Navigation",
    action: () => {
      console.log("Go to contacts");
      onClose();
    },
  },
  {
    id: "nav-documents",
    label: "문서로 이동",
    icon: <FileText size={18} strokeWidth={1.5} />,
    shortcut: "G D",
    group: "Navigation",
    action: () => {
      console.log("Go to documents");
      onClose();
    },
  },
  {
    id: "nav-images",
    label: "이미지로 이동",
    icon: <ImageIcon size={18} strokeWidth={1.5} />,
    shortcut: "G M",
    group: "Navigation",
    action: () => {
      console.log("Go to images");
      onClose();
    },
  },

  // Actions
  {
    id: "action-compose",
    label: "새 메일 작성",
    icon: <Plus size={18} strokeWidth={1.5} />,
    shortcut: "C",
    group: "Actions",
    action: () => {
      console.log("Compose email");
      onClose();
    },
  },
  {
    id: "action-event",
    label: "새 일정 만들기",
    icon: <Calendar size={18} strokeWidth={1.5} />,
    shortcut: "N",
    group: "Actions",
    action: () => {
      console.log("Create event");
      onClose();
    },
  },
  {
    id: "action-archive",
    label: "아카이브",
    icon: <Archive size={18} strokeWidth={1.5} />,
    shortcut: "E",
    group: "Actions",
    action: () => {
      console.log("Archive");
      onClose();
    },
  },
  {
    id: "action-star",
    label: "별표 추가/제거",
    icon: <Star size={18} strokeWidth={1.5} />,
    shortcut: "S",
    group: "Actions",
    action: () => {
      console.log("Toggle star");
      onClose();
    },
  },
  {
    id: "action-delete",
    label: "삭제",
    icon: <Trash2 size={18} strokeWidth={1.5} />,
    shortcut: "#",
    group: "Actions",
    action: () => {
      console.log("Delete");
      onClose();
    },
  },

  // Settings
  {
    id: "settings",
    label: "설정",
    icon: <Settings size={18} strokeWidth={1.5} />,
    shortcut: "⌘ ,",
    group: "Settings",
    action: () => {
      console.log("Open settings");
      onClose();
    },
  },
];

export function CommandPalette({ isOpen, onClose }: CommandPaletteProps) {
  const [query, setQuery] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const commands = useMemo(() => createCommands(onClose), [onClose]);

  // Filter commands based on query
  const filteredCommands = useMemo(() => {
    if (!query.trim()) return commands;

    const lowerQuery = query.toLowerCase();
    return commands.filter(
      (cmd) =>
        cmd.label.toLowerCase().includes(lowerQuery) ||
        cmd.description?.toLowerCase().includes(lowerQuery) ||
        cmd.group.toLowerCase().includes(lowerQuery),
    );
  }, [commands, query]);

  // Group filtered commands
  const groupedCommands = useMemo(() => {
    const groups: Record<string, CommandItem[]> = {};
    filteredCommands.forEach((cmd) => {
      if (!groups[cmd.group]) {
        groups[cmd.group] = [];
      }
      groups[cmd.group].push(cmd);
    });
    return groups;
  }, [filteredCommands]);

  // Reset state when opening
  useEffect(() => {
    if (isOpen) {
      setQuery("");
      setSelectedIndex(0);
      // Focus input after animation
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [isOpen]);

  // Scroll selected item into view
  useEffect(() => {
    if (listRef.current && filteredCommands.length > 0) {
      const selectedItem = listRef.current.querySelector(
        `[data-index="${selectedIndex}"]`,
      );
      selectedItem?.scrollIntoView({ block: "nearest" });
    }
  }, [selectedIndex, filteredCommands.length]);

  // Keyboard navigation
  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLInputElement>) => {
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          setSelectedIndex((prev) =>
            Math.min(prev + 1, filteredCommands.length - 1),
          );
          break;
        case "ArrowUp":
          e.preventDefault();
          setSelectedIndex((prev) => Math.max(prev - 1, 0));
          break;
        case "Enter":
          e.preventDefault();
          if (filteredCommands[selectedIndex]) {
            filteredCommands[selectedIndex].action();
          }
          break;
        case "Escape":
          e.preventDefault();
          onClose();
          break;
      }
    },
    [filteredCommands, selectedIndex, onClose],
  );

  // Reset selected index when query changes
  useEffect(() => {
    setSelectedIndex(0);
  }, [query]);

  if (!isOpen) return null;

  let flatIndex = 0;

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/20 z-50 animate-fade-in"
        onClick={onClose}
      />

      {/* Modal */}
      <div className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh]">
        <div
          className={cn(
            "w-full max-w-[560px] bg-bg-overlay rounded-xl shadow-xl",
            "border border-border-default overflow-hidden",
            "animate-scale-in",
          )}
        >
          {/* Search input */}
          <div className="flex items-center gap-3 px-4 py-3 border-b border-border-subtle">
            <Search size={20} className="text-text-tertiary shrink-0" />
            <input
              ref={inputRef}
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="검색하거나 명령 입력..."
              className="flex-1 bg-transparent text-text-primary placeholder:text-text-disabled outline-none text-base"
            />
            <kbd className="px-2 py-1 text-xs text-text-tertiary bg-bg-surface rounded border border-border-subtle">
              ESC
            </kbd>
          </div>

          {/* Results */}
          <div ref={listRef} className="max-h-[400px] overflow-y-auto py-2">
            {filteredCommands.length === 0 ? (
              <div className="px-4 py-8 text-center text-text-tertiary">
                <p>결과가 없습니다</p>
                <p className="text-sm mt-1">다른 검색어를 시도해보세요</p>
              </div>
            ) : (
              Object.entries(groupedCommands).map(([group, items]) => (
                <div key={group} className="mb-2">
                  <div className="px-4 py-1.5 text-xs font-medium text-text-tertiary uppercase tracking-wider">
                    {group}
                  </div>
                  {items.map((item) => {
                    const currentIndex = flatIndex++;
                    const isSelected = currentIndex === selectedIndex;

                    return (
                      <button
                        key={item.id}
                        data-index={currentIndex}
                        onClick={item.action}
                        onMouseEnter={() => setSelectedIndex(currentIndex)}
                        className={cn(
                          "w-full flex items-center gap-3 px-4 py-2.5 text-left",
                          "transition-colors duration-fast",
                          isSelected
                            ? "bg-bg-hover text-text-primary"
                            : "text-text-secondary hover:bg-bg-hover hover:text-text-primary",
                        )}
                      >
                        <span
                          className={cn(
                            "shrink-0",
                            isSelected
                              ? "text-accent-primary"
                              : "text-text-tertiary",
                          )}
                        >
                          {item.icon}
                        </span>

                        <div className="flex-1 min-w-0">
                          <div className="text-sm font-medium truncate">
                            {item.label}
                          </div>
                          {item.description && (
                            <div className="text-xs text-text-tertiary truncate">
                              {item.description}
                            </div>
                          )}
                        </div>

                        {item.shortcut && (
                          <kbd className="shrink-0 px-1.5 py-0.5 text-xs text-text-tertiary bg-bg-surface rounded border border-border-subtle">
                            {item.shortcut}
                          </kbd>
                        )}
                      </button>
                    );
                  })}
                </div>
              ))
            )}
          </div>

          {/* Footer hint */}
          <div className="px-4 py-2.5 border-t border-border-subtle flex items-center gap-4 text-xs text-text-tertiary">
            <span className="flex items-center gap-1">
              <kbd className="px-1 py-0.5 bg-bg-surface rounded border border-border-subtle">
                ↑↓
              </kbd>
              이동
            </span>
            <span className="flex items-center gap-1">
              <kbd className="px-1 py-0.5 bg-bg-surface rounded border border-border-subtle">
                ↵
              </kbd>
              선택
            </span>
            <span className="flex items-center gap-1">
              <kbd className="px-1 py-0.5 bg-bg-surface rounded border border-border-subtle">
                esc
              </kbd>
              닫기
            </span>
          </div>
        </div>
      </div>
    </>
  );
}
