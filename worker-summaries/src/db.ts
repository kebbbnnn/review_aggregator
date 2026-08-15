import {
  MovieSummaryResponse,
  PointRow,
  SummaryRow,
  ThemeRow,
} from './types';

export function buildSummaryResponse(
  summary: SummaryRow,
  points: { type: 'pro' | 'con'; content: string }[] = [],
  themes: string[] = []
): MovieSummaryResponse {
  const pros = points.filter((p) => p.type === 'pro').map((p) => p.content);
  const cons = points.filter((p) => p.type === 'con').map((p) => p.content);

  const res: MovieSummaryResponse = {
    movie_id: summary.movie_id,
    review_count: summary.review_count,
    last_updated: summary.last_updated,
  };

  if (summary.overall_sentiment !== null && summary.overall_sentiment !== undefined) {
    res.overall_sentiment = summary.overall_sentiment;
  }
  if (summary.audience_consensus) {
    res.audience_consensus = summary.audience_consensus;
  }
  if (summary.recommendation) {
    res.recommendation = summary.recommendation;
  }
  if (pros.length > 0) {
    res.pros = pros;
  }
  if (cons.length > 0) {
    res.cons = cons;
  }
  if (themes.length > 0) {
    res.themes = themes;
  }

  return res;
}

export async function getSummaryById(
  db: D1Database,
  movieId: string
): Promise<MovieSummaryResponse | null> {
  const [summaryRes, pointsRes, themesRes] = await db.batch<any>([
    db.prepare('SELECT * FROM movie_summaries WHERE movie_id = ? LIMIT 1').bind(movieId),
    db.prepare('SELECT type, content FROM movie_points WHERE movie_id = ?').bind(movieId),
    db.prepare('SELECT theme FROM movie_themes WHERE movie_id = ?').bind(movieId),
  ]);

  const summaryRows = (summaryRes.results as SummaryRow[]) || [];
  if (summaryRows.length === 0) {
    return null;
  }

  const summary = summaryRows[0];
  const points = (pointsRes.results as { type: 'pro' | 'con'; content: string }[]) || [];
  const themes = ((themesRes.results as { theme: string }[]) || []).map((t) => t.theme);

  return buildSummaryResponse(summary, points, themes);
}

export async function getSummariesBatch(
  db: D1Database,
  movieIds: string[]
): Promise<Record<string, MovieSummaryResponse>> {
  const validIds = Array.from(new Set(movieIds.filter((id) => Boolean(id && id.trim()))));
  if (validIds.length === 0) {
    return {};
  }

  const placeholders = validIds.map(() => '?').join(',');

  const [summariesRes, pointsRes, themesRes] = await db.batch<any>([
    db
      .prepare(`SELECT * FROM movie_summaries WHERE movie_id IN (${placeholders})`)
      .bind(...validIds),
    db
      .prepare(
        `SELECT movie_id, type, content FROM movie_points WHERE movie_id IN (${placeholders})`
      )
      .bind(...validIds),
    db
      .prepare(`SELECT movie_id, theme FROM movie_themes WHERE movie_id IN (${placeholders})`)
      .bind(...validIds),
  ]);

  const summaries = (summariesRes.results as SummaryRow[]) || [];
  if (summaries.length === 0) {
    return {};
  }

  const pointsByMovie = new Map<string, { type: 'pro' | 'con'; content: string }[]>();
  for (const p of (pointsRes.results as PointRow[]) || []) {
    const list = pointsByMovie.get(p.movie_id) || [];
    list.push({ type: p.type, content: p.content });
    pointsByMovie.set(p.movie_id, list);
  }

  const themesByMovie = new Map<string, string[]>();
  for (const t of (themesRes.results as ThemeRow[]) || []) {
    const list = themesByMovie.get(t.movie_id) || [];
    list.push(t.theme);
    themesByMovie.set(t.movie_id, list);
  }

  const result: Record<string, MovieSummaryResponse> = {};
  for (const summary of summaries) {
    result[summary.movie_id] = buildSummaryResponse(
      summary,
      pointsByMovie.get(summary.movie_id) || [],
      themesByMovie.get(summary.movie_id) || []
    );
  }

  return result;
}
