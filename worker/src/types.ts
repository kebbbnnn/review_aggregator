export interface Env {
  DB: D1Database;
  SUMMARY_WORKER_URL?: string;
  SUMMARY_API_KEY?: string;
  GITHUB_TOKEN?: string;
  GITHUB_REPO?: string;
}

export interface MovieRow {
  id: string;
  tmdb_id: number;
  imdb_id: string | null;
  title: string;
  release_date: string;
  poster_url: string | null;
  overview: string | null;
  imdb_score: number | null;
  rotten_tomatoes: number | null;
  last_updated: string;
}

export interface GenreRow {
  movie_id: string;
  genre: string;
}

export interface MovieScores {
  imdb?: string;
  rotten_tomatoes?: string;
}

export interface MovieSummary {
  overall_sentiment?: number;
  pros?: string[];
  cons?: string[];
  common_themes?: string[];
  audience_consensus?: string;
  recommendation?: string;
}

export interface MovieResponse {
  id: string;
  tmdb_id: number;
  imdb_id?: string;
  title: string;
  release_date: string;
  poster_url?: string;
  overview?: string;
  genres?: string[];
  scores: MovieScores;
  summary?: MovieSummary;
  review_count_analyzed: number;
  last_updated: string;
}

export interface ListMoviesOptions {
  genre?: string;
  sort?: 'release_date' | 'imdb_score' | 'rotten_tomatoes' | 'last_updated';
  order?: 'asc' | 'desc';
  limit?: number;
  offset?: number;
}
