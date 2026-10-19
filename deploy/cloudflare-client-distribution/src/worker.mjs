import {parseJSON, MAX_CURRENT_BYTES, validateCurrent} from '../site/shared/contracts.mjs';
const SECURITY = {
  'X-Content-Type-Options':'nosniff', 'Referrer-Policy':'no-referrer', 'X-Frame-Options':'DENY',
  'Content-Security-Policy':"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' https:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'",
};
function response(request, value, status = 200, extra = {}) {
  return new Response(request.method === 'HEAD' ? null : JSON.stringify(value) + '\n', {
    status, headers:{...SECURITY, 'Content-Type':'application/json; charset=utf-8', 'Cache-Control':'no-store', ...extra},
  });
}
async function current(env) {
  if (!env.RELEASES) throw new Error('release not published');
  const object = await env.RELEASES.get('current.json');
  if (!object || object.size > MAX_CURRENT_BYTES) throw new Error('release not available');
  // Only a tiny descriptor is read. Never fetch or stream resource/client objects here.
  const text = await object.text();
  if (new TextEncoder().encode(text).length > MAX_CURRENT_BYTES) throw new Error('descriptor too large');
  return validateCurrent(parseJSON(text));
}
export default {
  async fetch(request, env) {
    const url = new URL(request.url), path = url.pathname;
    if (!['GET', 'HEAD'].includes(request.method)) return response(request, {error:'read-only endpoint'}, 405, {Allow:'GET, HEAD'});
    if (path === '/dispatch.json' || path === '/release.json') {
      try {
        const release = await current(env);
        return response(request, path === '/dispatch.json' ? release.dispatch : release, 200,
          path === '/dispatch.json' ? {'Access-Control-Allow-Origin':'*'} : {});
      } catch {
        // Do not leak binding configuration, bucket names, parser errors or unpublished data.
        return response(request, {error:'No verified release is available'}, 503);
      }
    }
    if (path === '/') return new Response(null, {status:302, headers:{...SECURITY, Location:'/download/', 'Cache-Control':'no-store'}});
    if (path === '/download' || path === '/fantasy-db') return new Response(null, {status:308, headers:{...SECURITY, Location:path + '/', 'Cache-Control':'no-store'}});
    if (!/^\/(download|fantasy-db|shared)\//.test(path) || path.includes('\\') || /%2f|%5c/i.test(path)) return response(request, {error:'not found'}, 404);
    const asset = await env.ASSETS.fetch(request);
    const headers = new Headers(asset.headers);
    for (const [key, value] of Object.entries(SECURITY)) headers.set(key, value);
    return new Response(request.method === 'HEAD' ? null : asset.body, {status:asset.status, headers});
  },
};
