package pipeline_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"review_aggregator/internal/collector"
	"review_aggregator/internal/discovery"
	"review_aggregator/internal/llm"
	"review_aggregator/internal/pipeline"
	"review_aggregator/internal/processor"
	"review_aggregator/internal/store"
)

type mockDiscoverer struct {
	movies []discovery.Movie
}

func (m *mockDiscoverer) DiscoverRecentMovies(ctx context.Context, limit int) ([]discovery.Movie, error) {
	return m.movies, nil
}

type mockCollector struct {
	mu      sync.Mutex
	calls   int
	reviews []collector.Review
}

func (m *mockCollector) Name() string { return "mock" }
func (m *mockCollector) FetchReviews(ctx context.Context, movieTitle string) ([]collector.Review, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return m.reviews, nil
}

type mockLLMClient struct {
	mu     sync.Mutex
	calls  int
	result *llm.SummaryResponse
}

func (m *mockLLMClient) SummarizeMovie(ctx context.Context, title, overview string, reviews []collector.Review) (*llm.SummaryResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return m.result, nil
}

type mockMetadataStore struct {
	mu     sync.Mutex
	movies map[string]*store.MovieDocument
}

func newMockMetadataStore() *mockMetadataStore {
	return &mockMetadataStore{movies: make(map[string]*store.MovieDocument)}
}

func (m *mockMetadataStore) SaveMovie(ctx context.Context, doc *store.MovieDocument) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.movies[doc.ID] = doc
	return nil
}

func (m *mockMetadataStore) SaveMovieBatch(ctx context.Context, docs []*store.MovieDocument) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, doc := range docs {
		m.movies[doc.ID] = doc
	}
	return nil
}

func (m *mockMetadataStore) GetMovie(ctx context.Context, id string) (*store.MovieDocument, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, ok := m.movies[id]
	return doc, ok, nil
}

func (m *mockMetadataStore) MovieExists(ctx context.Context, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.movies[id]
	return ok, nil
}

func (m *mockMetadataStore) Close() error { return nil }

type mockSummaryStore struct {
	mu        sync.Mutex
	summaries map[string]*store.SummaryDocument
}

func newMockSummaryStore() *mockSummaryStore {
	return &mockSummaryStore{summaries: make(map[string]*store.SummaryDocument)}
}

func (m *mockSummaryStore) SaveSummary(ctx context.Context, doc *store.SummaryDocument) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.summaries[doc.MovieID] = doc
	return nil
}

func (m *mockSummaryStore) GetSummary(ctx context.Context, movieID string) (*store.SummaryDocument, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, ok := m.summaries[movieID]
	return doc, ok, nil
}

func (m *mockSummaryStore) SummaryExists(ctx context.Context, movieID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.summaries[movieID]
	return ok, nil
}

func (m *mockSummaryStore) Close() error { return nil }

func ptrInt(i int) *int { return &i }

