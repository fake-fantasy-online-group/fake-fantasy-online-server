#!/usr/bin/env python3
"""Reproduce migration 60 from the frozen pre-migration parity export; runtime reads SQL only."""
import json,pathlib,re
root=pathlib.Path(__file__).resolve().parents[3]
# Historical parity input only. Production consumes the generated normalized SQL.
fixture=json.loads((root/'server/internal/testfixture/catalog/endgame_catalog.json').read_text())
rows={'unique':{},'formation':{},'runes':fixture['Runes'],'variants':fixture['Variants']}
for section,kind in [('Uniques','unique'),('Formations','formation')]:
 for row in fixture[section]:rows[kind].setdefault(row['Version'],[]).append(row['Definition'])
# SQL has fully queryable scalar columns and ordered child rows; JSON views are projections only.
def snake(s): return re.sub(r'(?<!^)(?=[A-Z])','_',s).lower().replace('i_d','id').replace('b_p','bp').replace('m_s','ms').replace('p_x','px').replace('i_c_d','icd').replace('p_v_p','pvp').replace('h_p','hp')
def q(x):
 if isinstance(x,bool):return 'true' if x else 'false'
 if isinstance(x,(int,float)):return str(x)
 return "'"+str(x).replace("'","''")+"'"
core={k:'text' for k in ['BaseName','Name','Lore','LoreRoot','Role','Identity','Benefit','Tradeoff','PowerTier','SourceArm','SourceDesc']}
core.update({k:'integer' for k in ['BaseID','Professions','Sex','Slot','LevelRequirement','ResourceLevel','Channel','MinLevel']})
core.update({k:'boolean' for k in ['BeginnerAllowed','Unrestricted','NativeDropEligible','LegacyAffixPoolReady']})
children={'Affixes':{'Attr':'integer','Value':'integer','Mode':'integer'},'VariableAffixes':{'Attr':'integer','Min':'integer','Max':'integer'},'LocalEnhancements':{'Attr':'integer','Percent':'integer','MinPercent':'integer','MaxPercent':'integer'},'Traits':{'Kind':'text','PowerBP':'integer','ThresholdPct':'integer','SkillID':'integer','SkillName':'text'},'ReservedSkills':{'Scope':'text','Target':'text','Levels':'integer'},'SkillBonuses':{'Scope':'text','Profession':'integer','Branch':'text','School':'text','SkillID':'integer','Levels':'bigint'}}
effects={'Kind':'text','VariantID':'text','Element':'text','Value':'integer'}
proc={'ID':'text','Trigger':'text','SkillID':'integer','FixedRank':'integer','ChanceBP':'integer','ICDMS':'integer','SharedGroup':'text','Payload':'text','TargetRule':'text','MaxTargets':'integer','ScaleBP':'integer','PVPScaleBP':'integer','HPBelowPct':'integer','RequireMagic':'boolean','Priority':'integer','VisualKey':'text'}
aura={'ID':'text','FixedRank':'integer','Family':'text','Modifier':'text','Element':'text','Value':'integer','RadiusPX':'integer','RefreshMS':'integer','LeaseMS':'integer','MaxPlayers':'integer','IncludeActivePet':'boolean','VisualKey':'text','Priority':'integer','PVPScaleBP':'integer'}
effects.update({'Proc'+k:v for k,v in proc.items()});effects.update({'Aura'+k:v for k,v in aura.items()}); children['TypedEffects']=effects
children['Runes']={'ItemID':'integer'};children['Slots']={'Slot':'integer'}
def default(t):return False if t=='boolean' else '' if t=='text' else 0
def cols(schema):return ',\n '.join(snake(k)+' '+t+' NOT NULL DEFAULT '+q(default(t)) for k,t in schema.items())
sql=['-- Endgame content catalog. Additive only; no player rows, IDs, rolls or content versions are rewritten.','CREATE TABLE IF NOT EXISTS game_endgame_definitions (\n owner_kind text NOT NULL CHECK(owner_kind IN (\'unique\',\'formation\')), content_version integer NOT NULL CHECK(content_version IN (0,5,6)), definition_id integer NOT NULL CHECK(definition_id>0),\n '+cols(core)+',\n PRIMARY KEY(owner_kind,content_version,definition_id)\n);']
for key,schema in children.items():
 table='game_endgame_'+snake(key)
 sql.append(f'CREATE TABLE IF NOT EXISTS {table} (\n owner_kind text NOT NULL, content_version integer NOT NULL, definition_id integer NOT NULL, ordinal integer NOT NULL CHECK(ordinal>=0),\n '+cols(schema)+',\n PRIMARY KEY(owner_kind,content_version,definition_id,ordinal),\n FOREIGN KEY(owner_kind,content_version,definition_id) REFERENCES game_endgame_definitions(owner_kind,content_version,definition_id) ON DELETE CASCADE\n);')
