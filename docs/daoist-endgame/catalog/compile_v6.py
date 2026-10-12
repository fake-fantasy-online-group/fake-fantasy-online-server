#!/usr/bin/env python3
"""Compile explicit V6 authoring, preserving immutable V4/V5 evidence."""
import argparse,collections,hashlib,json,pathlib
from generate import validate_extra_affixes,validate_roll_model
HERE=pathlib.Path(__file__).resolve().parent
DOMAIN=HERE.parents[2]/'server/internal/domain'
def encode(x):return (json.dumps(x,ensure_ascii=False,separators=(',',':'))+'\n').encode()
def compile_catalog(rows=None):
 if rows is None:rows=json.loads((HERE/'v6/authored_catalog.json').read_text())
 old={d['BaseID']:d for p in (DOMAIN/'legacy_unique_v5').glob('*.json') for d in json.loads(p.read_text())}
 assert len(rows)==259 and {d['BaseID'] for d in rows}==set(old)
 for d in rows:
  prior=old[d['BaseID']]
  for k in ('ID','BaseID','Name','Lore','LoreRoot','Role','Professions','Slot','LevelRequirement','ResourceLevel','NativeDropEligible','LegacyAffixPoolReady','SourceArm','SourceDesc'):assert d[k]==prior[k],(d['BaseID'],'identity drift',k)
  assert d['PowerTier']==prior['PowerTier']
  validate_extra_affixes(d['Affixes'],d['BaseID'],unique=True)
  validate_roll_model(d['Affixes'],d['VariableAffixes'],d['LocalEnhancements'],d['BaseID'])
  assert all(e['kind']!='aura' for e in d['TypedEffects'])
 groups=collections.defaultdict(list)
 for d in rows:groups[d['Role']].append(d)
 return groups

def main():
 p=argparse.ArgumentParser();p.add_argument('--check',action='store_true');args=p.parse_args();groups=compile_catalog();rows=sum(groups.values(),[])
 outputs={DOMAIN/f'unique_catalog_{role}.json':encode(sorted(groups.get(role,[]),key=lambda d:d['BaseID'])) for role in ('warrior','swordsman','assassin','healer','warlock','sharedcaster','allclasses','unrestricted')}
 summary={'schema':6,'total':len(rows),'legacy_frozen_total':1619,'legacy_v5_total':259,'power_tier_counts':dict(collections.Counter(d['PowerTier'] for d in rows)),'typed_effect_counts':dict(collections.Counter(e['kind'] for d in rows for e in d['TypedEffects'])),'skill_item_count':sum(bool(d['SkillBonuses']) for d in rows),'validation_boundary':'Structural checks only; actual combat acceptance is tested separately.'}
 summary.update(native_drop_kind=259,legacy_affix_pool_ready=259,new_generation_excluded=1360,class_counts=dict(collections.Counter(d['Role'] for d in rows)),variable_count_distribution=dict(collections.Counter(str(len(d['VariableAffixes'])+sum(bool(v.get('MinPercent')) for v in d['LocalEnhancements'])) for d in rows)),catalog_sha256={p.name:hashlib.sha256(data).hexdigest() for p,data in outputs.items()},attack_speed_item_count=sum(any(a['Attr']==25 and a['Value']<0 for a in d['Affixes']) for d in rows),trait_item_count=sum(bool(d['Traits']) for d in rows))
 outputs[HERE/'coverage.json']=(json.dumps(summary,ensure_ascii=False,indent=2)+'\n').encode()
 for path,data in outputs.items():
  if args.check:assert path.read_bytes()==data,('stale V6 output',path)
  else:path.write_bytes(data)
 print('Verified V6:259 authored identities, frozen V5, zero unique auras, <=2 variable lines.')
if __name__=='__main__':main()
