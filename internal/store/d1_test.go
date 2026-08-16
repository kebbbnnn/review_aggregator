package store

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseScores(t *testing.T) {
	imdbTests := []struct {
		input    string
		expected *float64
	}{
		{"7.8", ptrFloat(7.8)},
		{"7.8/10", ptrFloat(7.8)},
		{" 8.5 ", ptrFloat(8.5)},
		{"N/A", nil},
		{"", nil},
		{"invalid", nil},
	}

	for _, tt := range imdbTests {
		res := ParseIMDbScore(tt.input)
		if tt.expected == nil {
			if res != nil {
				t.Errorf("ParseIMDbScore(%q) expected nil, got %v", tt.input, *res)
			}
		} else {
			if res == nil || *res != *tt.expected {
				t.Errorf("ParseIMDbScore(%q) expected %v, got %v", tt.input, *tt.expected, res)
			}
		}
	}

	rtTests := []struct {
		input    string
		expected *int
	}{
		{"84%", ptrInt(84)},
		{"84", ptrInt(84)},
		{" 95% ", ptrInt(95)},
		{"N/A", nil},
		{"", nil},
		{"bad", nil},
	}

	for _, tt := range rtTests {
		res := ParseRottenTomatoesScore(tt.input)
		if tt.expected == nil {
			if res != nil {
				t.Errorf("ParseRottenTomatoesScore(%q) expected nil, got %v", tt.input, *res)
			}
		} else {
			if res == nil || *res != *tt.expected {
				t.Errorf("ParseRottenTomatoesScore(%q) expected %v, got %v", tt.input, *tt.expected, res)
			}
		}
	}
}

func TestD1Store_SaveAndGetMovie(t *testing.T) {
	var receivedBatch []d1QueryItem

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req d1BatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		receivedBatch = req.Batch

		// Differentiate save vs get by the first query
		if len(req.Batch) > 0 && req.Batch[0].SQL == "DELETE FROM movie_genres WHERE movie_id = ?;" {
			// Save response
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(d1APIResponse{
				Success: true,
				Result: []d1StatementResult{
					{Success: true},
					{Success: true},
					{Success: true},
				},
			})
			return
		}

		// Get response
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(d1APIResponse{
			Success: true,
			Result: []d1StatementResult{
				{
					Success: true,
					Results: []map[string]any{
						{
							"id":              "tmdb_100",
							"tmdb_id":         float64(100),
							"imdb_id":         "tt0100",
							"title":           "Test Movie",
							"release_date":    "2025-07-11T00:00:00Z",
							"poster_url":      "https://example.com/poster.jpg",
							"overview":        "Test overview",
							"popularity":      float64(45.5),
							"imdb_score":      float64(8.2),
							"rotten_tomatoes": float64(88),
							"last_updated":    "2026-08-10T12:00:00Z",
						},
					},
				},
				{
					Success: true,
					Results: []map[string]any{
						{"genre": "Action"},
						{"genre": "Sci-Fi"},
					},
				},
			},
		})
	}))
	defer server.Close()

	store := &D1Store{
		accountID:  "acc-123",
		databaseID: "db-456",
		apiToken:   "test-token",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	doc := &MovieDocument{
		ID:             "tmdb_100",
		TMDBID:         100,
		IMDbID:         "tt0100",
		Title:          "Test Movie",
		ReleaseDate:    time.Date(2025, 7, 11, 0, 0, 0, 0, time.UTC),
		PosterURL:      "https://example.com/poster.jpg",
		Overview:       "Test overview",
		Genres:         []string{"Action", "Sci-Fi"},
		Popularity:     ptrFloat(45.5),
		IMDbScore:      ptrFloat(8.2),
		RottenTomatoes: ptrInt(88),
	}

	// Test SaveMovie
	err := store.SaveMovie(context.Background(), doc)
	if err != nil {
		t.Fatalf("SaveMovie failed: %v", err)
	}

	if len(receivedBatch) == 0 {
		t.Errorf("Expected non-empty batch in SaveMovie")
	}

	// Test GetMovie
	gotDoc, found, err := store.GetMovie(context.Background(), "tmdb_100")
	if err != nil {
		t.Fatalf("GetMovie failed: %v", err)
	}
	if !found || gotDoc == nil {
		t.Fatalf("Expected movie to be found")
	}

	if gotDoc.Title != "Test Movie" {
		t.Errorf("Expected title 'Test Movie', got %q", gotDoc.Title)
	}
	if len(gotDoc.Genres) != 2 || gotDoc.Genres[0] != "Action" {
		t.Errorf("Expected genres [Action, Sci-Fi], got %v", gotDoc.Genres)
	}
	if gotDoc.Popularity == nil || *gotDoc.Popularity != 45.5 {
		t.Errorf("Expected Popularity 45.5, got %v", gotDoc.Popularity)
	}
	if gotDoc.IMDbScore == nil || *gotDoc.IMDbScore != 8.2 {
		t.Errorf("Expected IMDbScore 8.2, got %v", gotDoc.IMDbScore)
	}
	if gotDoc.RottenTomatoes == nil || *gotDoc.RottenTomatoes != 88 {
		t.Errorf("Expected RottenTomatoes 88, got %v", gotDoc.RottenTomatoes)
	}
}

