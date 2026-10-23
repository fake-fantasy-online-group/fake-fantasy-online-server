#!/usr/bin/env python3
"""Import approved synergy v1 text, preserving all original IDs/eligibility.
Direction-only proposals are explicitly drafts, never executable definitions.
"""
import json
from pathlib import Path
root=Path(__file__).resolve().parents[1]
source=Path('/workspace/shared/legendary330/synergy-proposal')
path=root/'server/internal/domain/legendary_powers.json'
base=json.loads(path.read_text()); defs={x['id']:x for x in base}
for name in ['warrior','swordsman','assassin','sorcerer','healer','universal']:
 p=json.loads((source/(name+'.json')).read_text())
 key={'warrior':'items','swordsman':'classification','assassin':'powers','sorcerer':'all_50_classification','healer':'powers','universal':'items'}[name]
 details={x['id']:x for x in p.get('key_changes',p.get('representative_changes',[]))}
 for x in p[key]:
  d=defs[x['id']]; decision=x.get('decision',x.get('classification','draft')); full=details.get(x['id'],{})
  text=full.get('new_effect',full.get('new')) or x.get('proposed',x.get('planned_effect',x.get('new_mechanic',x.get('proposal',x.get('plan',x.get('reason',''))))))
  d['design_revision']='synergy-v1'; d['design_decision']=decision
  d['design_status']='retained' if decision=='keep' else ('specified' if decision=='rewrite' and text else 'draft')
  if decision=='rewrite' and (name=='warrior' or bool(full) or x.get('detailed') is True): d['design_status']='specified'
  # CK's focus list identifies the expressly detailed build changes, not a
  # claim that every other qualitative rewrite has sufficient numbers.
  if name=='assassin' and decision=='rewrite' and x.get('focus'): d['design_status']='specified'
  if decision!='keep' or name in ['warrior','healer','assassin']:
   if text:d['effect']=text
  newname=full.get('new_name',x.get('new_name',x.get('proposal_name')))
  if newname:d['name']=newname
  d['revision_note']='Approved synergy v1; '+decision+'; source '+name+'.json'
  d['implementation']='Explicit typed handler required; no prose execution.'
  if decision!='keep':
   d['design_note']='Synergy v1 rewrite replaces obsolete timing mechanics; exact runtime coverage is reported separately.'
   d['skill_basis']='See current effect and typed handler; superseded historical mechanics are not authoritative.'
# Explicit implementation tuning is distinguished from user-authored numbers.
for overlay in sorted(list(source.glob('*-implementation-spec.json')) + list((source.parent/'synergy-implementation').glob('*-replacements.json'))):
 data=json.loads(overlay.read_text())
 for entry in (data if isinstance(data,list) else data.get('powers',[])):
  d=defs[entry['id']]
  for key in ['name','effect']:
   if entry.get(key):d[key]=entry[key]
  d['design_note']=entry.get('design_note',entry.get('design_source','Engineering implementation tuning for approved synergy direction; not separately user-confirmed values.'))
  d['design_status']=entry.get('status','specified')
  if entry.get('classification'):d['design_decision']=entry['classification']
  d['revision_note']+='; engineering tuning: '+entry.get('tuning_basis',overlay.name)
assert len(defs)==330
path.write_text(json.dumps(base,ensure_ascii=False,indent=2)+'\n')
