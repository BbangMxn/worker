package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"

	"worker_server/pkg/logger"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// TokenBlacklist manages revoked tokens
type TokenBlacklist struct {
	redis  *redis.Client
	prefix string
}

var tokenBlacklist *TokenBlacklist

// InitTokenBlacklist initializes the token blacklist with Redis
func InitTokenBlacklist(redisClient *redis.Client) {
	if redisClient == nil {
		logger.Warn("Redis client not provided, token blacklist disabled")
		return
	}
	tokenBlacklist = &TokenBlacklist{
		redis:  redisClient,
		prefix: "token:blacklist:",
	}
	logger.Info("Token blacklist initialized")
}

// RevokeToken adds a token to the blacklist
func RevokeToken(ctx context.Context, tokenID string, expiry time.Duration) error {
	if tokenBlacklist == nil || tokenBlacklist.redis == nil {
		return nil
	}
	return tokenBlacklist.redis.Set(ctx, tokenBlacklist.prefix+tokenID, "1", expiry).Err()
}

// IsTokenRevoked checks if a token is blacklisted
func IsTokenRevoked(ctx context.Context, tokenID string) bool {
	if tokenBlacklist == nil || tokenBlacklist.redis == nil {
		return false
	}
	exists, _ := tokenBlacklist.redis.Exists(ctx, tokenBlacklist.prefix+tokenID).Result()
	return exists > 0
}

// JWKS represents a JSON Web Key Set
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a JSON Web Key
type JWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n,omitempty"`   // RSA modulus
	E   string `json:"e,omitempty"`   // RSA exponent
	Crv string `json:"crv,omitempty"` // EC curve
	X   string `json:"x,omitempty"`   // EC x coordinate
	Y   string `json:"y,omitempty"`   // EC y coordinate
}

// JWKSCache caches JWKS with TTL
type JWKSCache struct {
	mu        sync.RWMutex
	jwks      *JWKS
	fetchedAt time.Time
	ttl       time.Duration
	url       string
}

var jwksCache = &JWKSCache{
	ttl: 10 * time.Minute,
}

// SetURL sets the JWKS URL
func (c *JWKSCache) SetURL(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.url = url
}

// GetKey retrieves a key by kid from JWKS
func (c *JWKSCache) GetKey(kid string) (*JWK, error) {
	c.mu.RLock()
	if c.jwks != nil && time.Since(c.fetchedAt) < c.ttl {
		for _, key := range c.jwks.Keys {
			if key.Kid == kid {
				c.mu.RUnlock()
				return &key, nil
			}
		}
		c.mu.RUnlock()
		return nil, fmt.Errorf("key not found: %s", kid)
	}
	c.mu.RUnlock()

	// Fetch new JWKS
	if err := c.refresh(); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, key := range c.jwks.Keys {
		if key.Kid == kid {
			return &key, nil
		}
	}
	return nil, fmt.Errorf("key not found: %s", kid)
}

func (c *JWKSCache) refresh() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.url == "" {
		return fmt.Errorf("JWKS URL not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS fetch failed with status: %d", resp.StatusCode)
	}

	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("failed to decode JWKS: %w", err)
	}

	c.jwks = &jwks
	c.fetchedAt = time.Now()
	logger.Info("JWKS refreshed successfully, %d keys loaded", len(jwks.Keys))
	return nil
}

// parseECPublicKey parses EC public key from JWK
func parseECPublicKey(jwk *JWK) (*ecdsa.PublicKey, error) {
	xBytes, err := base64.RawURLEncoding.DecodeString(jwk.X)
	if err != nil {
		return nil, fmt.Errorf("failed to decode x: %w", err)
	}

	yBytes, err := base64.RawURLEncoding.DecodeString(jwk.Y)
	if err != nil {
		return nil, fmt.Errorf("failed to decode y: %w", err)
	}

	var curve elliptic.Curve
	switch jwk.Crv {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("unsupported curve: %s", jwk.Crv)
	}

	return &ecdsa.PublicKey{
		Curve: curve,
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}, nil
}

// parseRSAPublicKey parses RSA public key from JWK
func parseRSAPublicKey(jwk *JWK) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode n: %w", err)
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode e: %w", err)
	}

	var e int
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: e,
	}, nil
}

// InitJWKS initializes the JWKS cache with Supabase URL
func InitJWKS(supabaseURL string) {
	if supabaseURL == "" {
		logger.Warn("SUPABASE_URL not configured, JWKS verification disabled")
		return
	}

	jwksURL := strings.TrimSuffix(supabaseURL, "/") + "/auth/v1/.well-known/jwks.json"
	jwksCache.SetURL(jwksURL)
	logger.Info("JWKS URL configured: %s", jwksURL)

	// Pre-fetch JWKS in background
	go func() {
		if err := jwksCache.refresh(); err != nil {
			logger.WithError(err).Warn("Failed to pre-fetch JWKS, will retry on first request")
		}
	}()
}

