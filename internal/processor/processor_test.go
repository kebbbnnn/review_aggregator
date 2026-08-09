package processor

import (
	"testing"

	"review_aggregator/internal/collector"
)

func TestCleanAndDeduplicate(t *testing.T) {
	p := NewProcessor(5, 2)

	reviews := []collector.Review{
		{
			ID:      "1",
			Content: "Short",
			Score:   10,
		},
		{
			ID:      "2",
			Content: "This is a great movie with amazing visual effects and acting http://example.com",
			Score:   50,
		},
		{
			ID:      "3",
			Content: "This is a great movie with amazing visual effects and acting",
			Score:   40,
		},
		{
			ID:      "4",
			Content: "An absolute masterpiece of cinema with superb character development and pacing",
			Score:   100,
		},
	}

	result := p.CleanAndDeduplicate(reviews)

	if len(result) != 2 {
		t.Fatalf("Expected 2 reviews after filtering and deduplication, got %d", len(result))
	}

	if result[0].ID != "4" {
		t.Errorf("Expected highest score review ID 4 first, got %s", result[0].ID)
	}

	if result[1].ID != "2" {
		t.Errorf("Expected second highest score review ID 2, got %s", result[1].ID)
	}
}
