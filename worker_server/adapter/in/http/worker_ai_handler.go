package http

import (
	"bufio"
	"strconv"

	"worker_server/core/agent"
	"worker_server/core/port/in"
	"worker_server/core/port/out"

	"github.com/gofiber/fiber/v2"
)

type AIHandler struct {
	aiService         in.AIService
	orchestrator      *agent.Orchestrator
	personStore       out.ExtendedPersonalizationStore
	autocompleteStore out.ExtendedPersonalizationStore
}

func NewAIHandler(aiService in.AIService) *AIHandler {
	return &AIHandler{aiService: aiService}
}

// NewAIHandlerFull creates AI handler with orchestrator for proposal management
func NewAIHandlerFull(aiService in.AIService, orchestrator *agent.Orchestrator) *AIHandler {
	return &AIHandler{
		aiService:    aiService,
		orchestrator: orchestrator,
	}
}

// NewAIHandlerComplete creates AI handler with all features including personalization
func NewAIHandlerComplete(
	aiService in.AIService,
	orchestrator *agent.Orchestrator,
	personStore out.ExtendedPersonalizationStore,
) *AIHandler {
	return &AIHandler{
		aiService:         aiService,
		orchestrator:      orchestrator,
		personStore:       personStore,
		autocompleteStore: personStore,
	}
}

func (h *AIHandler) Register(app fiber.Router) {
	ai := app.Group("/ai")
	ai.Post("/classify/:id", h.ClassifyEmail)
	ai.Post("/classify/batch", h.ClassifyBatch)
	ai.Post("/summarize/:id", h.SummarizeEmail)
	ai.Post("/reply/:id", h.GenerateReply)
	ai.Post("/extract-meeting/:id", h.ExtractMeeting)
	ai.Post("/chat", h.Chat)
	ai.Get("/chat/stream", h.ChatStream)

	// Translation endpoints
	ai.Post("/translate/:id", h.TranslateEmail)
	ai.Post("/translate", h.TranslateText)

	// Proposal endpoints
	ai.Post("/proposals/:id/confirm", h.ConfirmProposal)
	ai.Post("/proposals/:id/reject", h.RejectProposal)
	ai.Get("/proposals", h.ListProposals)

	// Autocomplete & Personalization endpoints
	ai.Post("/autocomplete", h.GetAutocomplete)
	ai.Get("/autocomplete/context", h.GetAutocompleteContext)
	ai.Get("/profile", h.GetUserProfile)
	ai.Put("/profile", h.UpdateUserProfile)
	ai.Get("/contacts/frequent", h.GetFrequentContacts)
	ai.Get("/contacts/important", h.GetImportantContacts)
	ai.Get("/patterns", h.GetCommunicationPatterns)
	ai.Get("/phrases", h.GetFrequentPhrases)
}

func (h *AIHandler) ClassifyEmail(c *fiber.Ctx) error {
	emailID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	result, err := h.aiService.ClassifyEmail(c.Context(), emailID)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(result)
}

func (h *AIHandler) ClassifyBatch(c *fiber.Ctx) error {
	var req struct {
		EmailIDs []int64 `json:"email_ids"`
	}

	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	results, err := h.aiService.ClassifyEmailBatch(c.Context(), req.EmailIDs)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{"results": results})
}

func (h *AIHandler) SummarizeEmail(c *fiber.Ctx) error {
	emailID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	// Optional body from request (for hybrid mode when body not in DB)
	var req struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	c.BodyParser(&req)

	var summary string
	if req.Body != "" {
		// Use body from request directly (force=true: API 요청이므로 항상 실행)
		summary, err = h.aiService.SummarizeEmailDirect(c.Context(), req.Subject, req.Body, true)
	} else {
		// Fetch from DB (force=true: API 요청이므로 항상 실행)
		summary, err = h.aiService.SummarizeEmail(c.Context(), emailID, true)
	}

	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{"summary": summary})
}

