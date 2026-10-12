#!/usr/bin/env python3
"""Disposable PostgreSQL migration 58 checks; never connects to a live server."""
import argparse
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

def run(label, sql=None, paths=None):
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
    assert result.returncode == 0 and not errors, (label, errors[:3])
    inp.unlink()
    print('PASS', label, flush=True)

run('fresh-through-57', paths=[ROOT / f'server/migrations/000000_init-{i}.sql' for i in range(58)])
tables = ['game_skill_passive_status_resists', 'game_custom_statuses', 'game_skill_statuses', 'game_skill_passive_trigger_statuses', 'game_monster_skill_effects', 'gamedata.ov_exceptdesc', 'characters']
for i, table in enumerate(tables):
    run('snapshot-' + str(i), sql=f'CREATE TABLE before_{i} AS SELECT to_jsonb(t) AS row FROM {table} t;')
patch = ROOT / 'server/migrations/000000_init-58.sql'
run('migration-58-first', paths=[patch])
run('migration-58-repeat', paths=[patch])
for i, table in enumerate(tables):
    run('preserve-' + str(i), sql=f"DO $$ BEGIN IF EXISTS((SELECT row FROM before_{i} EXCEPT ALL SELECT to_jsonb(t) FROM {table} t) UNION ALL (SELECT to_jsonb(t) FROM {table} t EXCEPT ALL SELECT row FROM before_{i})) THEN RAISE EXCEPTION 'rows modified'; END IF; END $$;")
run('uncapped-and-explicit-zero', sql='''
UPDATE game_skill_passive_status_resists SET resist_pct=300;
UPDATE game_custom_statuses SET physical_damage_taken_pct=300,magic_damage_taken_pct=300;
UPDATE game_skill_statuses SET chance_bp=0;
UPDATE game_skill_passive_trigger_statuses SET chance_bp=0;
UPDATE game_monster_skill_effects SET chance_bp=0;
DO $$ BEGIN
  BEGIN UPDATE game_skill_statuses SET chance_bp=-1; RAISE EXCEPTION 'negative chance accepted'; EXCEPTION WHEN check_violation THEN NULL; END;
  BEGIN UPDATE game_skill_statuses SET chance_bp=10001; RAISE EXCEPTION 'chance above 100 accepted'; EXCEPTION WHEN check_violation THEN NULL; END;
  BEGIN UPDATE game_skill_passive_status_resists SET resist_pct=-1; RAISE EXCEPTION 'negative resistance accepted'; EXCEPTION WHEN check_violation THEN NULL; END;
  BEGIN UPDATE game_custom_statuses SET physical_damage_taken_pct=-1; RAISE EXCEPTION 'negative vulnerability accepted'; EXCEPTION WHEN check_violation THEN NULL; END;
END $$;
''')
print('EFFECT_CONTEST_V7_POSTGRES_PASS', flush=True)
