package main

import (
	"context"
	"flag"
	"log"
	"time"

	"review_aggregator/internal/collector"
	"review_aggregator/internal/config"
	"review_aggregator/internal/discovery"
	"review_aggregator/internal/llm"
	"review_aggregator/internal/pipeline"
	"review_aggregator/internal/processor"
	"review_aggregator/internal/store"
)

func main() {
	movieID := flag.String("movie-id", "", "Specific movie ID to process on-demand (e.g. tmdb_12345)")
	flag.Parse()

	log.Println("[INFO] Starting Movie Review Aggregator Pipeline (Dual DB Architecture)...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Loading config failed: %v", err)
	}

	ctx := context.Background()

	metaStore, err := store.NewMetadataStore(cfg.CFAccountID, cfg.CFD1DatabaseID, cfg.CFAPIToken)
	if err != nil {
		log.Fatalf("[FATAL] DB1 metadata store init failed: %v", err)
	}
	defer metaStore.Close()

	summaryStore, err := store.NewSummaryStore(cfg.CFSummaryAccountID, cfg.CFSummaryDatabaseID, cfg.CFSummaryAPIToken)
	if err != nil {
		log.Fatalf("[FATAL] DB2 summary store init failed: %v", err)
	}
	defer summaryStore.Close()

	tmdb := discovery.NewTMDBClient(cfg.TMDBAPIKey)
	omdb := discovery.NewOMDBClient(cfg.OMDBAPIKey)

	releasedAfter := time.Now().AddDate(0, -cfg.RecentMonths, 0)
	discoverer := discovery.NewPopularRecentDiscoverer(tmdb, cfg.MinPopularity, releasedAfter)

	collectors := []collector.Collector{
		collector.NewRedditCollector(cfg.RedditClientID, cfg.RedditClientSecret, cfg.RedditUserAgent),
		collector.NewLetterboxdCollector(cfg.RedditUserAgent),
	}

	proc := processor.NewProcessor(3, 30)

	var llmClient llm.Client
	if cfg.LLMAPIKey != "" {
		llmClient = llm.NewClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
	}

	orchestrator := pipeline.NewOrchestrator(discoverer, omdb, collectors, proc, llmClient, metaStore, summaryStore)

	var result *pipeline.SyncResult

	if *movieID != "" {
		log.Printf("[INFO] On-demand mode: targeting movie '%s' + batch fill", *movieID)
		result, err = orchestrator.RunWithTarget(ctx, *movieID, cfg.MaxMoviesPerSync)
	} else {
		result, err = orchestrator.Run(ctx, cfg.MaxMoviesPerSync)
	}

	if err != nil {
		log.Fatalf("[FATAL] Pipeline sync failed: %v", err)
	}

	log.Printf("[INFO] Pipeline sync completed successfully: %d discovered, %d processed, %d skipped, %d errors",
		result.MoviesDiscovered, result.MoviesProcessed, result.MoviesSkipped, len(result.Errors))

	if len(result.Errors) > 0 {
		log.Printf("[WARN] Pipeline completed with %d item-level errors.", len(result.Errors))
	}
}
