import type { APIRoute } from 'astro';

const USER_API = process.env.USER_API ?? 'http://localhost:8081';
const COURSE_API = process.env.COURSE_API ?? 'http://localhost:8082';

/** Prefixes course-service owns. Everything else belongs to user-service. */
const COURSE_PREFIXES = [
  'courses',
  'paths',
  'skills',
  'batch',
  'uploads',
  'admin/courses',
  'admin/cache',
  'admin/lab-checks',
  'admin/exports/lab-checks',
];

/**
 * Pick the backend for an /api path.
 *
 * A plain prefix test is enough because no service registers a route inside
 * another's prefix — the same property the Helm chart's routing table relies
 * on. This used to need four regexes for endpoints like
 * `courses/{slug}/enroll`, where the owner was only decided after the slug,
 * and one of them was wrong: it matched `admin/courses/{slug}/sessions/{id}`,
 * course-service's own session CRUD, and sent it to user-service.
 *
 * Keep this list in step with `pupitre.routeTable` in the Helm chart.
 */
function resolveTarget(path: string): string {
  for (const prefix of COURSE_PREFIXES) {
    if (path === prefix || path.startsWith(prefix + '/')) return COURSE_API;
  }
  return USER_API;
}

export const ALL: APIRoute = async ({ request, params }) => {
  const path = (params.path ?? '').replace(/^\/+/, '');
  const url = new URL(request.url);
  const target = resolveTarget(path);
  const upstream = `${target}/api/${path}${url.search}`;

  const headers = new Headers();
  for (const [k, v] of request.headers.entries()) {
    const lower = k.toLowerCase();
    if (lower === 'host' || lower === 'connection' || lower === 'transfer-encoding') continue;
    // Not forwarded: fetch() negotiates its own encoding with the upstream and
    // hands back a decoded body. Passing the browser's Accept-Encoding through
    // would ask the upstream to gzip a response this hop immediately gunzips —
    // wasted work on a hop that is localhost or intra-cluster.
    if (lower === 'accept-encoding') continue;
    headers.set(k, v);
  }

  let body: ArrayBuffer | null = null;
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    body = await request.arrayBuffer();
  }

  let res: Response;
  try {
    res = await fetch(upstream, {
      method: request.method,
      headers,
      body: body ?? undefined,
    });
  } catch (err: any) {
    console.error('Upstream fetch error', err);
    return new Response(JSON.stringify({ error: 'Upstream unavailable' }), {
      status: 502,
      headers: { 'content-type': 'application/json' },
    });
  }

  const resHeaders = new Headers();
  for (const [k, v] of res.headers.entries()) {
    const lower = k.toLowerCase();
    if (lower === 'transfer-encoding' || lower === 'connection') continue;
    // `res.body` is the *decoded* stream: fetch() gunzips a compressed
    // upstream response but leaves Content-Encoding and the compressed
    // Content-Length on the header list. Forwarding either describes the body
    // we are about to send incorrectly — the browser would try to gunzip
    // plain JSON and fail with ERR_CONTENT_DECODING_FAILED. Both are dropped
    // and left for this server to set for the hop it actually controls.
    if (lower === 'content-encoding' || lower === 'content-length') continue;
    resHeaders.set(k, v);
  }

  return new Response(res.body, { status: res.status, headers: resHeaders });
};
