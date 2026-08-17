package catalog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultCursor(t *testing.T) {
	c := DefaultCursor()
	if c.LastPage != 0 {
		t.Errorf("expected LastPage 0, got %d", c.LastPage)
	}
	if c.SortBy != "popularity.desc" {
		t.Errorf("expected SortBy 'popularity.desc', got %s", c.SortBy)
	}
	if c.CurrentYear != CurrentCatalogYear() {
		t.Errorf("expected CurrentYear %d, got %d", CurrentCatalogYear(), c.CurrentYear)
	}
	if c.Completed {
		t.Errorf("expected Completed false")
	}
}

func TestLoadCursor_NonExistentFile(t *testing.T) {
	c, err := LoadCursor("non_existent_cursor_file_12345.json")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if c.LastPage != 0 {
		t.Errorf("expected LastPage 0, got %d", c.LastPage)
	}
	if c.CurrentYear != CurrentCatalogYear() {
		t.Errorf("expected CurrentYear %d, got %d", CurrentCatalogYear(), c.CurrentYear)
	}
}

func TestSaveAndLoadCursor_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "test_cursor.json")

	initial := &Cursor{
		LastPage:                 42,
		TotalPages:               500,
		SortBy:                   "popularity.desc",
		CurrentYear:              2024,
		MoviesCatalogedThisCycle: 840,
		Completed:                false,
		LastRun:                  time.Now().UTC().Truncate(time.Second),
	}

	if err := SaveCursor(cursorPath, initial); err != nil {
		t.Fatalf("SaveCursor failed: %v", err)
	}

	loaded, err := LoadCursor(cursorPath)
	if err != nil {
		t.Fatalf("LoadCursor failed: %v", err)
	}

	if loaded.LastPage != 42 {
		t.Errorf("expected LastPage 42, got %d", loaded.LastPage)
	}
	if loaded.CurrentYear != 2024 {
		t.Errorf("expected CurrentYear 2024, got %d", loaded.CurrentYear)
	}
	if loaded.TotalPages != 500 {
		t.Errorf("expected TotalPages 500, got %d", loaded.TotalPages)
	}
	if loaded.MoviesCatalogedThisCycle != 840 {
		t.Errorf("expected MoviesCatalogedThisCycle 840, got %d", loaded.MoviesCatalogedThisCycle)
	}
	if loaded.Completed != false {
		t.Errorf("expected Completed false, got true")
	}
}

func TestLoadCursor_LegacyCursorMigration(t *testing.T) {
	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "legacy_cursor.json")

	// Legacy cursor from old system without current_year
	legacyData := []byte(`{
		"last_page": 500,
		"total_pages": 500,
		"sort_by": "revenue.desc",
		"movies_cataloged_this_cycle": 9999,
		"completed": true,
		"last_run": "2026-08-17T06:05:38Z"
	}`)
	if err := os.WriteFile(cursorPath, legacyData, 0644); err != nil {
		t.Fatalf("writing legacy cursor failed: %v", err)
	}

	loaded, err := LoadCursor(cursorPath)
	if err != nil {
		t.Fatalf("LoadCursor failed: %v", err)
	}

	// Should migrate to CurrentCatalogYear() at page 0
	if loaded.CurrentYear != CurrentCatalogYear() {
		t.Errorf("expected CurrentYear %d on legacy migration, got %d", CurrentCatalogYear(), loaded.CurrentYear)
	}
	if loaded.LastPage != 0 {
		t.Errorf("expected LastPage 0 on legacy migration, got %d", loaded.LastPage)
	}
	if loaded.Completed {
		t.Errorf("expected Completed false on legacy migration, got true")
	}
}

func TestLoadCursor_WrapAroundWhenCompletedOrMaxPages(t *testing.T) {
	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "completed_cursor.json")

	completedCursor := &Cursor{
		LastPage:                 500,
		TotalPages:               500,
		SortBy:                   "popularity.desc",
		CurrentYear:              2020,
		MoviesCatalogedThisCycle: 10000,
		Completed:                false,
		LastRun:                  time.Now().UTC(),
	}

	if err := SaveCursor(cursorPath, completedCursor); err != nil {
		t.Fatalf("SaveCursor failed: %v", err)
	}

	loaded, err := LoadCursor(cursorPath)
	if err != nil {
		t.Fatalf("LoadCursor failed: %v", err)
	}

	// Because LastPage was 500, LoadCursor advances to previous year (2019) at page 0
	if loaded.CurrentYear != 2019 {
		t.Errorf("expected CurrentYear 2019 on page limit advance, got %d", loaded.CurrentYear)
	}
	if loaded.LastPage != 0 {
		t.Errorf("expected LastPage 0 on page limit advance, got %d", loaded.LastPage)
	}
}

func TestLoadCursor_EmptyPath(t *testing.T) {
	c, err := LoadCursor("")
	if err != nil {
		t.Fatalf("expected no error for empty path, got %v", err)
	}
	if c.LastPage != 0 {
		t.Errorf("expected default cursor LastPage 0, got %d", c.LastPage)
	}
}

func TestSaveCursor_EmptyPathOrNil(t *testing.T) {
	if err := SaveCursor("", nil); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if err := SaveCursor("", DefaultCursor()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestLoadCursor_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	invalidPath := filepath.Join(tmpDir, "invalid.json")
	_ = os.WriteFile(invalidPath, []byte("invalid json {{{"), 0644)

	_, err := LoadCursor(invalidPath)
	if err == nil {
		t.Fatalf("expected error for invalid json, got nil")
	}
}

