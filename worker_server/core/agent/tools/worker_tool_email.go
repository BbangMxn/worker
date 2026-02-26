package tools

import (
	"context"
	"fmt"
	"time"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

// MailListTool lists emails from mailbox
type MailListTool struct {
	emailRepo domain.EmailRepository
}

func NewMailListTool(emailRepo domain.EmailRepository) *MailListTool {
	return &MailListTool{emailRepo: emailRepo}
}

func (t *MailListTool) Name() string           { return "mail.list" }
func (t *MailListTool) Category() ToolCategory { return CategoryMail }

func (t *MailListTool) Description() string {
	return "List emails from the user's mailbox. Can filter by folder, provider, read status, and search query."
}

func (t *MailListTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "folder", Type: "string", Description: "Folder: inbox, sent, drafts, trash, archive", Enum: []string{"inbox", "sent", "drafts", "trash", "archive"}, Default: "inbox"},
		{Name: "provider", Type: "string", Description: "Email provider: gmail, outlook, or all", Enum: []string{"gmail", "outlook", "all"}, Default: "all"},
		{Name: "is_read", Type: "boolean", Description: "Filter by read status"},
		{Name: "is_starred", Type: "boolean", Description: "Filter starred emails only"},
		{Name: "search", Type: "string", Description: "Search query for subject/body"},
		{Name: "from_email", Type: "string", Description: "Filter by sender email"},
		{Name: "limit", Type: "number", Description: "Maximum emails to return", Default: 20},
	}
}

func (t *MailListTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	folder := getStringArg(args, "folder", "inbox")
	limit := getIntArg(args, "limit", 20)
	search := getStringArg(args, "search", "")
	fromEmail := getStringArg(args, "from_email", "")

	f := domain.LegacyFolder(folder)
	filter := &domain.EmailFilter{
		UserID: userID,
		Folder: &f,
		Limit:  limit,
	}

	if search != "" {
		filter.Search = &search
	}
	if fromEmail != "" {
		filter.FromEmail = &fromEmail
	}
	if isRead, ok := args["is_read"].(bool); ok {
		filter.IsRead = &isRead
	}
	if isStarred, ok := args["is_starred"].(bool); ok {
		filter.IsStarred = &isStarred
	}

	emails, total, err := t.emailRepo.List(filter)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	// Return summary for AI context
	summaries := make([]map[string]any, len(emails))
	for i, e := range emails {
		summaries[i] = map[string]any{
			"id":         e.ID,
			"subject":    e.Subject,
			"from":       e.FromEmail,
			"date":       e.Date.Format("2006-01-02 15:04"),
			"is_read":    e.IsRead,
			"is_starred": e.IsStarred,
			"folder":     e.Folder,
			"summary":    e.AISummary,
		}
	}

	return &ToolResult{
		Success: true,
		Data:    summaries,
		Message: fmt.Sprintf("Found %d emails (showing %d)", total, len(emails)),
	}, nil
}

// MailReadTool reads a specific email
type MailReadTool struct {
	emailRepo domain.EmailRepository
}

func NewMailReadTool(emailRepo domain.EmailRepository) *MailReadTool {
	return &MailReadTool{emailRepo: emailRepo}
}

func (t *MailReadTool) Name() string           { return "mail.read" }
func (t *MailReadTool) Category() ToolCategory { return CategoryMail }

func (t *MailReadTool) Description() string {
	return "Read a specific email by ID to get full content including body."
}

func (t *MailReadTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "email_id", Type: "number", Description: "Email ID to read", Required: true},
	}
}

func (t *MailReadTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	emailID := int64(getIntArg(args, "email_id", 0))
	if emailID == 0 {
		return &ToolResult{Success: false, Error: "email_id is required"}, nil
	}

	email, err := t.emailRepo.GetByID(emailID)
	if err != nil {
		return &ToolResult{Success: false, Error: "email not found"}, nil
	}

	// Verify ownership
	if email.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	// Get body
	body, _ := t.emailRepo.GetBody(emailID)
	bodyText := ""
	if body != nil {
		bodyText = body.TextBody
	}

	result := map[string]any{
		"id":          email.ID,
		"subject":     email.Subject,
		"from":        email.FromEmail,
		"from_name":   email.FromName,
		"to":          email.ToEmails,
		"cc":          email.CcEmails,
		"date":        email.Date,
		"body":        bodyText,
		"folder":      email.Folder,
		"is_read":     email.IsRead,
		"is_starred":  email.IsStarred,
		"labels":      email.Labels,
		"ai_category": email.AICategory,
		"ai_priority": email.AIPriority,
		"ai_summary":  email.AISummary,
	}

	return &ToolResult{
		Success: true,
		Data:    result,
	}, nil
}

