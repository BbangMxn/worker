package llm

import (
	"context"
	"fmt"

	"worker_server/core/port/in"
)

// GenerateReply generates a reply to an email using user's writing style
func (c *Client) GenerateReply(ctx context.Context, subject, body, from, styleContext string, options *in.ReplyOptions) (string, error) {
	tone := "professional"
	length := "medium"

	if options != nil {
		if options.Tone != "" {
			tone = options.Tone
		}
		if options.Length != "" {
			length = options.Length
		}
	}

	systemPrompt := fmt.Sprintf(`You are an email reply assistant. Generate a reply that matches the user's writing style.

Tone: %s
Length: %s (short: 1-2 sentences, medium: 3-5 sentences, long: detailed response)

%s

Write a natural, contextually appropriate reply. Do not include subject line or email headers.
Only output the reply body.`, tone, length, styleContext)

	userPrompt := fmt.Sprintf("Original email from %s:\nSubject: %s\n\n%s\n\nGenerate a reply:", from, subject, truncateBody(body, 2000))

	return c.CompleteWithSystem(ctx, systemPrompt, userPrompt)
}

// ExtractMeeting wraps ExtractMeetingInfo and converts to port type
func (c *Client) ExtractMeeting(ctx context.Context, subject, body string) (*in.MeetingInfo, error) {
	info, err := c.ExtractMeetingInfo(ctx, subject, body)
	if err != nil {
		return nil, err
	}

	if !info.HasMeeting {
		return nil, nil
	}

	return &in.MeetingInfo{
		Title:       info.Title,
		StartTime:   info.StartTime,
		EndTime:     info.EndTime,
		Location:    info.Location,
		Attendees:   info.Attendees,
		Description: info.Description,
		MeetingURL:  info.MeetingURL,
	}, nil
}

// SummarizeThread wraps the internal method with domain types
func (c *Client) SummarizeThreadEmails(ctx context.Context, emails []*EmailContext) (string, error) {
	if len(emails) == 0 {
		return "", nil
	}

	systemPrompt := `You are an email thread summarization AI. Summarize the entire email conversation.
Include:
1. Main topic of discussion
2. Key points from each participant
3. Current status or conclusion
4. Any pending action items

Keep the summary concise but comprehensive.`

	userPrompt := "Email thread:\n\n"
	for i, email := range emails {
		body := email.Body
		if len(body) > 1000 {
			body = body[:1000] + "..."
		}
		userPrompt += fmt.Sprintf("--- Email %d ---\nFrom: %s\nDate: %s\nSubject: %s\n\n%s\n\n",
			i+1, email.From, email.Date, email.Subject, body)
	}

	return c.CompleteWithSystem(ctx, systemPrompt, userPrompt)
}
