package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"review_aggregator/internal/collector"
)

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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("creating LLM request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing LLM request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp openAIChatResponse
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != nil {
			return nil, fmt.Errorf("LLM API error (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("LLM API returned status %d", resp.StatusCode)
	}

	var chatResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decoding LLM response: %w", err)
	}

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

func (c *DefaultClient) buildPrompt(title, overview string, reviews []collector.Review) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Movie Title: %s\n", title))
	if overview != "" {
		builder.WriteString(fmt.Sprintf("Overview: %s\n", overview))
	}
	builder.WriteString("\nCollected Audience Reviews:\n")

	for i, r := range reviews {
		builder.WriteString(fmt.Sprintf("\n--- Review #%d (Source: %s, Score: %d) ---\n", i+1, r.Source, r.Score))
		builder.WriteString(r.Content)
		builder.WriteString("\n")
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
