package http

import (
	"worker_server/core/domain"
	"worker_server/core/service/image"

	"github.com/gofiber/fiber/v2"
)

// ImageHandler handles image generation API requests
type ImageHandler struct {
	imageService *image.Service
}

// NewImageHandler creates a new image handler
func NewImageHandler(imageService *image.Service) *ImageHandler {
	return &ImageHandler{imageService: imageService}
}

// Register registers all image routes
func (h *ImageHandler) Register(app fiber.Router) {
	img := app.Group("/images")

	// Simple image generation
	img.Post("/generate", h.GenerateImage)
	img.Post("/generate/batch", h.GenerateImages)

	// Icon generation
	img.Post("/icons/batch", h.GenerateIconBatch)

	// Poster generation
	img.Post("/posters", h.GeneratePoster)
	img.Post("/posters/multi-size", h.GenerateMultiSize)

	// Image management
	img.Get("/", h.ListImages)
	img.Get("/:id", h.GetImage)
	img.Delete("/:id", h.DeleteImage)
	img.Get("/batch/:batchId", h.GetBatchImages)
	img.Get("/batch/:batchId/download", h.DownloadBatch)

	// Brand kit management
	brandKit := app.Group("/brand-kits")
	brandKit.Get("/", h.ListBrandKits)
	brandKit.Post("/", h.CreateBrandKit)
	brandKit.Get("/:id", h.GetBrandKit)
	brandKit.Put("/:id", h.UpdateBrandKit)
	brandKit.Delete("/:id", h.DeleteBrandKit)
}

// =============================================================================
// Image Generation Endpoints
// =============================================================================

// GenerateImageRequest represents the request for simple image generation
type GenerateImageRequest struct {
	Prompt      string              `json:"prompt"`
	Type        domain.ImageType    `json:"type,omitempty"`
	Style       domain.ImageStyle   `json:"style,omitempty"`
	Quality     domain.ImageQuality `json:"quality,omitempty"`
	AspectRatio domain.AspectRatio  `json:"aspect_ratio,omitempty"`
	Preset      domain.SizePreset   `json:"preset,omitempty"`
	BrandKitID  string              `json:"brand_kit_id,omitempty"`
}

// GenerateImage generates a single image
// POST /images/generate
func (h *ImageHandler) GenerateImage(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req GenerateImageRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if req.Prompt == "" {
		return ErrorResponse(c, 400, "prompt is required")
	}

	// Set defaults
	if req.Type == "" {
		req.Type = domain.ImageTypeIllustration
	}
	if req.Quality == "" {
		req.Quality = domain.ImageQualityStandard
	}

	domainReq := domain.GenerateImageRequest{
		Prompt:      req.Prompt,
		Type:        req.Type,
		Style:       req.Style,
		Quality:     req.Quality,
		AspectRatio: req.AspectRatio,
		Preset:      req.Preset,
		BrandKitID:  req.BrandKitID,
		Count:       1,
	}

	image, err := h.imageService.GenerateImage(c.Context(), userID.String(), domainReq)
	if err != nil {
		return InternalErrorResponse(c, err, "image generation")
	}

	return c.JSON(fiber.Map{
		"success": true,
		"image":   image,
	})
}

// GenerateImagesRequest represents the request for batch image generation
type GenerateImagesRequest struct {
	Prompt      string              `json:"prompt"`
	Type        domain.ImageType    `json:"type,omitempty"`
	Style       domain.ImageStyle   `json:"style,omitempty"`
	Quality     domain.ImageQuality `json:"quality,omitempty"`
	AspectRatio domain.AspectRatio  `json:"aspect_ratio,omitempty"`
	Preset      domain.SizePreset   `json:"preset,omitempty"`
	BrandKitID  string              `json:"brand_kit_id,omitempty"`
	Count       int                 `json:"count,omitempty"` // 1-4
}

