package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Config holds authentication credentials.
type Config struct {
	StaticAPIKey  string // Optional static API Key for fallback
	AdminUsername string // Basic Auth username for /admin/ routes (optional)
	AdminPassword string // Basic Auth password for /admin/ routes (optional)
	Service       *Service
}

// SecurityHeadersMiddleware adds essential HTTP security headers (CSP, X-Frame-Options, Content-Type-Options).
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' https://unpkg.com https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data: /media/;")
		next.ServeHTTP(w, r)
	})
}

// APIKeyMiddleware validates API key from 'Authorization: Bearer <key>' or 'X-API-Key: <key>' headers.
func APIKeyMiddleware(staticKey string, authSvc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Allow OpenAPI spec without authentication
			if r.URL.Path == "/api/openapi.json" {
				next.ServeHTTP(w, r)
				return
			}

			// If no static key is configured and no dynamic service is provided, pass through (public mode)
			if staticKey == "" && authSvc == nil {
				next.ServeHTTP(w, r)
				return
			}

			reqKey := r.Header.Get("X-API-Key")
			if reqKey == "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					reqKey = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			if reqKey == "" {
				writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing API key in Authorization or X-API-Key header")
				return
			}

			// 1. Check static API Key if configured
			if staticKey != "" && subtle.ConstantTimeCompare([]byte(reqKey), []byte(staticKey)) == 1 {
				next.ServeHTTP(w, r)
				return
			}

			// 2. Check dynamic database API Key if service available
			if authSvc != nil {
				key, err := authSvc.ValidateToken(r.Context(), reqKey, r.Method)
				if err != nil {
					if strings.Contains(err.Error(), "permission") {
						writeAuthError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
						return
					}
					writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
					return
				}
				_ = key
				next.ServeHTTP(w, r)
				return
			}

			writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid API key")
		})
	}
}

// BasicAuthMiddleware protects routes with HTTP Basic Authentication.
func BasicAuthMiddleware(username, password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If no credentials configured, pass through
			if username == "" || password == "" {
				next.ServeHTTP(w, r)
				return
			}

			user, pass, ok := r.BasicAuth()
			if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(username)) != 1 || subtle.ConstantTimeCompare([]byte(pass), []byte(password)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="gohcms Admin"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(`{"error":{"code":"` + code + `","message":"` + message + `"}}`))
}