func TestD1Store_SaveAndGetSummary(t *testing.T) {
	var receivedBatch []d1QueryItem

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req d1BatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		receivedBatch = req.Batch

		if len(req.Batch) > 0 && req.Batch[0].SQL == "DELETE FROM movie_points WHERE movie_id = ?;" {
			// Save summary response
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(d1APIResponse{
				Success: true,
				Result: []d1StatementResult{
					{Success: true},
					{Success: true},
					{Success: true},
					{Success: true},
					{Success: true},
					{Success: true},
				},
			})
			return
		}

		// Get summary response
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(d1APIResponse{
			Success: true,
			Result: []d1StatementResult{
				{
					Success: true,
					Results: []map[string]any{
						{
							"movie_id":           "tmdb_100",
							"overall_sentiment":  float64(92),
							"audience_consensus": "Outstanding performance and visuals",
							"recommendation":     "Highly recommended",
							"review_count":       float64(24),
							"last_updated":       "2026-08-15T12:00:00Z",
						},
					},
				},
				{
					Success: true,
					Results: []map[string]any{
						{"type": "pro", "content": "Cinematography"},
						{"type": "con", "content": "Runtime"},
					},
				},
				{
					Success: true,
					Results: []map[string]any{
						{"theme": "Redemption"},
					},
				},
			},
		})
	}))
	defer server.Close()

	store := &D1Store{
		accountID:  "acc-sum-123",
		databaseID: "db-sum-456",
		apiToken:   "test-token",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	sumDoc := &SummaryDocument{
		MovieID:             "tmdb_100",
		OverallSentiment:    ptrInt(92),
		AudienceConsensus:   "Outstanding performance and visuals",
		Recommendation:      "Highly recommended",
		Pros:                []string{"Cinematography"},
		Cons:                []string{"Runtime"},
		Themes:              []string{"Redemption"},
		ReviewCountAnalyzed: 24,
	}

	err := store.SaveSummary(context.Background(), sumDoc)
	if err != nil {
		t.Fatalf("SaveSummary failed: %v", err)
	}

	if len(receivedBatch) == 0 {
		t.Errorf("Expected non-empty batch in SaveSummary")
	}

	gotSummary, found, err := store.GetSummary(context.Background(), "tmdb_100")
	if err != nil {
		t.Fatalf("GetSummary failed: %v", err)
	}
	if !found || gotSummary == nil {
		t.Fatalf("Expected summary to be found")
	}

	if gotSummary.OverallSentiment == nil || *gotSummary.OverallSentiment != 92 {
		t.Errorf("Expected sentiment 92, got %v", gotSummary.OverallSentiment)
	}
	if len(gotSummary.Pros) != 1 || gotSummary.Pros[0] != "Cinematography" {
		t.Errorf("Expected pros ['Cinematography'], got %v", gotSummary.Pros)
	}
	if len(gotSummary.Cons) != 1 || gotSummary.Cons[0] != "Runtime" {
		t.Errorf("Expected cons ['Runtime'], got %v", gotSummary.Cons)
	}
	if len(gotSummary.Themes) != 1 || gotSummary.Themes[0] != "Redemption" {
		t.Errorf("Expected themes ['Redemption'], got %v", gotSummary.Themes)
	}
}

func TestD1Store_SaveMovieBatch_And_MovieExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req d1BatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if len(req.Batch) > 0 && req.Batch[0].SQL == "SELECT id FROM movies WHERE id = ? LIMIT 1;" {
			id := req.Batch[0].Params[0].(string)
			if id == "tmdb_found" {
				_ = json.NewEncoder(w).Encode(d1APIResponse{
					Success: true,
					Result: []d1StatementResult{
						{
							Success: true,
							Results: []map[string]any{{"id": "tmdb_found"}},
						},
					},
				})
			} else {
				_ = json.NewEncoder(w).Encode(d1APIResponse{
					Success: true,
					Result: []d1StatementResult{
						{
							Success: true,
							Results: []map[string]any{},
						},
					},
				})
			}
			return
		}

		// Batch save response
		_ = json.NewEncoder(w).Encode(d1APIResponse{
			Success: true,
			Result: []d1StatementResult{
				{Success: true},
				{Success: true},
			},
		})
	}))
	defer server.Close()

	store := &D1Store{
		accountID:  "acc-123",
		databaseID: "db-456",
		apiToken:   "test-token",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	// Test MovieExists
	exists, err := store.MovieExists(context.Background(), "tmdb_found")
	if err != nil || !exists {
		t.Fatalf("expected tmdb_found to exist, got exists=%v, err=%v", exists, err)
	}

	notExists, err := store.MovieExists(context.Background(), "tmdb_not_found")
	if err != nil || notExists {
		t.Fatalf("expected tmdb_not_found to not exist, got exists=%v, err=%v", notExists, err)
	}

	// Test SaveMovieBatch
	docs := []*MovieDocument{
		{
			ID:          "tmdb_200",
			TMDBID:      200,
			Title:       "Batch Movie 1",
			ReleaseDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Genres:      []string{"Drama"},
		},
		{
			ID:          "tmdb_201",
			TMDBID:      201,
			Title:       "Batch Movie 2",
			ReleaseDate: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
			Genres:      []string{"Comedy", "Romance"},
		},
	}

	err = store.SaveMovieBatch(context.Background(), docs)
	if err != nil {
		t.Fatalf("SaveMovieBatch failed: %v", err)
	}

	// Test empty batch (no-op)
	err = store.SaveMovieBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("SaveMovieBatch(nil) failed: %v", err)
	}
}

func ptrFloat(f float64) *float64 { return &f }
func ptrInt(i int) *int           { return &i }
