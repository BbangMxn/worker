package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/goccy/go-json"

	"worker_server/core/agent/llm"
	"worker_server/core/agent/rag"
	"worker_server/core/agent/tools"
	"worker_server/core/domain"
	"worker_server/core/port/in"
	"worker_server/core/service/classification"
	"worker_server/pkg/logger"

	"github.com/google/uuid"
)

var (
	ErrLLMNotConfigured   = errors.New("LLM client not configured")
	ErrRepoNotInitialized = errors.New("repository not initialized")
)

// =============================================================================
// AI 최적화 상수
// =============================================================================

// MeetingKeywords - 미팅 추출 전 키워드 체크 (불필요한 API 호출 방지)
var MeetingKeywords = []string{
	"meeting", "meet", "call", "zoom", "teams", "calendar", "schedule",
	"appointment", "conference", "미팅", "회의", "일정", "통화", "약속",
	"invite", "invited", "join", "참석", "참여",
}

// minSummarizeLen is local reference to avoid redeclaration
const minSummarizeLen = 200

type Service struct {
	emailRepo              domain.EmailRepository
	settingsRepo           domain.SettingsRepository
	llmClient              *llm.Client
	ragRetriever           *rag.Retriever
	ragIndexer             *rag.IndexerService
	toolRegistry           *tools.Registry
	classificationPipeline *classification.Pipeline
}

func NewService(
	emailRepo domain.EmailRepository,
	settingsRepo domain.SettingsRepository,
	llmClient *llm.Client,
	ragRetriever *rag.Retriever,
	ragIndexer *rag.IndexerService,
	toolRegistry *tools.Registry,
) *Service {
	return &Service{
		emailRepo:    emailRepo,
		settingsRepo: settingsRepo,
		llmClient:    llmClient,
		ragRetriever: ragRetriever,
		ragIndexer:   ragIndexer,
		toolRegistry: toolRegistry,
	}
}

// SetClassificationPipeline sets the classification pipeline for 4-stage classification.
func (s *Service) SetClassificationPipeline(pipeline *classification.Pipeline) {
	s.classificationPipeline = pipeline
}

// ClassifyEmail classifies an email using the 4-stage classification pipeline.
// Stage 0: User Rules → Stage 1: Headers → Stage 2: Domain → Stage 3: LLM
// This saves ~75% of LLM API costs.
func (s *Service) ClassifyEmail(ctx context.Context, emailID int64) (*domain.ClassificationResult, error) {
	if s.emailRepo == nil {
		return nil, ErrRepoNotInitialized
	}

	// 1. Get email
	email, err := s.emailRepo.GetByID(emailID)
	if err != nil {
		return nil, fmt.Errorf("failed to get email: %w", err)
	}

	// 2. Get email body (from cache/mongodb)
	body := ""
	if emailBody, err := s.emailRepo.GetBody(emailID); err == nil && emailBody != nil {
		body = emailBody.TextBody
	}

	// 3. Use 4-stage classification pipeline if available
	if s.classificationPipeline != nil {
		// Build classification input
		input := &classification.ClassifyInput{
			UserID:  email.UserID,
			Email:   email,
			Headers: nil, // Headers will be populated during sync from provider
			Body:    body,
		}

		pipelineResult, err := s.classificationPipeline.Classify(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("classification pipeline failed: %w", err)
		}

		// Update sender profile for learning
		s.classificationPipeline.UpdateSenderProfile(ctx, email.UserID, email, pipelineResult)

		// Save classification result to email
		email.AICategory = &pipelineResult.Category
		email.AIPriority = &pipelineResult.Priority
		email.AISubCategory = pipelineResult.SubCategory
		email.AIScore = &pipelineResult.Confidence
		email.ClassificationSource = &pipelineResult.Source
		if err := s.emailRepo.Update(email); err != nil {
			logger.WithFields(map[string]any{"email_id": emailID, "error": err.Error()}).Warn("failed to save classification result")
		}

		// Convert pipeline result to domain result
		result := &domain.ClassificationResult{
			EmailID:     emailID,
			Category:    &pipelineResult.Category,
			Priority:    &pipelineResult.Priority,
			SubCategory: pipelineResult.SubCategory,
			Score:       pipelineResult.Confidence,
			Source:      pipelineResult.Source,
		}
		return result, nil
	}

	// Fallback: Direct LLM classification (if pipeline not configured)
	if s.llmClient == nil {
		return nil, ErrLLMNotConfigured
	}

	// Get user classification rules for LLM fallback
	var userRules []domain.ClassificationRule
	if s.settingsRepo != nil {
		rules, _ := s.settingsRepo.GetClassificationRules(ctx, email.UserID)
		if rules != nil {
			userRules = rules.ToRuleList()
		}
	}

	llmResult, err := s.llmClient.ClassifyEmail(ctx, email.Subject, body, email.FromEmail, userRules)
	if err != nil {
		return nil, fmt.Errorf("llm classification failed: %w", err)
	}

	category := domain.EmailCategory(llmResult.Category)
	priority := domain.Priority(llmResult.Priority)
	source := domain.ClassificationSourceLLM

	// Save classification result to email
	email.AICategory = &category
	email.AIPriority = &priority
	email.AITags = llmResult.Tags
	email.AIScore = &llmResult.Score
	email.ClassificationSource = &source
	if err := s.emailRepo.Update(email); err != nil {
		logger.WithFields(map[string]any{"email_id": emailID}).WithError(err).Warn("failed to save classification result")
	}

	return &domain.ClassificationResult{
		EmailID:  emailID,
		Category: &category,
		Priority: &priority,
		Tags:     llmResult.Tags,
		Score:    llmResult.Score,
		Source:   source,
	}, nil
}

