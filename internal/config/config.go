package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port               string
	CronSecret         string
	TMDBAPIKey         string
	OMDBAPIKey         string
	RedditClientID     string
	RedditClientSecret string
	RedditUserAgent    string
	LLMBaseURL         string
	LLMAPIKey          string
	LLMModel           string
	FirebaseProjectID  string
	MaxMoviesPerSync   int
}

func Load() (*Config, error) {
	_ = loadDotEnv(".env")

	port := getEnv("PORT", "8080")
	cronSecret := os.Getenv("CRON_SECRET")
	tmdbKey := os.Getenv("TMDB_API_KEY")
	omdbKey := os.Getenv("OMDB_API_KEY")
	redditID := os.Getenv("REDDIT_CLIENT_ID")
	redditSecret := os.Getenv("REDDIT_CLIENT_SECRET")
	redditUA := getEnv("REDDIT_USER_AGENT", "MovieReviewAggregator/1.0")
	llmBaseURL := getEnv("LLM_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai")
	llmAPIKey := os.Getenv("LLM_API_KEY")
	llmModel := getEnv("LLM_MODEL", "gemini-2.5-flash")
	firebaseProjectID := os.Getenv("FIREBASE_PROJECT_ID")

	maxMoviesStr := getEnv("MAX_MOVIES_PER_SYNC", "10")
	maxMovies, err := strconv.Atoi(maxMoviesStr)
	if err != nil {
		maxMovies = 10
	}

	cfg := &Config{
		Port:               port,
		CronSecret:         cronSecret,
		TMDBAPIKey:         tmdbKey,
		OMDBAPIKey:         omdbKey,
		RedditClientID:     redditID,
		RedditClientSecret: redditSecret,
		RedditUserAgent:    redditUA,
		LLMBaseURL:         llmBaseURL,
		LLMAPIKey:          llmAPIKey,
		LLMModel:           llmModel,
		FirebaseProjectID:  firebaseProjectID,
		MaxMoviesPerSync:   maxMovies,
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.CronSecret == "" {
		return fmt.Errorf("CRON_SECRET environment variable is required")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func loadDotEnv(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)

		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
	return scanner.Err()
}
