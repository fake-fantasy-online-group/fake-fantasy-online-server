// Shared by the read-only Worker, website and local release tools. No dependencies.
export const PLATFORMS = ['android', 'windows', 'macos', 'linux'];
export const MAX_CURRENT_BYTES = 256 * 1024;
export const MAX_MANIFEST_BYTES = 8 * 1024 * 1024;
export const MAX_CLIENT_BYTES = 8 * 1024 * 1024 * 1024;
export function clientFilename(platform,url) {
  const leaf = url.slice(url.lastIndexOf('/') + 1);
  assert(leaf.length > 0 && leaf.length <= 255 && /^[A-Za-z0-9._-]+$/.test(leaf), 'client URL must end in a plain filename');
  const extensions = {android:['.apk'],windows:['.exe','.msi','.zip'],macos:['.dmg','.pkg','.zip'],linux:['.appimage','.tar.gz','.deb','.rpm','.zip']};
  assert(extensions[platform]?.some(ext=>leaf.length > ext.length && leaf.toLowerCase().endsWith(ext)), 'unsupported client package extension');
}
export function assert(condition, message) { if (!condition) throw new Error(message); }
export function object(value, label) {
  assert(value !== null && typeof value === 'object' && !Array.isArray(value), `${label} must be an object`);
  return value;
}
export function fields(value, allowed, label) {
  object(value, label);
  for (const key of Object.keys(value)) assert(allowed.includes(key), `${label}: unknown field ${key}`);
}
export function version(value) {
  assert(typeof value === 'string' && value.length <= 64 && /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(value) && value.split('.').every(n => Number(n) <= 4294967295), 'invalid application version');
  return value;
}
export function httpsURL(value) {
  assert(typeof value === 'string' && value.length <= 4096 && !/[\\\s\u0000-\u001f\u007f]/u.test(value), 'invalid HTTPS URL');
  const u = new URL(value);
  assert(u.protocol === 'https:' && !u.username && !u.password && !u.search && !u.hash, 'URL must be HTTPS without credentials, query or fragment');
  assert(u.hostname !== 'localhost' && !u.hostname.endsWith('.localhost') && u.hostname !== '0.0.0.0' && !u.hostname.startsWith('127.') && !['[::]', '[::1]'].includes(u.hostname), 'local URL forbidden');
  assert(u.href === value, 'URL must be canonical (encode paths explicitly)');
  return u;
}
export function binding(value, label, urlField = 'manifestUrl', max = Number.MAX_SAFE_INTEGER) {
  object(value, label); httpsURL(value[urlField]);
  assert(/^[a-f0-9]{64}$/.test(value.sha256), `${label}: SHA-256 required`);
  assert(Number.isSafeInteger(value.sizeBytes) && value.sizeBytes > 0 && value.sizeBytes <= max, `${label}: invalid sizeBytes`);
}
export function validateDispatch(dispatch) {
  fields(dispatch, ['servers', 'update'], 'dispatch');
  assert(Array.isArray(dispatch.servers), 'dispatch.servers must be an array');
  for (const server of dispatch.servers) {
    fields(server, ['name', 'host', 'port', 'zone', 'pvp', 'busy'], 'server');
    for (const key of ['name', 'host', 'zone']) assert(typeof server[key] === 'string', `server.${key} required`);
    assert(server.host.length > 0 && !/[\s/\\?#@]/u.test(server.host), 'invalid server host');
    assert(Number.isInteger(server.port) && server.port >= 1 && server.port <= 65535, 'invalid game port');
    assert(typeof server.pvp === 'boolean' && Number.isSafeInteger(server.busy), 'invalid server flags');
  }
  if (dispatch.update === undefined) return dispatch;
  const u = dispatch.update;
  assert(new TextEncoder().encode(JSON.stringify(u)).length <= 65536, 'update exceeds 64 KiB');
  fields(u, ['schemaVersion', 'resourceManifestUrl', 'resourceManifestSha256', 'resourceManifestSizeBytes', 'resourceManifests', 'clients', 'hotupdateManifestUrl'], 'update');
  assert(u.schemaVersion === 1, 'update.schemaVersion must remain 1');
  if (u.resourceManifests === undefined || u.resourceManifestUrl !== undefined) httpsURL(u.resourceManifestUrl);
  if (u.resourceManifestSha256 !== undefined || u.resourceManifestSizeBytes !== undefined) {
    binding({manifestUrl:u.resourceManifestUrl, sha256:u.resourceManifestSha256, sizeBytes:u.resourceManifestSizeBytes}, 'default manifest', 'manifestUrl', MAX_MANIFEST_BYTES);
  }
  if (u.hotupdateManifestUrl !== undefined) httpsURL(u.hotupdateManifestUrl);
  if (u.resourceManifests !== undefined) {
    object(u.resourceManifests, 'resourceManifests');
    for (const [platform, versions] of Object.entries(u.resourceManifests)) {
      assert(PLATFORMS.includes(platform), 'unsupported resource platform'); object(versions, 'resource versions');
      for (const [appVersion, item] of Object.entries(versions)) {
        version(appVersion); fields(item, ['manifestUrl', 'sha256', 'sizeBytes'], 'mapped manifest');
        binding(item, 'mapped manifest', 'manifestUrl', MAX_MANIFEST_BYTES);
      }
    }
  }
  if (u.clients !== undefined) {
    object(u.clients, 'clients');
    for (const [platform, client] of Object.entries(u.clients)) {
      assert(PLATFORMS.includes(platform), 'unsupported client platform');
      fields(client, ['version', 'minVersion', 'artifactUrl', 'sha256', 'sizeBytes'], 'client');
      version(client.version); binding(client, 'client', 'artifactUrl', MAX_CLIENT_BYTES); clientFilename(platform,client.artifactUrl);
      if (client.minVersion !== undefined) {
        version(client.minVersion);
        const a = client.minVersion.split('.').map(Number), b = client.version.split('.').map(Number);
        assert(a[0] < b[0] || a[0] === b[0] && (a[1] < b[1] || a[1] === b[1] && a[2] <= b[2]), 'minVersion exceeds version');
      }
    }
  }
  return dispatch;
}
export function validateCurrent(current) {
  fields(current, ['schemaVersion', 'releaseId', 'defaultDispatchUrl', 'artifactBaseUrl', 'dispatch', 'catalog'], 'current');
  assert(current.schemaVersion === 1, 'unsupported release descriptor');
  assert(typeof current.releaseId === 'string' && /^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$/.test(current.releaseId), 'invalid releaseId');
  const dispatch = httpsURL(current.defaultDispatchUrl);
  assert(dispatch.pathname === '/dispatch.json', 'defaultDispatchUrl must end in /dispatch.json');
  const base = httpsURL(current.artifactBaseUrl);
  assert(base.pathname.endsWith('/'), 'artifactBaseUrl must end in /');
  assert(base.origin !== dispatch.origin, 'artifacts must use a separate direct R2 origin');
  validateDispatch(current.dispatch);
  assert(current.dispatch.servers.length > 0 && current.dispatch.update, 'published release requires servers and update metadata');
  if (current.catalog !== undefined) {
    fields(current.catalog, ['manifestUrl', 'sha256', 'sizeBytes'], 'catalog');
    binding(current.catalog, 'catalog', 'manifestUrl', MAX_MANIFEST_BYTES);
  }
  for (const url of descriptorURLs(current)) {
    assert(url.startsWith(base.href), 'all artifact URLs must be under artifactBaseUrl');
    const relative = decodeURIComponent(url.slice(base.href.length));
    assert(relative.split('/').every(p => p && p !== '.' && p !== '..'), 'invalid artifact path');
  }
  assert(new TextEncoder().encode(JSON.stringify(current)).length <= MAX_CURRENT_BYTES, 'current descriptor too large');
  return current;
}
export function descriptorURLs(current) {
  const urls = [], u = current.dispatch.update;
  if (u) {
    if (u.resourceManifestUrl) urls.push(u.resourceManifestUrl);
    if (u.hotupdateManifestUrl) urls.push(u.hotupdateManifestUrl);
    for (const versions of Object.values(u.resourceManifests || {})) for (const item of Object.values(versions)) urls.push(item.manifestUrl);
    for (const item of Object.values(u.clients || {})) urls.push(item.artifactUrl);
  }
  if (current.catalog) urls.push(current.catalog.manifestUrl);
  return urls;
}
export async function readBounded(response, max) {
  assert(response.ok, `HTTP ${response.status}`);
  assert(!response.headers.get('content-encoding') || response.headers.get('content-encoding') === 'identity', 'encoded payload rejected');
  const declared = response.headers.get('content-length');
  if (declared !== null) assert(/^\d+$/.test(declared) && Number(declared) <= max, 'response exceeds size limit');
  const reader = response.body?.getReader(); assert(reader, 'empty response');
  let size = 0; const chunks = [];
  try {
    while (true) {
      const {value, done} = await reader.read(); if (done) break;
      size += value.byteLength; assert(size <= max, 'response exceeds size limit'); chunks.push(value);
    }
  } catch (error) { await reader.cancel(); throw error; }
  const out = new Uint8Array(size); let offset = 0;
  for (const chunk of chunks) { out.set(chunk, offset); offset += chunk.byteLength; }
  return out;
}
export async function sha256(bytes) {
  return Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', bytes)), b => b.toString(16).padStart(2, '0')).join('');
}
export async function getVerifiedJSON(url, descriptor, fetcher = fetch) {
  const response = await fetcher(url, {cache:'no-store', redirect:'error', credentials:'omit'});
  const bytes = await readBounded(response, descriptor.sizeBytes);
  assert(bytes.byteLength === descriptor.sizeBytes && await sha256(bytes) === descriptor.sha256, '文件大小或 SHA-256 校验失败');
  return parseJSON(new TextDecoder('utf-8', {fatal:true}).decode(bytes));
}
// JSON.parse silently accepts duplicate keys. Reject ambiguous release metadata
// before parsing so the CLI, Worker and website agree on the same descriptor.
export function parseJSON(text) {
  let at = 0;
  const ws = () => { while (/\s/.test(text[at] || '') && at < text.length) at++; };
  function string() {
    const start = at++; let escape = false;
    while (at < text.length) {
      const c = text[at++];
      if (!escape && c === '"') return JSON.parse(text.slice(start, at));
      if (!escape && c === '\\') escape = true; else escape = false;
    }
    throw new Error('unterminated JSON string');
  }
  function value(depth = 0) {
    assert(depth <= 64, 'JSON nesting too deep'); ws();
    if (text[at] === '"') { string(); return; }
    if (text[at] === '{' || text[at] === '[') {
      const objectMode = text[at++] === '{', end = objectMode ? '}' : ']', seen = new Set();
      ws(); if (text[at] === end) { at++; return; }
      while (at < text.length) {
        if (objectMode) {
          ws(); assert(text[at] === '"', 'invalid JSON object');
          const key = string(); assert(!seen.has(key), `duplicate JSON key: ${key}`); seen.add(key);
          ws(); assert(text[at++] === ':', 'invalid JSON object');
        }
        value(depth + 1); ws();
        if (text[at] === end) { at++; return; }
        assert(text[at++] === ',', 'invalid JSON separator');
      }
      throw new Error('unterminated JSON container');
    }
    const start = at;
    while (at < text.length && !/[\s,\]}]/.test(text[at])) at++;
    assert(at > start, 'missing JSON value'); JSON.parse(text.slice(start, at));
  }
  value(); ws(); assert(at === text.length, 'trailing JSON data');
  return JSON.parse(text);
}
export function validateCatalog(index,snapshot) {
  fields(index,['schemaVersion','contentRevision','sourceRevision','generatedAt','snapshot','sha256','bytes','counts'],'catalog index');
  fields(snapshot,['schemaVersion','contentRevision','sourceRevision','generatedAt','meta','items'],'catalog snapshot');
  assert(index.schemaVersion === 1 && snapshot.schemaVersion === 1 && snapshot.contentRevision === index.contentRevision && snapshot.generatedAt === index.generatedAt && snapshot.sourceRevision === index.sourceRevision,'catalog revision mismatch');
  assert(/^sha256:[a-f0-9]{64}$/.test(snapshot.contentRevision) && !Number.isNaN(Date.parse(snapshot.generatedAt)),'invalid catalog provenance');
  assert(index.snapshot === `catalog-${index.sha256}.json` && /^[a-f0-9]{64}$/.test(index.sha256),'invalid catalog filename');
  fields(index.counts,['item','equipment','total'],'catalog counts');
  fields(snapshot.meta,['itemCount','equipmentCount','total','maxLevel','categories','npcCount','taskCount','skillCount','maps'],'catalog metadata');
  assert(Array.isArray(snapshot.items) && Array.isArray(snapshot.meta.categories),'invalid catalog rows');
  assert(snapshot.meta.npcCount === 0 && snapshot.meta.taskCount === 0 && snapshot.meta.skillCount === 0 && Array.isArray(snapshot.meta.maps) && snapshot.meta.maps.length === 0,'only item/equipment projection is publishable');
  const counts={item:0,equipment:0};
  const itemFields=['kind','id','name','typeCode','typeLabel','level','price','sellPrice','weight','stackable','buyable','tradable','droppable','minAttack','maxAttack','magicAttack','defense','magicDefense','durability','iconId','description','attributes','sex','professions','stats'];
  const stats=['attack','magicAttack','defense','magicDefense','str','vit','agi','int','spi','dex','luk'];
  for(const item of snapshot.items) {
    fields(item,itemFields,'public item'); assert(['item','equipment'].includes(item.kind),'unsupported public collection'); counts[item.kind]++;
    assert(Number.isSafeInteger(item.id) && typeof item.name === 'string' && typeof item.typeLabel === 'string','invalid public item identity');
    assert(Array.isArray(item.professions) && item.professions.every(v=>typeof v === 'string'),'invalid professions');
    fields(item.stats,stats,'item stats'); for(const value of Object.values(item.stats)) assert(Number.isSafeInteger(value),'invalid item stat');
    for(const attribute of item.attributes || []) {fields(attribute,['label','value','kind'],'item attribute');assert(typeof attribute.label === 'string' && typeof attribute.value === 'string','invalid item attribute');}
  }
  for(const category of snapshot.meta.categories) fields(category,['value','kind','code','label','count'],'catalog category');
  assert(snapshot.meta.total === snapshot.items.length && index.counts.total === snapshot.items.length && snapshot.meta.itemCount === counts.item && index.counts.item === counts.item && snapshot.meta.equipmentCount === counts.equipment && index.counts.equipment === counts.equipment,'catalog count mismatch');
  return snapshot;
}
