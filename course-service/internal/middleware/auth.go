// Package middleware provides HTTP middleware for course-service, including
// JWT-based authentication and authorization.
package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// roleAdmin is the Role value required to access admin-only endpoints.
const roleAdmin = "admin"

// Claims holds the JWT claims used by course-service, combining the
// standard registered claims with the application-specific user fields.
type Claims struct {
	jwt.RegisteredClaims

	Email string `json:"email"`
	Role  string `json:"role"`

	PreferredUsername string `json:"preferred_username"` //nolint:tagliatelle // This is a default claim name from OIDC providers, so we cannot change it without breaking compatibility.
}

// contextKey is the type used for values stored in a request context by
// this package, to avoid collisions with keys from other packages.
type contextKey string

// ClaimsKey is the request context key under which Auth and Admin store the
// verified Claims.
const ClaimsKey contextKey = "claims"

// CreateToken generates a signed JWT for the given user.
func CreateToken(userID, email, role, secret string, expiryHours int) (string, error) {
	claims := Claims{
		Email: email,
		Role:  role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expiryHours) * time.Hour)),
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}

	return token, nil
}

// VerifyToken validates a JWT and returns its claims.
func VerifyToken(tokenStr, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}

		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse jwt: %w", err)
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, jwt.ErrTokenInvalidClaims
}

// GetClaims extracts Claims from the request context (set by Auth or Admin
// middleware).
func GetClaims(r *http.Request) *Claims {
	if c, ok := r.Context().Value(ClaimsKey).(*Claims); ok {
		return c
	}

	return nil
}

// httpErr writes a JSON error response with the given status and message.
func httpErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck,errchkjson // encoding a static map[string]string cannot fail
}

// Auth middleware: extracts and validates JWT, adds Claims to context.
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
				slog.Error("token verification failed", "err", err)
				httpErr(resp, http.StatusUnauthorized, "Invalid token")

				return
			}

			ctx := context.WithValue(req.Context(), ClaimsKey, claims)
			next.ServeHTTP(resp, req.WithContext(ctx))
		})
	}
}

// Admin middleware: same as Auth but additionally enforces admin role.
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
				slog.Error("token verification failed", "err", err)
				httpErr(resp, http.StatusUnauthorized, "Invalid token")

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
