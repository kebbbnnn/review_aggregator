package processor

import (
	"regexp"
	"sort"
	"strings"

	"review_aggregator/internal/collector"
)

var urlRegex = regexp.MustCompile(`https?://\S+`)

type Processor struct {
	MinWordCount int
	MaxReviews   int
}

func NewProcessor(minWordCount, maxReviews int) *Processor {
	if minWordCount <= 0 {
		minWordCount = 10
	}
	if maxReviews <= 0 {
		maxReviews = 30
	}
	return &Processor{
		MinWordCount: minWordCount,
		MaxReviews:   maxReviews,
	}
}

func (p *Processor) CleanAndDeduplicate(reviews []collector.Review) []collector.Review {
	seenContent := make(map[string]bool)
	var processed []collector.Review

	for _, r := range reviews {
		cleanedContent := p.cleanText(r.Content)
		wordCount := len(strings.Fields(cleanedContent))

		if wordCount < p.MinWordCount {
			continue
		}

		// Simplified deduplication key using normalized prefix
		key := p.hashKey(cleanedContent)
		if seenContent[key] {
			continue
		}
		seenContent[key] = true

		r.Content = cleanedContent
		processed = append(processed, r)
	}

	// Sort by score descending (higher quality first)
	sort.Slice(processed, func(i, j int) bool {
		return processed[i].Score > processed[j].Score
	})

	if len(processed) > p.MaxReviews {
		processed = processed[:p.MaxReviews]
	}

	return processed
}

func (p *Processor) cleanText(text string) string {
	// Remove URLs
	text = urlRegex.ReplaceAllString(text, "")
	// Normalize whitespace
	lines := strings.Split(text, "\n")
	var cleanLines []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			cleanLines = append(cleanLines, trimmed)
		}
	}
	return strings.Join(cleanLines, "\n")
}

func (p *Processor) hashKey(text string) string {
	lower := strings.ToLower(text)
	words := strings.Fields(lower)
	if len(words) > 15 {
		words = words[:15]
	}
	return strings.Join(words, " ")
}
