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
	pages       map[int][]discovery.Movie
	totalPages  int
	calledPages []int
}

func (m *mockTMDB) FetchGenreMap(ctx context.Context) (map[int]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.genreMap, nil
}

func (m *mockTMDB) DiscoverAll(ctx context.Context, page int, sortBy string) ([]discovery.Movie, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calledPages = append(m.calledPages, page)

	movies, ok := m.pages[page]
	if !ok {
		return nil, m.totalPages, nil
	}
	return movies, m.totalPages, nil
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
	tmdb := &mockTMDB{
		genreMap:   map[int]string{28: "Action", 35: "Comedy"},
		totalPages: 3,
		pages: map[int][]discovery.Movie{
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
	}

	st := newMockCatalogStore()
	// Pre-populate movie 102 as existing
	st.existIDs["tmdb_102"] = true

	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursor.json")

	cursor := DefaultCursor()
	opts := CatalogOptions{
		MaxDuration:    10 * time.Second,
		MaxPages:       0,
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
	if !cursor.Completed {
		t.Errorf("expected cursor to be marked completed")
	}
	if cursor.LastPage != 3 {
		t.Errorf("expected cursor LastPage 3, got %d", cursor.LastPage)
	}

	// Verify genres mapped properly
	doc103 := st.movies["tmdb_103"]
	if doc103 == nil {
		t.Fatalf("expected tmdb_103 in store")
	}
	if len(doc103.Genres) != 2 || doc103.Genres[0] != "Action" || doc103.Genres[1] != "Comedy" {
		t.Errorf("expected genres [Action, Comedy], got %v", doc103.Genres)
	}

	// Verify cursor was written to disk and resets on load due to Completed: true
	diskCursor, err := LoadCursor(cursorPath)
	if err != nil {
		t.Fatalf("loading saved cursor failed: %v", err)
	}
	// Because Completed was true, LoadCursor resets LastPage to 0 for next cycle
	if diskCursor.LastPage != 0 {
		t.Errorf("expected loaded cursor LastPage 0 (wrap-around reset), got %d", diskCursor.LastPage)
	}
}

func TestCataloger_MaxPagesLimit(t *testing.T) {
	tmdb := &mockTMDB{
		genreMap:   map[int]string{28: "Action"},
		totalPages: 10,
		pages: map[int][]discovery.Movie{
			1: {{TMDBID: 101, Title: "Movie 101"}},
			2: {{TMDBID: 102, Title: "Movie 102"}},
			3: {{TMDBID: 103, Title: "Movie 103"}},
		},
	}

	st := newMockCatalogStore()
	cursor := &Cursor{LastPage: 0, TotalPages: 10, SortBy: "primary_release_date.desc"}
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
		genreMap:   map[int]string{28: "Action"},
		totalPages: 10,
		pages: map[int][]discovery.Movie{
			1: {{TMDBID: 101, Title: "Movie 101"}},
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
		genreMap:   map[int]string{28: "Action"},
		totalPages: 48000,
		pages: map[int][]discovery.Movie{
			500: {{TMDBID: 500, Title: "Movie 500"}},
		},
	}

	st := newMockCatalogStore()
	cursor := &Cursor{LastPage: 499, TotalPages: 0, SortBy: "primary_release_date.desc"}
	opts := CatalogOptions{
		MaxDuration:    10 * time.Second,
		MaxPages:       0,
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
	if !cursor.Completed {
		t.Errorf("expected cursor to be marked completed at page 500")
	}
	if cursor.LastPage != 500 {
		t.Errorf("expected LastPage 500, got %d", cursor.LastPage)
	}
	if cursor.TotalPages != 500 {
		t.Errorf("expected TotalPages capped to 500, got %d", cursor.TotalPages)
	}
}

func TestCataloger_ResumeAt500RotatesSortStrategy(t *testing.T) {
	// If cursor was at page 500 from previous run, next Run should rotate sort strategy to popularity.desc and resume at page 1
	tmdb := &mockTMDB{
		genreMap:   map[int]string{28: "Action"},
		totalPages: 10,
		pages: map[int][]discovery.Movie{
			1: {{TMDBID: 101, Title: "Movie 101"}},
		},
	}

	st := newMockCatalogStore()
	cursor := &Cursor{
		LastPage:                 500,
		TotalPages:               500,
		SortBy:                   "primary_release_date.desc",
		MoviesCatalogedThisCycle: 10000,
		Completed:                true,
	}
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
		t.Errorf("expected 1 page processed, got %d", result.PagesProcessed)
	}
	if cursor.SortBy != "popularity.desc" {
		t.Errorf("expected SortBy to rotate to 'popularity.desc', got %s", cursor.SortBy)
	}
	if cursor.LastPage != 1 {
		t.Errorf("expected LastPage 1, got %d", cursor.LastPage)
	}
}

