package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/spf13/cobra"
	"github.com/yutapok/gohcms/internal/adapter/postgres"
	"github.com/yutapok/gohcms/pkg/content"
	"github.com/yutapok/gohcms/pkg/introspection"
	"github.com/yutapok/gohcms/pkg/mcp"
	"github.com/yutapok/gohcms/pkg/schema"
)

var (
	mcpDBURL     string
	mcpSchemaDir string
	mcpDemo      bool
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the Stateless Model Context Protocol (MCP) server over stdio",
	Long:  `Runs the stateless JSON-RPC 2.0 MCP server over stdio for Cursor, Claude Desktop, and AI Agent integrations.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Resolve Schema Directory
		dir := mcpSchemaDir
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

		var svc *content.ContentService
		var auditRepo content.AuditRepository
		var dbSchema *introspection.DatabaseSchema

		if mcpDemo {
			fileRepo := content.NewFileBackedContentRepository("./.gohcms_demo.json")
			memAudit := content.NewMemoryAuditRepository()
			memRev := content.NewMemoryRevisionRepository()
			memUOW := content.NewMemoryUnitOfWork(fileRepo, memAudit, memRev)
			svc = content.NewService(memUOW, fileRepo, memAudit, memRev)
			content.SeedDemoData(context.Background(), svc, definitions)
			auditRepo = memAudit
			dbSchema = introspection.BuildMockDatabaseSchema(definitions)
		} else {
			dbURL := mcpDBURL
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

			repo := postgres.NewContentRepository(db)
			auditRepo = postgres.NewAuditRepository(db)
			revRepo := postgres.NewRevisionRepository(db)
			uow := postgres.NewUnitOfWork(db)
			svc = content.NewService(uow, repo, auditRepo, revRepo)
		}

		server := mcp.NewServer(svc, auditRepo, definitions, dbSchema)
		return server.Serve(os.Stdin, os.Stdout)
	},
}

func init() {
	mcpCmd.Flags().StringVarP(&mcpDBURL, "db", "d", "", "PostgreSQL database connection URL (or DATABASE_URL env)")
	mcpCmd.Flags().StringVar(&mcpSchemaDir, "schema-dir", "", "Directory containing YAML Resource Definitions (default: ./resources or CMS_SCHEMA_DIR env)")
	mcpCmd.Flags().BoolVar(&mcpDemo, "demo", false, "Run MCP server in demo in-memory mode without requiring a database")

	rootCmd.AddCommand(mcpCmd)
}
