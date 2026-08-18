package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yutapok/gohcms/pkg/auth"
	"github.com/yutapok/gohcms/pkg/content"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	handler := auth.SecurityHeadersMiddleware(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("expected X-Frame-Options DENY, got %s", w.Header().Get("X-Frame-Options"))
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected X-Content-Type-Options nosniff, got %s", w.Header().Get("X-Content-Type-Options"))
	}
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Error("expected Content-Security-Policy header to be set")
	}
}

func TestCSRFProtectionMiddleware(t *testing.T) {
	handler := auth.CSRFProtectionMiddleware(okHandler())

	// 1. Safe GET request -> 200 OK (no CSRF check)
	reqGet := httptest.NewRequest(http.MethodGet, "/admin/resources/article", nil)
	reqGet.Header.Set("Sec-Fetch-Site", "cross-site")
	wGet := httptest.NewRecorder()
	handler.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Errorf("expected 200 for GET, got %d", wGet.Code)
	}

	// 2. Cross-site mutating POST -> 403 Forbidden
	reqCross := httptest.NewRequest(http.MethodPost, "/admin/resources/article", nil)
	reqCross.Header.Set("Sec-Fetch-Site", "cross-site")
	wCross := httptest.NewRecorder()
	handler.ServeHTTP(wCross, reqCross)
	if wCross.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for cross-site POST, got %d", wCross.Code)
	}

	// 3. Same-origin mutating POST with Origin -> 200 OK
	reqSame := httptest.NewRequest(http.MethodPost, "http://localhost:8080/admin/resources/article", nil)
	reqSame.Host = "localhost:8080"
	reqSame.Header.Set("Origin", "http://localhost:8080")
	wSame := httptest.NewRecorder()
	handler.ServeHTTP(wSame, reqSame)
	if wSame.Code != http.StatusOK {
		t.Errorf("expected 200 for same-origin POST, got %d", wSame.Code)
	}

	// 4. Mismatched Origin mutating POST -> 403 Forbidden
	reqMismatch := httptest.NewRequest(http.MethodPost, "http://localhost:8080/admin/resources/article", nil)
	reqMismatch.Host = "localhost:8080"
	reqMismatch.Header.Set("Origin", "http://evil-attacker.com")
	wMismatch := httptest.NewRecorder()
	handler.ServeHTTP(wMismatch, reqMismatch)
	if wMismatch.Code != http.StatusForbidden {
		t.Errorf("expected 403 for mismatched origin, got %d", wMismatch.Code)
	}

	// 5. Fail-Closed: Mutating POST with NO headers -> 403 Forbidden
	reqNoHeaders := httptest.NewRequest(http.MethodPost, "http://localhost:8080/admin/resources/article", nil)
	reqNoHeaders.Host = "localhost:8080"
	wNoHeaders := httptest.NewRecorder()
	handler.ServeHTTP(wNoHeaders, reqNoHeaders)
	if wNoHeaders.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for POST without headers (fail-closed), got %d", wNoHeaders.Code)
	}

	// 6. Sec-Fetch-Site: same-origin -> 200 OK
	reqSecSame := httptest.NewRequest(http.MethodPost, "http://localhost:8080/admin/resources/article", nil)
	reqSecSame.Host = "localhost:8080"
	reqSecSame.Header.Set("Sec-Fetch-Site", "same-origin")
	wSecSame := httptest.NewRecorder()
	handler.ServeHTTP(wSecSame, reqSecSame)
	if wSecSame.Code != http.StatusOK {
		t.Errorf("expected 200 OK for Sec-Fetch-Site same-origin, got %d", wSecSame.Code)
	}

	// 7. IPv6 loopback match [::1] -> 200 OK
	reqIPv6 := httptest.NewRequest(http.MethodPost, "http://[::1]:8080/admin/resources/article", nil)
	reqIPv6.Host = "[::1]:8080"
	reqIPv6.Header.Set("Origin", "http://[::1]:8080")
	wIPv6 := httptest.NewRecorder()
	handler.ServeHTTP(wIPv6, reqIPv6)
	if wIPv6.Code != http.StatusOK {
		t.Errorf("expected 200 OK for IPv6 origin match, got %d", wIPv6.Code)
	}
}

