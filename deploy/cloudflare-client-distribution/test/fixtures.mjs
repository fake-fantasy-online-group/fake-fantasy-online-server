import {mkdtemp,writeFile,mkdir,readFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {createHash} from 'node:crypto';
export const hash = bytes => createHash('sha256').update(bytes).digest('hex');
export const base = 'https://assets.example.invalid/';
export const jsonBytes = value => Buffer.from(JSON.stringify(value)+'\n');
export function item(overrides = {}) { return {kind:'item',id:1,name:'测试物品',typeCode:1,typeLabel:'普通物品',level:0,price:100,sellPrice:20,weight:1,stackable:true,buyable:true,tradable:true,droppable:true,description:'仅供自动化测试',sex:0,professions:[],stats:{attack:0,defense:0},...overrides}; }
export async function fixture({catalog = true} = {}) {
  const root = await mkdtemp(path.join(tmpdir(),'ffo-cf-test-')), input = path.join(root,'input'); await mkdir(input);
  const files = [], blobs = new Map(), releaseId = 'synthetic-test';
  async function add(relative,bytes) {
    const key = `releases/${releaseId}/${relative}`, source = path.join(input,relative); await mkdir(path.dirname(source),{recursive:true}); await writeFile(source,bytes);
    files.push({path:source,key}); blobs.set(base+key,bytes); return base+key;
  }
  const payload = Buffer.from('synthetic passive resource\n'), payloadURL = await add('resources/data/test.pkg',payload);
  const update = {schemaVersion:1,resourceManifests:{},clients:{}};
  for (const platform of ['android','windows','macos']) {
    const bytes = Buffer.from(`FFO-MOBILE-1\n@client\t${platform}\t1.0.0\ndata/test.pkg\tdata/test.pkg\t${payload.length}\t${hash(payload)}\n`);
    const manifestUrl = await add(`resources/${platform}.tsv`,bytes);
    update.resourceManifests[platform] = {'1.0.0':{manifestUrl}};
    const artifactUrl = await add(`clients/${platform}.${platform === 'android' ? 'apk' : 'zip'}`,Buffer.from(`synthetic ${platform} package, not runnable\n`));
    update.clients[platform] = {version:'1.0.0',artifactUrl};
  }
  const dispatch = {servers:[{name:'测试服务器',host:'192.168.1.10',port:19000,zone:'测试',pvp:false,busy:0}],update};
  const dispatchFile = path.join(root,'dispatch.json'); await writeFile(dispatchFile,jsonBytes(dispatch));
  const config = {releaseId,defaultDispatchUrl:'https://download.example.invalid/dispatch.json',artifactBaseUrl:base,dispatchFile,files};
  let snapshot;
  if (catalog) {
    snapshot = {schemaVersion:1,contentRevision:'sha256:'+hash('synthetic fixture'),generatedAt:'2026-10-19T00:00:00Z',meta:{itemCount:1,equipmentCount:1,total:2,categories:[{value:'item:1',kind:'item',code:1,label:'普通物品',count:1}],maxLevel:20,npcCount:0,taskCount:0,skillCount:0,maps:[]},items:[item(),item({kind:'equipment',id:2,name:'测试剑',level:20,price:1000,typeCode:2,typeLabel:'剑',maxAttack:10,sex:1,professions:['战士'],stackable:false,stats:{attack:10,defense:0}})]};
    const bytes = jsonBytes(snapshot), snapshotName = `catalog-${hash(bytes)}.json`; await add('catalog/'+snapshotName,bytes);
    config.catalogManifestUrl = await add('catalog/manifest.json',jsonBytes({schemaVersion:1,contentRevision:snapshot.contentRevision,generatedAt:snapshot.generatedAt,snapshot:snapshotName,sha256:hash(bytes),bytes:bytes.length,counts:{item:1,equipment:1,total:2}}));
  }
  const configFile = path.join(root,'config.json'); await writeFile(configFile,jsonBytes(config));
  return {root,input,config,configFile,stage:path.join(root,'stage'),blobs,payloadURL,dispatch,snapshot};
}
export class FakeStore {
  constructor() { this.objects = new Map(); this.puts = []; this.beforePut = null; }
  async get(key) { const o = this.objects.get(key); return o ? new Response(o.bytes,{headers:{etag:o.etag,'content-length':String(o.bytes.length)}}) : null; }
  async head(key) { const o = this.objects.get(key); return o ? new Response(null,{headers:{etag:o.etag}}) : null; }
  async put(key,options) {
    if (this.beforePut) await this.beforePut(key,options);
    const old = this.objects.get(key);
    if (options.ifNoneMatch === '*' && old || options.ifMatch && old?.etag !== options.ifMatch) throw new Error('412 conditional conflict');
    const bytes = options.path ? await readFile(options.path) : Buffer.from(options.body), etag = '"'+hash(bytes).slice(0,32)+'"';
    this.objects.set(key,{bytes,etag}); this.puts.push({key,options}); return new Response(null,{headers:{etag}});
  }
}
export function publicFetcher(blobs,extra = {}) {
  return async (url,options) => {
    const bytes = blobs.get(String(url));
    if (!bytes) return new Response('missing',{status:404});
    return new Response(bytes,{headers:{'content-length':String(bytes.length),'cache-control':'public, max-age=31536000, immutable','access-control-allow-origin':'*',...extra}});
  };
}
