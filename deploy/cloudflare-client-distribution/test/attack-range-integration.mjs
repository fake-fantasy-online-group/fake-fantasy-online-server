// Actual Go exporter serialization -> deployed public card formatter regression.
// An overlay adds a temporary test to the exporter package without editing any
// server files. Everything produced is a small synthetic fixture outside Git.
import assert from 'node:assert/strict';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {mkdtemp,readFile,writeFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {loadPrimaryStat} from './render-helpers.mjs';
const run=promisify(execFile),root=fileURLToPath(new URL('../../../',import.meta.url));
const temp=await mkdtemp(path.join(tmpdir(),'ffo-attack-range-'));
try {
  const goFile=path.join(temp,'range_test.go'),output=path.join(temp,'snapshot.json');
  await writeFile(goFile,`package publiccatalog
import("os";"testing";"time")
func TestCFPublicAttackRangeFixture(t *testing.T) {
  rows:=[]Item{
    normalize(Item{Kind:"equipment",ID:901,Name:"Synthetic zero minimum",TypeCode:2,MaxAttack:10},rawStats{}),
    normalize(Item{Kind:"equipment",ID:902,Name:"Synthetic nonzero range",TypeCode:2,MinAttack:3,MaxAttack:10},rawStats{}),
    normalize(Item{Kind:"equipment",ID:903,Name:"Synthetic zero maximum",TypeCode:2,MinAttack:7},rawStats{}),
    normalize(Item{Kind:"equipment",ID:904,Name:"Synthetic magic fallback",TypeCode:2,MagicAttack:5},rawStats{}),
  }
  artifact,err:=Build(rows,time.Date(2026,10,19,0,0,0,0,time.UTC),"synthetic-fixture-only")
  if err!=nil{t.Fatal(err)}
  if err:=os.WriteFile(os.Getenv("FFO_RANGE_FIXTURE"),artifact.JSON,0600);err!=nil{t.Fatal(err)}
}
`);
  const overlay=path.join(temp,'overlay.json');
  await writeFile(overlay,JSON.stringify({Replace:{[path.join(root,'server/internal/publiccatalog/cf_range_overlay_test.go')]:goFile}}));
  // go vet cannot open a new overlay-only file on Go 1.21; this harness compiles
  // and executes the actual exporter, while normal Go tests/vet remain separate.
  await run(process.env.GO_BINARY || 'go',['test','-vet=off','-overlay',overlay,'./internal/publiccatalog','-run','^TestCFPublicAttackRangeFixture$','-count=1'],{cwd:path.join(root,'server'),env:{...process.env,FFO_RANGE_FIXTURE:output,GOPROXY:'off',GOTOOLCHAIN:'local'},maxBuffer:1024*1024});
  const snapshot=JSON.parse(await readFile(output,'utf8')),render=await loadPrimaryStat();
  const [zeroMinimum,nonzero,zeroMaximum,magic]=snapshot.items;
  assert.equal(Object.hasOwn(zeroMinimum,'minAttack'),false,'real Go zero minimum must be omitted');
  assert.equal(zeroMinimum.maxAttack,10);assert.equal(zeroMinimum.stats.attack,10);
  assert.equal(zeroMinimum.attributes.find(a=>a.label==='物理攻击').value,'0–10');
  assert.equal(render(zeroMinimum),'物攻 0–10');assert.equal(render(nonzero),'物攻 3–10');
  assert.equal(Object.hasOwn(zeroMaximum,'maxAttack'),false);assert.equal(render(zeroMaximum),'物攻 7–0');
  assert.equal(render(magic),'魔攻 5');
  console.log('Go exporter → deployed public renderer PASS: omitted-zero 0–10, explicit 3–10, omitted-maximum 7–0, magic fallback. stats.attack remains a maximum-only filter field.');
}finally{await rm(temp,{recursive:true,force:true});}
