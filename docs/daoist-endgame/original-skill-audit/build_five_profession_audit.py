#!/usr/bin/env python3
"""Read-only reconciliation of tracked imports, loader output and optional current resources.
All outputs stay in this audit directory. No business source/DB writes.
"""
from pathlib import Path
import csv, hashlib, importlib.util, json, re, subprocess

OUT = Path(__file__).resolve().parent
SERVER = Path('/workspace/shared/server-development')
CLIENT = Path('/workspace/shared/client-development')
SPEC = importlib.util.spec_from_file_location('source_audit', SERVER/'server/tests/original_skill_source_audit.py')
MODULE = importlib.util.module_from_spec(SPEC); SPEC.loader.exec_module(MODULE)
RAW = MODULE.original_rows()
LOADED = json.loads((SERVER/'docs/daoist-endgame/v5/skill-original-ranks.json').read_text())
CLASSES = {1:'战士',2:'剑客',3:'刺客',4:'药师',5:'术士'}

def git(repo,*args): return subprocess.check_output(['git','-C',str(repo),*args],text=True).strip()
def digest(path): return hashlib.sha256(path.read_bytes()).hexdigest()
def dump(name,value): (OUT/name).write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n')

def issue(ids,field,category,detail,evidence,confidence='confirmed_vs_imported_baseline'):
    return dict(ids=ids,field=field,category=category,detail=detail,evidence=evidence,confidence=confidence)

