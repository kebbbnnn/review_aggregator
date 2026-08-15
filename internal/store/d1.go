package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type D1Store struct {
	accountID  string
	databaseID string
	apiToken   string
	httpClient *http.Client
	baseURL    string
}

func NewD1Store(accountID, databaseID, apiToken string) (*D1Store, error) {
	if accountID == "" || databaseID == "" || apiToken == "" {
		log.Println("[WARN] Cloudflare D1 credentials missing. Operating D1 store in no-op mock mode.")
		return &D1Store{
			accountID:  accountID,
			databaseID: databaseID,
			apiToken:   apiToken,
			httpClient: nil,
			baseURL:    "https://api.cloudflare.com/client/v4",
		}, nil
	}

	return &D1Store{
		accountID:  accountID,
		databaseID: databaseID,
		apiToken:   apiToken,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    "https://api.cloudflare.com/client/v4",
	}, nil
}

func NewMetadataStore(accountID, databaseID, apiToken string) (MetadataStore, error) {
	return NewD1Store(accountID, databaseID, apiToken)
}

func NewSummaryStore(accountID, databaseID, apiToken string) (SummaryStore, error) {
	return NewD1Store(accountID, databaseID, apiToken)
}

type d1QueryItem struct {
	SQL    string `json:"sql"`
	Params []any  `json:"params,omitempty"`
}

type d1BatchRequest struct {
	Batch []d1QueryItem `json:"batch"`
}

type d1APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type d1StatementResult struct {
	Results []map[string]any `json:"results"`
	Success bool             `json:"success"`
}

type d1APIResponse struct {
	Result  []d1StatementResult `json:"result"`
	Success bool                `json:"success"`
	Errors  []d1APIError        `json:"errors"`
}

// -------------------------------------------------------------
// MetadataStore Implementation (DB1: Movies & Genres)
// -------------------------------------------------------------

func (s *D1Store) SaveMovie(ctx context.Context, doc *MovieDocument) error {
	if s.httpClient == nil {
		log.Printf("[MOCK STORE] SaveMovie: %s (TMDB ID: %d)", doc.Title, doc.TMDBID)
		return nil
	}

	now := time.Now().UTC()
	doc.LastUpdated = now

	releaseDateStr := ""
	if !doc.ReleaseDate.IsZero() {
		releaseDateStr = doc.ReleaseDate.UTC().Format(time.RFC3339)
	}
	lastUpdatedStr := now.Format(time.RFC3339)

	var batch []d1QueryItem

	// 1. Delete existing genres
	batch = append(batch, d1QueryItem{
		SQL:    "DELETE FROM movie_genres WHERE movie_id = ?;",
		Params: []any{doc.ID},
	})

	// 2. Insert or replace movie row
	upsertSQL := `INSERT INTO movies (
		id, tmdb_id, imdb_id, title, release_date, poster_url, overview,
		imdb_score, rotten_tomatoes, last_updated
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		tmdb_id=excluded.tmdb_id,
		imdb_id=excluded.imdb_id,
		title=excluded.title,
		release_date=excluded.release_date,
		poster_url=excluded.poster_url,
		overview=excluded.overview,
		imdb_score=excluded.imdb_score,
		rotten_tomatoes=excluded.rotten_tomatoes,
		last_updated=excluded.last_updated;`

	batch = append(batch, d1QueryItem{
		SQL: upsertSQL,
		Params: []any{
			doc.ID,
			doc.TMDBID,
			doc.IMDbID,
			doc.Title,
			releaseDateStr,
			doc.PosterURL,
			doc.Overview,
			doc.IMDbScore,
			doc.RottenTomatoes,
			lastUpdatedStr,
		},
	})

	// 3. Insert genres
	for _, genre := range doc.Genres {
		if genre == "" {
			continue
		}
		batch = append(batch, d1QueryItem{
			SQL:    "INSERT OR IGNORE INTO movie_genres (movie_id, genre) VALUES (?, ?);",
			Params: []any{doc.ID, genre},
		})
	}

	_, err := s.executeBatch(ctx, batch)
	if err != nil {
		return fmt.Errorf("saving movie %s to D1: %w", doc.ID, err)
	}

	return nil
}

