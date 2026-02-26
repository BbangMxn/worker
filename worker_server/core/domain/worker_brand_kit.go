package domain

import (
	"time"
)

// BrandKit represents a brand identity configuration for consistent image generation
type BrandKit struct {
	ID         string          `json:"id"`
	UserID     string          `json:"user_id"`
	Name       string          `json:"name"`
	IsDefault  bool            `json:"is_default"`
	Colors     BrandColors     `json:"colors"`
	Fonts      BrandFonts      `json:"fonts"`
	Logo       BrandLogo       `json:"logo"`
	StyleGuide BrandStyleGuide `json:"style_guide"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// BrandColors defines the color palette
type BrandColors struct {
	Primary   string   `json:"primary"`             // Main brand color
	Secondary string   `json:"secondary,omitempty"` // Secondary color
	Accent    string   `json:"accent,omitempty"`    // Accent/highlight color
	Neutral   []string `json:"neutral,omitempty"`   // Neutral colors (white, gray, black)
	Gradient  []string `json:"gradient,omitempty"`  // Gradient colors
}

// BrandFonts defines typography preferences
type BrandFonts struct {
	Heading string `json:"heading,omitempty"` // Font for headings
	Body    string `json:"body,omitempty"`    // Font for body text
	Accent  string `json:"accent,omitempty"`  // Font for accents/CTAs
}

// BrandLogo defines logo assets
type BrandLogo struct {
	PrimaryURL string `json:"primary_url,omitempty"` // Full color logo
	IconURL    string `json:"icon_url,omitempty"`    // Icon/mark only
	WhiteURL   string `json:"white_url,omitempty"`   // White/mono version
	DarkURL    string `json:"dark_url,omitempty"`    // Dark version for light backgrounds
}

// BrandStyleGuide defines style preferences
type BrandStyleGuide struct {
	Keywords   []string `json:"keywords,omitempty"`    // e.g., ["professional", "modern", "trustworthy"]
	Industry   string   `json:"industry,omitempty"`    // e.g., "technology", "finance", "healthcare"
	Tone       string   `json:"tone,omitempty"`        // e.g., "confident_friendly", "formal", "playful"
	AvoidWords []string `json:"avoid_words,omitempty"` // Words to avoid in generated content
}

// CreateBrandKitRequest represents a request to create a brand kit
type CreateBrandKitRequest struct {
	Name       string           `json:"name"`
	Colors     BrandColors      `json:"colors"`
	Fonts      *BrandFonts      `json:"fonts,omitempty"`
	Logo       *BrandLogo       `json:"logo,omitempty"`
	StyleGuide *BrandStyleGuide `json:"style_guide,omitempty"`
	IsDefault  bool             `json:"is_default,omitempty"`
}

// UpdateBrandKitRequest represents a request to update a brand kit
type UpdateBrandKitRequest struct {
	Name       *string          `json:"name,omitempty"`
	Colors     *BrandColors     `json:"colors,omitempty"`
	Fonts      *BrandFonts      `json:"fonts,omitempty"`
	Logo       *BrandLogo       `json:"logo,omitempty"`
	StyleGuide *BrandStyleGuide `json:"style_guide,omitempty"`
	IsDefault  *bool            `json:"is_default,omitempty"`
}

// ToBrandPromptContext converts brand kit to prompt context for image generation
func (b *BrandKit) ToBrandPromptContext() string {
	context := ""

	// Colors
	if b.Colors.Primary != "" {
		context += "Brand colors: primary " + b.Colors.Primary
		if b.Colors.Secondary != "" {
			context += ", secondary " + b.Colors.Secondary
		}
		if b.Colors.Accent != "" {
			context += ", accent " + b.Colors.Accent
		}
		context += ". "
	}

	// Style keywords
	if len(b.StyleGuide.Keywords) > 0 {
		context += "Style: "
		for i, kw := range b.StyleGuide.Keywords {
			if i > 0 {
				context += ", "
			}
			context += kw
		}
		context += ". "
	}

	// Industry
	if b.StyleGuide.Industry != "" {
		context += "Industry: " + b.StyleGuide.Industry + ". "
	}

	// Tone
	if b.StyleGuide.Tone != "" {
		context += "Tone: " + b.StyleGuide.Tone + ". "
	}

	return context
}

// ValidateColors validates that required colors are provided
func (c *BrandColors) ValidateColors() bool {
	return c.Primary != ""
}

// GetColorPalette returns all colors as a slice
func (c *BrandColors) GetColorPalette() []string {
	colors := []string{c.Primary}
	if c.Secondary != "" {
		colors = append(colors, c.Secondary)
	}
	if c.Accent != "" {
		colors = append(colors, c.Accent)
	}
	colors = append(colors, c.Neutral...)
	return colors
}
