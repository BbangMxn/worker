package domain

import (
	"time"
)

// ImageType represents the type of image generation
type ImageType string

const (
	ImageTypeIcon         ImageType = "icon"
	ImageTypePoster       ImageType = "poster"
	ImageTypeInfographic  ImageType = "infographic"
	ImageTypeSocial       ImageType = "social"
	ImageTypeMockup       ImageType = "mockup"
	ImageTypeIllustration ImageType = "illustration"
	ImageTypeDiagram      ImageType = "diagram"
	ImageTypeBackground   ImageType = "background"
)

// ImageStyle represents the visual style for image generation
type ImageStyle string

const (
	ImageStyleFlat          ImageStyle = "flat"
	ImageStyleOutline       ImageStyle = "outline"
	ImageStyleFilled        ImageStyle = "filled"
	ImageStyle3D            ImageStyle = "3d"
	ImageStyleGradient      ImageStyle = "gradient"
	ImageStyleGlassmorphism ImageStyle = "glassmorphism"
	ImageStyleMinimal       ImageStyle = "minimal"
	ImageStyleCorporate     ImageStyle = "corporate"
	ImageStyleHandDrawn     ImageStyle = "hand_drawn"
	ImageStyleIsometric     ImageStyle = "isometric"
	ImageStyleWatercolor    ImageStyle = "watercolor"
	ImageStyleCartoon       ImageStyle = "cartoon"
	ImageStyleGeometric     ImageStyle = "geometric"
	ImageStyleLineArt       ImageStyle = "line_art"
	ImageStyleRealistic     ImageStyle = "realistic"
	ImageStyleArtistic      ImageStyle = "artistic"
)

// ImageQuality represents the quality level for image generation
type ImageQuality string

const (
	ImageQualityDraft    ImageQuality = "draft"    // Fast, lower cost
	ImageQualityStandard ImageQuality = "standard" // Normal quality
	ImageQualityHigh     ImageQuality = "high"     // High quality, higher cost
)

// ImageFormat represents the output format
type ImageFormat string

const (
	ImageFormatPNG  ImageFormat = "png"
	ImageFormatJPEG ImageFormat = "jpeg"
	ImageFormatSVG  ImageFormat = "svg"
	ImageFormatWEBP ImageFormat = "webp"
)

// ImageStatus represents the generation status
type ImageStatus string

const (
	ImageStatusPending    ImageStatus = "pending"
	ImageStatusProcessing ImageStatus = "processing"
	ImageStatusCompleted  ImageStatus = "completed"
	ImageStatusFailed     ImageStatus = "failed"
)

// AspectRatio represents common aspect ratios
type AspectRatio string

const (
	AspectRatio1x1  AspectRatio = "1:1"
	AspectRatio4x3  AspectRatio = "4:3"
	AspectRatio3x4  AspectRatio = "3:4"
	AspectRatio16x9 AspectRatio = "16:9"
	AspectRatio9x16 AspectRatio = "9:16"
)

// SizePreset represents predefined size presets
type SizePreset string

const (
	// Social Media
	SizePresetInstagramPost    SizePreset = "instagram_post"    // 1080x1080
	SizePresetInstagramStory   SizePreset = "instagram_story"   // 1080x1920
	SizePresetFacebookCover    SizePreset = "facebook_cover"    // 820x312
	SizePresetLinkedInBanner   SizePreset = "linkedin_banner"   // 1584x396
	SizePresetTwitterHeader    SizePreset = "twitter_header"    // 1500x500
	SizePresetYouTubeThumbnail SizePreset = "youtube_thumbnail" // 1280x720

	// Presentation
	SizePresetSlide16x9 SizePreset = "slide_16_9" // 1920x1080
	SizePresetSlide4x3  SizePreset = "slide_4_3"  // 1024x768
	SizePresetKeynote   SizePreset = "keynote"    // 1920x1080

	// Print
	SizePresetA4Portrait   SizePreset = "a4_portrait"   // 2480x3508
	SizePresetA4Landscape  SizePreset = "a4_landscape"  // 3508x2480
	SizePresetBusinessCard SizePreset = "business_card" // 1050x600

	// Web
	SizePresetHeroBanner  SizePreset = "hero_banner"  // 1920x600
	SizePresetOGImage     SizePreset = "og_image"     // 1200x630
	SizePresetEmailHeader SizePreset = "email_header" // 600x200
)