// ClassifyEmailBatch classifies multiple emails with concurrency control
func (s *Service) ClassifyEmailBatch(ctx context.Context, emailIDs []int64) ([]*domain.ClassificationResult, error) {
	if s.emailRepo == nil {
		return nil, ErrRepoNotInitialized
	}
	if s.llmClient == nil {
		return nil, ErrLLMNotConfigured
	}

	results := make([]*domain.ClassificationResult, 0, len(emailIDs))

	// Process with concurrency limit (5 concurrent)
	sem := make(chan struct{}, 5)
	resultCh := make(chan *domain.ClassificationResult, len(emailIDs))
	errCh := make(chan error, len(emailIDs))

	for _, id := range emailIDs {
		sem <- struct{}{}
		go func(emailID int64) {
			defer func() { <-sem }()
			result, err := s.ClassifyEmail(ctx, emailID)
			if err != nil {
				errCh <- err
				return
			}
			resultCh <- result
		}(id)
	}

	// Collect results
	for i := 0; i < len(emailIDs); i++ {
		select {
		case result := <-resultCh:
			if result != nil {
				results = append(results, result)
			}
		case <-errCh:
			// Log error but continue
		}
	}

	return results, nil
}

// SummarizeEmail generates a summary for an email
// force=true: API 요청 시 길이 관계없이 AI 실행
// force=false: 자동 처리 시 200자 미만은 본문 자체를 반환 (API 비용 절감)
func (s *Service) SummarizeEmail(ctx context.Context, emailID int64, force bool) (string, error) {
	if s.emailRepo == nil {
		return "", ErrRepoNotInitialized
	}
	if s.llmClient == nil {
		return "", ErrLLMNotConfigured
	}

	email, err := s.emailRepo.GetByID(emailID)
	if err != nil {
		return "", err
	}

	// 이미 요약이 있고 강제 실행이 아니면 캐시된 결과 반환
	if !force && email.AISummary != nil && *email.AISummary != "" {
		return *email.AISummary, nil
	}

	body := ""
	if emailBody, err := s.emailRepo.GetBody(emailID); err == nil && emailBody != nil {
		body = llm.CleanEmailBody(emailBody.TextBody)
	}

	// 자동 처리 시에만 길이 체크 (force=false)
	contentLength := len(email.Subject) + len(body)
	if !force && contentLength < minSummarizeLen {
		summary := body
		if summary == "" {
			summary = email.Subject
		}
		// DB에 저장
		email.AISummary = &summary
		s.emailRepo.Update(email)
		return summary, nil
	}

	// API 호출
	summary, err := s.llmClient.SummarizeEmail(ctx, email.Subject, body)
	if err != nil {
		return "", err
	}

	// DB에 저장
	email.AISummary = &summary
	s.emailRepo.Update(email)

	return summary, nil
}