// JWTAuth validates Supabase JWT tokens
func JWTAuth(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip auth for CORS preflight requests
		if c.Method() == "OPTIONS" {
			return c.Next()
		}

		// Skip auth for webhook endpoints (called by Google/Microsoft without auth)
		path := c.Path()
		if strings.Contains(path, "/webhook/") || strings.Contains(path, "/webhooks/") {
			return c.Next()
		}

		var tokenString string

		// First, try Authorization header
		authHeader := c.Get("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenString = parts[1]
			}
		}

		// If no header token, check query param (for SSE/EventSource which can't set headers)
		if tokenString == "" {
			tokenString = c.Query("token")
		}

		if tokenString == "" {
			return c.Status(401).JSON(fiber.Map{"error": "missing authorization"})
		}

		// Parse and validate token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			switch token.Method.(type) {
			case *jwt.SigningMethodHMAC:
				// HS256 legacy - use JWT secret
				if secret == "" {
					return nil, fmt.Errorf("JWT secret not configured")
				}
				return []byte(secret), nil

			case *jwt.SigningMethodECDSA:
				// ES256 - use JWKS public key
				kid, ok := token.Header["kid"].(string)
				if !ok || kid == "" {
					return nil, fmt.Errorf("missing kid in token header")
				}

				jwk, err := jwksCache.GetKey(kid)
				if err != nil {
					return nil, fmt.Errorf("failed to get public key: %w", err)
				}

				return parseECPublicKey(jwk)

			case *jwt.SigningMethodRSA:
				// RS256 - use JWKS public key
				kid, ok := token.Header["kid"].(string)
				if !ok || kid == "" {
					return nil, fmt.Errorf("missing kid in token header")
				}

				jwk, err := jwksCache.GetKey(kid)
				if err != nil {
					return nil, fmt.Errorf("failed to get public key: %w", err)
				}

				return parseRSAPublicKey(jwk)

			default:
				return nil, fmt.Errorf("unsupported signing method: %v", token.Header["alg"])
			}
		})

		if err != nil {
			logger.WithError(err).Warn("JWT validation failed")
			return c.Status(401).JSON(fiber.Map{
				"error":  "invalid token",
				"detail": err.Error(),
			})
		}

		if !token.Valid {
			return c.Status(401).JSON(fiber.Map{"error": "invalid token"})
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return c.Status(401).JSON(fiber.Map{"error": "invalid claims"})
		}

		// Validate token expiration (exp claim)
		if exp, ok := claims["exp"].(float64); ok {
			if time.Now().Unix() > int64(exp) {
				return c.Status(401).JSON(fiber.Map{
					"error": "token expired",
					"code":  "TOKEN_EXPIRED",
				})
			}
		}

		// Validate token issued at (iat claim) - reject tokens issued too far in the future
		if iat, ok := claims["iat"].(float64); ok {
			issuedAt := time.Unix(int64(iat), 0)
			// Allow 1 minute clock skew
			if issuedAt.After(time.Now().Add(time.Minute)) {
				return c.Status(401).JSON(fiber.Map{
					"error": "token issued in the future",
					"code":  "INVALID_TOKEN_TIME",
				})
			}
		}

		// Check token blacklist (for logout/revocation)
		if jti, ok := claims["jti"].(string); ok && jti != "" {
			if IsTokenRevoked(c.Context(), jti) {
				return c.Status(401).JSON(fiber.Map{
					"error": "token has been revoked",
					"code":  "TOKEN_REVOKED",
				})
			}
		}

		// Extract user ID from "sub" claim
		userIDStr, ok := claims["sub"].(string)
		if !ok {
			return c.Status(401).JSON(fiber.Map{"error": "missing user id in token"})
		}

		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "invalid user id format"})
		}

		// Extract email if available
		email := ""
		if emailClaim, ok := claims["email"].(string); ok {
			email = emailClaim
		}

		// Store session info for audit logging
		sessionID := ""
		if sid, ok := claims["session_id"].(string); ok {
			sessionID = sid
		}

		c.Locals("user_id", userID)
		c.Locals("user_email", email)
		c.Locals("session_id", sessionID)
		c.Locals("claims", claims)

		return c.Next()
	}
}

func OptionalJWTAuth(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Next()
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			return c.Next()
		}

		tokenString := parts[1]

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			switch token.Method.(type) {
			case *jwt.SigningMethodHMAC:
				return []byte(secret), nil
			case *jwt.SigningMethodECDSA:
				kid, _ := token.Header["kid"].(string)
				jwk, err := jwksCache.GetKey(kid)
				if err != nil {
					return nil, err
				}
				return parseECPublicKey(jwk)
			case *jwt.SigningMethodRSA:
				kid, _ := token.Header["kid"].(string)
				jwk, err := jwksCache.GetKey(kid)
				if err != nil {
					return nil, err
				}
				return parseRSAPublicKey(jwk)
			default:
				return nil, fmt.Errorf("unsupported signing method")
			}
		})

		if err != nil || !token.Valid {
			return c.Next()
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return c.Next()
		}

		if userIDStr, ok := claims["sub"].(string); ok {
			if userID, err := uuid.Parse(userIDStr); err == nil {
				c.Locals("user_id", userID)
				c.Locals("claims", claims)

				if email, ok := claims["email"].(string); ok {
					c.Locals("user_email", email)
				}
			}
		}

		return c.Next()
	}
}
