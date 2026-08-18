package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yutapok/gohcms/pkg/scaffold"
)

var (
	initDir   string
	initForce bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new gohcms CMS project with starter files",
	Long:  `Creates starter Resource Definitions, database migrations, and environment configuration files.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		createdFiles, err := scaffold.Init(initDir, initForce)
		if err != nil {
			return err
		}

		fmt.Println("🎉 Successfully initialized gohcms project!")
		fmt.Println("\nCreated files:")
		for _, f := range createdFiles {
			fmt.Printf("  ✓ %s\n", f)
		}

		fmt.Println("\nNext Steps:")
		fmt.Println("  1. Run in Demo Mode (No DB needed):")
		fmt.Println("     cms serve --demo --schema-dir ./resources --port 8080")
		fmt.Println("\n  2. Or connect to PostgreSQL:")
		fmt.Println("     cp .env.example .env")
		fmt.Println("     psql $DATABASE_URL -f migrations/001_create_articles.sql")
		fmt.Println("     cms validate --db \"$DATABASE_URL\" --schema-dir ./resources")
		fmt.Println("     cms serve --db \"$DATABASE_URL\" --schema-dir ./resources --port 8080")

		return nil
	},
}

func init() {
	initCmd.Flags().StringVarP(&initDir, "dir", "d", ".", "Target directory to initialize (default: current directory)")
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite existing files if they exist")

	rootCmd.AddCommand(initCmd)
}
