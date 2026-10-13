#!/usr/bin/env python3
"""Audit only. Inputs are local projections; never writes gameplay or database state.
Run beside current-profession-skills.json and historical-profession-skills.json.
The projections/evidence package are private cloud inputs and must NOT be committed.
"""
from pathlib import Path
from collections import Counter
import importlib.util,json,hashlib,sys
sys.dont_write_bytecode = True
P=Path(__file__).resolve().parent
S=Path('/workspace/shared/server-development')
spec=importlib.util.spec_from_file_location('audit',S/'server/tests/original_skill_source_audit.py')
a=importlib.util.module_from_spec(spec);spec.loader.exec_module(a)
raw={(int(r['skill_id']),int(r['skill_level'])):r for r in a.original_rows()}
curdoc=json.loads((P/'current-profession-skills.json').read_text()); olddoc=json.loads((P/'historical-profession-skills.json').read_text())
cur={r['id']:r for r in curdoc['skills']};old={r['id']:r for r in olddoc['skills']}
prof={1:'战士',2:'剑客',3:'刺客',4:'药师',5:'术士'}
M={'name':'name','icon':'small_map','tgt':'skill_type','shape':'range_type','radius':'radius','dist':'dist','rwidth':'angle','rectl':'width','rectw':'height','maxlv':'max_level','canSelf':'target_self','canFriendly':'target_team','canEnemy':'target_mon','pre':'depend1_id'}
LM={'desc':'desc_','lreq':'level_need','mp':'sp_chg_start','cast':'prepare','cd':'sep','money':'learn_money'}
def sqlv(sid,k,rank=1):
 r=raw.get((sid,rank));m=LM if k.startswith('levels.') else M;key=k.split('.')[-1]
 if not r or key not in m:return None
 v=r[m[key]] if key in {'name','desc'}else int(r[m[key]])
 if key=='mp' and int(r['sp_chg_control'])==1:v &=65535
 return v

def delta(sid,field,b,c,rank=None):
 q=dict(field=field,historical=b,current=c)
 if rank is not None:q['rank']=rank
 v=sqlv(sid,field,rank or 1)
 if v is not None:q.update(imported_sql=v,old_matches_sql=b==v,current_matches_sql=c==v)
 return q

def flags(sid):
 # Each recommendation applies only to the named fields, not whole row rollback.
 d=[]
 if sid in {10014,10024,10118,10203,10218,10229,10306,10307,10327}:d.append('custom_design_replaces_native_identity_or_mechanic')
 if sid in {10008,10025,10026,10108,10314,10409,10433}:d.append('custom_effect_or_scope_added')
 if sid in {10012,10015,10027,10104,10115,10123,10124,10302,10303}:d.append('native_effect_numeric_or_tradeoff_changed')
 if sid in {10102,10122,10317}:d.append('confirmed_single_to_area_with_SQL_and_historical_support')
 if sid in {10022,10130,10308,10311,10315}:d.append('current_geometry_repairs_old_conflict_do_not_blindly_revert')
 if sid in {10213,10214,10215,10216,10228}:d.append('trap_geometry_is_routing_metadata_requires_semantic_resolution')
 if sid==10227:d.append('friendly_target_change_matches_authored_prose_do_not_blindly_revert')
 if sid==10212:d.append('old_levels_corrupt_current_theft_direction_supported_exact_restrictions_unproven')
 if sid==10029:d.append('old_top_description_obsolete_after_2005_12_22_official_labeled_update')
 if sid in {10006,10028,10121,10127,10313,10327}:d.append('radius_text_table_conflict_requires_field_level_decision')
 if sid in {10115,10124}:d.append('old_top_15s_vs_old_rank_30s_conflict_do_not_choose_duration_from_top_alone')
 if sid==10409:d.append('old_top_165pct_vs_old_rank_100pct_conflict')
 if sid in {10316,10318,10321}:d.append('missing_native_skill_restore_identity_and_links_with_SQL_historical_support')
 return d or ['parameter_change_verified_official_version_approval_still_required']
