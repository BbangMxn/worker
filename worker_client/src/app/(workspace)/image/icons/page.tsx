"use client";

import { useState, useRef, useEffect, useCallback } from "react";
import {
  Wand2,
  Download,
  Check,
  Copy,
  ChevronLeft,
  Loader2,
  Sparkles,
  History,
  X,
} from "lucide-react";
import Link from "next/link";
import { cn } from "@/shared/lib";

// Icon styles with visual preview
const ICON_STYLES = [
  { id: "line", name: "Line", preview: "○", desc: "Minimal outline style" },
  { id: "filled", name: "Filled", preview: "●", desc: "Solid filled style" },
  { id: "duotone", name: "Duo-tone", preview: "◐", desc: "Two-tone depth" },
  { id: "gradient", name: "Gradient", preview: "◑", desc: "Smooth gradients" },
] as const;

// Batch counts
const BATCH_COUNTS = [1, 4, 9, 16] as const;

// Export sizes
const EXPORT_SIZES = [16, 24, 32, 48, 64, 128, 256, 512] as const;

// Example prompts for inspiration
const EXAMPLE_PROMPTS = [
  "home icon",
  "settings gear",
  "user profile",
  "shopping cart",
  "search magnifier",
  "notification bell",
  "heart favorite",
  "star rating",
];

interface GeneratedIcon {
  id: string;
  prompt: string;
  style: string;
  url: string; // placeholder for now
  selected: boolean;
}