func (s *D1Store) SaveMovieBatch(ctx context.Context, docs []*MovieDocument) error {
	if len(docs) == 0 {
		return nil
	}

	if s.httpClient == nil {
		log.Printf("[MOCK STORE] SaveMovieBatch: %d movies", len(docs))
		return nil
	}

	now := time.Now().UTC()
	lastUpdatedStr := now.Format(time.RFC3339)

	var batch []d1QueryItem

	upsertSQL := `INSERT INTO movies (
		id, tmdb_id, imdb_id, title, release_date, poster_url, overview,
		imdb_score, rotten_tomatoes, last_updated
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		tmdb_id=excluded.tmdb_id,
		imdb_id=excluded.imdb_id,
		title=excluded.title,
		release_date=excluded.release_date,
		poster_url=excluded.poster_url,
		overview=excluded.overview,
		imdb_score=excluded.imdb_score,
		rotten_tomatoes=excluded.rotten_tomatoes,
		last_updated=excluded.last_updated;`

	for _, doc := range docs {
		doc.LastUpdated = now

		releaseDateStr := ""
		if !doc.ReleaseDate.IsZero() {
			releaseDateStr = doc.ReleaseDate.UTC().Format(time.RFC3339)
		}

		// Delete existing genres
		batch = append(batch, d1QueryItem{
			SQL:    "DELETE FROM movie_genres WHERE movie_id = ?;",
			Params: []any{doc.ID},
		})

		// Upsert movie
		batch = append(batch, d1QueryItem{
			SQL: upsertSQL,
			Params: []any{
				doc.ID,
				doc.TMDBID,
				doc.IMDbID,
				doc.Title,
				releaseDateStr,
				doc.PosterURL,
				doc.Overview,
				doc.IMDbScore,
				doc.RottenTomatoes,
				lastUpdatedStr,
			},
		})

		// Insert genres
		for _, genre := range doc.Genres {
			if genre == "" {
				continue
			}
			batch = append(batch, d1QueryItem{
				SQL:    "INSERT OR IGNORE INTO movie_genres (movie_id, genre) VALUES (?, ?);",
				Params: []any{doc.ID, genre},
			})
		}
	}

	chunkSize := 80
	for i := 0; i < len(batch); i += chunkSize {
		end := i + chunkSize
		if end > len(batch) {
			end = len(batch)
		}
		subBatch := batch[i:end]
		_, err := s.executeBatch(ctx, subBatch)
		if err != nil {
			return fmt.Errorf("saving movie batch to D1: %w", err)
		}
	}

	return nil
}

func (s *D1Store) MovieExists(ctx context.Context, id string) (bool, error) {
	if s.httpClient == nil {
		return false, nil
	}

	batch := []d1QueryItem{
		{
			SQL:    "SELECT id FROM movies WHERE id = ? LIMIT 1;",
			Params: []any{id},
		},
	}

	resp, err := s.executeBatch(ctx, batch)
	if err != nil {
		return false, fmt.Errorf("checking movie exists %s in D1: %w", id, err)
	}

	if len(resp.Result) == 0 || len(resp.Result[0].Results) == 0 {
		return false, nil
	}

	return true, nil
}

func (s *D1Store) GetMovie(ctx context.Context, id string) (*MovieDocument, bool, error) {
	if s.httpClient == nil {
		return nil, false, nil
	}

	batch := []d1QueryItem{
		{
			SQL:    "SELECT id, tmdb_id, imdb_id, title, release_date, poster_url, overview, imdb_score, rotten_tomatoes, last_updated FROM movies WHERE id = ? LIMIT 1;",
			Params: []any{id},
		},
		{
			SQL:    "SELECT genre FROM movie_genres WHERE movie_id = ?;",
			Params: []any{id},
		},
	}

	resp, err := s.executeBatch(ctx, batch)
	if err != nil {
		return nil, false, fmt.Errorf("getting movie %s from D1: %w", id, err)
	}

	if len(resp.Result) == 0 || len(resp.Result[0].Results) == 0 {
		return nil, false, nil
	}

	row := resp.Result[0].Results[0]
	doc := parseMovieRow(row)

	// Genres
	if len(resp.Result) > 1 {
		for _, gRow := range resp.Result[1].Results {
			if g, ok := gRow["genre"].(string); ok && g != "" {
				doc.Genres = append(doc.Genres, g)
			}
		}
	}

	return doc, true, nil
}