func (h *AIHandler) GenerateReply(c *fiber.Ctx) error {
	emailID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	var options in.ReplyOptions
	if err := c.BodyParser(&options); err != nil {
		// Use defaults
		options = in.ReplyOptions{
			Tone:   "professional",
			Intent: "inform",
		}
	}

	reply, err := h.aiService.GenerateReply(c.Context(), emailID, &options)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{"reply": reply})
}

func (h *AIHandler) ExtractMeeting(c *fiber.Ctx) error {
	emailID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	meeting, err := h.aiService.ExtractMeetingInfo(c.Context(), emailID)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(meeting)
}

// TranslateEmail translates an email to the target language.
// POST /ai/translate/:id
// Body: { "target_lang": "ko", "subject": "optional", "body": "optional" }
func (h *AIHandler) TranslateEmail(c *fiber.Ctx) error {
	emailID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return ErrorResponse(c, 400, "invalid email id")
	}

	var req struct {
		TargetLang string `json:"target_lang"`
		Subject    string `json:"subject"`
		Body       string `json:"body"`
	}
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if req.TargetLang == "" {
		return ErrorResponse(c, 400, "target_lang is required")
	}

	var result *in.TranslateEmailResult

	// If subject/body provided, use direct translation (no DB lookup)
	if req.Subject != "" || req.Body != "" {
		result, err = h.aiService.TranslateEmailDirect(c.Context(), req.Subject, req.Body, req.TargetLang)
	} else {
		result, err = h.aiService.TranslateEmail(c.Context(), emailID, req.TargetLang)
	}

	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(result)
}

// TranslateText translates arbitrary text to the target language.
// POST /ai/translate
// Body: { "text": "Hello", "target_lang": "ko" }
func (h *AIHandler) TranslateText(c *fiber.Ctx) error {
	var req struct {
		Text       string `json:"text"`
		TargetLang string `json:"target_lang"`
	}
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if req.Text == "" {
		return ErrorResponse(c, 400, "text is required")
	}
	if req.TargetLang == "" {
		return ErrorResponse(c, 400, "target_lang is required")
	}

	result, err := h.aiService.TranslateText(c.Context(), req.Text, req.TargetLang)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(result)
}

func (h *AIHandler) Chat(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req in.ChatRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	resp, err := h.aiService.Chat(c.Context(), userID, &req)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(resp)
}

func (h *AIHandler) ChatStream(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	message := c.Query("message")
	sessionID := c.Query("session_id")

	if message == "" {
		return ErrorResponse(c, 400, "message is required")
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		req := &in.ChatRequest{
			Message:   message,
			SessionID: sessionID,
		}

		handler := func(chunk string) error {
			_, err := w.WriteString("data: " + chunk + "\n\n")
			if err != nil {
				return err
			}
			return w.Flush()
		}

		if err := h.aiService.ChatStream(c.Context(), userID, req, handler); err != nil {
			w.WriteString("data: [ERROR] " + err.Error() + "\n\n")
			w.Flush()
		}

		w.WriteString("data: [DONE]\n\n")
		w.Flush()
	})

	return nil
}

// ConfirmProposal confirms and executes a pending proposal
func (h *AIHandler) ConfirmProposal(c *fiber.Ctx) error {
	if h.orchestrator == nil {
		return ErrorResponse(c, 500, "orchestrator not configured")
	}

	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	proposalID := c.Params("id")
	if proposalID == "" {
		return ErrorResponse(c, 400, "proposal id is required")
	}

	resp, err := h.orchestrator.ConfirmProposal(c.Context(), userID, proposalID)
	if err != nil {
		return ErrorResponse(c, 404, err.Error())
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": resp.Message,
		"data":    resp.Data,
	})
}

// RejectProposal cancels a pending proposal
func (h *AIHandler) RejectProposal(c *fiber.Ctx) error {
	if h.orchestrator == nil {
		return ErrorResponse(c, 500, "orchestrator not configured")
	}

	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	proposalID := c.Params("id")
	if proposalID == "" {
		return ErrorResponse(c, 400, "proposal id is required")
	}

	if err := h.orchestrator.RejectProposal(c.Context(), userID, proposalID); err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "proposal rejected",
	})
}

