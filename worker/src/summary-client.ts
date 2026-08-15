import { Env, MovieSummary } from './types';

export interface SummaryResult {
  summary?: MovieSummary;
  review_count: number;
}

interface RawSummaryResponse {
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

function parseRawSummary(raw: RawSummaryResponse): SummaryResult {
  const hasContent =
    raw.overall_sentiment !== undefined ||
    Boolean(raw.audience_consensus) ||
    Boolean(raw.recommendation) ||
    (raw.pros && raw.pros.length > 0) ||
    (raw.cons && raw.cons.length > 0) ||
    (raw.themes && raw.themes.length > 0);

  if (!hasContent) {
    return { review_count: raw.review_count || 0 };
  }

  const summary: MovieSummary = {
    pros: raw.pros || [],
    cons: raw.cons || [],
    common_themes: raw.themes || [],
  };

  if (raw.overall_sentiment !== undefined && raw.overall_sentiment !== null) {
    summary.overall_sentiment = raw.overall_sentiment;
  }
  if (raw.audience_consensus) {
    summary.audience_consensus = raw.audience_consensus;
  }
  if (raw.recommendation) {
    summary.recommendation = raw.recommendation;
  }

  return {
    summary,
    review_count: raw.review_count || 0,
  };
}

export async function fetchSummary(
  movieId: string,
  env: Env
): Promise<SummaryResult | null> {
  if (!env.SUMMARY_WORKER_URL) {
    return null;
  }

  const cacheKey = new Request(`https://cache.internal/summaries/${encodeURIComponent(movieId)}`);
  const cache = typeof caches !== 'undefined' ? caches.default : null;

  if (cache) {
    try {
      const cached = await cache.match(cacheKey);
      if (cached) {
        const data = (await cached.json()) as RawSummaryResponse;
        return parseRawSummary(data);
      }
    } catch (err) {
      console.warn(`[CACHE] Error reading cache for ${movieId}:`, err);
    }
  }

  try {
    const url = `${env.SUMMARY_WORKER_URL.replace(/\/+$/, '')}/summaries/${encodeURIComponent(movieId)}`;
    const headers: Record<string, string> = {
      'Accept': 'application/json',
    };
    if (env.SUMMARY_API_KEY) {
      headers['X-Api-Key'] = env.SUMMARY_API_KEY;
    }

    const res = await fetch(url, { headers });
    if (res.status === 404) {
      return null;
    }
    if (!res.ok) {
      console.warn(`[SUMMARY_CLIENT] Non-200 from summary worker (${res.status}) for ${movieId}`);
      return null;
    }

    const body = (await res.json()) as { result?: RawSummaryResponse };
    if (!body || !body.result) {
      return null;
    }

    const raw = body.result;

    if (cache) {
      try {
        const cacheRes = new Response(JSON.stringify(raw), {
          headers: {
            'Content-Type': 'application/json',
            'Cache-Control': 'public, max-age=3600',
          },
        });
        await cache.put(cacheKey, cacheRes);
      } catch (err) {
        console.warn(`[CACHE] Error saving cache for ${movieId}:`, err);
      }
    }

    return parseRawSummary(raw);
  } catch (err) {
    console.warn(`[SUMMARY_CLIENT] Failed to fetch summary for ${movieId}:`, err);
    return null;
  }
}

export async function fetchSummariesBatch(
  movieIds: string[],
  env: Env
): Promise<Map<string, SummaryResult>> {
  const result = new Map<string, SummaryResult>();
  if (!env.SUMMARY_WORKER_URL || movieIds.length === 0) {
    return result;
  }

  const missingIds: string[] = [];
  const cache = typeof caches !== 'undefined' ? caches.default : null;

  // 1. Check edge cache for each movie ID
  if (cache) {
    await Promise.all(
      movieIds.map(async (id) => {
        try {
          const cacheKey = new Request(`https://cache.internal/summaries/${encodeURIComponent(id)}`);
          const cached = await cache.match(cacheKey);
          if (cached) {
            const data = (await cached.json()) as RawSummaryResponse;
            result.set(id, parseRawSummary(data));
          } else {
            missingIds.push(id);
          }
        } catch {
          missingIds.push(id);
        }
      })
    );
  } else {
    missingIds.push(...movieIds);
  }

  if (missingIds.length === 0) {
    return result;
  }

  // 2. Fetch missing summaries in batch from Summary Worker
  try {
    const url = `${env.SUMMARY_WORKER_URL.replace(/\/+$/, '')}/summaries/batch`;
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'Accept': 'application/json',
    };
    if (env.SUMMARY_API_KEY) {
      headers['X-Api-Key'] = env.SUMMARY_API_KEY;
    }

    const res = await fetch(url, {
      method: 'POST',
      headers,
      body: JSON.stringify({ ids: missingIds }),
    });

    if (!res.ok) {
      console.warn(`[SUMMARY_CLIENT] Non-200 from summary batch endpoint (${res.status})`);
      return result;
    }

    const body = (await res.json()) as { results?: Record<string, RawSummaryResponse> };
    const fetchedResults = body?.results || {};

    for (const [id, raw] of Object.entries(fetchedResults)) {
      const parsed = parseRawSummary(raw);
      result.set(id, parsed);

      // Cache individual response
      if (cache) {
        try {
          const cacheKey = new Request(`https://cache.internal/summaries/${encodeURIComponent(id)}`);
          const cacheRes = new Response(JSON.stringify(raw), {
            headers: {
              'Content-Type': 'application/json',
              'Cache-Control': 'public, max-age=3600',
            },
          });
          await cache.put(cacheKey, cacheRes);
        } catch (err) {
          console.warn(`[CACHE] Error storing cache for ${id}:`, err);
        }
      }
    }
  } catch (err) {
    console.warn('[SUMMARY_CLIENT] Error in fetchSummariesBatch:', err);
  }

  return result;
}