func TestOrchestrator_SkipSummaryForExistingMovie(t *testing.T) {
	movie := discovery.Movie{
		TMDBID:      1001,
		Title:       "Existing Summary Movie",
		ReleaseDate: time.Now().Add(-48 * time.Hour),
		Overview:    "Overview",
	}

	movieID := discovery.FormatTMDBID(movie.TMDBID)

	metaStore := newMockMetadataStore()
	metaStore.movies[movieID] = &store.MovieDocument{
		ID:          movieID,
		TMDBID:      movie.TMDBID,
		Title:       movie.Title,
		LastUpdated: time.Now().Add(-25 * time.Hour), // Older than 24h
	}

	sumStore := newMockSummaryStore()
	sumStore.summaries[movieID] = &store.SummaryDocument{
		MovieID:             movieID,
		OverallSentiment:    ptrInt(85),
		AudienceConsensus:   "Already summarized consensus",
		ReviewCountAnalyzed: 10,
		LastUpdated:         time.Now().Add(-25 * time.Hour),
	}

	disc := &mockDiscoverer{movies: []discovery.Movie{movie}}
	coll := &mockCollector{reviews: []collector.Review{{ID: "r1", Content: "New review"}}}
	llmCl := &mockLLMClient{result: &llm.SummaryResponse{OverallSentiment: 99}}
	proc := processor.NewProcessor(3, 30)

	orc := pipeline.NewOrchestrator(disc, nil, []collector.Collector{coll}, proc, llmCl, metaStore, sumStore)

	res, err := orc.Run(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.MoviesProcessed != 1 {
		t.Errorf("expected 1 processed movie, got %d", res.MoviesProcessed)
	}
	if res.MoviesSkipped != 0 {
		t.Errorf("expected 0 skipped movies, got %d", res.MoviesSkipped)
	}

	if llmCl.calls != 0 {
		t.Errorf("expected 0 LLM calls, got %d", llmCl.calls)
	}
	if coll.calls != 0 {
		t.Errorf("expected 0 collector calls, got %d", coll.calls)
	}

	savedDoc := metaStore.movies[movieID]
	if savedDoc == nil {
		t.Fatalf("expected movie doc to be saved in metaStore")
	}

	savedSum := sumStore.summaries[movieID]
	if savedSum == nil {
		t.Fatalf("expected summary to be preserved in sumStore")
	}
	if savedSum.AudienceConsensus != "Already summarized consensus" {
		t.Errorf("expected summary to be preserved, got %s", savedSum.AudienceConsensus)
	}
}

func TestOrchestrator_GenerateSummaryForMovieWithoutExistingSummary(t *testing.T) {
	movie := discovery.Movie{
		TMDBID:      1002,
		Title:       "New Movie",
		ReleaseDate: time.Now(),
		Overview:    "Overview",
	}

	movieID := discovery.FormatTMDBID(movie.TMDBID)

	metaStore := newMockMetadataStore()
	sumStore := newMockSummaryStore()
	disc := &mockDiscoverer{movies: []discovery.Movie{movie}}
	coll := &mockCollector{reviews: []collector.Review{{ID: "r1", Content: "Great movie!"}}}
	newSummary := &llm.SummaryResponse{
		OverallSentiment:  90,
		AudienceConsensus: "Newly generated consensus",
		Pros:              []string{"Visuals"},
		Cons:              []string{"Pacing"},
		CommonThemes:      []string{"Heroism"},
	}
	llmCl := &mockLLMClient{result: newSummary}
	proc := processor.NewProcessor(3, 30)

	orc := pipeline.NewOrchestrator(disc, nil, []collector.Collector{coll}, proc, llmCl, metaStore, sumStore)

	res, err := orc.Run(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.MoviesProcessed != 1 {
		t.Errorf("expected 1 processed movie, got %d", res.MoviesProcessed)
	}

	if llmCl.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", llmCl.calls)
	}
	if coll.calls != 1 {
		t.Errorf("expected 1 collector call, got %d", coll.calls)
	}

	savedDoc := metaStore.movies[movieID]
	if savedDoc == nil {
		t.Fatalf("expected movie doc to be saved in metaStore")
	}
	if savedDoc.Title != "New Movie" {
		t.Errorf("expected title 'New Movie', got %s", savedDoc.Title)
	}

	savedSum := sumStore.summaries[movieID]
	if savedSum == nil {
		t.Fatalf("expected summary to be saved in sumStore")
	}
	if savedSum.AudienceConsensus != "Newly generated consensus" {
		t.Errorf("expected new summary, got %s", savedSum.AudienceConsensus)
	}
	if len(savedSum.Pros) != 1 || savedSum.Pros[0] != "Visuals" {
		t.Errorf("expected pros ['Visuals'], got %v", savedSum.Pros)
	}
}

func TestOrchestrator_FreshnessCheckSkipsMovie(t *testing.T) {
	movie := discovery.Movie{
		TMDBID:      1003,
		Title:       "Fresh Movie",
		ReleaseDate: time.Now(),
	}

	movieID := discovery.FormatTMDBID(movie.TMDBID)
	metaStore := newMockMetadataStore()
	metaStore.movies[movieID] = &store.MovieDocument{
		ID:          movieID,
		TMDBID:      movie.TMDBID,
		Title:       movie.Title,
		LastUpdated: time.Now().Add(-1 * time.Hour), // Fresh (< 24h)
	}

	sumStore := newMockSummaryStore()
	sumStore.summaries[movieID] = &store.SummaryDocument{
		MovieID:          movieID,
		OverallSentiment: ptrInt(80),
		LastUpdated:      time.Now().Add(-1 * time.Hour),
	}

	disc := &mockDiscoverer{movies: []discovery.Movie{movie}}
	coll := &mockCollector{}
	llmCl := &mockLLMClient{}
	proc := processor.NewProcessor(3, 30)

	orc := pipeline.NewOrchestrator(disc, nil, []collector.Collector{coll}, proc, llmCl, metaStore, sumStore)

	res, err := orc.Run(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.MoviesSkipped != 1 {
		t.Errorf("expected 1 skipped movie, got %d", res.MoviesSkipped)
	}
	if res.MoviesProcessed != 0 {
		t.Errorf("expected 0 processed movies, got %d", res.MoviesProcessed)
	}
}

func TestOrchestrator_RunWithTarget_Success(t *testing.T) {
	// Target movie in DB1 catalog
	targetID := "tmdb_555"
	targetMovie := &store.MovieDocument{
		ID:          targetID,
		TMDBID:      555,
		Title:       "Targeted Movie",
		ReleaseDate: time.Now(),
		Overview:    "A targeted movie overview",
	}

	metaStore := newMockMetadataStore()
	metaStore.movies[targetID] = targetMovie

	sumStore := newMockSummaryStore()

	// Additional discovered movie to fill batch
	otherMovie := discovery.Movie{
		TMDBID:      666,
		Title:       "Batch Fill Movie",
		ReleaseDate: time.Now(),
	}

	disc := &mockDiscoverer{movies: []discovery.Movie{otherMovie}}
	coll := &mockCollector{reviews: []collector.Review{{ID: "r1", Content: "Great audience review!"}}}
	llmCl := &mockLLMClient{
		result: &llm.SummaryResponse{
			OverallSentiment:  88,
			AudienceConsensus: "Strong targeted summary",
			Pros:              []string{"Pacing"},
			Cons:              []string{"Ending"},
			CommonThemes:      []string{"Cinema"},
		},
	}
	proc := processor.NewProcessor(3, 30)

	orc := pipeline.NewOrchestrator(disc, nil, []collector.Collector{coll}, proc, llmCl, metaStore, sumStore)

	// Run on-demand for targetID with batch limit of 2
	res, err := orc.RunWithTarget(context.Background(), targetID, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should process targeted movie + 1 additional movie from discovery
	if res.MoviesProcessed != 2 {
		t.Errorf("expected 2 processed movies (target + batch fill), got %d", res.MoviesProcessed)
	}

	// Verify target movie summary was saved to DB2
	savedSum, found, _ := sumStore.GetSummary(context.Background(), targetID)
	if !found || savedSum == nil {
		t.Fatalf("expected summary for target movie %s in sumStore", targetID)
	}
	if savedSum.AudienceConsensus != "Strong targeted summary" {
		t.Errorf("expected 'Strong targeted summary', got '%s'", savedSum.AudienceConsensus)
	}
}

func TestOrchestrator_RunWithTarget_NotFound(t *testing.T) {
	metaStore := newMockMetadataStore()
	sumStore := newMockSummaryStore()
	disc := &mockDiscoverer{}
	coll := &mockCollector{}
	llmCl := &mockLLMClient{}
	proc := processor.NewProcessor(3, 30)

	orc := pipeline.NewOrchestrator(disc, nil, []collector.Collector{coll}, proc, llmCl, metaStore, sumStore)

	res, err := orc.RunWithTarget(context.Background(), "tmdb_nonexistent", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Errors) != 1 {
		t.Errorf("expected 1 error for non-existent target, got %d", len(res.Errors))
	}
}

func TestOrchestrator_CandidatePool_SkipFreshAndProcessRemaining(t *testing.T) {
	// 3 candidates discovered: first 2 are fresh with summaries, 3rd is new
	movie1 := discovery.Movie{TMDBID: 101, Title: "Fresh Movie 1"}
	movie2 := discovery.Movie{TMDBID: 102, Title: "Fresh Movie 2"}
	movie3 := discovery.Movie{TMDBID: 103, Title: "New Movie 3"}

	metaStore := newMockMetadataStore()
	sumStore := newMockSummaryStore()

	// Seed movie1 and movie2 as fresh (< 24h) with summaries
	for _, id := range []string{"tmdb_101", "tmdb_102"} {
		metaStore.movies[id] = &store.MovieDocument{
			ID:          id,
			LastUpdated: time.Now().Add(-1 * time.Hour),
		}
		sumStore.summaries[id] = &store.SummaryDocument{
			MovieID:          id,
			OverallSentiment: ptrInt(80),
			LastUpdated:      time.Now().Add(-1 * time.Hour),
		}
	}

	disc := &mockDiscoverer{movies: []discovery.Movie{movie1, movie2, movie3}}
	coll := &mockCollector{reviews: []collector.Review{{ID: "r1", Content: "Good movie"}}}
	llmCl := &mockLLMClient{
		result: &llm.SummaryResponse{
			OverallSentiment:  85,
			AudienceConsensus: "Positive consensus",
		},
	}
	proc := processor.NewProcessor(3, 30)

	orc := pipeline.NewOrchestrator(disc, nil, []collector.Collector{coll}, proc, llmCl, metaStore, sumStore)

	// Target processing limit of 1
	res, err := orc.Run(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.MoviesDiscovered != 3 {
		t.Errorf("expected 3 discovered candidates, got %d", res.MoviesDiscovered)
	}
	if res.MoviesSkipped != 2 {
		t.Errorf("expected 2 skipped movies, got %d", res.MoviesSkipped)
	}
	if res.MoviesProcessed != 1 {
		t.Errorf("expected 1 processed movie, got %d", res.MoviesProcessed)
	}

	// Verify movie 103 was processed and saved
	if doc, ok := metaStore.movies["tmdb_103"]; !ok || doc == nil {
		t.Errorf("expected tmdb_103 to be saved in metaStore")
	}
	if sum, ok := sumStore.summaries["tmdb_103"]; !ok || sum == nil {
		t.Errorf("expected tmdb_103 to have summary saved in sumStore")
	}
}


