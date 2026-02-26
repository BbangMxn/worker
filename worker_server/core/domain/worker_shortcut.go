package domain

import (
	"time"

	"github.com/google/uuid"
)

// ShortcutPreset defines available preset types
type ShortcutPreset string

const (
	PresetSuperhuman ShortcutPreset = "superhuman"
	PresetGmail      ShortcutPreset = "gmail"
	PresetCustom     ShortcutPreset = "custom"
)

// KeyboardShortcuts represents user's keyboard shortcut settings
type KeyboardShortcuts struct {
	ID        int64             `json:"id"`
	UserID    uuid.UUID         `json:"user_id"`
	Preset    ShortcutPreset    `json:"preset"`
	Enabled   bool              `json:"enabled"`
	ShowHints bool              `json:"show_hints"`
	Shortcuts map[string]string `json:"shortcuts"` // action -> key mapping
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// ShortcutAction defines all available shortcut actions
type ShortcutAction string

const (
	// Mail Navigation
	ActionMailNext      ShortcutAction = "mail.next"
	ActionMailPrev      ShortcutAction = "mail.prev"
	ActionMailOpen      ShortcutAction = "mail.open"
	ActionMailBack      ShortcutAction = "mail.back"
	ActionMailExpand    ShortcutAction = "mail.expand"
	ActionMailExpandAll ShortcutAction = "mail.expand_all"

	// Mail Actions
	ActionMailDone        ShortcutAction = "mail.done"
	ActionMailUndone      ShortcutAction = "mail.undone"
	ActionMailSnooze      ShortcutAction = "mail.snooze"
	ActionMailStar        ShortcutAction = "mail.star"
	ActionMailReadToggle  ShortcutAction = "mail.read_toggle"
	ActionMailTrash       ShortcutAction = "mail.trash"
	ActionMailSpam        ShortcutAction = "mail.spam"
	ActionMailMute        ShortcutAction = "mail.mute"
	ActionMailUnsubscribe ShortcutAction = "mail.unsubscribe"
	ActionMailUndo        ShortcutAction = "mail.undo"

	// Mail Compose
	ActionMailCompose  ShortcutAction = "mail.compose"
	ActionMailReplyAll ShortcutAction = "mail.reply_all"
	ActionMailReply    ShortcutAction = "mail.reply"
	ActionMailForward  ShortcutAction = "mail.forward"
	ActionMailSend     ShortcutAction = "mail.send"
	ActionMailSendDone ShortcutAction = "mail.send_done"

	// Mail Labels
	ActionMailLabel       ShortcutAction = "mail.label"
	ActionMailLabelRemove ShortcutAction = "mail.label_remove"
	ActionMailMove        ShortcutAction = "mail.move"

	// Mail Selection
	ActionMailSelect    ShortcutAction = "mail.select"
	ActionMailSelectAll ShortcutAction = "mail.select_all"

	// Navigation
	ActionNavInbox     ShortcutAction = "nav.inbox"
	ActionNavStarred   ShortcutAction = "nav.starred"
	ActionNavSent      ShortcutAction = "nav.sent"
	ActionNavDone      ShortcutAction = "nav.done"
	ActionNavReminders ShortcutAction = "nav.reminders"
	ActionNavDrafts    ShortcutAction = "nav.drafts"
	ActionNavSpam      ShortcutAction = "nav.spam"
	ActionNavTrash     ShortcutAction = "nav.trash"
	ActionNavAll       ShortcutAction = "nav.all"
	ActionNavLabel     ShortcutAction = "nav.label"
	ActionNavCalendar  ShortcutAction = "nav.calendar"
	ActionNavContacts  ShortcutAction = "nav.contacts"

	// Filters
	ActionFilterUnread    ShortcutAction = "filter.unread"
	ActionFilterStarred   ShortcutAction = "filter.starred"
	ActionFilterImportant ShortcutAction = "filter.important"
	ActionFilterNoReply   ShortcutAction = "filter.no_reply"

	// Calendar
	ActionCalendarToggle ShortcutAction = "calendar.toggle"
	ActionCalendarPrev   ShortcutAction = "calendar.prev"
	ActionCalendarNext   ShortcutAction = "calendar.next"
	ActionCalendarToday  ShortcutAction = "calendar.today"
	ActionCalendarDay    ShortcutAction = "calendar.day"
	ActionCalendarWeek   ShortcutAction = "calendar.week"
	ActionCalendarMonth  ShortcutAction = "calendar.month"
	ActionCalendarCreate ShortcutAction = "calendar.create"

	// Global
	ActionGlobalCommand ShortcutAction = "global.command"
	ActionGlobalSearch  ShortcutAction = "global.search"
	ActionGlobalHelp    ShortcutAction = "global.help"
	ActionGlobalEscape  ShortcutAction = "global.escape"

	// Proposal
	ActionProposalConfirm ShortcutAction = "proposal.confirm"
	ActionProposalReject  ShortcutAction = "proposal.reject"

	// Format
	ActionFormatBold          ShortcutAction = "format.bold"
	ActionFormatItalic        ShortcutAction = "format.italic"
	ActionFormatUnderline     ShortcutAction = "format.underline"
	ActionFormatLink          ShortcutAction = "format.link"
	ActionFormatStrikethrough ShortcutAction = "format.strikethrough"
	ActionFormatNumbered      ShortcutAction = "format.numbered"
	ActionFormatBullet        ShortcutAction = "format.bullet"
	ActionFormatQuote         ShortcutAction = "format.quote"
)

// DefaultSuperhumanShortcuts returns Superhuman-style shortcuts
func DefaultSuperhumanShortcuts() map[string]string {
	return map[string]string{
		// Mail Navigation
		"mail.next":       "j",
		"mail.prev":       "k",
		"mail.open":       "Enter",
		"mail.back":       "Escape",
		"mail.expand":     "o",
		"mail.expand_all": "Shift+o",

		// Mail Actions
		"mail.done":        "e",
		"mail.undone":      "Shift+e",
		"mail.snooze":      "h",
		"mail.star":        "s",
		"mail.read_toggle": "u",
		"mail.trash":       "#",
		"mail.spam":        "!",
		"mail.mute":        "Shift+m",
		"mail.unsubscribe": "Cmd+u",
		"mail.undo":        "z",

		// Mail Compose
		"mail.compose":   "c",
		"mail.reply_all": "Enter",
		"mail.reply":     "r",
		"mail.forward":   "f",
		"mail.send":      "Cmd+Enter",
		"mail.send_done": "Shift+Cmd+Enter",

		// Mail Labels
		"mail.label":        "l",
		"mail.label_remove": "y",
		"mail.move":         "v",

		// Mail Selection
		"mail.select":     "x",
		"mail.select_all": "Cmd+a",

		// Navigation
		"nav.inbox":     "g i",
		"nav.starred":   "g s",
		"nav.sent":      "g t",
		"nav.done":      "g e",
		"nav.reminders": "g h",
		"nav.drafts":    "g d",
		"nav.spam":      "g !",
		"nav.trash":     "g #",
		"nav.all":       "g a",
		"nav.label":     "g l",
		"nav.calendar":  "g c",
		"nav.contacts":  "g p",

		// Filters
		"filter.unread":    "Shift+u",
		"filter.starred":   "Shift+s",
		"filter.important": "Shift+i",
		"filter.no_reply":  "Shift+r",

		// Calendar
		"calendar.toggle": "0",
		"calendar.prev":   "-",
		"calendar.next":   "=",
		"calendar.today":  "t",
		"calendar.day":    "d",
		"calendar.week":   "w",
		"calendar.month":  "m",
		"calendar.create": "b",

		// Global
		"global.command": "Cmd+k",
		"global.search":  "/",
		"global.help":    "?",
		"global.escape":  "Escape",

		// Proposal
		"proposal.confirm": "y",
		"proposal.reject":  "n",

		// Format
		"format.bold":          "Cmd+b",
		"format.italic":        "Cmd+i",
		"format.underline":     "Cmd+u",
		"format.link":          "Cmd+k",
		"format.strikethrough": "Shift+Cmd+x",
		"format.numbered":      "Shift+Cmd+7",
		"format.bullet":        "Shift+Cmd+8",
		"format.quote":         "Shift+Cmd+9",
	}
}

// DefaultGmailShortcuts returns Gmail-style shortcuts
func DefaultGmailShortcuts() map[string]string {
	return map[string]string{
		// Mail Navigation
		"mail.next":       "j",
		"mail.prev":       "k",
		"mail.open":       "o",
		"mail.back":       "u",
		"mail.expand":     "o",
		"mail.expand_all": ";",

		// Mail Actions
		"mail.done":        "e",
		"mail.undone":      "Shift+e",
		"mail.snooze":      "b",
		"mail.star":        "s",
		"mail.read_toggle": "Shift+i",
		"mail.trash":       "#",
		"mail.spam":        "!",
		"mail.mute":        "m",
		"mail.undo":        "z",

		// Mail Compose
		"mail.compose":   "c",
		"mail.reply_all": "a",
		"mail.reply":     "r",
		"mail.forward":   "f",
		"mail.send":      "Cmd+Enter",
		"mail.send_done": "Cmd+Enter",

		// Mail Labels
		"mail.label":        "l",
		"mail.label_remove": "y",
		"mail.move":         "v",

		// Mail Selection
		"mail.select":     "x",
		"mail.select_all": "*+a",

		// Navigation
		"nav.inbox":    "g i",
		"nav.starred":  "g s",
		"nav.sent":     "g t",
		"nav.done":     "g e",
		"nav.drafts":   "g d",
		"nav.spam":     "g !",
		"nav.trash":    "g #",
		"nav.all":      "g a",
		"nav.label":    "g l",
		"nav.calendar": "g c",
		"nav.contacts": "g p",

		// Global
		"global.search": "/",
		"global.help":   "?",
		"global.escape": "Escape",

		// Format
		"format.bold":      "Cmd+b",
		"format.italic":    "Cmd+i",
		"format.underline": "Cmd+u",
		"format.link":      "Cmd+k",
	}
}

// GetDefaultShortcuts returns default shortcuts for given preset
func GetDefaultShortcuts(preset ShortcutPreset) map[string]string {
	switch preset {
	case PresetGmail:
		return DefaultGmailShortcuts()
	case PresetSuperhuman, PresetCustom:
		return DefaultSuperhumanShortcuts()
	default:
		return DefaultSuperhumanShortcuts()
	}
}

// ShortcutDefinition describes a shortcut action
type ShortcutDefinition struct {
	Action      string `json:"action"`
	Key         string `json:"key"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// GetShortcutDefinitions returns all shortcut definitions with descriptions
func GetShortcutDefinitions() []ShortcutDefinition {
	return []ShortcutDefinition{
		// Mail Navigation
		{Action: "mail.next", Description: "Next conversation", Category: "mail"},
		{Action: "mail.prev", Description: "Previous conversation", Category: "mail"},
		{Action: "mail.open", Description: "Open conversation", Category: "mail"},
		{Action: "mail.back", Description: "Back to list", Category: "mail"},
		{Action: "mail.expand", Description: "Expand message", Category: "mail"},
		{Action: "mail.expand_all", Description: "Expand all messages", Category: "mail"},

		// Mail Actions
		{Action: "mail.done", Description: "Mark done (archive)", Category: "mail"},
		{Action: "mail.undone", Description: "Mark not done", Category: "mail"},
		{Action: "mail.snooze", Description: "Snooze / Remind me", Category: "mail"},
		{Action: "mail.star", Description: "Star / Unstar", Category: "mail"},
		{Action: "mail.read_toggle", Description: "Toggle read/unread", Category: "mail"},
		{Action: "mail.trash", Description: "Move to trash", Category: "mail"},
		{Action: "mail.spam", Description: "Mark as spam", Category: "mail"},
		{Action: "mail.mute", Description: "Mute conversation", Category: "mail"},
		{Action: "mail.unsubscribe", Description: "Unsubscribe", Category: "mail"},
		{Action: "mail.undo", Description: "Undo last action", Category: "mail"},

		// Mail Compose
		{Action: "mail.compose", Description: "Compose new email", Category: "compose"},
		{Action: "mail.reply_all", Description: "Reply all", Category: "compose"},
		{Action: "mail.reply", Description: "Reply", Category: "compose"},
		{Action: "mail.forward", Description: "Forward", Category: "compose"},
		{Action: "mail.send", Description: "Send email", Category: "compose"},
		{Action: "mail.send_done", Description: "Send and mark done", Category: "compose"},

		// Mail Labels
		{Action: "mail.label", Description: "Add/remove label", Category: "label"},
		{Action: "mail.label_remove", Description: "Remove label", Category: "label"},
		{Action: "mail.move", Description: "Move to folder", Category: "label"},

		// Mail Selection
		{Action: "mail.select", Description: "Select conversation", Category: "selection"},
		{Action: "mail.select_all", Description: "Select all", Category: "selection"},

		// Navigation
		{Action: "nav.inbox", Description: "Go to Inbox", Category: "navigation"},
		{Action: "nav.starred", Description: "Go to Starred", Category: "navigation"},
		{Action: "nav.sent", Description: "Go to Sent", Category: "navigation"},
		{Action: "nav.done", Description: "Go to Done/Archive", Category: "navigation"},
		{Action: "nav.reminders", Description: "Go to Reminders", Category: "navigation"},
		{Action: "nav.drafts", Description: "Go to Drafts", Category: "navigation"},
		{Action: "nav.spam", Description: "Go to Spam", Category: "navigation"},
		{Action: "nav.trash", Description: "Go to Trash", Category: "navigation"},
		{Action: "nav.all", Description: "Go to All Mail", Category: "navigation"},
		{Action: "nav.label", Description: "Go to Label", Category: "navigation"},
		{Action: "nav.calendar", Description: "Go to Calendar", Category: "navigation"},
		{Action: "nav.contacts", Description: "Go to Contacts", Category: "navigation"},

		// Filters
		{Action: "filter.unread", Description: "Filter unread", Category: "filter"},
		{Action: "filter.starred", Description: "Filter starred", Category: "filter"},
		{Action: "filter.important", Description: "Filter important", Category: "filter"},
		{Action: "filter.no_reply", Description: "Filter no reply", Category: "filter"},

		// Calendar
		{Action: "calendar.toggle", Description: "Toggle calendar", Category: "calendar"},
		{Action: "calendar.prev", Description: "Previous day", Category: "calendar"},
		{Action: "calendar.next", Description: "Next day", Category: "calendar"},
		{Action: "calendar.today", Description: "Go to today", Category: "calendar"},
		{Action: "calendar.day", Description: "Day view", Category: "calendar"},
		{Action: "calendar.week", Description: "Week view", Category: "calendar"},
		{Action: "calendar.month", Description: "Month view", Category: "calendar"},
		{Action: "calendar.create", Description: "Create event", Category: "calendar"},

		// Global
		{Action: "global.command", Description: "Command palette", Category: "global"},
		{Action: "global.search", Description: "Search", Category: "global"},
		{Action: "global.help", Description: "Show shortcuts", Category: "global"},
		{Action: "global.escape", Description: "Close / Cancel", Category: "global"},

		// Proposal
		{Action: "proposal.confirm", Description: "Confirm proposal", Category: "proposal"},
		{Action: "proposal.reject", Description: "Reject proposal", Category: "proposal"},

		// Format
		{Action: "format.bold", Description: "Bold", Category: "format"},
		{Action: "format.italic", Description: "Italic", Category: "format"},
		{Action: "format.underline", Description: "Underline", Category: "format"},
		{Action: "format.link", Description: "Insert link", Category: "format"},
		{Action: "format.strikethrough", Description: "Strikethrough", Category: "format"},
		{Action: "format.numbered", Description: "Numbered list", Category: "format"},
		{Action: "format.bullet", Description: "Bullet list", Category: "format"},
		{Action: "format.quote", Description: "Quote", Category: "format"},
	}
}
