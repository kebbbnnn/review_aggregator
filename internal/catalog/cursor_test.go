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
	if c.SortBy != "primary_release_date.desc" {
		t.Errorf("expected SortBy 'primary_release_date.desc', got %s", c.SortBy)
	}
	if c.Completed {
		t.Errorf("expected Completed false")
	}
}

func TestNextSortStrategy(t *testing.T) {
	tests := []struct {
		current  string
		expected string
	}{
		{"primary_release_date.desc", "popularity.desc"},
		{"popularity.desc", "vote_count.desc"},
		{"vote_count.desc", "revenue.desc"},
		{"revenue.desc", "primary_release_date.desc"},
		{"unknown.sort", "primary_release_date.desc"},
	}

	for _, tt := range tests {
		res := NextSortStrategy(tt.current)
		if res != tt.expected {
			t.Errorf("NextSortStrategy(%q) expected %q, got %q", tt.current, tt.expected, res)
		}
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
}

func TestSaveAndLoadCursor_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "test_cursor.json")

	initial := &Cursor{
		LastPage:                 42,
		TotalPages:               500,
		SortBy:                   "primary_release_date.desc",
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

func TestLoadCursor_WrapAroundWhenCompletedOrMaxPages(t *testing.T) {
	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "completed_cursor.json")

	completedCursor := &Cursor{
		LastPage:                 500,
		TotalPages:               500,
		SortBy:                   "primary_release_date.desc",
		MoviesCatalogedThisCycle: 10000,
		Completed:                true,
		LastRun:                  time.Now().UTC(),
	}

	if err := SaveCursor(cursorPath, completedCursor); err != nil {
		t.Fatalf("SaveCursor failed: %v", err)
	}

	loaded, err := LoadCursor(cursorPath)
	if err != nil {
		t.Fatalf("LoadCursor failed: %v", err)
	}

	// Should reset for a new cycle with next sort strategy
	if loaded.LastPage != 0 {
		t.Errorf("expected LastPage 0 on wrap-around, got %d", loaded.LastPage)
	}
	if loaded.SortBy != "popularity.desc" {
		t.Errorf("expected SortBy 'popularity.desc' on wrap-around, got %s", loaded.SortBy)
	}
	if loaded.MoviesCatalogedThisCycle != 0 {
		t.Errorf("expected MoviesCatalogedThisCycle 0 on wrap-around, got %d", loaded.MoviesCatalogedThisCycle)
	}
	if loaded.Completed {
		t.Errorf("expected Completed false on wrap-around, got true")
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