sql.append('CREATE TABLE IF NOT EXISTS game_endgame_rune_rules (item_id integer PRIMARY KEY, mansion text NOT NULL, quadrant text NOT NULL, regeneration_interval_ms integer NOT NULL CHECK(regeneration_interval_ms IN (0,2000)));')
for kind in ['unique','formation']:
 for version,defs in rows[kind].items():
  for d in defs:
   base=[kind,int(version),d['ID']]; values=base+[d.get(k,default(t)) for k,t in core.items()]
   names=['owner_kind','content_version','definition_id']+list(map(snake,core))
   sql.append('INSERT INTO game_endgame_definitions('+','.join(names)+') VALUES('+','.join(map(q,values))+') ON CONFLICT(owner_kind,content_version,definition_id) DO NOTHING;')
   for key,schema in children.items():
    for ordinal,obj in enumerate(d.get(key,[])):
     if key=='Runes':obj={'ItemID':obj}
     if key=='Slots':obj={'Slot':obj}
     if key=='TypedEffects':
      obj={**obj,**{'Proc'+k:v for k,v in (obj.get('Proc') or obj.get('proc') or {}).items()},**{'Aura'+k:v for k,v in (obj.get('Aura') or obj.get('aura') or {}).items()}}
     lookup={k.lower():v for k,v in obj.items()}
     values=base+[ordinal]+[lookup.get(k.lower(),default(t)) for k,t in schema.items()]
     sql.append('INSERT INTO game_endgame_'+snake(key)+'('+','.join(['owner_kind','content_version','definition_id','ordinal']+list(map(snake,schema)))+') VALUES('+','.join(map(q,values))+') ON CONFLICT(owner_kind,content_version,definition_id,ordinal) DO NOTHING;')
for r in rows['runes']:
 sql.append('INSERT INTO game_endgame_rune_rules VALUES('+','.join(map(q,[r['Item'],r['Mansion'],r['Quadrant'],r.get('RegenerationIntervalMS',0)]))+') ON CONFLICT(item_id) DO NOTHING;')
def obj_expr(schema,alias='x',prefix='',lower=False):
 return "jsonb_build_object("+','.join(q((k[0].lower()+k[1:]) if lower else k)+','+alias+'.'+snake(prefix+k) for k in schema)+")"
view="jsonb_build_object('ID',d.definition_id,"+','.join(q(k)+',d.'+snake(k) for k in core)+")"
for key,schema in children.items():
 if key=='TypedEffects':
  expr=obj_expr({k:v for k,v in schema.items() if not k.startswith(('Proc','Aura'))},lower=True)
  expr+="||jsonb_build_object('proc',CASE WHEN x.proc_id='' THEN NULL ELSE "+obj_expr(proc,prefix='Proc',lower=True)+" END,'aura',CASE WHEN x.aura_id='' THEN NULL ELSE "+obj_expr(aura,prefix='Aura',lower=True)+" END)"
 elif key=='SkillBonuses':expr=obj_expr(schema,lower=True)
 elif key=='Runes':expr='x.item_id'
 elif key=='Slots':expr='x.slot'
 else:expr=obj_expr(schema)
 view+="||jsonb_build_object("+q(key)+",COALESCE((SELECT jsonb_agg("+expr+" ORDER BY x.ordinal) FROM game_endgame_"+snake(key)+" x WHERE (x.owner_kind,x.content_version,x.definition_id)=(d.owner_kind,d.content_version,d.definition_id)),'[]'::jsonb))"
sql.append('CREATE OR REPLACE VIEW game_endgame_catalog AS SELECT d.owner_kind AS kind,d.content_version,d.definition_id,'+view+' AS definition FROM game_endgame_definitions d;')
# Finite equipment behavior presets have scalar parameters, never executable text.
vcore={'ID':'text','Name':'text','Description':'text','SkillID':'integer','Profession':'integer','MaxTargets':'integer','Behavior':'text'}
vparams={'DamagePercent':'integer','SecondaryDamagePercent':'integer','SecondaryNoStatuses':'boolean','SecondaryNoKnockback':'boolean','MinCooldownMS':'integer','TrapSkillID':'integer','TrapSearchRadius':'integer','TrapMinRadius':'integer','TrapMaxTargets':'integer','ImmediateHealPercent':'integer','DelayedHealPercent':'integer','DelayMS':'integer'}
area={k:'integer' for k in ['Shape','Center','Radius','Angle','Length','Width']}
vschema={**vcore,**vparams,**{'Area'+k:v for k,v in area.items()}}
sql.append('CREATE TABLE IF NOT EXISTS game_equipment_skill_variants (\n '+cols(vschema)+', PRIMARY KEY(id)\n);')
for v in rows['variants']:
 obj={**v,**v.get('Params',{}),**{'Area'+k:x for k,x in v.get('Params',{}).get('Area',{}).items()}}
 sql.append('INSERT INTO game_equipment_skill_variants('+','.join(map(snake,vschema))+') VALUES('+','.join(q(obj.get(k,default(t))) for k,t in vschema.items())+') ON CONFLICT(id) DO NOTHING;')
