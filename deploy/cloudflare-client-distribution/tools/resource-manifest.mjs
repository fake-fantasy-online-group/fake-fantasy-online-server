import {assert, version, PLATFORMS, MAX_MANIFEST_BYTES} from '../site/shared/contracts.mjs';
const passive = new Set(['.json','.png','.jpg','.jpeg','.bytes','.txt','.bin','.ogg','.wav']);
export function safePath(path) {
  assert(typeof path === 'string' && path && path.length <= 1024 && !/[\\:?#%<>"|*\p{Cc}]/u.test(path) && !path.startsWith('/'), 'unsafe resource path');
  const parts = path.split('/'); assert(parts.length <= 64, 'resource path too deep');
  for (const p of parts) assert(p && p !== '.' && p !== '..' && !/[. ]$/.test(p) && !/\.part$/i.test(p) && !/^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?:\.|$)/i.test(p), 'unsafe resource path segment');
  return path;
}
function passiveOverlay(path) {
  // Match the client generator/installer's reviewed data-only override paths.
  // Do not admit raw world trees or generic respatch/plugin extensions.
  const parts = path.split('/'), ext = path.slice(path.lastIndexOf('.')).toLowerCase();
  return (parts.length === 4 && path.startsWith('gameassets/respatch/effect/') && ext === '.eff') ||
    (parts.length === 5 && path.startsWith('gameassets/respatch/magic/magic/') && ext === '.gso');
}
function allowedDestination(path) {
  const parts = path.split('/'), ext = path.slice(path.lastIndexOf('.')).toLowerCase();
  return parts.length >= 2 && (passiveOverlay(path) || (parts[0] === 'data' && parts.length === 2 && ext === '.pkg') ||
    (parts[0] === 'Music' && ext === '.ogg') ||
    (['gameassets','ReferenceClient'].includes(parts[0]) && (passive.has(ext) || parts.length >= 3 && parts[1] === 'font' && ['.idx','.wid'].includes(ext))) ||
    (['sndeffect','sndeffect24_pcm'].includes(parts[0]) && ['.wav','.ogg'].includes(ext)));
}
function insertPath(path, files, spellings) {
  const lower = path.toLowerCase();
  assert(!files.has(lower), 'duplicate resource path');
  assert(!spellings.has(lower), 'resource file/directory collision');
  const parts = path.split('/');
  for (let i=1;i<=parts.length;i++) {
    const prefix = parts.slice(0,i).join('/'), key = prefix.toLowerCase();
    assert(!spellings.has(key) || spellings.get(key) === prefix, 'resource directory casing collision');
    assert(i === parts.length || !files.has(key), 'resource file/directory collision'); spellings.set(key,prefix);
  }
  files.add(lower);
}
export function parseResourceManifest(bytes, manifestURL) {
  assert(bytes.byteLength <= MAX_MANIFEST_BYTES, 'resource manifest too large');
  const lines = new TextDecoder('utf-8',{fatal:true}).decode(bytes).replace(/\r\n/g,'\n').split('\n');
  if (lines.at(-1) === '') lines.pop();
  assert(lines.shift() === 'FFO-MOBILE-1', 'only online passive resource manifests are publishable');
  const declaration = lines.shift()?.split('\t');
  assert(declaration?.length === 3 && declaration[0] === '@client' && PLATFORMS.includes(declaration[1]), 'explicit client platform/version required'); version(declaration[2]);
  assert(lines.length > 0 && lines.length <= 100000, 'invalid resource entry count');
  const sources = new Set(), destinations = new Set(), sourceSpellings = new Map(), destinationSpellings = new Map();
  let total = 0;
  const entries = lines.map(line => {
    const parts = line.split('\t'); assert(parts.length === 4, 'invalid resource entry');
    const [source,destination,length,sha256] = parts; safePath(source); safePath(destination);
    assert(passiveOverlay(source) || new Set([...passive,'.pkg','.idx','.wid']).has(source.slice(source.lastIndexOf('.')).toLowerCase()), 'only passive source files can be downloaded');
    assert(allowedDestination(destination), 'executable or unsupported resource destination');
    insertPath(source,sources,sourceSpellings); insertPath(destination,destinations,destinationSpellings);
    assert(/^\d+$/.test(length) && Number.isSafeInteger(Number(length)) && Number(length) > 0, 'invalid resource length');
    assert(/^[a-f0-9]{64}$/.test(sha256), 'invalid resource digest'); total += Number(length);
    assert(total <= 64 * 1024 * 1024 * 1024, 'resource set exceeds 64 GiB');
    const url = new URL(source.split('/').map(encodeURIComponent).join('/'), manifestURL).href;
    assert(url.startsWith(new URL('.', manifestURL).href), 'resource URL escapes manifest directory');
    return {url,source,destination,sizeBytes:Number(length),sha256};
  });
  return {platform:declaration[1],appVersion:declaration[2],entries};
}