ISSUES = [
 issue([10102],'target_shape','confirmed_resource_conversion','External tgt1/castkind1 has cone shape3/radius120/rwidth120/dist100; imported five-rank baseline is single shape0/dist75. Stage fix restores entity-single and75px admission.', ['read-only local resource MD5 cbe007d8955b60a992dd4ef3da682c37','client66c7712 NativeSwordSkillTargeting.cs']),
 issue([10122],'target_shape','confirmed_resource_conversion','External tgt1/castkind1 has circle shape2/radius150/dist75; imported baseline is single shape0/dist75. Stage fix restores entity-single.', ['read-only local resource MD5 cbe007d8955b60a992dd4ef3da682c37','client66c7712 NativeSwordSkillTargeting.cs']),
 issue([10009],'effect','missing_native_effect','Description requires15s defense reduction30% at rank1; loaded skill has damage only, no status application.', ['skill-original-ranks.json','gamedb.go skillStatusApplications']),
 issue([10102],'effect','missing_native_effect','Description requires knockback; loaded skill contains damage only. Historical knockback distance not established.', ['skill-original-ranks.json','init-31.sql']),
 issue([10103],'effect','missing_native_effect','Description requires15s accuracy reduction30..50%; loaded skill has no status. Status1008 levels6..10 or11..15 match magnitude but require15s duration override.', ['skill-original-ranks.json','skill-status-anchors.json']),
 issue([10111,10221],'effect','missing_native_effect','Descriptions require movement restriction (10111:6..10s;10221:fixed5s at all five ranks). No status is applied; status1034 cannot be copied unchanged because it also stuns and removes defense.', ['skill-original-ranks.json','skill-status-anchors.json status1034']),
 issue([10113],'effect','missing_native_effect','Description requires6..10s inability to use skills; no status. Status1003 is verified silence but duration needs explicit skill override.', ['skill-original-ranks.json','skill-status-anchors.json status1003']),
 issue([10122],'effect','missing_native_effect','Description requires10..14s rejection of beneficial states; no status. Status1029 BlockBeneficial matches semantic but needs explicit duration.', ['skill-original-ranks.json','skill-status-anchors.json status1029']),
 issue([10201],'effect','partial_passive_implementation','Native descriptions include5/10/15/20/25% stun chance in addition to preserving invisibility. Loaded passive only supplies preserve-invisibility; no stun trigger. Duration not established.', ['skill-original-ranks.json','init-33.sql']),
 issue([10219],'effect','missing_native_effect','Description requires stun5..9s; loaded skill has damage only, no status.', ['skill-original-ranks.json','gamedb.go skillStatusApplications']),
 issue([10224],'effect','unscoped_status_completion','Description and status1007/11..15 mention hit and magic-attack loss only. completeKnownStatusSemantics mirrors magic attack to magic defense for every StatusDampen rank, yielding an extra magic-defense penalty.', ['gamedb.go completeKnownStatusSemantics','skill-status-anchors.json status1007/11..15']),
 issue([10312],'target_filter','missing_native_restriction','Description limits damage to undead/demon enemies. SkillDef and target resolution contain no creature-taxonomy restriction; runtime documentation admits no audited taxonomy.', ['skill-original-ranks.json','scene/skill.go resolveTargets','skill-implementation.md']),
 issue([10313],'effect','missing_native_effect','Description promises random harmful states, but loaded skill has damage only. Status pool/probabilities/durations unproven.', ['skill-original-ranks.json']),
 issue([10317],'effect','missing_native_effect','Description promises blindness after damage; no status. Magnitude/duration are not provided in the imported skill prose.', ['skill-original-ranks.json']),
 issue([10326],'effect','missing_native_effect','Descriptions promise a chance of immobilization6..10s; no status. The original chance is not specified.', ['skill-original-ranks.json']),
 issue([10306,10316,10318,10433],'probability','explicit_authored_assumption','init-33 says original success scripts were missing and deliberately authors20/40/60/80/100% dispel chance. These are implementation choices, not verified original values.', ['init-33.sql B3 comment']),
 issue([10327],'duration','explicit_authored_assumption','init-33 explicitly authors10/15/20/25/30s anti-invisibility suppression because original timing was absent.', ['init-33.sql D4 comment']),
 issue([10223,10020,10226],'distance','explicit_authored_assumption','init-33 explicitly sets10223 knockback100 world units with no historical distance evidence;10020/10226 reuse it.', ['init-33.sql D5 comment and passive trigger effects']),
 issue([10024,10222],'progression','unproven_numeric_supplement','Imported descriptions give no numeric durability curve; supplements set maintenance25% per rank (cap100) and thrown recovery10% per rank. Historical support needs verification.', ['init-31.sql','init-33.sql']),
 issue([10212],'name_effect','historical_identity_conflict','Imported/current server identity 游龙掌 deals100% damage; historical profession table lists 偷窃 in the corresponding slot. Requires version corroboration before selecting restore baseline.', ['research worker fo.17173.com/character/character_3.html'],'candidate_version_sensitive'),
 issue([10030],'target_shape','historical_target_conflict','Historical static warrior page describes area disarm, while imported table is enemy-single. Version boundary unresolved.', ['research worker fo.17173.com/character/character_2.html','init-31.sql'],'candidate_version_sensitive'),
 issue([10115,10124],'duration','historical_duration_conflict','Historical static sword page reports15s; imported table uses30s. May be a later patch rather than arbitrary mutation.', ['research worker fo.17173.com/character/character_1.html','init-31.sql'],'candidate_version_sensitive'),
 issue([10408,10411],'target_shape','historical_single_target_crosscheck','Historical character5 page and imported raw rows both describe enemy-single with450px range. Current resource projection must determine whether these were converted to area.', ['research worker fo.17173.com/character/character_5.html','init-31.sql'],'supported_but_current_resource_pending'),
]

first={int(r['skill_id']):r for r in RAW if r['skill_level']=='1'}
loaded_by={}
for row in LOADED: loaded_by.setdefault(row['ID'],[]).append(row)
old=git(SERVER,'show','5269260:server/internal/domain/skill_curves_v5.go')
old_curves={int(m[1]):dict(carrier=m[2],note=m[3]) for m in re.finditer(r'^\s*(\d+): \{carrier: (\w+),.*?note: "([^"]*)"\},',old,re.M)}
current_path=OUT/'current-profession-skills.json'
current={}
if current_path.exists():
    data=json.loads(current_path.read_text());data=data.get('skills',data) if isinstance(data,dict) else data
    current={int(r['id']):r for r in data}

