package admin

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/yutapok/gohcms/pkg/api"
	"github.com/yutapok/gohcms/pkg/auth"
	"github.com/yutapok/gohcms/pkg/content"
	"github.com/yutapok/gohcms/pkg/introspection"
	"github.com/yutapok/gohcms/pkg/media"
	"github.com/yutapok/gohcms/pkg/schema"
	"github.com/yutapok/gohcms/pkg/validator"
)

//go:embed templates/* static/*
var contentFS embed.FS

// Role specifies which endpoints are served.
type Role string

const (
	RoleAll   Role = "all"
	RoleAdmin Role = "admin"
	RoleAPI   Role = "api"
)

// Server is the HTTP server for gohcms Admin UI.
type Server struct {
	svc         *content.ContentService
	auditor     content.AuditRepository
	reviser     content.RevisionRepository
	mediaSvc    *media.Service
	authSvc     *auth.Service
	definitions []*schema.ResourceDefinition
	dbSchema    *introspection.DatabaseSchema
	valResult   *validator.ValidationResult
	authConfig  auth.Config
	templates   map[string]*template.Template
	apiHandler  http.Handler
}

// NewServer creates a new Admin UI server.
func NewServer(
	svc *content.ContentService,
	auditor content.AuditRepository,
	reviser content.RevisionRepository,
	definitions []*schema.ResourceDefinition,
	dbSchema *introspection.DatabaseSchema,
	valResult *validator.ValidationResult,
) (*Server, error) {
	return NewServerWithFull(svc, auditor, reviser, nil, nil, definitions, dbSchema, valResult, auth.Config{})
}

// NewServerWithAuth creates a new Admin UI server with custom authentication settings.
func NewServerWithAuth(
	svc *content.ContentService,
	auditor content.AuditRepository,
	reviser content.RevisionRepository,
	definitions []*schema.ResourceDefinition,
	dbSchema *introspection.DatabaseSchema,
	valResult *validator.ValidationResult,
	authConfig auth.Config,
) (*Server, error) {
	return NewServerWithFull(svc, auditor, reviser, nil, authConfig.Service, definitions, dbSchema, valResult, authConfig)
}

// NewServerWithFull creates a new Admin UI server with all services configured.
func NewServerWithFull(
	svc *content.ContentService,
	auditor content.AuditRepository,
	reviser content.RevisionRepository,
	mediaSvc *media.Service,
	authSvc *auth.Service,
	definitions []*schema.ResourceDefinition,
	dbSchema *introspection.DatabaseSchema,
	valResult *validator.ValidationResult,
	authConfig auth.Config,
) (*Server, error) {
	if mediaSvc == nil {
		memStorage := media.NewMemoryStorage()
		memRepo := media.NewMemoryMediaRepository()
		mediaSvc = media.NewService(memStorage, memRepo, auditor)
	}
	if authSvc == nil {
		memKeyRepo := auth.NewMemoryAPIKeyRepository()
		authSvc = auth.NewService(memKeyRepo, auditor)
	}

	s := &Server{
		svc:         svc,
		auditor:     auditor,
		reviser:     reviser,
		mediaSvc:    mediaSvc,
		authSvc:     authSvc,
		definitions: definitions,
		dbSchema:    dbSchema,
		valResult:   valResult,
		authConfig:  authConfig,
		templates:   make(map[string]*template.Template),
		apiHandler:  api.NewHandler(svc, definitions),
	}

	if err := s.parseTemplates(); err != nil {
		return nil, fmt.Errorf("failed to parse admin templates: %w", err)
	}

	return s, nil
}

func formatDateTimeLocal(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case time.Time:
		return v.In(time.Local).Format("2006-01-02T15:04")
	case string:
		if v == "" {
			return ""
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t.In(time.Local).Format("2006-01-02T15:04")
		}
		if len(v) >= 16 && strings.Contains(v, "T") {
			return v[:16]
		}
		return v
	default:
		return fmt.Sprintf("%v", val)
	}
}

