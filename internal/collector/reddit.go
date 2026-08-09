package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RedditCollector struct {
	clientID     string
	clientSecret string
	userAgent    string
	httpClient   *http.Client
	token        string
	tokenExpiry  time.Time
	mu           sync.Mutex
}

func NewRedditCollector(clientID, clientSecret, userAgent string) *RedditCollector {
	if userAgent == "" || userAgent == "MovieReviewAggregator/1.0" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	}
	return &RedditCollector{
		clientID:     clientID,
		clientSecret: clientSecret,
		userAgent:    userAgent,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (r *RedditCollector) Name() string {
	return "reddit"
}

type redditTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type redditSearchResponse struct {
	Data struct {
		Children []struct {
			Data struct {
				ID        string  `json:"id"`
				Title     string  `json:"title"`
				Selftext  string  `json:"selftext"`
				Author    string  `json:"author"`
				Score     int     `json:"score"`
				Permalink string  `json:"permalink"`
				Created   float64 `json:"created_utc"`
				Over18    bool    `json:"over_18"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

func (r *RedditCollector) getAccessToken(ctx context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.token != "" && time.Now().Before(r.tokenExpiry) {
		return r.token, nil
	}

	if r.clientID == "" || r.clientSecret == "" {
		return "", nil // Unauthenticated access
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.reddit.com/api/v1/access_token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(r.clientID, r.clientSecret)
	req.Header.Set("User-Agent", r.userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("reddit OAuth status %d", resp.StatusCode)
	}

	var tokenResp redditTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	r.token = tokenResp.AccessToken
	r.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)

	return r.token, nil
}

func (r *RedditCollector) FetchReviews(ctx context.Context, movieTitle string) ([]Review, error) {
	token, _ := r.getAccessToken(ctx)

	var reqURL string
	query := fmt.Sprintf(`title:"%s" (review OR opinion OR thoughts)`, movieTitle)

	if token != "" {
		reqURL = fmt.Sprintf("https://oauth.reddit.com/r/movies/search?q=%s&restrict_sr=on&sort=relevance&limit=15", url.QueryEscape(query))
	} else {
		reqURL = fmt.Sprintf("https://www.reddit.com/r/movies/search.json?q=%s&restrict_sr=on&sort=relevance&limit=15", url.QueryEscape(query))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating reddit search request: %w", err)
	}

	req.Header.Set("User-Agent", r.userAgent)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing reddit search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reddit API returned status %d", resp.StatusCode)
	}

	var searchResp redditSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decoding reddit search response: %w", err)
	}

	var reviews []Review
	for _, child := range searchResp.Data.Children {
		item := child.Data
		if item.Over18 {
			continue
		}

		content := item.Selftext
		if content == "" || content == "[deleted]" || content == "[removed]" {
			content = item.Title
		} else {
			content = item.Title + "\n\n" + content
		}

		createdTime := time.Unix(int64(item.Created), 0)

		reviews = append(reviews, Review{
			ID:        "reddit_" + item.ID,
			Source:    "reddit",
			Author:    item.Author,
			Content:   content,
			Score:     item.Score,
			URL:       "https://reddit.com" + item.Permalink,
			CreatedAt: createdTime,
		})
	}

	return reviews, nil
}

func FormatScore(score int) string {
	return strconv.Itoa(score)
}
