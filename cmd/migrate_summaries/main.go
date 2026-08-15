package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"review_aggregator/internal/config"
	"review_aggregator/internal/store"
)

type d1QueryItem struct {
	SQL    string `json:"sql"`
	Params []any  `json:"params,omitempty"`
}

type d1BatchRequest struct {
	Batch []d1QueryItem `json:"batch"`
}

type d1StatementResult struct {
	Results []map[string]any `json:"results"`
	Success bool             `json:"success"`
}

type d1APIResponse struct {
	Result  []d1StatementResult `json:"result"`
	Success bool                `json:"success"`
}

func main() {
	log.Println("[INFO] Starting Summary Migration Tool (DB1 -> DB2)...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Failed to load configuration: %v", err)
	}

	if cfg.CFAccountID == "" || cfg.CFD1DatabaseID == "" || cfg.CFAPIToken == "" {
		log.Fatalf("[FATAL] DB1 Cloudflare credentials (CF_ACCOUNT_ID, CF_D1_DATABASE_ID, CF_API_TOKEN) are required")
	}

	if cfg.CFSummaryAccountID == "" || cfg.CFSummaryDatabaseID == "" || cfg.CFSummaryAPIToken == "" {
		log.Fatalf("[FATAL] DB2 Cloudflare credentials (CF_SUMMARY_ACCOUNT_ID, CF_SUMMARY_DATABASE_ID, CF_SUMMARY_API_TOKEN) are required")
	}

	ctx := context.Background()

	summaryStore, err := store.NewSummaryStore(cfg.CFSummaryAccountID, cfg.CFSummaryDatabaseID, cfg.CFSummaryAPIToken)
	if err != nil {
		log.Fatalf("[FATAL] Failed to init DB2 summary store: %v", err)
	}
	defer summaryStore.Close()

	log.Println("[INFO] Querying summaries from DB1...")

	// Query DB1 for all movies with summary fields
	httpClient := &http.Client{Timeout: 60 * time.Second}
	db1URL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/d1/database/%s/query", cfg.CFAccountID, cfg.CFD1DatabaseID)

	batchReq := d1BatchRequest{
		Batch: []d1QueryItem{
			{
				SQL: "SELECT id, overall_sentiment, audience_consensus, recommendation, review_count, last_updated FROM movies WHERE overall_sentiment IS NOT NULL OR audience_consensus IS NOT NULL OR recommendation IS NOT NULL;",
			},
			{
				SQL: "SELECT movie_id, type, content FROM movie_points;",
			},
			{
				SQL: "SELECT movie_id, theme FROM movie_themes;",
			},
		},
	}

	reqBody, _ := json.Marshal(batchReq)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, db1URL, bytes.NewReader(reqBody))
	if err != nil {
		log.Fatalf("[FATAL] Failed to create HTTP request to DB1: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.CFAPIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatalf("[FATAL] Request to DB1 failed: %v", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		log.Fatalf("[FATAL] DB1 API returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var d1Resp d1APIResponse
	if err := json.Unmarshal(respBytes, &d1Resp); err != nil {
		log.Fatalf("[FATAL] Failed to decode DB1 response: %v", err)
	}

	if len(d1Resp.Result) == 0 {
		log.Println("[INFO] No summary records found in DB1. Nothing to migrate.")
		return
	}

	movieRows := d1Resp.Result[0].Results
	log.Printf("[INFO] Discovered %d movies with summaries in DB1", len(movieRows))

	// Group points and themes
	pointsByMovie := make(map[string][]struct {
		pointType string
		content   string
	})
	if len(d1Resp.Result) > 1 {
		for _, pRow := range d1Resp.Result[1].Results {
			mID, _ := pRow["movie_id"].(string)
			pType, _ := pRow["type"].(string)
			content, _ := pRow["content"].(string)
			if mID != "" && content != "" {
				pointsByMovie[mID] = append(pointsByMovie[mID], struct {
					pointType string
					content   string
				}{pointType: pType, content: content})
			}
		}
	}

	themesByMovie := make(map[string][]string)
	if len(d1Resp.Result) > 2 {
		for _, tRow := range d1Resp.Result[2].Results {
			mID, _ := tRow["movie_id"].(string)
			theme, _ := tRow["theme"].(string)
			if mID != "" && theme != "" {
				themesByMovie[mID] = append(themesByMovie[mID], theme)
			}
		}
	}

	migratedCount := 0
	errorCount := 0

	for _, row := range movieRows {
		movieID, _ := row["id"].(string)
		if movieID == "" {
			continue
		}

		var sentiment *int
		if v, ok := row["overall_sentiment"].(float64); ok {
			val := int(v)
			sentiment = &val
		}

		consensus, _ := row["audience_consensus"].(string)
		recommendation, _ := row["recommendation"].(string)
		var reviewCount int
		if v, ok := row["review_count"].(float64); ok {
			reviewCount = int(v)
		}

		var pros, cons []string
		for _, p := range pointsByMovie[movieID] {
			if p.pointType == "pro" {
				pros = append(pros, p.content)
			} else if p.pointType == "con" {
				cons = append(cons, p.content)
			}
		}

		themes := themesByMovie[movieID]

		sumDoc := &store.SummaryDocument{
			MovieID:             movieID,
			OverallSentiment:    sentiment,
			AudienceConsensus:   consensus,
			Recommendation:      recommendation,
			Pros:                pros,
			Cons:                cons,
			Themes:              themes,
			ReviewCountAnalyzed: reviewCount,
			LastUpdated:         time.Now().UTC(),
		}

		if err := summaryStore.SaveSummary(ctx, sumDoc); err != nil {
			log.Printf("[ERROR] Failed to migrate summary for movie %s: %v", movieID, err)
			errorCount++
		} else {
			migratedCount++
			if migratedCount%10 == 0 || migratedCount == len(movieRows) {
				log.Printf("[INFO] Migrated %d/%d summaries to DB2...", migratedCount, len(movieRows))
			}
		}
	}

	log.Printf("[INFO] Migration complete! Successfully migrated: %d, Errors: %d", migratedCount, errorCount)
}