vexpr=obj_expr(vcore)+"||jsonb_build_object('Params',"+obj_expr(vparams)+"||jsonb_build_object('Area',"+obj_expr(area,prefix='Area')+"))"
sql.append('CREATE OR REPLACE VIEW game_equipment_skill_variant_catalog AS SELECT id,'+vexpr+' AS definition FROM game_equipment_skill_variants x;')
p=root/'server/migrations/000000_init-60.sql';p.write_text('\n'.join(sql)+'\n')
print(p,len(sql),p.stat().st_size)
# Completeness metadata is data too: current catalogs can grow without Go constants.
manifest=['CREATE TABLE IF NOT EXISTS game_endgame_catalog_manifest(kind text NOT NULL, content_version integer NOT NULL, expected_count integer NOT NULL CHECK(expected_count>0), PRIMARY KEY(kind,content_version));','CREATE TABLE IF NOT EXISTS game_endgame_definition_manifest(kind text NOT NULL, content_version integer NOT NULL, definition_id integer NOT NULL, section text NOT NULL, expected_rows integer NOT NULL CHECK(expected_rows>=0), PRIMARY KEY(kind,content_version,definition_id,section), FOREIGN KEY(kind,content_version,definition_id) REFERENCES game_endgame_definitions(owner_kind,content_version,definition_id) ON DELETE CASCADE);']
for kind in ['unique','formation']:
 for v,defs in rows[kind].items():
  manifest.append(f'INSERT INTO game_endgame_catalog_manifest VALUES({q(kind)},{v},{len(defs)}) ON CONFLICT(kind,content_version) DO NOTHING;')
  for d in defs:
   for section in children:manifest.append('INSERT INTO game_endgame_definition_manifest VALUES('+','.join(map(q,[kind,int(v),d['ID'],section,len(d.get(section,[]))]))+') ON CONFLICT(kind,content_version,definition_id,section) DO NOTHING;')
for kind,n in [('rune',len(rows['runes'])),('variant',len(rows['variants']))]:manifest.append(f'INSERT INTO game_endgame_catalog_manifest VALUES({q(kind)},0,{n}) ON CONFLICT(kind,content_version) DO NOTHING;')
body='\n'.join(sql+manifest)+'\n';body=body.replace('regeneration_interval_ms IN (0,2000)','regeneration_interval_ms=0 OR (regeneration_interval_ms BETWEEN 100 AND 60000 AND regeneration_interval_ms%100=0)')
# Group value rows by table to keep the migration compact and reviewable.
headers=[]; batches={}
for line in body.splitlines():
 if line.startswith('INSERT INTO '):
  prefix,value=line.split(' VALUES(',1);value,suffix=value.rsplit(') ON CONFLICT',1)
  batches.setdefault((prefix,suffix),[]).append('('+value+')')
 else:headers.append(line)
compact=['-- Generated by docs/daoist-endgame/catalog/compile_database_catalog.py.', '-- Apply transactionally through the standard migration runner; incremental use requires one transaction.','\n'.join(headers)]
# Only a newly seeded definition installs its children and completeness rows.
# Capture pre-existing parents before any insert: replaying this migration must
# preserve deliberate child deletions and their edited manifests as well as
# scalar edits. The temporary relation is scoped to this transaction and removed
# below so repeated applications in the same transaction remain independent.
existing='_migration60_existing_endgame_definitions'
compact.extend([
 '-- Existing definitions own their configured children; never reseed them on replay.',
 f'CREATE TEMP TABLE {existing} ON COMMIT DROP AS SELECT owner_kind,content_version,definition_id FROM game_endgame_definitions;',
])
child_tables={'game_endgame_'+snake(section) for section in children}
for (prefix,suffix),values in batches.items():
 table=prefix.removeprefix('INSERT INTO ').split('(')[0].strip()
 for i in range(0,len(values),1000):
  chunk=',\n'.join(values[i:i+1000])
  if table in child_tables or table=='game_endgame_definition_manifest':
   names=(prefix.split('(',1)[1][:-1] if '(' in prefix else
          'kind,content_version,definition_id,section,expected_rows')
   owner='kind' if table=='game_endgame_definition_manifest' else 'owner_kind'
   compact.append(f'INSERT INTO {table}({names}) SELECT seed.* FROM (VALUES\n'+chunk+
    f') AS seed({names}) WHERE NOT EXISTS (SELECT 1 FROM pg_temp.{existing} old WHERE '+
    f'(old.owner_kind,old.content_version,old.definition_id)=(seed.{owner},seed.content_version,seed.definition_id)) ON CONFLICT'+suffix)
  else:
   compact.append(prefix+' VALUES\n'+chunk+' ON CONFLICT'+suffix)
compact.append(f'DROP TABLE pg_temp.{existing};')
p.write_text('\n'.join(compact)+'\n')
print('Compact normalized SQL:',p.stat().st_size,'bytes')
