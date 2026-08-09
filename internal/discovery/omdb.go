package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type OMDBClient struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

func NewOMDBClient(apiKey string) *OMDBClient {
	return &OMDBClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    "https://www.omdbapi.com",
	}
}

type omdbResponse struct {
	Response string `json:"Response"`
	Error    string `json:"Error"`
	Ratings  []struct {
		Source string `json:"Source"`
		Value  string `json:"Value"`
	} `json:"Ratings"`
	IMDbRating string `json:"imdbRating"`
}

func (c *OMDBClient) EnrichScores(ctx context.Context, movie *Movie) error {
	if c.apiKey == "" {
		return nil // Graceful skip if OMDb key is missing
	}

	var endpoint string
	if movie.IMDbID != "" {
		endpoint = fmt.Sprintf("%s/?apikey=%s&i=%s", c.baseURL, url.QueryEscape(c.apiKey), url.QueryEscape(movie.IMDbID))
	} else if movie.Title != "" {
		endpoint = fmt.Sprintf("%s/?apikey=%s&t=%s", c.baseURL, url.QueryEscape(c.apiKey), url.QueryEscape(movie.Title))
	} else {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("creating OMDb request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing OMDb request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OMDb API returned status %d", resp.StatusCode)
	}

	var omdbResp omdbResponse
	if err := json.NewDecoder(resp.Body).Decode(&omdbResp); err != nil {
		return fmt.Errorf("decoding OMDb response: %w", err)
	}

	if omdbResp.Response == "False" {
		return nil // Movie not found on OMDb, keep scores empty
	}

	if omdbResp.IMDbRating != "" && omdbResp.IMDbRating != "N/A" {
		movie.Scores.IMDb = omdbResp.IMDbRating
	}

	for _, rating := range omdbResp.Ratings {
		if rating.Source == "Rotten Tomatoes" {
			movie.Scores.RottenTomatoes = rating.Value
		}
	}

	return nil
}
