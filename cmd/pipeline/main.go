package main

import (
	"context"
	"log"

	"review_aggregator/internal/collector"
	"review_aggregator/internal/config"
	"review_aggregator/internal/discovery"
	"review_aggregator/internal/llm"
	"review_aggregator/internal/pipeline"
	"review_aggregator/internal/processor"
	"review_aggregator/internal/store"
)

func main() {
	log.Println("[INFO] Starting Movie Review Aggregator Pipeline...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Loading config failed: %v", err)
	}

	ctx := context.Background()

	st, err := store.NewD1Store(cfg.CFAccountID, cfg.CFD1DatabaseID, cfg.CFAPIToken)
	if err != nil {
		log.Fatalf("[FATAL] D1 store init failed: %v", err)
	}
	defer st.Close()

	tmdb := discovery.NewTMDBClient(cfg.TMDBAPIKey)
	omdb := discovery.NewOMDBClient(cfg.OMDBAPIKey)

	collectors := []collector.Collector{
		collector.NewRedditCollector(cfg.RedditClientID, cfg.RedditClientSecret, cfg.RedditUserAgent),
		collector.NewLetterboxdCollector(cfg.RedditUserAgent),
	}

	proc := processor.NewProcessor(3, 30)

	var llmClient llm.Client
	if cfg.LLMAPIKey != "" {
		llmClient = llm.NewClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
	}

	orchestrator := pipeline.NewOrchestrator(tmdb, omdb, collectors, proc, llmClient, st)

	result, err := orchestrator.Run(ctx, cfg.MaxMoviesPerSync)
	if err != nil {
		log.Fatalf("[FATAL] Pipeline sync failed: %v", err)
	}

	log.Printf("[INFO] Pipeline sync completed successfully: %d discovered, %d processed, %d skipped, %d errors",
		result.MoviesDiscovered, result.MoviesProcessed, result.MoviesSkipped, len(result.Errors))

	if len(result.Errors) > 0 {
		log.Printf("[WARN] Pipeline completed with %d item-level errors.", len(result.Errors))
	}
}
