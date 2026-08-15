import {
  GenreRow,
  ListMoviesOptions,
  MovieResponse,
  MovieRow,
} from './types';

export function buildMovieMetadataResponse(
  movie: MovieRow,
  genres: string[] = []
): MovieResponse {
  const res: MovieResponse = {
    id: movie.id,
    tmdb_id: movie.tmdb_id,
    title: movie.title,
    release_date: movie.release_date,
    review_count_analyzed: 0,
    last_updated: movie.last_updated,
    scores: {},
  };

  if (movie.imdb_id) res.imdb_id = movie.imdb_id;
  if (movie.poster_url) res.poster_url = movie.poster_url;
  if (movie.overview) res.overview = movie.overview;
  if (genres.length > 0) res.genres = genres;

  if (movie.imdb_score !== null && movie.imdb_score !== undefined) {
    res.scores.imdb = movie.imdb_score.toString();
  }
  if (movie.rotten_tomatoes !== null && movie.rotten_tomatoes !== undefined) {
    res.scores.rotten_tomatoes = `${movie.rotten_tomatoes}%`;
  }

  return res;
}

export async function getMovieById(db: D1Database, id: string): Promise<MovieResponse | null> {
  const [movieRes, genresRes] = await db.batch<any>([
    db.prepare('SELECT * FROM movies WHERE id = ? LIMIT 1').bind(id),
    db.prepare('SELECT genre FROM movie_genres WHERE movie_id = ?').bind(id),
  ]);

  const movieRows = (movieRes.results as MovieRow[]) || [];
  if (movieRows.length === 0) {
    return null;
  }

  const movie = movieRows[0];
  const genres = ((genresRes.results as { genre: string }[]) || []).map((g) => g.genre);

  return buildMovieMetadataResponse(movie, genres);
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
  return populateGenres(db, movies);
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
  const results = await populateGenres(db, movies);

  return { total, limit, offset, results };
}

async function populateGenres(db: D1Database, movies: MovieRow[]): Promise<MovieResponse[]> {
  if (movies.length === 0) {
    return [];
  }

  const movieIds = movies.map((m) => m.id);
  const placeholders = movieIds.map(() => '?').join(',');

  const genresRes = await db
    .prepare(`SELECT movie_id, genre FROM movie_genres WHERE movie_id IN (${placeholders})`)
    .bind(...movieIds)
    .all<GenreRow>();

  const genresByMovie = new Map<string, string[]>();
  for (const g of genresRes.results || []) {
    const list = genresByMovie.get(g.movie_id) || [];
    list.push(g.genre);
    genresByMovie.set(g.movie_id, list);
  }

  return movies.map((m) =>
    buildMovieMetadataResponse(m, genresByMovie.get(m.id) || [])
  );
}
