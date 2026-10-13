#!/usr/bin/env python3
"""Verify migration 62 in a newly created disposable PostgreSQL cluster only."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
from validate_daoist_postgres import concatenate_sources, write_single_user_commands

ROOT = Path(__file__).resolve().parents[3]
p = argparse.ArgumentParser()
p.add_argument('--work-dir', type=Path, required=True)
a = p.parse_args()
w = a.work_dir.resolve()
w.mkdir(parents=True, exist_ok=False)
bins = Path(os.environ['PG_BIN'])
data = w / 'data'
subprocess.run([str(bins / 'initdb'), '-D', str(data), '--no-locale', '-E', 'UTF8', '-U', 'validator', '-A', 'trust'], check=True, stdout=(w / 'init.log').open('w'), stderr=subprocess.STDOUT)


def run(label, sql=None, paths=None, expected_error=None):
    raw, inp, log = [w / (label + suffix) for suffix in ('.source.sql', '.sql', '.log')]
    with raw.open('wb') as f:
        if paths:
            concatenate_sources(paths, f)
        else:
            f.write(sql.encode())
    with raw.open() as source, inp.open('w') as target:
        write_single_user_commands(source, target)
    raw.unlink()
    with inp.open() as source, log.open('w') as out:
        result = subprocess.run([str(bins / 'postgres'), '--single', '-j', '-D', str(data), 'postgres'], stdin=source, stdout=out, stderr=subprocess.STDOUT)
    errors = [x.strip() for x in log.read_text().splitlines() if re.search(r'\b(?:ERROR|FATAL|PANIC):', x)]
    if expected_error:
        assert any(expected_error in e for e in errors), (label, errors[:3])
    else:
        assert result.returncode == 0 and not errors, (label, errors[:3])
    inp.unlink()
    print('PASS', label, flush=True)


run('fresh-through-59', paths=[ROOT / f'server/migrations/000000_init-{i}.sql' for i in range(60)])
patch = ROOT / 'server/migrations/000000_init-62.sql'
body = patch.read_text()
run('seed-existing-character', sql="""
INSERT INTO accounts(id,username) VALUES(930620001,'before_skill_config');
INSERT INTO characters(id,account_id,slot,name,race,map_id,pos_x,pos_y,level,skill_points)
 VALUES(930620001,930620001,0,'before_skill_config',1,7,10,20,60,29);
INSERT INTO character_skills(char_id,skill_id,level) VALUES(930620001,10002,20),(930620001,10009,10);
""")
tables = ['gamedata.ov_skilldesc','gamedata.ov_exceptdesc','gamedata.ov_exceptdetail','characters','character_skills']
for i, table in enumerate(tables):
 run('snapshot-'+str(i), sql=f'CREATE TABLE IF NOT EXISTS before_skills_{i} AS SELECT to_jsonb(t) AS row FROM {table} t;')
run('migration-62-first', sql='BEGIN;\n'+body+'\nCOMMIT;')
run('migration-62-repeat', sql='BEGIN;\n'+body+'\nCOMMIT;')
for i, table in enumerate(tables):
 run('preserve-'+str(i), sql=f"DO $$ BEGIN IF EXISTS((SELECT row FROM before_skills_{i} EXCEPT ALL SELECT to_jsonb(t) FROM {table} t) UNION ALL (SELECT to_jsonb(t) FROM {table} t EXCEPT ALL SELECT row FROM before_skills_{i})) THEN RAISE EXCEPTION 'original rows modified in {table}'; END IF; END $$;")
run('typed-config-counts', sql="""DO $$ BEGIN
 IF (SELECT count(*) FROM game_skill_progression) <>149 OR (SELECT count(*) FROM game_skill_growth_policy) <>149 OR (SELECT count(*) FROM game_skill_synergies) <>128 OR (SELECT count(*) FROM game_skill_prerequisite_overrides) <>1 OR (SELECT count(*) FROM game_custom_statuses WHERE configuration_version=62) <>30 OR (SELECT count(*) FROM game_skill_statuses WHERE configuration_version=62) <>35 THEN RAISE EXCEPTION 'configuration coverage'; END IF;
END $$;""")
run('replay-preserves-admin-configuration',sql="""BEGIN;
UPDATE game_skill_growth_policy SET efficiency_cap_rank=77 WHERE skill_id=10018;
DELETE FROM game_skill_synergies WHERE edge_order=1;
DELETE FROM game_custom_status_affixes WHERE status_id=10009 AND status_level=1;
"""+body+"""
DO $$ BEGIN
 IF (SELECT efficiency_cap_rank FROM game_skill_growth_policy WHERE skill_id=10018) <>77 OR EXISTS(SELECT FROM game_skill_synergies WHERE edge_order=1) OR EXISTS(SELECT FROM game_custom_status_affixes WHERE status_id=10009 AND status_level=1) THEN RAISE EXCEPTION 'replay changed administrator configuration'; END IF;
END $$;
ROLLBACK;""")
for column in ['efficiency_cap_rank','range_cap_rank','protection_cap_rank','attack_cap_rank']:
 run('reject-cap-'+column,sql=f'BEGIN; UPDATE game_skill_growth_policy SET {column}=4 WHERE skill_id=10018; COMMIT;',expected_error='check constraint')
print('SKILL_CONFIGURATION_POSTGRES_PASS',flush=True)
print('Read-only loader: FANTASY_SKILL_CONFIG_PG_DATA='+str(data),flush=True)
