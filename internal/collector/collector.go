package collector

import (
	"context"
	"time"
)

type Review struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	Author    string    `json:"author"`
	Content   string    `json:"content"`
	Score     int       `json:"score"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

type Collector interface {
	Name() string
	FetchReviews(ctx context.Context, movieTitle string) ([]Review, error)
}
