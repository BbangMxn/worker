package image

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"worker_server/core/agent/llm"
	"worker_server/core/domain"
	"worker_server/pkg/logger"

	"github.com/google/uuid"
)

// Service handles image generation operations
type Service struct {
	imageClient  *llm.ImageClient
	brandKitRepo BrandKitRepository
	imageRepo    ImageRepository
	storageURL   string // Base URL for image storage
}

// BrandKitRepository interface for brand kit persistence
type BrandKitRepository interface {
	GetByID(ctx context.Context, id string) (*domain.BrandKit, error)
	GetByUserID(ctx context.Context, userID string) ([]*domain.BrandKit, error)
	Save(ctx context.Context, brandKit *domain.BrandKit) error
	Update(ctx context.Context, brandKit *domain.BrandKit) error
	Delete(ctx context.Context, id string) error
}

// ImageRepository interface for image persistence
type ImageRepository interface {
	GetByID(ctx context.Context, id string) (*domain.Image, error)
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*domain.Image, error)
	GetByBatchID(ctx context.Context, batchID string) ([]*domain.Image, error)
	Save(ctx context.Context, image *domain.Image) error
	SaveBatch(ctx context.Context, images []*domain.Image) error
	Update(ctx context.Context, image *domain.Image) error
	Delete(ctx context.Context, id string) error
}

// NewService creates a new image service
func NewService(imageClient *llm.ImageClient, brandKitRepo BrandKitRepository, imageRepo ImageRepository) *Service {
	return &Service{
		imageClient:  imageClient,
		brandKitRepo: brandKitRepo,
		imageRepo:    imageRepo,
		storageURL:   "/api/v1/images/storage", // Default storage URL
	}
}

// SetStorageURL sets the base URL for image storage
func (s *Service) SetStorageURL(url string) {
	s.storageURL = url
}

// =============================================================================
// Simple Image Generation
// =============================================================================

