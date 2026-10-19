// Minimal R2-only S3 transport: GET, HEAD and conditional PUT, never bucket,
// DNS, Access, credential, deletion or account management. No third-party deps.
import {createHash,createHmac} from 'node:crypto';
import {createReadStream} from 'node:fs';
import {assert} from '../site/shared/contracts.mjs';
const hash = value => createHash('sha256').update(value).digest('hex');
const hmac = (key, value) => createHmac('sha256',key).update(value).digest();
export function sign({method,url,headers,accessKey,secretKey,date = new Date()}) {
  const timestamp = date.toISOString().replace(/[:-]|\.\d{3}/g,''), day = timestamp.slice(0,8);
  const h = Object.fromEntries(Object.entries({...headers,host:url.host,'x-amz-date':timestamp}).map(([k,v])=>[k.toLowerCase(),String(v).trim()]));
  const names = Object.keys(h).sort(), signed = names.join(';');
  const canonical = [method,url.pathname,'',names.map(k=>`${k}:${h[k]}\n`).join(''),signed,h['x-amz-content-sha256']].join('\n');
  const scope = `${day}/auto/s3/aws4_request`;
  const signingKey = hmac(hmac(hmac(hmac('AWS4'+secretKey,day),'auto'),'s3'),'aws4_request');
  const signature = createHmac('sha256',signingKey).update(`AWS4-HMAC-SHA256\n${timestamp}\n${scope}\n${hash(canonical)}`).digest('hex');
  h.authorization = `AWS4-HMAC-SHA256 Credential=${accessKey}/${scope}, SignedHeaders=${signed}, Signature=${signature}`;
  delete h.host;
  return h;
}
export class R2Store {
  constructor({accountId,bucket,accessKey,secretKey,fetcher = fetch}) {
    assert(/^[a-f0-9]{32}$/.test(accountId), 'explicit Cloudflare account ID required');
    assert(/^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$/.test(bucket) && !bucket.includes('..'), 'invalid existing bucket name');
    assert(accessKey && secretKey, 'R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY must already be available in the environment');
    Object.assign(this,{origin:`https://${accountId}.r2.cloudflarestorage.com`,bucket,accessKey,secretKey,fetcher});
  }
  async request(method,key,{body,path,sha256 = hash(''),sizeBytes,ifMatch,ifNoneMatch,contentType,cacheControl} = {}) {
    const encoded = key.split('/').map(p=>encodeURIComponent(p).replace(/[!'()*]/g,c=>'%'+c.charCodeAt(0).toString(16).toUpperCase())).join('/');
    const url = new URL(`${this.origin}/${this.bucket}/${encoded}`);
    const headers = {'x-amz-content-sha256':sha256};
    if (contentType) headers['content-type'] = contentType;
    if (cacheControl) headers['cache-control'] = cacheControl;
    if (ifMatch) headers['if-match'] = ifMatch;
    if (ifNoneMatch) headers['if-none-match'] = ifNoneMatch;
    if (sizeBytes !== undefined) headers['content-length'] = String(sizeBytes);
    const response = await this.fetcher(url,{method,headers:sign({method,url,headers,accessKey:this.accessKey,secretKey:this.secretKey}),body:path ? createReadStream(path) : body,duplex:'half',redirect:'error',signal:AbortSignal.timeout(30*60*1000)});
    if (response.status === 404 && method !== 'PUT') return null;
    if (response.status === 412) { await response.body?.cancel(); throw new Error('R2 conditional write conflict (412); do not retry with a newer ETag automatically'); }
    if (!response.ok) { await response.body?.cancel(); throw new Error(`R2 ${method} failed (${response.status}); publication stopped`); }
    return response;
  }
  get(key) { return this.request('GET',key); }
  head(key) { return this.request('HEAD',key); }
  put(key,options) { return this.request('PUT',key,options); }
}
