package auth

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// CSRFProtectionMiddleware enforces fail-closed Cross-Site Request Forgery (CSRF) protection
// for state-mutating requests (POST, PUT, PATCH, DELETE) against the Admin UI.
//
// Enforcement rules:
// 1. Safe methods (GET, HEAD, OPTIONS) are permitted without CSRF checks.
// 2. If Sec-Fetch-Site is "cross-site", the request is immediately rejected (403).
// 3. For state-mutating requests, at least one valid trusted source indicator (Origin, Referer,
//    or Sec-Fetch-Site: same-origin/same-site) MUST be present and match the server's Host.
// 4. If neither Origin, Referer, nor valid Sec-Fetch-Site is provided, the request is rejected (Fail-Closed).
func CSRFProtectionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Safe methods pass through
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
			next.ServeHTTP(w, r)
			return
		}

		// 2. Reject explicit cross-site fetch
		secFetchSite := r.Header.Get("Sec-Fetch-Site")
		if secFetchSite == "cross-site" {
			http.Error(w, "Forbidden: Cross-site request rejected (CSRF Protection)", http.StatusForbidden)
			return
		}

		// 3. Validate Origin header if present
		origin := r.Header.Get("Origin")
		if origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !isHostAndSchemeMatch(u, r) {
				http.Error(w, "Forbidden: Origin verification failed (CSRF Protection)", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// 4. Validate Referer header if present
		referer := r.Header.Get("Referer")
		if referer != "" {
			u, err := url.Parse(referer)
			if err != nil || !isHostAndSchemeMatch(u, r) {
				http.Error(w, "Forbidden: Referer verification failed (CSRF Protection)", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// 5. If Sec-Fetch-Site is explicitly same-origin or same-site, allow
		if secFetchSite == "same-origin" || secFetchSite == "same-site" {
			next.ServeHTTP(w, r)
			return
		}

		// 6. Fail-Closed: No valid origin/referer/sec-fetch-site metadata present on mutating request
		http.Error(w, "Forbidden: Missing CSRF verification headers on mutating request (CSRF Protection)", http.StatusForbidden)
	})
}

// isHostAndSchemeMatch verifies that the target URL host and scheme match the incoming HTTP request.
func isHostAndSchemeMatch(target *url.URL, r *http.Request) bool {
	if target == nil {
		return false
	}

	// Scheme check: If request is over HTTPS, origin/referer must also be HTTPS
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		if strings.ToLower(target.Scheme) != "https" {
			return false
		}
	}

	targetHost := extractHost(target.Host)
	reqHost := extractHost(r.Host)

	if targetHost == "" || reqHost == "" {
		return false
	}

	// Exact match
	if strings.EqualFold(targetHost, reqHost) {
		return true
	}

	// Loopback equivalence (localhost vs 127.0.0.1 vs [::1])
	if isLoopback(targetHost) && isLoopback(reqHost) {
		return true
	}

	return false
}

// extractHost safely extracts the hostname without port for both IPv4, IPv6, and domain names.
func extractHost(hostWithPort string) string {
	if hostWithPort == "" {
		return ""
	}
	h, _, err := net.SplitHostPort(hostWithPort)
	if err == nil {
		return strings.Trim(h, "[]")
	}
	// No port present, strip brackets if IPv6
	return strings.Trim(hostWithPort, "[]")
}

func isLoopback(h string) bool {
	h = strings.ToLower(strings.Trim(h, "[]"))
	if h == "localhost" || h == "127.0.0.1" || h == "::1" || h == "0.0.0.0" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