// SummarizeEmailDirect generates a summary using provided subject and body directly
// force=true: API 요청 시 길이 관계없이 AI 실행
// force=false: 자동 처리 시 200자 미만은 본문 자체를 반환 (API 비용 절감)
func (s *Service) SummarizeEmailDirect(ctx context.Context, subject, body string, force bool) (string, error) {
	if s.llmClient == nil {
		return "", ErrLLMNotConfigured
	}

	cleanBody := llm.CleanEmailBody(body)

	// 자동 처리 시에만 길이 체크 (force=false)
	contentLength := len(subject) + len(cleanBody)
	if !force && contentLength < minSummarizeLen {
		if cleanBody != "" {
			return cleanBody, nil
		}
		return subject, nil
	}

	return s.llmClient.SummarizeEmail(ctx, subject, cleanBody)
}

// SummarizeThread generates a summary for an email thread
func (s *Service) SummarizeThread(ctx context.Context, threadID string) (string, error) {
	if s.emailRepo == nil {
		return "", ErrRepoNotInitialized
	}
	if s.llmClient == nil {
		return "", ErrLLMNotConfigured
	}

	emails, err := s.emailRepo.GetByThreadID(threadID)
	if err != nil {
		return "", err
	}

	if len(emails) == 0 {
		return "", fmt.Errorf("no emails found in thread")
	}

	// Convert to LLM format
	emailContexts := make([]*llm.EmailContext, len(emails))
	for i, e := range emails {
		body := ""
		if emailBody, err := s.emailRepo.GetBody(e.ID); err == nil && emailBody != nil {
			body = emailBody.TextBody
		}
		emailContexts[i] = &llm.EmailContext{
			From:    e.FromEmail,
			Date:    e.Date.Format("2006-01-02 15:04"),
			Subject: e.Subject,
			Body:    body,
		}
	}

	return s.llmClient.SummarizeThreadEmails(ctx, emailContexts)
}

// GenerateReply generates a reply using RAG for style learning
func (s *Service) GenerateReply(ctx context.Context, emailID int64, options *in.ReplyOptions) (string, error) {
	if s.emailRepo == nil {
		return "", ErrRepoNotInitialized
	}
	if s.llmClient == nil {
		return "", ErrLLMNotConfigured
	}

	email, err := s.emailRepo.GetByID(emailID)
	if err != nil {
		return "", err
	}

	// 1. Get original email body
	body := ""
	if emailBody, err := s.emailRepo.GetBody(emailID); err == nil && emailBody != nil {
		body = emailBody.TextBody
	}

	// 2. Retrieve similar sent emails for style learning (RAG)
	styleContext := ""
	if s.ragRetriever != nil {
		results, err := s.ragRetriever.RetrieveForStyle(ctx, email.UserID, email.Subject+" "+body, 3)
		if err == nil && len(results) > 0 {
			styleContext = "User's writing style examples:\n"
			for _, r := range results {
				styleContext += fmt.Sprintf("---\n%s\n", r.Content)
			}
		}
	}

	// 3. Generate reply with LLM
	return s.llmClient.GenerateReply(ctx, email.Subject, body, email.FromEmail, styleContext, options)
}