// -------------------------------------------------------------
// SummaryStore Implementation (DB2: Summaries, Points, Themes)
// -------------------------------------------------------------

func (s *D1Store) SaveSummary(ctx context.Context, doc *SummaryDocument) error {
	if s.httpClient == nil {
		log.Printf("[MOCK STORE] SaveSummary: movie %s", doc.MovieID)
		return nil
	}

	now := time.Now().UTC()
	doc.LastUpdated = now
	lastUpdatedStr := now.Format(time.RFC3339)

	var batch []d1QueryItem

	// 1. Delete existing points and themes
	batch = append(batch, d1QueryItem{
		SQL:    "DELETE FROM movie_points WHERE movie_id = ?;",
		Params: []any{doc.MovieID},
	})
	batch = append(batch, d1QueryItem{
		SQL:    "DELETE FROM movie_themes WHERE movie_id = ?;",
		Params: []any{doc.MovieID},
	})

	// 2. Upsert summary row
	upsertSQL := `INSERT INTO movie_summaries (
		movie_id, overall_sentiment, audience_consensus, recommendation, review_count, last_updated
	) VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(movie_id) DO UPDATE SET
		overall_sentiment=excluded.overall_sentiment,
		audience_consensus=excluded.audience_consensus,
		recommendation=excluded.recommendation,
		review_count=excluded.review_count,
		last_updated=excluded.last_updated;`

	batch = append(batch, d1QueryItem{
		SQL: upsertSQL,
		Params: []any{
			doc.MovieID,
			doc.OverallSentiment,
			doc.AudienceConsensus,
			doc.Recommendation,
			doc.ReviewCountAnalyzed,
			lastUpdatedStr,
		},
	})

	// 3. Insert pros & cons
	for _, pro := range doc.Pros {
		if pro == "" {
			continue
		}
		batch = append(batch, d1QueryItem{
			SQL:    "INSERT INTO movie_points (movie_id, type, content) VALUES (?, 'pro', ?);",
			Params: []any{doc.MovieID, pro},
		})
	}
	for _, con := range doc.Cons {
		if con == "" {
			continue
		}
		batch = append(batch, d1QueryItem{
			SQL:    "INSERT INTO movie_points (movie_id, type, content) VALUES (?, 'con', ?);",
			Params: []any{doc.MovieID, con},
		})
	}

	// 4. Insert themes
	for _, theme := range doc.Themes {
		if theme == "" {
			continue
		}
		batch = append(batch, d1QueryItem{
			SQL:    "INSERT INTO movie_themes (movie_id, theme) VALUES (?, ?);",
			Params: []any{doc.MovieID, theme},
		})
	}

	_, err := s.executeBatch(ctx, batch)
	if err != nil {
		return fmt.Errorf("saving summary for movie %s to D1: %w", doc.MovieID, err)
	}

	return nil
}

func (s *D1Store) GetSummary(ctx context.Context, movieID string) (*SummaryDocument, bool, error) {
	if s.httpClient == nil {
		return nil, false, nil
	}

	batch := []d1QueryItem{
		{
			SQL:    "SELECT movie_id, overall_sentiment, audience_consensus, recommendation, review_count, last_updated FROM movie_summaries WHERE movie_id = ? LIMIT 1;",
			Params: []any{movieID},
		},
		{
			SQL:    "SELECT type, content FROM movie_points WHERE movie_id = ?;",
			Params: []any{movieID},
		},
		{
			SQL:    "SELECT theme FROM movie_themes WHERE movie_id = ?;",
			Params: []any{movieID},
		},
	}

	resp, err := s.executeBatch(ctx, batch)
	if err != nil {
		return nil, false, fmt.Errorf("getting summary %s from D1: %w", movieID, err)
	}

	if len(resp.Result) == 0 || len(resp.Result[0].Results) == 0 {
		return nil, false, nil
	}

	row := resp.Result[0].Results[0]
	doc := parseSummaryRow(row)

	// Points (Pros / Cons)
	if len(resp.Result) > 1 {
		for _, pRow := range resp.Result[1].Results {
			pType, _ := pRow["type"].(string)
			pContent, _ := pRow["content"].(string)
			if pContent == "" {
				continue
			}
			if pType == "pro" {
				doc.Pros = append(doc.Pros, pContent)
			} else if pType == "con" {
				doc.Cons = append(doc.Cons, pContent)
			}
		}
	}

	// Themes
	if len(resp.Result) > 2 {
		for _, tRow := range resp.Result[2].Results {
			if t, ok := tRow["theme"].(string); ok && t != "" {
				doc.Themes = append(doc.Themes, t)
			}
		}
	}

	return doc, true, nil
}

