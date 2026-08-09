package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")

	content := `
# Test comment
TEST_PORT=9090
TEST_SECRET="my_secret_val"
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp .env: %v", err)
	}

	if err := loadDotEnv(envPath); err != nil {
		t.Fatalf("Failed to load dot env: %v", err)
	}

	if os.Getenv("TEST_PORT") != "9090" {
		t.Errorf("Expected TEST_PORT to be 9090, got %s", os.Getenv("TEST_PORT"))
	}
	if os.Getenv("TEST_SECRET") != "my_secret_val" {
		t.Errorf("Expected TEST_SECRET to be my_secret_val, got %s", os.Getenv("TEST_SECRET"))
	}
}
