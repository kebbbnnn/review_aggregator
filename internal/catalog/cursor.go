package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Cursor tracks progress through the TMDB catalog.
type Cursor struct {
	LastPage                 int       `json:"last_page"`
	TotalPages               int       `json:"total_pages"`
	SortBy                   string    `json:"sort_by"`
	MoviesCatalogedThisCycle int       `json:"movies_cataloged_this_cycle"`
	Completed                bool      `json:"completed"`
	LastRun                  time.Time `json:"last_run"`
}

// DefaultCursor returns an initialized cursor starting from page 0.
func DefaultCursor() *Cursor {
	return &Cursor{
		LastPage:                 0,
		TotalPages:               0,
		SortBy:                   "primary_release_date.desc",
		MoviesCatalogedThisCycle: 0,
		Completed:                false,
		LastRun:                  time.Now().UTC(),
	}
}

// LoadCursor reads the cursor from the given file path.
// If the file does not exist, it returns a DefaultCursor without error.
// If the previous run completed, it resets to page 0 for a new cycle.
func LoadCursor(path string) (*Cursor, error) {
	if path == "" {
		return DefaultCursor(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultCursor(), nil
		}
		return nil, fmt.Errorf("reading cursor file %s: %w", path, err)
	}

	var cursor Cursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, fmt.Errorf("unmarshaling cursor from %s: %w", path, err)
	}

	if cursor.SortBy == "" {
		cursor.SortBy = "primary_release_date.desc"
	}

	// Reset for wrap-around cycle if previous run finished entire catalog
	if cursor.Completed {
		cursor.LastPage = 0
		cursor.MoviesCatalogedThisCycle = 0
		cursor.Completed = false
	}

	return &cursor, nil
}

// SaveCursor writes the cursor to disk in formatted JSON.
func SaveCursor(path string, cursor *Cursor) error {
	if path == "" || cursor == nil {
		return nil
	}

	cursor.LastRun = time.Now().UTC()
	data, err := json.MarshalIndent(cursor, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling cursor: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing cursor file %s: %w", path, err)
	}

	return nil
}