// ExtractMeetingInfo extracts meeting information from email
// 미팅 관련 키워드가 없으면 API 호출 없이 빈 결과 반환 (비용 절감)
func (s *Service) ExtractMeetingInfo(ctx context.Context, emailID int64) (*in.MeetingInfo, error) {
	if s.emailRepo == nil {
		return nil, ErrRepoNotInitialized
	}
	if s.llmClient == nil {
		return nil, ErrLLMNotConfigured
	}

	email, err := s.emailRepo.GetByID(emailID)
	if err != nil {
		return nil, err
	}

	body := ""
	if emailBody, err := s.emailRepo.GetBody(emailID); err == nil && emailBody != nil {
		body = emailBody.TextBody
	}

	// 미팅 키워드 체크 - 키워드 없으면 API 호출 불필요
	if !containsMeetingKeyword(email.Subject, body) {
		return &in.MeetingInfo{HasMeeting: false}, nil
	}

	return s.llmClient.ExtractMeeting(ctx, email.Subject, body)
}

// containsMeetingKeyword checks if content contains any meeting-related keywords
func containsMeetingKeyword(subject, body string) bool {
	content := strings.ToLower(subject + " " + body)
	for _, keyword := range MeetingKeywords {
		if strings.Contains(content, keyword) {
			return true
		}
	}
	return false
}

// Chat handles conversational AI with RAG context and tool execution
func (s *Service) Chat(ctx context.Context, userID uuid.UUID, req *in.ChatRequest) (*in.ChatResponse, error) {
	if s.llmClient == nil {
		return nil, ErrLLMNotConfigured
	}

	// 1. Retrieve relevant context from RAG
	var ragContext string
	if s.ragRetriever != nil {
		results, err := s.ragRetriever.RetrieveForContext(ctx, userID, req.Message, 5)
		if err == nil && len(results) > 0 {
			ragContext = "Relevant emails:\n"
			for _, r := range results {
				ragContext += fmt.Sprintf("---\n%s\n", r.Content)
			}
		}
	}

	// 2. Get available tools
	toolDefs := []tools.ToolDefinition{}
	if s.toolRegistry != nil {
		toolDefs = s.toolRegistry.GetDefinitions()
	}

	// 3. Build prompt with context and tools
	systemPrompt := buildSystemPrompt(toolDefs)
	userPrompt := req.Message
	if ragContext != "" {
		userPrompt = ragContext + "\n\nUser question: " + req.Message
	}

	// 4. Get LLM response with function calling
	response, toolCalls, err := s.llmClient.CompleteWithTools(ctx, systemPrompt, userPrompt, toolDefs)
	if err != nil {
		return nil, err
	}

	// 5. Execute tool calls if any
	var toolResults []*tools.ToolResult
	if len(toolCalls) > 0 && s.toolRegistry != nil {
		for _, tc := range toolCalls {
			result, err := s.toolRegistry.Execute(ctx, userID, tc.Name, tc.Args)
			if err != nil {
				result = &tools.ToolResult{
					Success: false,
					Error:   err.Error(),
				}
			}
			toolResults = append(toolResults, result)
		}
	}

	return &in.ChatResponse{
		Message:     response,
		SessionID:   req.SessionID,
		ToolResults: toolResults,
	}, nil
}

// ChatStream handles streaming conversational AI
func (s *Service) ChatStream(ctx context.Context, userID uuid.UUID, req *in.ChatRequest, handler in.StreamHandler) error {
	if s.llmClient == nil {
		return ErrLLMNotConfigured
	}

	// 1. Retrieve relevant context from RAG
	var ragContext string
	if s.ragRetriever != nil {
		results, _ := s.ragRetriever.RetrieveForContext(ctx, userID, req.Message, 5)
		if len(results) > 0 {
			ragContext = "Relevant emails:\n"
			for _, r := range results {
				ragContext += fmt.Sprintf("---\n%s\n", r.Content)
			}
		}
	}

	// 2. Build prompt with context
	prompt := systemPromptAgent + "\n\n"
	if ragContext != "" {
		prompt += ragContext + "\n\n"
	}
	prompt += "User: " + req.Message

	// 3. Stream LLM response
	return s.llmClient.Stream(ctx, prompt, handler)
}

