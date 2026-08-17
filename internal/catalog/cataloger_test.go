package catalog

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"review_aggregator/internal/discovery"
	"review_aggregator/internal/store"
)

type mockTMDB struct {
	mu          sync.Mutex
	genreMap    map[int]string
	pages       map[int]map[int][]discovery.Movie // year -> page -> movies
	totalPages  map[int]int                       // year -> totalPages
	calledPages []int
	calledYears []int
}

func (m *mockTMDB) FetchGenreMap(ctx context.Context) (map[int]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.genreMap, nil
}

func (m *mockTMDB) DiscoverCatalog(ctx context.Context, page int, sortBy string, year int) ([]discovery.Movie, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calledPages = append(m.calledPages, page)
	m.calledYears = append(m.calledYears, year)

	total := 1
	if m.totalPages != nil {
		if t, ok := m.totalPages[year]; ok {
			total = t
		}
	}

	if m.pages != nil {
		if yearPages, ok := m.pages[year]; ok {
			if movies, ok := yearPages[page]; ok {
				return movies, total, nil
			}
		}
	}

	return nil, total, nil
}

type mockCatalogStore struct {
	mu       sync.Mutex
	movies   map[string]*store.MovieDocument
	batches  [][]*store.MovieDocument
	existIDs map[string]bool
}

func newMockCatalogStore() *mockCatalogStore {
	return &mockCatalogStore{
		movies:   make(map[string]*store.MovieDocument),
		existIDs: make(map[string]bool),
	}
}

func (s *mockCatalogStore) SaveMovie(ctx context.Context, doc *store.MovieDocument) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.movies[doc.ID] = doc
	s.existIDs[doc.ID] = true
	return nil
}

func (s *mockCatalogStore) SaveMovieBatch(ctx context.Context, docs []*store.MovieDocument) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batches = append(s.batches, docs)
	for _, doc := range docs {
		s.movies[doc.ID] = doc
		s.existIDs[doc.ID] = true
	}
	return nil
}

func (s *mockCatalogStore) GetMovie(ctx context.Context, id string) (*store.MovieDocument, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.movies[id]
	return doc, ok, nil
}

func (s *mockCatalogStore) MovieExists(ctx context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.existIDs[id], nil
}

func (s *mockCatalogStore) Close() error { return nil }

