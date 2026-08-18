package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yutapok/gohcms/pkg/openapi"
	"github.com/yutapok/gohcms/pkg/schema"
)

var (
	openapiSchemaDir string
	openapifile      string
	openapiOutput    string
	openapiFormat    string
)

var openapiCmd = &cobra.Command{
	Use:   "openapi",
	Short: "OpenAPI 3.1 specification tools",
}

var openapiExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export OpenAPI 3.1 specification from Resource Definitions",
	Long:  `Generates and exports an OpenAPI 3.1 specification document in JSON or YAML format from Resource Definitions.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var definitions []*schema.ResourceDefinition

		if openapifile != "" {
			def, err := schema.ParseFile(openapifile)
			if err != nil {
				return fmt.Errorf("failed to parse resource definition file '%s': %w", openapifile, err)
			}
			definitions = append(definitions, def)
		} else {
			dir := openapiSchemaDir
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

		gen := openapi.NewGenerator("gohcms Headless API", "1.0.0", "Auto-generated OpenAPI 3.1 specification for content platform")

		var outputData []byte
		var err error

		if openapiFormat == "yaml" || openapiFormat == "yml" {
			outputData, err = gen.ToYAML(definitions)
		} else {
			outputData, err = gen.ToJSON(definitions)
		}

		if err != nil {
			return fmt.Errorf("failed to serialize OpenAPI specification: %w", err)
		}

		if openapiOutput != "" {
			if err := os.WriteFile(openapiOutput, outputData, 0644); err != nil {
				return fmt.Errorf("failed to write OpenAPI output to '%s': %w", openapiOutput, err)
			}
			fmt.Printf("✓ OpenAPI 3.1 specification successfully exported to '%s'\n", openapiOutput)
		} else {
			fmt.Println(string(outputData))
		}

		return nil
	},
}

func init() {
	openapiExportCmd.Flags().StringVar(&openapiSchemaDir, "schema-dir", "", "Directory containing YAML Resource Definitions (default: ./resources or CMS_SCHEMA_DIR env)")
	openapiExportCmd.Flags().StringVarP(&openapifile, "file", "f", "", "Single YAML Resource Definition file to export")
	openapiExportCmd.Flags().StringVarP(&openapiOutput, "output", "o", "", "Output file path (default: stdout)")
	openapiExportCmd.Flags().StringVarP(&openapiFormat, "format", "", "json", "Output format: json or yaml (default: json)")

	openapiCmd.AddCommand(openapiExportCmd)
	rootCmd.AddCommand(openapiCmd)
}
