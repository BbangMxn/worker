package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/goccy/go-json"
)

type MeetingInfo struct {
	Title       string   `json:"title"`
	StartTime   string   `json:"start_time,omitempty"`
	EndTime     string   `json:"end_time,omitempty"`
	Location    string   `json:"location,omitempty"`
	Attendees   []string `json:"attendees,omitempty"`
	Description string   `json:"description,omitempty"`
	MeetingURL  string   `json:"meeting_url,omitempty"`
	HasMeeting  bool     `json:"has_meeting"`
}

func (c *Client) ExtractMeetingInfo(ctx context.Context, subject, body string) (*MeetingInfo, error) {
	systemPrompt := `You are a meeting information extraction AI. Analyze the email and extract any meeting details.

If no meeting information is found, return has_meeting: false.

Respond with this exact JSON format:
{
  "has_meeting": true|false,
  "title": "meeting title",
  "start_time": "ISO 8601 datetime or human readable",
  "end_time": "ISO 8601 datetime or human readable",
  "location": "physical location or virtual",
  "attendees": ["email1", "email2"],
  "description": "brief description",
  "meeting_url": "zoom/teams/meet URL if present"
}`

	userPrompt := fmt.Sprintf("Subject: %s\n\nBody:\n%s", subject, truncateBody(body, 3000))

	resp, err := c.CompleteWithSystem(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}

	// Parse JSON response
	var result MeetingInfo
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return nil, fmt.Errorf("failed to parse meeting info: %w", err)
	}

	return &result, nil
}

type ContactInfo struct {
	Name    string `json:"name,omitempty"`
	Email   string `json:"email,omitempty"`
	Phone   string `json:"phone,omitempty"`
	Company string `json:"company,omitempty"`
	Title   string `json:"title,omitempty"`
}

func (c *Client) ExtractContactInfo(ctx context.Context, body, signature string) (*ContactInfo, error) {
	systemPrompt := `You are a contact information extraction AI. Extract contact details from the email signature or body.

Respond with this exact JSON format:
{
  "name": "full name",
  "email": "email address",
  "phone": "phone number",
  "company": "company name",
  "title": "job title"
}

Use null for any field that cannot be determined.`

	userPrompt := fmt.Sprintf("Email body:\n%s\n\nSignature:\n%s", truncateBody(body, 1000), signature)

	resp, err := c.CompleteWithSystem(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}

	var result ContactInfo
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return nil, fmt.Errorf("failed to parse contact info: %w", err)
	}

	return &result, nil
}