func (s *Server) parseTemplates() error {
	funcMap := template.FuncMap{
		"formatDateTimeLocal": formatDateTimeLocal,
	}

	views := []string{
		"content_list_table", "content_list_kanban", "content_list_timeline",
		"content_form", "status", "media_list", "api_keys",
	}
	for _, view := range views {
		tmpl, err := template.New("layout.html").Funcs(funcMap).ParseFS(contentFS, "templates/layout.html", fmt.Sprintf("templates/%s.html", view))
		if err != nil {
			return fmt.Errorf("error parsing template %s: %w", view, err)
		}
		s.templates[view] = tmpl
	}

	// Modal partials
	historyTmpl, err := template.New("content_history.html").Funcs(funcMap).ParseFS(contentFS, "templates/content_history.html")
	if err != nil {
		return fmt.Errorf("error parsing history modal: %w", err)
	}
	s.templates["content_history"] = historyTmpl

	pickerTmpl, err := template.New("media_picker_modal.html").Funcs(funcMap).ParseFS(contentFS, "templates/media_picker_modal.html")
	if err != nil {
		return fmt.Errorf("error parsing media picker modal: %w", err)
	}
	s.templates["media_picker_modal"] = pickerTmpl

	return nil
}

// Handler returns the HTTP handler with all admin, media, auth, and REST API routes configured (default: RoleAll).
func (s *Server) Handler() http.Handler {
	return s.HandlerForRole(RoleAll)
}

// HandlerForRole returns an HTTP handler isolated for the specified runtime role.
func (s *Server) HandlerForRole(role Role) http.Handler {
	switch role {
	case RoleAPI:
		// API Role: Only serve Headless REST API and Media binary delivery
		mux := http.NewServeMux()
		mux.Handle("/media/", media.NewDeliveryHandler(s.mediaSvc))

		apiMux := http.NewServeMux()
		apiMux.Handle("/api/media", media.NewAPIHandler(s.mediaSvc))
		apiMux.Handle("/api/media/", media.NewAPIHandler(s.mediaSvc))
		apiMux.Handle("/api/", s.apiHandler)

		apiProtected := auth.APIKeyMiddleware(s.authConfig.StaticAPIKey, s.authSvc)(apiMux)
		mux.Handle("/api/", apiProtected)

		return auth.SecurityHeadersMiddleware(mux)

	case RoleAdmin:
		// Admin Role: Only serve Admin UI, Static Assets, and Media binary delivery
		adminMux := http.NewServeMux()
		adminMux.Handle("/media/", media.NewDeliveryHandler(s.mediaSvc))
		adminMux.Handle("/static/", http.FileServer(http.FS(contentFS)))
		adminMux.HandleFunc("/", s.handleRoot)
		adminMux.HandleFunc("/admin/status", s.handleStatus)
		adminMux.HandleFunc("/admin/media", s.handleMedia)
		adminMux.HandleFunc("/admin/media/", s.handleMediaDispatch)
		adminMux.HandleFunc("/admin/api-keys", s.handleAPIKeys)
		adminMux.HandleFunc("/admin/api-keys/", s.handleAPIKeysDispatch)
		adminMux.HandleFunc("/admin/resources/", s.handleResourceDispatch)

		adminCSRF := auth.CSRFProtectionMiddleware(adminMux)
		adminProtected := auth.BasicAuthMiddleware(s.authConfig.AdminUsername, s.authConfig.AdminPassword)(adminCSRF)
		return auth.SecurityHeadersMiddleware(adminProtected)

	default: // RoleAll
		mux := http.NewServeMux()

		// 1. Binary Media Delivery Route (/media/{id})
		mux.Handle("/media/", media.NewDeliveryHandler(s.mediaSvc))

		// 2. Headless REST API routes (/api/...) with optional API Key Auth
		apiMux := http.NewServeMux()
		apiMux.Handle("/api/media", media.NewAPIHandler(s.mediaSvc))
		apiMux.Handle("/api/media/", media.NewAPIHandler(s.mediaSvc))
		apiMux.Handle("/api/", s.apiHandler)

		apiProtected := auth.APIKeyMiddleware(s.authConfig.StaticAPIKey, s.authSvc)(apiMux)
		mux.Handle("/api/", apiProtected)

		// 3. Static Assets
		mux.Handle("/static/", http.FileServer(http.FS(contentFS)))

		// 4. Admin UI routes with CSRF Protection and optional Basic Auth
		adminMux := http.NewServeMux()
		adminMux.HandleFunc("/", s.handleRoot)
		adminMux.HandleFunc("/admin/status", s.handleStatus)
		adminMux.HandleFunc("/admin/media", s.handleMedia)
		adminMux.HandleFunc("/admin/media/", s.handleMediaDispatch)
		adminMux.HandleFunc("/admin/api-keys", s.handleAPIKeys)
		adminMux.HandleFunc("/admin/api-keys/", s.handleAPIKeysDispatch)
		adminMux.HandleFunc("/admin/resources/", s.handleResourceDispatch)

		adminCSRF := auth.CSRFProtectionMiddleware(adminMux)
		adminProtected := auth.BasicAuthMiddleware(s.authConfig.AdminUsername, s.authConfig.AdminPassword)(adminCSRF)
		mux.Handle("/", adminProtected)

		return auth.SecurityHeadersMiddleware(mux)
	}
}

