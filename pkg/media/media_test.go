package media_test

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yutapok/gohcms/pkg/content"
	"github.com/yutapok/gohcms/pkg/media"
)

func setupTestMediaService() (*media.Service, *content.MemoryAuditRepository) {
	storage := media.NewMemoryStorage()
	repo := media.NewMemoryMediaRepository()
	audit := content.NewMemoryAuditRepository()
	svc := media.NewService(storage, repo, audit)
	return svc, audit
}

func TestMediaService_UploadAndDelivery(t *testing.T) {
	svc, audit := setupTestMediaService()
	ctx := context.Background()
	mctx := content.MutationContext{Actor: "test-user", ActorType: content.ActorTypeUser}

	fileContent := []byte("fake-png-image-data-stream")
	uploaded, err := svc.Upload(ctx, "logo.png", "image/png", int64(len(fileContent)), bytes.NewReader(fileContent), mctx)
	if err != nil {
		t.Fatalf("failed to upload media: %v", err)
	}

	if uploaded.ID == "" {
		t.Error("expected non-empty ID")
	}
	if uploaded.Filename != "logo.png" {
		t.Errorf("expected filename 'logo.png', got %s", uploaded.Filename)
	}
	if !uploaded.IsImage() {
		t.Error("expected IsImage to be true for image/png")
	}

	// Verify Audit Log
	logs, _ := audit.List(ctx, "cms_media", uploaded.ID)
	if len(logs) != 1 || logs[0].Operation != content.OpCreate {
		t.Errorf("expected 1 create audit log, got %d", len(logs))
	}

	// Test Delivery Handler
	delivery := media.NewDeliveryHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/media/"+uploaded.ID, nil)
	w := httptest.NewRecorder()
	delivery.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from delivery, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "image/png" {
		t.Errorf("expected Content-Type image/png, got %s", w.Header().Get("Content-Type"))
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected X-Content-Type-Options: nosniff, got %s", w.Header().Get("X-Content-Type-Options"))
	}
	if w.Header().Get("Content-Disposition") != `inline; filename="logo.png"` {
		t.Errorf("expected inline Content-Disposition, got %s", w.Header().Get("Content-Disposition"))
	}
	if !bytes.Equal(w.Body.Bytes(), fileContent) {
		t.Error("delivered content does not match uploaded content")
	}

	// Test Unsafe / Dangerous Upload Delivery (e.g. SVG or HTML with potential script execution)
	svgContent := []byte("<svg><script>alert(1)</script></svg>")
	svgUploaded, err := svc.Upload(ctx, "vector.svg", "image/svg+xml", int64(len(svgContent)), bytes.NewReader(svgContent), mctx)
	if err != nil {
		t.Fatalf("failed to upload svg: %v", err)
	}

	reqSVG := httptest.NewRequest(http.MethodGet, "/media/"+svgUploaded.ID, nil)
	wSVG := httptest.NewRecorder()
	delivery.ServeHTTP(wSVG, reqSVG)

	if wSVG.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from svg delivery, got %d", wSVG.Code)
	}
	if wSVG.Header().Get("Content-Disposition") != `attachment; filename="vector.svg"` {
		t.Errorf("expected attachment Content-Disposition for SVG, got %s", wSVG.Header().Get("Content-Disposition"))
	}
	if wSVG.Header().Get("Content-Security-Policy") != "default-src 'none'; sandbox" {
		t.Errorf("expected CSP sandbox for SVG, got %s", wSVG.Header().Get("Content-Security-Policy"))
	}
}

func TestMediaAPIHandler(t *testing.T) {
	svc, _ := setupTestMediaService()
	apiHandler := media.NewAPIHandler(svc)

	// 1. Upload via multipart POST
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "document.pdf")
	part.Write([]byte("pdf content bytes"))
	writer.Close()

	reqUpload := httptest.NewRequest(http.MethodPost, "/api/media", &body)
	reqUpload.Header.Set("Content-Type", writer.FormDataContentType())
	wUpload := httptest.NewRecorder()
	apiHandler.ServeHTTP(wUpload, reqUpload)

	if wUpload.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on media upload API, got %d. Body: %s", wUpload.Code, wUpload.Body.String())
	}

	// 2. List media via GET /api/media
	reqList := httptest.NewRequest(http.MethodGet, "/api/media", nil)
	wList := httptest.NewRecorder()
	apiHandler.ServeHTTP(wList, reqList)

	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on media list API, got %d", wList.Code)
	}
}

func TestLocalStorage(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := media.NewLocalStorage(tempDir)
	if err != nil {
		t.Fatalf("failed to init local storage: %v", err)
	}

	ctx := context.Background()
	contentData := []byte("local-file-data")
	storedPath, err := storage.Save(ctx, "id-123", "test.txt", bytes.NewReader(contentData))
	if err != nil {
		t.Fatalf("failed to save to local storage: %v", err)
	}

	rsc, err := storage.Open(ctx, storedPath)
	if err != nil {
		t.Fatalf("failed to open stored file: %v", err)
	}
	defer rsc.Close()

	readData, _ := io.ReadAll(rsc)
	if !bytes.Equal(readData, contentData) {
		t.Error("read content does not match saved content")
	}

	if err := storage.Delete(ctx, storedPath); err != nil {
		t.Fatalf("failed to delete stored file: %v", err)
	}
}
