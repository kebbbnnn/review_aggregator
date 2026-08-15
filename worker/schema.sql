-- Cloudflare D1 Database Schema for Movie Review Aggregator (DB1: Metadata & Catalog)

CREATE TABLE IF NOT EXISTS movies (
    id                TEXT PRIMARY KEY,   -- Format: "tmdb_12345"
    tmdb_id           INTEGER NOT NULL UNIQUE,
    imdb_id           TEXT,
    title             TEXT NOT NULL,
    release_date      TEXT NOT NULL,       -- ISO 8601 string (e.g. 2025-07-11T00:00:00Z)
    poster_url        TEXT,
    overview          TEXT,
    imdb_score        REAL,               -- Numeric (e.g. 7.8)
    rotten_tomatoes   INTEGER,            -- Percentage integer (e.g. 84)
    last_updated      TEXT NOT NULL        -- ISO 8601 string
);

CREATE TABLE IF NOT EXISTS movie_genres (
    movie_id TEXT NOT NULL REFERENCES movies(id) ON DELETE CASCADE,
    genre    TEXT NOT NULL,
    PRIMARY KEY (movie_id, genre)
);

-- Performance Indexes
CREATE INDEX IF NOT EXISTS idx_movies_tmdb_id ON movies(tmdb_id);
CREATE INDEX IF NOT EXISTS idx_movies_title ON movies(title);
CREATE INDEX IF NOT EXISTS idx_movies_imdb_score ON movies(imdb_score DESC);
CREATE INDEX IF NOT EXISTS idx_movies_rotten_tomatoes ON movies(rotten_tomatoes DESC);
CREATE INDEX IF NOT EXISTS idx_movies_last_updated ON movies(last_updated DESC);
CREATE INDEX IF NOT EXISTS idx_movie_genres_genre ON movie_genres(genre);