// GetPresetDimensions returns width and height for a preset
func GetPresetDimensions(preset SizePreset) (width, height int) {
	switch preset {
	// Social Media
	case SizePresetInstagramPost:
		return 1080, 1080
	case SizePresetInstagramStory:
		return 1080, 1920
	case SizePresetFacebookCover:
		return 820, 312
	case SizePresetLinkedInBanner:
		return 1584, 396
	case SizePresetTwitterHeader:
		return 1500, 500
	case SizePresetYouTubeThumbnail:
		return 1280, 720
	// Presentation
	case SizePresetSlide16x9, SizePresetKeynote:
		return 1920, 1080
	case SizePresetSlide4x3:
		return 1024, 768
	// Print
	case SizePresetA4Portrait:
		return 2480, 3508
	case SizePresetA4Landscape:
		return 3508, 2480
	case SizePresetBusinessCard:
		return 1050, 600
	// Web
	case SizePresetHeroBanner:
		return 1920, 600
	case SizePresetOGImage:
		return 1200, 630
	case SizePresetEmailHeader:
		return 600, 200
	default:
		return 1024, 1024
	}
}

// Image represents a generated image
type Image struct {
	ID              string       `json:"id"`
	UserID          string       `json:"user_id"`
	Type            ImageType    `json:"type"`
	Prompt          string       `json:"prompt"`
	OptimizedPrompt string       `json:"optimized_prompt,omitempty"`
	Style           ImageStyle   `json:"style"`
	Quality         ImageQuality `json:"quality"`
	Status          ImageStatus  `json:"status"`

	// Dimensions
	Width  int        `json:"width"`
	Height int        `json:"height"`
	Preset SizePreset `json:"preset,omitempty"`

	// Output
	URL          string      `json:"url,omitempty"`
	ThumbnailURL string      `json:"thumbnail_url,omitempty"`
	Format       ImageFormat `json:"format"`

	// Metadata
	BrandKitID string         `json:"brand_kit_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Tags       []string       `json:"tags,omitempty"`

	// Batch info
	BatchID      string `json:"batch_id,omitempty"`
	VariationOf  string `json:"variation_of,omitempty"`
	VariationNum int    `json:"variation_num,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Error info (if failed)
	ErrorMessage string `json:"error_message,omitempty"`
}