// GenerateImages generates multiple image variations
// POST /images/generate/batch
func (h *ImageHandler) GenerateImages(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req GenerateImagesRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if req.Prompt == "" {
		return ErrorResponse(c, 400, "prompt is required")
	}

	// Set defaults
	if req.Type == "" {
		req.Type = domain.ImageTypeIllustration
	}
	if req.Quality == "" {
		req.Quality = domain.ImageQualityStandard
	}
	if req.Count <= 0 {
		req.Count = 2
	}
	if req.Count > 4 {
		req.Count = 4
	}

	domainReq := domain.GenerateImageRequest{
		Prompt:      req.Prompt,
		Type:        req.Type,
		Style:       req.Style,
		Quality:     req.Quality,
		AspectRatio: req.AspectRatio,
		Preset:      req.Preset,
		BrandKitID:  req.BrandKitID,
		Count:       req.Count,
	}

	images, err := h.imageService.GenerateImages(c.Context(), userID.String(), domainReq)
	if err != nil {
		return InternalErrorResponse(c, err, "image generation")
	}

	return c.JSON(fiber.Map{
		"success": true,
		"images":  images,
		"count":   len(images),
	})
}

// =============================================================================
// Icon Generation Endpoints
// =============================================================================

// IconBatchRequest represents the request for batch icon generation
type IconBatchRequest struct {
	TemplateID string               `json:"template_id,omitempty"`
	Style      *domain.IconStyle    `json:"style,omitempty"`
	Icons      []domain.IconRequest `json:"icons"`
	Sizes      []int                `json:"sizes,omitempty"`
	Formats    []domain.ImageFormat `json:"formats,omitempty"`
	Variations int                  `json:"variations,omitempty"`
}

// GenerateIconBatch generates a batch of icons with consistent style
// POST /images/icons/batch
func (h *ImageHandler) GenerateIconBatch(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req IconBatchRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if len(req.Icons) == 0 {
		return ErrorResponse(c, 400, "at least one icon is required")
	}

	if len(req.Icons) > 10 {
		return ErrorResponse(c, 400, "maximum 10 icons per batch")
	}

	domainReq := domain.IconBatchRequest{
		TemplateID: req.TemplateID,
		Style:      req.Style,
		Icons:      req.Icons,
		Sizes:      req.Sizes,
		Formats:    req.Formats,
		Variations: req.Variations,
	}

	result, err := h.imageService.GenerateIconBatch(c.Context(), userID.String(), domainReq)
	if err != nil {
		return InternalErrorResponse(c, err, "icon batch generation")
	}

	return c.JSON(fiber.Map{
		"success": true,
		"result":  result,
	})
}

// =============================================================================
// Poster Generation Endpoints
// =============================================================================

// PosterRequest represents the request for poster generation
type PosterRequest struct {
	Prompt     string                 `json:"prompt"`
	Preset     domain.SizePreset      `json:"preset,omitempty"`
	CustomSize *domain.ImageSize      `json:"custom_size,omitempty"`
	Style      domain.PosterStyle     `json:"style,omitempty"`
	Elements   *domain.PosterElements `json:"elements,omitempty"`
	BrandKitID string                 `json:"brand_kit_id,omitempty"`
	Count      int                    `json:"count,omitempty"`
}

// GeneratePoster generates poster/banner images
// POST /images/posters
func (h *ImageHandler) GeneratePoster(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req PosterRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if req.Prompt == "" {
		return ErrorResponse(c, 400, "prompt is required")
	}

	domainReq := domain.PosterRequest{
		Prompt:     req.Prompt,
		Preset:     req.Preset,
		CustomSize: req.CustomSize,
		Style:      req.Style,
		Elements:   req.Elements,
		BrandKitID: req.BrandKitID,
		Count:      req.Count,
	}

	images, err := h.imageService.GeneratePoster(c.Context(), userID.String(), domainReq)
	if err != nil {
		return InternalErrorResponse(c, err, "poster generation")
	}

	return c.JSON(fiber.Map{
		"success": true,
		"images":  images,
		"count":   len(images),
	})
}

// MultiSizeRequest represents the request for multi-size generation
type MultiSizeRequest struct {
	Prompt      string                 `json:"prompt"`
	BaseDesign  domain.PosterStyle     `json:"base_design,omitempty"`
	Elements    *domain.PosterElements `json:"elements,omitempty"`
	Sizes       []domain.SizeRequest   `json:"sizes"`
	BrandKitID  string                 `json:"brand_kit_id,omitempty"`
	AdaptLayout bool                   `json:"adapt_layout,omitempty"`
}