// MailSearchTool searches emails
type MailSearchTool struct {
	emailRepo domain.EmailRepository
}

func NewMailSearchTool(emailRepo domain.EmailRepository) *MailSearchTool {
	return &MailSearchTool{emailRepo: emailRepo}
}

func (t *MailSearchTool) Name() string           { return "mail.search" }
func (t *MailSearchTool) Category() ToolCategory { return CategoryMail }

func (t *MailSearchTool) Description() string {
	return "Search emails by keywords, sender, date range, or other criteria."
}

func (t *MailSearchTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "query", Type: "string", Description: "Search keywords", Required: true},
		{Name: "from", Type: "string", Description: "Filter by sender"},
		{Name: "date_from", Type: "string", Description: "Start date (YYYY-MM-DD)"},
		{Name: "date_to", Type: "string", Description: "End date (YYYY-MM-DD)"},
		{Name: "folder", Type: "string", Description: "Search in specific folder"},
		{Name: "limit", Type: "number", Description: "Maximum results", Default: 10},
	}
}

func (t *MailSearchTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	query := getStringArg(args, "query", "")
	from := getStringArg(args, "from", "")
	dateFrom := getStringArg(args, "date_from", "")
	dateTo := getStringArg(args, "date_to", "")
	folder := getStringArg(args, "folder", "")
	limit := getIntArg(args, "limit", 10)

	filter := &domain.EmailFilter{
		UserID: userID,
		Search: &query,
		Limit:  limit,
	}

	if from != "" {
		filter.FromEmail = &from
	}
	if folder != "" {
		f := domain.LegacyFolder(folder)
		filter.Folder = &f
	}
	if dateFrom != "" {
		if t, err := time.Parse("2006-01-02", dateFrom); err == nil {
			filter.DateFrom = &t
		}
	}
	if dateTo != "" {
		if t, err := time.Parse("2006-01-02", dateTo); err == nil {
			filter.DateTo = &t
		}
	}

	emails, total, err := t.emailRepo.List(filter)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	summaries := make([]map[string]any, len(emails))
	for i, e := range emails {
		summaries[i] = map[string]any{
			"id":      e.ID,
			"subject": e.Subject,
			"from":    e.FromEmail,
			"date":    e.Date.Format("2006-01-02"),
			"folder":  e.Folder,
			"summary": e.AISummary,
		}
	}

	return &ToolResult{
		Success: true,
		Data:    summaries,
		Message: fmt.Sprintf("Found %d emails matching '%s'", total, query),
	}, nil
}

// MailSendTool prepares an email for sending (returns proposal)
type MailSendTool struct {
	// Dependencies would be injected
}

func NewMailSendTool() *MailSendTool {
	return &MailSendTool{}
}

func (t *MailSendTool) Name() string           { return "mail.send" }
func (t *MailSendTool) Category() ToolCategory { return CategoryMail }

func (t *MailSendTool) Description() string {
	return "Prepare an email to send. Returns a proposal for user confirmation before actually sending."
}

func (t *MailSendTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "to", Type: "array", Description: "Recipient email addresses", Required: true},
		{Name: "subject", Type: "string", Description: "Email subject", Required: true},
		{Name: "body", Type: "string", Description: "Email body", Required: true},
		{Name: "provider", Type: "string", Description: "Send from: gmail or outlook", Enum: []string{"gmail", "outlook"}, Required: true},
		{Name: "cc", Type: "array", Description: "CC recipients"},
		{Name: "is_html", Type: "boolean", Description: "Body is HTML", Default: false},
	}
}

