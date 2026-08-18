package scaffold_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yutapok/gohcms/pkg/scaffold"
)

func TestInit_Success(t *testing.T) {
	tempDir := t.TempDir()

	created, err := scaffold.Init(tempDir, false)
	if err != nil {
		t.Fatalf("unexpected error during init: %v", err)
	}

	if len(created) != 4 {
		t.Fatalf("expected 4 created files, got %d", len(created))
	}

	expectedFiles := []string{
		filepath.Join(tempDir, "resources/article.yaml"),
		filepath.Join(tempDir, "migrations/001_create_articles.sql"),
		filepath.Join(tempDir, ".env.example"),
		filepath.Join(tempDir, "README.md"),
	}

	for _, ef := range expectedFiles {
		if _, err := os.Stat(ef); os.IsNotExist(err) {
			t.Errorf("expected file '%s' to exist", ef)
		}
	}
}

func TestInit_FileExistsError(t *testing.T) {
	tempDir := t.TempDir()

	// Pre-create README.md
	readmePath := filepath.Join(tempDir, "README.md")
	os.WriteFile(readmePath, []byte("existing"), 0644)

	// Init without force should fail
	_, err := scaffold.Init(tempDir, false)
	if err == nil {
		t.Fatal("expected error for existing file, but got nil")
	}

	// Init with force should succeed
	created, err := scaffold.Init(tempDir, true)
	if err != nil {
		t.Fatalf("expected success with force=true, got error: %v", err)
	}
	if len(created) != 4 {
		t.Errorf("expected 4 files created, got %d", len(created))
	}
}
