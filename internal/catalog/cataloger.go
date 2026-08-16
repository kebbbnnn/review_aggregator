package catalog

import (
	"context"
	"fmt"
	"log"
	"time"

	"review_aggregator/internal/discovery"
	"review_aggregator/internal/store"
)

// TMDBDiscoverer defines the TMDB operations required by the cataloger.
type TMDBDiscoverer interface {
	FetchGenreMap(ctx context.Context) (map[int]string, error)
	DiscoverAll(ctx context.Context, page int, sortBy string) ([]discovery.Movie, int, error)
}

// CatalogOptions allows customizing runtime behavior.
type CatalogOptions struct {
	MaxDuration    time.Duration
	MaxPages       int
	RateLimitDelay time.Duration
	CursorPath     string
}

// DefaultOptions provides standard production settings.
func DefaultOptions() CatalogOptions {
	return CatalogOptions{
		MaxDuration:    4*time.Hour + 45*time.Minute,
		MaxPages:       0, // 0 = unlimited
		RateLimitDelay: 35 * time.Millisecond,
		CursorPath:     "cursor.json",
	}
}

// CatalogResult records metrics from a catalog sync run.
type CatalogResult struct {
	PagesProcessed   int      `json:"pages_processed"`
	MoviesDiscovered int      `json:"movies_discovered"`
	MoviesSaved      int      `json:"movies_saved"`
	MoviesSkipped    int      `json:"movies_skipped"`
	Cursor           *Cursor  `json:"cursor"`
	Errors           []string `json:"errors,omitempty"`
}

// Cataloger orchestrates TMDB catalog ingestion into Cloudflare D1.
type Cataloger struct {
	tmdb    TMDBDiscoverer
	store   store.Store
	cursor  *Cursor
	options CatalogOptions
}

// NewCataloger creates a new Cataloger.
func NewCataloger(tmdb TMDBDiscoverer, store store.Store, cursor *Cursor, opts CatalogOptions) *Cataloger {
	if cursor == nil {
		cursor = DefaultCursor()
	}
	if opts.RateLimitDelay <= 0 {
		opts.RateLimitDelay = 35 * time.Millisecond
	}
	if opts.MaxDuration <= 0 {
		opts.MaxDuration = 4*time.Hour + 45*time.Minute
	}
	return &Cataloger{
		tmdb:    tmdb,
		store:   store,
		cursor:  cursor,
		options: opts,
	}
}

