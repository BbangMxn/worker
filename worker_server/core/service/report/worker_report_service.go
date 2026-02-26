package report

import (
	"context"
	"fmt"
	"time"

	"worker_server/core/agent/llm"
	"worker_server/core/domain"

	"github.com/google/uuid"
)

type Service struct {
	emailRepo  domain.EmailRepository
	reportRepo domain.ReportRepository
	llmClient  *llm.Client
}

func NewService(
	emailRepo domain.EmailRepository,
	reportRepo domain.ReportRepository,
	llmClient *llm.Client,
) *Service {
	return &Service{
		emailRepo:  emailRepo,
		reportRepo: reportRepo,
		llmClient:  llmClient,
	}
}

// GenerateReport generates a report for the specified period
func (s *Service) GenerateReport(ctx context.Context, userID uuid.UUID, reportType string, startDate, endDate time.Time) (*domain.Report, error) {
	// 1. Get emails in date range
	emails, err := s.emailRepo.GetByDateRange(userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get emails: %w", err)
	}

	// 2. Extract important emails (priority >= 4)
	var importantEmails []*domain.Email
	var needsReplyEmails []*domain.Email
	for _, e := range emails {
		if e.AIPriority != nil && *e.AIPriority >= 0.60 {
			importantEmails = append(importantEmails, e)
		}
		// Check if needs reply (received, unread, not in sent folder)
		if e.Folder == "inbox" && !e.IsRead {
			needsReplyEmails = append(needsReplyEmails, e)
		}
	}

	// 3. Generate summaries using LLM
	importantSummary, _ := s.summarizeEmails(ctx, importantEmails, "important")
	replyNeededSummary, _ := s.summarizeEmails(ctx, needsReplyEmails, "needs_reply")

	// 4. Extract schedule suggestions and action items
	scheduleSuggestions, _ := s.extractScheduleSuggestions(ctx, emails)
	actionItems, _ := s.extractActionItems(ctx, emails)

	// 5. Build report
	report := &domain.Report{
		UserID:     userID,
		ReportType: reportType,
		StartDate:  startDate,
		EndDate:    endDate,

		TotalEmails:      len(emails),
		ImportantEmails:  len(importantEmails),
		NeedsReplyEmails: len(needsReplyEmails),

		ImportantSummary:    importantSummary,
		ReplyNeededSummary:  replyNeededSummary,
		ScheduleSuggestions: scheduleSuggestions,
		ActionItems:         actionItems,

		GeneratedAt: time.Now(),
	}

	// 6. Save report
	if err := s.reportRepo.Create(ctx, report); err != nil {
		return nil, fmt.Errorf("failed to save report: %w", err)
	}

	return report, nil
}

func (s *Service) summarizeEmails(ctx context.Context, emails []*domain.Email, category string) (string, error) {
	if len(emails) == 0 {
		return "No emails in this category.", nil
	}

	prompt := fmt.Sprintf("Summarize these %d %s emails briefly:\n\n", len(emails), category)
	for i, e := range emails {
		if i >= 10 { // Limit for token efficiency
			prompt += fmt.Sprintf("... and %d more emails\n", len(emails)-10)
			break
		}
		prompt += fmt.Sprintf("- From: %s, Subject: %s\n", e.FromEmail, e.Subject)
	}

	return s.llmClient.Complete(ctx, prompt)
}

func (s *Service) extractScheduleSuggestions(ctx context.Context, emails []*domain.Email) ([]domain.ScheduleSuggestion, error) {
	var suggestions []domain.ScheduleSuggestion

	for _, email := range emails {
		if email.AICategory != nil && *email.AICategory == "primary" {
			// Get email body
			body := ""
			if emailBody, err := s.emailRepo.GetBody(email.ID); err == nil && emailBody != nil {
				body = emailBody.TextBody
			}

			// Use LLM to check for meeting info
			info, err := s.llmClient.ExtractMeetingInfo(ctx, email.Subject, body)
			if err != nil || !info.HasMeeting {
				continue
			}

			suggestions = append(suggestions, domain.ScheduleSuggestion{
				EmailID:     email.ID,
				Title:       info.Title,
				Description: info.Description,
				Location:    info.Location,
				Attendees:   info.Attendees,
				Confidence:  0.8,
				Source:      "email",
			})
		}
	}

	return suggestions, nil
}

func (s *Service) extractActionItems(ctx context.Context, emails []*domain.Email) ([]domain.ActionItem, error) {
	var items []domain.ActionItem

	// Use LLM to extract action items from high priority emails
	for _, email := range emails {
		if email.AIPriority == nil || *email.AIPriority < 0.40 {
			continue
		}

		prompt := fmt.Sprintf("Extract any action items from this email. Subject: %s. Respond with a JSON array of action items or empty array if none.", email.Subject)
		_, _ = s.llmClient.Complete(ctx, prompt)

		// Simplified - would parse JSON in production
		items = append(items, domain.ActionItem{
			EmailID:     email.ID,
			Description: "Review and respond to: " + email.Subject,
			Priority:    *email.AIPriority,
			Status:      "pending",
			Confidence:  0.7,
		})
	}

	return items, nil
}

// GetReport retrieves a report by ID
func (s *Service) GetReport(ctx context.Context, reportID int64) (*domain.Report, error) {
	return s.reportRepo.GetByID(ctx, reportID)
}

// ListReports lists reports for a user
func (s *Service) ListReports(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Report, int, error) {
	return s.reportRepo.ListByUserID(ctx, userID, limit, offset)
}

// GetLatestReport gets the latest report of a specific type
func (s *Service) GetLatestReport(ctx context.Context, userID uuid.UUID, reportType string) (*domain.Report, error) {
	return s.reportRepo.GetLatest(ctx, userID, reportType)
}
