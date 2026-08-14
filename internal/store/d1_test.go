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
							"id":                 "tmdb_100",
							"tmdb_id":            float64(100),
							"imdb_id":            "tt0100",
							"title":              "Test Movie",
							"release_date":       "2025-07-11T00:00:00Z",
							"poster_url":         "https://example.com/poster.jpg",
							"overview":           "Test overview",
							"imdb_score":         float64(8.2),
							"rotten_tomatoes":    float64(88),
							"overall_sentiment":  float64(90),
							"audience_consensus": "Great test movie",
							"recommendation":     "Must watch",
							"review_count":       float64(15),
							"last_updated":       "2026-08-10T12:00:00Z",
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
				{
					Success: true,
					Results: []map[string]any{
						{"type": "pro", "content": "Great visuals"},
						{"type": "con", "content": "Pacing issues"},
					},
				},
				{
					Success: true,
					Results: []map[string]any{
						{"theme": "Identity"},
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
		ID:                  "tmdb_100",
		TMDBID:              100,
		IMDbID:              "tt0100",
		Title:               "Test Movie",
		ReleaseDate:         time.Date(2025, 7, 11, 0, 0, 0, 0, time.UTC),
		PosterURL:           "https://example.com/poster.jpg",
		Overview:            "Test overview",
		Genres:              []string{"Action", "Sci-Fi"},
		IMDbScore:           ptrFloat(8.2),
		RottenTomatoes:      ptrInt(88),
		OverallSentiment:    ptrInt(90),
		AudienceConsensus:   "Great test movie",
		Recommendation:      "Must watch",
		Pros:                []string{"Great visuals"},
		Cons:                []string{"Pacing issues"},
		Themes:              []string{"Identity"},
		ReviewCountAnalyzed: 15,
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
	if len(gotDoc.Pros) != 1 || gotDoc.Pros[0] != "Great visuals" {
		t.Errorf("Expected pros ['Great visuals'], got %v", gotDoc.Pros)
	}
	if len(gotDoc.Cons) != 1 || gotDoc.Cons[0] != "Pacing issues" {
		t.Errorf("Expected cons ['Pacing issues'], got %v", gotDoc.Cons)
	}
	if len(gotDoc.Themes) != 1 || gotDoc.Themes[0] != "Identity" {
		t.Errorf("Expected themes ['Identity'], got %v", gotDoc.Themes)
	}
	if gotDoc.IMDbScore == nil || *gotDoc.IMDbScore != 8.2 {
		t.Errorf("Expected IMDbScore 8.2, got %v", gotDoc.IMDbScore)
	}
	if gotDoc.RottenTomatoes == nil || *gotDoc.RottenTomatoes != 88 {
		t.Errorf("Expected RottenTomatoes 88, got %v", gotDoc.RottenTomatoes)
	}
}

func ptrFloat(f float64) *float64 { return &f }
func ptrInt(i int) *int           { return &i }