// GenerateMultiSize generates the same design in multiple sizes
// POST /images/posters/multi-size
func (h *ImageHandler) GenerateMultiSize(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req MultiSizeRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if req.Prompt == "" {
		return ErrorResponse(c, 400, "prompt is required")
	}

	if len(req.Sizes) == 0 {
		return ErrorResponse(c, 400, "at least one size is required")
	}

	if len(req.Sizes) > 5 {
		return ErrorResponse(c, 400, "maximum 5 sizes per request")
	}

	domainReq := domain.MultiSizeRequest{
		Prompt:      req.Prompt,
		BaseDesign:  req.BaseDesign,
		Elements:    req.Elements,
		Sizes:       req.Sizes,
		BrandKitID:  req.BrandKitID,
		AdaptLayout: req.AdaptLayout,
	}

	result, err := h.imageService.GenerateMultiSize(c.Context(), userID.String(), domainReq)
	if err != nil {
		return InternalErrorResponse(c, err, "multi-size generation")
	}

	return c.JSON(fiber.Map{
		"success": true,
		"result":  result,
	})
}

// =============================================================================
// Image Management Endpoints
// =============================================================================

// ListImages returns paginated list of user's images
// GET /images?limit=20&offset=0
func (h *ImageHandler) ListImages(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	limit := c.QueryInt("limit", 20)
	offset := c.QueryInt("offset", 0)

	images, err := h.imageService.GetUserImages(c.Context(), userID.String(), limit, offset)
	if err != nil {
		return InternalErrorResponse(c, err, "list images")
	}

	return c.JSON(fiber.Map{
		"images": images,
		"count":  len(images),
		"limit":  limit,
		"offset": offset,
	})
}

// GetImage returns a single image by ID
// GET /images/:id
func (h *ImageHandler) GetImage(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	imageID := c.Params("id")
	if imageID == "" {
		return ErrorResponse(c, 400, "image id is required")
	}

	image, err := h.imageService.GetImage(c.Context(), userID.String(), imageID)
	if err != nil {
		return ErrorResponse(c, 404, "image not found")
	}

	return c.JSON(fiber.Map{
		"image": image,
	})
}

// DeleteImage deletes an image
// DELETE /images/:id
func (h *ImageHandler) DeleteImage(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	imageID := c.Params("id")
	if imageID == "" {
		return ErrorResponse(c, 400, "image id is required")
	}

	if err := h.imageService.DeleteImage(c.Context(), userID.String(), imageID); err != nil {
		return InternalErrorResponse(c, err, "delete image")
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "image deleted",
	})
}

// GetBatchImages returns all images in a batch
// GET /images/batch/:batchId
func (h *ImageHandler) GetBatchImages(c *fiber.Ctx) error {
	batchID := c.Params("batchId")
	if batchID == "" {
		return ErrorResponse(c, 400, "batch id is required")
	}

	images, err := h.imageService.GetBatchImages(c.Context(), batchID)
	if err != nil {
		return InternalErrorResponse(c, err, "get batch images")
	}

	return c.JSON(fiber.Map{
		"batch_id": batchID,
		"images":   images,
		"count":    len(images),
	})
}

// DownloadBatch downloads all images in a batch as ZIP
// GET /images/batch/:batchId/download
func (h *ImageHandler) DownloadBatch(c *fiber.Ctx) error {
	batchID := c.Params("batchId")
	if batchID == "" {
		return ErrorResponse(c, 400, "batch id is required")
	}

	zipData, err := h.imageService.DownloadBatch(c.Context(), batchID)
	if err != nil {
		return InternalErrorResponse(c, err, "download batch")
	}

	c.Set("Content-Type", "application/zip")
	c.Set("Content-Disposition", "attachment; filename=\"images-"+batchID+".zip\"")

	return c.Send(zipData)
}

// =============================================================================
// Brand Kit Endpoints
// =============================================================================

// BrandKitRequest represents the request for creating/updating a brand kit
type BrandKitRequest struct {
	Name       string                  `json:"name"`
	Colors     *domain.BrandColors     `json:"colors,omitempty"`
	Fonts      *domain.BrandFonts      `json:"fonts,omitempty"`
	Logo       *domain.BrandLogo       `json:"logo,omitempty"`
	StyleGuide *domain.BrandStyleGuide `json:"style_guide,omitempty"`
	IsDefault  bool                    `json:"is_default,omitempty"`
}

