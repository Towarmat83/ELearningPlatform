import type { APIRoute } from 'astro';

const USER_API = process.env.USER_API ?? 'http://localhost:8081';
const COURSE_API = process.env.COURSE_API ?? 'http://localhost:8082';

function resolveTarget(path: string): string {
  // Enrollment endpoints live in user-service (must check before generic courses/ rule)
  if (/^courses\/[^/]+\/(enroll|unenroll)$/.test(path)) return USER_API;
  // Admin enrollment management also lives in user-service
  if (/^admin\/courses\/[^/]+\/enrollments/.test(path)) return USER_API;

  // Course-service paths
  if (
    path === 'courses' ||
    path.startsWith('courses/') ||
    path === 'paths' ||
    path.startsWith('paths/') ||
    path.startsWith('uploads/') ||
    path.startsWith('admin/courses') ||
    path.startsWith('admin/cache') ||
    path.startsWith('admin/lab-checks')
  ) {
    return COURSE_API;
  }
  // Everything else (auth, settings, my, admin/users, admin/groups, admin/settings, etc.)
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
    return new Response(JSON.stringify({ error: `Upstream unreachable: ${err.message}` }), {
      status: 502,
      headers: { 'content-type': 'application/json' },
    });
  }

  const resHeaders = new Headers();
  for (const [k, v] of res.headers.entries()) {
    const lower = k.toLowerCase();
    if (lower === 'transfer-encoding' || lower === 'connection') continue;
    resHeaders.set(k, v);
  }

  return new Response(res.body, { status: res.status, headers: resHeaders });
};
