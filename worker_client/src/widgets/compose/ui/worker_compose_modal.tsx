"use client";

import { useState, useRef, useEffect, useCallback, memo } from "react";
import {
  X,
  Minus,
  Maximize2,
  Paperclip,
  Image as ImageIcon,
  Link,
  Smile,
  Send,
} from "lucide-react";
import { cn } from "@/shared/lib";
import { Button, IconButton } from "@/shared/ui";

interface ComposeModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSend: (email: { to: string; subject: string; body: string }) => void;
  replyTo?: { email: string; subject: string };
}

interface SlashCommand {
  id: string;
  name: string;
  shortcut: string;
  icon: string;
  description: string;
  template: string;
}

const SLASH_COMMANDS: SlashCommand[] = [
  {
    id: "ack",
    name: "확인",
    shortcut: "/ack",
    icon: "✓",
    description: "메일 확인 응답",
    template: "메일 확인했습니다. 검토 후 회신드리겠습니다.",
  },
  {
    id: "ty",
    name: "감사",
    shortcut: "/ty",
    icon: "🙏",
    description: "감사 인사",
    template: "감사합니다.",
  },
  {
    id: "meeting",
    name: "회의 요청",
    shortcut: "/meeting",
    icon: "📅",
    description: "회의 일정 요청",
    template: "회의 일정을 조율하고자 합니다.\n\n- 일시: \n- 장소: \n- 안건: ",
  },
  {
    id: "fu",
    name: "팔로업",
    shortcut: "/fu",
    icon: "↩",
    description: "이전 메일 팔로업",
    template: "이전에 보내드린 내용 확인하셨을까요? 편하실 때 알려주세요.",
  },
  {
    id: "sig",
    name: "서명",
    shortcut: "/sig",
    icon: "✍",
    description: "서명 삽입",
    template: "\n\n감사합니다.\n[이름]\n[직함]",
  },
];

