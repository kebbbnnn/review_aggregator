-- Cloudflare D1 Database Schema for Movie Review Aggregator

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
    overall_sentiment INTEGER,            -- 0-100 score
    audience_consensus TEXT,
    recommendation    TEXT,
    review_count      INTEGER DEFAULT 0,
    last_updated      TEXT NOT NULL        -- ISO 8601 string
);

CREATE TABLE IF NOT EXISTS movie_genres (
    movie_id TEXT NOT NULL REFERENCES movies(id) ON DELETE CASCADE,
    genre    TEXT NOT NULL,
    PRIMARY KEY (movie_id, genre)
);

CREATE TABLE IF NOT EXISTS movie_points (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    movie_id TEXT NOT NULL REFERENCES movies(id) ON DELETE CASCADE,
    type     TEXT NOT NULL CHECK(type IN ('pro', 'con')),
    content  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS movie_themes (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    movie_id TEXT NOT NULL REFERENCES movies(id) ON DELETE CASCADE,
    theme    TEXT NOT NULL
);

-- Performance Indexes
CREATE INDEX IF NOT EXISTS idx_movies_tmdb_id ON movies(tmdb_id);
CREATE INDEX IF NOT EXISTS idx_movies_title ON movies(title);
CREATE INDEX IF NOT EXISTS idx_movies_imdb_score ON movies(imdb_score DESC);
CREATE INDEX IF NOT EXISTS idx_movies_rotten_tomatoes ON movies(rotten_tomatoes DESC);
CREATE INDEX IF NOT EXISTS idx_movies_overall_sentiment ON movies(overall_sentiment DESC);
CREATE INDEX IF NOT EXISTS idx_movie_genres_genre ON movie_genres(genre);
CREATE INDEX IF NOT EXISTS idx_movie_points_movie_id ON movie_points(movie_id);
CREATE INDEX IF NOT EXISTS idx_movie_themes_movie_id ON movie_themes(movie_id);
