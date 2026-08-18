package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/yutapok/gohcms/internal/adapter/postgres"
	"github.com/yutapok/gohcms/pkg/schema"
	"github.com/yutapok/gohcms/pkg/validator"
)

var (
	validateDBURL     string
	validateSchemaDir string
	validateFile      string
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate Resource Definitions against the PostgreSQL database schema",
	Long:  `Validates that the database schema strictly matches the Resource Definitions (tables, columns, types, lifecycle).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Resolve database URL
		dbURL := validateDBURL
		if dbURL == "" {
			dbURL = os.Getenv("DATABASE_URL")
		}
		if dbURL == "" {
			return fmt.Errorf("database URL is required via --db flag or DATABASE_URL environment variable")
		}

		// 2. Load resource definitions
		var definitions []*schema.ResourceDefinition
		if validateFile != "" {
			def, err := schema.ParseFile(validateFile)
			if err != nil {
				return fmt.Errorf("failed to load resource file '%s': %w", validateFile, err)
			}
			definitions = append(definitions, def)
		} else {
			dir := validateSchemaDir
			if dir == "" {
				dir = os.Getenv("CMS_SCHEMA_DIR")
			}
			if dir == "" {
				dir = "./resources"
			}
			defs, err := schema.LoadDirectory(dir)
			if err != nil {
				return fmt.Errorf("failed to load schema directory '%s': %w", dir, err)
			}
			definitions = defs
		}

		// 3. Connect to database and introspect schema
		inspector, err := postgres.NewInspector(dbURL)
		if err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}
		defer inspector.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var tableNames []string
		for _, def := range definitions {
			tableNames = append(tableNames, def.Storage.Table)
		}

		dbSchema, err := inspector.ReadSchema(ctx, tableNames)
		if err != nil {
			return fmt.Errorf("failed to introspect database schema: %w", err)
		}

		// 4. Validate definitions against introspected schema
		v := validator.New()
		result := v.ValidateAll(definitions, dbSchema)

		// 5. Output report
		report := result.FormatReport()
		if report != "" {
			fmt.Print(report)
		}

		if !result.IsValid() {
			os.Exit(1)
		}

		return nil
	},
}

func init() {
	validateCmd.Flags().StringVarP(&validateDBURL, "db", "d", "", "PostgreSQL database connection URL (or DATABASE_URL env)")
	validateCmd.Flags().StringVar(&validateSchemaDir, "schema-dir", "", "Directory containing YAML Resource Definitions (default: ./resources or CMS_SCHEMA_DIR env)")
	validateCmd.Flags().StringVarP(&validateFile, "file", "f", "", "Single YAML Resource Definition file to validate")

	rootCmd.AddCommand(validateCmd)
}