rows=[]
for sid in sorted(set(old)|set(cur)):
 b=old.get(sid);c=cur.get(sid);r=raw[(sid,1)];ds=[]
 if b is None or c is None:
  kind='added'if b is None else 'removed';ds=[dict(field='presence',historical=b is not None,current=c is not None)]
 else:
  kind='modified'
  for k in sorted(set(b)|set(c)):
   if k=='levels':continue
   if b.get(k)!=c.get(k):ds.append(delta(sid,k,b.get(k),c.get(k)))
  bl={v['lv']:v for v in b['levels']};cl={v['lv']:v for v in c['levels']}
  for rank in sorted(set(bl)|set(cl)):
   if rank not in bl or rank not in cl:ds.append(delta(sid,'levels.presence',rank in bl,rank in cl,rank));continue
   for k in sorted(set(bl[rank])|set(cl[rank])):
    if bl[rank].get(k)!=cl[rank].get(k):ds.append(delta(sid,'levels.'+k,bl[rank].get(k),cl[rank].get(k),rank))
 if not ds:continue
 rows.append(dict(id=sid,profession=prof[int(r['prof'])],historical_name=(b or c)['name'],current_name=c['name'] if c else None,kind=kind,field_differences=ds,decision_flags=flags(sid)))

# Report pre-existing defects independently from the 71 changes.
oldconf=[]
for sid,b in sorted(old.items()):
 ds=[]
 for k in M:
  v=sqlv(sid,k)
  if b.get(k)!=v:ds.append(dict(field=k,historical=b.get(k),imported_sql=v))
 for lv in b['levels']:
  rank=lv['lv']
  if (sid,rank) not in raw:continue
  for k in LM:
   v=sqlv(sid,'levels.'+k,rank)
   if lv.get(k)!=v:ds.append(dict(field='levels.'+k,rank=rank,historical=lv.get(k),imported_sql=v))
 if ds:oldconf.append(dict(id=sid,name=b['name'],differences=ds))

count=lambda k:sum(any(d['field']==k for d in r['field_differences']) for r in rows)
summary=dict(status='audit_only_not_official_baseline_certification',current_resource_md5=curdoc['md5'],historical_resource_md5=olddoc['md5'],current_count=len(cur),historical_count=len(old),modified=sum(r['kind']=='modified'for r in rows),removed=[r['id']for r in rows if r['kind']=='removed'],per_profession={p:dict(modified=sum(r['profession']==p and r['kind']=='modified' for r in rows),removed=sum(r['profession']==p and r['kind']=='removed' for r in rows))for p in prof.values()},field_changed_skill_counts=dict(Counter(d['field']for r in rows for d in {q['field']:q for q in r['field_differences']}.values())),historical_rows_with_sql_differences=len(oldconf),historical_extra_level_rows=[dict(id=sid,levels=len(r['levels']),maxlv=r['maxlv'])for sid,r in old.items()if len(r['levels'])>r['maxlv']],notes=['68 modified means changed between two local artifacts, not 68 independently authenticated original bugs.','Three removed combat skills are independent from the life skill11005, which neither projection includes.','SQL comparison uses like-for-like exported fields; target permissions may encode different concepts and require loader validation.','Client cast is milliseconds; client cd is deciseconds. Raw SQL prepare and sep match those units.','Old/current resource mp is verified directly flattened at rank1 for most skills; local full-JSON reprojection matched exactly. Do not adopt it as server growth truth.'])
for n,v in [('verified-asset-differences.json',rows),('asset-history-summary.json',summary),('historical-resource-vs-sql.json',oldconf)]:
 (P/n).write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n')

def show(v):return json.dumps(v,ensure_ascii=False)if not isinstance(v,str)else v
lines=['# Verified five-profession asset change ledger','', 'Read-only comparison of local historical backup0a25fd… and current resourcecbe007…. These are proven asset changes, not blanket approval to roll back every changed field. Raw resources stay cloud-local.','', '68 modified skills and three removed skills. Exact per-rank values and SQL corroboration: `verified-asset-differences.json`.','']
for r in rows:
 lines.extend([f"## {r['id']} {r['historical_name']} → {r['current_name'] or '[removed]'} ({r['profession']})",'', 'Decision: '+ '; '.join(r['decision_flags']), ''])
 group={}
 for d in r['field_differences']:
  if 'rank'in d:group.setdefault(d['field'],[]).append(d)
  elif d['field']!='desc':lines.append(f"- {d['field']}: {show(d['historical'])} → {show(d['current'])}")
  else:lines.extend(['- General description:', '  - Old: '+d['historical'],'  - Current: '+d['current']])
 for k,ds in group.items():
  if k=='levels.desc':
   if len(ds)>2:ds=[ds[0],ds[-1]]
   for d in ds:lines.extend([f"- Rank{d['rank']} description:",'  - Old: '+show(d['historical']),'  - Current: '+show(d['current'])])
  else:
   lines.append('- '+k+': '+ '; '.join(f"rank{d['rank']} {show(d['historical'])}→{show(d['current'])}"for d in ds))
 lines.append('')
(P/'verified-asset-change-ledger.md').write_text('\n'.join(lines)+'\n')
print(json.dumps(summary,ensure_ascii=False,indent=2))
