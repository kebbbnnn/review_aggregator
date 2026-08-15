export interface Env {
  DB: D1Database;
  SUMMARY_API_KEY?: string;
}

export interface SummaryRow {
  movie_id: string;
  overall_sentiment: number | null;
  audience_consensus: string | null;
  recommendation: string | null;
  review_count: number;
  last_updated: string;
}

export interface PointRow {
  id: number;
  movie_id: string;
  type: 'pro' | 'con';
  content: string;
}

export interface ThemeRow {
  id: number;
  movie_id: string;
  theme: string;
}

export interface MovieSummaryResponse {
  movie_id: string;
  overall_sentiment?: number;
  audience_consensus?: string;
  recommendation?: string;
  review_count: number;
  pros?: string[];
  cons?: string[];
  themes?: string[];
  last_updated: string;
}
