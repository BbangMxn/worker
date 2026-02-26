package tools

import (
	"context"
	"fmt"
	"time"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

// CategoryImage is the image tool category
const CategoryImage ToolCategory = "image"

// ImageService interface for image operations
type ImageService interface {
	GenerateImage(ctx context.Context, userID string, req domain.GenerateImageRequest) (*domain.Image, error)
	GenerateImages(ctx context.Context, userID string, req domain.GenerateImageRequest) ([]domain.Image, error)
	GenerateIconBatch(ctx context.Context, userID string, req domain.IconBatchRequest) (*domain.IconBatchResult, error)
	GeneratePoster(ctx context.Context, userID string, req domain.PosterRequest) ([]domain.Image, error)
	GenerateMultiSize(ctx context.Context, userID string, req domain.MultiSizeRequest) (*domain.MultiSizeResult, error)
	GetBrandKit(ctx context.Context, userID string, brandKitID string) (*domain.BrandKit, error)
}

// ================================================================
// Image Generate Tool (Simple)
// ================================================================

// ImageGenerateTool generates images from prompts
type ImageGenerateTool struct {
	imageService ImageService
}

func NewImageGenerateTool(imageService ImageService) *ImageGenerateTool {
	return &ImageGenerateTool{imageService: imageService}
}

func (t *ImageGenerateTool) Name() string           { return "image.generate" }
func (t *ImageGenerateTool) Category() ToolCategory { return CategoryImage }

func (t *ImageGenerateTool) Description() string {
	return "Generate images using AI. Supports various styles and sizes for business use cases like presentations, posters, and marketing materials."
}

func (t *ImageGenerateTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "prompt", Type: "string", Description: "Description of the image to generate", Required: true},
		{Name: "type", Type: "string", Description: "Image type", Enum: []string{"icon", "poster", "infographic", "social", "mockup", "illustration", "diagram", "background"}, Default: "illustration"},
		{Name: "style", Type: "string", Description: "Visual style", Enum: []string{"flat", "outline", "3d", "gradient", "minimal", "corporate", "realistic", "artistic"}, Default: "corporate"},
		{Name: "quality", Type: "string", Description: "Quality level", Enum: []string{"draft", "standard", "high"}, Default: "standard"},
		{Name: "aspect_ratio", Type: "string", Description: "Aspect ratio", Enum: []string{"1:1", "4:3", "16:9", "9:16"}, Default: "1:1"},
		{Name: "count", Type: "number", Description: "Number of variations (1-4)", Default: 1},
		{Name: "brand_kit_id", Type: "string", Description: "Brand kit ID for consistent styling"},
	}
}

func (t *ImageGenerateTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	prompt := getStringArg(args, "prompt", "")
	if prompt == "" {
		return &ToolResult{Success: false, Error: "prompt is required"}, nil
	}

	imageType := domain.ImageType(getStringArg(args, "type", "illustration"))
	style := domain.ImageStyle(getStringArg(args, "style", "corporate"))
	quality := domain.ImageQuality(getStringArg(args, "quality", "standard"))
	aspectRatio := domain.AspectRatio(getStringArg(args, "aspect_ratio", "1:1"))
	count := getIntArg(args, "count", 1)
	brandKitID := getStringArg(args, "brand_kit_id", "")

	req := domain.GenerateImageRequest{
		Prompt:      prompt,
		Type:        imageType,
		Style:       style,
		Quality:     quality,
		AspectRatio: aspectRatio,
		Count:       count,
		BrandKitID:  brandKitID,
	}

	// Image generation is a proposal-based action (costs money)
	proposal := &ActionProposal{
		ID:          uuid.New().String(),
		Action:      "image.generate",
		Description: fmt.Sprintf("Generate %d %s image(s): %s", count, imageType, truncateString(prompt, 50)),
		Data: map[string]any{
			"prompt":       prompt,
			"type":         imageType,
			"style":        style,
			"quality":      quality,
			"aspect_ratio": aspectRatio,
			"count":        count,
			"brand_kit_id": brandKitID,
			"request":      req,
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  fmt.Sprintf("Ready to generate %d %s image(s). Please confirm to proceed.", count, imageType),
		Proposal: proposal,
	}, nil
}

// ================================================================
// Icon Batch Tool
// ================================================================

// IconBatchTool generates multiple icons with consistent style
type IconBatchTool struct {
	imageService ImageService
}

func NewIconBatchTool(imageService ImageService) *IconBatchTool {
	return &IconBatchTool{imageService: imageService}
}

func (t *IconBatchTool) Name() string           { return "image.icon.batch" }
func (t *IconBatchTool) Category() ToolCategory { return CategoryImage }

func (t *IconBatchTool) Description() string {
	return "Generate multiple icons with consistent style. Perfect for creating icon sets for apps, websites, or presentations."
}

func (t *IconBatchTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "icons", Type: "array", Description: "Array of icons to generate, each with 'name' and 'description'", Required: true},
		{Name: "style_type", Type: "string", Description: "Icon style", Enum: []string{"flat", "outline", "filled", "3d", "gradient", "glassmorphism"}, Default: "flat"},
		{Name: "primary_color", Type: "string", Description: "Primary color (hex)", Default: "#1a73e8"},
		{Name: "secondary_color", Type: "string", Description: "Secondary color (hex)", Default: "#ffffff"},
		{Name: "corner_radius", Type: "string", Description: "Corner style", Enum: []string{"sharp", "rounded", "circle"}, Default: "rounded"},
		{Name: "sizes", Type: "array", Description: "Sizes to generate (px)", Default: []int{24, 48, 64}},
		{Name: "variations", Type: "number", Description: "Variations per icon", Default: 1},
	}
}