// ExecuteTool executes a specific tool by name
func (s *Service) ExecuteTool(ctx context.Context, userID uuid.UUID, toolName string, args map[string]any) (*tools.ToolResult, error) {
	if s.toolRegistry == nil {
		return nil, fmt.Errorf("tool registry not available")
	}

	return s.toolRegistry.Execute(ctx, userID, toolName, args)
}

// ConfirmProposal confirms and executes a pending action proposal
func (s *Service) ConfirmProposal(ctx context.Context, userID uuid.UUID, proposalID string) (*tools.ToolResult, error) {
	// TODO: Retrieve proposal from cache/redis
	// TODO: Execute the proposed action
	// For now, return not implemented
	return &tools.ToolResult{
		Success: false,
		Error:   "proposal confirmation not implemented yet",
	}, nil
}

// ListTools returns all available tools
func (s *Service) ListTools() []tools.ToolDefinition {
	if s.toolRegistry == nil {
		return []tools.ToolDefinition{}
	}
	return s.toolRegistry.GetDefinitions()
}

// buildSystemPrompt constructs system prompt with tool descriptions
func buildSystemPrompt(toolDefs []tools.ToolDefinition) string {
	prompt := `You are an AI assistant for Workspace - an email and calendar automation platform.

Your capabilities:
1. Email Management: Read, search, and analyze emails
2. Calendar Management: View, search, and propose calendar events
3. Contact Management: Search and retrieve contact information
4. Semantic Search: Find relevant emails and events by meaning

IMPORTANT RULES:
- For actions that modify data (send email, create event), you MUST return a PROPOSAL for user confirmation
- Never directly execute write operations without user approval
- Use tools to retrieve information and provide helpful suggestions
- Be concise and professional

Available tools:`

	for _, tool := range toolDefs {
		toolJSON, _ := json.MarshalIndent(tool, "", "  ")
		prompt += fmt.Sprintf("\n\n%s", string(toolJSON))
	}

	return prompt
}

const systemPromptAgent = `You are an AI email assistant. You help users manage their emails, calendar, and tasks.
You have access to the user's email history for context.
Be helpful, concise, and professional.
When asked about specific emails, use the provided context to answer accurately.`

// =============================================================================
// Translation Methods
// =============================================================================

// TranslateEmail translates an email's subject and body to the target language.
// 결과는 DB에 저장하지 않음 (클라이언트에서 캐싱)
func (s *Service) TranslateEmail(ctx context.Context, emailID int64, targetLang string) (*in.TranslateEmailResult, error) {
	if s.emailRepo == nil {
		return nil, ErrRepoNotInitialized
	}
	if s.llmClient == nil {
		return nil, ErrLLMNotConfigured
	}

	email, err := s.emailRepo.GetByID(emailID)
	if err != nil {
		return nil, err
	}

	body := ""
	if emailBody, err := s.emailRepo.GetBody(emailID); err == nil && emailBody != nil {
		if emailBody.TextBody != "" {
			body = emailBody.TextBody
		} else {
			body = emailBody.HTMLBody
		}
	}

	translatedSubject, translatedBody, err := s.llmClient.TranslateEmail(ctx, email.Subject, body, targetLang)
	if err != nil {
		return nil, err
	}

	return &in.TranslateEmailResult{
		EmailID:        emailID,
		Subject:        translatedSubject,
		Body:           translatedBody,
		TargetLanguage: targetLang,
	}, nil
}

