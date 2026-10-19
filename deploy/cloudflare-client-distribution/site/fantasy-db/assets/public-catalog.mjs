import {parseJSON, assert, MAX_CURRENT_BYTES, readBounded, validateCurrent, validateCatalog, getVerifiedJSON, httpsURL} from '../../shared/contracts.mjs';
let pending;
// Failure clears the cache: Retry can recover after an interrupted first load.
export function loadSnapshot(fetcher = fetch) {
  if (!pending) pending = load(fetcher).catch(e => { pending = undefined; throw e; });
  return pending;
}
async function load(fetcher) {
  const response = await fetcher('/release.json', {cache:'no-store', redirect:'error'});
  const release = validateCurrent(parseJSON(new TextDecoder().decode(await readBounded(response, MAX_CURRENT_BYTES))));
  assert(release.catalog, '尚未发布公开资料快照');
  const m = await getVerifiedJSON(release.catalog.manifestUrl, release.catalog, fetcher);
  assert(m.schemaVersion === 1 && /^catalog-[a-f0-9]{64}\.json$/.test(m.snapshot) && /^[a-f0-9]{64}$/.test(m.sha256), '公开资料索引无效');
  assert(Number.isSafeInteger(m.bytes) && m.bytes > 0 && m.bytes <= 64 * 1024 * 1024, '公开资料超过大小限制');
  const url = new URL(m.snapshot, release.catalog.manifestUrl).href; httpsURL(url);
  const snapshot = await getVerifiedJSON(url, {sizeBytes:m.bytes, sha256:m.sha256}, fetcher);
  return validateCatalog(m,snapshot);
}
const stats = ['attack','magicAttack','defense','magicDefense','str','vit','agi','int','spi','dex','luk'];
export function queryCatalog(snapshot, params) {
  const get = key => params.get(key) || '';
  const numeric = (key, fallback = 0) => {
    const s = get(key); if (!s) return fallback;
    assert(/^\d+$/.test(s) && Number.isSafeInteger(Number(s)), `${key} 必须是非负整数`); return Number(s);
  };
  const kind = get('kind') || 'all', query = get('q').trim(), category = get('category');
  assert(['all','item','equipment'].includes(kind), '无效的物品类型');
  assert([...query].length <= 80, '搜索词最多 80 个字符');
  const profession = get('profession'), sex = get('sex'), stat = get('stat');
  assert(!profession || ['初行者','战士','剑客','刺客','药师','术士'].includes(profession), '无效的职业');
  assert(!sex || ['1','2'].includes(sex), '无效的性别');
  assert(!stat || stats.includes(stat), '无效的属性');
  assert(!category || /^(item|equipment):-?\d+$/.test(category), '无效的分类');
  const minLevel = numeric('minLevel'), maxLevel = numeric('maxLevel', Infinity);
  const minPrice = numeric('minPrice'), maxPrice = numeric('maxPrice', Infinity), minStat = Math.max(1,numeric('minStat',1));
  assert(minLevel <= maxLevel && minPrice <= maxPrice, '最小值不能高于最大值');
  const page = numeric('page', 1), pageSize = numeric('pageSize', 24);
  assert(page >= 1 && page <= 1000000 && pageSize >= 1 && pageSize <= 60, '无效的分页');
  const flags = ['tradable','buyable','droppable','stackable'];
  for (const flag of flags) assert(['','0','1'].includes(get(flag)), '无效的流通属性');
  const idQuery = /^\d+$/.test(query) && Number(query) > 0;
  const needle = query.toLocaleLowerCase();
  const rows = snapshot.items.filter(item =>
    (kind === 'all' || item.kind === kind) &&
    (!query || (idQuery ? String(item.id) === String(Number(query)) : `${item.name}\n${item.description || ''}`.toLocaleLowerCase().includes(needle) || String(item.id) === query)) &&
    (!category || `${item.kind}:${item.typeCode}` === category) &&
    item.level >= minLevel && item.level <= maxLevel && item.price >= minPrice && item.price <= maxPrice &&
    flags.every(flag => !get(flag) || item[flag] === (get(flag) === '1')) &&
    (!profession || item.kind === 'equipment' && item.professions.includes(profession)) &&
    (!sex || item.kind === 'equipment' && [0,Number(sex)].includes(item.sex)) &&
    (!stat || (item.stats[stat] || 0) >= minStat));
  const sort = get('sort') || 'id-asc';
  const sorts = {'id-asc':['id',1], 'id-desc':['id',-1], 'name-asc':['name',1], 'level-asc':['level',1], 'level-desc':['level',-1], 'price-asc':['price',1], 'price-desc':['price',-1], 'attack-desc':['maxAttack',-1], 'defense-desc':['defense',-1]};
  assert(Object.hasOwn(sorts,sort), '不支持的排序方式');
  const [key, direction] = sorts[sort];
  rows.sort((a,b) => direction * (key === 'name' ? a.name.localeCompare(b.name,'zh-CN') : (a[key] || 0) - (b[key] || 0)) || a.id - b.id || a.kind.localeCompare(b.kind));
  return {total:rows.length, page, pageSize, items:rows.slice((page-1)*pageSize,page*pageSize)};
}
