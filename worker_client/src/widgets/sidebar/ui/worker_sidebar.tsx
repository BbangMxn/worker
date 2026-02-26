"use client";

import { useState, useCallback, useEffect } from "react";
import { useRouter, usePathname } from "next/navigation";
import {
  Inbox,
  Send,
  FileText,
  Trash2,
  Star,
  Archive,
  Clock,
  Search,
  Settings,
  Calendar,
  CheckSquare,
  Mail,
  ChevronLeft,
  ChevronRight,
  Plus,
  Image,
  Wand2,
  History,
  Sparkles,
  Palette,
  Grid3X3,
  Users,
  FolderOpen,
  PanelLeftClose,
  PanelLeft,
} from "lucide-react";
import { cn } from "@/shared/lib";

type AppType = "mail" | "calendar" | "image" | "contacts" | "documents";

interface NavItem {
  id: string;
  label: string;
  icon: React.ReactNode;
  count?: number;
  shortcut?: string;
}

// Icon size constants
const ICON_SIZE = 20;
const ICON_STROKE = 1.5;

const mailNav: NavItem[] = [
  {
    id: "all",
    label: "All Mail",
    icon: <Mail size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G A",
  },
  {
    id: "inbox",
    label: "Inbox",
    icon: <Inbox size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    count: 12,
    shortcut: "G I",
  },
  {
    id: "todo",
    label: "Todo",
    icon: <Clock size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    count: 2,
    shortcut: "G T",
  },
  {
    id: "done",
    label: "Done",
    icon: <CheckSquare size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G D",
  },
  {
    id: "starred",
    label: "Starred",
    icon: <Star size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G S",
  },
  {
    id: "sent",
    label: "Sent",
    icon: <Send size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G E",
  },
  {
    id: "drafts",
    label: "Drafts",
    icon: <FileText size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    count: 3,
    shortcut: "G R",
  },
  {
    id: "archive",
    label: "Archive",
    icon: <Archive size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G V",
  },
  {
    id: "trash",
    label: "Trash",
    icon: <Trash2 size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G #",
  },
];

const calendarNav: NavItem[] = [
  {
    id: "today",
    label: "Today",
    icon: <Calendar size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G T",
  },
  {
    id: "upcoming",
    label: "Upcoming",
    icon: <Clock size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G U",
  },
  {
    id: "tasks",
    label: "Tasks",
    icon: <CheckSquare size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    count: 5,
    shortcut: "G K",
  },
];

const contactsNav: NavItem[] = [
  {
    id: "all",
    label: "All Contacts",
    icon: <Users size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G A",
  },
  {
    id: "favorites",
    label: "Favorites",
    icon: <Star size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G F",
  },
];

const documentsNav: NavItem[] = [
  {
    id: "recent",
    label: "Recent",
    icon: <Clock size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G R",
  },
  {
    id: "folders",
    label: "Folders",
    icon: <FolderOpen size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G F",
  },
];

const imageNav: NavItem[] = [
  {
    id: "generate",
    label: "Generate",
    icon: <Wand2 size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G G",
  },
  {
    id: "icons",
    label: "Icons",
    icon: <Grid3X3 size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G I",
  },
  {
    id: "enhance",
    label: "Enhance",
    icon: <Sparkles size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G E",
  },
  {
    id: "styles",
    label: "Styles",
    icon: <Palette size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    shortcut: "G S",
  },
  {
    id: "history",
    label: "History",
    icon: <History size={ICON_SIZE} strokeWidth={ICON_STROKE} />,
    count: 24,
    shortcut: "G H",
  },
];

const APP_CONFIG: Record<
  AppType,
  { title: string; action: string; nav: NavItem[]; path: string }
> = {
  mail: { title: "Mail", action: "Compose", nav: mailNav, path: "/mail" },
  calendar: {
    title: "Calendar",
    action: "New Event",
    nav: calendarNav,
    path: "/calendar",
  },
  contacts: {
    title: "Contacts",
    action: "Add Contact",
    nav: contactsNav,
    path: "/contacts",
  },
  documents: {
    title: "Documents",
    action: "New Doc",
    nav: documentsNav,
    path: "/documents",
  },
  image: { title: "AI Image", action: "Create", nav: imageNav, path: "/image" },
};

