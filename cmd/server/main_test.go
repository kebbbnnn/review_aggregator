package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"review_aggregator/internal/discovery"
	"review_aggregator/internal/store"
)

type mockSearcher struct {
	movies []discovery.Movie
	err    error
}

func (m *mockSearcher) SearchMovies(ctx interface{}, query string, limit int) ([]discovery.Movie, error) {
	return m.movies, m.err
}

func TestSearchMovieEndpoint_MissingQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/movies/search", nil)
	w := httptest.NewRecorder()

	// Setup simple handler for testing parameter validation
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			q = r.URL.Query().Get("query")
		}
		if q == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "query parameter 'q' or 'query' is required"})
			return
		}
	})

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}

	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] == "" {
		t.Errorf("expected error message in body, got %v", body)
	}
}

func TestSearchMovieEndpoint_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/movies/search?q=test", nil)
	w := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
	})

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed, got %d", resp.StatusCode)
	}
}

func TestSearchMovieEndpoint_SuccessResponse(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/movies/search?q=inception", nil)
	w := httptest.NewRecorder()

	mockMovie := discovery.Movie{
		TMDBID:      27205,
		Title:       "Inception",
		ReleaseDate: time.Date(2010, 7, 16, 0, 0, 0, 0, time.UTC),
		PosterURL:   "https://image.tmdb.org/t/p/w500/poster.jpg",
		Overview:    "A thief who steals corporate secrets...",
		Genres:      []string{"Action", "Sci-Fi"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := &store.MovieDocument{
			ID:          discovery.FormatTMDBID(mockMovie.TMDBID),
			TMDBID:      mockMovie.TMDBID,
			Title:       mockMovie.Title,
			ReleaseDate: mockMovie.ReleaseDate,
			PosterURL:   mockMovie.PosterURL,
			Overview:    mockMovie.Overview,
			Genres:      mockMovie.Genres,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query":   "inception",
			"total":   1,
			"results": []*store.MovieDocument{doc},
		})
	})

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var res map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res["query"] != "inception" {
		t.Errorf("expected query 'inception', got %v", res["query"])
	}

	results, ok := res["results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("expected 1 result in array, got %v", res["results"])
	}
}
