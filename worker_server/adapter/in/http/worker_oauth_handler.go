package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"worker_server/core/domain"
	"worker_server/core/port/in"
	"worker_server/pkg/logger"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// OAuthStateStore OAuth state 저장/검증 인터페이스 (CSRF 보호)
type OAuthStateStore interface {
	// StoreState state를 저장하고 TTL 설정
	StoreState(ctx context.Context, state string, userID uuid.UUID, ttl time.Duration) error
	// ValidateState state 검증 및 userID 반환 (검증 후 삭제)
	ValidateState(ctx context.Context, state string) (uuid.UUID, error)
}

// OAuthStateKey Redis key prefix
const OAuthStateKey = "oauth:state:"

// OAuthStateTTL state 유효 시간 (10분)
const OAuthStateTTL = 10 * time.Minute

type OAuthHandler struct {
	oauthService in.OAuthService
	stateStore   OAuthStateStore
}

func NewOAuthHandler(oauthService in.OAuthService) *OAuthHandler {
	return &OAuthHandler{
		oauthService: oauthService,
		stateStore:   nil, // 기본값: 검증 비활성화 (하위 호환성)
	}
}

// NewOAuthHandlerWithStateStore CSRF 보호가 활성화된 OAuth 핸들러 생성
func NewOAuthHandlerWithStateStore(oauthService in.OAuthService, stateStore OAuthStateStore) *OAuthHandler {
	return &OAuthHandler{
		oauthService: oauthService,
		stateStore:   stateStore,
	}
}

// generateSecureState 암호학적으로 안전한 state 생성
func generateSecureState() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure state: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func (h *OAuthHandler) Register(app fiber.Router) {
	oauth := app.Group("/oauth")
	oauth.Get("/connect/:provider", h.Connect)
	oauth.Get("/callback/:provider", h.Callback)
	oauth.Get("/:provider/callback", h.Callback) // Support /oauth/google/callback format
	oauth.Get("/connections", h.ListConnections)
	oauth.Get("/connections/default", h.GetDefaultConnection)
	oauth.Post("/connections/:id/default", h.SetDefaultConnection)
	oauth.Delete("/connections/:id", h.Disconnect)
}

func (h *OAuthHandler) Connect(c *fiber.Ctx) error {
	provider := domain.OAuthProvider(c.Params("provider"))
	logger.Info("[OAuth Connect] Provider: %s", provider)

	userID, err := GetUserID(c)
	if err != nil {
		logger.WithError(err).Error("[OAuth Connect] GetUserID failed")
		return ErrorResponse(c, 401, "unauthorized")
	}
	logger.Info("[OAuth Connect] UserID: %s", userID)

	// 암호학적으로 안전한 state 생성
	secureRandom, err := generateSecureState()
	if err != nil {
		logger.WithError(err).Error("[OAuth Connect] Failed to generate secure state")
		return ErrorResponse(c, 500, "failed to generate state")
	}

	// state 형식: "userID:secureRandomString"
	state := userID.String() + ":" + secureRandom
	logger.Debug("[OAuth Connect] Generated state: %s", state[:50]+"...")

	// CSRF 보호: state를 Redis에 저장 (활성화된 경우)
	if h.stateStore != nil {
		if err := h.stateStore.StoreState(c.Context(), state, userID, OAuthStateTTL); err != nil {
			logger.WithError(err).Error("[OAuth Connect] Failed to store state")
			return ErrorResponse(c, 500, "failed to store state")
		}
		logger.Debug("[OAuth Connect] State stored in Redis with TTL: %v", OAuthStateTTL)
	}

	authURL, err := h.oauthService.GetAuthURL(c.Context(), provider, state)
	if err != nil {
		logger.WithError(err).Error("[OAuth Connect] GetAuthURL failed")
		return InternalErrorResponse(c, err, "operation")
	}

	logger.Info("[OAuth Connect] Redirecting to: %s", authURL)
	return c.JSON(fiber.Map{
		"auth_url": authURL,
		"state":    state,
	})
}

