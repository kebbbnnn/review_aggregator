import { getMovieById, listMovies, searchMovies } from './db';
import { Env, ListMoviesOptions } from './types';

const CORS_HEADERS: Record<string, string> = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, HEAD, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type, Authorization',
  'Content-Type': 'application/json',
};

function jsonResponse(data: any, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: CORS_HEADERS,
  });
}

function errorResponse(message: string, status = 400): Response {
  return jsonResponse({ error: message }, status);
}

export default {
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    if (request.method === 'OPTIONS') {
      return new Response(null, { headers: CORS_HEADERS });
    }

    if (request.method !== 'GET' && request.method !== 'HEAD') {
      return errorResponse('Method Not Allowed', 405);
    }

    const url = new URL(request.url);
    const pathname = url.pathname.replace(/\/+$/, ''); // Strip trailing slash

    try {
      // 1. Health check
      if (pathname === '/healthz' || pathname === '') {
        return jsonResponse({ status: 'ok', service: 'review-aggregator-api' });
      }

      // 2. Search movies by title (/api/v1/movies/search)
      if (pathname === '/api/v1/movies/search') {
        const q = url.searchParams.get('q') || url.searchParams.get('query') || '';
        if (!q.trim()) {
          return errorResponse("Query parameter 'q' or 'query' is required", 400);
        }

        const limitStr = url.searchParams.get('limit');
        const limit = limitStr ? parseInt(limitStr, 10) : 10;
        const safeLimit = isNaN(limit) || limit <= 0 ? 10 : Math.min(limit, 50);

        const results = await searchMovies(env.DB, q.trim(), safeLimit);
        return jsonResponse({
          query: q.trim(),
          total: results.length,
          results,
        });
      }

      // 3. Get single movie by ID (/api/v1/movies/:id)
      const movieMatch = pathname.match(/^\/api\/v1\/movies\/([^\/]+)$/);
      if (movieMatch) {
        const id = decodeURIComponent(movieMatch[1]);
        const movie = await getMovieById(env.DB, id);
        if (!movie) {
          return errorResponse(`Movie with ID '${id}' not found`, 404);
        }
        return jsonResponse({ result: movie });
      }

      // 4. Browse / filter movies (/api/v1/movies)
      if (pathname === '/api/v1/movies') {
        const genre = url.searchParams.get('genre') || undefined;
        const sort = (url.searchParams.get('sort') as any) || undefined;
        const order = (url.searchParams.get('order') as any) || undefined;
        const limitStr = url.searchParams.get('limit');
        const offsetStr = url.searchParams.get('offset');

        const options: ListMoviesOptions = {
          genre,
          sort,
          order,
          limit: limitStr ? parseInt(limitStr, 10) : 20,
          offset: offsetStr ? parseInt(offsetStr, 10) : 0,
        };

        const result = await listMovies(env.DB, options);
        return jsonResponse(result);
      }

      return errorResponse('Not Found', 404);
    } catch (err: any) {
      console.error('Unhandled worker error:', err);
      return errorResponse(err?.message || 'Internal Server Error', 500);
    }
  },
};
