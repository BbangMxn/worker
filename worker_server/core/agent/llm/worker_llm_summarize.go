package llm

import (
	"context"
	"fmt"
)

// SummarizeEmailOptions contains options for email summarization.
type SummarizeEmailOptions struct {
	Language string // User's preferred language (e.g., "ko", "en", "ja")
}

// getLanguageInstruction returns language instruction for the prompt.
func getLanguageInstruction(lang string) string {
	switch lang {
	case "ko":
		return "반드시 한국어로 요약해주세요."
	case "ja":
		return "必ず日本語で要約してください。"
	case "zh":
		return "请用中文进行总结。"
	case "es":
		return "Resume en español."
	case "fr":
		return "Résumez en français."
	case "de":
		return "Fassen Sie auf Deutsch zusammen."
	default:
		return "Summarize in English."
	}
}

// SummarizeEmail summarizes a single email in the user's language.
func (c *Client) SummarizeEmail(ctx context.Context, subject, body string) (string, error) {
	return c.SummarizeEmailWithLang(ctx, subject, body, "en")
}

// SummarizeEmailWithLang summarizes a single email in the specified language.
func (c *Client) SummarizeEmailWithLang(ctx context.Context, subject, body, language string) (string, error) {
	langInstruction := getLanguageInstruction(language)

	systemPrompt := fmt.Sprintf(`You are an email summarization AI. Create a brief, clear summary of the email.
Keep the summary to 1-3 sentences. Focus on the main point and any action items.

IMPORTANT: %s`, langInstruction)

	userPrompt := fmt.Sprintf("Subject: %s\n\nBody:\n%s", subject, truncateBody(body, 3000))

	return c.CompleteWithSystem(ctx, systemPrompt, userPrompt)
}

// SummarizeThread summarizes an email thread in English (default).
func (c *Client) SummarizeThread(ctx context.Context, emails []EmailContext) (string, error) {
	return c.SummarizeThreadWithLang(ctx, emails, "en")
}

// SummarizeThreadWithLang summarizes an email thread in the specified language.
func (c *Client) SummarizeThreadWithLang(ctx context.Context, emails []EmailContext, language string) (string, error) {
	langInstruction := getLanguageInstruction(language)

	systemPrompt := fmt.Sprintf(`You are an email thread summarization AI. Summarize the entire email conversation.
Include:
1. Main topic of discussion
2. Key points from each participant
3. Current status or conclusion
4. Any pending action items

Keep the summary concise but comprehensive.

IMPORTANT: %s`, langInstruction)

	userPrompt := "Email thread:\n\n"
	for i, email := range emails {
		userPrompt += fmt.Sprintf("--- Email %d ---\nFrom: %s\nDate: %s\nSubject: %s\n\n%s\n\n",
			i+1, email.From, email.Date, email.Subject, truncateBody(email.Body, 1000))
	}

	return c.CompleteWithSystem(ctx, systemPrompt, userPrompt)
}

type EmailContext struct {
	From    string
	Date    string
	Subject string
	Body    string
}