// ListBrandKits returns all brand kits for the user
// GET /brand-kits
func (h *ImageHandler) ListBrandKits(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	brandKits, err := h.imageService.GetUserBrandKits(c.Context(), userID.String())
	if err != nil {
		return InternalErrorResponse(c, err, "list brand kits")
	}

	return c.JSON(fiber.Map{
		"brand_kits": brandKits,
		"count":      len(brandKits),
	})
}

// CreateBrandKit creates a new brand kit
// POST /brand-kits
func (h *ImageHandler) CreateBrandKit(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	var req BrandKitRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	if req.Name == "" {
		return ErrorResponse(c, 400, "name is required")
	}

	brandKit := &domain.BrandKit{
		UserID:    userID.String(),
		Name:      req.Name,
		IsDefault: req.IsDefault,
	}
	if req.Colors != nil {
		brandKit.Colors = *req.Colors
	}
	if req.Fonts != nil {
		brandKit.Fonts = *req.Fonts
	}
	if req.Logo != nil {
		brandKit.Logo = *req.Logo
	}
	if req.StyleGuide != nil {
		brandKit.StyleGuide = *req.StyleGuide
	}

	if err := h.imageService.CreateBrandKit(c.Context(), brandKit); err != nil {
		return InternalErrorResponse(c, err, "create brand kit")
	}

	return c.Status(201).JSON(fiber.Map{
		"success":   true,
		"brand_kit": brandKit,
	})
}

// GetBrandKit returns a single brand kit by ID
// GET /brand-kits/:id
func (h *ImageHandler) GetBrandKit(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	brandKitID := c.Params("id")
	if brandKitID == "" {
		return ErrorResponse(c, 400, "brand kit id is required")
	}

	brandKit, err := h.imageService.GetBrandKit(c.Context(), userID.String(), brandKitID)
	if err != nil {
		return ErrorResponse(c, 404, "brand kit not found")
	}

	return c.JSON(fiber.Map{
		"brand_kit": brandKit,
	})
}

// UpdateBrandKit updates a brand kit
// PUT /brand-kits/:id
func (h *ImageHandler) UpdateBrandKit(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	brandKitID := c.Params("id")
	if brandKitID == "" {
		return ErrorResponse(c, 400, "brand kit id is required")
	}

	// Get existing brand kit to verify ownership
	existing, err := h.imageService.GetBrandKit(c.Context(), userID.String(), brandKitID)
	if err != nil {
		return ErrorResponse(c, 404, "brand kit not found")
	}

	var req BrandKitRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, 400, "invalid request body")
	}

	// Update fields
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Colors != nil {
		existing.Colors = *req.Colors
	}
	if req.Fonts != nil {
		existing.Fonts = *req.Fonts
	}
	if req.Logo != nil {
		existing.Logo = *req.Logo
	}
	if req.StyleGuide != nil {
		existing.StyleGuide = *req.StyleGuide
	}
	if req.IsDefault {
		existing.IsDefault = req.IsDefault
	}

	if err := h.imageService.UpdateBrandKit(c.Context(), existing); err != nil {
		return InternalErrorResponse(c, err, "update brand kit")
	}

	return c.JSON(fiber.Map{
		"success":   true,
		"brand_kit": existing,
	})
}

// DeleteBrandKit deletes a brand kit
// DELETE /brand-kits/:id
func (h *ImageHandler) DeleteBrandKit(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	brandKitID := c.Params("id")
	if brandKitID == "" {
		return ErrorResponse(c, 400, "brand kit id is required")
	}

	// Verify ownership first
	_, err = h.imageService.GetBrandKit(c.Context(), userID.String(), brandKitID)
	if err != nil {
		return ErrorResponse(c, 404, "brand kit not found")
	}

	if err := h.imageService.DeleteBrandKit(c.Context(), brandKitID); err != nil {
		return InternalErrorResponse(c, err, "delete brand kit")
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "brand kit deleted",
	})
}