func TestAPIKeyMiddleware_DynamicService(t *testing.T) {
	memRepo := auth.NewMemoryAPIKeyRepository()
	memAudit := content.NewMemoryAuditRepository()
	authSvc := auth.NewService(memRepo, memAudit)
	ctx := context.Background()
	mctx := content.MutationContext{Actor: "admin", ActorType: content.ActorTypeUser}

	// 1. Create Read-Only Key
	readToken, _, err := authSvc.CreateKey(ctx, "Read Token", auth.PermissionRead, mctx)
	if err != nil {
		t.Fatalf("failed to create read key: %v", err)
	}

	// 2. Create Read-Write Key
	rwToken, rwKey, err := authSvc.CreateKey(ctx, "RW Token", auth.PermissionReadWrite, mctx)
	if err != nil {
		t.Fatalf("failed to create rw key: %v", err)
	}

	handler := auth.APIKeyMiddleware("", authSvc)(okHandler())

	// Test GET with Read-Only Key -> 200 OK
	reqGet := httptest.NewRequest(http.MethodGet, "/api/article", nil)
	reqGet.Header.Set("Authorization", "Bearer "+readToken)
	wGet := httptest.NewRecorder()
	handler.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Errorf("expected 200 OK for GET with read token, got %d", wGet.Code)
	}

	// Test POST with Read-Only Key -> 403 Forbidden
	reqPost := httptest.NewRequest(http.MethodPost, "/api/article", nil)
	reqPost.Header.Set("Authorization", "Bearer "+readToken)
	wPost := httptest.NewRecorder()
	handler.ServeHTTP(wPost, reqPost)
	if wPost.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for POST with read token, got %d", wPost.Code)
	}

	// Test POST with Read-Write Key -> 200 OK
	reqPostRW := httptest.NewRequest(http.MethodPost, "/api/article", nil)
	reqPostRW.Header.Set("X-API-Key", rwToken)
	wPostRW := httptest.NewRecorder()
	handler.ServeHTTP(wPostRW, reqPostRW)
	if wPostRW.Code != http.StatusOK {
		t.Errorf("expected 200 OK for POST with rw token, got %d", wPostRW.Code)
	}

	// Revoke RW Key
	if err := authSvc.RevokeKey(ctx, rwKey.ID, mctx); err != nil {
		t.Fatalf("failed to revoke key: %v", err)
	}

	// Test POST with Revoked Key -> 401 Unauthorized
	reqRevoked := httptest.NewRequest(http.MethodPost, "/api/article", nil)
	reqRevoked.Header.Set("X-API-Key", rwToken)
	wRevoked := httptest.NewRecorder()
	handler.ServeHTTP(wRevoked, reqRevoked)
	if wRevoked.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for revoked key, got %d", wRevoked.Code)
	}
}

func TestBasicAuthMiddleware(t *testing.T) {
	user := "admin"
	pass := "secret"
	handler := auth.BasicAuthMiddleware(user, pass)(okHandler())

	// 1. No credentials -> 401
	reqNoAuth := httptest.NewRequest(http.MethodGet, "/admin", nil)
	wNoAuth := httptest.NewRecorder()
	handler.ServeHTTP(wNoAuth, reqNoAuth)
	if wNoAuth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing basic auth, got %d", wNoAuth.Code)
	}

	// 2. Correct credentials -> 200
	reqAuth := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqAuth.SetBasicAuth(user, pass)
	wAuth := httptest.NewRecorder()
	handler.ServeHTTP(wAuth, reqAuth)
	if wAuth.Code != http.StatusOK {
		t.Errorf("expected 200 for valid basic auth, got %d", wAuth.Code)
	}
}