func (s *Server) getDefinition(name string) *schema.ResourceDefinition {
	for _, d := range s.definitions {
		if d.Resource == name {
			return d
		}
	}
	return nil
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if len(s.definitions) > 0 {
		http.Redirect(w, r, fmt.Sprintf("/admin/resources/%s", s.definitions[0].Resource), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/admin/status", http.StatusFound)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":            "System Status",
		"Resources":        s.definitions,
		"ValidationResult": s.valResult,
		"ValidationReport": s.valResult.FormatReport(),
	}
	s.templates["status"].Execute(w, data)
}

// Media Management Handlers
func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	mctx := content.MutationContext{
		Actor:     "admin-user",
		ActorType: content.ActorTypeUser,
		RequestID: "req-admin-media",
	}

	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "Failed to parse file upload (exceeds 32MB limit): "+err.Error(), http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Missing file in upload", http.StatusBadRequest)
			return
		}
		defer file.Close()

		mimeType := header.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		_, err = s.mediaSvc.Upload(ctx, header.Filename, mimeType, header.Size, file, mctx)
		if err != nil {
			http.Error(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/admin/media", http.StatusSeeOther)
		return
	}

	items, err := s.mediaSvc.List(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Title":      "Media Library",
		"Resources":  s.definitions,
		"MediaItems": items,
	}
	s.templates["media_list"].Execute(w, data)
}

func (s *Server) handleMediaDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/media/")
	ctx := r.Context()
	mctx := content.MutationContext{
		Actor:     "admin-user",
		ActorType: content.ActorTypeUser,
		RequestID: "req-admin-media",
	}

	// /admin/media/picker?target_field=cover_image
	if path == "picker" {
		targetField := r.URL.Query().Get("target_field")
		items, _ := s.mediaSvc.List(ctx)
		data := map[string]interface{}{
			"TargetField": targetField,
			"MediaItems":  items,
		}
		s.templates["media_picker_modal"].Execute(w, data)
		return
	}

	// /admin/media/upload-inline?target_field=cover_image
	if path == "upload-inline" && r.Method == http.MethodPost {
		targetField := r.URL.Query().Get("target_field")
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
		if err := r.ParseMultipartForm(32 << 20); err == nil {
			file, header, err := r.FormFile("file")
			if err == nil {
				defer file.Close()
				mimeType := header.Header.Get("Content-Type")
				item, err := s.mediaSvc.Upload(ctx, header.Filename, mimeType, header.Size, file, mctx)
				if err == nil {
					// Safely encode JSON to eliminate XSS
					w.Header().Set("Content-Type", "text/html")
					jsonTarget, _ := json.Marshal(targetField)
					jsonID, _ := json.Marshal(item.ID)
					jsonFilename, _ := json.Marshal(item.Filename)
					fmt.Fprintf(w, `<script>selectMedia(%s, %s, %s, %t);</script>`, jsonTarget, jsonID, jsonFilename, item.IsImage())
					return
				}
			}
		}
		http.Error(w, "Upload failed", http.StatusBadRequest)
		return
	}

	// /admin/media/{id} DELETE
	if r.Method == http.MethodDelete {
		mediaID := path
		if err := s.mediaSvc.Delete(ctx, mediaID, mctx); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusOK)
		return
	}

	http.NotFound(w, r)
}

// API Key Management Handlers
func (s *Server) handleAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	mctx := content.MutationContext{
		Actor:     "admin-user",
		ActorType: content.ActorTypeUser,
		RequestID: "req-admin-apikeys",
	}

	var generatedToken string

	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		perm := auth.Permission(r.FormValue("permission"))
		token, _, err := s.authSvc.CreateKey(ctx, name, perm, mctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		generatedToken = token
	}

	keys, err := s.authSvc.ListKeys(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Title":          "API Keys",
		"Resources":      s.definitions,
		"APIKeys":        keys,
		"GeneratedToken": generatedToken,
	}
	s.templates["api_keys"].Execute(w, data)
}

func (s *Server) handleAPIKeysDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/api-keys/")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) == 2 && parts[1] == "revoke" && r.Method == http.MethodPost {
		keyID := parts[0]
		ctx := r.Context()
		mctx := content.MutationContext{
			Actor:     "admin-user",
			ActorType: content.ActorTypeUser,
			RequestID: "req-admin-apikeys",
		}
		if err := s.authSvc.RevokeKey(ctx, keyID, mctx); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "/admin/api-keys", http.StatusSeeOther)
		return
	}

	http.NotFound(w, r)
}

