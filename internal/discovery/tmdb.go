package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type TMDBClient struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

func NewTMDBClient(apiKey string) *TMDBClient {
	return &TMDBClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    "https://api.themoviedb.org/3",
	}
}

type tmdbNowPlayingResponse struct {
	Results []struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		ReleaseDate string `json:"release_date"`
		PosterPath  string `json:"poster_path"`
		Overview    string `json:"overview"`
		GenreIDs    []int  `json:"genre_ids"`
	} `json:"results"`
}

type tmdbMovieDetailsResponse struct {
	IMDbID string `json:"imdb_id"`
	Genres []struct {
		Name string `json:"name"`
	} `json:"genres"`
}

func (c *TMDBClient) DiscoverRecentMovies(ctx context.Context, limit int) ([]Movie, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("TMDB API key not configured")
	}

	endpoint := fmt.Sprintf("%s/movie/now_playing?api_key=%s&language=en-US&page=1", c.baseURL, url.QueryEscape(c.apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating TMDB request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing TMDB request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API returned status %d", resp.StatusCode)
	}

	var nowPlaying tmdbNowPlayingResponse
	if err := json.NewDecoder(resp.Body).Decode(&nowPlaying); err != nil {
		return nil, fmt.Errorf("decoding TMDB response: %w", err)
	}

	var movies []Movie
	for i, item := range nowPlaying.Results {
		if limit > 0 && i >= limit {
			break
		}

		relDate, _ := time.Parse("2006-01-02", item.ReleaseDate)
		posterURL := ""
		if item.PosterPath != "" {
			posterURL = "https://image.tmdb.org/t/p/w500" + item.PosterPath
		}

		m := Movie{
			TMDBID:      item.ID,
			Title:       item.Title,
			ReleaseDate: relDate,
			PosterURL:   posterURL,
			Overview:    item.Overview,
		}

		// Enrich with IMDb ID if possible
		if imdbID, genres, err := c.fetchDetails(ctx, item.ID); err == nil {
			m.IMDbID = imdbID
			m.Genres = genres
		}

		movies = append(movies, m)
	}

	return movies, nil
}

func (c *TMDBClient) SearchMovies(ctx context.Context, query string, limit int) ([]Movie, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("TMDB API key not configured")
	}

	if query == "" {
		return nil, fmt.Errorf("query string cannot be empty")
	}

	endpoint := fmt.Sprintf("%s/search/movie?api_key=%s&query=%s&language=en-US&page=1", c.baseURL, url.QueryEscape(c.apiKey), url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating TMDB search request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing TMDB search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API returned status %d", resp.StatusCode)
	}

	var searchResp tmdbNowPlayingResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decoding TMDB search response: %w", err)
	}

	var movies []Movie
	for i, item := range searchResp.Results {
		if limit > 0 && i >= limit {
			break
		}

		relDate, _ := time.Parse("2006-01-02", item.ReleaseDate)
		posterURL := ""
		if item.PosterPath != "" {
			posterURL = "https://image.tmdb.org/t/p/w500" + item.PosterPath
		}

		m := Movie{
			TMDBID:      item.ID,
			Title:       item.Title,
			ReleaseDate: relDate,
			PosterURL:   posterURL,
			Overview:    item.Overview,
		}

		// Enrich with IMDb ID if possible
		if imdbID, genres, err := c.fetchDetails(ctx, item.ID); err == nil {
			m.IMDbID = imdbID
			m.Genres = genres
		}

		movies = append(movies, m)
	}

	return movies, nil
}


func (c *TMDBClient) fetchDetails(ctx context.Context, tmdbID int) (string, []string, error) {
	endpoint := fmt.Sprintf("%s/movie/%d?api_key=%s", c.baseURL, tmdbID, url.QueryEscape(c.apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var details tmdbMovieDetailsResponse
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return "", nil, err
	}

	var genres []string
	for _, g := range details.Genres {
		genres = append(genres, g.Name)
	}

	return details.IMDbID, genres, nil
}

func FormatTMDBID(tmdbID int) string {
	return "tmdb_" + strconv.Itoa(tmdbID)
}
