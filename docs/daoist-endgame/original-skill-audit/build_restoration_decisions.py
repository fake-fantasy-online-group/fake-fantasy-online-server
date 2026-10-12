#!/usr/bin/env python3
"""Generate field-level audit decisions. Does NOT modify client/server gameplay."""
from pathlib import Path
import json,collections
P=Path(__file__).resolve().parent
rows=json.loads((P/'verified-asset-differences.json').read_text())
# H=hash-verified historical artifact; S=imported SQL or actual loader; W=independent historical documentation.
S='server/migrations/000000_init-31.sql + docs/daoist-endgame/v5/skill-original-ranks.json'
H='historical MD5 0a25fd994d85e4cf858511d1847c7949; current MD5 cbe007d8955b60a992dd4ef3da682c37'
W={1:'https://fo.17173.com/character/character_2.html',2:'https://fo.17173.com/character/character_1.html',3:'https://fo.17173.com/character/character_3.html',4:'https://fo.17173.com/character/character_4.html',5:'https://fo.17173.com/character/character_5.html'}
classes={'战士':1,'剑客':2,'刺客':3,'药师':4,'术士':5}

def decide(r,d):
 sid=r['id'];f=d['field'];rank=d.get('rank');old=d['historical'];cur=d['current'];src=[H,S,W[classes[r['profession']]]]
 action='hold';value=None;reason='Version or semantic evidence is insufficient; do not choose an original value by age alone.'
 if f=='presence':
  action='restore';value=True;reason='Skill exists in backup, imported SQL, loaded server catalog and historical profession documentation. Restore complete client metadata and selection, icon and tooltip identity; do not copy flattened backup MP.'
 elif f=='name':
  action='restore';value=old;reason='Backup, SQL and independent historical name agree; current name accompanies a documented replacement mechanic.'
 elif f=='pre':
  reason='Backup requires10321 but raw SQL for this skill has no prerequisite. Current0 matches SQL; no independently verified historical prerequisite. Restore missing10321 first, keep current prerequisite until version evidence resolves it.'
 elif f=='levels.lreq':
  action='restore';value=d.get('imported_sql',old);reason='Historical backup and imported per-rank unlock values agree; current changes are verified. Use authoritative learning gate and render the same requirement.'
 elif f=='levels.mp':
  action='restore_server_contract';value=d.get('imported_sql');reason='Current100/120 local gate exceeds server native rank cost. Use current authoritative server rank cost; backup flattens MP so do not copy old level2..5 values. Historical MP curve authenticity remains separate.'
 elif f in {'levels.cast','levels.cd'}:
  if d.get('old_matches_sql'):
   action='restore';value=old;reason='Backup and raw per-rank SQL agree on this timing; current parameter is a later local change. Cast unit is milliseconds and cd unit is0.1s.'
  else:
   action='restore_server_contract';value=d.get('imported_sql');reason='Both client versions flatten some native timing progressions. Synchronize request/display with server actual rank timing; official historical slope is not independently authenticated.'
 elif f in {'desc','levels.desc'}:
  if sid==10212:
   action='keep_with_unverified_clause';value=cur;reason='Old rank text is incorrectly pure damage despite old top text and name describing theft. Current50..74% theft agrees with historical source direction; per-monster-once restriction is not independently verified. Do not restore damage text; server identity/effect still needs a separately approved theft implementation.'
  elif sid==10029 and f=='desc':
   action='keep';value=cur;reason='Old general fear/unimplemented text is obsolete. Dated2005-12-22 official-labeled update explicitly establishes radius150 stun5..9s; SQL and old rank text match that update.';src.append('https://fo.17173.com/gengxin/20051222.shtml')
  elif sid==10315:
   action='keep';value=cur;reason='This text change only spells one minute as1分钟 rather than一分钟; no mechanic difference.'
  elif sid in {10006,10121,10313}:
   action='hold_geometry_contract';value=old;reason='Original per-rank wording is corroborated by backup and SQL, but runtime numeric radius currently conflicts. Preserve exact original wording as evidence; decide and synchronize authoritative radius before presenting it as realized gameplay. Do not silently change only text.'
  elif sid in {10115,10124} and f=='desc':
   action='restore_from_verified_ranks';value=None;reason='Remove redesigned stance/tradeoff changes, but do not copy obsolete15s general text: both backup rank descriptions and SQL say30s. Generate general original summary from verified ranks with attack/defense tradeoff and10/13/16/19/22% progression.'
  elif sid==10409 and f=='desc':
   action='restore_from_verified_ranks';value=None;reason='Remove unconditional slow and added damage. Old general165% text contradicts old/SQL five-rank100/105/110/115/120% and historical100%+5%. Regenerate summary from corroborated ranks.'
  else:
   action='restore';value=old;reason='Original rank wording is supported by backup and identical SQL descriptions; historical profession documentation corroborates native mechanic/scope. Remove verified later redesign. This does not itself prove the described effect is implemented.'
   if sid==10312:reason+=' Server lacks audited undead/demon taxonomy; enforce that original target restriction before claiming behavior restored.'
 elif f in {'shape','radius','rwidth','dist','castkind','canFriendly'}:
  if sid in {10102,10122,10317}:
   action='restore';value=old;reason='Backup, raw server single-target kind and historical target description agree. Restore entity-single routing and remove fake AoE; source10102 range must remain server-aligned75 rather than backup100.'
  elif sid in {10022,10130,10308,10311,10315}:
   action='keep';value=cur;reason='Current geometry corrects an internally inconsistent backup: corroborated self buff or genuine area. Historical prose and SQL support current semantic classification; do not restore malformed old shape/radius.'
  elif sid in {10213,10214,10215,10216}:
   action='keep';value=cur;reason='Native trap effect is area100 in all descriptions; shape/radius are preview/placement metadata. Changing shape back to0 adds no original effect and can damage presentation. Use explicit trap classification.'
  elif sid==10218:
   action='restore_native_trap_disarm';value=old;reason='Remove invented AoE attack and rename. Native action selects a visible trap; classify explicitly as trap-disarm, not generic castkind0 Instant. Restore original probability55/63/71/79/87% from verified ranks.'
  elif sid==10227:
   action='keep';value=cur;reason='Enemy-or-teammate leap is explicitly described by backup, SQL and historical table. Friendly targeting1 repairs old contradictory0 flag.'
  elif sid==10228:
   action='restore';value=old;reason='Backup and SQL preserve native self-centered detection area200. Restore detector radius/metadata while retaining self-action routing, not an enemy AoE attack.'
  elif sid in {10028,10127,10327}:
   action='keep_client_hold_server_radius';value=cur;reason='Current client radius matches original authored descriptions and historical page. Old raw radius and SQL disagree with that prose; do not reintroduce old radius solely by blanket rollback. Server radius still needs explicit evidence-based reconciliation.'
  elif sid==10313:
   if f in {'shape','dist'}:
    action='keep';value=cur;reason='Original description and SQL identify targeted circular AoE, whereas backup shape0/dist0 is incomplete. Current shape2/dist450 restores native target mode.'
   else:
    action='hold_geometry_contract';value=None;reason='Current200 agrees with raw SQL radius; historical/backup/SQL prose says150. Preserve current numeric setting pending a deliberate canonical-radius decision; never relabel this as certified original.'
 return dict(field=f,rank=rank,action=action,recommended_value=value,reason=reason,sources=src)