// Content Resource Handlers
func (s *Server) handleResourceDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/resources/")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	resourceName := parts[0]
	def := s.getDefinition(resourceName)
	if def == nil {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()

	// /admin/resources/{resource}
	if len(parts) == 1 {
		if r.Method == http.MethodGet {
			s.handleContentList(w, r, def)
			return
		} else if r.Method == http.MethodPost {
			s.handleContentCreate(w, r, def)
			return
		}
	}

	// /admin/resources/{resource}/new
	if len(parts) == 2 && parts[1] == "new" {
		s.handleContentNew(w, r, def)
		return
	}

	// /admin/resources/{resource}/{id}
	if len(parts) == 2 {
		recordID := parts[1]
		if r.Method == http.MethodPost {
			s.handleContentUpdate(w, r, def, recordID)
			return
		} else if r.Method == http.MethodDelete {
			s.handleContentDelete(w, r, def, recordID)
			return
		}
	}

	// /admin/resources/{resource}/{id}/{action}
	if len(parts) == 3 {
		recordID := parts[1]
		action := parts[2]

		mctx := content.MutationContext{
			Actor:     "admin-user",
			ActorType: content.ActorTypeUser,
			RequestID: "req-admin-ui",
		}

		switch action {
		case "edit":
			s.handleContentEdit(w, r, def, recordID)
		case "history":
			s.handleContentHistory(w, r, def, recordID)
		case "publish":
			rec, err := s.svc.Publish(ctx, def, recordID, mctx)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.renderUpdatedRowOrCard(w, r, def, rec)
		case "unpublish":
			rec, err := s.svc.Unpublish(ctx, def, recordID, mctx)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.renderUpdatedRowOrCard(w, r, def, rec)
		case "finish":
			rec, err := s.svc.Finish(ctx, def, recordID, mctx)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.renderUpdatedRowOrCard(w, r, def, rec)
		default:
			http.NotFound(w, r)
		}
		return
	}

	http.NotFound(w, r)
}

func (s *Server) handleContentList(w http.ResponseWriter, r *http.Request, def *schema.ResourceDefinition) {
	ctx := r.Context()
	records, _, err := s.svc.List(ctx, def, content.ContentFilter{}, content.Pagination{Limit: 100})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	view := r.URL.Query().Get("view")
	if view == "" {
		view = "table"
	}

	var draftRecords, pubRecords, finRecords []*content.Record
	for _, rec := range records {
		switch rec.Status {
		case content.StatusDraft:
			draftRecords = append(draftRecords, rec)
		case content.StatusPublished:
			pubRecords = append(pubRecords, rec)
		case content.StatusFinished:
			finRecords = append(finRecords, rec)
		}
	}

	data := map[string]interface{}{
		"Title":            def.Resource,
		"CurrentResource":  def.Resource,
		"Resources":        s.definitions,
		"Resource":         def,
		"Records":          records,
		"DraftRecords":     draftRecords,
		"PublishedRecords": pubRecords,
		"FinishedRecords":  finRecords,
		"Today":            time.Now().Format("2006-01-02"),
		"NowFormatted":     time.Now().Format("2006-01-02 15:04"),
	}

	tmplName := fmt.Sprintf("content_list_%s", view)
	if tmpl, ok := s.templates[tmplName]; ok {
		tmpl.Execute(w, data)
	} else {
		s.templates["content_list_table"].Execute(w, data)
	}
}

func (s *Server) buildReferenceOptions(ctx context.Context, def *schema.ResourceDefinition) map[string][]*content.Record {
	options := make(map[string][]*content.Record)
	for fieldName, field := range def.Fields {
		if field.Type == schema.FieldTypeReference {
			targetResName := field.Resource
			if targetResName == "" {
				targetResName = def.Resource
			}
			targetDef := s.getDefinition(targetResName)
			if targetDef != nil {
				records, _, _ := s.svc.List(ctx, targetDef, content.ContentFilter{}, content.Pagination{Limit: 200})
				options[fieldName] = records
			}
		}
	}
	return options
}

