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
	discoverer   discovery.Discoverer
	omdb         *discovery.OMDBClient
	collectors   []collector.Collector
	processor    *processor.Processor
	llmClient    llm.Client
	metaStore    store.MetadataStore
	summaryStore store.SummaryStore
}

func NewOrchestrator(
	discoverer discovery.Discoverer,
	omdb *discovery.OMDBClient,
	collectors []collector.Collector,
	processor *processor.Processor,
	llmClient llm.Client,
	metaStore store.MetadataStore,
	summaryStore store.SummaryStore,
) *Orchestrator {
	return &Orchestrator{
		discoverer:   discoverer,
		omdb:         omdb,
		collectors:   collectors,
		processor:    processor,
		llmClient:    llmClient,
		metaStore:    metaStore,
		summaryStore: summaryStore,
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

		var existingMovie *store.MovieDocument
		if m, found, _ := o.metaStore.GetMovie(ctx, movieID); found {
			existingMovie = m
		}

		var hasExistingSummary bool
		if o.summaryStore != nil {
			if s, found, _ := o.summaryStore.GetSummary(ctx, movieID); found && s != nil && s.HasContent() {
				hasExistingSummary = true
			}
		}

		// Freshness check (skip if updated within 24h AND already has a valid summary)
		if existingMovie != nil && time.Since(existingMovie.LastUpdated) < 24*time.Hour && hasExistingSummary {
			log.Printf("[PIPELINE] Skipping '%s' (fresh & has summary)", movie.Title)
			result.MoviesSkipped++
			continue
		}

		log.Printf("[PIPELINE] Processing movie: '%s' (TMDB ID: %d)", movie.Title, movie.TMDBID)

		// Enrich scores via OMDb
		if o.omdb != nil {
			if err := o.omdb.EnrichScores(ctx, &movie); err != nil {
				log.Printf("[WARN] Failed to enrich scores for '%s': %v", movie.Title, err)
			}
		}

		// Process LLM summary if needed
		if !hasExistingSummary && o.summaryStore != nil {
			// Concurrently fetch reviews across all collectors
			rawReviews := o.collectReviewsConcurrently(ctx, movie.Title)
			if len(rawReviews) == 0 {
				log.Printf("[WARN] No reviews found for '%s'", movie.Title)
			}

			// Preprocess and deduplicate
			cleanReviews := o.processor.CleanAndDeduplicate(rawReviews)
			reviewCount := len(cleanReviews)

			var overallSentiment *int
			var audienceConsensus string
			var recommendation string
			var pros, cons, themes []string

			// Generate summary via LLM
			if o.llmClient != nil && (len(cleanReviews) > 0 || movie.Overview != "") {
				sum, err := o.llmClient.SummarizeMovie(ctx, movie.Title, movie.Overview, cleanReviews)
				if err != nil {
					errMsg := fmt.Sprintf("LLM error for '%s': %v", movie.Title, err)
					log.Println("[ERROR]", errMsg)
					result.Errors = append(result.Errors, errMsg)
					continue
				} else if sum != nil {
					overallSentiment = &sum.OverallSentiment
					audienceConsensus = sum.AudienceConsensus
					recommendation = sum.Recommendation
					pros = sum.Pros
					cons = sum.Cons
					themes = sum.CommonThemes
				}
			}

			sumDoc := &store.SummaryDocument{
				MovieID:             movieID,
				OverallSentiment:    overallSentiment,
				AudienceConsensus:   audienceConsensus,
				Recommendation:      recommendation,
				Pros:                pros,
				Cons:                cons,
				Themes:              themes,
				ReviewCountAnalyzed: reviewCount,
				LastUpdated:         time.Now().UTC(),
			}

			if err := o.summaryStore.SaveSummary(ctx, sumDoc); err != nil {
				log.Printf("[WARN] Failed to save summary for '%s' to DB2: %v", movie.Title, err)
			}
		}

		doc := &store.MovieDocument{
			ID:             movieID,
			TMDBID:         movie.TMDBID,
			IMDbID:         movie.IMDbID,
			Title:          movie.Title,
			ReleaseDate:    movie.ReleaseDate,
			PosterURL:      movie.PosterURL,
			Overview:       movie.Overview,
			Genres:         movie.Genres,
			IMDbScore:      store.ParseIMDbScore(movie.Scores.IMDb),
			RottenTomatoes: store.ParseRottenTomatoesScore(movie.Scores.RottenTomatoes),
			LastUpdated:    time.Now().UTC(),
		}

		if err := o.metaStore.SaveMovie(ctx, doc); err != nil {
			errMsg := fmt.Sprintf("Failed to save metadata for '%s' to DB1: %v", movie.Title, err)
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