// ImageBatch represents a batch of generated images
type ImageBatch struct {
	ID             string      `json:"id"`
	UserID         string      `json:"user_id"`
	Type           ImageType   `json:"type"`
	Status         ImageStatus `json:"status"`
	TotalCount     int         `json:"total_count"`
	CompletedCount int         `json:"completed_count"`
	FailedCount    int         `json:"failed_count"`
	Images         []Image     `json:"images,omitempty"`
	DownloadURL    string      `json:"download_url,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	CompletedAt    *time.Time  `json:"completed_at,omitempty"`
}

// IconTemplate represents an icon style template
type IconTemplate struct {
	ID          string          `json:"id"`
	UserID      string          `json:"user_id"`
	Name        string          `json:"name"`
	Style       IconStyle       `json:"style"`
	Constraints IconConstraints `json:"constraints"`
	PreviewURL  string          `json:"preview_url,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// IconStyle defines the visual style for icons
type IconStyle struct {
	Type         ImageStyle `json:"type"`          // flat, outline, filled, etc.
	CornerRadius string     `json:"corner_radius"` // sharp, rounded, circle
	StrokeWidth  int        `json:"stroke_width"`  // for outline style (px)
	Colors       IconColors `json:"colors"`
	Background   string     `json:"background"` // transparent, solid, gradient
	Shadow       string     `json:"shadow"`     // none, soft, hard
	SizeBase     int        `json:"size_base"`  // base size in px
}

// IconColors defines color scheme for icons
type IconColors struct {
	Primary   string `json:"primary"`
	Secondary string `json:"secondary"`
	Accent    string `json:"accent,omitempty"`
}

// IconConstraints defines constraints for icon generation
type IconConstraints struct {
	MaxElements  int     `json:"max_elements"`  // max elements in icon
	Symmetry     string  `json:"symmetry"`      // required, preferred, none
	PaddingRatio float64 `json:"padding_ratio"` // padding as ratio of size
}

// IconRequest represents a single icon to generate
type IconRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// IconBatchRequest represents a batch icon generation request
type IconBatchRequest struct {
	TemplateID string        `json:"template_id,omitempty"`
	Style      *IconStyle    `json:"style,omitempty"` // inline style if no template
	Icons      []IconRequest `json:"icons"`
	Sizes      []int         `json:"sizes"`                // e.g., [16, 24, 32, 48, 64, 128]
	Formats    []ImageFormat `json:"formats"`              // e.g., [svg, png]
	Variations int           `json:"variations,omitempty"` // variations per icon
}

// IconBatchResult represents the result of batch icon generation
type IconBatchResult struct {
	BatchID     string       `json:"batch_id"`
	TotalIcons  int          `json:"total_icons"`
	TotalFiles  int          `json:"total_files"`
	Icons       []IconResult `json:"icons"`
	DownloadURL string       `json:"download_all_url,omitempty"`
}

// IconResult represents a generated icon with variations
type IconResult struct {
	Name       string          `json:"name"`
	Variations []IconVariation `json:"variations"`
}

// IconVariation represents one variation of an icon
type IconVariation struct {
	ID         string            `json:"id"`
	PreviewURL string            `json:"preview_url"`
	Files      map[string]string `json:"files"` // format_size -> url (e.g., "png_24" -> "https://...")
}

// PosterRequest represents a poster/banner generation request
type PosterRequest struct {
	Prompt     string          `json:"prompt"`
	Preset     SizePreset      `json:"preset,omitempty"`
	CustomSize *ImageSize      `json:"custom_size,omitempty"`
	Style      PosterStyle     `json:"style"`
	Elements   *PosterElements `json:"elements,omitempty"`
	BrandKitID string          `json:"brand_kit_id,omitempty"`
	Count      int             `json:"count,omitempty"` // number of variations
}

// ImageSize represents custom dimensions
type ImageSize struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Unit   string `json:"unit,omitempty"` // px (default)
}

// PosterStyle defines style options for posters
type PosterStyle struct {
	Mood        string `json:"mood,omitempty"`         // festive, professional, minimal, bold, elegant
	ColorScheme string `json:"color_scheme,omitempty"` // brand, warm, cool, monochrome, vibrant
	Layout      string `json:"layout,omitempty"`       // centered, left_aligned, split, overlapping
}

// PosterElements defines text and visual elements
type PosterElements struct {
	Headline     string `json:"headline,omitempty"`
	Subheadline  string `json:"subheadline,omitempty"`
	CTA          string `json:"cta,omitempty"`           // call to action
	LogoPosition string `json:"logo_position,omitempty"` // top_left, top_right, bottom_left, bottom_right
	IncludeDate  string `json:"include_date,omitempty"`
}

// MultiSizeRequest represents a request to generate same design in multiple sizes
type MultiSizeRequest struct {
	Prompt      string          `json:"prompt"`
	BaseDesign  PosterStyle     `json:"base_design"`
	Elements    *PosterElements `json:"elements,omitempty"`
	Sizes       []SizeRequest   `json:"sizes"`
	BrandKitID  string          `json:"brand_kit_id,omitempty"`
	AdaptLayout bool            `json:"adapt_layout"` // auto-adjust layout for each size
}

// SizeRequest represents a single size in multi-size request
type SizeRequest struct {
	Preset SizePreset `json:"preset"`
	Use    string     `json:"use,omitempty"` // description of use case
}

// MultiSizeResult represents the result of multi-size generation
type MultiSizeResult struct {
	BatchID     string             `json:"batch_id"`
	Designs     []SizeDesignResult `json:"designs"`
	DownloadURL string             `json:"download_all_url,omitempty"`
}

// SizeDesignResult represents one size variant
type SizeDesignResult struct {
	Size            SizePreset `json:"size"`
	Use             string     `json:"use,omitempty"`
	Dimensions      string     `json:"dimensions"`
	Images          []Image    `json:"images"`
	AdaptationNotes string     `json:"adaptation_notes,omitempty"`
}

