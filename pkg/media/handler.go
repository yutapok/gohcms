package media

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/yutapok/gohcms/pkg/content"
)

// APIHandler handles REST API routes for media.
type APIHandler struct {
	svc *Service
}

// NewAPIHandler creates a handler for /api/media endpoints.
func NewAPIHandler(svc *Service) http.Handler {
	h := &APIHandler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/media", h.handleMediaCollection)
	mux.HandleFunc("/api/media/", h.handleMediaItem)
	return mux
}

// DeliveryHandler serves binary files at /media/{id}.
func NewDeliveryHandler(svc *Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/media/")
		id = strings.Split(id, "/")[0] // Handle /media/{id} or /media/{id}/{filename}

		if id == "" {
			http.NotFound(w, r)
			return
		}

		stream, meta, err := svc.OpenFile(r.Context(), id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer stream.Close()

		if meta.MimeType != "" {
			w.Header().Set("Content-Type", meta.MimeType)
		}
		w.Header().Set("Cache-Control", "private, max-age=3600, no-transform")
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent Stored XSS via SVG or HTML:
		// Safe image MIME types can be displayed inline; scriptable/unknown types force attachment with sandbox CSP
		if isSafeInlineMime(meta.MimeType) {
			w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", meta.Filename))
		} else {
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", meta.Filename))
			w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		}

		http.ServeContent(w, r, meta.Filename, meta.CreatedAt, stream)
	})
}

func isSafeInlineMime(mime string) bool {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/jpeg", "image/png", "image/webp", "image/gif", "image/avif":
		return true
	default:
		return false
	}
}

func (h *APIHandler) handleMediaCollection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	mctx := content.MutationContext{
		Actor:     "api-client",
		ActorType: content.ActorTypeAPIClient,
		RequestID: r.Header.Get("X-Request-ID"),
	}

	switch r.Method {
	case http.MethodGet:
		items, err := h.svc.List(ctx)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": items,
		})
		return

	case http.MethodPost:
		// Restrict request body size to 32MB to prevent upload memory exhaustion
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

		// Parse multipart form up to 32MB
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeJSONError(w, http.StatusBadRequest, "INVALID_FORM", "Failed to parse multipart form or file exceeds 32MB limit")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "MISSING_FILE", "Field 'file' is required in form-data")
			return
		}
		defer file.Close()

		mimeType := header.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		item, err := h.svc.Upload(ctx, header.Filename, mimeType, header.Size, file, mctx)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "UPLOAD_FAILED", err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"data": item,
		})
		return
	}

	writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
}

func (h *APIHandler) handleMediaItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := strings.TrimPrefix(r.URL.Path, "/api/media/")
	id = strings.Trim(id, "/")

	if id == "" {
		writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "Media ID required")
		return
	}

	mctx := content.MutationContext{
		Actor:     "api-client",
		ActorType: content.ActorTypeAPIClient,
		RequestID: r.Header.Get("X-Request-ID"),
	}

	switch r.Method {
	case http.MethodGet:
		item, err := h.svc.Get(ctx, id)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("Media '%s' not found", id))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": item,
		})
		return

	case http.MethodDelete:
		if err := h.svc.Delete(ctx, id, mctx); err != nil {
			writeJSONError(w, http.StatusBadRequest, "DELETE_FAILED", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