func (t *MailSendTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	to := getStringArrayArg(args, "to")
	subject := getStringArg(args, "subject", "")
	body := getStringArg(args, "body", "")
	provider := getStringArg(args, "provider", "")
	cc := getStringArrayArg(args, "cc")
	isHTML := getBoolArg(args, "is_html", false)

	if len(to) == 0 || subject == "" || body == "" {
		return &ToolResult{Success: false, Error: "to, subject, and body are required"}, nil
	}

	// Create proposal for confirmation
	proposal := &ActionProposal{
		ID:          uuid.New().String(),
		Action:      "mail.send",
		Description: fmt.Sprintf("Send email to %v: '%s'", to, subject),
		Data: map[string]any{
			"to":       to,
			"cc":       cc,
			"subject":  subject,
			"body":     body,
			"provider": provider,
			"is_html":  isHTML,
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  fmt.Sprintf("Ready to send email '%s' to %v via %s", subject, to, provider),
		Proposal: proposal,
	}, nil
}

// MailTranslateTool translates email content
type MailTranslateTool struct {
	emailRepo domain.EmailRepository
	llmClient LLMTranslator
}

// LLMTranslator interface for translation
type LLMTranslator interface {
	TranslateEmail(ctx context.Context, subject, body, targetLang string) (string, string, error)
}

func NewMailTranslateTool(emailRepo domain.EmailRepository, llmClient LLMTranslator) *MailTranslateTool {
	return &MailTranslateTool{emailRepo: emailRepo, llmClient: llmClient}
}

func (t *MailTranslateTool) Name() string           { return "mail.translate" }
func (t *MailTranslateTool) Category() ToolCategory { return CategoryMail }

func (t *MailTranslateTool) Description() string {
	return "Translate an email's subject and body to a target language."
}

func (t *MailTranslateTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "email_id", Type: "number", Description: "Email ID to translate", Required: true},
		{Name: "target_language", Type: "string", Description: "Target language (e.g., Korean, English, Japanese, Chinese)", Required: true},
	}
}

func (t *MailTranslateTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	emailID := int64(getIntArg(args, "email_id", 0))
	targetLang := getStringArg(args, "target_language", "")

	if emailID == 0 {
		return &ToolResult{Success: false, Error: "email_id is required"}, nil
	}
	if targetLang == "" {
		return &ToolResult{Success: false, Error: "target_language is required"}, nil
	}

	// Get email
	email, err := t.emailRepo.GetByID(emailID)
	if err != nil {
		return &ToolResult{Success: false, Error: "email not found"}, nil
	}

	// Verify ownership
	if email.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	// Get body
	body, _ := t.emailRepo.GetBody(emailID)
	bodyText := ""
	if body != nil {
		bodyText = body.TextBody
	}

	// Translate using LLM
	translatedSubject, translatedBody, err := t.llmClient.TranslateEmail(ctx, email.Subject, bodyText, targetLang)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("translation failed: %v", err)}, nil
	}

	result := map[string]any{
		"email_id":           emailID,
		"original_subject":   email.Subject,
		"original_body":      bodyText,
		"translated_subject": translatedSubject,
		"translated_body":    translatedBody,
		"target_language":    targetLang,
	}

	return &ToolResult{
		Success: true,
		Data:    result,
		Message: fmt.Sprintf("Email translated to %s", targetLang),
	}, nil
}

// MailReplyTool prepares a reply to an email
type MailReplyTool struct {
	emailRepo domain.EmailRepository
}

func NewMailReplyTool(emailRepo domain.EmailRepository) *MailReplyTool {
	return &MailReplyTool{emailRepo: emailRepo}
}

func (t *MailReplyTool) Name() string           { return "mail.reply" }
func (t *MailReplyTool) Category() ToolCategory { return CategoryMail }

func (t *MailReplyTool) Description() string {
	return "Prepare a reply to an existing email. Use with AI-generated reply content."
}

func (t *MailReplyTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "email_id", Type: "number", Description: "Original email ID to reply to", Required: true},
		{Name: "body", Type: "string", Description: "Reply body", Required: true},
		{Name: "reply_all", Type: "boolean", Description: "Reply to all recipients", Default: false},
	}
}