func (t *IconBatchTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	// Parse icons array
	iconsRaw, ok := args["icons"].([]any)
	if !ok || len(iconsRaw) == 0 {
		return &ToolResult{Success: false, Error: "icons array is required"}, nil
	}

	icons := make([]domain.IconRequest, 0, len(iconsRaw))
	for _, iconRaw := range iconsRaw {
		iconMap, ok := iconRaw.(map[string]any)
		if !ok {
			continue
		}
		icons = append(icons, domain.IconRequest{
			Name:        getStringArg(iconMap, "name", ""),
			Description: getStringArg(iconMap, "description", ""),
		})
	}

	if len(icons) == 0 {
		return &ToolResult{Success: false, Error: "at least one icon is required"}, nil
	}

	// Parse style
	style := domain.IconStyle{
		Type:         domain.ImageStyle(getStringArg(args, "style_type", "flat")),
		CornerRadius: getStringArg(args, "corner_radius", "rounded"),
		Colors: domain.IconColors{
			Primary:   getStringArg(args, "primary_color", "#1a73e8"),
			Secondary: getStringArg(args, "secondary_color", "#ffffff"),
		},
		Background: "transparent",
		SizeBase:   64,
	}

	// Parse sizes
	sizes := []int{24, 48, 64}
	if sizesRaw, ok := args["sizes"].([]any); ok {
		sizes = make([]int, 0, len(sizesRaw))
		for _, s := range sizesRaw {
			if size, ok := s.(float64); ok {
				sizes = append(sizes, int(size))
			}
		}
	}

	variations := getIntArg(args, "variations", 1)

	req := domain.IconBatchRequest{
		Style:      &style,
		Icons:      icons,
		Sizes:      sizes,
		Formats:    []domain.ImageFormat{domain.ImageFormatPNG, domain.ImageFormatSVG},
		Variations: variations,
	}

	totalFiles := len(icons) * len(sizes) * 2 * variations // icons × sizes × formats × variations

	proposal := &ActionProposal{
		ID:          uuid.New().String(),
		Action:      "image.icon.batch",
		Description: fmt.Sprintf("Generate %d icons (%d files total) with %s style", len(icons), totalFiles, style.Type),
		Data: map[string]any{
			"icons":       icons,
			"style":       style,
			"sizes":       sizes,
			"variations":  variations,
			"total_files": totalFiles,
			"request":     req,
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  fmt.Sprintf("Ready to generate %d icons (%d files). Please confirm to proceed.", len(icons), totalFiles),
		Proposal: proposal,
	}, nil
}

// ================================================================
// Poster Generate Tool
// ================================================================

// PosterGenerateTool generates posters and banners
type PosterGenerateTool struct {
	imageService ImageService
}

func NewPosterGenerateTool(imageService ImageService) *PosterGenerateTool {
	return &PosterGenerateTool{imageService: imageService}
}

func (t *PosterGenerateTool) Name() string           { return "image.poster.generate" }
func (t *PosterGenerateTool) Category() ToolCategory { return CategoryImage }

func (t *PosterGenerateTool) Description() string {
	return "Generate posters, banners, and marketing materials with various size presets for social media, presentations, and print."
}

func (t *PosterGenerateTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "prompt", Type: "string", Description: "Description of the poster/banner", Required: true},
		{Name: "preset", Type: "string", Description: "Size preset", Enum: []string{"instagram_post", "instagram_story", "facebook_cover", "linkedin_banner", "youtube_thumbnail", "slide_16_9", "slide_4_3", "hero_banner", "og_image", "email_header"}, Default: "slide_16_9"},
		{Name: "mood", Type: "string", Description: "Visual mood", Enum: []string{"festive", "professional", "minimal", "bold", "elegant"}, Default: "professional"},
		{Name: "color_scheme", Type: "string", Description: "Color scheme", Enum: []string{"brand", "warm", "cool", "monochrome", "vibrant"}, Default: "brand"},
		{Name: "headline", Type: "string", Description: "Main headline text"},
		{Name: "subheadline", Type: "string", Description: "Secondary text"},
		{Name: "count", Type: "number", Description: "Number of variations", Default: 2},
		{Name: "brand_kit_id", Type: "string", Description: "Brand kit ID"},
	}
}

