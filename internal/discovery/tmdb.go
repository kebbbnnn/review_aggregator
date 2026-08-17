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

type tmdbDiscoverResponse struct {
	Page         int `json:"page"`
	TotalPages   int `json:"total_pages"`
	TotalResults int `json:"total_results"`
	Results      []struct {
		ID          int     `json:"id"`
		Title       string  `json:"title"`
		ReleaseDate string  `json:"release_date"`
		PosterPath  string  `json:"poster_path"`
		Overview    string  `json:"overview"`
		GenreIDs    []int   `json:"genre_ids"`
		Popularity  float64 `json:"popularity"`
	} `json:"results"`
}

type tmdbGenreListResponse struct {
	Genres []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"genres"`
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

	if limit <= 0 {
		limit = 20
	}

	var movies []Movie
	page := 1
	maxPages := (limit + 19) / 20
	if maxPages > 5 {
		maxPages = 5
	}

	for page <= maxPages {
		endpoint := fmt.Sprintf("%s/movie/now_playing?api_key=%s&language=en-US&page=%d", c.baseURL, url.QueryEscape(c.apiKey), page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("creating TMDB request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("executing TMDB request: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("TMDB API returned status %d", resp.StatusCode)
		}

		var nowPlaying tmdbNowPlayingResponse
		if err := json.NewDecoder(resp.Body).Decode(&nowPlaying); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding TMDB response: %w", err)
		}
		resp.Body.Close()

		if len(nowPlaying.Results) == 0 {
			break
		}

		for _, item := range nowPlaying.Results {
			if len(movies) >= limit {
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

		if len(movies) >= limit {
			break
		}
		page++
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


func (c *TMDBClient) FetchGenreMap(ctx context.Context) (map[int]string, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("TMDB API key not configured")
	}

	endpoint := fmt.Sprintf("%s/genre/movie/list?api_key=%s&language=en-US", c.baseURL, url.QueryEscape(c.apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating TMDB genre request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing TMDB genre request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API returned status %d", resp.StatusCode)
	}

	var genreResp tmdbGenreListResponse
	if err := json.NewDecoder(resp.Body).Decode(&genreResp); err != nil {
		return nil, fmt.Errorf("decoding TMDB genre response: %w", err)
	}

	genreMap := make(map[int]string, len(genreResp.Genres))
	for _, g := range genreResp.Genres {
		genreMap[g.ID] = g.Name
	}

	return genreMap, nil
}

func (c *TMDBClient) DiscoverCatalog(ctx context.Context, page int, sortBy string, year int) ([]Movie, int, error) {
	if c.apiKey == "" {
		return nil, 0, fmt.Errorf("TMDB API key not configured")
	}

	if page < 1 {
		page = 1
	}
	if sortBy == "" {
		sortBy = "popularity.desc"
	}

	var endpoint string
	if year > 0 {
		endpoint = fmt.Sprintf("%s/discover/movie?api_key=%s&language=en-US&sort_by=%s&page=%d&primary_release_year=%d",
			c.baseURL, url.QueryEscape(c.apiKey), url.QueryEscape(sortBy), page, year)
	} else {
		endpoint = fmt.Sprintf("%s/discover/movie?api_key=%s&language=en-US&sort_by=%s&page=%d",
			c.baseURL, url.QueryEscape(c.apiKey), url.QueryEscape(sortBy), page)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("creating TMDB discover request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("executing TMDB discover request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("TMDB API returned status %d", resp.StatusCode)
	}

	var discoverResp tmdbDiscoverResponse
	if err := json.NewDecoder(resp.Body).Decode(&discoverResp); err != nil {
		return nil, 0, fmt.Errorf("decoding TMDB discover response: %w", err)
	}

	var movies []Movie
	for _, item := range discoverResp.Results {
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
			Popularity:  item.Popularity,
			GenreIDs:    item.GenreIDs,
		}

		movies = append(movies, m)
	}

	return movies, discoverResp.TotalPages, nil
}

func (c *TMDBClient) DiscoverAll(ctx context.Context, page int, sortBy string) ([]Movie, int, error) {
	return c.DiscoverCatalog(ctx, page, sortBy, 0)
}

func (c *TMDBClient) DiscoverPopularRecent(ctx context.Context, minPopularity float64, releasedAfter time.Time, limit int) ([]Movie, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("TMDB API key not configured")
	}

	dateStr := ""
	if !releasedAfter.IsZero() {
		dateStr = releasedAfter.Format("2006-01-02")
	}

	if limit <= 0 {
		limit = 10
	}

	// Fetch a candidate pool across multiple pages so the orchestrator has enough
	// unprocessed movies to fill its target limit even if top items are already cached.
	targetCandidates := limit * 5
	if targetCandidates < 40 {
		targetCandidates = 40
	}
	if targetCandidates > 100 {
		targetCandidates = 100
	}

	var movies []Movie
	page := 1
	maxPages := 5

	for page <= maxPages {
		endpoint := fmt.Sprintf("%s/discover/movie?api_key=%s&language=en-US&sort_by=popularity.desc&popularity.gte=%.2f&primary_release_date.gte=%s&page=%d",
			c.baseURL,
			url.QueryEscape(c.apiKey),
			minPopularity,
			url.QueryEscape(dateStr),
			page,
		)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("creating TMDB popular/recent request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("executing TMDB popular/recent request: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("TMDB API returned status %d", resp.StatusCode)
		}

		var discoverResp tmdbDiscoverResponse
		if err := json.NewDecoder(resp.Body).Decode(&discoverResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding TMDB popular/recent response: %w", err)
		}
		resp.Body.Close()

		if len(discoverResp.Results) == 0 {
			break
		}

		for _, item := range discoverResp.Results {
			if len(movies) >= targetCandidates {
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
				Popularity:  item.Popularity,
				GenreIDs:    item.GenreIDs,
			}

			// Enrich with IMDb ID and Genre names for deep processing
			if imdbID, genres, err := c.fetchDetails(ctx, item.ID); err == nil {
				m.IMDbID = imdbID
				m.Genres = genres
			}

			movies = append(movies, m)
		}

		if len(movies) >= targetCandidates || page >= discoverResp.TotalPages {
			break
		}
		page++
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

// PopularRecentDiscoverer adapts TMDBClient's DiscoverPopularRecent to the Discoverer interface.
type PopularRecentDiscoverer struct {
	client        *TMDBClient
	minPopularity float64
	releasedAfter time.Time
}

func NewPopularRecentDiscoverer(client *TMDBClient, minPopularity float64, releasedAfter time.Time) *PopularRecentDiscoverer {
	return &PopularRecentDiscoverer{
		client:        client,
		minPopularity: minPopularity,
		releasedAfter: releasedAfter,
	}
}

func (d *PopularRecentDiscoverer) DiscoverRecentMovies(ctx context.Context, limit int) ([]Movie, error) {
	return d.client.DiscoverPopularRecent(ctx, d.minPopularity, d.releasedAfter, limit)
}