func (t *MailReplyTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	emailID := int64(getIntArg(args, "email_id", 0))
	body := getStringArg(args, "body", "")
	replyAll := getBoolArg(args, "reply_all", false)

	if emailID == 0 || body == "" {
		return &ToolResult{Success: false, Error: "email_id and body are required"}, nil
	}

	// Get original email
	original, err := t.emailRepo.GetByID(emailID)
	if err != nil {
		return &ToolResult{Success: false, Error: "original email not found"}, nil
	}

	// Verify ownership
	if original.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	// Build recipients
	to := []string{original.FromEmail}
	var cc []string
	if replyAll && len(original.ToEmails) > 0 {
		cc = original.ToEmails
	}

	proposal := &ActionProposal{
		ID:          uuid.New().String(),
		Action:      "mail.reply",
		Description: fmt.Sprintf("Reply to '%s' from %s", original.Subject, original.FromEmail),
		Data: map[string]any{
			"original_id": emailID,
			"to":          to,
			"cc":          cc,
			"subject":     "Re: " + original.Subject,
			"body":        body,
			"provider":    original.Provider,
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  fmt.Sprintf("Ready to reply to '%s'", original.Subject),
		Proposal: proposal,
	}, nil
}

// =============================================================================
// Label Tools
// =============================================================================

// LabelListTool lists all labels/folders for a user
type LabelListTool struct {
	labelRepo domain.LabelRepository
}

func NewLabelListTool(labelRepo domain.LabelRepository) *LabelListTool {
	return &LabelListTool{labelRepo: labelRepo}
}

func (t *LabelListTool) Name() string           { return "label.list" }
func (t *LabelListTool) Category() ToolCategory { return CategoryMail }

func (t *LabelListTool) Description() string {
	return "List all email labels/folders available for the user. Use this to see what labels exist before adding/removing."
}

func (t *LabelListTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "provider", Type: "string", Description: "Email provider: gmail, outlook, or all", Enum: []string{"gmail", "outlook", "all"}, Default: "all"},
	}
}

func (t *LabelListTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	// Get labels from repository
	labels, err := t.labelRepo.ListByUser(userID)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	// Format for AI context
	labelSummaries := make([]map[string]any, len(labels))
	for i, l := range labels {
		labelSummaries[i] = map[string]any{
			"id":          l.ID,
			"name":        l.Name,
			"color":       l.Color,
			"is_system":   l.IsSystem,
			"email_count": l.EmailCount,
		}
	}

	return &ToolResult{
		Success: true,
		Data:    labelSummaries,
		Message: fmt.Sprintf("Found %d labels", len(labels)),
	}, nil
}

// LabelAddTool adds a label to an email
type LabelAddTool struct {
	emailRepo domain.EmailRepository
	labelRepo domain.LabelRepository
}

func NewLabelAddTool(emailRepo domain.EmailRepository, labelRepo domain.LabelRepository) *LabelAddTool {
	return &LabelAddTool{emailRepo: emailRepo, labelRepo: labelRepo}
}

func (t *LabelAddTool) Name() string           { return "label.add" }
func (t *LabelAddTool) Category() ToolCategory { return CategoryMail }

func (t *LabelAddTool) Description() string {
	return "Add a label to an email. Use label.list first to see available labels."
}

func (t *LabelAddTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "email_id", Type: "number", Description: "Email ID to add label to", Required: true},
		{Name: "label_id", Type: "number", Description: "Label ID to add", Required: true},
	}
}

func (t *LabelAddTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	emailID := int64(getIntArg(args, "email_id", 0))
	labelID := int64(getIntArg(args, "label_id", 0))

	if emailID == 0 || labelID == 0 {
		return &ToolResult{Success: false, Error: "email_id and label_id are required"}, nil
	}

	// Get email to verify ownership
	email, err := t.emailRepo.GetByID(emailID)
	if err != nil {
		return &ToolResult{Success: false, Error: "email not found"}, nil
	}

	if email.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	// Get label to verify ownership and get name
	label, err := t.labelRepo.GetByID(labelID)
	if err != nil {
		return &ToolResult{Success: false, Error: "label not found"}, nil
	}

	if label.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	// Create proposal for adding label
	proposal := &ActionProposal{
		ID:          uuid.New().String(),
		Action:      "label.add",
		Description: fmt.Sprintf("Add label '%s' to email '%s'", label.Name, email.Subject),
		Data: map[string]any{
			"email_id":    emailID,
			"label_id":    labelID,
			"label_name":  label.Name,
			"provider":    email.Provider,
			"provider_id": email.ProviderID,
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  fmt.Sprintf("Ready to add label '%s' to email", label.Name),
		Proposal: proposal,
	}, nil
}

// LabelRemoveTool removes a label from an email
type LabelRemoveTool struct {
	emailRepo domain.EmailRepository
	labelRepo domain.LabelRepository
}

func NewLabelRemoveTool(emailRepo domain.EmailRepository, labelRepo domain.LabelRepository) *LabelRemoveTool {
	return &LabelRemoveTool{emailRepo: emailRepo, labelRepo: labelRepo}
}

func (t *LabelRemoveTool) Name() string           { return "label.remove" }
func (t *LabelRemoveTool) Category() ToolCategory { return CategoryMail }

func (t *LabelRemoveTool) Description() string {
	return "Remove a label from an email."
}

func (t *LabelRemoveTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "email_id", Type: "number", Description: "Email ID to remove label from", Required: true},
		{Name: "label_id", Type: "number", Description: "Label ID to remove", Required: true},
	}
}

