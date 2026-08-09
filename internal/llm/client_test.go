package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"review_aggregator/internal/collector"
)

func TestSummarizeMovie(t *testing.T) {
	mockResponse := openAIChatResponse{
		Choices: []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{
			{
				Message: struct {
					Content string `json:"content"`
				}{
					Content: `{
						"overall_sentiment": 85,
						"pros": ["Great visuals", "Strong acting"],
						"cons": ["Slow start"],
						"common_themes": ["Family", "Destiny"],
						"audience_consensus": "Audiences loved it.",
						"recommendation": "Highly recommended."
					}`,
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Unexpected Authorization header: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-model")
	reviews := []collector.Review{
		{ID: "1", Source: "reddit", Content: "Awesome movie!"},
	}

	summary, err := client.SummarizeMovie(context.Background(), "Inception", "Mind-bending thriller", reviews)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if summary.OverallSentiment != 85 {
		t.Errorf("Expected overall_sentiment 85, got %d", summary.OverallSentiment)
	}
	if len(summary.Pros) != 2 || summary.Pros[0] != "Great visuals" {
		t.Errorf("Unexpected pros: %v", summary.Pros)
	}
}
