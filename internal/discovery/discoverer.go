package discovery

import (
	"context"
	"time"
)

type Scores struct {
	IMDb           string `json:"imdb,omitempty"`
	RottenTomatoes string `json:"rotten_tomatoes,omitempty"`
}

type Movie struct {
	TMDBID      int       `json:"tmdb_id"`
	IMDbID      string    `json:"imdb_id,omitempty"`
	Title       string    `json:"title"`
	ReleaseDate time.Time `json:"release_date"`
	PosterURL   string    `json:"poster_url,omitempty"`
	Overview    string    `json:"overview,omitempty"`
	Genres      []string  `json:"genres,omitempty"`
	Scores      Scores    `json:"scores"`
}

type Discoverer interface {
	DiscoverRecentMovies(ctx context.Context, limit int) ([]Movie, error)
}
