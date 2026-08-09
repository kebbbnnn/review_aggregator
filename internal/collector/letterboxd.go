package collector

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type LetterboxdCollector struct {
	httpClient *http.Client
	userAgent  string
}

func NewLetterboxdCollector(userAgent string) *LetterboxdCollector {
	if userAgent == "" {
		userAgent = "MovieReviewAggregator/1.0"
	}
	return &LetterboxdCollector{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		userAgent:  userAgent,
	}
}

func (l *LetterboxdCollector) Name() string {
	return "letterboxd"
}

type rssChannel struct {
	XMLName xml.Name  `xml:"rss"`
	Items   []rssItem `xml:"channel>item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Creator     string `xml:"creator"`
}

func (l *LetterboxdCollector) FetchReviews(ctx context.Context, movieTitle string) ([]Review, error) {
	// Google News RSS search feed for movie reviews
	searchURL := fmt.Sprintf("https://news.google.com/rss/search?q=%s+movie+review&hl=en-US&gl=US&ceid=US:en", url.QueryEscape(movieTitle))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Letterboxd RSS request: %w", err)
	}

	req.Header.Set("User-Agent", l.userAgent)

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing Letterboxd RSS request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Letterboxd RSS may return 404 if no results
		return nil, nil
	}

	var rss rssChannel
	if err := xml.NewDecoder(resp.Body).Decode(&rss); err != nil {
		return nil, fmt.Errorf("decoding Letterboxd RSS: %w", err)
	}

	var reviews []Review
	for i, item := range rss.Items {
		if i >= 10 {
			break
		}

		cleanDesc := stripHTMLTags(item.Description)
		if strings.TrimSpace(cleanDesc) == "" {
			cleanDesc = item.Title
		}

		pubTime, _ := time.Parse(time.RFC1123, item.PubDate)
		if pubTime.IsZero() {
			pubTime, _ = time.Parse(time.RFC1123Z, item.PubDate)
		}

		author := item.Creator
		if author == "" {
			author = "Letterboxd User"
		}

		reviews = append(reviews, Review{
			ID:        fmt.Sprintf("letterboxd_%d", i+1),
			Source:    "letterboxd",
			Author:    author,
			Content:   cleanDesc,
			Score:     0,
			URL:       item.Link,
			CreatedAt: pubTime,
		})
	}

	return reviews, nil
}

func stripHTMLTags(s string) string {
	var builder strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			builder.WriteRune(r)
		}
	}
	return strings.TrimSpace(builder.String())
}
