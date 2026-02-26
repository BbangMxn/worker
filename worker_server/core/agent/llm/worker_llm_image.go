package llm

import (
	"worker_server/core/domain"
	"context"
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// ImageClient handles image generation using OpenAI DALL-E
type ImageClient struct {
	client *openai.Client
	model  string
}

// ImageClientConfig configuration for ImageClient
type ImageClientConfig struct {
	APIKey string
	Model  string // dall-e-3 or dall-e-2
}

// NewImageClient creates a new ImageClient
func NewImageClient(apiKey string) *ImageClient {
	return &ImageClient{
		client: openai.NewClient(apiKey),
		model:  openai.CreateImageModelDallE3,
	}
}

// NewImageClientWithConfig creates a new ImageClient with config
func NewImageClientWithConfig(cfg ImageClientConfig) *ImageClient {
	model := cfg.Model
	if model == "" {
		model = openai.CreateImageModelDallE3
	}
	return &ImageClient{
		client: openai.NewClient(cfg.APIKey),
		model:  model,
	}
}

// GenerateImageRequest is the request for image generation
type GenerateImageRequest struct {
	Prompt   string
	Size     string // 1024x1024, 1792x1024, 1024x1792
	Quality  string // standard, hd
	Style    string // vivid, natural
	N        int    // number of images (only 1 for DALL-E 3)
	BrandKit *domain.BrandKit
}

// GenerateImageResponse is the response from image generation
type GenerateImageResponse struct {
	URL           string
	RevisedPrompt string
}

// GenerateImage generates a single image using DALL-E
func (c *ImageClient) GenerateImage(ctx context.Context, req GenerateImageRequest) (*GenerateImageResponse, error) {
	// Enhance prompt with brand context if available
	prompt := req.Prompt
	if req.BrandKit != nil {
		brandContext := req.BrandKit.ToBrandPromptContext()
		if brandContext != "" {
			prompt = brandContext + prompt
		}
	}

	// Determine size
	size := req.Size
	if size == "" {
		size = openai.CreateImageSize1024x1024
	}

	// Determine quality
	quality := req.Quality
	if quality == "" {
		quality = openai.CreateImageQualityStandard
	}

	// Determine style
	style := req.Style
	if style == "" {
		style = openai.CreateImageStyleVivid
	}

	// Create request
	imageReq := openai.ImageRequest{
		Model:          c.model,
		Prompt:         prompt,
		Size:           size,
		Quality:        quality,
		Style:          style,
		N:              1, // DALL-E 3 only supports 1
		ResponseFormat: openai.CreateImageResponseFormatURL,
	}

	resp, err := c.client.CreateImage(ctx, imageReq)
	if err != nil {
		return nil, fmt.Errorf("failed to generate image: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no image generated")
	}

	return &GenerateImageResponse{
		URL:           resp.Data[0].URL,
		RevisedPrompt: resp.Data[0].RevisedPrompt,
	}, nil
}

// GenerateImages generates multiple images (runs multiple requests for DALL-E 3)
func (c *ImageClient) GenerateImages(ctx context.Context, req GenerateImageRequest) ([]GenerateImageResponse, error) {
	count := req.N
	if count <= 0 {
		count = 1
	}
	if count > 4 {
		count = 4 // max 4 variations
	}

	results := make([]GenerateImageResponse, 0, count)
	for i := 0; i < count; i++ {
		result, err := c.GenerateImage(ctx, req)
		if err != nil {
			return results, fmt.Errorf("failed to generate image %d: %w", i+1, err)
		}
		results = append(results, *result)
	}

	return results, nil
}

// OptimizePrompt enhances user prompt for better image generation
func (c *ImageClient) OptimizePrompt(ctx context.Context, userPrompt string, imageType domain.ImageType, style domain.ImageStyle) (string, error) {
	systemPrompt := `You are an expert at crafting prompts for AI image generation.
Convert the user's description into an optimized prompt for DALL-E 3.

Guidelines:
- Be specific and descriptive
- Include style, mood, and composition details
- Add technical quality keywords (high resolution, professional, etc.)
- Keep it under 1000 characters
- For icons: emphasize simplicity, clean lines, consistency
- For posters: include layout suggestions, typography hints
- For business images: maintain professional, corporate aesthetic

Return ONLY the optimized prompt, no explanations.`

	typeContext := getTypeContext(imageType)
	styleContext := getStyleContext(style)

	userMessage := fmt.Sprintf("Image type: %s\nStyle: %s\nUser request: %s", typeContext, styleContext, userPrompt)

	// Use the parent client for text completion
	client := &Client{
		client:      c.client,
		model:       "gpt-4o-mini",
		maxTokens:   500,
		temperature: 0.3,
	}

	optimized, err := client.CompleteWithSystem(ctx, systemPrompt, userMessage)
	if err != nil {
		// Fallback to original prompt if optimization fails
		return userPrompt, nil
	}

	return strings.TrimSpace(optimized), nil
}

// getTypeContext returns context string for image type
func getTypeContext(t domain.ImageType) string {
	switch t {
	case domain.ImageTypeIcon:
		return "Icon - simple, minimal, vector-style, suitable for UI/app icons"
	case domain.ImageTypePoster:
		return "Poster/Banner - marketing material, eye-catching, with space for text"
	case domain.ImageTypeInfographic:
		return "Infographic - data visualization, clean layout, informative"
	case domain.ImageTypeSocial:
		return "Social Media - engaging, platform-optimized, shareable"
	case domain.ImageTypeMockup:
		return "Mockup - product visualization, realistic, professional presentation"
	case domain.ImageTypeIllustration:
		return "Illustration - artistic, custom artwork, storytelling"
	case domain.ImageTypeDiagram:
		return "Diagram - flowchart, process visualization, clear connections"
	case domain.ImageTypeBackground:
		return "Background - subtle, non-distracting, suitable for text overlay"
	default:
		return "General business image"
	}
}

// getStyleContext returns context string for image style
func getStyleContext(s domain.ImageStyle) string {
	switch s {
	case domain.ImageStyleFlat:
		return "Flat design - 2D, solid colors, no shadows, modern"
	case domain.ImageStyleOutline:
		return "Outline/Line art - clean lines, minimal fills, vector-like"
	case domain.ImageStyleFilled:
		return "Filled solid - bold shapes, solid colors, impactful"
	case domain.ImageStyle3D:
		return "3D rendered - depth, lighting, realistic materials"
	case domain.ImageStyleGradient:
		return "Gradient - smooth color transitions, modern, vibrant"
	case domain.ImageStyleGlassmorphism:
		return "Glassmorphism - frosted glass effect, blur, transparency"
	case domain.ImageStyleMinimal:
		return "Minimal - simple, clean, lots of whitespace"
	case domain.ImageStyleCorporate:
		return "Corporate - professional, trustworthy, business-appropriate"
	case domain.ImageStyleHandDrawn:
		return "Hand-drawn - sketchy, artistic, personal touch"
	case domain.ImageStyleIsometric:
		return "Isometric - 3D perspective, technical, precise angles"
	case domain.ImageStyleRealistic:
		return "Realistic - photorealistic, detailed, lifelike"
	case domain.ImageStyleArtistic:
		return "Artistic - creative, expressive, unique style"
	default:
		return "Professional, clean, modern"
	}
}

// GetDALLESize converts domain size/aspect ratio to DALL-E size string
func GetDALLESize(width, height int) string {
	ratio := float64(width) / float64(height)

	// DALL-E 3 supported sizes: 1024x1024, 1792x1024, 1024x1792
	if ratio > 1.5 {
		return openai.CreateImageSize1792x1024 // landscape
	} else if ratio < 0.67 {
		return openai.CreateImageSize1024x1792 // portrait
	}
	return openai.CreateImageSize1024x1024 // square
}

// GetDALLESizeFromPreset converts size preset to DALL-E size
func GetDALLESizeFromPreset(preset domain.SizePreset) string {
	width, height := domain.GetPresetDimensions(preset)
	return GetDALLESize(width, height)
}

// GetDALLESizeFromAspectRatio converts aspect ratio to DALL-E size
func GetDALLESizeFromAspectRatio(ratio domain.AspectRatio) string {
	switch ratio {
	case domain.AspectRatio16x9:
		return openai.CreateImageSize1792x1024
	case domain.AspectRatio9x16:
		return openai.CreateImageSize1024x1792
	default:
		return openai.CreateImageSize1024x1024
	}
}

// GenerateIconPrompt creates an optimized prompt for icon generation
func GenerateIconPrompt(name, description string, style domain.IconStyle) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("Simple, clean icon representing '%s'", name))

	if description != "" {
		parts = append(parts, description)
	}

	// Style
	styleStr := string(style.Type)
	if styleStr != "" {
		parts = append(parts, styleStr+" style")
	}

	// Colors
	if style.Colors.Primary != "" {
		parts = append(parts, fmt.Sprintf("using %s as primary color", style.Colors.Primary))
	}

	// Corner radius
	if style.CornerRadius != "" {
		parts = append(parts, style.CornerRadius+" corners")
	}

	// Common icon qualities
	parts = append(parts, "minimalist", "vector-like", "suitable for UI", "single object", "centered", "white or transparent background")

	return strings.Join(parts, ", ")
}

// GeneratePosterPrompt creates an optimized prompt for poster generation
func GeneratePosterPrompt(req domain.PosterRequest, brandKit *domain.BrandKit) string {
	var parts []string

	parts = append(parts, req.Prompt)

	// Style
	if req.Style.Mood != "" {
		parts = append(parts, req.Style.Mood+" mood")
	}
	if req.Style.ColorScheme != "" {
		parts = append(parts, req.Style.ColorScheme+" color scheme")
	}
	if req.Style.Layout != "" {
		parts = append(parts, req.Style.Layout+" layout")
	}

	// Brand context
	if brandKit != nil {
		parts = append(parts, brandKit.ToBrandPromptContext())
	}

	// Common poster qualities
	parts = append(parts, "professional", "high quality", "suitable for business presentation")

	// Space for text if elements are defined
	if req.Elements != nil && req.Elements.Headline != "" {
		parts = append(parts, "with space for text overlay")
	}

	return strings.Join(parts, ", ")
}
