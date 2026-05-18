package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Email string `json:"email"`
	Role  string `json:"role"`
	jwt.RegisteredClaims
}

type contextKey string

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
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
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
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrTokenInvalidClaims
}

// GetClaims extracts Claims from the request context (set by Auth or Admin middleware).
func GetClaims(r *http.Request) *Claims {
	if c, ok := r.Context().Value(ClaimsKey).(*Claims); ok {
		return c
	}
	return nil
}

func httpErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck
}

// Auth middleware: extracts and validates JWT, adds Claims to context.
func Auth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				httpErr(w, http.StatusUnauthorized, "Missing Authorization header")
				return
			}
			claims, err := VerifyToken(strings.TrimPrefix(auth, "Bearer "), secret)
			if err != nil {
				httpErr(w, http.StatusUnauthorized, "Invalid token: "+err.Error())
				return
			}
			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Admin middleware: same as Auth but additionally enforces admin role.
func Admin(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				httpErr(w, http.StatusUnauthorized, "Missing Authorization header")
				return
			}
			claims, err := VerifyToken(strings.TrimPrefix(auth, "Bearer "), secret)
			if err != nil {
				httpErr(w, http.StatusUnauthorized, "Invalid token: "+err.Error())
				return
			}
			if claims.Role != "admin" {
				httpErr(w, http.StatusForbidden, "Admin access required")
				return
			}
			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
