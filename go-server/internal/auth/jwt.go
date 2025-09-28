package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	secretKey     []byte
	tokenDuration time.Duration
}

func NewJWTManager(secretKey string, tokenDuration time.Duration) *JWTManager {
	return &JWTManager{
		secretKey:     []byte(secretKey),
		tokenDuration: tokenDuration,
	}
}

// Generate creates a new JWT token
func (manager *JWTManager) Generate(userID, username, role string) (string, error) {
	claims := &Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(manager.tokenDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "odin-websocket-server",
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(manager.secretKey)
}

// Verify validates the JWT token and returns the claims
func (manager *JWTManager) Verify(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&Claims{},
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return manager.secretKey, nil
		},
	)

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	return claims, nil
}

// ExtractTokenFromHeader extracts JWT token from Authorization header
func ExtractTokenFromHeader(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", errors.New("authorization header missing")
	}

	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return "", errors.New("invalid authorization header format")
	}

	return strings.TrimPrefix(authHeader, bearerPrefix), nil
}

// ExtractTokenFromQuery extracts JWT token from query parameter
func ExtractTokenFromQuery(r *http.Request) (string, error) {
	token := r.URL.Query().Get("token")
	if token == "" {
		return "", errors.New("token query parameter missing")
	}
	return token, nil
}

// AuthMiddleware creates HTTP middleware for JWT authentication
func (manager *JWTManager) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Try to extract token from header first, then query param
		token, err := ExtractTokenFromHeader(r)
		if err != nil {
			token, err = ExtractTokenFromQuery(r)
			if err != nil {
				http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
				return
			}
		}

		claims, err := manager.Verify(token)
		if err != nil {
			http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
			return
		}

		// Add claims to request context
		ctx := r.Context()
		ctx = SetUserContext(ctx, claims)
		r = r.WithContext(ctx)

		next(w, r)
	}
}

// WebSocketAuth validates JWT token for WebSocket connections
func (manager *JWTManager) WebSocketAuth(r *http.Request) (*Claims, error) {
	// Try to extract token from query parameter (common for WebSocket)
	token, err := ExtractTokenFromQuery(r)
	if err != nil {
		// Fallback to header
		token, err = ExtractTokenFromHeader(r)
		if err != nil {
			return nil, fmt.Errorf("no valid token found: %w", err)
		}
	}

	return manager.Verify(token)
}

// Generate a simple token for testing (without user validation)
func (manager *JWTManager) GenerateTestToken() (string, error) {
	return manager.Generate(
		"test-user-123",
		"testuser",
		"user",
	)
}