func (t *LabelRemoveTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	emailID := int64(getIntArg(args, "email_id", 0))
	labelID := int64(getIntArg(args, "label_id", 0))

	if emailID == 0 || labelID == 0 {
		return &ToolResult{Success: false, Error: "email_id and label_id are required"}, nil
	}

	// Get email to verify ownership
	email, err := t.emailRepo.GetByID(emailID)
	if err != nil {
		return &ToolResult{Success: false, Error: "email not found"}, nil
	}

	if email.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	// Get label to verify ownership and get name
	label, err := t.labelRepo.GetByID(labelID)
	if err != nil {
		return &ToolResult{Success: false, Error: "label not found"}, nil
	}

	if label.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	// Create proposal for removing label
	proposal := &ActionProposal{
		ID:          uuid.New().String(),
		Action:      "label.remove",
		Description: fmt.Sprintf("Remove label '%s' from email '%s'", label.Name, email.Subject),
		Data: map[string]any{
			"email_id":    emailID,
			"label_id":    labelID,
			"label_name":  label.Name,
			"provider":    email.Provider,
			"provider_id": email.ProviderID,
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  fmt.Sprintf("Ready to remove label '%s' from email", label.Name),
		Proposal: proposal,
	}, nil
}

// LabelCreateTool creates a new label
type LabelCreateTool struct {
	labelRepo domain.LabelRepository
}

func NewLabelCreateTool(labelRepo domain.LabelRepository) *LabelCreateTool {
	return &LabelCreateTool{labelRepo: labelRepo}
}

func (t *LabelCreateTool) Name() string           { return "label.create" }
func (t *LabelCreateTool) Category() ToolCategory { return CategoryMail }

func (t *LabelCreateTool) Description() string {
	return "Create a new label/folder in the mailbox."
}

func (t *LabelCreateTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "name", Type: "string", Description: "Label name to create", Required: true},
		{Name: "color", Type: "string", Description: "Label color (optional)"},
		{Name: "provider", Type: "string", Description: "Email provider", Enum: []string{"gmail", "outlook"}, Required: true},
	}
}

func (t *LabelCreateTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	name := getStringArg(args, "name", "")
	color := getStringArg(args, "color", "")
	provider := getStringArg(args, "provider", "")

	if name == "" || provider == "" {
		return &ToolResult{Success: false, Error: "name and provider are required"}, nil
	}

	// Create proposal for creating label
	proposal := &ActionProposal{
		ID:          uuid.New().String(),
		Action:      "label.create",
		Description: fmt.Sprintf("Create new label '%s' in %s", name, provider),
		Data: map[string]any{
			"name":     name,
			"color":    color,
			"provider": provider,
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  fmt.Sprintf("Ready to create label '%s'", name),
		Proposal: proposal,
	}, nil
}

// =============================================================================
// Mail Action Tools (Destructive - Require Proposal)
// =============================================================================

// MailDeleteTool deletes an email (moves to trash or permanently deletes)
type MailDeleteTool struct {
	emailRepo domain.EmailRepository
}

func NewMailDeleteTool(emailRepo domain.EmailRepository) *MailDeleteTool {
	return &MailDeleteTool{emailRepo: emailRepo}
}

func (t *MailDeleteTool) Name() string           { return "mail.delete" }
func (t *MailDeleteTool) Category() ToolCategory { return CategoryMail }

func (t *MailDeleteTool) Description() string {
	return "Delete an email. Moves to trash by default, or permanently deletes if already in trash."
}

func (t *MailDeleteTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "email_id", Type: "number", Description: "Email ID to delete", Required: true},
		{Name: "permanent", Type: "boolean", Description: "Permanently delete (skip trash)", Default: false},
	}
}

