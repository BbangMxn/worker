package http

import (
	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/pkg/logger"

	"github.com/gofiber/fiber/v2"
)

// CategoryHandler handles category-related API endpoints.
type CategoryHandler struct {
	emailRepo out.EmailRepository
}

// NewCategoryHandler creates a new CategoryHandler.
func NewCategoryHandler(emailRepo out.EmailRepository) *CategoryHandler {
	return &CategoryHandler{emailRepo: emailRepo}
}

// Register registers category routes.
func (h *CategoryHandler) Register(app fiber.Router) {
	cat := app.Group("/categories")
	cat.Get("/", h.ListCategories)                 // 전체 카테고리 목록
	cat.Get("/stats", h.GetCategoryStats)          // 카테고리별 통계
	cat.Get("/priorities", h.ListPriorities)       // 우선순위 레벨 정보
	cat.Get("/subcategories", h.ListSubCategories) // 서브카테고리 목록
}

// =============================================================================
// Category Metadata
// =============================================================================

// CategoryMeta contains metadata for a category.
type CategoryMeta struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	NameKo      string `json:"name_ko"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
	SortOrder   int    `json:"sort_order"`
	IsInbox     bool   `json:"is_inbox"` // Inbox 뷰에 표시되는 카테고리
}

// SubCategoryMeta contains metadata for a sub-category.
type SubCategoryMeta struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	NameKo      string `json:"name_ko"`
	ParentKey   string `json:"parent_key"` // 상위 카테고리
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

// PriorityLevelMeta contains metadata for a priority level.
type PriorityLevelMeta struct {
	Level       string  `json:"level"`
	Name        string  `json:"name"`
	NameKo      string  `json:"name_ko"`
	MinScore    float64 `json:"min_score"`
	MaxScore    float64 `json:"max_score"`
	Color       string  `json:"color"`
	Description string  `json:"description"`
}

// categoryMetadata defines all category metadata.
var categoryMetadata = []CategoryMeta{
	// ===========================================
	// Inbox categories (사람이 보낸 중요 메일)
	// ===========================================
	{Key: "primary", Name: "Primary", NameKo: "중요", Description: "Important emails requiring attention", Icon: "inbox", Color: "#4285F4", SortOrder: 1, IsInbox: true},
	{Key: "work", Name: "Work", NameKo: "업무", Description: "Work emails including dev (GitHub, GitLab, Jira)", Icon: "briefcase", Color: "#34A853", SortOrder: 2, IsInbox: true},
	{Key: "personal", Name: "Personal", NameKo: "개인", Description: "Personal emails from contacts", Icon: "user", Color: "#9C27B0", SortOrder: 3, IsInbox: true},

	// ===========================================
	// Feed categories (자동 발송 메일)
	// ===========================================
	{Key: "notification", Name: "Notification", NameKo: "알림", Description: "Automated notifications (subscribed, CI/CD)", Icon: "bell", Color: "#607D8B", SortOrder: 10, IsInbox: false},
	{Key: "newsletter", Name: "Newsletter", NameKo: "뉴스레터", Description: "Newsletters and subscriptions", Icon: "newspaper", Color: "#FF9800", SortOrder: 11, IsInbox: false},
	{Key: "marketing", Name: "Marketing", NameKo: "마케팅", Description: "Marketing and promotional emails", Icon: "megaphone", Color: "#E91E63", SortOrder: 12, IsInbox: false},
	{Key: "social", Name: "Social", NameKo: "소셜", Description: "Social network notifications", Icon: "users", Color: "#03A9F4", SortOrder: 13, IsInbox: false},
	{Key: "finance", Name: "Finance", NameKo: "금융", Description: "Receipts, invoices, payment notifications", Icon: "dollar-sign", Color: "#4CAF50", SortOrder: 20, IsInbox: false},
	{Key: "shopping", Name: "Shopping", NameKo: "쇼핑", Description: "Order confirmations and shipping", Icon: "shopping-cart", Color: "#FF5722", SortOrder: 21, IsInbox: false},
	{Key: "travel", Name: "Travel", NameKo: "여행", Description: "Travel bookings and itineraries", Icon: "plane", Color: "#00BCD4", SortOrder: 22, IsInbox: false},

	// ===========================================
	// Other
	// ===========================================
	{Key: "spam", Name: "Spam", NameKo: "스팸", Description: "Spam and unwanted emails", Icon: "shield-off", Color: "#F44336", SortOrder: 98, IsInbox: false},
	{Key: "other", Name: "Other", NameKo: "기타", Description: "Uncategorized emails", Icon: "folder", Color: "#9E9E9E", SortOrder: 99, IsInbox: false},
}

// subCategoryMetadata defines all sub-category metadata.
var subCategoryMetadata = []SubCategoryMeta{
	// Finance sub-categories
	{Key: "receipt", Name: "Receipt", NameKo: "영수증", ParentKey: "finance", Description: "Payment receipts", Icon: "receipt"},
	{Key: "invoice", Name: "Invoice", NameKo: "청구서", ParentKey: "finance", Description: "Bills and invoices", Icon: "file-text"},

	// Shopping sub-categories
	{Key: "shipping", Name: "Shipping", NameKo: "배송", ParentKey: "shopping", Description: "Shipping notifications", Icon: "truck"},
	{Key: "order", Name: "Order", NameKo: "주문", ParentKey: "shopping", Description: "Order confirmations", Icon: "package"},

	// Travel sub-categories
	{Key: "travel", Name: "Travel", NameKo: "여행", ParentKey: "travel", Description: "Travel bookings", Icon: "map"},

	// Notification sub-categories
	{Key: "calendar", Name: "Calendar", NameKo: "일정", ParentKey: "notification", Description: "Calendar invites and reminders", Icon: "calendar"},
	{Key: "account", Name: "Account", NameKo: "계정", ParentKey: "notification", Description: "Account notifications", Icon: "user-check"},
	{Key: "security", Name: "Security", NameKo: "보안", ParentKey: "notification", Description: "Security alerts", Icon: "shield"},
	{Key: "notification", Name: "System", NameKo: "시스템", ParentKey: "notification", Description: "System notifications", Icon: "bell"},
	{Key: "alert", Name: "Alert", NameKo: "알림", ParentKey: "notification", Description: "Important alerts", Icon: "alert-triangle"},

	// Social sub-categories
	{Key: "sns", Name: "SNS", NameKo: "SNS", ParentKey: "social", Description: "Social network notifications", Icon: "share-2"},
	{Key: "comment", Name: "Comment", NameKo: "댓글", ParentKey: "social", Description: "Comment notifications", Icon: "message-circle"},

	// Work sub-categories
	{Key: "developer", Name: "Developer", NameKo: "개발자", ParentKey: "work", Description: "GitHub, GitLab, CI/CD", Icon: "code"},

	// Marketing sub-categories
	{Key: "newsletter", Name: "Newsletter", NameKo: "뉴스레터", ParentKey: "newsletter", Description: "Email newsletters", Icon: "mail"},
	{Key: "marketing", Name: "Promotion", NameKo: "프로모션", ParentKey: "marketing", Description: "Promotional emails", Icon: "tag"},
	{Key: "deal", Name: "Deal", NameKo: "할인", ParentKey: "marketing", Description: "Deals and offers", Icon: "percent"},
}

// priorityLevelMetadata defines priority level metadata.
var priorityLevelMetadata = []PriorityLevelMeta{
	{Level: "urgent", Name: "Urgent", NameKo: "긴급", MinScore: 0.80, MaxScore: 1.00, Color: "#F44336", Description: "Requires immediate action"},
	{Level: "high", Name: "High", NameKo: "높음", MinScore: 0.60, MaxScore: 0.79, Color: "#FF9800", Description: "Important, should address soon"},
	{Level: "normal", Name: "Normal", NameKo: "보통", MinScore: 0.40, MaxScore: 0.59, Color: "#4CAF50", Description: "Relevant, worth reading"},
	{Level: "low", Name: "Low", NameKo: "낮음", MinScore: 0.20, MaxScore: 0.39, Color: "#2196F3", Description: "Can be deferred"},
	{Level: "lowest", Name: "Lowest", NameKo: "최저", MinScore: 0.00, MaxScore: 0.19, Color: "#9E9E9E", Description: "Background noise"},
}

// =============================================================================
// Handlers
// =============================================================================

// ListCategories returns all category metadata.
// GET /categories
func (h *CategoryHandler) ListCategories(c *fiber.Ctx) error {
	// Optional: filter by inbox
	inboxOnly := c.QueryBool("inbox_only", false)

	var categories []CategoryMeta
	if inboxOnly {
		for _, cat := range categoryMetadata {
			if cat.IsInbox {
				categories = append(categories, cat)
			}
		}
	} else {
		categories = categoryMetadata
	}

	return c.JSON(fiber.Map{
		"categories": categories,
		"total":      len(categories),
	})
}

// ListSubCategories returns all sub-category metadata.
// GET /categories/subcategories?parent=finance
func (h *CategoryHandler) ListSubCategories(c *fiber.Ctx) error {
	parent := c.Query("parent")

	var subCategories []SubCategoryMeta
	if parent != "" {
		for _, sub := range subCategoryMetadata {
			if sub.ParentKey == parent {
				subCategories = append(subCategories, sub)
			}
		}
	} else {
		subCategories = subCategoryMetadata
	}

	return c.JSON(fiber.Map{
		"sub_categories": subCategories,
		"total":          len(subCategories),
	})
}

// ListPriorities returns priority level metadata.
// GET /categories/priorities
func (h *CategoryHandler) ListPriorities(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"priorities": priorityLevelMetadata,
		"total":      len(priorityLevelMetadata),
	})
}

// CategoryStats represents category statistics.
type CategoryStats struct {
	Category string `json:"category"`
	Name     string `json:"name"`
	NameKo   string `json:"name_ko"`
	Total    int    `json:"total"`
	Unread   int    `json:"unread"`
	Color    string `json:"color"`
	Icon     string `json:"icon"`
}

// GetCategoryStats returns email counts per category.
// GET /categories/stats?connection_id=123
func (h *CategoryHandler) GetCategoryStats(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	connectionID := GetConnectionID(c)

	// Get stats from repository
	stats, err := h.emailRepo.GetCategoryStats(c.Context(), userID, connectionID)
	if err != nil {
		logger.WithError(err).Error("[CategoryHandler] Failed to get category stats")
		return InternalErrorResponse(c, err, "get category stats")
	}

	// Merge with metadata
	result := make([]CategoryStats, 0, len(categoryMetadata))
	for _, meta := range categoryMetadata {
		stat := CategoryStats{
			Category: meta.Key,
			Name:     meta.Name,
			NameKo:   meta.NameKo,
			Color:    meta.Color,
			Icon:     meta.Icon,
			Total:    0,
			Unread:   0,
		}

		// Find matching stats
		if s, ok := stats[meta.Key]; ok {
			stat.Total = s.Total
			stat.Unread = s.Unread
		}

		result = append(result, stat)
	}

	// Calculate inbox totals
	var inboxTotal, inboxUnread int
	for _, stat := range result {
		for _, meta := range categoryMetadata {
			if meta.Key == stat.Category && meta.IsInbox {
				inboxTotal += stat.Total
				inboxUnread += stat.Unread
				break
			}
		}
	}

	return c.JSON(fiber.Map{
		"categories": result,
		"inbox": fiber.Map{
			"total":  inboxTotal,
			"unread": inboxUnread,
		},
	})
}

// =============================================================================
// Helper Functions
// =============================================================================

// GetCategoryMeta returns metadata for a category.
func GetCategoryMeta(key string) *CategoryMeta {
	for _, meta := range categoryMetadata {
		if meta.Key == key {
			return &meta
		}
	}
	return nil
}

// GetSubCategoryMeta returns metadata for a sub-category.
func GetSubCategoryMeta(key string) *SubCategoryMeta {
	for _, meta := range subCategoryMetadata {
		if meta.Key == key {
			return &meta
		}
	}
	return nil
}

// GetPriorityLevel returns the priority level for a score.
func GetPriorityLevel(score float64) string {
	return domain.Priority(score).Level()
}

// GetPriorityLevelMeta returns metadata for a priority score.
func GetPriorityLevelMeta(score float64) *PriorityLevelMeta {
	level := GetPriorityLevel(score)
	for _, meta := range priorityLevelMetadata {
		if meta.Level == level {
			return &meta
		}
	}
	return nil
}