out=[]
for r in rows:
 q={k:r[k]for k in ['id','profession','historical_name','current_name','kind']}
 q['decisions']=[decide(r,d)for d in r['field_differences']]
 out.append(q)
(P/'restoration-decisions.json').write_text(json.dumps(out,ensure_ascii=False,indent=2)+'\n')
lines=['# Five-profession field-level restoration decisions','', 'All71 changed/removed skill entries are covered. A decision applies to the named field, not to the entire historical resource row. This is an audit proposal, not implemented source code.','', 'Evidence: hash-verified local backup and current resource; root SQL/actual loaded rank fixture; separately versioned historical documentation. Exact numeric differences are in verified-asset-differences.json.','', 'Important: all745 rank1–5 descriptions in the historical backup match imported SQL exactly. The backup still contains malformed shape fields, flattened MP, inconsistent general text and16 unused rank6–10 blocks. It is not an authenticated official release.','']
for r in out:
 lines+= [f"## {r['id']} {r['historical_name']} ({r['profession']})",'']
 groups={}
 for d in r['decisions']:
  key=(d['field'],d['action'],d['reason']);groups.setdefault(key,[]).append(d)
 for (field,action,reason),ds in groups.items():
  values=', '.join((f"rank{d['rank']}:"if d['rank']else '')+json.dumps(d['recommended_value'],ensure_ascii=False)for d in ds)
  if field in {'desc','levels.desc'}:values='See exact rank/general values in verified-asset-differences.json.'
  lines += [f'- {field} → **{action}**. {reason}',f'  - Value: {values}']
 lines+=['  - Sources: '+ '; '.join(dict.fromkeys(s for d in r['decisions']for s in d['sources'])),'']
(P/'restoration-decisions.md').write_text('\n'.join(lines)+'\n')
print(len(out),'skill entries;',sum(len(x['decisions'])for x in out),'field/rank decisions;',dict(collections.Counter(d['action']for r in out for d in r['decisions'])))
