package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/spf13/cobra"
	"github.com/yutapok/gohcms/internal/adapter/postgres"
	"github.com/yutapok/gohcms/internal/admin"
	"github.com/yutapok/gohcms/pkg/auth"
	"github.com/yutapok/gohcms/pkg/content"
	"github.com/yutapok/gohcms/pkg/introspection"
	"github.com/yutapok/gohcms/pkg/media"
	"github.com/yutapok/gohcms/pkg/schema"
	"github.com/yutapok/gohcms/pkg/validator"
)

var (
	serveDBURL         string
	serveSchemaDir     string
	servePort          int
	serveDemo          bool
	serveRole          string
	serveAPIKey        string
	serveAdminUser     string
	serveAdminPassword string
	serveUploadDir     string
	serveHost          string
	serveAllowUnauth   bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the gohcms Admin UI and API server",
	Long:  `Starts the embedded Go + htmx Admin UI and Core Service HTTP server. Use --demo to run with in-memory mock data (no database required). Use --role to isolate endpoints (all, admin, api).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Resolve Schema Directory
		dir := serveSchemaDir
		if dir == "" {
			dir = os.Getenv("CMS_SCHEMA_DIR")
		}
		if dir == "" {
			dir = "./resources"
		}

		definitions, err := schema.LoadDirectory(dir)
		if err != nil {
			return fmt.Errorf("failed to load schema directory '%s': %w", dir, err)
		}

		// Resolve Role (all, admin, api)
		roleStr := serveRole
		if roleStr == "" {
			roleStr = os.Getenv("CMS_ROLE")
		}
		if roleStr == "" {
			roleStr = "all"
		}
		role := admin.Role(roleStr)
		if role != admin.RoleAll && role != admin.RoleAdmin && role != admin.RoleAPI {
			return fmt.Errorf("invalid role '%s': must be one of 'all', 'admin', 'api'", roleStr)
		}

		var svc *content.ContentService
		var auditRepo content.AuditRepository
		var revRepo content.RevisionRepository
		var mediaSvc *media.Service
		var authSvc *auth.Service
		var dbSchema *introspection.DatabaseSchema
		var valResult *validator.ValidationResult

		// Mode A: Demo In-Memory / File-Synced Mode (No Database Needed)
		if serveDemo {
			fmt.Println("🚀 Starting gohcms in Demo / File-Synced Mode (No database required)...")

			fileRepo := content.NewFileBackedContentRepository("./.gohcms_demo.json")
			memAudit := content.NewMemoryAuditRepository()
			memRev := content.NewMemoryRevisionRepository()
			memUOW := content.NewMemoryUnitOfWork(fileRepo, memAudit, memRev)
			svc = content.NewService(memUOW, fileRepo, memAudit, memRev)

			auditRepo = memAudit
			revRepo = memRev

			memStorage := media.NewMemoryStorage()
			memMediaRepo := media.NewMemoryMediaRepository()
			mediaSvc = media.NewService(memStorage, memMediaRepo, auditRepo)

			memKeyRepo := auth.NewMemoryAPIKeyRepository()
			authSvc = auth.NewService(memKeyRepo, auditRepo)
			dbSchema = introspection.BuildMockDatabaseSchema(definitions)

			v := validator.New()
			valResult = v.ValidateAll(definitions, dbSchema)

			// Seed sample demo content
			ctx := context.Background()
			mctx := content.MutationContext{Actor: "demo-admin", ActorType: content.ActorTypeUser, RequestID: "req-demo-init"}

			// Seed a sample API Key
			demoToken, _, _ := authSvc.CreateKey(ctx, "Default Demo Token", auth.PermissionReadWrite, mctx)
			fmt.Printf("✓ Seeded demo API Key: %s\n", demoToken)

			content.SeedDemoData(ctx, svc, definitions)
			fmt.Println("✓ Seeded sample demonstration content with 3-state lifecycle and scheduled publishing.")

		} else {
			// Mode B: PostgreSQL Mode
			dbURL := serveDBURL
			if dbURL == "" {
				dbURL = os.Getenv("DATABASE_URL")
			}
			if dbURL == "" {
				return fmt.Errorf("database URL is required via --db flag or DATABASE_URL env (or run with --demo for in-memory mode)")
			}

			db, err := sql.Open("pgx", dbURL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer db.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := db.PingContext(ctx); err != nil {
				return fmt.Errorf("failed to ping database: %w", err)
			}

			inspector := postgres.NewInspectorWithDB(db)
			var tableNames []string
			for _, def := range definitions {
				tableNames = append(tableNames, def.Storage.Table)
			}

			dbSchema, err = inspector.ReadSchema(ctx, tableNames)
			if err != nil {
				return fmt.Errorf("failed to introspect database schema: %w", err)
			}

			v := validator.New()
			valResult = v.ValidateAll(definitions, dbSchema)
			if !valResult.IsValid() {
				fmt.Println("⚠️  Warning: Schema validation detected discrepancies:")
				fmt.Print(valResult.FormatReport())
			} else {
				fmt.Println("✓ Schema validation successful. All Resource Definitions matched database.")
			}

			repo := postgres.NewContentRepository(db)
			auditRepo = postgres.NewAuditRepository(db)
			revRepo = postgres.NewRevisionRepository(db)
			uow := postgres.NewUnitOfWork(db)
			svc = content.NewService(uow, repo, auditRepo, revRepo)

			// Media Storage & Repository
			upDir := serveUploadDir
			if upDir == "" {
				upDir = os.Getenv("CMS_UPLOAD_DIR")
			}
			if upDir == "" {
				upDir = "./uploads"
			}
			localStorage, err := media.NewLocalStorage(upDir)
			if err != nil {
				return fmt.Errorf("failed to initialize media storage in '%s': %w", upDir, err)
			}
			mediaRepo := postgres.NewMediaRepository(db)
			mediaSvc = media.NewService(localStorage, mediaRepo, auditRepo)

			// Auth Repository & Service
			keyRepo := postgres.NewAPIKeyRepository(db)
			authSvc = auth.NewService(keyRepo, auditRepo)
		}

		// Resolve Auth Settings
		apiKey := serveAPIKey
		if apiKey == "" {
			apiKey = os.Getenv("CMS_API_KEY")
		}
		adminUser := serveAdminUser
		if adminUser == "" {
			adminUser = os.Getenv("CMS_ADMIN_USER")
		}
		adminPassword := serveAdminPassword
		if adminPassword == "" {
			adminPassword = os.Getenv("CMS_ADMIN_PASSWORD")
		}

		authCfg := auth.Config{
			StaticAPIKey:  apiKey,
			AdminUsername: adminUser,
			AdminPassword: adminPassword,
			Service:       authSvc,
		}

		// Initialize Admin UI Server with Full Services
		adminServer, err := admin.NewServerWithFull(svc, auditRepo, revRepo, mediaSvc, authSvc, definitions, dbSchema, valResult, authCfg)
		if err != nil {
			return fmt.Errorf("failed to initialize admin server: %w", err)
		}

		host := serveHost
		if host == "" {
			host = os.Getenv("CMS_HOST")
		}
		if host == "" {
			host = os.Getenv("CMS_BIND")
		}
		if host == "" {
			host = "127.0.0.1"
		}

		// Fail-Safe: Prevent accidental unauthenticated Admin UI exposure on 0.0.0.0 in non-demo mode
		if !serveDemo && role != admin.RoleAPI && (authCfg.AdminUsername == "" || authCfg.AdminPassword == "") {
			if host == "0.0.0.0" && !serveAllowUnauth {
				return fmt.Errorf("security error: Admin UI is bound to 0.0.0.0 without authentication credentials.\nSet CMS_ADMIN_USER and CMS_ADMIN_PASSWORD, or pass --allow-unauthenticated to override")
			}
		}

		addr := fmt.Sprintf("%s:%d", host, servePort)
		fmt.Printf("\n⚡ gohcms server is running at http://%s/ (Role: %s)\n", addr, role)
		fmt.Printf("   Managing %d resources from '%s'\n", len(definitions), dir)
		if role != admin.RoleAPI && authCfg.AdminUsername != "" {
			fmt.Printf("   🔒 Basic authentication enabled for /admin/ routes (user: %s)\n", authCfg.AdminUsername)
		} else if role != admin.RoleAPI {
			fmt.Println("   ⚠️  Admin UI is running in unauthenticated mode")
		}
		fmt.Println()

		server := &http.Server{
			Addr:              addr,
			Handler:           adminServer.HandlerForRole(role),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    1 << 20, // 1MB
		}

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}

		return nil
	},
}

func init() {
	serveCmd.Flags().StringVarP(&serveDBURL, "db", "d", "", "PostgreSQL database connection URL (or DATABASE_URL env)")
	serveCmd.Flags().StringVar(&serveSchemaDir, "schema-dir", "", "Directory containing YAML Resource Definitions (default: ./resources or CMS_SCHEMA_DIR env)")
	serveCmd.Flags().StringVarP(&serveHost, "host", "H", "", "Host IP address to bind server (default: 127.0.0.1, or CMS_HOST env)")
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 8080, "Port to run the Admin HTTP server on (default: 8080)")
	serveCmd.Flags().BoolVar(&serveDemo, "demo", false, "Run in demo in-memory mode without requiring a PostgreSQL database")
	serveCmd.Flags().StringVar(&serveRole, "role", "all", "Runtime role: 'all' (default: Admin UI + API), 'admin' (Admin UI only), 'api' (Headless REST API only)")
	serveCmd.Flags().StringVar(&serveAPIKey, "api-key", "", "Static fallback API Key for /api/ endpoints (or CMS_API_KEY env)")
	serveCmd.Flags().StringVar(&serveAdminUser, "admin-user", "", "Basic Auth username for Admin UI (or CMS_ADMIN_USER env)")
	serveCmd.Flags().StringVar(&serveAdminPassword, "admin-password", "", "Basic Auth password for Admin UI (or CMS_ADMIN_PASSWORD env)")
	serveCmd.Flags().StringVar(&serveUploadDir, "upload-dir", "./uploads", "Directory for local media file storage")
	serveCmd.Flags().BoolVar(&serveAllowUnauth, "allow-unauthenticated", false, "Allow binding to 0.0.0.0 without Admin credentials")

	rootCmd.AddCommand(serveCmd)
}