// Run executes the catalog synchronization loop.
func (c *Cataloger) Run(ctx context.Context) (*CatalogResult, error) {
	log.Printf("[CATALOG] Starting catalog sync (Resuming from page %d, sort: %s)...", c.cursor.LastPage, c.cursor.SortBy)
	startTime := time.Now()

	result := &CatalogResult{
		Cursor: c.cursor,
	}

	genreMap, err := c.tmdb.FetchGenreMap(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching genre map: %w", err)
	}
	log.Printf("[CATALOG] Loaded %d genre definitions from TMDB", len(genreMap))

	// Ensure cursor is initialized and wrapped if already completed or reached TMDB page limit
	if c.cursor.Completed || c.cursor.LastPage >= MaxTMDBPages {
		c.cursor.SortBy = NextSortStrategy(c.cursor.SortBy)
		c.cursor.LastPage = 0
		c.cursor.MoviesCatalogedThisCycle = 0
		c.cursor.Completed = false
	}

	startPage := c.cursor.LastPage + 1
	currentPage := startPage

	for {
		// 1. Check Context Cancellation
		if ctx.Err() != nil {
			log.Println("[CATALOG] Context cancelled, stopping sync.")
			break
		}

		// 2. Check Max Duration Limit
		if time.Since(startTime) >= c.options.MaxDuration {
			log.Printf("[CATALOG] Max duration (%v) reached, gracefully stopping sync.", c.options.MaxDuration)
			break
		}

		// 3. Check Max Pages Limit
		if c.options.MaxPages > 0 && result.PagesProcessed >= c.options.MaxPages {
			log.Printf("[CATALOG] Max pages limit (%d) reached, stopping sync.", c.options.MaxPages)
			break
		}

		// 4. Check TMDB hard ceiling (500 pages per discover query)
		if currentPage > MaxTMDBPages {
			log.Printf("[CATALOG] Reached TMDB max discover page limit (%d). Marking cycle completed.", MaxTMDBPages)
			c.cursor.Completed = true
			break
		}

		// 5. Fetch TMDB Page
		movies, totalPages, err := c.tmdb.DiscoverAll(ctx, currentPage, c.cursor.SortBy)
		if err != nil {
			errMsg := fmt.Sprintf("Error fetching page %d: %v", currentPage, err)
			log.Println("[ERROR]", errMsg)
			result.Errors = append(result.Errors, errMsg)
			// Break to allow saving cursor at last known successful state
			break
		}

		effectiveTotalPages := totalPages
		if effectiveTotalPages > MaxTMDBPages {
			effectiveTotalPages = MaxTMDBPages
		}
		c.cursor.TotalPages = effectiveTotalPages

		if len(movies) == 0 || (effectiveTotalPages > 0 && currentPage > effectiveTotalPages) {
			log.Printf("[CATALOG] Reached end of catalog at page %d (totalPages=%d, effective=%d). Marking completed.", currentPage, totalPages, effectiveTotalPages)
			c.cursor.Completed = true
			break
		}

		result.MoviesDiscovered += len(movies)

		// 6. Filter & Build Movie Documents
		var docsToSave []*store.MovieDocument
		for _, m := range movies {
			docID := discovery.FormatTMDBID(m.TMDBID)

			// Lightweight existence check in D1
			exists, err := c.store.MovieExists(ctx, docID)
			if err != nil {
				log.Printf("[WARN] Error checking existence for movie %s: %v", docID, err)
			}
			if exists {
				result.MoviesSkipped++
				continue
			}

			// Map genre IDs to names
			var genres []string
			for _, gid := range m.GenreIDs {
				if gName, ok := genreMap[gid]; ok && gName != "" {
					genres = append(genres, gName)
				}
			}

			pop := m.Popularity
			doc := &store.MovieDocument{
				ID:          docID,
				TMDBID:      m.TMDBID,
				IMDbID:      m.IMDbID,
				Title:       m.Title,
				ReleaseDate: m.ReleaseDate,
				PosterURL:   m.PosterURL,
				Overview:    m.Overview,
				Genres:      genres,
				Popularity:  &pop,
				LastUpdated: time.Now().UTC(),
			}

			docsToSave = append(docsToSave, doc)
		}

		// 7. Batch Save to D1
		if len(docsToSave) > 0 {
			if err := c.store.SaveMovieBatch(ctx, docsToSave); err != nil {
				errMsg := fmt.Sprintf("Error saving batch for page %d: %v", currentPage, err)
				log.Println("[ERROR]", errMsg)
				result.Errors = append(result.Errors, errMsg)
				break
			}
			result.MoviesSaved += len(docsToSave)
			c.cursor.MoviesCatalogedThisCycle += len(docsToSave)
		}

		result.PagesProcessed++
		c.cursor.LastPage = currentPage

		// Check if this was the last page (either end of TMDB pages or reached TMDB 500-page limit)
		if (effectiveTotalPages > 0 && currentPage >= effectiveTotalPages) || currentPage >= MaxTMDBPages {
			log.Printf("[CATALOG] Processed final page %d of %d (sort: %s). Cycle complete!", currentPage, effectiveTotalPages, c.cursor.SortBy)
			c.cursor.Completed = true
			if c.options.CursorPath != "" {
				_ = SaveCursor(c.options.CursorPath, c.cursor)
			}
			break
		}

		// Progress Log every 10 pages
		if result.PagesProcessed%10 == 0 {
			log.Printf("[CATALOG] Progress: page %d/%d (Processed: %d, Saved: %d, Skipped: %d, Elapsed: %v)",
				currentPage, totalPages, result.PagesProcessed, result.MoviesSaved, result.MoviesSkipped, time.Since(startTime).Round(time.Second))
		}

		// Auto-save cursor after every page
		if c.options.CursorPath != "" {
			_ = SaveCursor(c.options.CursorPath, c.cursor)
		}

		currentPage++

		// 7. Rate Limiting Delay
		if c.options.RateLimitDelay > 0 {
			select {
			case <-ctx.Done():
				break
			case <-time.After(c.options.RateLimitDelay):
			}
		}
	}

	// Final cursor save
	if c.options.CursorPath != "" {
		_ = SaveCursor(c.options.CursorPath, c.cursor)
	}

	log.Printf("[CATALOG] Sync finished: %d pages processed, %d movies discovered, %d saved, %d skipped, %d errors (Elapsed: %v)",
		result.PagesProcessed, result.MoviesDiscovered, result.MoviesSaved, result.MoviesSkipped, len(result.Errors), time.Since(startTime).Round(time.Second))

	return result, nil
}
