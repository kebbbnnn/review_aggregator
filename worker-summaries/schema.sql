-- Cloudflare D1 Database Schema for Movie Review Aggregator (DB2: LLM Summaries)

CREATE TABLE IF NOT EXISTS movie_summaries (
    movie_id            TEXT PRIMARY KEY,   -- Matches DB1 id, e.g. "tmdb_12345"
    overall_sentiment   INTEGER,            -- 0-100 score
    audience_consensus  TEXT,
    recommendation      TEXT,
    review_count        INTEGER DEFAULT 0,
    last_updated        TEXT NOT NULL        -- ISO 8601 string
);

CREATE TABLE IF NOT EXISTS movie_points (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    movie_id TEXT NOT NULL REFERENCES movie_summaries(movie_id) ON DELETE CASCADE,
    type     TEXT NOT NULL CHECK(type IN ('pro', 'con')),
    content  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS movie_themes (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    movie_id TEXT NOT NULL REFERENCES movie_summaries(movie_id) ON DELETE CASCADE,
    theme    TEXT NOT NULL
);

-- Performance Indexes
CREATE INDEX IF NOT EXISTS idx_movie_summaries_sentiment ON movie_summaries(overall_sentiment DESC);
CREATE INDEX IF NOT EXISTS idx_movie_summaries_last_updated ON movie_summaries(last_updated DESC);
CREATE INDEX IF NOT EXISTS idx_movie_points_movie_id ON movie_points(movie_id);
CREATE INDEX IF NOT EXISTS idx_movie_themes_movie_id ON movie_themes(movie_id);