export const ComposeModal = memo(function ComposeModal({
  isOpen,
  onClose,
  onSend,
  replyTo,
}: ComposeModalProps) {
  const [to, setTo] = useState("");
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [minimized, setMinimized] = useState(false);
  const [showCommands, setShowCommands] = useState(false);
  const [commandFilter, setCommandFilter] = useState("");
  const [selectedCommand, setSelectedCommand] = useState(0);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Initialize with reply data
  useEffect(() => {
    if (replyTo) {
      setTo(replyTo.email);
      setSubject(
        replyTo.subject.startsWith("Re:")
          ? replyTo.subject
          : `Re: ${replyTo.subject}`,
      );
    }
  }, [replyTo]);

  // Focus textarea when opened
  useEffect(() => {
    if (isOpen && !minimized && textareaRef.current) {
      textareaRef.current.focus();
    }
  }, [isOpen, minimized]);

  // Filter commands based on input
  const filteredCommands = SLASH_COMMANDS.filter(
    (cmd) =>
      cmd.shortcut.toLowerCase().includes(commandFilter.toLowerCase()) ||
      cmd.name.includes(commandFilter.replace("/", "")),
  );

  const handleBodyChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      const value = e.target.value;
      setBody(value);

      // Check for slash command trigger
      const lastWord = value.split(/\s/).pop() || "";
      if (lastWord.startsWith("/") && lastWord.length > 0) {
        setShowCommands(true);
        setCommandFilter(lastWord);
        setSelectedCommand(0);
      } else {
        setShowCommands(false);
        setCommandFilter("");
      }
    },
    [],
  );

  const insertCommand = useCallback(
    (command: SlashCommand) => {
      const newBody = body.replace(/\/\w*$/, command.template);
      setBody(newBody);
      setShowCommands(false);
      setCommandFilter("");
      textareaRef.current?.focus();
    },
    [body],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (showCommands && filteredCommands.length > 0) {
        switch (e.key) {
          case "ArrowDown":
            e.preventDefault();
            setSelectedCommand((prev) =>
              Math.min(prev + 1, filteredCommands.length - 1),
            );
            break;
          case "ArrowUp":
            e.preventDefault();
            setSelectedCommand((prev) => Math.max(prev - 1, 0));
            break;
          case "Enter":
          case "Tab":
            e.preventDefault();
            if (filteredCommands[selectedCommand]) {
              insertCommand(filteredCommands[selectedCommand]);
            }
            break;
          case "Escape":
            e.preventDefault();
            setShowCommands(false);
            break;
        }
        return;
      }

      // Cmd/Ctrl + Enter to send
      if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
        e.preventDefault();
        handleSend();
      }
    },
    [showCommands, filteredCommands, selectedCommand, insertCommand], // eslint-disable-line react-hooks/exhaustive-deps
  );

  const handleSend = useCallback(() => {
    if (to.trim() && subject.trim() && body.trim()) {
      onSend({ to, subject, body });
      // Reset form
      setTo("");
      setSubject("");
      setBody("");
      onClose();
    }
  }, [to, subject, body, onSend, onClose]);

  const handleClose = useCallback(() => {
    setTo("");
    setSubject("");
    setBody("");
    setMinimized(false);
    onClose();
  }, [onClose]);

  if (!isOpen) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="compose-title"
      className={cn(
        "fixed right-6 bottom-0 z-50",
        "bg-bg-surface rounded-t-xl border border-border-default border-b-0",
        "shadow-lg transition-all duration-normal",
        minimized ? "w-[320px]" : "w-[560px]",
      )}
    >
      {/* Header */}
      <header className="flex items-center justify-between px-4 h-12 border-b border-border-subtle">
        <span
          id="compose-title"
          className="font-medium text-sm text-text-primary"
        >
          New Message
        </span>
        <div className="flex items-center gap-1">
          <IconButton
            size="sm"
            onClick={() => setMinimized(!minimized)}
            aria-label={minimized ? "Expand" : "Minimize"}
          >
            <Minus size={16} strokeWidth={1.5} />
          </IconButton>
          <IconButton size="sm" aria-label="Maximize">
            <Maximize2 size={16} strokeWidth={1.5} />
          </IconButton>
          <IconButton size="sm" onClick={handleClose} aria-label="Close">
            <X size={16} strokeWidth={1.5} />
          </IconButton>
        </div>
      </header>

      {!minimized && (
        <>
          {/* Form */}
          <div className="p-4 space-y-3">
            {/* To field */}
            <div className="flex items-center gap-2 border-b border-border-subtle pb-3">
              <label className="text-text-tertiary text-sm w-12">To</label>
              <input
                type="email"
                value={to}
                onChange={(e) => setTo(e.target.value)}
                placeholder="recipient@email.com"
                className="flex-1 bg-transparent text-text-primary placeholder:text-text-disabled focus:outline-none text-sm"
              />
            </div>

            {/* Subject field */}
            <div className="flex items-center gap-2 border-b border-border-subtle pb-3">
              <label className="text-text-tertiary text-sm w-12">Subject</label>
              <input
                type="text"
                value={subject}
                onChange={(e) => setSubject(e.target.value)}
                placeholder="Subject"
                className="flex-1 bg-transparent text-text-primary placeholder:text-text-disabled focus:outline-none text-sm"
              />
            </div>

            {/* Body with slash commands */}
            <div className="relative">
              <textarea
                ref={textareaRef}
                value={body}
                onChange={handleBodyChange}
                onKeyDown={handleKeyDown}
                placeholder="Write your message... (Type / for commands)"
                className={cn(
                  "w-full h-48 bg-transparent text-text-primary text-sm",
                  "placeholder:text-text-disabled resize-none focus:outline-none",
                  "leading-relaxed",
                )}
              />

              {/* Slash command popup */}
              {showCommands && filteredCommands.length > 0 && (
                <div
                  role="listbox"
                  className={cn(
                    "absolute left-0 bottom-full mb-2 w-72",
                    "bg-bg-overlay border border-border-default rounded-lg",
                    "shadow-lg overflow-hidden animate-scale-in",
                  )}
                >
                  <div className="px-3 py-2 border-b border-border-subtle">
                    <span className="text-xs text-text-tertiary font-medium">
                      Commands
                    </span>
                  </div>
                  {filteredCommands.map((cmd, index) => (
                    <button
                      key={cmd.id}
                      role="option"
                      aria-selected={index === selectedCommand}
                      onClick={() => insertCommand(cmd)}
                      onMouseEnter={() => setSelectedCommand(index)}
                      className={cn(
                        "w-full flex items-center gap-3 px-3 py-2.5 text-left",
                        "transition-colors duration-fast",
                        index === selectedCommand
                          ? "bg-bg-hover"
                          : "hover:bg-bg-hover",
                      )}
                    >
                      <span className="text-lg w-6 text-center">
                        {cmd.icon}
                      </span>
                      <div className="flex-1 min-w-0">
                        <div className="text-sm font-medium text-text-primary">
                          {cmd.shortcut}
                        </div>
                        <div className="text-xs text-text-tertiary truncate">
                          {cmd.description}
                        </div>
                      </div>
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* Footer */}
          <footer className="flex items-center justify-between px-4 py-3 border-t border-border-subtle">
            <div className="flex items-center gap-1">
              <IconButton size="sm" aria-label="Attach file">
                <Paperclip size={18} strokeWidth={1.5} />
              </IconButton>
              <IconButton size="sm" aria-label="Insert image">
                <ImageIcon size={18} strokeWidth={1.5} />
              </IconButton>
              <IconButton size="sm" aria-label="Insert link">
                <Link size={18} strokeWidth={1.5} />
              </IconButton>
              <IconButton size="sm" aria-label="Insert emoji">
                <Smile size={18} strokeWidth={1.5} />
              </IconButton>
            </div>

            <div className="flex items-center gap-2">
              <span className="text-xs text-text-tertiary hidden sm:block">
                ⌘ Enter to send
              </span>
              <Button onClick={handleSend} size="sm">
                <Send size={16} strokeWidth={1.5} />
                Send
              </Button>
            </div>
          </footer>
        </>
      )}
    </div>
  );
});
