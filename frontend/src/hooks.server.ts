import type { Handle } from '@sveltejs/kit';

const API_URL = process.env.API_URL || 'http://localhost:8080';

export const handle: Handle = async ({ event, resolve }) => {
  if (event.url.pathname.startsWith('/api') || event.url.pathname.startsWith('/uploads')) {
    const targetUrl = `${API_URL}${event.url.pathname}${event.url.search}`;

    const headers = new Headers(event.request.headers);
    headers.delete('host');

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
