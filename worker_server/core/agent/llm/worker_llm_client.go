package llm

import (
	"worker_server/core/agent/tools"
	"context"
	"fmt"

	"github.com/goccy/go-json"

	openai "github.com/sashabaranov/go-openai"
)

type Client struct {
	client      *openai.Client
	model       string
	maxTokens   int
	temperature float32
}

type ClientConfig struct {
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature float64
}

const DefaultModel = "gpt-4o-mini"

func NewClient(apiKey string) *Client {
	return &Client{
		client:      openai.NewClient(apiKey),
		model:       DefaultModel,
		maxTokens:   2048,
		temperature: 0.7,
	}
}

func NewClientWithConfig(cfg ClientConfig) *Client {
	model := cfg.Model
	if model == "" {
		model = DefaultModel
	}
	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 2048
	}
	temperature := cfg.Temperature
	if temperature == 0 {
		temperature = 0.7
	}
	return &Client{
		client:      openai.NewClient(cfg.APIKey),
		model:       model,
		maxTokens:   maxTokens,
		temperature: float32(temperature),
	}
}

func (c *Client) Complete(ctx context.Context, prompt string) (string, error) {
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
	})
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", nil
	}

	return resp.Choices[0].Message.Content, nil
}

func (c *Client) CompleteWithSystem(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: userPrompt,
			},
		},
	})
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", nil
	}

	return resp.Choices[0].Message.Content, nil
}

func (c *Client) Stream(ctx context.Context, prompt string, handler func(chunk string) error) error {
	stream, err := c.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		Stream: true,
	})
	if err != nil {
		return err
	}
	defer stream.Close()

	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		if len(resp.Choices) > 0 {
			if err := handler(resp.Choices[0].Delta.Content); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Client) Embedding(ctx context.Context, text string) ([]float32, error) {
	resp, err := c.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.AdaEmbeddingV2,
		Input: []string{text},
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Data) == 0 {
		return nil, nil
	}

	return resp.Data[0].Embedding, nil
}

func (c *Client) EmbeddingBatch(ctx context.Context, texts []string) ([][]float32, error) {
	resp, err := c.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.AdaEmbeddingV2,
		Input: texts,
	})
	if err != nil {
		return nil, err
	}

	result := make([][]float32, len(resp.Data))
	for i, data := range resp.Data {
		result[i] = data.Embedding
	}

	return result, nil
}

// CompleteJSON returns a JSON response from LLM
func (c *Client) CompleteJSON(ctx context.Context, prompt string) (string, error) {
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "{}", nil
	}

	return resp.Choices[0].Message.Content, nil
}

// CompleteWithTools calls LLM with function calling capability
func (c *Client) CompleteWithTools(ctx context.Context, systemPrompt, userPrompt string, toolDefs []tools.ToolDefinition) (string, []tools.ToolCall, error) {
	// Convert tool definitions to OpenAI format
	openaiTools := make([]openai.Tool, len(toolDefs))
	for i, t := range toolDefs {
		openaiTools[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: userPrompt,
			},
		},
		Tools: openaiTools,
	})
	if err != nil {
		return "", nil, err
	}

	if len(resp.Choices) == 0 {
		return "", nil, nil
	}

	choice := resp.Choices[0]

	// Extract tool calls
	var toolCalls []tools.ToolCall
	for _, tc := range choice.Message.ToolCalls {
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			continue
		}
		toolCalls = append(toolCalls, tools.ToolCall{
			ID:   tc.ID,
			Name: tc.Function.Name,
			Args: args,
		})
	}

	return choice.Message.Content, toolCalls, nil
}

// GenerateReplySimple generates a reply with simple parameters
func (c *Client) GenerateReplySimple(ctx context.Context, subject, body, from, styleContext, tone string) (string, error) {
	prompt := fmt.Sprintf(`Generate a reply to this email.

Original email:
From: %s
Subject: %s
Body:
%s

%s

Tone: %s

Generate a professional reply that matches the user's writing style if context is provided.
Only output the reply body, no subject line or signatures.`,
		from, subject, body, styleContext, tone)

	return c.Complete(ctx, prompt)
}

// TranslateText translates text to the target language
func (c *Client) TranslateText(ctx context.Context, text, targetLang string) (string, error) {
	prompt := fmt.Sprintf(`Translate the following text to %s.
Keep the formatting and tone consistent with the original.
Only output the translated text, nothing else.

Text to translate:
%s`, targetLang, text)

	return c.Complete(ctx, prompt)
}

// TranslateEmail translates email subject and body
func (c *Client) TranslateEmail(ctx context.Context, subject, body, targetLang string) (translatedSubject, translatedBody string, err error) {
	prompt := fmt.Sprintf(`Translate the following email to %s.
Keep the formatting and tone consistent with the original.
Return a JSON object with "subject" and "body" fields.

Subject: %s

Body:
%s`, targetLang, subject, body)

	result, err := c.CompleteJSON(ctx, prompt)
	if err != nil {
		return "", "", err
	}

	var translated struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := json.Unmarshal([]byte(result), &translated); err != nil {
		return "", "", err
	}

	return translated.Subject, translated.Body, nil
}
