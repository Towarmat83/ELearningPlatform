package middleware

import (
	"crypto/subtle"
	"net/http"
)

// internalSecretHeader is the HTTP header used to authenticate
// service-to-service calls on /internal/* routes.
const internalSecretHeader = "X-Internal-Secret"

// InternalAuth returns middleware that rejects requests whose
// X-Internal-Secret header does not match the configured shared secret.
func InternalAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			got := req.Header.Get(internalSecretHeader)
			if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
				httpErr(resp, http.StatusUnauthorized, "invalid internal secret")

				return
			}

			next.ServeHTTP(resp, req)
		})
	}
}