records=[]
for id,raw in sorted(first.items()):
    rows=[r for r in RAW if int(r['skill_id'])==id]
    record=dict(id=id,profession=int(raw['prof']),class_name=CLASSES[int(raw['prof'])],imported_name=raw['name'],is_life=id==11005,
      raw_imported_ranks=rows,actual_loaded_ranks=loaded_by.get(id,[]),current_resource=current.get(id),
      source_authority='earliest_tracked_import_not_independently_original_official',
      issues=[i for i in ISSUES if id in i['ids']], v5_removed_authored_carrier=old_curves.get(id), current_resource_differences=[])
    cur=current.get(id)
    if cur:
        for key,rawkey in [('name','name'),('tgt','skill_type'),('shape','range_type'),('radius','radius'),('dist','dist'),('rwidth','angle'),('rectl','width'),('rectw','height'),('canSelf','target_self'),('canFriendly','target_team'),('canEnemy','target_mon')]:
            if key not in cur: continue
            base=raw[rawkey] if key=='name' else int(raw[rawkey]);actual=cur[key]
            if actual!=base: record['current_resource_differences'].append(dict(field=key,imported=base,current=actual,authority='resource_vs_imported_not_automatically_original'))
        raw_levels={int(r['skill_level']):r for r in rows}
        for lv in cur.get('levels',[]):
            rank=lv.get('lv');original=raw_levels.get(rank)
            if not original: continue
            expected_mp=int(original['sp_chg_start']);control=int(original['sp_chg_control'])
            if control==1: expected_mp &= 65535
            for k,value in [('desc',original['desc_']),('lreq',int(original['level_need'])),('mp',expected_mp),('cast',int(original['prepare'])),('cd',int(original['sep']))]:
                if k in lv and lv[k]!=value:record['current_resource_differences'].append(dict(field=f'levels[{rank}].{k}',imported=value,current=lv[k],authority='resource_vs_imported_not_automatically_original'))
    records.append(record)

source_paths=['server/migrations/000000_init-31.sql','server/migrations/000000_init-33.sql','server/internal/data/gamedb.go','docs/daoist-endgame/v5/skill-original-ranks.json','docs/daoist-endgame/v5/skill-status-anchors.json']
sources=[dict(repository='server',path=p,sha256=digest(SERVER/p)) for p in source_paths]
summary=dict(client_commit=git(CLIENT,'rev-parse','HEAD'),server_commit=git(SERVER,'rev-parse','HEAD'),
    status='audit_only_not_original_certification',profession_ids=len(records),combat_ids=sum(not r['is_life'] for r in records),raw_rows=len(RAW),loaded_combat_rows=len(LOADED),
    resource_projection_available=bool(current),resource_rows=len(current),known_issue_groups=len(ISSUES),known_issue_skill_ids=len({id for i in ISSUES for id in i['ids']}),
    per_profession={CLASSES[p]:dict(total=sum(r['profession']==p for r in records),with_known_issues=sum(r['profession']==p and bool(r['issues']) for r in records)) for p in CLASSES},
    sources=sources,warning='Earliest tracked imported rows, actual loader output, current resource, and dated historical documentation are separate authorities. Do not equate them.')
dump('five-profession-source-matrix.json',records)
dump('five-profession-known-issues.json',ISSUES)
dump('five-profession-summary.json',summary)
with (OUT/'five-profession-summary.csv').open('w',newline='') as f:
    w=csv.writer(f);w.writerow(['id','class','imported_name','raw_kind','raw_shape','raw_radius','raw_dist','current_name','current_shape','current_radius','current_dist','resource_difference_count','known_issue_categories','rank1_imported_description'])
    for r in records:
        raw=r['raw_imported_ranks'][0];cur=r['current_resource'] or {}
        w.writerow([r['id'],r['class_name'],r['imported_name'],raw['skill_type'],raw['range_type'],raw['radius'],raw['dist'],cur.get('name','pending'),cur.get('shape','pending'),cur.get('radius','pending'),cur.get('dist','pending'),len(r['current_resource_differences']),';'.join(i['category'] for i in r['issues']),raw['desc_']])
print(json.dumps(summary,ensure_ascii=False,indent=2))