// GenerateImage generates a single image
func (s *Service) GenerateImage(ctx context.Context, userID string, req domain.GenerateImageRequest) (*domain.Image, error) {
	if s.imageClient == nil {
		return nil, fmt.Errorf("image client not configured")
	}

	// Get brand kit if specified
	var brandKit *domain.BrandKit
	if req.BrandKitID != "" && s.brandKitRepo != nil {
		bk, err := s.brandKitRepo.GetByID(ctx, req.BrandKitID)
		if err == nil {
			brandKit = bk
		}
	}

	// Optimize prompt
	optimizedPrompt, err := s.imageClient.OptimizePrompt(ctx, req.Prompt, req.Type, req.Style)
	if err != nil {
		optimizedPrompt = req.Prompt // fallback to original
	}

	// Determine size
	size := llm.GetDALLESizeFromAspectRatio(req.AspectRatio)
	if req.Preset != "" {
		size = llm.GetDALLESizeFromPreset(req.Preset)
	}

	// Determine quality
	quality := "standard"
	if req.Quality == domain.ImageQualityHigh {
		quality = "hd"
	}

	// Generate image
	llmReq := llm.GenerateImageRequest{
		Prompt:   optimizedPrompt,
		Size:     size,
		Quality:  quality,
		Style:    "vivid",
		N:        1,
		BrandKit: brandKit,
	}

	result, err := s.imageClient.GenerateImage(ctx, llmReq)
	if err != nil {
		return nil, fmt.Errorf("failed to generate image: %w", err)
	}

	// Parse dimensions from size
	width, height := parseDimensions(size)

	// Create image record
	image := &domain.Image{
		ID:              uuid.New().String(),
		UserID:          userID,
		Type:            req.Type,
		Prompt:          req.Prompt,
		OptimizedPrompt: result.RevisedPrompt,
		Style:           req.Style,
		Quality:         req.Quality,
		Status:          domain.ImageStatusCompleted,
		Width:           width,
		Height:          height,
		Preset:          req.Preset,
		URL:             result.URL,
		Format:          domain.ImageFormatPNG,
		BrandKitID:      req.BrandKitID,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Save to repository if available
	if s.imageRepo != nil {
		if err := s.imageRepo.Save(ctx, image); err != nil {
			logger.WithFields(map[string]any{"error": err.Error()}).Warn("failed to save image to repository")
		}
	}

	return image, nil
}

// GenerateImages generates multiple images (variations)
func (s *Service) GenerateImages(ctx context.Context, userID string, req domain.GenerateImageRequest) ([]domain.Image, error) {
	if s.imageClient == nil {
		return nil, fmt.Errorf("image client not configured")
	}

	count := req.Count
	if count <= 0 {
		count = 1
	}
	if count > 4 {
		count = 4
	}

	// Get brand kit if specified
	var brandKit *domain.BrandKit
	if req.BrandKitID != "" && s.brandKitRepo != nil {
		bk, err := s.brandKitRepo.GetByID(ctx, req.BrandKitID)
		if err == nil {
			brandKit = bk
		}
	}

	// Optimize prompt
	optimizedPrompt, err := s.imageClient.OptimizePrompt(ctx, req.Prompt, req.Type, req.Style)
	if err != nil {
		optimizedPrompt = req.Prompt
	}

	// Determine size
	size := llm.GetDALLESizeFromAspectRatio(req.AspectRatio)
	if req.Preset != "" {
		size = llm.GetDALLESizeFromPreset(req.Preset)
	}

	// Determine quality
	quality := "standard"
	if req.Quality == domain.ImageQualityHigh {
		quality = "hd"
	}

	// Generate images
	llmReq := llm.GenerateImageRequest{
		Prompt:   optimizedPrompt,
		Size:     size,
		Quality:  quality,
		Style:    "vivid",
		N:        count,
		BrandKit: brandKit,
	}

	results, err := s.imageClient.GenerateImages(ctx, llmReq)
	if err != nil {
		return nil, fmt.Errorf("failed to generate images: %w", err)
	}

	// Parse dimensions
	width, height := parseDimensions(size)
	batchID := uuid.New().String()

	// Create image records
	images := make([]domain.Image, len(results))
	for i, result := range results {
		images[i] = domain.Image{
			ID:              uuid.New().String(),
			UserID:          userID,
			Type:            req.Type,
			Prompt:          req.Prompt,
			OptimizedPrompt: result.RevisedPrompt,
			Style:           req.Style,
			Quality:         req.Quality,
			Status:          domain.ImageStatusCompleted,
			Width:           width,
			Height:          height,
			Preset:          req.Preset,
			URL:             result.URL,
			Format:          domain.ImageFormatPNG,
			BrandKitID:      req.BrandKitID,
			BatchID:         batchID,
			VariationNum:    i + 1,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
	}

	// Save to repository if available
	if s.imageRepo != nil {
		imagePtrs := make([]*domain.Image, len(images))
		for i := range images {
			imagePtrs[i] = &images[i]
		}
		if err := s.imageRepo.SaveBatch(ctx, imagePtrs); err != nil {
			logger.WithFields(map[string]any{"error": err.Error()}).Warn("failed to save images to repository")
		}
	}

	return images, nil
}

// =============================================================================
// Icon Batch Generation
// =============================================================================

// GenerateIconBatch generates a batch of icons with consistent style
func (s *Service) GenerateIconBatch(ctx context.Context, userID string, req domain.IconBatchRequest) (*domain.IconBatchResult, error) {
	if s.imageClient == nil {
		return nil, fmt.Errorf("image client not configured")
	}

	// Get or create icon style
	style := req.Style
	if style == nil && req.TemplateID != "" {
		// TODO: Load template from repository
		style = &domain.IconStyle{
			Type:         domain.ImageStyleFlat,
			CornerRadius: "rounded",
			StrokeWidth:  2,
			Colors: domain.IconColors{
				Primary:   "#2563EB",
				Secondary: "#64748B",
			},
			Background: "transparent",
			Shadow:     "none",
			SizeBase:   64,
		}
	}
	if style == nil {
		// Default style
		style = &domain.IconStyle{
			Type:         domain.ImageStyleFlat,
			CornerRadius: "rounded",
			StrokeWidth:  2,
			Colors: domain.IconColors{
				Primary:   "#2563EB",
				Secondary: "#64748B",
			},
			Background: "transparent",
			Shadow:     "none",
			SizeBase:   64,
		}
	}

	// Default sizes and formats
	sizes := req.Sizes
	if len(sizes) == 0 {
		sizes = []int{24, 32, 48, 64, 128}
	}
	formats := req.Formats
	if len(formats) == 0 {
		formats = []domain.ImageFormat{domain.ImageFormatPNG}
	}

	batchID := uuid.New().String()
	variations := req.Variations
	if variations <= 0 {
		variations = 1
	}
	if variations > 3 {
		variations = 3
	}

	// Generate icons concurrently (max 3 concurrent)
	semaphore := make(chan struct{}, 3)
	var wg sync.WaitGroup
	var mu sync.Mutex

	iconResults := make([]domain.IconResult, len(req.Icons))

	for i, iconReq := range req.Icons {
		wg.Add(1)
		go func(idx int, icon domain.IconRequest) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := s.generateSingleIcon(ctx, userID, batchID, icon, *style, sizes, formats, variations)
			mu.Lock()
			iconResults[idx] = result
			mu.Unlock()
		}(i, iconReq)
	}

	wg.Wait()

	// Calculate totals
	totalFiles := len(req.Icons) * variations * len(sizes) * len(formats)

	return &domain.IconBatchResult{
		BatchID:     batchID,
		TotalIcons:  len(req.Icons),
		TotalFiles:  totalFiles,
		Icons:       iconResults,
		DownloadURL: fmt.Sprintf("%s/batch/%s/download", s.storageURL, batchID),
	}, nil
}

// generateSingleIcon generates one icon with variations
func (s *Service) generateSingleIcon(ctx context.Context, userID, batchID string, iconReq domain.IconRequest, style domain.IconStyle, sizes []int, formats []domain.ImageFormat, variations int) domain.IconResult {
	result := domain.IconResult{
		Name:       iconReq.Name,
		Variations: make([]domain.IconVariation, 0, variations),
	}

	// Generate prompt for icon
	prompt := llm.GenerateIconPrompt(iconReq.Name, iconReq.Description, style)

	for v := 0; v < variations; v++ {
		llmReq := llm.GenerateImageRequest{
			Prompt:  prompt,
			Size:    "1024x1024", // Always generate at max size
			Quality: "standard",
			Style:   "vivid",
			N:       1,
		}

		resp, err := s.imageClient.GenerateImage(ctx, llmReq)
		if err != nil {
			logger.WithFields(map[string]any{
				"icon":      iconReq.Name,
				"variation": v,
				"error":     err.Error(),
			}).Warn("failed to generate icon variation")
			continue
		}

		// Create file URLs for each size/format combination
		files := make(map[string]string)
		variationID := uuid.New().String()

		for _, size := range sizes {
			for _, format := range formats {
				key := fmt.Sprintf("%s_%d", format, size)
				// In production, this would be the actual resized image URL
				// For now, we use the original URL with size params
				files[key] = fmt.Sprintf("%s?size=%d&format=%s", resp.URL, size, format)
			}
		}

		variation := domain.IconVariation{
			ID:         variationID,
			PreviewURL: resp.URL,
			Files:      files,
		}
		result.Variations = append(result.Variations, variation)
	}

	return result
}

// =============================================================================
// Poster Generation
// =============================================================================

// GeneratePoster generates poster/banner images
func (s *Service) GeneratePoster(ctx context.Context, userID string, req domain.PosterRequest) ([]domain.Image, error) {
	if s.imageClient == nil {
		return nil, fmt.Errorf("image client not configured")
	}

	// Get brand kit if specified
	var brandKit *domain.BrandKit
	if req.BrandKitID != "" && s.brandKitRepo != nil {
		bk, err := s.brandKitRepo.GetByID(ctx, req.BrandKitID)
		if err == nil {
			brandKit = bk
		}
	}

	// Determine size
	var width, height int
	var size string
	if req.CustomSize != nil {
		width = req.CustomSize.Width
		height = req.CustomSize.Height
		size = llm.GetDALLESize(width, height)
	} else if req.Preset != "" {
		width, height = domain.GetPresetDimensions(req.Preset)
		size = llm.GetDALLESizeFromPreset(req.Preset)
	} else {
		size = "1024x1024"
		width, height = 1024, 1024
	}

	// Build poster prompt
	prompt := llm.GeneratePosterPrompt(req, brandKit)

	count := req.Count
	if count <= 0 {
		count = 1
	}
	if count > 4 {
		count = 4
	}

	// Generate posters
	llmReq := llm.GenerateImageRequest{
		Prompt:   prompt,
		Size:     size,
		Quality:  "hd", // Posters should be high quality
		Style:    "vivid",
		N:        count,
		BrandKit: brandKit,
	}

	results, err := s.imageClient.GenerateImages(ctx, llmReq)
	if err != nil {
		return nil, fmt.Errorf("failed to generate poster: %w", err)
	}

	batchID := uuid.New().String()

	// Create image records
	images := make([]domain.Image, len(results))
	for i, result := range results {
		images[i] = domain.Image{
			ID:              uuid.New().String(),
			UserID:          userID,
			Type:            domain.ImageTypePoster,
			Prompt:          req.Prompt,
			OptimizedPrompt: result.RevisedPrompt,
			Style:           domain.ImageStyle(req.Style.Mood),
			Quality:         domain.ImageQualityHigh,
			Status:          domain.ImageStatusCompleted,
			Width:           width,
			Height:          height,
			Preset:          req.Preset,
			URL:             result.URL,
			Format:          domain.ImageFormatPNG,
			BrandKitID:      req.BrandKitID,
			BatchID:         batchID,
			VariationNum:    i + 1,
			Metadata: map[string]any{
				"style":    req.Style,
				"elements": req.Elements,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}

	// Save to repository
	if s.imageRepo != nil {
		imagePtrs := make([]*domain.Image, len(images))
		for i := range images {
			imagePtrs[i] = &images[i]
		}
		if err := s.imageRepo.SaveBatch(ctx, imagePtrs); err != nil {
			logger.WithFields(map[string]any{"error": err.Error()}).Warn("failed to save posters to repository")
		}
	}

	return images, nil
}

// =============================================================================
// Multi-Size Generation
// =============================================================================

// GenerateMultiSize generates the same design in multiple sizes
func (s *Service) GenerateMultiSize(ctx context.Context, userID string, req domain.MultiSizeRequest) (*domain.MultiSizeResult, error) {
	if s.imageClient == nil {
		return nil, fmt.Errorf("image client not configured")
	}

	// Get brand kit if specified
	var brandKit *domain.BrandKit
	if req.BrandKitID != "" && s.brandKitRepo != nil {
		bk, err := s.brandKitRepo.GetByID(ctx, req.BrandKitID)
		if err == nil {
			brandKit = bk
		}
	}

	batchID := uuid.New().String()
	results := make([]domain.SizeDesignResult, 0, len(req.Sizes))

	// Generate for each size
	for _, sizeReq := range req.Sizes {
		width, height := domain.GetPresetDimensions(sizeReq.Preset)
		dalleSize := llm.GetDALLESizeFromPreset(sizeReq.Preset)

		// Build prompt with layout adaptation hints
		prompt := req.Prompt
		if req.AdaptLayout {
			prompt = adaptPromptForSize(prompt, sizeReq.Preset, req.BaseDesign)
		}

		// Apply brand context
		if brandKit != nil {
			prompt = brandKit.ToBrandPromptContext() + prompt
		}

		llmReq := llm.GenerateImageRequest{
			Prompt:   prompt,
			Size:     dalleSize,
			Quality:  "hd",
			Style:    "vivid",
			N:        1,
			BrandKit: brandKit,
		}

		resp, err := s.imageClient.GenerateImage(ctx, llmReq)
		if err != nil {
			logger.WithFields(map[string]any{
				"preset": sizeReq.Preset,
				"error":  err.Error(),
			}).Warn("failed to generate size variant")
			continue
		}

		image := domain.Image{
			ID:              uuid.New().String(),
			UserID:          userID,
			Type:            domain.ImageTypePoster,
			Prompt:          req.Prompt,
			OptimizedPrompt: resp.RevisedPrompt,
			Status:          domain.ImageStatusCompleted,
			Width:           width,
			Height:          height,
			Preset:          sizeReq.Preset,
			URL:             resp.URL,
			Format:          domain.ImageFormatPNG,
			BrandKitID:      req.BrandKitID,
			BatchID:         batchID,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		sizeResult := domain.SizeDesignResult{
			Size:       sizeReq.Preset,
			Use:        sizeReq.Use,
			Dimensions: fmt.Sprintf("%dx%d", width, height),
			Images:     []domain.Image{image},
		}

		if req.AdaptLayout {
			sizeResult.AdaptationNotes = getAdaptationNotes(sizeReq.Preset)
		}

		results = append(results, sizeResult)
	}

	return &domain.MultiSizeResult{
		BatchID:     batchID,
		Designs:     results,
		DownloadURL: fmt.Sprintf("%s/batch/%s/download", s.storageURL, batchID),
	}, nil
}

// =============================================================================
// Brand Kit Operations
// =============================================================================

// GetBrandKit retrieves a brand kit by ID
func (s *Service) GetBrandKit(ctx context.Context, userID string, brandKitID string) (*domain.BrandKit, error) {
	if s.brandKitRepo == nil {
		return nil, fmt.Errorf("brand kit repository not configured")
	}

	brandKit, err := s.brandKitRepo.GetByID(ctx, brandKitID)
	if err != nil {
		return nil, fmt.Errorf("failed to get brand kit: %w", err)
	}

	// Verify ownership
	if brandKit.UserID != userID {
		return nil, fmt.Errorf("brand kit not found")
	}

	return brandKit, nil
}

// GetUserBrandKits retrieves all brand kits for a user
func (s *Service) GetUserBrandKits(ctx context.Context, userID string) ([]*domain.BrandKit, error) {
	if s.brandKitRepo == nil {
		return nil, fmt.Errorf("brand kit repository not configured")
	}

	return s.brandKitRepo.GetByUserID(ctx, userID)
}

// CreateBrandKit creates a new brand kit
func (s *Service) CreateBrandKit(ctx context.Context, brandKit *domain.BrandKit) error {
	if s.brandKitRepo == nil {
		return fmt.Errorf("brand kit repository not configured")
	}

	brandKit.ID = uuid.New().String()
	brandKit.CreatedAt = time.Now()
	brandKit.UpdatedAt = time.Now()

	return s.brandKitRepo.Save(ctx, brandKit)
}

// UpdateBrandKit updates an existing brand kit
func (s *Service) UpdateBrandKit(ctx context.Context, brandKit *domain.BrandKit) error {
	if s.brandKitRepo == nil {
		return fmt.Errorf("brand kit repository not configured")
	}

	brandKit.UpdatedAt = time.Now()
	return s.brandKitRepo.Update(ctx, brandKit)
}

// DeleteBrandKit deletes a brand kit
func (s *Service) DeleteBrandKit(ctx context.Context, id string) error {
	if s.brandKitRepo == nil {
		return fmt.Errorf("brand kit repository not configured")
	}

	return s.brandKitRepo.Delete(ctx, id)
}

// =============================================================================
// Image Operations
// =============================================================================

// GetImage retrieves an image by ID
func (s *Service) GetImage(ctx context.Context, userID string, imageID string) (*domain.Image, error) {
	if s.imageRepo == nil {
		return nil, fmt.Errorf("image repository not configured")
	}

	image, err := s.imageRepo.GetByID(ctx, imageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get image: %w", err)
	}

	// Verify ownership
	if image.UserID != userID {
		return nil, fmt.Errorf("image not found")
	}

	return image, nil
}

// GetUserImages retrieves images for a user with pagination
func (s *Service) GetUserImages(ctx context.Context, userID string, limit, offset int) ([]*domain.Image, error) {
	if s.imageRepo == nil {
		return nil, fmt.Errorf("image repository not configured")
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	return s.imageRepo.GetByUserID(ctx, userID, limit, offset)
}

// GetBatchImages retrieves all images in a batch
func (s *Service) GetBatchImages(ctx context.Context, batchID string) ([]*domain.Image, error) {
	if s.imageRepo == nil {
		return nil, fmt.Errorf("image repository not configured")
	}

	return s.imageRepo.GetByBatchID(ctx, batchID)
}

// DeleteImage deletes an image
func (s *Service) DeleteImage(ctx context.Context, userID string, imageID string) error {
	if s.imageRepo == nil {
		return fmt.Errorf("image repository not configured")
	}

	// Verify ownership first
	image, err := s.imageRepo.GetByID(ctx, imageID)
	if err != nil {
		return fmt.Errorf("image not found: %w", err)
	}
	if image.UserID != userID {
		return fmt.Errorf("image not found")
	}

	return s.imageRepo.Delete(ctx, imageID)
}

// DownloadBatch creates a ZIP file containing all images in a batch
func (s *Service) DownloadBatch(ctx context.Context, batchID string) ([]byte, error) {
	if s.imageRepo == nil {
		return nil, fmt.Errorf("image repository not configured")
	}

	images, err := s.imageRepo.GetByBatchID(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch images: %w", err)
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("no images found in batch")
	}

	// Create ZIP file
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	for i, img := range images {
		// Download image from URL
		resp, err := http.Get(img.URL)
		if err != nil {
			logger.WithFields(map[string]any{"image_id": img.ID, "error": err.Error()}).Warn("failed to download image")
			continue
		}
		defer resp.Body.Close()

		// Create file in ZIP
		filename := fmt.Sprintf("%s_%d.png", sanitizeFilename(img.Type), i+1)
		writer, err := zipWriter.Create(filename)
		if err != nil {
			continue
		}

		io.Copy(writer, resp.Body)
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to create zip: %w", err)
	}

	return buf.Bytes(), nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// parseDimensions extracts width and height from DALL-E size string
func parseDimensions(size string) (int, int) {
	switch size {
	case "1024x1024":
		return 1024, 1024
	case "1792x1024":
		return 1792, 1024
	case "1024x1792":
		return 1024, 1792
	default:
		return 1024, 1024
	}
}

// adaptPromptForSize modifies prompt based on size/aspect ratio
func adaptPromptForSize(prompt string, preset domain.SizePreset, style domain.PosterStyle) string {
	width, height := domain.GetPresetDimensions(preset)
	ratio := float64(width) / float64(height)

	var layoutHint string
	switch {
	case ratio > 2.5: // Very wide (banners)
		layoutHint = "horizontal banner layout, elements spread across width, minimal vertical stacking"
	case ratio > 1.5: // Wide
		layoutHint = "landscape orientation, horizontal composition"
	case ratio < 0.5: // Very tall (stories)
		layoutHint = "vertical story layout, stacked elements, content flows top to bottom"
	case ratio < 0.8: // Tall
		layoutHint = "portrait orientation, vertical composition"
	default: // Square-ish
		layoutHint = "balanced square composition, centered layout"
	}

	return fmt.Sprintf("%s. Layout: %s", prompt, layoutHint)
}

// getAdaptationNotes returns notes about how layout was adapted
func getAdaptationNotes(preset domain.SizePreset) string {
	switch preset {
	case domain.SizePresetInstagramStory:
		return "Vertical layout optimized for mobile viewing, text positioned in safe zones"
	case domain.SizePresetFacebookCover:
		return "Wide banner layout, main content centered to avoid profile picture overlap"
	case domain.SizePresetLinkedInBanner:
		return "Professional wide format, content positioned for profile area clearance"
	case domain.SizePresetInstagramPost:
		return "Square format, centered composition for feed viewing"
	case domain.SizePresetYouTubeThumbnail:
		return "16:9 format, bold text optimized for small preview sizes"
	default:
		return "Layout adapted for target dimensions"
	}
}

// sanitizeFilename removes invalid characters from filename
func sanitizeFilename(t domain.ImageType) string {
	name := string(t)
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	return name
}
