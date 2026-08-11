package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchMovies_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search/movie" {
			q := r.URL.Query().Get("query")
			if q != "batman" {
				t.Errorf("expected query 'batman', got '%s'", q)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"results": [
					{
						"id": 268,
						"title": "Batman",
						"release_date": "1989-06-23",
						"poster_path": "/2bX2EKZvR22yOvhOKQq975Vc2Wv.jpg",
						"overview": "The Dark Knight of Gotham City..."
					}
				]
			}`))
			return
		}

		if r.URL.Path == "/movie/268" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"imdb_id": "tt0096895",
				"genres": [{"name": "Action"}, {"name": "Fantasy"}]
			}`))
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewTMDBClient("test_key")
	client.baseURL = server.URL

	movies, err := client.SearchMovies(context.Background(), "batman", 5)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(movies) != 1 {
		t.Fatalf("expected 1 movie, got %d", len(movies))
	}

	m := movies[0]
	if m.TMDBID != 268 {
		t.Errorf("expected TMDB ID 268, got %d", m.TMDBID)
	}
	if m.Title != "Batman" {
		t.Errorf("expected title 'Batman', got '%s'", m.Title)
	}
	if m.IMDbID != "tt0096895" {
		t.Errorf("expected IMDb ID 'tt0096895', got '%s'", m.IMDbID)
	}
	if len(m.Genres) != 2 || m.Genres[0] != "Action" {
		t.Errorf("unexpected genres: %v", m.Genres)
	}
}

func TestSearchMovies_EmptyQuery(t *testing.T) {
	client := NewTMDBClient("test_key")
	_, err := client.SearchMovies(context.Background(), "", 5)
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}
}

func TestSearchMovies_MissingAPIKey(t *testing.T) {
	client := NewTMDBClient("")
	_, err := client.SearchMovies(context.Background(), "batman", 5)
	if err == nil {
		t.Fatal("expected error for missing API key, got nil")
	}
}