const APP_LIST = [
  { type: "mail" as AppType, icon: Mail, label: "Mail" },
  { type: "calendar" as AppType, icon: Calendar, label: "Calendar" },
  { type: "contacts" as AppType, icon: Users, label: "Contacts" },
  { type: "documents" as AppType, icon: FileText, label: "Documents" },
  { type: "image" as AppType, icon: Image, label: "AI Image" },
];

interface SidebarProps {
  activeFolder: string;
  onFolderChange: (folder: string) => void;
  onCompose: () => void;
  collapsed?: boolean;
  onCollapsedChange?: (collapsed: boolean) => void;
}

export function Sidebar({
  activeFolder,
  onFolderChange,
  onCompose,
  collapsed = false,
  onCollapsedChange,
}: SidebarProps) {
  const router = useRouter();
  const pathname = usePathname();

  // Determine active app from pathname
  const [activeApp, setActiveApp] = useState<AppType>(() => {
    for (const [app, config] of Object.entries(APP_CONFIG)) {
      if (pathname?.startsWith(config.path)) {
        return app as AppType;
      }
    }
    return "mail";
  });

  // Sync active app with pathname
  useEffect(() => {
    for (const [app, config] of Object.entries(APP_CONFIG)) {
      if (pathname?.startsWith(config.path)) {
        setActiveApp(app as AppType);
        return;
      }
    }
  }, [pathname]);

  const config = APP_CONFIG[activeApp];

  const toggleCollapse = useCallback(() => {
    onCollapsedChange?.(!collapsed);
  }, [collapsed, onCollapsedChange]);

  // Handle app switch
  const handleAppSwitch = useCallback(
    (app: AppType) => {
      setActiveApp(app);
      const appConfig = APP_CONFIG[app];
      router.push(appConfig.path);
    },
    [router],
  );

  // Keyboard shortcut for sidebar toggle ([ key)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "[" && !e.metaKey && !e.ctrlKey && !e.altKey) {
        const target = e.target as HTMLElement;
        if (target.tagName !== "INPUT" && target.tagName !== "TEXTAREA") {
          e.preventDefault();
          toggleCollapse();
        }
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [toggleCollapse]);

  return (
    <aside
      className={cn(
        "fixed left-0 top-0 h-screen bg-bg-base flex z-50",
        "transition-[width] duration-normal ease-smooth",
      )}
    >
      {/* App Rail - Always visible (60px) */}
      <div className="w-[60px] flex flex-col items-center py-4 border-r border-border-subtle bg-bg-elevated">
        {/* Logo - Desk Icon */}
        <button
          onClick={() => router.push("/mail")}
          className="w-9 h-9 rounded-lg bg-gradient-to-br from-accent-primary to-blue-600 flex items-center justify-center mb-4 hover:opacity-90 transition-opacity"
          title="Home"
          aria-label="Go to home"
        >
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
            className="text-white"
          >
            {/* Desk surface */}
            <rect x="2" y="10" width="20" height="3" rx="1" />
            {/* Left leg */}
            <path d="M4 13v7" />
            {/* Right leg */}
            <path d="M20 13v7" />
            {/* Monitor */}
            <rect x="8" y="4" width="8" height="6" rx="1" />
            {/* Monitor stand */}
            <path d="M12 10v2" />
          </svg>
        </button>

        {/* Sidebar Toggle Button */}
        <button
          onClick={toggleCollapse}
          className={cn(
            "w-10 h-10 rounded-lg flex items-center justify-center mb-4",
            "text-text-tertiary hover:text-text-primary hover:bg-bg-hover",
            "transition-colors duration-fast",
          )}
          title={collapsed ? "Expand sidebar ([)" : "Collapse sidebar ([)"}
          aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
        >
          {collapsed ? (
            <PanelLeft size={ICON_SIZE} strokeWidth={ICON_STROKE} />
          ) : (
            <PanelLeftClose size={ICON_SIZE} strokeWidth={ICON_STROKE} />
          )}
        </button>

        {/* Divider */}
        <div className="w-8 h-px bg-border-subtle mb-4" />

        {/* App Switcher */}
        <nav className="flex-1 flex flex-col items-center gap-1">
          {APP_LIST.map(({ type, icon: Icon, label }) => (
            <button
              key={type}
              onClick={() => handleAppSwitch(type)}
              className={cn(
                "w-10 h-10 rounded-lg flex items-center justify-center",
                "transition-colors duration-fast",
                activeApp === type
                  ? "bg-accent-primary/10 text-accent-primary"
                  : "text-text-tertiary hover:text-text-primary hover:bg-bg-hover",
              )}
              title={label}
              aria-label={label}
              aria-current={activeApp === type ? "page" : undefined}
            >
              <Icon size={ICON_SIZE} strokeWidth={ICON_STROKE} />
            </button>
          ))}
        </nav>

        {/* Bottom Actions */}
        <div className="flex flex-col items-center gap-1">
          <button
            className="w-10 h-10 rounded-lg flex items-center justify-center text-text-tertiary hover:text-text-primary hover:bg-bg-hover transition-colors duration-fast"
            title="Search (⌘K)"
            aria-label="Search"
          >
            <Search size={ICON_SIZE} strokeWidth={ICON_STROKE} />
          </button>
          <button
            className="w-10 h-10 rounded-lg flex items-center justify-center text-text-tertiary hover:text-text-primary hover:bg-bg-hover transition-colors duration-fast"
            title="Settings"
            aria-label="Settings"
          >
            <Settings size={ICON_SIZE} strokeWidth={ICON_STROKE} />
          </button>
        </div>
      </div>

      {/* Navigation Panel - Collapsible */}
      <div
        className={cn(
          "flex flex-col border-r border-border-default bg-bg-elevated overflow-hidden",
          "transition-all duration-normal ease-smooth",
          collapsed ? "w-0 opacity-0" : "w-[200px] opacity-100",
        )}
      >
        {/* Header */}
        <div className="h-14 flex items-center justify-between px-4 border-b border-border-subtle shrink-0">
          <span className="font-semibold text-text-primary text-sm">
            {config.title}
          </span>
          <button
            onClick={toggleCollapse}
            className="p-1.5 rounded-md hover:bg-bg-hover text-text-tertiary hover:text-text-primary transition-colors duration-fast"
            aria-label="Collapse sidebar"
            title="Collapse ([)"
          >
            <ChevronLeft size={16} strokeWidth={2} />
          </button>
        </div>

        {/* Primary Action */}
        <div className="p-3 shrink-0">
          <button
            onClick={onCompose}
            className={cn(
              "flex items-center justify-center gap-2 w-full py-2.5 rounded-lg",
              "bg-accent-primary hover:bg-accent-hover text-white font-medium text-sm",
              "transition-colors duration-fast shadow-sm",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-primary focus-visible:ring-offset-2",
            )}
          >
            <Plus size={16} strokeWidth={2} />
            <span>{config.action}</span>
          </button>
        </div>

        {/* Navigation Items */}
        <nav className="flex-1 px-2 py-1 overflow-y-auto" role="navigation">
          {config.nav.map((item) => (
            <button
              key={item.id}
              onClick={() => onFolderChange(item.id)}
              className={cn(
                "flex items-center gap-3 w-full px-3 py-2 rounded-lg mb-0.5",
                "transition-colors duration-fast group text-sm",
                activeFolder === item.id
                  ? "bg-accent-primary/10 text-accent-primary font-medium"
                  : "text-text-secondary hover:bg-bg-hover hover:text-text-primary",
              )}
              aria-current={activeFolder === item.id ? "page" : undefined}
            >
              <span className="shrink-0">{item.icon}</span>
              <span className="flex-1 text-left truncate">{item.label}</span>
              {item.count !== undefined && item.count > 0 && (
                <span
                  className={cn(
                    "text-xs tabular-nums px-1.5 py-0.5 rounded-full",
                    activeFolder === item.id
                      ? "bg-accent-primary/20 text-accent-primary"
                      : "bg-bg-surface text-text-tertiary",
                  )}
                >
                  {item.count}
                </span>
              )}
            </button>
          ))}
        </nav>
      </div>

      {/* Expand Handle - Hover trigger when collapsed */}
      {collapsed && (
        <div
          className="absolute left-[60px] top-0 bottom-0 w-2 cursor-pointer group"
          onClick={toggleCollapse}
        >
          <div
            className={cn(
              "absolute left-0 top-1/2 -translate-y-1/2",
              "w-4 h-12 bg-bg-elevated border border-border-default rounded-r-lg",
              "flex items-center justify-center",
              "text-text-disabled group-hover:text-text-primary group-hover:bg-bg-hover",
              "transition-all duration-fast",
              "opacity-0 group-hover:opacity-100",
            )}
          >
            <ChevronRight size={14} strokeWidth={2} />
          </div>
        </div>
      )}
    </aside>
  );
}