export default function IconGeneratorPage() {
  // State
  const [prompt, setPrompt] = useState("");
  const [style, setStyle] = useState<string>("line");
  const [batchCount, setBatchCount] = useState<number>(4);
  const [isGenerating, setIsGenerating] = useState(false);
  const [progress, setProgress] = useState(0);
  const [generatedIcons, setGeneratedIcons] = useState<GeneratedIcon[]>([]);
  const [history, setHistory] = useState<string[]>([]);
  const [showHistory, setShowHistory] = useState(false);
  const [exportSize, setExportSize] = useState<number>(64);

  // Refs
  const inputRef = useRef<HTMLInputElement>(null);

  // Auto-focus input on mount
  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Generate on Enter (when input focused)
      if (e.key === "Enter" && document.activeElement === inputRef.current) {
        e.preventDefault();
        handleGenerate();
        return;
      }

      // Batch count shortcuts (1-4 keys)
      if (["1", "2", "3", "4"].includes(e.key) && !e.metaKey && !e.ctrlKey) {
        const index = parseInt(e.key) - 1;
        if (BATCH_COUNTS[index]) {
          setBatchCount(BATCH_COUNTS[index]);
        }
        return;
      }

      // Select all (Cmd/Ctrl + A)
      if ((e.metaKey || e.ctrlKey) && e.key === "a" && generatedIcons.length > 0) {
        e.preventDefault();
        setGeneratedIcons((prev) =>
          prev.map((icon) => ({ ...icon, selected: true }))
        );
        return;
      }

      // Download selected (Cmd/Ctrl + S)
      if ((e.metaKey || e.ctrlKey) && e.key === "s" && generatedIcons.some(i => i.selected)) {
        e.preventDefault();
        handleDownloadSelected();
        return;
      }

      // Clear selection (Escape)
      if (e.key === "Escape") {
        setGeneratedIcons((prev) =>
          prev.map((icon) => ({ ...icon, selected: false }))
        );
        setShowHistory(false);
        return;
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [generatedIcons, prompt]); // eslint-disable-line react-hooks/exhaustive-deps

  // Generate icons
  const handleGenerate = useCallback(async () => {
    if (!prompt.trim() || isGenerating) return;

    setIsGenerating(true);
    setProgress(0);
    setGeneratedIcons([]);

    // Add to history
    setHistory((prev) => {
      const newHistory = [prompt, ...prev.filter((p) => p !== prompt)].slice(0, 10);
      return newHistory;
    });

    // Simulate generation with progress
    const interval = setInterval(() => {
      setProgress((prev) => {
        if (prev >= 95) {
          clearInterval(interval);
          return prev;
        }
        return prev + Math.random() * 15;
      });
    }, 200);

    // Simulate API delay
    await new Promise((resolve) => setTimeout(resolve, 2000));

    clearInterval(interval);
    setProgress(100);

    // Generate placeholder icons
    const icons: GeneratedIcon[] = Array.from({ length: batchCount }, (_, i) => ({
      id: `icon-${Date.now()}-${i}`,
      prompt,
      style,
      url: `/api/placeholder/${style}/${i}`, // placeholder
      selected: false,
    }));

    setGeneratedIcons(icons);
    setIsGenerating(false);
  }, [prompt, style, batchCount, isGenerating]);

  // Toggle icon selection
  const toggleIconSelection = (id: string) => {
    setGeneratedIcons((prev) =>
      prev.map((icon) =>
        icon.id === id ? { ...icon, selected: !icon.selected } : icon
      )
    );
  };

  // Select all icons
  const selectAll = () => {
    setGeneratedIcons((prev) =>
      prev.map((icon) => ({ ...icon, selected: true }))
    );
  };

  // Download selected icons
  const handleDownloadSelected = () => {
    const selected = generatedIcons.filter((i) => i.selected);
    if (selected.length === 0) return;

    // TODO: Implement actual download
    console.log("Downloading:", selected, "at size:", exportSize);
    alert(`Downloading ${selected.length} icon(s) at ${exportSize}px`);
  };

  // Use example prompt
  const setExamplePrompt = (example: string) => {
    setPrompt(example);
    inputRef.current?.focus();
  };

  const selectedCount = generatedIcons.filter((i) => i.selected).length;

  return (
    <div className="flex-1 flex flex-col h-screen overflow-hidden">
      {/* Header */}
      <header className="h-14 flex items-center justify-between px-6 border-b border-border-subtle shrink-0">
        <div className="flex items-center gap-3">
          <Link
            href="/image"
            className="p-1.5 -ml-1.5 rounded-lg hover:bg-bg-hover text-text-tertiary hover:text-text-primary transition-colors"
          >
            <ChevronLeft size={20} strokeWidth={1.5} />
          </Link>
          <h1 className="text-lg font-semibold">Icon Generator</h1>
        </div>

        {/* History toggle */}
        <button
          onClick={() => setShowHistory(!showHistory)}
          className={cn(
            "flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm transition-colors",
            showHistory
              ? "bg-bg-active text-text-primary"
              : "text-text-tertiary hover:text-text-primary hover:bg-bg-hover"
          )}
        >
          <History size={16} strokeWidth={1.5} />
          <span>History</span>
        </button>
      </header>

      {/* Main content */}
      <div className="flex-1 flex overflow-hidden">
        {/* Left panel - Input & Options */}
        <div className="w-[360px] border-r border-border-default flex flex-col shrink-0">
          {/* Prompt input */}
          <div className="p-4 border-b border-border-subtle">
            <label className="block text-sm font-medium text-text-secondary mb-2">
              Describe your icon
            </label>
            <div className="relative">
              <input
                ref={inputRef}
                type="text"
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                placeholder="e.g., home icon, settings gear..."
                className="w-full bg-bg-secondary border border-border-default rounded-lg px-4 py-3 pr-10 text-text-primary placeholder:text-text-muted focus:border-accent-primary focus:ring-1 focus:ring-accent-primary outline-none transition-colors"
              />
              {prompt && (
                <button
                  onClick={() => setPrompt("")}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-text-primary transition-colors"
                >
                  <X size={16} />
                </button>
              )}
            </div>
            <p className="text-xs text-text-muted mt-2">
              Press <kbd className="px-1.5 py-0.5 bg-bg-tertiary rounded text-text-tertiary">Enter</kbd> to generate
            </p>
          </div>

          {/* Example prompts */}
          {!prompt && (
            <div className="p-4 border-b border-border-subtle">
              <p className="text-xs text-text-muted mb-2">Try an example:</p>
              <div className="flex flex-wrap gap-2">
                {EXAMPLE_PROMPTS.slice(0, 6).map((example) => (
                  <button
                    key={example}
                    onClick={() => setExamplePrompt(example)}
                    className="px-2.5 py-1 bg-bg-tertiary hover:bg-bg-hover text-text-secondary hover:text-text-primary text-xs rounded-md transition-colors"
                  >
                    {example}
                  </button>
                ))}
              </div>
            </div>
          )}

          {/* Style selection */}
          <div className="p-4 border-b border-border-subtle">
            <label className="block text-sm font-medium text-text-secondary mb-3">
              Style
            </label>
            <div className="grid grid-cols-2 gap-2">
              {ICON_STYLES.map((s) => (
                <button
                  key={s.id}
                  onClick={() => setStyle(s.id)}
                  className={cn(
                    "flex flex-col items-center gap-1.5 p-3 rounded-lg border transition-all",
                    style === s.id
                      ? "border-accent-primary bg-accent-primary/10 text-text-primary"
                      : "border-border-default hover:border-border-light text-text-secondary hover:text-text-primary"
                  )}
                >
                  <span className="text-2xl">{s.preview}</span>
                  <span className="text-xs font-medium">{s.name}</span>
                </button>
              ))}
            </div>
          </div>

          {/* Batch count */}
          <div className="p-4 border-b border-border-subtle">
            <label className="block text-sm font-medium text-text-secondary mb-3">
              Generate count
            </label>
            <div className="flex gap-2">
              {BATCH_COUNTS.map((count, index) => (
                <button
                  key={count}
                  onClick={() => setBatchCount(count)}
                  className={cn(
                    "flex-1 py-2 rounded-lg border text-sm font-medium transition-all",
                    batchCount === count
                      ? "border-accent-primary bg-accent-primary/10 text-text-primary"
                      : "border-border-default hover:border-border-light text-text-secondary hover:text-text-primary"
                  )}
                >
                  {count}
                  <span className="text-xs text-text-muted ml-1">({index + 1})</span>
                </button>
              ))}
            </div>
          </div>

          {/* Export size */}
          <div className="p-4 border-b border-border-subtle">
            <label className="block text-sm font-medium text-text-secondary mb-3">
              Export size
            </label>
            <select
              value={exportSize}
              onChange={(e) => setExportSize(Number(e.target.value))}
              className="w-full bg-bg-secondary border border-border-default rounded-lg px-4 py-2.5 text-text-primary focus:border-accent-primary focus:ring-1 focus:ring-accent-primary outline-none transition-colors"
            >
              {EXPORT_SIZES.map((size) => (
                <option key={size} value={size}>
                  {size} x {size} px
                </option>
              ))}
            </select>
          </div>

          {/* Generate button */}
          <div className="p-4 mt-auto">
            <button
              onClick={handleGenerate}
              disabled={!prompt.trim() || isGenerating}
              className={cn(
                "flex items-center justify-center gap-2 w-full py-3 rounded-lg font-medium transition-all",
                prompt.trim() && !isGenerating
                  ? "bg-accent-primary hover:bg-accent-hover text-white"
                  : "bg-bg-tertiary text-text-muted cursor-not-allowed"
              )}
            >
              {isGenerating ? (
                <>
                  <Loader2 size={18} className="animate-spin" />
                  <span>Generating...</span>
                </>
              ) : (
                <>
                  <Sparkles size={18} strokeWidth={1.5} />
                  <span>Generate {batchCount} Icon{batchCount > 1 ? "s" : ""}</span>
                </>
              )}
            </button>

            {/* Progress bar */}
            {isGenerating && (
              <div className="mt-3">
                <div className="h-1 bg-bg-tertiary rounded-full overflow-hidden">
                  <div
                    className="h-full bg-accent-primary transition-all duration-300"
                    style={{ width: `${progress}%` }}
                  />
                </div>
                <p className="text-xs text-text-muted text-center mt-2">
                  {Math.round(progress)}% complete
                </p>
              </div>
            )}
          </div>
        </div>

        {/* Right panel - Results */}
        <div className="flex-1 flex flex-col overflow-hidden">
          {/* Results header */}
          {generatedIcons.length > 0 && (
            <div className="h-12 flex items-center justify-between px-4 border-b border-border-subtle shrink-0">
              <div className="flex items-center gap-3">
                <span className="text-sm text-text-secondary">
                  {generatedIcons.length} icon{generatedIcons.length > 1 ? "s" : ""} generated
                </span>
                {selectedCount > 0 && (
                  <span className="text-sm text-accent-primary font-medium">
                    {selectedCount} selected
                  </span>
                )}
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={selectAll}
                  className="px-3 py-1.5 text-sm text-text-secondary hover:text-text-primary hover:bg-bg-hover rounded-lg transition-colors"
                >
                  Select all
                </button>
                <button
                  onClick={handleDownloadSelected}
                  disabled={selectedCount === 0}
                  className={cn(
                    "flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-all",
                    selectedCount > 0
                      ? "bg-accent-primary hover:bg-accent-hover text-white"
                      : "bg-bg-tertiary text-text-muted cursor-not-allowed"
                  )}
                >
                  <Download size={14} />
                  <span>Download</span>
                </button>
              </div>
            </div>
          )}

          {/* Results grid */}
          <div className="flex-1 p-6 overflow-y-auto">
            {generatedIcons.length > 0 ? (
              <div
                className={cn(
                  "grid gap-4",
                  batchCount === 1 && "grid-cols-1 max-w-[200px] mx-auto",
                  batchCount === 4 && "grid-cols-2 max-w-[420px] mx-auto",
                  batchCount === 9 && "grid-cols-3 max-w-[620px] mx-auto",
                  batchCount === 16 && "grid-cols-4 max-w-[820px] mx-auto"
                )}
              >
                {generatedIcons.map((icon) => (
                  <button
                    key={icon.id}
                    onClick={() => toggleIconSelection(icon.id)}
                    className={cn(
                      "group relative aspect-square bg-bg-tertiary rounded-xl border-2 transition-all overflow-hidden",
                      icon.selected
                        ? "border-accent-primary ring-2 ring-accent-primary/20"
                        : "border-transparent hover:border-border-light"
                    )}
                  >
                    {/* Placeholder icon content */}
                    <div className="absolute inset-0 flex items-center justify-center">
                      <div
                        className={cn(
                          "w-16 h-16 rounded-xl flex items-center justify-center text-3xl",
                          style === "line" && "border-2 border-text-secondary",
                          style === "filled" && "bg-text-secondary text-bg-primary",
                          style === "duotone" && "bg-gradient-to-br from-accent-primary/30 to-accent-primary border border-accent-primary",
                          style === "gradient" && "bg-gradient-to-br from-violet-500 to-purple-600 text-white"
                        )}
                      >
                        <Wand2 size={28} strokeWidth={1.5} />
                      </div>
                    </div>

                    {/* Selection indicator */}
                    <div
                      className={cn(
                        "absolute top-2 right-2 w-6 h-6 rounded-full flex items-center justify-center transition-all",
                        icon.selected
                          ? "bg-accent-primary text-white"
                          : "bg-bg-primary/80 text-text-muted opacity-0 group-hover:opacity-100"
                      )}
                    >
                      {icon.selected ? (
                        <Check size={14} strokeWidth={2} />
                      ) : (
                        <span className="text-xs">+</span>
                      )}
                    </div>

                    {/* Hover overlay with actions */}
                    <div className="absolute inset-0 bg-bg-primary/80 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center gap-2">
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          // Copy to clipboard
                          alert("Copied to clipboard!");
                        }}
                        className="p-2 bg-bg-tertiary hover:bg-bg-hover rounded-lg text-text-secondary hover:text-text-primary transition-colors"
                        title="Copy to clipboard"
                      >
                        <Copy size={16} />
                      </button>
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          // Download single
                          alert(`Downloading at ${exportSize}px`);
                        }}
                        className="p-2 bg-bg-tertiary hover:bg-bg-hover rounded-lg text-text-secondary hover:text-text-primary transition-colors"
                        title="Download"
                      >
                        <Download size={16} />
                      </button>
                    </div>
                  </button>
                ))}
              </div>
            ) : (
              // Empty state
              <div className="h-full flex flex-col items-center justify-center text-text-tertiary">
                <div className="w-20 h-20 rounded-2xl bg-bg-tertiary flex items-center justify-center mb-4">
                  <Wand2 size={32} strokeWidth={1} className="opacity-50" />
                </div>
                <p className="text-lg font-medium mb-1">No icons yet</p>
                <p className="text-sm">Enter a prompt and click Generate</p>
              </div>
            )}
          </div>
        </div>

        {/* History sidebar */}
        {showHistory && (
          <div className="w-[240px] border-l border-border-default flex flex-col shrink-0">
            <div className="h-12 flex items-center justify-between px-4 border-b border-border-subtle">
              <span className="text-sm font-medium">Recent prompts</span>
              <button
                onClick={() => setShowHistory(false)}
                className="p-1 hover:bg-bg-hover rounded text-text-tertiary hover:text-text-primary transition-colors"
              >
                <X size={16} />
              </button>
            </div>
            <div className="flex-1 overflow-y-auto p-2">
              {history.length > 0 ? (
                history.map((item, index) => (
                  <button
                    key={index}
                    onClick={() => {
                      setPrompt(item);
                      setShowHistory(false);
                      inputRef.current?.focus();
                    }}
                    className="w-full text-left px-3 py-2 text-sm text-text-secondary hover:text-text-primary hover:bg-bg-hover rounded-lg transition-colors truncate"
                  >
                    {item}
                  </button>
                ))
              ) : (
                <p className="text-sm text-text-muted text-center py-4">
                  No history yet
                </p>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