// ListProposals returns all pending proposals for the user
func (h *AIHandler) ListProposals(c *fiber.Ctx) error {
	if h.orchestrator == nil {
		return ErrorResponse(c, 500, "orchestrator not configured")
	}

	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	proposals := h.orchestrator.ListProposals(c.Context(), userID)

	return c.JSON(fiber.Map{
		"proposals": proposals,
		"count":     len(proposals),
	})
}

// =============================================================================
// Autocomplete & Personalization Endpoints
// =============================================================================

// AutocompleteRequest represents autocomplete request payload.
type AutocompleteRequest struct {
	InputPrefix    string `json:"input_prefix"`
	RecipientEmail string `json:"recipient_email,omitempty"`
	Context        string `json:"context,omitempty"` // greeting, body, closing
	MaxSuggestions int    `json:"max_suggestions,omitempty"`
}

// AutocompleteResponse represents autocomplete response.
type AutocompleteResponse struct {
	Suggestions []AutocompleteSuggestion `json:"suggestions"`
	Context     *out.AutocompleteContext `json:"context,omitempty"`
}

// AutocompleteSuggestion represents a single autocomplete suggestion.
type AutocompleteSuggestion struct {
	Text       string  `json:"text"`
	Type       string  `json:"type"` // phrase, pattern, completion
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source,omitempty"` // learned, pattern, ai
}

// GetAutocomplete returns autocomplete suggestions based on user profile and context.
func (h *AIHandler) GetAutocomplete(c *fiber.Ctx) error {
	if h.personStore == nil {
		return ErrorResponse(c, 500, "personalization not configured")
	}

	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req AutocompleteRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if req.MaxSuggestions == 0 {
		req.MaxSuggestions = 5
	}

	// Get autocomplete context from Neo4j
	ctx := c.Context()
	autoCtx, err := h.personStore.GetAutocompleteContext(ctx, userID.String(), req.RecipientEmail, req.InputPrefix)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	// Build suggestions based on context
	suggestions := h.buildAutocompleteSuggestions(autoCtx, &req)

	return c.JSON(AutocompleteResponse{
		Suggestions: suggestions,
		Context:     autoCtx,
	})
}

// buildAutocompleteSuggestions builds autocomplete suggestions from context.
func (h *AIHandler) buildAutocompleteSuggestions(ctx *out.AutocompleteContext, req *AutocompleteRequest) []AutocompleteSuggestion {
	var suggestions []AutocompleteSuggestion
	inputLower := toLower(req.InputPrefix)

	// 1. Match from frequent phrases
	if ctx != nil && ctx.RelevantPhrases != nil {
		for _, phrase := range ctx.RelevantPhrases {
			if hasPrefix(toLower(phrase.Text), inputLower) {
				suggestions = append(suggestions, AutocompleteSuggestion{
					Text:       phrase.Text,
					Type:       "phrase",
					Confidence: float64(phrase.Count) / 100.0,
					Source:     "learned",
				})
			}
		}
	}

	// 2. Match from communication patterns
	if ctx != nil && ctx.Patterns != nil {
		for _, pattern := range ctx.Patterns {
			if hasPrefix(toLower(pattern.Text), inputLower) {
				suggestions = append(suggestions, AutocompleteSuggestion{
					Text:       pattern.Text,
					Type:       "pattern",
					Confidence: pattern.Confidence,
					Source:     "pattern",
				})
			}
		}
	}

	// 3. Sort by confidence and limit
	sortSuggestionsByConfidence(suggestions)
	if len(suggestions) > req.MaxSuggestions {
		suggestions = suggestions[:req.MaxSuggestions]
	}

	return suggestions
}

