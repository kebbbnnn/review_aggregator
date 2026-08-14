import {
  GenreRow,
  ListMoviesOptions,
  MovieResponse,
  MovieRow,
  PointRow,
  ThemeRow,
} from './types';

export function buildMovieResponse(
  movie: MovieRow,
  genres: string[] = [],
  points: { type: 'pro' | 'con'; content: string }[] = [],
  themes: string[] = []
): MovieResponse {
  const pros = points.filter((p) => p.type === 'pro').map((p) => p.content);
  const cons = points.filter((p) => p.type === 'con').map((p) => p.content);

  const hasSummary =
    movie.overall_sentiment !== null ||
    (movie.audience_consensus && movie.audience_consensus !== '') ||
    (movie.recommendation && movie.recommendation !== '') ||
    pros.length > 0 ||
    cons.length > 0 ||
    themes.length > 0;

  const res: MovieResponse = {
    id: movie.id,
    tmdb_id: movie.tmdb_id,
    title: movie.title,
    release_date: movie.release_date,
    review_count_analyzed: movie.review_count,
    last_updated: movie.last_updated,
    scores: {},
  };

  if (movie.imdb_id) res.imdb_id = movie.imdb_id;
  if (movie.poster_url) res.poster_url = movie.poster_url;
  if (movie.overview) res.overview = movie.overview;
  if (genres.length > 0) res.genres = genres;

  if (movie.imdb_score !== null) {
    res.scores.imdb = movie.imdb_score.toString();
  }
  if (movie.rotten_tomatoes !== null) {
    res.scores.rotten_tomatoes = `${movie.rotten_tomatoes}%`;
  }

  if (hasSummary) {
    res.summary = {
      pros,
      cons,
      common_themes: themes,
    };
    if (movie.overall_sentiment !== null) {
      res.summary.overall_sentiment = movie.overall_sentiment;
    }
    if (movie.audience_consensus) {
      res.summary.audience_consensus = movie.audience_consensus;
    }
    if (movie.recommendation) {
      res.summary.recommendation = movie.recommendation;
    }
  }

  return res;
}

export async function getMovieById(db: D1Database, id: string): Promise<MovieResponse | null> {
  const [movieRes, genresRes, pointsRes, themesRes] = await db.batch<any>([
    db.prepare('SELECT * FROM movies WHERE id = ? LIMIT 1').bind(id),
    db.prepare('SELECT genre FROM movie_genres WHERE movie_id = ?').bind(id),
    db.prepare('SELECT type, content FROM movie_points WHERE movie_id = ?').bind(id),
    db.prepare('SELECT theme FROM movie_themes WHERE movie_id = ?').bind(id),
  ]);

  const movieRows = (movieRes.results as MovieRow[]) || [];
  if (movieRows.length === 0) {
    return null;
  }

  const movie = movieRows[0];
  const genres = ((genresRes.results as { genre: string }[]) || []).map((g) => g.genre);
  const points = (pointsRes.results as { type: 'pro' | 'con'; content: string }[]) || [];
  const themes = ((themesRes.results as { theme: string }[]) || []).map((t) => t.theme);

  return buildMovieResponse(movie, genres, points, themes);
}

export async function searchMovies(
  db: D1Database,
  query: string,
  limit: number = 10
): Promise<MovieResponse[]> {
  const movieRes = await db
    .prepare('SELECT * FROM movies WHERE title LIKE ? ORDER BY last_updated DESC LIMIT ?')
    .bind(`%${query}%`, limit)
    .all<MovieRow>();

  const movies = movieRes.results || [];
  return populateRelations(db, movies);
}

export async function listMovies(
  db: D1Database,
  options: ListMoviesOptions = {}
): Promise<{ total: number; limit: number; offset: number; results: MovieResponse[] }> {
  const limit = Math.min(Math.max(options.limit || 20, 1), 100);
  const offset = Math.max(options.offset || 0, 0);

  const allowedSorts = [
    'release_date',
    'imdb_score',
    'rotten_tomatoes',
    'overall_sentiment',
    'last_updated',
  ];
  const sortCol = allowedSorts.includes(options.sort || '') ? options.sort! : 'last_updated';
  const sortOrder = (options.order || 'desc').toLowerCase() === 'asc' ? 'ASC' : 'DESC';

  let countSql = 'SELECT COUNT(DISTINCT m.id) as count FROM movies m';
  let selectSql = `SELECT DISTINCT m.* FROM movies m`;
  const params: any[] = [];

  if (options.genre) {
    const joinClause = ' INNER JOIN movie_genres g ON m.id = g.movie_id WHERE g.genre = ?';
    countSql += joinClause;
    selectSql += joinClause;
    params.push(options.genre);
  }

  selectSql += ` ORDER BY m.${sortCol} ${sortOrder} LIMIT ? OFFSET ?`;

  const countRes = await db
    .prepare(countSql)
    .bind(...params)
    .first<{ count: number }>();
  const total = countRes ? countRes.count : 0;

  const movieRes = await db
    .prepare(selectSql)
    .bind(...params, limit, offset)
    .all<MovieRow>();

  const movies = movieRes.results || [];
  const results = await populateRelations(db, movies);

  return { total, limit, offset, results };
}

async function populateRelations(db: D1Database, movies: MovieRow[]): Promise<MovieResponse[]> {
  if (movies.length === 0) {
    return [];
  }

  const movieIds = movies.map((m) => m.id);
  const placeholders = movieIds.map(() => '?').join(',');

  const [genresRes, pointsRes, themesRes] = await db.batch<any>([
    db
      .prepare(`SELECT movie_id, genre FROM movie_genres WHERE movie_id IN (${placeholders})`)
      .bind(...movieIds),
    db
      .prepare(
        `SELECT movie_id, type, content FROM movie_points WHERE movie_id IN (${placeholders})`
      )
      .bind(...movieIds),
    db
      .prepare(`SELECT movie_id, theme FROM movie_themes WHERE movie_id IN (${placeholders})`)
      .bind(...movieIds),
  ]);

  const genresByMovie = new Map<string, string[]>();
  for (const g of (genresRes.results as GenreRow[]) || []) {
    const list = genresByMovie.get(g.movie_id) || [];
    list.push(g.genre);
    genresByMovie.set(g.movie_id, list);
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

  return movies.map((m) =>
    buildMovieResponse(
      m,
      genresByMovie.get(m.id) || [],
      pointsByMovie.get(m.id) || [],
      themesByMovie.get(m.id) || []
    )
  );
}
