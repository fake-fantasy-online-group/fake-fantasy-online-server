#!/usr/bin/env python3
"""Validate migration66 only in a fresh/copy-isolated socket-free PostgreSQL."""
import argparse
import os
from pathlib import Path
import re
import shutil
import subprocess
from validate_daoist_postgres import concatenate_sources, write_single_user_commands

ROOT = Path(__file__).resolve().parents[3]
p = argparse.ArgumentParser()
p.add_argument('--work-dir', required=True, type=Path)
p.add_argument('--seed-dir', type=Path, help='idle disposable cluster through at least migration62')
a = p.parse_args()
w = a.work_dir.resolve()
w.mkdir(parents=True, exist_ok=False)
bins = Path(os.environ['PG_BIN'])
data = w / 'data'
if a.seed_dir:
    seed = a.seed_dir.resolve()
    if (seed / 'postmaster.pid').exists():
        raise SystemExit('refusing running/stale-postmaster fixture')
    shutil.copytree(seed, data)
else:
    subprocess.run([str(bins/'initdb'), '-D', str(data), '--no-locale', '-E', 'UTF8', '-U', 'validator', '-A', 'trust'], check=True, stdout=(w/'init.log').open('w'), stderr=subprocess.STDOUT)

def run(label, sql=None, paths=None, expected_error=None):
    raw, inp, log = [w/(label+x) for x in ('.source.sql','.sql','.log')]
    with raw.open('wb') as f:
        if paths:
            f.write(b'BEGIN;\n')
            concatenate_sources(paths,f)
            f.write(b'\nCOMMIT;\n')
        else:
            f.write(sql.encode())
    with raw.open() as source, inp.open('w') as target:
        write_single_user_commands(source,target)
    raw.unlink()
    with inp.open() as source, log.open('w') as out:
        result = subprocess.run([str(bins/'postgres'),'--single','-j','-D',str(data),'postgres'],stdin=source,stdout=out,stderr=subprocess.STDOUT)
    errors = [x.strip() for x in log.read_text().splitlines() if re.search(r'\b(?:ERROR|FATAL|PANIC):',x)]
    if expected_error:
        assert any(expected_error in e for e in errors),(label,errors[:3])
    else:
        assert result.returncode == 0 and not errors,(label,errors[:3])
    inp.unlink()
    print('PASS',label,flush=True)

if not a.seed_dir:
    run('fresh-through65',paths=[ROOT/f'server/migrations/000000_init-{i}.sql' for i in range(66)])
body = (ROOT/'server/migrations/000000_init-66.sql').read_text()
run('require-unmigrated',sql="""DO $$ BEGIN IF to_regclass('game_skill_mechanism_sources') IS NOT NULL THEN RAISE EXCEPTION 'fixture already migrated'; END IF; END $$;""")
run('rollback-on-failure',sql='BEGIN;\n'+body+"\nDO $$ BEGIN RAISE EXCEPTION 'deliberate rollback'; END $$; COMMIT;",expected_error='deliberate rollback')
run('rollback-left-no-schema',sql="DO $$ BEGIN IF to_regclass('game_skill_mechanism_sources') IS NOT NULL THEN RAISE EXCEPTION 'failed transaction leaked'; END IF; END $$;")
run('preserve-preexisting-admin-skill',sql="""BEGIN;
INSERT INTO game_skill_statuses(skill_id,skill_level,status_order,status_id,status_level,chance_bp,duration_sec)
 VALUES(10317,1,1,1008,2,3210,17);
"""+body+"""
DO $$ BEGIN
 IF (SELECT count(*) FROM game_skill_statuses WHERE skill_id=10317)<>1 OR
 NOT EXISTS(SELECT FROM game_skill_statuses WHERE skill_id=10317 AND chance_bp=3210 AND duration_sec=17) OR
 EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10317 AND imported)
 THEN RAISE EXCEPTION 'existing administrator skill changed'; END IF;
END $$;
ROLLBACK;
""")
tables = ['gamedata.ov_skilldesc','gamedata.ov_exceptdesc','gamedata.ov_exceptdetail','gamedata.ov_exceptdetail_entry','gamedata.ov_exceptdetail_effect_entry','game_skill_effects','characters','character_skills']
for i,table in enumerate(tables):
    run('snapshot-'+str(i),sql=f'CREATE TABLE before_mechanisms_{i} AS SELECT to_jsonb(t) row FROM {table} t;')