func (t *MailDeleteTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	emailID := int64(getIntArg(args, "email_id", 0))
	permanent := getBoolArg(args, "permanent", false)

	if emailID == 0 {
		return &ToolResult{Success: false, Error: "email_id is required"}, nil
	}

	// Get email to verify ownership
	email, err := t.emailRepo.GetByID(emailID)
	if err != nil {
		return &ToolResult{Success: false, Error: "email not found"}, nil
	}

	if email.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	action := "mail.delete"
	description := fmt.Sprintf("Move email '%s' to trash", email.Subject)
	if permanent || email.Folder == domain.LegacyFolderTrash {
		description = fmt.Sprintf("Permanently delete email '%s'", email.Subject)
	}

	proposal := &ActionProposal{
		ID:          uuid.New().String(),
		Action:      action,
		Description: description,
		Data: map[string]any{
			"email_id":    emailID,
			"subject":     email.Subject,
			"provider":    email.Provider,
			"provider_id": email.ProviderID,
			"permanent":   permanent || email.Folder == domain.LegacyFolderTrash,
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  description,
		Proposal: proposal,
	}, nil
}

// MailArchiveTool archives an email
type MailArchiveTool struct {
	emailRepo domain.EmailRepository
}

func NewMailArchiveTool(emailRepo domain.EmailRepository) *MailArchiveTool {
	return &MailArchiveTool{emailRepo: emailRepo}
}

func (t *MailArchiveTool) Name() string           { return "mail.archive" }
func (t *MailArchiveTool) Category() ToolCategory { return CategoryMail }

func (t *MailArchiveTool) Description() string {
	return "Archive an email. Removes from inbox but keeps the email accessible."
}

func (t *MailArchiveTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "email_id", Type: "number", Description: "Email ID to archive", Required: true},
	}
}

func (t *MailArchiveTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	emailID := int64(getIntArg(args, "email_id", 0))

	if emailID == 0 {
		return &ToolResult{Success: false, Error: "email_id is required"}, nil
	}

	// Get email to verify ownership
	email, err := t.emailRepo.GetByID(emailID)
	if err != nil {
		return &ToolResult{Success: false, Error: "email not found"}, nil
	}

	if email.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	proposal := &ActionProposal{
		ID:          uuid.New().String(),
		Action:      "mail.archive",
		Description: fmt.Sprintf("Archive email '%s' from %s", email.Subject, email.FromEmail),
		Data: map[string]any{
			"email_id":    emailID,
			"subject":     email.Subject,
			"provider":    email.Provider,
			"provider_id": email.ProviderID,
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  fmt.Sprintf("Ready to archive email '%s'", email.Subject),
		Proposal: proposal,
	}, nil
}

// MailMarkReadTool marks an email as read or unread
type MailMarkReadTool struct {
	emailRepo domain.EmailRepository
}

func NewMailMarkReadTool(emailRepo domain.EmailRepository) *MailMarkReadTool {
	return &MailMarkReadTool{emailRepo: emailRepo}
}

func (t *MailMarkReadTool) Name() string           { return "mail.mark_read" }
func (t *MailMarkReadTool) Category() ToolCategory { return CategoryMail }

func (t *MailMarkReadTool) Description() string {
	return "Mark an email as read or unread."
}

func (t *MailMarkReadTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "email_id", Type: "number", Description: "Email ID to mark", Required: true},
		{Name: "is_read", Type: "boolean", Description: "Mark as read (true) or unread (false)", Required: true},
	}
}

func (t *MailMarkReadTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	emailID := int64(getIntArg(args, "email_id", 0))
	isRead := getBoolArg(args, "is_read", true)

	if emailID == 0 {
		return &ToolResult{Success: false, Error: "email_id is required"}, nil
	}

	// Get email to verify ownership
	email, err := t.emailRepo.GetByID(emailID)
	if err != nil {
		return &ToolResult{Success: false, Error: "email not found"}, nil
	}

	if email.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	status := "read"
	if !isRead {
		status = "unread"
	}

	proposal := &ActionProposal{
		ID:          uuid.New().String(),
		Action:      "mail.mark_read",
		Description: fmt.Sprintf("Mark email '%s' as %s", email.Subject, status),
		Data: map[string]any{
			"email_id":    emailID,
			"is_read":     isRead,
			"provider":    email.Provider,
			"provider_id": email.ProviderID,
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  fmt.Sprintf("Ready to mark email as %s", status),
		Proposal: proposal,
	}, nil
}

// MailStarTool stars or unstars an email
type MailStarTool struct {
	emailRepo domain.EmailRepository
}

func NewMailStarTool(emailRepo domain.EmailRepository) *MailStarTool {
	return &MailStarTool{emailRepo: emailRepo}
}

func (t *MailStarTool) Name() string           { return "mail.star" }
func (t *MailStarTool) Category() ToolCategory { return CategoryMail }

func (t *MailStarTool) Description() string {
	return "Star or unstar an email to mark it as important."
}

func (t *MailStarTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "email_id", Type: "number", Description: "Email ID to star/unstar", Required: true},
		{Name: "starred", Type: "boolean", Description: "Star (true) or unstar (false)", Required: true},
	}
}