// TranslateEmailDirect translates email content without loading from DB.
// 결과는 DB에 저장하지 않음 (클라이언트에서 캐싱)
func (s *Service) TranslateEmailDirect(ctx context.Context, subject, body, targetLang string) (*in.TranslateEmailResult, error) {
	if s.llmClient == nil {
		return nil, ErrLLMNotConfigured
	}

	translatedSubject, translatedBody, err := s.llmClient.TranslateEmail(ctx, subject, body, targetLang)
	if err != nil {
		return nil, err
	}

	return &in.TranslateEmailResult{
		Subject:        translatedSubject,
		Body:           translatedBody,
		TargetLanguage: targetLang,
	}, nil
}

// TranslateText translates arbitrary text to the target language.
// 결과는 DB에 저장하지 않음 (클라이언트에서 캐싱)
func (s *Service) TranslateText(ctx context.Context, text, targetLang string) (*in.TranslateTextResult, error) {
	if s.llmClient == nil {
		return nil, ErrLLMNotConfigured
	}

	translated, err := s.llmClient.TranslateText(ctx, text, targetLang)
	if err != nil {
		return nil, err
	}

	return &in.TranslateTextResult{
		TranslatedText: translated,
		TargetLanguage: targetLang,
	}, nil
}

// DetectLanguage detects the language of the given text.
func (s *Service) DetectLanguage(ctx context.Context, text string) (string, float64, error) {
	if s.llmClient == nil {
		return "", 0, ErrLLMNotConfigured
	}

	// 간단한 휴리스틱으로 언어 감지 (LLM 호출 없이)
	// 한글이 포함되어 있으면 ko, 일본어면 ja, 그 외 en
	lang := detectLanguageSimple(text)
	return lang, 0.8, nil
}

// detectLanguageSimple performs simple language detection without LLM.
func detectLanguageSimple(text string) string {
	for _, r := range text {
		// 한글 범위
		if r >= 0xAC00 && r <= 0xD7A3 {
			return "ko"
		}
		// 일본어 히라가나/가타카나
		if (r >= 0x3040 && r <= 0x309F) || (r >= 0x30A0 && r <= 0x30FF) {
			return "ja"
		}
		// 중국어 (CJK 통합 한자)
		if r >= 0x4E00 && r <= 0x9FFF {
			return "zh"
		}
	}
	return "en"
}

// =============================================================================
// Enhanced Summary Methods
// =============================================================================

// SummarizeEmailWithLang generates a summary in the specified language.
func (s *Service) SummarizeEmailWithLang(ctx context.Context, emailID int64, language string) (string, error) {
	if s.emailRepo == nil {
		return "", ErrRepoNotInitialized
	}
	if s.llmClient == nil {
		return "", ErrLLMNotConfigured
	}

	email, err := s.emailRepo.GetByID(emailID)
	if err != nil {
		return "", err
	}

	body := ""
	if emailBody, err := s.emailRepo.GetBody(emailID); err == nil && emailBody != nil {
		body = llm.CleanEmailBody(emailBody.TextBody)
	}

	return s.llmClient.SummarizeEmailWithLang(ctx, email.Subject, body, language)
}

// SummarizeThreadWithLang generates a thread summary in the specified language.
func (s *Service) SummarizeThreadWithLang(ctx context.Context, threadID string, language string) (string, error) {
	if s.emailRepo == nil {
		return "", ErrRepoNotInitialized
	}
	if s.llmClient == nil {
		return "", ErrLLMNotConfigured
	}

	emails, err := s.emailRepo.GetByThreadID(threadID)
	if err != nil {
		return "", err
	}

	if len(emails) == 0 {
		return "", fmt.Errorf("no emails found in thread")
	}

	// Convert to LLM format
	emailContexts := make([]llm.EmailContext, len(emails))
	for i, e := range emails {
		body := ""
		if emailBody, err := s.emailRepo.GetBody(e.ID); err == nil && emailBody != nil {
			body = emailBody.TextBody
		}
		emailContexts[i] = llm.EmailContext{
			From:    e.FromEmail,
			Date:    e.Date.Format("2006-01-02 15:04"),
			Subject: e.Subject,
			Body:    body,
		}
	}

	return s.llmClient.SummarizeThreadWithLang(ctx, emailContexts, language)
}