run('first-import',sql='BEGIN;\n'+body+'\nCOMMIT;')
run('repeat-import',sql='BEGIN;\n'+body+'\nCOMMIT;')
for i,table in enumerate(tables):
    run('preserve-'+str(i),sql=f"DO $$ BEGIN IF EXISTS((SELECT row FROM before_mechanisms_{i} EXCEPT ALL SELECT to_jsonb(t) FROM {table} t) UNION ALL (SELECT to_jsonb(t) FROM {table} t EXCEPT ALL SELECT row FROM before_mechanisms_{i})) THEN RAISE EXCEPTION 'original data changed'; END IF; END $$;")
run('complete-scoped-config',sql="""DO $$ BEGIN
 IF (SELECT count(*) FROM game_skill_statuses WHERE configuration_version=66)<>20 OR
 (SELECT count(*) FROM game_custom_statuses WHERE configuration_version=66)<>11 OR
 (SELECT count(*) FROM game_skill_mechanism_sources WHERE imported)<>2 OR
 EXISTS(SELECT FROM game_custom_statuses WHERE configuration_version=66 AND (icon<=0 OR NOT icon_known))
 THEN RAISE EXCEPTION 'scoped configuration coverage'; END IF;
 IF EXISTS(SELECT FROM game_custom_statuses c JOIN game_custom_status_affixes a USING(status_id,status_level)
 WHERE c.configuration_version=66 AND c.status_id=1034)
 THEN RAISE EXCEPTION 'root imported unrelated stat penalty'; END IF;
END $$;""")
run('replay-preserves-admin-edits-and-deletions',sql="""BEGIN;
UPDATE game_skill_statuses SET chance_bp=1234 WHERE skill_id=10326 AND skill_level=1 AND status_order=1;
DELETE FROM game_skill_statuses WHERE skill_id=10317 AND skill_level=2 AND status_order=2;
DELETE FROM game_custom_status_affixes WHERE status_id=1008 AND status_level=21;
DELETE FROM game_status_overlay_strengths WHERE status_id=1008;
"""+body+"""
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM game_skill_statuses WHERE skill_id=10326 AND skill_level=1 AND status_order=1 AND chance_bp=1234) OR
 EXISTS(SELECT FROM game_skill_statuses WHERE skill_id=10317 AND skill_level=2 AND status_order=2) OR
 EXISTS(SELECT FROM game_custom_status_affixes WHERE status_id=1008 AND status_level=21) OR
 EXISTS(SELECT FROM game_status_overlay_strengths WHERE status_id=1008)
 THEN RAISE EXCEPTION 'replay changed administrator configuration'; END IF;
END $$; ROLLBACK;""")
run('reject-unknown-target-type',sql="BEGIN; UPDATE game_skill_statuses SET target_type='guess' WHERE skill_id=10317; COMMIT;",expected_error='check constraint')
run('reject-negative-logical-level',sql="BEGIN; UPDATE game_custom_statuses SET legacy_overlay_level_bp=-1 WHERE status_id=1008 AND status_level=21; COMMIT;",expected_error='check constraint')
run('reject-overflow-logical-level',sql="BEGIN; UPDATE game_custom_statuses SET legacy_overlay_level_bp=9223372036854775808 WHERE status_id=1008 AND status_level=21; COMMIT;",expected_error='out of range')
run('reject-invalid-strength-order',sql="BEGIN; UPDATE game_status_overlay_strengths SET value_order=0 WHERE status_id=1008; COMMIT;",expected_error='check constraint')
run('reject-invalid-strength-mode',sql="BEGIN; UPDATE game_status_overlay_strengths SET mode=2 WHERE status_id=1008; COMMIT;",expected_error='check constraint')
print('SKILL_MECHANISMS_POSTGRES_PASS',flush=True)
print('FANTASY_SCOPED_SKILLS_PG_DATA='+str(data),flush=True)
