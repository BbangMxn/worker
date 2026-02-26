"use client";

import { Suspense, useState, useCallback, useMemo } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Sidebar } from "@/widgets/sidebar";
import { CommandPalette } from "@/widgets/command-palette";
import { useGlobalShortcuts } from "@/shared/hooks";

// Route configuration - centralized routing logic
const ROUTE_CONFIG = {
  mail: {
    path: "/mail",
    folders: [
      "all",
      "inbox",
      "todo",
      "done",
      "starred",
      "sent",
      "drafts",
      "archive",
      "trash",
    ],
  },
  calendar: {
    path: "/calendar",
    folders: ["today", "upcoming", "tasks"],
  },
  contacts: {
    path: "/contacts",
    folders: ["all", "favorites"],
  },
  documents: {
    path: "/documents",
    folders: ["recent", "folders"],
  },
  image: {
    path: "/image",
    folders: ["generate", "icons", "enhance", "styles", "history"],
  },
} as const;

type AppType = keyof typeof ROUTE_CONFIG;

function WorkspaceLayoutContent({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);

  // Determine current app and folder from URL
  const { currentApp, activeFolder } = useMemo(() => {
    const path = pathname || "/mail";
    const folder = searchParams?.get("folder");

    for (const [app, config] of Object.entries(ROUTE_CONFIG)) {
      if (path.startsWith(config.path)) {
        return {
          currentApp: app as AppType,
          activeFolder: folder || config.folders[0],
        };
      }
    }

    return { currentApp: "mail" as AppType, activeFolder: folder || "inbox" };
  }, [pathname, searchParams]);

  // Handle folder navigation
  const handleFolderChange = useCallback(
    (folder: string) => {
      const config = ROUTE_CONFIG[currentApp];

      // Check if folder belongs to current app
      if (config.folders.includes(folder as never)) {
        router.push(`${config.path}?folder=${folder}`);
      } else {
        // Find which app this folder belongs to
        for (const [app, appConfig] of Object.entries(ROUTE_CONFIG)) {
          if (appConfig.folders.includes(folder as never)) {
            router.push(`${appConfig.path}?folder=${folder}`);
            return;
          }
        }
        // Default fallback
        router.push(`${config.path}?folder=${folder}`);
      }
    },
    [currentApp, router],
  );

  // Handle compose action based on current app
  const handleCompose = useCallback(() => {
    // Dispatch custom event for compose - pages can listen to this
    window.dispatchEvent(
      new CustomEvent("app:compose", { detail: { app: currentApp } }),
    );
  }, [currentApp]);

  // Global keyboard shortcuts
  useGlobalShortcuts({
    onCommandPalette: () => setCommandPaletteOpen(true),
    onCompose: handleCompose,
    onSearch: () => setCommandPaletteOpen(true),
  });

  // Sidebar width calculation
  const sidebarWidth = sidebarCollapsed ? 60 : 260;

  return (
    <div className="h-screen flex bg-bg-base overflow-hidden">
      {/* Sidebar - Fixed position */}
      <Sidebar
        activeFolder={activeFolder}
        onFolderChange={handleFolderChange}
        onCompose={handleCompose}
        collapsed={sidebarCollapsed}
        onCollapsedChange={setSidebarCollapsed}
      />

      {/* Main Content Area */}
      <main
        className="flex-1 flex transition-[margin] duration-normal ease-smooth overflow-hidden"
        style={{ marginLeft: sidebarWidth }}
      >
        {children}
      </main>

      {/* Command Palette - Global */}
      <CommandPalette
        isOpen={commandPaletteOpen}
        onClose={() => setCommandPaletteOpen(false)}
      />

      {/* Keyboard Shortcut Hint */}
      <div className="fixed bottom-4 right-4 z-40">
        <button
          onClick={() => setCommandPaletteOpen(true)}
          className="flex items-center gap-2 px-3 py-2 bg-bg-elevated border border-border-default rounded-lg text-sm text-text-tertiary hover:text-text-primary hover:bg-bg-hover transition-colors shadow-sm"
        >
          <kbd className="px-1.5 py-0.5 bg-bg-surface rounded text-xs font-mono">
            ⌘
          </kbd>
          <kbd className="px-1.5 py-0.5 bg-bg-surface rounded text-xs font-mono">
            K
          </kbd>
          <span className="ml-1 text-text-disabled">to search</span>
        </button>
      </div>
    </div>
  );
}

export default function WorkspaceLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <Suspense
      fallback={
        <div className="h-screen flex items-center justify-center bg-bg-base">
          <div className="w-8 h-8 border-2 border-accent-primary border-t-transparent rounded-full animate-spin" />
        </div>
      }
    >
      <WorkspaceLayoutContent>{children}</WorkspaceLayoutContent>
    </Suspense>
  );
}