// GetAutocompleteContext returns the full autocomplete context for a user.
func (h *AIHandler) GetAutocompleteContext(c *fiber.Ctx) error {
	if h.personStore == nil {
		return ErrorResponse(c, 500, "personalization not configured")
	}

	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	recipientEmail := c.Query("recipient_email", "")
	inputPrefix := c.Query("input_prefix", "")

	ctx := c.Context()
	autoCtx, err := h.personStore.GetAutocompleteContext(ctx, userID.String(), recipientEmail, inputPrefix)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(autoCtx)
}

// GetUserProfile returns the extended user profile.
func (h *AIHandler) GetUserProfile(c *fiber.Ctx) error {
	if h.personStore == nil {
		return ErrorResponse(c, 500, "personalization not configured")
	}

	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	profile, err := h.personStore.GetExtendedProfile(c.Context(), userID.String())
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	if profile == nil {
		return c.JSON(fiber.Map{
			"profile": nil,
			"message": "profile not found",
		})
	}

	return c.JSON(fiber.Map{"profile": profile})
}

// UpdateUserProfile updates the user profile manually.
func (h *AIHandler) UpdateUserProfile(c *fiber.Ctx) error {
	if h.personStore == nil {
		return ErrorResponse(c, 500, "personalization not configured")
	}

	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var profile out.ExtendedUserProfile
	if err := c.BodyParser(&profile); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	profile.UserID = userID.String()

	if err := h.personStore.UpdateExtendedProfile(c.Context(), userID.String(), &profile); err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "profile updated",
	})
}

// GetFrequentContacts returns frequently contacted contacts.
func (h *AIHandler) GetFrequentContacts(c *fiber.Ctx) error {
	if h.personStore == nil {
		return ErrorResponse(c, 500, "personalization not configured")
	}

	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	limit := c.QueryInt("limit", 10)

	contacts, err := h.personStore.GetFrequentContacts(c.Context(), userID.String(), limit)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{
		"contacts": contacts,
		"count":    len(contacts),
	})
}

// GetImportantContacts returns important contacts.
func (h *AIHandler) GetImportantContacts(c *fiber.Ctx) error {
	if h.personStore == nil {
		return ErrorResponse(c, 500, "personalization not configured")
	}

	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	limit := c.QueryInt("limit", 10)

	contacts, err := h.personStore.GetImportantContacts(c.Context(), userID.String(), limit)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{
		"contacts": contacts,
		"count":    len(contacts),
	})
}

// GetCommunicationPatterns returns learned communication patterns.
func (h *AIHandler) GetCommunicationPatterns(c *fiber.Ctx) error {
	if h.personStore == nil {
		return ErrorResponse(c, 500, "personalization not configured")
	}

	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	patternType := c.Query("type", "greeting")
	limit := c.QueryInt("limit", 10)

	patterns, err := h.personStore.GetCommunicationPatterns(c.Context(), userID.String(), patternType, limit)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{
		"patterns": patterns,
		"count":    len(patterns),
	})
}

// GetFrequentPhrases returns frequently used phrases.
func (h *AIHandler) GetFrequentPhrases(c *fiber.Ctx) error {
	if h.personStore == nil {
		return ErrorResponse(c, 500, "personalization not configured")
	}

	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	limit := c.QueryInt("limit", 20)

	phrases, err := h.personStore.GetFrequentPhrases(c.Context(), userID.String(), limit)
	if err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{
		"phrases": phrases,
		"count":   len(phrases),
	})
}

// =============================================================================
// Helper Functions
// =============================================================================

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

func hasPrefix(s, prefix string) bool {
	if len(prefix) > len(s) {
		return false
	}
	return s[:len(prefix)] == prefix
}

func sortSuggestionsByConfidence(suggestions []AutocompleteSuggestion) {
	// Simple bubble sort for small arrays
	for i := 0; i < len(suggestions)-1; i++ {
		for j := 0; j < len(suggestions)-i-1; j++ {
			if suggestions[j].Confidence < suggestions[j+1].Confidence {
				suggestions[j], suggestions[j+1] = suggestions[j+1], suggestions[j]
			}
		}
	}
}
