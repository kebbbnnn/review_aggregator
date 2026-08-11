package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
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
	log.Println("[INFO] Starting Movie Review Aggregator Service...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Loading config failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Firestore
	st, err := store.NewFirestoreStore(ctx, cfg.FirebaseProjectID)
	if err != nil {
		log.Fatalf("[FATAL] Firestore init failed: %v", err)
	}
	defer st.Close()

	// Initialize Discovery & Clients
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

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("/api/v1/jobs/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		if cfg.CronSecret != "" {
			secret := r.Header.Get("X-Cron-Secret")
			if secret != cfg.CronSecret {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		result, err := orchestrator.Run(r.Context(), cfg.MaxMoviesPerSync)
		if err != nil {
			log.Printf("[ERROR] Job sync failed: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	})

	searchHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		query := r.URL.Query().Get("q")
		if query == "" {
			query = r.URL.Query().Get("query")
		}

		if query == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "query parameter 'q' or 'query' is required"})
			return
		}

		limitStr := r.URL.Query().Get("limit")
		limit := 10
		if limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
			}
		}

		movies, err := tmdb.SearchMovies(r.Context(), query, limit)
		if err != nil {
			log.Printf("[ERROR] Movie search failed for '%s': %v", query, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		results := make([]*store.MovieDocument, 0, len(movies))
		for _, m := range movies {
			docID := discovery.FormatTMDBID(m.TMDBID)
			if existing, found, err := st.GetMovie(r.Context(), docID); err == nil && found {
				results = append(results, existing)
			} else {
				doc := &store.MovieDocument{
					ID:          docID,
					TMDBID:      m.TMDBID,
					IMDbID:      m.IMDbID,
					Title:       m.Title,
					ReleaseDate: m.ReleaseDate,
					PosterURL:   m.PosterURL,
					Overview:    m.Overview,
					Genres:      m.Genres,
					Scores:      m.Scores,
				}
				results = append(results, doc)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query":   query,
			"total":   len(results),
			"results": results,
		})
	}

	mux.HandleFunc("/api/v1/movies/search", searchHandler)
	mux.HandleFunc("/api/v1/movies", searchHandler)


	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 10 * time.Minute,
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[INFO] Server listening on port %s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Server error: %v", err)
		}
	}()

	<-stopChan
	log.Println("[INFO] Shutting down server gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] Forced shutdown: %v", err)
	}
	log.Println("[INFO] Server stopped.")
}