func (t *PosterGenerateTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	prompt := getStringArg(args, "prompt", "")
	if prompt == "" {
		return &ToolResult{Success: false, Error: "prompt is required"}, nil
	}

	preset := domain.SizePreset(getStringArg(args, "preset", "slide_16_9"))
	width, height := domain.GetPresetDimensions(preset)

	req := domain.PosterRequest{
		Prompt: prompt,
		Preset: preset,
		Style: domain.PosterStyle{
			Mood:        getStringArg(args, "mood", "professional"),
			ColorScheme: getStringArg(args, "color_scheme", "brand"),
		},
		Elements: &domain.PosterElements{
			Headline:    getStringArg(args, "headline", ""),
			Subheadline: getStringArg(args, "subheadline", ""),
		},
		BrandKitID: getStringArg(args, "brand_kit_id", ""),
		Count:      getIntArg(args, "count", 2),
	}

	proposal := &ActionProposal{
		ID:          uuid.New().String(),
		Action:      "image.poster.generate",
		Description: fmt.Sprintf("Generate %s poster (%dx%d): %s", preset, width, height, truncateString(prompt, 40)),
		Data: map[string]any{
			"prompt":     prompt,
			"preset":     preset,
			"dimensions": fmt.Sprintf("%dx%d", width, height),
			"count":      req.Count,
			"request":    req,
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  fmt.Sprintf("Ready to generate %d %s poster(s) (%dx%d). Please confirm to proceed.", req.Count, preset, width, height),
		Proposal: proposal,
	}, nil
}

// ================================================================
// Multi-Size Poster Tool
// ================================================================

// MultiSizePosterTool generates same design in multiple sizes
type MultiSizePosterTool struct {
	imageService ImageService
}

func NewMultiSizePosterTool(imageService ImageService) *MultiSizePosterTool {
	return &MultiSizePosterTool{imageService: imageService}
}

func (t *MultiSizePosterTool) Name() string           { return "image.poster.multi_size" }
func (t *MultiSizePosterTool) Category() ToolCategory { return CategoryImage }

func (t *MultiSizePosterTool) Description() string {
	return "Generate the same design in multiple sizes at once. Perfect for creating consistent branding across different platforms."
}

func (t *MultiSizePosterTool) Parameters() []ParameterSpec {
	return []ParameterSpec{
		{Name: "prompt", Type: "string", Description: "Description of the design", Required: true},
		{Name: "sizes", Type: "array", Description: "Array of size presets", Required: true},
		{Name: "headline", Type: "string", Description: "Main headline"},
		{Name: "subheadline", Type: "string", Description: "Secondary text"},
		{Name: "mood", Type: "string", Description: "Visual mood", Default: "professional"},
		{Name: "brand_kit_id", Type: "string", Description: "Brand kit ID"},
		{Name: "adapt_layout", Type: "boolean", Description: "Auto-adjust layout for each size", Default: true},
	}
}

func (t *MultiSizePosterTool) Execute(ctx context.Context, userID uuid.UUID, args map[string]any) (*ToolResult, error) {
	prompt := getStringArg(args, "prompt", "")
	if prompt == "" {
		return &ToolResult{Success: false, Error: "prompt is required"}, nil
	}

	// Parse sizes
	sizesRaw, ok := args["sizes"].([]any)
	if !ok || len(sizesRaw) == 0 {
		return &ToolResult{Success: false, Error: "sizes array is required"}, nil
	}

	sizes := make([]domain.SizeRequest, 0, len(sizesRaw))
	for _, sizeRaw := range sizesRaw {
		sizeStr, ok := sizeRaw.(string)
		if !ok {
			continue
		}
		sizes = append(sizes, domain.SizeRequest{
			Preset: domain.SizePreset(sizeStr),
		})
	}

	if len(sizes) == 0 {
		return &ToolResult{Success: false, Error: "at least one size is required"}, nil
	}

	req := domain.MultiSizeRequest{
		Prompt: prompt,
		BaseDesign: domain.PosterStyle{
			Mood: getStringArg(args, "mood", "professional"),
		},
		Elements: &domain.PosterElements{
			Headline:    getStringArg(args, "headline", ""),
			Subheadline: getStringArg(args, "subheadline", ""),
		},
		Sizes:       sizes,
		BrandKitID:  getStringArg(args, "brand_kit_id", ""),
		AdaptLayout: getBoolArg(args, "adapt_layout", true),
	}

	// Build size descriptions
	sizeDescs := make([]string, len(sizes))
	for i, s := range sizes {
		w, h := domain.GetPresetDimensions(s.Preset)
		sizeDescs[i] = fmt.Sprintf("%s (%dx%d)", s.Preset, w, h)
	}

	proposal := &ActionProposal{
		ID:          uuid.New().String(),
		Action:      "image.poster.multi_size",
		Description: fmt.Sprintf("Generate design in %d sizes: %s", len(sizes), truncateString(prompt, 30)),
		Data: map[string]any{
			"prompt":  prompt,
			"sizes":   sizeDescs,
			"count":   len(sizes),
			"request": req,
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	return &ToolResult{
		Success:  true,
		Message:  fmt.Sprintf("Ready to generate design in %d sizes. Please confirm to proceed.", len(sizes)),
		Proposal: proposal,
	}, nil
}

// ================================================================
// Helper Functions
// ================================================================

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
