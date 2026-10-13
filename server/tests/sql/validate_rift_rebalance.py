#!/usr/bin/env python3
"""Validate migration78 against an explicitly supplied idle disposable migration77 cluster.

No cluster copies or daemon are needed. --data-dir must be a test fixture, never
an operator database. The final successful migration is retained for Go loader tests.
"""
import argparse
import io
import os
from pathlib import Path
import re
import subprocess
from validate_daoist_postgres import write_single_user_commands

p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--data-dir',required=True,type=Path)
p.add_argument('--work-dir',required=True,type=Path)
a=p.parse_args()
root=Path(__file__).resolve().parents[3]
data=a.data_dir.resolve();work=a.work_dir.resolve();work.mkdir(parents=True,exist_ok=True)
if not (data/'PG_VERSION').is_file() or (data/'postmaster.pid').exists():
 raise SystemExit('require explicitly supplied idle disposable test cluster')
body=(root/'server/migrations/000000_init-78.sql').read_text()

def run(label,sql,error=None):
 command=io.StringIO();write_single_user_commands(io.StringIO(sql),command)
 result=subprocess.run([str(Path(os.environ['PG_BIN'])/'postgres'),'--single','-j','-D',str(data),'postgres'],input=command.getvalue(),text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
 (work/(label+'.log')).write_text(result.stdout)
 errors=[line for line in result.stdout.splitlines() if re.search(r'\b(?:ERROR|FATAL|PANIC):',line)]
 assert result.returncode==0,(label,result.returncode,errors)
 if error:
  assert any(error in line for line in errors),(label,errors)
  assert all(error in line or 'current transaction is aborted' in line for line in errors),(label,errors)
 else:assert not errors,(label,errors)
 print('PASS',label,flush=True)

def clone(target):
 return f"""INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,to_jsonb(r)||'{{"version":{target},"published":false}}'::jsonb)).* FROM game_rift_rule_versions r WHERE version=3;
 INSERT INTO game_rift_class_rules SELECT {target},class,hp_bp,damage_bp FROM game_rift_class_rules WHERE version=3;
 INSERT INTO game_rift_ability_rules SELECT {target},id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct FROM game_rift_ability_rules WHERE version=3;"""

run('require-migration77',"""DO $$ BEGIN IF (SELECT version FROM game_rift_rule_active WHERE id=1)<>3 OR EXISTS(SELECT FROM game_rift_rule_versions WHERE version=4) THEN RAISE EXCEPTION 'require unchanged migration77 disposable fixture'; END IF; END $$;""")
# Compare old rows excluding only the three defaulted additive schema columns.
snapshot="""CREATE TEMP TABLE before_rift AS SELECT version,to_jsonb(r) AS value FROM game_rift_rule_versions r;
 CREATE TEMP TABLE before_classes AS SELECT * FROM game_rift_class_rules;
 CREATE TEMP TABLE before_abilities AS SELECT * FROM game_rift_ability_rules;
 CREATE TEMP TABLE before_loot AS SELECT 'versions' AS name,jsonb_agg(to_jsonb(r) ORDER BY version) AS value FROM game_loot_rule_versions r
 UNION ALL SELECT 'pool',jsonb_agg(to_jsonb(r) ORDER BY version,seq) FROM game_loot_rune_pool r
 UNION ALL SELECT 'grades',jsonb_agg(to_jsonb(r) ORDER BY version,grade) FROM game_loot_grades r
 UNION ALL SELECT 'sockets',jsonb_agg(to_jsonb(r) ORDER BY version,min_level) FROM game_loot_socket_levels r;
"""
assertions="""DO $$ BEGIN
 IF EXISTS(SELECT FROM before_rift b JOIN game_rift_rule_versions r USING(version) WHERE b.value IS DISTINCT FROM to_jsonb(r)-'stat_anchor_tier'-'normal_boss_equipment'-'completion_level_experience') THEN RAISE EXCEPTION 'historical rift changed'; END IF;
 IF EXISTS((SELECT * FROM before_classes EXCEPT ALL SELECT * FROM game_rift_class_rules WHERE version<4) UNION ALL (SELECT * FROM game_rift_class_rules WHERE version<4 EXCEPT ALL SELECT * FROM before_classes)) OR EXISTS((SELECT * FROM before_abilities EXCEPT ALL SELECT * FROM game_rift_ability_rules WHERE version<4) UNION ALL (SELECT * FROM game_rift_ability_rules WHERE version<4 EXCEPT ALL SELECT * FROM before_abilities)) THEN RAISE EXCEPTION 'historical children changed'; END IF;
 IF NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=4 AND published AND stat_anchor_tier=30 AND normal_boss_equipment=10 AND completion_level_experience AND loot_version=2 AND floor_population_multiplier=2 AND experience_divisor=10 AND exit_name='洞天残魂' AND npc_model=362 AND exit_sprite='npc362' AND exit_portrait='') THEN RAISE EXCEPTION 'new configuration mismatch'; END IF;
 IF (SELECT to_jsonb(r)-'version'-'published'-'stat_anchor_tier'-'normal_boss_equipment'-'completion_level_experience'-'exit_name'-'exit_greeting'-'npc_model'-'exit_sprite'-'exit_portrait' FROM game_rift_rule_versions r WHERE version=3) IS DISTINCT FROM (SELECT to_jsonb(r)-'version'-'published'-'stat_anchor_tier'-'normal_boss_equipment'-'completion_level_experience'-'exit_name'-'exit_greeting'-'npc_model'-'exit_sprite'-'exit_portrait' FROM game_rift_rule_versions r WHERE version=4) THEN RAISE EXCEPTION 'unrelated new rules changed'; END IF;
 IF EXISTS(SELECT FROM before_loot b JOIN (SELECT 'versions' AS name,jsonb_agg(to_jsonb(r) ORDER BY version) AS value FROM game_loot_rule_versions r UNION ALL SELECT 'pool',jsonb_agg(to_jsonb(r) ORDER BY version,seq) FROM game_loot_rune_pool r UNION ALL SELECT 'grades',jsonb_agg(to_jsonb(r) ORDER BY version,grade) FROM game_loot_grades r UNION ALL SELECT 'sockets',jsonb_agg(to_jsonb(r) ORDER BY version,min_level) FROM game_loot_socket_levels r) n USING(name) WHERE b.value IS DISTINCT FROM n.value) THEN RAISE EXCEPTION 'rune or loot rules changed'; END IF;
 END $$;"""
run('atomic-first-import-and-preservation','BEGIN;'+snapshot+body+assertions+'ROLLBACK;')
run('atomic-rollback','BEGIN;'+body+"DO $$ BEGIN RAISE EXCEPTION 'deliberate rollback'; END $$;COMMIT;",'deliberate rollback')
run('rollback-restored-schema',"""DO $$ BEGIN IF EXISTS(SELECT FROM game_rift_rule_versions WHERE version=4) OR EXISTS(SELECT FROM pg_attribute WHERE attrelid='game_rift_rule_versions'::regclass AND attname='stat_anchor_tier' AND NOT attisdropped) THEN RAISE EXCEPTION 'rollback leaked'; END IF; END $$;""")
schema=body.split('DO $rift_rebalance$')[0]
run('occupied-version-rejected','BEGIN;'+schema+clone(4)+body+'COMMIT;','version 4 is already occupied')
for field,value in [('stat_anchor_tier','151'),('normal_boss_equipment','21'),('normal_boss_equipment','-1')]:
 run('invalid-'+field+'-'+value,'BEGIN;'+schema+clone(4)+f'UPDATE game_rift_rule_versions SET {field}={value} WHERE version=4;ROLLBACK;','violates check constraint')
run('future-active-preserved','BEGIN;'+schema+clone(5)+"UPDATE game_rift_rule_versions SET published=true WHERE version=5;UPDATE game_rift_rule_active SET version=5 WHERE id=1;"+body+"DO $$ BEGIN IF (SELECT version FROM game_rift_rule_active WHERE id=1)<>5 THEN RAISE EXCEPTION 'future active overwritten';END IF;END $$;ROLLBACK;")
run('publish-and-replay','BEGIN;'+snapshot+body+body+assertions+'COMMIT;')
run('immutable-v4','BEGIN;UPDATE game_rift_rule_versions SET normal_boss_equipment=11 WHERE version=4;ROLLBACK;','immutable')
run('immutable-v1','BEGIN;UPDATE game_rift_rule_versions SET stat_anchor_tier=30 WHERE version=1;ROLLBACK;','immutable')
print('Migration78 verified; use this same idle cluster for the Go production loader.',flush=True)