func (h *OAuthHandler) Callback(c *fiber.Ctx) error {
	provider := domain.OAuthProvider(c.Params("provider"))
	code := c.Query("code")
	state := c.Query("state")
	errorParam := c.Query("error")

	logger.Info("[OAuth Callback] Provider: %s, State length: %d, Code length: %d", provider, len(state), len(code))

	// Frontend URL for redirects
	frontendURL := "http://localhost:3000"
	if origin := c.Get("Origin"); origin != "" {
		frontendURL = origin
	}
	// Also check Referer header for production
	if referer := c.Get("Referer"); referer != "" && strings.Contains(referer, "worker") {
		if strings.Contains(referer, "https://") {
			parts := strings.Split(referer, "/")
			if len(parts) >= 3 {
				frontendURL = parts[0] + "//" + parts[2]
			}
		}
	}

	// Handle OAuth errors
	if errorParam != "" {
		errorDesc := c.Query("error_description")
		logger.Warn("[OAuth Callback] Error from provider: %s - %s", errorParam, errorDesc)
		return c.Redirect(frontendURL + "/settings?error=" + errorParam + "&error_description=" + errorDesc)
	}

	if code == "" {
		logger.Warn("[OAuth Callback] Missing code")
		return c.Redirect(frontendURL + "/settings?error=missing_code")
	}

	if state == "" {
		logger.Warn("[OAuth Callback] Missing state - potential CSRF attack")
		return c.Redirect(frontendURL + "/settings?error=missing_state")
	}

	var userID uuid.UUID

	// CSRF 보호: Redis에서 state 검증 (활성화된 경우)
	if h.stateStore != nil {
		validatedUserID, err := h.stateStore.ValidateState(c.Context(), state)
		if err != nil {
			logger.WithError(err).Warn("[OAuth Callback] State validation failed (CSRF protection)")
			return c.Redirect(frontendURL + "/settings?error=invalid_state&message=csrf_validation_failed")
		}
		userID = validatedUserID
		logger.Info("[OAuth Callback] State validated successfully for user: %s", userID)
	} else {
		// 하위 호환성: stateStore가 없으면 state에서 userID 파싱
		parts := strings.Split(state, ":")
		logger.Debug("[OAuth Callback] State parts count: %d", len(parts))
		if len(parts) >= 1 {
			if parsed, err := uuid.Parse(parts[0]); err == nil {
				userID = parsed
				logger.Debug("[OAuth Callback] Parsed userID from state: %s", userID)
			} else {
				logger.WithError(err).Warn("[OAuth Callback] Failed to parse userID from state")
			}
		}
	}

	// If no userID from state, try from locals (authenticated request)
	if userID == uuid.Nil {
		if uid, err := GetUserID(c); err == nil {
			userID = uid
			logger.Debug("[OAuth Callback] Got userID from auth: %s", userID)
		}
	}

	if userID == uuid.Nil {
		logger.Warn("[OAuth Callback] No valid userID found")
		return c.Redirect(frontendURL + "/settings?error=invalid_state")
	}

	logger.Info("[OAuth Callback] Processing callback for user: %s, provider: %s", userID, provider)

	conn, err := h.oauthService.HandleCallback(c.Context(), provider, code, userID)
	if err != nil {
		logger.WithError(err).Error("[OAuth Callback] HandleCallback error")
		return c.Redirect(frontendURL + "/settings?error=oauth_failed&message=" + err.Error())
	}

	logger.Info("[OAuth Callback] Success! Connection ID: %d, Email: %s", conn.ID, conn.Email)

	// Redirect to frontend settings page on success
	return c.Redirect(frontendURL + "/settings?oauth=success&provider=" + string(provider))
}

func (h *OAuthHandler) ListConnections(c *fiber.Ctx) error {
	logger.Debug("[OAuth ListConnections] Request received")

	userID, err := GetUserID(c)
	if err != nil {
		logger.WithError(err).Error("[OAuth ListConnections] GetUserID failed")
		return ErrorResponse(c, 401, "unauthorized")
	}
	logger.Debug("[OAuth ListConnections] UserID: %s", userID)

	connections, err := h.oauthService.GetConnectionsByUser(c.Context(), userID)
	if err != nil {
		logger.WithError(err).Error("[OAuth ListConnections] GetConnectionsByUser failed")
		return InternalErrorResponse(c, err, "operation")
	}

	logger.Info("[OAuth ListConnections] Found %d connections for user %s", len(connections), userID)
	return c.JSON(fiber.Map{
		"connections": connections,
	})
}

func (h *OAuthHandler) Disconnect(c *fiber.Ctx) error {
	// Verify user is authenticated
	_, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	connID, err := c.ParamsInt("id")
	if err != nil {
		return ErrorResponse(c, 400, "invalid connection id")
	}

	if err := h.oauthService.Disconnect(c.Context(), int64(connID)); err != nil {
		return InternalErrorResponse(c, err, "operation")
	}

	return c.JSON(fiber.Map{"status": "disconnected"})
}

// GetDefaultConnection returns the default connection for sending emails.
func (h *OAuthHandler) GetDefaultConnection(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	conn, err := h.oauthService.GetDefaultConnection(c.Context(), userID)
	if err != nil {
		return ErrorResponse(c, 404, "no default connection found")
	}

	return c.JSON(fiber.Map{
		"connection": conn,
	})
}

// SetDefaultConnection sets a connection as the default for sending emails.
func (h *OAuthHandler) SetDefaultConnection(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return ErrorResponse(c, 401, "unauthorized")
	}

	connID, err := c.ParamsInt("id")
	if err != nil {
		return ErrorResponse(c, 400, "invalid connection id")
	}

	if err := h.oauthService.SetDefaultConnection(c.Context(), userID, int64(connID)); err != nil {
		return ErrorResponse(c, 400, err.Error())
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "default connection updated",
	})
}
