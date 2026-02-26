package worker

import (
	"context"
	"fmt"
	"time"

	"worker_server/core/agent/rag"
	"worker_server/core/port/out"
	"worker_server/pkg/logger"

	"github.com/google/uuid"
)

type RAGProcessor struct {
	indexer       *rag.IndexerService
	styleAnalyzer *rag.StyleAnalyzer
	emailRepo     out.EmailRepository
	bodyRepo      out.EmailBodyRepository
}

func NewRAGProcessor(
	indexer *rag.IndexerService,
	styleAnalyzer *rag.StyleAnalyzer,
	emailRepo out.EmailRepository,
	bodyRepo out.EmailBodyRepository,
) *RAGProcessor {
	return &RAGProcessor{
		indexer:       indexer,
		styleAnalyzer: styleAnalyzer,
		emailRepo:     emailRepo,
		bodyRepo:      bodyRepo,
	}
}

// RAGIndexMinimalPayload is the minimal payload from sync (only IDs)
type RAGIndexMinimalPayload struct {
	UserID  string `json:"user_id"`
	EmailID int64  `json:"email_id"`
}

func (p *RAGProcessor) ProcessIndex(ctx context.Context, msg *Message) error {
	minPayload, err := ParsePayload[RAGIndexMinimalPayload](msg)
	if err != nil {
		return err
	}

	log := logger.WithFields(map[string]any{
		"job":      "rag.index",
		"email_id": minPayload.EmailID,
		"user_id":  minPayload.UserID,
	})

	if p.indexer == nil {
		log.Debug("RAG indexer not configured, skipping")
		return nil
	}

	if p.emailRepo == nil {
		log.Error("email repository not configured")
		return fmt.Errorf("email repository not configured")
	}

	email, err := p.emailRepo.GetByID(ctx, minPayload.EmailID)
	if err != nil {
		log.WithError(err).Error("failed to fetch email")
		return err
	}
	if email == nil {
		log.Debug("email not found, skipping")
		return nil
	}

	// Fetch email body from MongoDB
	var bodyText string
	if p.bodyRepo != nil {
		body, err := p.bodyRepo.GetBody(ctx, minPayload.EmailID)
		if err != nil {
			log.WithError(err).Debug("body fetch failed, using snippet")
		} else if body != nil {
			if body.Text != "" {
				bodyText = body.Text
			} else {
				bodyText = body.HTML
			}
		}
	}

	if bodyText == "" {
		bodyText = email.Snippet
	}

	direction := "inbound"
	if email.Folder == "sent" {
		direction = "outbound"
	}

	userUUID, err := uuid.Parse(minPayload.UserID)
	if err != nil {
		log.WithError(err).Error("invalid user ID format")
		return err
	}

	req := &rag.EmailIndexRequest{
		EmailID:    email.ID,
		UserID:     userUUID,
		Subject:    email.Subject,
		Body:       bodyText,
		FromEmail:  email.FromEmail,
		Direction:  direction,
		Folder:     string(email.Folder),
		ReceivedAt: email.ReceivedAt,
	}

	if err := p.indexer.IndexEmail(ctx, req); err != nil {
		log.WithError(err).Error("indexing failed")
		return err
	}

	log.Debug("indexed successfully")
	return nil
}

// RAGBatchIndexMinimalPayload is the minimal payload for batch indexing (only IDs)
type RAGBatchIndexMinimalPayload struct {
	UserID       string  `json:"user_id"`
	ConnectionID int64   `json:"connection_id"`
	EmailIDs     []int64 `json:"email_ids"`
}

func (p *RAGProcessor) ProcessBatchIndex(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[RAGBatchIndexMinimalPayload](msg)
	if err != nil {
		return err
	}

	log := logger.WithFields(map[string]any{
		"job":     "rag.batch_index",
		"user_id": payload.UserID,
		"count":   len(payload.EmailIDs),
	})

	if p.indexer == nil {
		log.Debug("RAG indexer not configured, skipping")
		return nil
	}

	if p.emailRepo == nil {
		log.Error("email repository not configured")
		return fmt.Errorf("email repository not configured")
	}

	userUUID, err := uuid.Parse(payload.UserID)
	if err != nil {
		log.WithError(err).Error("invalid user ID format")
		return err
	}

	var requests []*rag.EmailIndexRequest
	for _, emailID := range payload.EmailIDs {
		email, err := p.emailRepo.GetByID(ctx, emailID)
		if err != nil || email == nil {
			continue
		}

		var bodyText string
		if p.bodyRepo != nil {
			body, err := p.bodyRepo.GetBody(ctx, emailID)
			if err == nil && body != nil {
				if body.Text != "" {
					bodyText = body.Text
				} else {
					bodyText = body.HTML
				}
			}
		}
		if bodyText == "" {
			bodyText = email.Snippet
		}

		direction := "inbound"
		if email.Folder == "sent" {
			direction = "outbound"
		}

		requests = append(requests, &rag.EmailIndexRequest{
			EmailID:    email.ID,
			UserID:     userUUID,
			Subject:    email.Subject,
			Body:       bodyText,
			FromEmail:  email.FromEmail,
			Direction:  direction,
			Folder:     string(email.Folder),
			ReceivedAt: email.ReceivedAt,
		})
	}

	if len(requests) == 0 {
		log.Debug("no emails found for indexing")
		return nil
	}

	if err := p.indexer.IndexBatch(ctx, requests); err != nil {
		log.WithError(err).Error("batch indexing failed")
		return err
	}

	log.WithField("indexed", len(requests)).Debug("batch indexed successfully")
	return nil
}

