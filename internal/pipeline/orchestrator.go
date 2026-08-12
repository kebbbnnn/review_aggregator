package pipeline

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"review_aggregator/internal/collector"
	"review_aggregator/internal/discovery"
	"review_aggregator/internal/llm"
	"review_aggregator/internal/processor"
	"review_aggregator/internal/store"
)

type Orchestrator struct {
	discoverer discovery.Discoverer
	omdb       *discovery.OMDBClient
	collectors []collector.Collector
	processor  *processor.Processor
	llmClient  llm.Client
	store      store.Store
}

func NewOrchestrator(
	discoverer discovery.Discoverer,
	omdb *discovery.OMDBClient,
	collectors []collector.Collector,
	processor *processor.Processor,
	llmClient llm.Client,
	store store.Store,
) *Orchestrator {
	return &Orchestrator{
		discoverer: discoverer,
		omdb:       omdb,
		collectors: collectors,
		processor:  processor,
		llmClient:  llmClient,
		store:      store,
	}
}

type SyncResult struct {
	MoviesDiscovered int      `json:"movies_discovered"`
	MoviesProcessed  int      `json:"movies_processed"`
	MoviesSkipped    int      `json:"movies_skipped"`
	Errors           []string `json:"errors,omitempty"`
}

func (o *Orchestrator) Run(ctx context.Context, limit int) (*SyncResult, error) {
	log.Println("[PIPELINE] Starting movie sync job...")

	movies, err := o.discoverer.DiscoverRecentMovies(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("discovering movies: %w", err)
	}

	result := &SyncResult{
		MoviesDiscovered: len(movies),
	}

	for _, movie := range movies {
		movieID := discovery.FormatTMDBID(movie.TMDBID)

		var existingDoc *store.MovieDocument
		// Freshness check (skip if updated within 24h AND already has a valid summary)
		if existing, found, _ := o.store.GetMovie(ctx, movieID); found {
			existingDoc = existing
			if time.Since(existing.LastUpdated) < 24*time.Hour && existing.Summary != nil {
				log.Printf("[PIPELINE] Skipping '%s' (fresh & has summary)", movie.Title)
				result.MoviesSkipped++
				continue
			}
		}

		log.Printf("[PIPELINE] Processing movie: '%s' (TMDB ID: %d)", movie.Title, movie.TMDBID)

		// Enrich scores via OMDb
		if o.omdb != nil {
			if err := o.omdb.EnrichScores(ctx, &movie); err != nil {
				log.Printf("[WARN] Failed to enrich scores for '%s': %v", movie.Title, err)
			}
		}

		var summary *llm.SummaryResponse
		var reviewCount int

		if existingDoc != nil && existingDoc.Summary != nil {
			log.Printf("[PIPELINE] Existing summary found for '%s', skipping new summary generation", movie.Title)
			summary = existingDoc.Summary
			reviewCount = existingDoc.ReviewCountAnalyzed
		} else {
			// Concurrently fetch reviews across all collectors
			rawReviews := o.collectReviewsConcurrently(ctx, movie.Title)

			if len(rawReviews) == 0 {
				log.Printf("[WARN] No reviews found for '%s'", movie.Title)
			}

			// Preprocess and deduplicate
			cleanReviews := o.processor.CleanAndDeduplicate(rawReviews)
			reviewCount = len(cleanReviews)

			// Generate summary via LLM
			if o.llmClient != nil && (len(cleanReviews) > 0 || movie.Overview != "") {
				sum, err := o.llmClient.SummarizeMovie(ctx, movie.Title, movie.Overview, cleanReviews)
				if err != nil {
					errMsg := fmt.Sprintf("LLM error for '%s': %v", movie.Title, err)
					log.Println("[ERROR]", errMsg)
					result.Errors = append(result.Errors, errMsg)
					continue
				} else {
					summary = sum
				}
			}
		}

		doc := &store.MovieDocument{
			ID:                  movieID,
			TMDBID:              movie.TMDBID,
			IMDbID:              movie.IMDbID,
			Title:               movie.Title,
			ReleaseDate:         movie.ReleaseDate,
			PosterURL:           movie.PosterURL,
			Overview:            movie.Overview,
			Genres:              movie.Genres,
			Scores:              movie.Scores,
			Summary:             summary,
			ReviewCountAnalyzed: reviewCount,
			LastUpdated:         time.Now(),
		}

		if err := o.store.SaveMovie(ctx, doc); err != nil {
			errMsg := fmt.Sprintf("Failed to save '%s' to store: %v", movie.Title, err)
			log.Println("[ERROR]", errMsg)
			result.Errors = append(result.Errors, errMsg)
		} else {
			result.MoviesProcessed++
		}
	}

	log.Printf("[PIPELINE] Finished sync: %d processed, %d skipped, %d errors",
		result.MoviesProcessed, result.MoviesSkipped, len(result.Errors))

	return result, nil
}

func (o *Orchestrator) collectReviewsConcurrently(ctx context.Context, title string) []collector.Review {
	var mu sync.Mutex
	var allReviews []collector.Review

	g, gCtx := errgroup.WithContext(ctx)

	for _, c := range o.collectors {
		c := c
		g.Go(func() error {
			revs, err := c.FetchReviews(gCtx, title)
			if err != nil {
				log.Printf("[WARN] Collector '%s' error for '%s': %v", c.Name(), title, err)
				return nil // Don't abort other collectors on single source error
			}

			mu.Lock()
			allReviews = append(allReviews, revs...)
			mu.Unlock()

			return nil
		})
	}

	_ = g.Wait()
	return allReviews
}
