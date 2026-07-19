// Package middleware provides HTTP middleware for authentication and
// authorization using JWT bearer tokens.
package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/golang-jwt/jwt/v5"
)

// roleAdmin is the role name granted administrative access.
const roleAdmin = "admin"

// jwtIssuer is the iss claim set on every session token.
const jwtIssuer = "user-service"

// jwtAudience is the aud claim set on every session token and required on
// parse.
const jwtAudience = "pupitre-api"

// Claims are the JWT claims embedded in issued access tokens.
type Claims struct {
	jwt.RegisteredClaims

	Email             string `json:"email"`
	Role              string `json:"role"`
	PreferredUsername string `json:"preferred_username,omitempty"` //nolint:tagliatelle // preferred_username is the standard OIDC claim name
}

// contextKey is the type used for context values set by this package.
type contextKey string

// ClaimsKey is the context key under which verified Claims are stored.
const ClaimsKey contextKey = "claims"

// CreateToken issues a signed JWT for the given user identity.
func CreateToken(userID, email, role, username, secret string, expiryHours int) (string, error) {
	claims := Claims{
		Email:             email,
		Role:              role,
		PreferredUsername: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expiryHours) * time.Hour)),
			Issuer:    jwtIssuer,
			Audience:  jwt.ClaimStrings{jwtAudience},
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}

	return token, nil
}

// VerifyToken parses and validates a JWT, returning its Claims.
func VerifyToken(tokenStr, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}

		return []byte(secret), nil
	}, jwt.WithAudience(jwtAudience), jwt.WithIssuer(jwtIssuer))
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, jwt.ErrTokenInvalidClaims
}

// GetClaims retrieves the Claims stored in the request context by Auth.
func GetClaims(req *http.Request) *Claims {
	if c, ok := req.Context().Value(ClaimsKey).(*Claims); ok {
		return c
	}

	return nil
}

// httpErr writes a JSON error response with the given status and message.
func httpErr(writer http.ResponseWriter, status int, msg string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)

	err := json.NewEncoder(writer).Encode(map[string]string{"error": msg})
	if err != nil {
		return
	}
}

// Auth returns middleware that requires a valid bearer token.
func Auth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			auth := req.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				httpErr(resp, http.StatusUnauthorized, "Missing Authorization header")

				return
			}

			claims, err := VerifyToken(strings.TrimPrefix(auth, "Bearer "), secret)
			if err != nil {
				zap.L().Error("token verification failed", zap.Error(err))
				httpErr(resp, http.StatusUnauthorized, "Invalid token")

				return
			}

			if claims.Subject == "" {
				httpErr(resp, http.StatusUnauthorized, "Invalid token: missing user ID")

				return
			}

			ctx := context.WithValue(req.Context(), ClaimsKey, claims)
			next.ServeHTTP(resp, req.WithContext(ctx))
		})
	}
}

// Admin returns middleware that requires a valid bearer token whose
// claims carry the admin role.
func Admin(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			auth := req.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				httpErr(resp, http.StatusUnauthorized, "Missing Authorization header")

				return
			}

			claims, err := VerifyToken(strings.TrimPrefix(auth, "Bearer "), secret)
			if err != nil {
				zap.L().Error("token verification failed", zap.Error(err))
				httpErr(resp, http.StatusUnauthorized, "Invalid token")

				return
			}

			if claims.Subject == "" {
				httpErr(resp, http.StatusUnauthorized, "Invalid token: missing user ID")

				return
			}

			if claims.Role != roleAdmin {
				httpErr(resp, http.StatusForbidden, "Admin access required")

				return
			}

			ctx := context.WithValue(req.Context(), ClaimsKey, claims)
			next.ServeHTTP(resp, req.WithContext(ctx))
		})
	}
}
