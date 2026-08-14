package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"review_aggregator/internal/catalog"
	"review_aggregator/internal/config"
	"review_aggregator/internal/discovery"
	"review_aggregator/internal/store"
)

func main() {
	log.Println("[INFO] Starting TMDB Catalog Sync Pipeline...")

	cursorPath := flag.String("cursor", "cursor.json", "Path to cursor JSON file")
	maxPages := flag.Int("max-pages", 0, "Max pages to process (0 for unlimited)")
	maxDurationStr := flag.String("max-duration", "4h45m", "Max duration to run before graceful exit")
	flag.Parse()

	maxDuration, err := time.ParseDuration(*maxDurationStr)
	if err != nil {
		maxDuration = 4*time.Hour + 45*time.Minute
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Loading config failed: %v", err)
	}

	if cfg.TMDBAPIKey == "" {
		log.Fatalf("[FATAL] TMDB_API_KEY environment variable is required")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	st, err := store.NewD1Store(cfg.CFAccountID, cfg.CFD1DatabaseID, cfg.CFAPIToken)
	if err != nil {
		log.Fatalf("[FATAL] D1 store init failed: %v", err)
	}
	defer st.Close()

	tmdb := discovery.NewTMDBClient(cfg.TMDBAPIKey)

	cursor, err := catalog.LoadCursor(*cursorPath)
	if err != nil {
		log.Fatalf("[FATAL] Failed to load cursor from %s: %v", *cursorPath, err)
	}

	opts := catalog.CatalogOptions{
		MaxDuration:    maxDuration,
		MaxPages:       *maxPages,
		RateLimitDelay: 35 * time.Millisecond,
		CursorPath:     *cursorPath,
	}

	cataloger := catalog.NewCataloger(tmdb, st, cursor, opts)

	result, err := cataloger.Run(ctx)
	if err != nil {
		log.Fatalf("[FATAL] Catalog sync failed: %v", err)
	}

	log.Printf("[INFO] Catalog sync run complete: %d pages, %d discovered, %d saved, %d skipped, %d errors. Cursor now at page %d (completed=%v)",
		result.PagesProcessed, result.MoviesDiscovered, result.MoviesSaved, result.MoviesSkipped, len(result.Errors),
		result.Cursor.LastPage, result.Cursor.Completed)
}
