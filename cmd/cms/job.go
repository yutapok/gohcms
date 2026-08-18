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
	"github.com/yutapok/gohcms/pkg/job"
	"github.com/yutapok/gohcms/pkg/schema"
)

var (
	jobDBURL     string
	jobSchemaDir string
	jobDemo      bool
)

var jobCmd = &cobra.Command{
	Use:   "job <task-name>",
	Short: "Execute a one-shot background or batch job and exit",
	Long:  `Runs a registered one-shot task (e.g. 'validate', 'publish-scheduled') and terminates with an appropriate exit status code (0 for success, 1 for failure).`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskName := args[0]
		exitCode, err := executeJob(taskName, args[1:])
		if err != nil {
			return err
		}
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return nil
	},
}

func executeJob(taskName string, jobArgs []string) (int, error) {
	// 1. Resolve Schema Directory
	dir := jobSchemaDir
	if dir == "" {
		dir = os.Getenv("CMS_SCHEMA_DIR")
	}
	if dir == "" {
		dir = "./resources"
	}

	definitions, err := schema.LoadDirectory(dir)
	if err != nil {
		return 1, fmt.Errorf("failed to load schema directory '%s': %w", dir, err)
	}

	var svc *content.ContentService
	var dbSchema *introspection.DatabaseSchema

	if jobDemo {
		fileRepo := content.NewFileBackedContentRepository("./.gohcms_demo.json")
		memAudit := content.NewMemoryAuditRepository()
		memRev := content.NewMemoryRevisionRepository()
		memUOW := content.NewMemoryUnitOfWork(fileRepo, memAudit, memRev)
		svc = content.NewService(memUOW, fileRepo, memAudit, memRev)
		content.SeedDemoData(context.Background(), svc, definitions)
		dbSchema = introspection.BuildMockDatabaseSchema(definitions)
	} else {
		dbURL := jobDBURL
		if dbURL == "" {
			dbURL = os.Getenv("DATABASE_URL")
		}
		if dbURL == "" {
			return 1, fmt.Errorf("database URL is required via --db flag or DATABASE_URL env (or run with --demo for in-memory mode)")
		}

		db, err := sql.Open("pgx", dbURL)
		if err != nil {
			return 1, fmt.Errorf("failed to connect to database: %w", err)
		}
		defer db.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			return 1, fmt.Errorf("failed to ping database: %w", err)
		}

		inspector := postgres.NewInspectorWithDB(db)
		var tableNames []string
		for _, def := range definitions {
			tableNames = append(tableNames, def.Storage.Table)
		}

		dbSchema, err = inspector.ReadSchema(ctx, tableNames)
		if err != nil {
			return 1, fmt.Errorf("failed to introspect database schema: %w", err)
		}

		repo := postgres.NewContentRepository(db)
		auditRepo := postgres.NewAuditRepository(db)
		revRepo := postgres.NewRevisionRepository(db)
		uow := postgres.NewUnitOfWork(db)
		svc = content.NewService(uow, repo, auditRepo, revRepo)
	}

	// 2. Initialize Registry & Register Jobs
	registry := job.NewRegistry()
	registry.Register(job.NewValidateJob(definitions, dbSchema))
	registry.Register(job.NewPublishScheduledJob(svc, definitions))

	j, ok := registry.Get(taskName)
	if !ok {
		return 1, fmt.Errorf("unknown job '%s'. Available jobs: %v", taskName, registry.List())
	}

	return j.Run(context.Background(), jobArgs)
}

func init() {
	jobCmd.Flags().StringVarP(&jobDBURL, "db", "d", "", "PostgreSQL database connection URL (or DATABASE_URL env)")
	jobCmd.Flags().StringVar(&jobSchemaDir, "schema-dir", "", "Directory containing YAML Resource Definitions (default: ./resources or CMS_SCHEMA_DIR env)")
	jobCmd.Flags().BoolVar(&jobDemo, "demo", false, "Run job in demo in-memory mode without requiring a database")

	rootCmd.AddCommand(jobCmd)
}
