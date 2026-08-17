package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const (
	MaxTMDBPages   = 500
	MinCatalogYear = 1890
)

// CurrentCatalogYear returns the current calendar year in UTC.
func CurrentCatalogYear() int {
	return time.Now().UTC().Year()
}

// Cursor tracks progress through the TMDB catalog across release years and sort strategies.
type Cursor struct {
	LastPage                 int       `json:"last_page"`
	TotalPages               int       `json:"total_pages"`
	SortBy                   string    `json:"sort_by"`
	CurrentYear              int       `json:"current_year,omitempty"`
	MoviesCatalogedThisCycle int       `json:"movies_cataloged_this_cycle"`
	Completed                bool      `json:"completed"`
	LastRun                  time.Time `json:"last_run"`
}

// DefaultCursor returns an initialized cursor starting at the current year and page 0.
func DefaultCursor() *Cursor {
	return &Cursor{
		LastPage:                 0,
		TotalPages:               0,
		SortBy:                   "popularity.desc",
		CurrentYear:              CurrentCatalogYear(),
		MoviesCatalogedThisCycle: 0,
		Completed:                false,
		LastRun:                  time.Now().UTC(),
	}
}

// LoadCursor reads the cursor from the given file path.
// If the file does not exist, it returns a DefaultCursor without error.
// If the previous run completed, it resets to CurrentCatalogYear() at page 0.
// If the cursor is from a legacy run (no CurrentYear set), it initializes CurrentYear to CurrentCatalogYear().
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
		cursor.SortBy = "popularity.desc"
	}

	// Legacy cursor migration: if no CurrentYear was recorded, initialize to CurrentCatalogYear()
	if cursor.CurrentYear == 0 {
		cursor.CurrentYear = CurrentCatalogYear()
		cursor.LastPage = 0
		cursor.Completed = false
	}

	// If previous run marked completion or reached year limit, reset for a new cycle
	if cursor.Completed || cursor.CurrentYear < MinCatalogYear {
		cursor.CurrentYear = CurrentCatalogYear()
		cursor.LastPage = 0
		cursor.MoviesCatalogedThisCycle = 0
		cursor.Completed = false
	}

	// If LastPage >= MaxTMDBPages, advance to previous year
	if cursor.LastPage >= MaxTMDBPages {
		cursor.CurrentYear--
		cursor.LastPage = 0
		cursor.TotalPages = 0
		if cursor.CurrentYear < MinCatalogYear {
			cursor.CurrentYear = CurrentCatalogYear()
			cursor.Completed = false
		}
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