// InfographicType represents types of infographics
type InfographicType string

const (
	InfographicTypeStatistics InfographicType = "statistics"
	InfographicTypeTimeline   InfographicType = "timeline"
	InfographicTypeComparison InfographicType = "comparison"
	InfographicTypeProcess    InfographicType = "process"
	InfographicTypeHierarchy  InfographicType = "hierarchy"
	InfographicTypeGeographic InfographicType = "geographic"
	InfographicTypeList       InfographicType = "list"
)

// InfographicRequest represents an infographic generation request
type InfographicRequest struct {
	Type       InfographicType       `json:"type"`
	Title      string                `json:"title"`
	Data       []InfographicDataItem `json:"data"`
	Style      InfographicStyle      `json:"style"`
	Preset     SizePreset            `json:"preset,omitempty"`
	BrandKitID string                `json:"brand_kit_id,omitempty"`
}

// InfographicDataItem represents a data point in infographic
type InfographicDataItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Icon  string `json:"icon,omitempty"`
}

// InfographicStyle defines style for infographics
type InfographicStyle struct {
	Layout      string     `json:"layout,omitempty"` // grid_2x2, horizontal, vertical, circular
	ColorScheme string     `json:"color_scheme,omitempty"`
	IconStyle   ImageStyle `json:"icon_style,omitempty"`
}

// DiagramType represents types of diagrams
type DiagramType string

const (
	DiagramTypeFlowchart DiagramType = "flowchart"
	DiagramTypeMindmap   DiagramType = "mindmap"
	DiagramTypeOrgChart  DiagramType = "org_chart"
	DiagramTypeNetwork   DiagramType = "network"
	DiagramTypeSequence  DiagramType = "sequence"
)

// DiagramRequest represents a diagram generation request
type DiagramRequest struct {
	Type        DiagramType         `json:"type"`
	Title       string              `json:"title"`
	Nodes       []DiagramNode       `json:"nodes"`
	Connections []DiagramConnection `json:"connections"`
	Style       DiagramStyle        `json:"style"`
	Direction   string              `json:"direction,omitempty"` // horizontal, vertical, radial
}

// DiagramNode represents a node in a diagram
type DiagramNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type,omitempty"` // start, end, process, decision
}

// DiagramConnection represents a connection between nodes
type DiagramConnection struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label,omitempty"`
}

// DiagramStyle defines style for diagrams
type DiagramStyle struct {
	Theme     string `json:"theme,omitempty"`      // modern, classic, minimal
	NodeStyle string `json:"node_style,omitempty"` // rounded, sharp, pill
}

// SocialPlatform represents social media platforms
type SocialPlatform string

const (
	SocialPlatformInstagram SocialPlatform = "instagram"
	SocialPlatformFacebook  SocialPlatform = "facebook"
	SocialPlatformLinkedIn  SocialPlatform = "linkedin"
	SocialPlatformTwitter   SocialPlatform = "twitter"
	SocialPlatformYouTube   SocialPlatform = "youtube"
)

// SocialContentType represents types of social content
type SocialContentType string

const (
	SocialContentPost      SocialContentType = "post"
	SocialContentStory     SocialContentType = "story"
	SocialContentReelCover SocialContentType = "reel_cover"
	SocialContentCarousel  SocialContentType = "carousel"
)

// SocialRequest represents a social media image request
type SocialRequest struct {
	Platform    SocialPlatform    `json:"platform"`
	ContentType SocialContentType `json:"content_type"`
	Prompt      string            `json:"prompt"`
	Content     *SocialContent    `json:"content,omitempty"`
	Style       string            `json:"style,omitempty"`
	BrandKitID  string            `json:"brand_kit_id,omitempty"`
}

// SocialContent defines text content for social media
type SocialContent struct {
	Headline string   `json:"headline,omitempty"`
	Body     string   `json:"body,omitempty"`
	Hashtags []string `json:"hashtags,omitempty"`
}

// CarouselRequest represents a carousel image set request
type CarouselRequest struct {
	Platform SocialPlatform  `json:"platform"`
	Slides   []CarouselSlide `json:"slides"`
	Style    CarouselStyle   `json:"style"`
}

