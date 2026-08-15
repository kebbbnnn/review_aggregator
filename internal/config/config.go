package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	TMDBAPIKey         string
	OMDBAPIKey         string
	RedditClientID     string
	RedditClientSecret string
	RedditUserAgent    string
	LLMBaseURL         string
	LLMAPIKey          string
	LLMModel           string
	CFAccountID          string
	CFD1DatabaseID       string
	CFAPIToken           string
	CFSummaryAccountID   string
	CFSummaryDatabaseID  string
	CFSummaryAPIToken    string
	MaxMoviesPerSync     int
	MinPopularity        float64
	RecentMonths         int
}

func Load() (*Config, error) {
	_ = loadDotEnv(".env")

	tmdbKey := os.Getenv("TMDB_API_KEY")
	omdbKey := os.Getenv("OMDB_API_KEY")
	redditID := os.Getenv("REDDIT_CLIENT_ID")
	redditSecret := os.Getenv("REDDIT_CLIENT_SECRET")
	redditUA := getEnv("REDDIT_USER_AGENT", "MovieReviewAggregator/1.0")
	llmBaseURL := getEnv("LLM_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai")
	llmAPIKey := os.Getenv("LLM_API_KEY")
	llmModel := getEnv("LLM_MODEL", "gemini-2.5-flash")
	cfAccountID := os.Getenv("CF_ACCOUNT_ID")
	cfDatabaseID := os.Getenv("CF_D1_DATABASE_ID")
	cfAPIToken := os.Getenv("CF_API_TOKEN")

	cfSummaryAccountID := getEnv("CF_SUMMARY_ACCOUNT_ID", cfAccountID)
	cfSummaryDatabaseID := getEnv("CF_SUMMARY_DATABASE_ID", cfDatabaseID)
	cfSummaryAPIToken := getEnv("CF_SUMMARY_API_TOKEN", cfAPIToken)

	maxMoviesStr := getEnv("MAX_MOVIES_PER_SYNC", "10")
	maxMovies, err := strconv.Atoi(maxMoviesStr)
	if err != nil {
		maxMovies = 10
	}

	minPopStr := getEnv("MIN_POPULARITY", "50.0")
	minPop, err := strconv.ParseFloat(minPopStr, 64)
	if err != nil {
		minPop = 50.0
	}

	recentMonthsStr := getEnv("RECENT_MONTHS", "6")
	recentMonths, err := strconv.Atoi(recentMonthsStr)
	if err != nil {
		recentMonths = 6
	}

	cfg := &Config{
		TMDBAPIKey:          tmdbKey,
		OMDBAPIKey:          omdbKey,
		RedditClientID:      redditID,
		RedditClientSecret:  redditSecret,
		RedditUserAgent:     redditUA,
		LLMBaseURL:          llmBaseURL,
		LLMAPIKey:           llmAPIKey,
		LLMModel:            llmModel,
		CFAccountID:          cfAccountID,
		CFD1DatabaseID:       cfDatabaseID,
		CFAPIToken:           cfAPIToken,
		CFSummaryAccountID:  cfSummaryAccountID,
		CFSummaryDatabaseID: cfSummaryDatabaseID,
		CFSummaryAPIToken:   cfSummaryAPIToken,
		MaxMoviesPerSync:    maxMovies,
		MinPopularity:       minPop,
		RecentMonths:        recentMonths,
	}

	return cfg, nil
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
