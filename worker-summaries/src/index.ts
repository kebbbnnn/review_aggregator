import { getSummariesBatch, getSummaryById } from './db';
import { Env } from './types';

const CORS_HEADERS: Record<string, string> = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, POST, HEAD, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type, Authorization, X-Api-Key',
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

function checkAuth(request: Request, env: Env): boolean {
  if (!env.SUMMARY_API_KEY) {
    return true; // No key configured, allow in dev/open mode
  }
  const authHeader = request.headers.get('X-Api-Key') || request.headers.get('Authorization');
  if (!authHeader) {
    return false;
  }
  const cleanHeader = authHeader.replace(/^Bearer\s+/i, '').trim();
  return cleanHeader === env.SUMMARY_API_KEY.trim();
}

export default {
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    if (request.method === 'OPTIONS') {
      return new Response(null, { headers: CORS_HEADERS });
    }

    const url = new URL(request.url);
    const pathname = url.pathname.replace(/\/+$/, ''); // Strip trailing slash

    try {
      // 1. Health check (public)
      if (pathname === '/healthz' || pathname === '') {
        return jsonResponse({ status: 'ok', service: 'movie-summaries-api' });
      }

      // Authenticate protected endpoints
      if (!checkAuth(request, env)) {
        return errorResponse('Unauthorized: Invalid or missing API key', 401);
      }

      // 2. Batch summaries lookup: POST /summaries/batch
      if (pathname === '/summaries/batch') {
        if (request.method !== 'POST') {
          return errorResponse('Method Not Allowed. Use POST for batch queries.', 405);
        }

        let body: any;
        try {
          body = await request.json();
        } catch {
          return errorResponse('Invalid JSON body', 400);
        }

        const ids: string[] = Array.isArray(body?.ids) ? body.ids : [];
        if (ids.length === 0) {
          return jsonResponse({ results: {} });
        }

        // Limit batch size to 100
        const safeIds = ids.slice(0, 100);
        const results = await getSummariesBatch(env.DB, safeIds);
        return jsonResponse({ results });
      }

      // 3. Single summary lookup: GET /summaries/:movieId
      const summaryMatch = pathname.match(/^\/summaries\/([^\/]+)$/);
      if (summaryMatch) {
        if (request.method !== 'GET' && request.method !== 'HEAD') {
          return errorResponse('Method Not Allowed', 405);
        }

        const movieId = decodeURIComponent(summaryMatch[1]);
        const summary = await getSummaryById(env.DB, movieId);
        if (!summary) {
          return errorResponse(`Summary for movie ID '${movieId}' not found`, 404);
        }
        return jsonResponse({ result: summary });
      }

      return errorResponse('Not Found', 404);
    } catch (err: any) {
      console.error('Unhandled summary worker error:', err);
      return errorResponse(err?.message || 'Internal Server Error', 500);
    }
  },
};
