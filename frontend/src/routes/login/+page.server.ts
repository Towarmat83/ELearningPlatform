import type { PageServerLoad } from './$types';
import { request } from 'node:http';

const USER_API = process.env.USER_API || 'http://elearning-user-service:8081';

function httpGet(url: string): Promise<unknown> {
  return new Promise((resolve, reject) => {
    const req = request(url, { headers: { connection: 'close' } }, (res) => {
      if (!res.statusCode || res.statusCode >= 400) {
        reject(new Error(`HTTP ${res.statusCode}`));
        res.resume();
        return;
      }
      let data = '';
      res.on('data', (chunk) => (data += chunk));
      res.on('end', () => {
        try { resolve(JSON.parse(data)); } catch { reject(new Error('invalid JSON')); }
      });
    });
    req.on('error', reject);
    req.end();
  });
}

export const load: PageServerLoad = async () => {
  try {
    const settings = await httpGet(`${USER_API}/api/settings/public`);
    return { settings };
  } catch {
    // fall through to defaults
  }
  return {
    settings: {
      oidc_enabled: 'false',
      ldap_enabled: 'false',
      sso_local_login_enabled: 'true',
      registration_enabled: 'true',
    },
  };
};