func (t *MailStarTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	emailID := int64(getIntArg(args, "email_id", 0))
	starred := getBoolArg(args, "starred", true)

	if emailID == 0 {
		return &ToolResult{Success: false, Error: "email_id is required"}, nil
	}

	// Get email to verify ownership
	email, err := t.emailRepo.GetByID(emailID)
	if err != nil {
		return &ToolResult{Success: false, Error: "email not found"}, nil
	}

	if email.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	action := "star"
	if !starred {
		action = "unstar"
	}

	proposal := &ActionProposal{
		ID:          uuid.New().String(),
		Action:      "mail.star",
		Description: fmt.Sprintf("%s email '%s'", action, email.Subject),
		Data: map[string]any{
			"email_id":    emailID,
			"starred":     starred,
			"provider":    email.Provider,
			"provider_id": email.ProviderID,
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  fmt.Sprintf("Ready to %s email", action),
		Proposal: proposal,
	}, nil
}

// =============================================================================
// Mail AI Tools (Read-only)
// =============================================================================

// MailSummarizeTool summarizes an email using AI in user's language
type MailSummarizeTool struct {
	emailRepo    domain.EmailRepository
	settingsRepo domain.SettingsRepository
	llmClient    LLMSummarizer
}

// LLMSummarizer interface for LLM summarization
type LLMSummarizer interface {
	SummarizeEmailWithLang(ctx context.Context, subject, body, language string) (string, error)
}

func NewMailSummarizeTool(emailRepo domain.EmailRepository, settingsRepo domain.SettingsRepository, llmClient LLMSummarizer) *MailSummarizeTool {
	return &MailSummarizeTool{
		emailRepo:    emailRepo,
		settingsRepo: settingsRepo,
		llmClient:    llmClient,
	}
}

func (t *MailSummarizeTool) Name() string           { return "mail.summarize" }
func (t *MailSummarizeTool) Category() ToolCategory { return CategoryMail }

func (t *MailSummarizeTool) Description() string {
	return "Summarize an email in the user's preferred language. Use this when user asks to summarize an email."
}

func (t *MailSummarizeTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "email_id", Type: "number", Description: "Email ID to summarize", Required: true},
	}
}

func (t *MailSummarizeTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	emailID := int64(getIntArg(args, "email_id", 0))

	if emailID == 0 {
		return &ToolResult{Success: false, Error: "email_id is required"}, nil
	}

	// Get email to verify ownership
	email, err := t.emailRepo.GetByID(emailID)
	if err != nil {
		return &ToolResult{Success: false, Error: "email not found"}, nil
	}

	if email.UserID != userID {
		return &ToolResult{Success: false, Error: "unauthorized"}, nil
	}

	// Get email body
	body, _ := t.emailRepo.GetBody(emailID)
	bodyText := ""
	if body != nil {
		bodyText = body.TextBody
	}

	if bodyText == "" {
		return &ToolResult{Success: false, Error: "email body is empty"}, nil
	}

	// Get user's language setting
	language := "en" // default
	if t.settingsRepo != nil {
		settings, err := t.settingsRepo.GetByUserID(userID)
		if err == nil && settings != nil && settings.Language != "" {
			language = settings.Language
		}
	}

	// Summarize using LLM in user's language
	summary, err := t.llmClient.SummarizeEmailWithLang(ctx, email.Subject, bodyText, language)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("summarization failed: %v", err)}, nil
	}

	result := map[string]any{
		"email_id": emailID,
		"subject":  email.Subject,
		"from":     email.FromEmail,
		"date":     email.Date.Format("2006-01-02 15:04"),
		"summary":  summary,
		"language": language,
	}

	return &ToolResult{
		Success: true,
		Data:    result,
		Message: summary,
	}, nil
}