// CarouselSlide represents one slide in a carousel
type CarouselSlide struct {
	Type     string `json:"type"` // cover, content, cta
	Number   int    `json:"number,omitempty"`
	Title    string `json:"title,omitempty"`
	Headline string `json:"headline,omitempty"`
	Body     string `json:"body,omitempty"`
	Button   string `json:"button,omitempty"`
}

// CarouselStyle defines style for carousel
type CarouselStyle struct {
	Consistency    string `json:"consistency,omitempty"`     // high, medium, low
	TransitionHint bool   `json:"transition_hint,omitempty"` // include swipe hint
}

// MockupType represents types of mockups
type MockupType string

const (
	MockupTypeDevice      MockupType = "device"
	MockupTypePrint       MockupType = "print"
	MockupTypePackaging   MockupType = "packaging"
	MockupTypeApparel     MockupType = "apparel"
	MockupTypeStationery  MockupType = "stationery"
	MockupTypeEnvironment MockupType = "environment"
)

// MockupRequest represents a mockup generation request
type MockupRequest struct {
	Type         MockupType  `json:"type"`
	Device       string      `json:"device,omitempty"` // e.g., iphone_15_pro
	ScreenImage  string      `json:"screen_image_url,omitempty"`
	ScreenPrompt string      `json:"screen_prompt,omitempty"` // generate screen content
	Scene        MockupScene `json:"scene"`
	Variations   int         `json:"variations,omitempty"`
}

// MockupScene defines the scene for mockup
type MockupScene struct {
	Angle      string `json:"angle,omitempty"`      // front, three_quarter, side, top
	Background string `json:"background,omitempty"` // solid, gradient, office, lifestyle
	Lighting   string `json:"lighting,omitempty"`   // studio, natural, dramatic
	Shadow     string `json:"shadow,omitempty"`     // soft, hard, none
}

// BackgroundType represents types of backgrounds
type BackgroundType string

const (
	BackgroundTypeGradient  BackgroundType = "gradient"
	BackgroundTypeAbstract  BackgroundType = "abstract"
	BackgroundTypeGeometric BackgroundType = "geometric"
	BackgroundTypeNature    BackgroundType = "nature"
	BackgroundTypeTexture   BackgroundType = "texture"
	BackgroundTypeBokeh     BackgroundType = "bokeh"
	BackgroundTypeTech      BackgroundType = "tech"
)

// BackgroundRequest represents a background image request
type BackgroundRequest struct {
	Type       BackgroundType `json:"type"`
	Mood       string         `json:"mood,omitempty"` // professional, calm, energetic
	Colors     []string       `json:"colors,omitempty"`
	Intensity  string         `json:"intensity,omitempty"`   // subtle, medium, bold
	FocalPoint string         `json:"focal_point,omitempty"` // center, top, bottom
	Preset     SizePreset     `json:"preset,omitempty"`
	Seamless   bool           `json:"seamless,omitempty"` // tileable
}

// IllustrationRequest represents an illustration request
type IllustrationRequest struct {
	Prompt           string      `json:"prompt"`
	Style            ImageStyle  `json:"style"`
	SceneElements    []string    `json:"scene_elements,omitempty"`
	Mood             string      `json:"mood,omitempty"`
	ColorPalette     string      `json:"color_palette,omitempty"` // brand, warm, cool, etc.
	AspectRatio      AspectRatio `json:"aspect_ratio,omitempty"`
	IncludeDiversity bool        `json:"include_diversity,omitempty"`
}

// GenerateImageRequest is a unified request for simple image generation
type GenerateImageRequest struct {
	Prompt      string       `json:"prompt"`
	Type        ImageType    `json:"type,omitempty"`
	Style       ImageStyle   `json:"style,omitempty"`
	Quality     ImageQuality `json:"quality,omitempty"`
	AspectRatio AspectRatio  `json:"aspect_ratio,omitempty"`
	Preset      SizePreset   `json:"preset,omitempty"`
	Count       int          `json:"count,omitempty"`
	BrandKitID  string       `json:"brand_kit_id,omitempty"`
}