// =============================================================================
// Profile Analysis Processing
// =============================================================================

// ProcessProfileAnalysis analyzes sent emails for user profile learning.
func (p *RAGProcessor) ProcessProfileAnalysis(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[ProfileAnalysisPayload](msg)
	if err != nil {
		return err
	}

	log := logger.WithFields(map[string]any{
		"job":      "profile.analyze",
		"email_id": payload.EmailID,
		"user_id":  payload.UserID,
	})

	if p.styleAnalyzer == nil {
		log.Debug("style analyzer not configured, skipping")
		return nil
	}

	userUUID, err := uuid.Parse(payload.UserID)
	if err != nil {
		log.WithError(err).Error("invalid user ID format")
		return err
	}

	input := &rag.AnalysisInput{
		UserID:         userUUID,
		EmailID:        payload.EmailID,
		Subject:        payload.Subject,
		Body:           payload.Body,
		RecipientEmail: payload.RecipientEmail,
		RecipientName:  payload.RecipientName,
		SentAt:         payload.SentAt,
		IsReply:        payload.IsReply,
		ThreadID:       payload.ThreadID,
	}

	result, err := p.styleAnalyzer.AnalyzeSentEmail(ctx, input)
	if err != nil {
		log.WithError(err).Error("analysis failed")
		return err
	}

	if result != nil {
		log.WithFields(map[string]any{
			"tone":      result.ToneEstimate,
			"formality": result.FormalityScore,
		}).Debug("analysis completed")
	}

	return nil
}

// ProcessBatchProfileAnalysis analyzes multiple sent emails for profile learning.
func (p *RAGProcessor) ProcessBatchProfileAnalysis(ctx context.Context, msg *Message) error {
	payload, err := ParsePayload[BatchProfileAnalysisPayload](msg)
	if err != nil {
		return err
	}

	log := logger.WithFields(map[string]any{
		"job":   "profile.batch_analyze",
		"count": len(payload.Emails),
	})

	if p.styleAnalyzer == nil {
		log.Debug("style analyzer not configured, skipping")
		return nil
	}

	analyzed := 0
	for _, email := range payload.Emails {
		userUUID, err := uuid.Parse(email.UserID)
		if err != nil {
			continue
		}

		input := &rag.AnalysisInput{
			UserID:         userUUID,
			EmailID:        email.EmailID,
			Subject:        email.Subject,
			Body:           email.Body,
			RecipientEmail: email.RecipientEmail,
			RecipientName:  email.RecipientName,
			SentAt:         email.SentAt,
			IsReply:        email.IsReply,
			ThreadID:       email.ThreadID,
		}

		if _, err := p.styleAnalyzer.AnalyzeSentEmail(ctx, input); err == nil {
			analyzed++
		}
	}

	log.WithField("analyzed", analyzed).Debug("batch analysis completed")
	return nil
}

// ProfileAnalysisPayload represents payload for profile analysis job.
type ProfileAnalysisPayload struct {
	EmailID        int64     `json:"email_id"`
	UserID         string    `json:"user_id"`
	Subject        string    `json:"subject"`
	Body           string    `json:"body"`
	RecipientEmail string    `json:"recipient_email"`
	RecipientName  string    `json:"recipient_name"`
	SentAt         time.Time `json:"sent_at"`
	IsReply        bool      `json:"is_reply"`
	ThreadID       string    `json:"thread_id"`
}

// BatchProfileAnalysisPayload represents payload for batch profile analysis.
type BatchProfileAnalysisPayload struct {
	Emails []ProfileAnalysisPayload `json:"emails"`
}

// =============================================================================
// Autocomplete Context Retrieval
// =============================================================================

// AutocompleteContextProvider provides context for autocomplete.
type AutocompleteContextProvider struct {
	personStore out.ExtendedPersonalizationStore
}

// NewAutocompleteContextProvider creates a new autocomplete context provider.
func NewAutocompleteContextProvider(personStore out.ExtendedPersonalizationStore) *AutocompleteContextProvider {
	return &AutocompleteContextProvider{
		personStore: personStore,
	}
}

// GetContext retrieves autocomplete context for a user and recipient.
func (p *AutocompleteContextProvider) GetContext(ctx context.Context, userID, recipientEmail, inputPrefix string) (*out.AutocompleteContext, error) {
	return p.personStore.GetAutocompleteContext(ctx, userID, recipientEmail, inputPrefix)
}
