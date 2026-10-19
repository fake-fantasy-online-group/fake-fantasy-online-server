import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';

// Exercise the actual deployed classic-script formatter without replacing it
// with a test-side implementation or needing a browser/DOM for this pure function.
export async function loadPrimaryStat() {
  const source=await readFile(new URL('../site/fantasy-db/assets/app.js',import.meta.url),'utf8');
  const start=source.indexOf('function primaryStat(item) {'), end=source.indexOf('\nfunction accentFor(',start);
  assert.ok(start>=0 && end>start,'deployed primaryStat function must exist');
  const context={formatNumber:value=>new Intl.NumberFormat('zh-CN').format(value)};
  vm.runInNewContext(source.slice(start,end)+'\nthis.renderPrimaryStat = primaryStat;',context);
  return context.renderPrimaryStat;
}
