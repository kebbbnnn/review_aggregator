package store

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// MovieDocument represents the complete persisted movie metadata and catalog record (DB1).
type MovieDocument struct {
	ID             string    `json:"id"`
	TMDBID         int       `json:"tmdb_id"`
	IMDbID         string    `json:"imdb_id,omitempty"`
	Title          string    `json:"title"`
	ReleaseDate    time.Time `json:"release_date"`
	PosterURL      string    `json:"poster_url,omitempty"`
	Overview       string    `json:"overview,omitempty"`
	Genres         []string  `json:"genres,omitempty"`
	Popularity     *float64  `json:"popularity,omitempty"`
	IMDbScore      *float64  `json:"imdb_score,omitempty"`
	RottenTomatoes *int      `json:"rotten_tomatoes,omitempty"`
	LastUpdated    time.Time `json:"last_updated"`
}

// SummaryDocument represents the persisted movie review and summary record (DB2).
type SummaryDocument struct {
	MovieID             string    `json:"movie_id"`
	OverallSentiment    *int      `json:"overall_sentiment,omitempty"`
	AudienceConsensus   string    `json:"audience_consensus,omitempty"`
	Recommendation      string    `json:"recommendation,omitempty"`
	Pros                []string  `json:"pros,omitempty"`
	Cons                []string  `json:"cons,omitempty"`
	Themes              []string  `json:"themes,omitempty"`
	ReviewCountAnalyzed int       `json:"review_count_analyzed"`
	LastUpdated         time.Time `json:"last_updated"`
}

// HasContent returns true if the summary document contains meaningful summary data.
func (s *SummaryDocument) HasContent() bool {
	if s == nil {
		return false
	}
	return s.OverallSentiment != nil || s.AudienceConsensus != "" || s.Recommendation != "" || len(s.Pros) > 0 || len(s.Cons) > 0 || len(s.Themes) > 0
}

// ParseIMDbScore extracts a numeric float from strings like "7.8" or "7.8/10".
func ParseIMDbScore(raw string) *float64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "N/A" {
		return nil
	}
	if idx := strings.Index(trimmed, "/"); idx != -1 {
		trimmed = trimmed[:idx]
	}
	val, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return nil
	}
	return &val
}

// ParseRottenTomatoesScore extracts an integer percentage from strings like "84%" or "84/100".
func ParseRottenTomatoesScore(raw string) *int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "N/A" {
		return nil
	}
	trimmed = strings.TrimSuffix(trimmed, "%")
	if idx := strings.Index(trimmed, "/"); idx != -1 {
		trimmed = trimmed[:idx]
	}
	val, err := strconv.Atoi(trimmed)
	if err != nil {
		return nil
	}
	return &val
}

// MetadataStore defines persistence operations for movie metadata (DB1).
type MetadataStore interface {
	SaveMovie(ctx context.Context, doc *MovieDocument) error
	SaveMovieBatch(ctx context.Context, docs []*MovieDocument) error
	GetMovie(ctx context.Context, id string) (*MovieDocument, bool, error)
	MovieExists(ctx context.Context, id string) (bool, error)
	Close() error
}

// SummaryStore defines persistence operations for movie summaries (DB2).
type SummaryStore interface {
	SaveSummary(ctx context.Context, doc *SummaryDocument) error
	GetSummary(ctx context.Context, movieID string) (*SummaryDocument, bool, error)
	SummaryExists(ctx context.Context, movieID string) (bool, error)
	Close() error
}

// Store is an alias for MetadataStore for backward-compatibility.
type Store = MetadataStore