func (s *D1Store) SummaryExists(ctx context.Context, movieID string) (bool, error) {
	if s.httpClient == nil {
		return false, nil
	}

	batch := []d1QueryItem{
		{
			SQL:    "SELECT movie_id FROM movie_summaries WHERE movie_id = ? LIMIT 1;",
			Params: []any{movieID},
		},
	}

	resp, err := s.executeBatch(ctx, batch)
	if err != nil {
		return false, fmt.Errorf("checking summary exists %s in D1: %w", movieID, err)
	}

	if len(resp.Result) == 0 || len(resp.Result[0].Results) == 0 {
		return false, nil
	}

	return true, nil
}

func (s *D1Store) Close() error {
	return nil
}

func (s *D1Store) executeBatch(ctx context.Context, batch []d1QueryItem) (*d1APIResponse, error) {
	url := fmt.Sprintf("%s/accounts/%s/d1/database/%s/query", s.baseURL, s.accountID, s.databaseID)

	reqBody, err := json.Marshal(d1BatchRequest{Batch: batch})
	if err != nil {
		return nil, fmt.Errorf("marshaling D1 batch request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("creating D1 request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing D1 HTTP request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading D1 response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("D1 API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var apiResp d1APIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("decoding D1 response: %w", err)
	}

	if !apiResp.Success {
		var errMsgs []string
		for _, e := range apiResp.Errors {
			errMsgs = append(errMsgs, fmt.Sprintf("[%d] %s", e.Code, e.Message))
		}
		return nil, fmt.Errorf("D1 query failure: %v", errMsgs)
	}

	return &apiResp, nil
}

func parseMovieRow(row map[string]any) *MovieDocument {
	doc := &MovieDocument{}

	if v, ok := row["id"].(string); ok {
		doc.ID = v
	}
	if v, ok := row["tmdb_id"].(float64); ok {
		doc.TMDBID = int(v)
	}
	if v, ok := row["imdb_id"].(string); ok {
		doc.IMDbID = v
	}
	if v, ok := row["title"].(string); ok {
		doc.Title = v
	}
	if v, ok := row["release_date"].(string); ok && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			doc.ReleaseDate = t
		} else if t, err := time.Parse("2006-01-02", v); err == nil {
			doc.ReleaseDate = t
		}
	}
	if v, ok := row["poster_url"].(string); ok {
		doc.PosterURL = v
	}
	if v, ok := row["overview"].(string); ok {
		doc.Overview = v
	}
	if v, ok := row["imdb_score"].(float64); ok {
		doc.IMDbScore = &v
	}
	if v, ok := row["rotten_tomatoes"].(float64); ok {
		val := int(v)
		doc.RottenTomatoes = &val
	}
	if v, ok := row["last_updated"].(string); ok && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			doc.LastUpdated = t
		}
	}

	return doc
}

func parseSummaryRow(row map[string]any) *SummaryDocument {
	doc := &SummaryDocument{}

	if v, ok := row["movie_id"].(string); ok {
		doc.MovieID = v
	}
	if v, ok := row["overall_sentiment"].(float64); ok {
		val := int(v)
		doc.OverallSentiment = &val
	}
	if v, ok := row["audience_consensus"].(string); ok {
		doc.AudienceConsensus = v
	}
	if v, ok := row["recommendation"].(string); ok {
		doc.Recommendation = v
	}
	if v, ok := row["review_count"].(float64); ok {
		doc.ReviewCountAnalyzed = int(v)
	}
	if v, ok := row["last_updated"].(string); ok && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			doc.LastUpdated = t
		}
	}

	return doc
}
