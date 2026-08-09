package store

import (
	"context"
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/firestore"
	"review_aggregator/internal/discovery"
	"review_aggregator/internal/llm"
)

type MovieDocument struct {
	ID                  string              `firestore:"id"`
	TMDBID              int                 `firestore:"tmdb_id"`
	IMDbID              string              `firestore:"imdb_id,omitempty"`
	Title               string              `firestore:"title"`
	ReleaseDate         time.Time           `firestore:"release_date"`
	PosterURL           string              `firestore:"poster_url,omitempty"`
	Overview            string              `firestore:"overview,omitempty"`
	Genres              []string            `firestore:"genres,omitempty"`
	Scores              discovery.Scores    `firestore:"scores"`
	Summary             *llm.SummaryResponse `firestore:"summary,omitempty"`
	ReviewCountAnalyzed int                 `firestore:"review_count_analyzed"`
	LastUpdated         time.Time           `firestore:"last_updated"`
}

type Store interface {
	SaveMovie(ctx context.Context, doc *MovieDocument) error
	GetMovie(ctx context.Context, id string) (*MovieDocument, bool, error)
	Close() error
}

type FirestoreStore struct {
	client *firestore.Client
}

func NewFirestoreStore(ctx context.Context, projectID string) (*FirestoreStore, error) {
	if projectID == "" {
		log.Println("[WARN] FIREBASE_PROJECT_ID is empty. Operating Firestore store in no-op mock mode.")
		return &FirestoreStore{client: nil}, nil
	}

	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("initializing Firestore client: %w", err)
	}

	return &FirestoreStore{client: client}, nil
}

func (s *FirestoreStore) SaveMovie(ctx context.Context, doc *MovieDocument) error {
	if s.client == nil {
		log.Printf("[MOCK STORE] SaveMovie: %s (TMDB ID: %d)", doc.Title, doc.TMDBID)
		return nil
	}

	doc.LastUpdated = time.Now()
	_, err := s.client.Collection("movies").Doc(doc.ID).Set(ctx, doc)
	if err != nil {
		return fmt.Errorf("saving movie %s to Firestore: %w", doc.ID, err)
	}

	return nil
}

func (s *FirestoreStore) GetMovie(ctx context.Context, id string) (*MovieDocument, bool, error) {
	if s.client == nil {
		return nil, false, nil
	}

	docSnap, err := s.client.Collection("movies").Doc(id).Get(ctx)
	if err != nil {
		return nil, false, nil
	}

	var doc MovieDocument
	if err := docSnap.DataTo(&doc); err != nil {
		return nil, false, err
	}

	return &doc, true, nil
}

func (s *FirestoreStore) Close() error {
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}
