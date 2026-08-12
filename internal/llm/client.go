package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"review_aggregator/internal/collector"
)

const maxRetries = 3

type Client interface {
	SummarizeMovie(ctx context.Context, title, overview string, reviews []collector.Review) (*SummaryResponse, error)
}

type DefaultClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewClient(baseURL, apiKey, model string) Client {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
	}
	if model == "" {
		model = "gemini-2.5-flash"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL = baseURL + "/v1"
	}

	return &DefaultClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *DefaultClient) SummarizeMovie(ctx context.Context, title, overview string, reviews []collector.Review) (*SummaryResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("LLM API key not configured")
	}

	prompt := c.buildPrompt(title, overview, reviews)

	reqBody := openAIChatRequest{
		Model: c.model,
		Messages: []openAIChatMessage{
			{
				Role:    "system",
				Content: "You are a professional film critic and audience sentiment analyst. Analyze movie reviews and output structured JSON strictly matching the requested schema.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.3,
		ResponseFormat: openAIResponseFormat{
			Type: "json_object",
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling LLM request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", c.baseURL)

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second // 1s, 2s, 4s
			log.Printf("[LLM] Retry %d/%d for '%s' after %v", attempt, maxRetries, title, backoff)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBytes))
		if err != nil {
			return nil, fmt.Errorf("creating LLM request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("executing LLM request: %w", err)
			continue // network error — retry
		}

		if isRetryableStatus(resp.StatusCode) {
			resp.Body.Close()
			lastErr = fmt.Errorf("LLM API returned status %d", resp.StatusCode)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			var errResp openAIChatResponse
			_ = json.NewDecoder(resp.Body).Decode(&errResp)
			resp.Body.Close()
			if errResp.Error != nil {
				return nil, fmt.Errorf("LLM API error (%d): %s", resp.StatusCode, errResp.Error.Message)
			}
			return nil, fmt.Errorf("LLM API returned status %d", resp.StatusCode)
		}

		var chatResp openAIChatResponse
		if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding LLM response: %w", err)
		}
		resp.Body.Close()

		if len(chatResp.Choices) == 0 {
			return nil, fmt.Errorf("LLM returned empty choices")
		}

		content := chatResp.Choices[0].Message.Content

		var summary SummaryResponse
		if err := json.Unmarshal([]byte(content), &summary); err != nil {
			return nil, fmt.Errorf("parsing LLM JSON response: %w (raw response: %s)", err, content)
		}

		return &summary, nil
	}

	return nil, fmt.Errorf("LLM request failed after %d retries: %w", maxRetries, lastErr)
}

func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,     // 429
		http.StatusBadGateway,           // 502
		http.StatusServiceUnavailable,   // 503
		http.StatusGatewayTimeout:       // 504
		return true
	}
	return false
}

func (c *DefaultClient) buildPrompt(title, overview string, reviews []collector.Review) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Movie Title: %s\n", title))
	if overview != "" {
		builder.WriteString(fmt.Sprintf("Overview: %s\n", overview))
	}

	if len(reviews) > 0 {
		builder.WriteString("\nCollected Audience Reviews:\n")
		for i, r := range reviews {
			builder.WriteString(fmt.Sprintf("\n--- Review #%d (Source: %s, Score: %d) ---\n", i+1, r.Source, r.Score))
			builder.WriteString(r.Content)
			builder.WriteString("\n")
		}
	} else {
		builder.WriteString("\nNote: No audience reviews collected yet (unreleased or newly announced movie). Please generate a summary based on the overview/synopsis.\n")
	}

	builder.WriteString(`
Please summarize the audience consensus into a single JSON object with the following exact keys:
{
  "overall_sentiment": <integer between 0 and 100 representing overall positive percentage>,
  "pros": [<string list of 2-4 major praised aspects>],
  "cons": [<string list of 2-4 major criticized aspects>],
  "common_themes": [<string list of 2-4 recurring topics/themes discussed>],
  "audience_consensus": "<short 2-3 sentence summary of how audiences feel>",
  "recommendation": "<short 1-2 sentence recommendation on who should watch this>"
}
Ensure the output is ONLY valid raw JSON with no Markdown markdown codeblock wrapping if possible.
`)

	return builder.String()
}
