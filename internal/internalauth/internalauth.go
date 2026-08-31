// Package internalauth guards service-to-service routes with a shared
// secret. It is deliberately separate from internal/middleware so that
// services which never handle end-user JWTs (checker-service) do not link
// the JWT machinery just to authenticate their internal callers.
package internalauth

import (
	"crypto/subtle"
	"net/http"

	"github.com/genesary/pupitre/internal/httperr"
)

// HeaderName is the HTTP header carrying the shared secret on internal
// service-to-service calls.
const HeaderName = "X-Internal-Secret"

// Middleware returns middleware that rejects requests whose HeaderName value
// does not match the configured shared secret. The comparison is
// constant-time so that a caller cannot recover the secret byte by byte from
// response timings.
func Middleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			got := req.Header.Get(HeaderName)
			if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
				httperr.Write(resp, http.StatusUnauthorized, "invalid internal secret")

				return
			}

			next.ServeHTTP(resp, req)
		})
	}
}
