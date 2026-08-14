package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestFetchGenreMap_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/genre/movie/list" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"genres": [
					{"id": 28, "name": "Action"},
					{"id": 12, "name": "Adventure"},
					{"id": 878, "name": "Science Fiction"}
				]
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewTMDBClient("test_key")
	client.baseURL = server.URL

	genreMap, err := client.FetchGenreMap(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(genreMap) != 3 {
		t.Fatalf("expected 3 genres, got %d", len(genreMap))
	}
	if genreMap[28] != "Action" || genreMap[878] != "Science Fiction" {
		t.Errorf("unexpected genre map values: %v", genreMap)
	}
}

func TestDiscoverAll_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/discover/movie" {
			page := r.URL.Query().Get("page")
			sortBy := r.URL.Query().Get("sort_by")
			if page != "2" || sortBy != "primary_release_date.desc" {
				t.Errorf("unexpected query params: page=%s, sort_by=%s", page, sortBy)
			}

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"page": 2,
				"total_pages": 500,
				"total_results": 10000,
				"results": [
					{
						"id": 101,
						"title": "Movie 101",
						"release_date": "2026-08-01",
						"poster_path": "/path101.jpg",
						"overview": "Overview 101",
						"genre_ids": [28, 12],
						"popularity": 75.5
					},
					{
						"id": 102,
						"title": "Movie 102",
						"release_date": "2026-07-15",
						"poster_path": "",
						"overview": "Overview 102",
						"genre_ids": [878],
						"popularity": 42.0
					}
				]
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewTMDBClient("test_key")
	client.baseURL = server.URL

	movies, totalPages, err := client.DiscoverAll(context.Background(), 2, "primary_release_date.desc")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if totalPages != 500 {
		t.Errorf("expected totalPages 500, got %d", totalPages)
	}
	if len(movies) != 2 {
		t.Fatalf("expected 2 movies, got %d", len(movies))
	}

	if movies[0].TMDBID != 101 || movies[0].Title != "Movie 101" || movies[0].Popularity != 75.5 {
		t.Errorf("unexpected movie 0: %+v", movies[0])
	}
	if len(movies[0].GenreIDs) != 2 || movies[0].GenreIDs[0] != 28 {
		t.Errorf("unexpected genre IDs for movie 0: %v", movies[0].GenreIDs)
	}
	if movies[0].PosterURL != "https://image.tmdb.org/t/p/w500/path101.jpg" {
		t.Errorf("unexpected poster URL: %s", movies[0].PosterURL)
	}
	if movies[1].PosterURL != "" {
		t.Errorf("expected empty poster URL, got %s", movies[1].PosterURL)
	}
}

func TestDiscoverPopularRecent_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/discover/movie" {
			popGte := r.URL.Query().Get("popularity.gte")
			dateGte := r.URL.Query().Get("primary_release_date.gte")
			if popGte != "50.00" || dateGte != "2026-02-14" {
				t.Errorf("unexpected query params: popularity.gte=%s, primary_release_date.gte=%s", popGte, dateGte)
			}

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"page": 1,
				"total_pages": 10,
				"results": [
					{
						"id": 201,
						"title": "Blockbuster",
						"release_date": "2026-05-20",
						"poster_path": "/blockbuster.jpg",
						"overview": "Big film",
						"genre_ids": [28],
						"popularity": 120.0
					}
				]
			}`))
			return
		}

		if r.URL.Path == "/movie/201" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"imdb_id": "tt201201",
				"genres": [{"name": "Action"}]
			}`))
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewTMDBClient("test_key")
	client.baseURL = server.URL

	releasedAfter := time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC)
	movies, err := client.DiscoverPopularRecent(context.Background(), 50.0, releasedAfter, 5)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(movies) != 1 {
		t.Fatalf("expected 1 movie, got %d", len(movies))
	}
	if movies[0].TMDBID != 201 || movies[0].IMDbID != "tt201201" || len(movies[0].Genres) != 1 || movies[0].Genres[0] != "Action" {
		t.Errorf("unexpected movie details: %+v", movies[0])
	}
}

