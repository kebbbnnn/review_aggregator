import { getMovieById, listMovies, searchMovies, getReviewRequest, upsertReviewRequest, movieExistsInCatalog } from './db';
import { fetchSummariesBatch, fetchSummary } from './summary-client';
import { Env, ListMoviesOptions } from './types';

const CORS_HEADERS: Record<string, string> = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, HEAD, POST, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type, Authorization',
  'Content-Type': 'application/json',
};

const REVIEW_COOLDOWN_HOURS = 6;

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

    const url = new URL(request.url);
    const pathname = url.pathname.replace(/\/+$/, ''); // Strip trailing slash

    // Handle POST routes
    if (request.method === 'POST') {
      const reviewMatch = pathname.match(/^\/api\/v1\/movies\/([^\/]+)\/request-review$/);
      if (reviewMatch) {
        return handleRequestReview(decodeURIComponent(reviewMatch[1]), env);
      }
      return errorResponse('Not Found', 404);
    }

    if (request.method !== 'GET' && request.method !== 'HEAD') {
      return errorResponse('Method Not Allowed', 405);
    }

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
        
        // Enrich with summaries from DB2 (via edge cache)
        if (results.length > 0) {
          const ids = results.map((m) => m.id);
          const summaryMap = await fetchSummariesBatch(ids, env);
          for (const movie of results) {
            const summaryData = summaryMap.get(movie.id);
            if (summaryData) {
              if (summaryData.summary) {
                movie.summary = summaryData.summary;
              }
              movie.review_count_analyzed = summaryData.review_count;
            }
          }
        }

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

        // Enrich with summary from DB2 (via edge cache)
        const summaryData = await fetchSummary(id, env);
        if (summaryData) {
          if (summaryData.summary) {
            movie.summary = summaryData.summary;
          }
          movie.review_count_analyzed = summaryData.review_count;
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

        const listResult = await listMovies(env.DB, options);

        // Enrich with summaries from DB2 (via edge cache)
        if (listResult.results.length > 0) {
          const ids = listResult.results.map((m) => m.id);
          const summaryMap = await fetchSummariesBatch(ids, env);
          for (const movie of listResult.results) {
            const summaryData = summaryMap.get(movie.id);
            if (summaryData) {
              if (summaryData.summary) {
                movie.summary = summaryData.summary;
              }
              movie.review_count_analyzed = summaryData.review_count;
            }
          }
        }

        return jsonResponse(listResult);
      }

      return errorResponse('Not Found', 404);
    } catch (err: any) {
      console.error('Unhandled worker error:', err);
      return errorResponse(err?.message || 'Internal Server Error', 500);
    }
  },
};

async function handleRequestReview(movieId: string, env: Env): Promise<Response> {
  // 1. Validate required secrets
  if (!env.GITHUB_TOKEN || !env.GITHUB_REPO) {
    return errorResponse('On-demand review is not configured on this server', 503);
  }

  // 2. Validate movie exists in catalog
  const exists = await movieExistsInCatalog(env.DB, movieId);
  if (!exists) {
    return errorResponse(`Movie '${movieId}' not found in catalog`, 404);
  }

  // 3. Check cooldown
  const existing = await getReviewRequest(env.DB, movieId);
  if (existing) {
    const requestedAt = new Date(existing.requested_at);
    const hoursElapsed = (Date.now() - requestedAt.getTime()) / (1000 * 60 * 60);
    if (hoursElapsed < REVIEW_COOLDOWN_HOURS) {
      const retryAfter = Math.round((REVIEW_COOLDOWN_HOURS - hoursElapsed) * 10) / 10;
      return jsonResponse(
        { status: 'cooldown', movie_id: movieId, retry_after_hours: retryAfter },
        429
      );
    }
  }

  // 4. Trigger GitHub Actions workflow_dispatch
  const [owner, repo] = env.GITHUB_REPO.split('/');
  const ghRes = await fetch(
    `https://api.github.com/repos/${owner}/${repo}/actions/workflows/on_demand_review.yml/dispatches`,
    {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${env.GITHUB_TOKEN}`,
        Accept: 'application/vnd.github.v3+json',
        'User-Agent': 'review-aggregator-worker',
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        ref: 'main',
        inputs: { movie_id: movieId },
      }),
    }
  );

  if (!ghRes.ok) {
    const errBody = await ghRes.text();
    console.error(`GitHub API error: ${ghRes.status} ${errBody}`);
    return errorResponse('Failed to trigger review processing', 502);
  }

  // 5. Record the request
  await upsertReviewRequest(env.DB, movieId);

  return jsonResponse({ status: 'queued', movie_id: movieId }, 202);
}
