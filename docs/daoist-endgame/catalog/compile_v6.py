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
    raise SystemExit("Retired embedded-catalog writer: production reads database migrations 59-63. See docs/daoist-endgame/catalog/README.md. Historical validation functions remain importable for regression tests.")

if __name__=='__main__':main()
