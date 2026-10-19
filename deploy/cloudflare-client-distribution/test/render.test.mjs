import test from 'node:test';
import assert from 'node:assert/strict';
import {loadPrimaryStat} from './render-helpers.mjs';

test('omitted and explicit zero attack endpoints render the same numeric range',async()=>{
  const render=await loadPrimaryStat();
  assert.equal(render({maxAttack:10,stats:{attack:10}}),'物攻 0–10');
  assert.equal(render({minAttack:0,maxAttack:10,stats:{attack:10}}),'物攻 0–10');
  assert.equal(render({minAttack:7,stats:{attack:0}}),'物攻 7–0');
  assert.equal(render({minAttack:7,maxAttack:0,stats:{attack:0}}),'物攻 7–0');
});
test('display range uses exporter endpoints, never the normalized maximum-only filter key',async()=>{
  const render=await loadPrimaryStat();
  assert.equal(render({minAttack:3,maxAttack:10,stats:{attack:999}}),'物攻 3–10');
  assert.equal(render({stats:{attack:10}}),'—');
});
test('absent or zero attack range keeps the existing fallback stat selection',async()=>{
  const render=await loadPrimaryStat();
  assert.equal(render({}),'—');
  assert.equal(render({minAttack:0,maxAttack:0}),'—');
  assert.equal(render({minAttack:0,maxAttack:0,magicAttack:5}),'魔攻 5');
  assert.equal(render({defense:4}),'防御 4');
});
