import type { Handle } from '@sveltejs/kit';

const COURSE_API = process.env.COURSE_API || 'http://elearning-course-service:8082';
const USER_API = process.env.USER_API || 'http://elearning-user-service:8081';

export const handle: Handle = async ({ event, resolve }) => {
  if ((event.url.pathname.startsWith('/api/admin/courses') &&
       !event.url.pathname.match(/^\/api\/admin\/courses\/[^/]+\/enrollments/)) ||
      event.url.pathname === '/api/admin/cache/clear' ||
      event.url.pathname.startsWith('/api/admin/patterns') ||
      event.url.pathname.startsWith('/api/patterns')) {
    const targetUrl = `${COURSE_API}${event.url.pathname}${event.url.search}`;

    const headers = new Headers(event.request.headers);
    headers.delete('host');
    headers.set('connection', 'close');

    const response = await fetch(targetUrl, {
      method: event.request.method,
      headers,
      body: ['GET', 'HEAD'].includes(event.request.method) ? undefined : event.request.body,
      // @ts-ignore
      duplex: 'half',
    });

    return new Response(response.body, {
      status: response.status,
      headers: response.headers,
    });
  }

  if (event.url.pathname.startsWith('/api/auth') ||
      event.url.pathname.startsWith('/api/settings') ||
      event.url.pathname.startsWith('/api/my') ||
      event.url.pathname.startsWith('/api/admin') ||
      /^\/api\/courses\/[^\/]+\/(enroll|unenroll)$/.test(event.url.pathname)) {
    const targetUrl = `${USER_API}${event.url.pathname}${event.url.search}`;

    const headers = new Headers(event.request.headers);
    headers.delete('host');
    headers.set('connection', 'close');

    const response = await fetch(targetUrl, {
      method: event.request.method,
      headers,
      body: ['GET', 'HEAD'].includes(event.request.method) ? undefined : event.request.body,
      // @ts-ignore
      duplex: 'half',
    });

    return new Response(response.body, {
      status: response.status,
      headers: response.headers,
    });
  }

  if (event.url.pathname.startsWith('/api') || event.url.pathname.startsWith('/uploads')) {
    const targetUrl = `${COURSE_API}${event.url.pathname}${event.url.search}`;

    const headers = new Headers(event.request.headers);
    headers.delete('host');
    headers.set('connection', 'close');

    const response = await fetch(targetUrl, {
      method: event.request.method,
      headers,
      body: ['GET', 'HEAD'].includes(event.request.method) ? undefined : event.request.body,
      // @ts-ignore
      duplex: 'half',
    });

    return new Response(response.body, {
      status: response.status,
      headers: response.headers,
    });
  }

  return resolve(event);
};
