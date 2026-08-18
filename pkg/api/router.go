package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/yutapok/gohcms/pkg/content"
	"github.com/yutapok/gohcms/pkg/openapi"
	"github.com/yutapok/gohcms/pkg/schema"
)

// APIHandler coordinates REST API routes.
type APIHandler struct {
	svc         *content.ContentService
	definitions []*schema.ResourceDefinition
	openapiDoc  []byte
}

// NewHandler creates a new HTTP handler for the Headless REST API.
func NewHandler(svc *content.ContentService, definitions []*schema.ResourceDefinition) http.Handler {
	gen := openapi.NewGenerator("gohcms Headless API", "1.0.0", "Auto-generated REST API for content resources")
	openAPIData, _ := gen.ToJSON(definitions)

	h := &APIHandler{
		svc:         svc,
		definitions: definitions,
		openapiDoc:  openAPIData,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/openapi.json", h.handleOpenAPI)
	mux.HandleFunc("/api/", h.handleDispatch)

	return mux
}

func (h *APIHandler) getDefinition(name string) *schema.ResourceDefinition {
	for _, d := range h.definitions {
		if d.Resource == name {
			return d
		}
	}
	return nil
}

func (h *APIHandler) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(h.openapiDoc)
}

func (h *APIHandler) handleDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Resource endpoint not found")
		return
	}

	resourceName := parts[0]
	def := h.getDefinition(resourceName)
	if def == nil {
		writeError(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", fmt.Sprintf("Resource '%s' not registered", resourceName))
		return
	}

	ctx := r.Context()
	mctx := content.MutationContext{
		Actor:     "api-client",
		ActorType: content.ActorTypeAPIClient,
		RequestID: r.Header.Get("X-Request-ID"),
	}
	if mctx.RequestID == "" {
		mctx.RequestID = fmt.Sprintf("req-%d", r.Context().Value("req_id"))
	}

	// 1. /api/{resource}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			// List records
			limit := 20
			offset := 0
			if lStr := r.URL.Query().Get("limit"); lStr != "" {
				if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
					limit = l
				}
			}
			if oStr := r.URL.Query().Get("offset"); oStr != "" {
				if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
					offset = o
				}
			}

			filter := content.ContentFilter{}
			statusParam := r.URL.Query().Get("status")
			if statusParam == "" {
				// Default to published records only for resources with managed lifecycle
				if def.Lifecycle.Mode == schema.LifecycleModeManaged {
					pubStatus := content.StatusPublished
					filter.Status = &pubStatus
				}
			} else if statusParam != "all" {
				st := content.ContentStatus(statusParam)
				filter.Status = &st
			}

			records, total, err := h.svc.List(ctx, def, filter, content.Pagination{Limit: limit, Offset: offset})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"data": records,
				"meta": map[string]interface{}{
					"total":  total,
					"limit":  limit,
					"offset": offset,
				},
			})
			return

		case http.MethodPost:
			// Create record (Max 2MB body limit)
			r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_JSON", "Failed to parse JSON request body or payload exceeded limit")
				return
			}

			record, err := h.svc.Create(ctx, def, payload, mctx)
			if err != nil {
				writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
				return
			}

			writeJSON(w, http.StatusCreated, map[string]interface{}{
				"data": record,
			})
			return
		}
	}

	// 2. /api/{resource}/{id}
	if len(parts) == 2 {
		recordID := parts[1]
		switch r.Method {
		case http.MethodGet:
			// Get single record
			record, err := h.svc.Get(ctx, def, recordID)
			if err != nil {
				writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("Record '%s' not found", recordID))
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"data": record,
			})
			return

		case http.MethodPatch, http.MethodPut:
			// Update record (Max 2MB body limit)
			r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_JSON", "Failed to parse JSON request body or payload exceeded limit")
				return
			}

			record, err := h.svc.Update(ctx, def, recordID, payload, mctx)
			if err != nil {
				writeError(w, http.StatusBadRequest, "UPDATE_FAILED", err.Error())
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"data": record,
			})
			return

		case http.MethodDelete:
			// Delete record
			if err := h.svc.Delete(ctx, def, recordID, mctx); err != nil {
				writeError(w, http.StatusBadRequest, "DELETE_FAILED", err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	// 3. /api/{resource}/{id}/{action} (publish, unpublish, finish)
	if len(parts) == 3 && r.Method == http.MethodPost {
		recordID := parts[1]
		action := parts[2]

		var record *content.Record
		var err error

		switch action {
		case "publish":
			record, err = h.svc.Publish(ctx, def, recordID, mctx)
		case "unpublish":
			record, err = h.svc.Unpublish(ctx, def, recordID, mctx)
		case "finish":
			record, err = h.svc.Finish(ctx, def, recordID, mctx)
		default:
			writeError(w, http.StatusNotFound, "ACTION_NOT_FOUND", fmt.Sprintf("Action '%s' not supported", action))
			return
		}

		if err != nil {
			writeError(w, http.StatusBadRequest, "TRANSITION_FAILED", err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": record,
		})
		return
	}

	writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed for this route")
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