func TestCataloger_MultiPage_Success(t *testing.T) {
	currentYear := 2026
	tmdb := &mockTMDB{
		genreMap: map[int]string{28: "Action", 35: "Comedy"},
		totalPages: map[int]int{
			2026: 3,
		},
		pages: map[int]map[int][]discovery.Movie{
			2026: {
				1: {
					{TMDBID: 101, Title: "Movie 101", GenreIDs: []int{28}, ReleaseDate: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
					{TMDBID: 102, Title: "Movie 102", GenreIDs: []int{35}, ReleaseDate: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)},
				},
				2: {
					{TMDBID: 103, Title: "Movie 103", GenreIDs: []int{28, 35}, ReleaseDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
				},
				3: {
					{TMDBID: 104, Title: "Movie 104", GenreIDs: []int{28}, ReleaseDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
				},
			},
		},
	}

	st := newMockCatalogStore()
	// Pre-populate movie 102 as existing
	st.existIDs["tmdb_102"] = true

	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursor.json")

	cursor := &Cursor{
		LastPage:    0,
		TotalPages:  0,
		SortBy:      "popularity.desc",
		CurrentYear: currentYear,
	}
	opts := CatalogOptions{
		MaxDuration:    10 * time.Second,
		MaxPages:       3,
		RateLimitDelay: 1 * time.Millisecond,
		CursorPath:     cursorPath,
	}

	cataloger := NewCataloger(tmdb, st, cursor, opts)

	result, err := cataloger.Run(context.Background())
	if err != nil {
		t.Fatalf("cataloger.Run failed: %v", err)
	}

	if result.PagesProcessed != 3 {
		t.Errorf("expected 3 pages processed, got %d", result.PagesProcessed)
	}
	if result.MoviesDiscovered != 4 {
		t.Errorf("expected 4 movies discovered, got %d", result.MoviesDiscovered)
	}
	if result.MoviesSkipped != 1 {
		t.Errorf("expected 1 movie skipped (tmdb_102), got %d", result.MoviesSkipped)
	}
	if result.MoviesSaved != 3 {
		t.Errorf("expected 3 movies saved, got %d", result.MoviesSaved)
	}
	// After finishing page 3 of 3 for 2026, it advances to 2025
	if cursor.CurrentYear != 2025 {
		t.Errorf("expected cursor CurrentYear 2025, got %d", cursor.CurrentYear)
	}
	if cursor.LastPage != 0 {
		t.Errorf("expected cursor LastPage 0, got %d", cursor.LastPage)
	}

	// Verify genres mapped properly
	doc103 := st.movies["tmdb_103"]
	if doc103 == nil {
		t.Fatalf("expected tmdb_103 in store")
	}
	if len(doc103.Genres) != 2 || doc103.Genres[0] != "Action" || doc103.Genres[1] != "Comedy" {
		t.Errorf("expected genres [Action, Comedy], got %v", doc103.Genres)
	}
}

func TestCataloger_YearTransition(t *testing.T) {
	// Tests that when year 2024 finishes page 1 (only 1 page), it automatically advances to year 2023 in the same run
	tmdb := &mockTMDB{
		genreMap: map[int]string{28: "Action"},
		totalPages: map[int]int{
			2024: 1,
			2023: 2,
		},
		pages: map[int]map[int][]discovery.Movie{
			2024: {
				1: {{TMDBID: 2401, Title: "2024 Movie"}},
			},
			2023: {
				1: {{TMDBID: 2301, Title: "2023 Movie P1"}},
				2: {{TMDBID: 2302, Title: "2023 Movie P2"}},
			},
		},
	}

	st := newMockCatalogStore()
	cursor := &Cursor{LastPage: 0, TotalPages: 0, SortBy: "popularity.desc", CurrentYear: 2024}
	opts := CatalogOptions{
		MaxDuration:    10 * time.Second,
		MaxPages:       2, // Process 2 pages total (1 from 2024, 1 from 2023)
		RateLimitDelay: 1 * time.Millisecond,
	}

	cataloger := NewCataloger(tmdb, st, cursor, opts)
	result, err := cataloger.Run(context.Background())
	if err != nil {
		t.Fatalf("cataloger.Run failed: %v", err)
	}

	if result.PagesProcessed != 2 {
		t.Errorf("expected 2 pages processed across years, got %d", result.PagesProcessed)
	}
	if result.MoviesSaved != 2 {
		t.Errorf("expected 2 movies saved, got %d", result.MoviesSaved)
	}
	if cursor.CurrentYear != 2023 {
		t.Errorf("expected cursor CurrentYear 2023, got %d", cursor.CurrentYear)
	}
	if cursor.LastPage != 1 {
		t.Errorf("expected cursor LastPage 1 (in year 2023), got %d", cursor.LastPage)
	}
}

func TestCataloger_MaxPagesLimit(t *testing.T) {
	tmdb := &mockTMDB{
		genreMap: map[int]string{28: "Action"},
		totalPages: map[int]int{
			2026: 10,
		},
		pages: map[int]map[int][]discovery.Movie{
			2026: {
				1: {{TMDBID: 101, Title: "Movie 101"}},
				2: {{TMDBID: 102, Title: "Movie 102"}},
				3: {{TMDBID: 103, Title: "Movie 103"}},
			},
		},
	}

	st := newMockCatalogStore()
	cursor := &Cursor{LastPage: 0, TotalPages: 10, SortBy: "popularity.desc", CurrentYear: 2026}
	opts := CatalogOptions{
		MaxDuration:    10 * time.Second,
		MaxPages:       2, // Limit to 2 pages
		RateLimitDelay: 1 * time.Millisecond,
	}

	cataloger := NewCataloger(tmdb, st, cursor, opts)

	result, err := cataloger.Run(context.Background())
	if err != nil {
		t.Fatalf("cataloger.Run failed: %v", err)
	}

	if result.PagesProcessed != 2 {
		t.Errorf("expected 2 pages processed, got %d", result.PagesProcessed)
	}
	if cursor.LastPage != 2 {
		t.Errorf("expected cursor LastPage 2, got %d", cursor.LastPage)
	}
	if cursor.Completed {
		t.Errorf("expected cursor Completed false")
	}
}

func TestCataloger_ContextCancellation(t *testing.T) {
	tmdb := &mockTMDB{
		genreMap: map[int]string{28: "Action"},
		totalPages: map[int]int{
			2026: 10,
		},
		pages: map[int]map[int][]discovery.Movie{
			2026: {
				1: {{TMDBID: 101, Title: "Movie 101"}},
			},
		},
	}

	st := newMockCatalogStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cataloger := NewCataloger(tmdb, st, DefaultCursor(), DefaultOptions())
	result, err := cataloger.Run(ctx)
	if err != nil {
		t.Fatalf("expected no error on cancelled context, got %v", err)
	}
	if result.PagesProcessed != 0 {
		t.Errorf("expected 0 pages processed, got %d", result.PagesProcessed)
	}
}

func TestCataloger_TMDB500PageLimit(t *testing.T) {
	// TMDB returns 48,000 totalPages, but API only supports up to page 500
	tmdb := &mockTMDB{
		genreMap: map[int]string{28: "Action"},
		totalPages: map[int]int{
			2024: 48000,
		},
		pages: map[int]map[int][]discovery.Movie{
			2024: {
				500: {{TMDBID: 500, Title: "Movie 500"}},
			},
		},
	}

	st := newMockCatalogStore()
	cursor := &Cursor{LastPage: 499, TotalPages: 0, SortBy: "popularity.desc", CurrentYear: 2024}
	opts := CatalogOptions{
		MaxDuration:    10 * time.Second,
		MaxPages:       1,
		RateLimitDelay: 1 * time.Millisecond,
	}

	cataloger := NewCataloger(tmdb, st, cursor, opts)
	result, err := cataloger.Run(context.Background())
	if err != nil {
		t.Fatalf("cataloger.Run failed: %v", err)
	}

	if result.PagesProcessed != 1 {
		t.Errorf("expected 1 page processed (page 500), got %d", result.PagesProcessed)
	}
	// Reached 500 in 2024, should advance to 2023
	if cursor.CurrentYear != 2023 {
		t.Errorf("expected CurrentYear 2023 on 500-page limit, got %d", cursor.CurrentYear)
	}
	if cursor.LastPage != 0 {
		t.Errorf("expected LastPage 0 on year advance, got %d", cursor.LastPage)
	}
}