func (s *Server) normalizeFormData(r *http.Request, def *schema.ResourceDefinition) map[string]interface{} {
	formData := make(map[string]interface{})
	for fieldName, field := range def.Fields {
		val := r.FormValue(fieldName)
		if val == "" {
			continue
		}
		if field.Type == schema.FieldTypeDateTime {
			// Convert datetime-local YYYY-MM-DDTHH:mm to RFC3339
			if t, err := time.ParseInLocation("2006-01-02T15:04", val, time.Local); err == nil {
				formData[fieldName] = t.Format(time.RFC3339)
				continue
			}
		}
		formData[fieldName] = val
	}
	return formData
}

func (s *Server) handleContentNew(w http.ResponseWriter, r *http.Request, def *schema.ResourceDefinition) {
	ctx := r.Context()
	refOptions := s.buildReferenceOptions(ctx, def)

	data := map[string]interface{}{
		"Title":            fmt.Sprintf("New %s", def.Resource),
		"CurrentResource":  def.Resource,
		"Resources":        s.definitions,
		"Resource":         def,
		"IsEdit":           false,
		"ReferenceOptions": refOptions,
	}
	s.templates["content_form"].Execute(w, data)
}

func (s *Server) handleContentCreate(w http.ResponseWriter, r *http.Request, def *schema.ResourceDefinition) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	formData := s.normalizeFormData(r, def)

	mctx := content.MutationContext{
		Actor:     "admin-user",
		ActorType: content.ActorTypeUser,
		RequestID: "req-admin-create",
	}

	_, err := s.svc.Create(ctx, def, formData, mctx)
	if err != nil {
		refOptions := s.buildReferenceOptions(ctx, def)
		data := map[string]interface{}{
			"Title":            fmt.Sprintf("New %s", def.Resource),
			"CurrentResource":  def.Resource,
			"Resources":        s.definitions,
			"Resource":         def,
			"IsEdit":           false,
			"ReferenceOptions": refOptions,
			"Error":            err.Error(),
		}
		s.templates["content_form"].Execute(w, data)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/resources/%s", def.Resource), http.StatusSeeOther)
}

func (s *Server) handleContentEdit(w http.ResponseWriter, r *http.Request, def *schema.ResourceDefinition, id string) {
	ctx := r.Context()
	record, err := s.svc.Get(ctx, def, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	refOptions := s.buildReferenceOptions(ctx, def)

	data := map[string]interface{}{
		"Title":            fmt.Sprintf("Edit %s", def.Resource),
		"CurrentResource":  def.Resource,
		"Resources":        s.definitions,
		"Resource":         def,
		"IsEdit":           true,
		"Record":           record,
		"ReferenceOptions": refOptions,
	}
	s.templates["content_form"].Execute(w, data)
}

func (s *Server) handleContentUpdate(w http.ResponseWriter, r *http.Request, def *schema.ResourceDefinition, id string) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	formData := s.normalizeFormData(r, def)

	mctx := content.MutationContext{
		Actor:     "admin-user",
		ActorType: content.ActorTypeUser,
		RequestID: "req-admin-update",
	}

	_, err := s.svc.Update(ctx, def, id, formData, mctx)
	if err != nil {
		record, _ := s.svc.Get(ctx, def, id)
		refOptions := s.buildReferenceOptions(ctx, def)
		data := map[string]interface{}{
			"Title":            fmt.Sprintf("Edit %s", def.Resource),
			"CurrentResource":  def.Resource,
			"Resources":        s.definitions,
			"Resource":         def,
			"IsEdit":           true,
			"Record":           record,
			"ReferenceOptions": refOptions,
			"Error":            err.Error(),
		}
		s.templates["content_form"].Execute(w, data)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/resources/%s", def.Resource), http.StatusSeeOther)
}

func (s *Server) handleContentDelete(w http.ResponseWriter, r *http.Request, def *schema.ResourceDefinition, id string) {
	ctx := r.Context()
	mctx := content.MutationContext{
		Actor:     "admin-user",
		ActorType: content.ActorTypeUser,
		RequestID: "req-admin-delete",
	}

	if err := s.svc.Delete(ctx, def, id, mctx); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleContentHistory(w http.ResponseWriter, r *http.Request, def *schema.ResourceDefinition, id string) {
	ctx := r.Context()
	record, err := s.svc.Get(ctx, def, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	logs, _ := s.auditor.List(ctx, def.Resource, id)
	revs, _ := s.reviser.List(ctx, def.Resource, id)

	data := map[string]interface{}{
		"Record":    record,
		"AuditLogs": logs,
		"Revisions": revs,
	}

	s.templates["content_history"].Execute(w, data)
}

func (s *Server) renderUpdatedRowOrCard(w http.ResponseWriter, r *http.Request, def *schema.ResourceDefinition, rec *content.Record) {
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}
