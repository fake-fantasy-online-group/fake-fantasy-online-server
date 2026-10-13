#!/usr/bin/env python3
"""Verify migration68 only in a newly created/copied disposable PostgreSQL cluster."""
import argparse
import os
from pathlib import Path
import re
import shutil
import subprocess
from validate_daoist_postgres import concatenate_sources, write_single_user_commands

ROOT = Path(__file__).resolve().parents[3]
p = argparse.ArgumentParser()
p.add_argument('--work-dir', type=Path, required=True)
p.add_argument('--seed-dir', type=Path, help='idle disposable cluster through migration65, without migration66')
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
    raw, inp, log = [w / (label + suffix) for suffix in ('.source.sql', '.sql', '.log')]
    with raw.open('wb') as f:
        if paths:
            f.write(b'BEGIN;\n')
            concatenate_sources(paths, f)
            f.write(b'\nCOMMIT;\n')
        else:
            f.write(sql.encode())
    with raw.open() as source, inp.open('w') as target:
        write_single_user_commands(source, target)
    raw.unlink()
    with inp.open() as source, log.open('w') as out:
        result = subprocess.run([str(bins/'postgres'), '--single', '-j', '-D', str(data), 'postgres'], stdin=source, stdout=out, stderr=subprocess.STDOUT)
    errors = [x.strip() for x in log.read_text().splitlines() if re.search(r'\b(?:ERROR|FATAL|PANIC):', x)]
    if expected_error:
        assert any(expected_error in e for e in errors), (label, errors[:3])
    else:
        assert result.returncode == 0 and not errors, (label, errors[:3])
    inp.unlink()
    print('PASS', label, flush=True)


start = 66 if a.seed_dir else 0
run('initialize-through67', paths=[ROOT / f'server/migrations/000000_init-{i}.sql' for i in range(start, 68)])
body = (ROOT/'server/migrations/000000_init-68.sql').read_text()
run('require-original-radius-fixture', sql="""DO $$ BEGIN
 IF EXISTS(SELECT FROM game_skill_mechanism_sources WHERE mechanic='original_radius') OR
    (SELECT count(*) FROM gamedata.ov_skilldesc WHERE skill_id IN (10028,10127,10313) AND skill_level BETWEEN 1 AND 5 AND radius=200)<>15
 THEN RAISE EXCEPTION 'fixture already corrected or changed'; END IF;
END $$;""")
run('rollback-on-failure', sql='BEGIN;\n'+body+"\nDO $$ BEGIN RAISE EXCEPTION 'deliberate rollback'; END $$; COMMIT;", expected_error='deliberate rollback')
run('rollback-left-no-changes', sql="""DO $$ BEGIN
 IF EXISTS(SELECT FROM game_skill_mechanism_sources WHERE mechanic='original_radius') OR
    (SELECT count(*) FROM gamedata.ov_skilldesc WHERE skill_id IN (10028,10127,10313) AND skill_level BETWEEN 1 AND 5 AND radius=200)<>15
 THEN RAISE EXCEPTION 'failed transaction leaked'; END IF;
END $$;""")
run('reject-missing-original-rank', sql="BEGIN; DELETE FROM gamedata.ov_skilldesc WHERE skill_id=10028 AND skill_level=2;\n"+body+'\nCOMMIT;', expected_error='missing original radius anchors')
run('preserve-preexisting-admin-radius', sql="""BEGIN;
UPDATE gamedata.ov_skilldesc SET radius=123 WHERE skill_id IN (10028,10127,10313) AND skill_level=2;
"""+body+"""
DO $$ BEGIN
 IF (SELECT count(*) FROM gamedata.ov_skilldesc WHERE skill_id IN (10028,10127,10313) AND skill_level=2 AND radius=123)<>3 OR
 (SELECT count(*) FROM gamedata.ov_skilldesc WHERE skill_id IN (10028,10127,10313) AND skill_level IN (1,3,4,5) AND radius=CASE WHEN skill_id=10313 THEN 150 ELSE 100 END)<>12
 THEN RAISE EXCEPTION 'administrator radius changed or ordinary anchor not restored'; END IF;
END $$; ROLLBACK;
""")
tables = ['gamedata.ov_skilldesc', 'gamedata.ov_exceptdesc', 'gamedata.ov_exceptdetail', 'gamedata.ov_exceptdetail_entry', 'gamedata.ov_exceptdetail_effect_entry', 'game_skill_effects', 'game_skill_statuses', 'game_custom_statuses', 'game_custom_status_affixes', 'game_skill_growth_policy', 'game_skill_progression', 'game_skill_synergies', 'characters', 'character_skills']
for i, table in enumerate(tables):
    run('snapshot-'+str(i), sql=f'CREATE TABLE before_original_ranges_{i} AS SELECT to_jsonb(t) AS row FROM {table} t;')
run('first-import', sql='BEGIN;\n'+body+'\nCOMMIT;')
run('repeat-import', sql='BEGIN;\n'+body+'\nCOMMIT;')
run('only-fifteen-radius-fields-change', sql="""DO $$ BEGIN
 IF EXISTS(
  WITH expected AS (
   SELECT CASE WHEN (row->>'skill_id')::integer IN (10028,10127,10313) AND (row->>'skill_level')::integer BETWEEN 1 AND 5
    THEN jsonb_set(row,'{radius}',to_jsonb(CASE WHEN (row->>'skill_id')::integer=10313 THEN 150 ELSE 100 END)) ELSE row END AS row
   FROM before_original_ranges_0
  )
  (SELECT row FROM expected EXCEPT ALL SELECT to_jsonb(t) FROM gamedata.ov_skilldesc t)
  UNION ALL
  (SELECT to_jsonb(t) FROM gamedata.ov_skilldesc t EXCEPT ALL SELECT row FROM expected)
 ) THEN RAISE EXCEPTION 'unexpected native skill change'; END IF;
 IF (SELECT count(*) FROM game_skill_mechanism_sources WHERE mechanic='original_radius' AND imported)<>3 OR
    (SELECT count(*) FROM game_skill_mechanism_sources WHERE mechanic='target_scoped_status' AND imported)<>2
 THEN RAISE EXCEPTION 'source ledger coverage'; END IF;
END $$;""")
for i, table in enumerate(tables[1:], 1):
    run('preserve-'+str(i), sql=f"DO $$ BEGIN IF EXISTS((SELECT row FROM before_original_ranges_{i} EXCEPT ALL SELECT to_jsonb(t) FROM {table} t) UNION ALL (SELECT to_jsonb(t) FROM {table} t EXCEPT ALL SELECT row FROM before_original_ranges_{i})) THEN RAISE EXCEPTION 'unrelated data changed: {table}'; END IF; END $$;")
run('replay-preserves-admin-edits-and-deletions', sql="""BEGIN;
UPDATE gamedata.ov_skilldesc SET radius=200 WHERE skill_id=10028 AND skill_level=1;
UPDATE gamedata.ov_skilldesc SET radius=123 WHERE skill_id=10127 AND skill_level=2;
DELETE FROM gamedata.ov_skilldesc WHERE skill_id=10313 AND skill_level=3;
"""+body+"""
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM gamedata.ov_skilldesc WHERE skill_id=10028 AND skill_level=1 AND radius=200) OR
 NOT EXISTS(SELECT FROM gamedata.ov_skilldesc WHERE skill_id=10127 AND skill_level=2 AND radius=123) OR
 EXISTS(SELECT FROM gamedata.ov_skilldesc WHERE skill_id=10313 AND skill_level=3)
 THEN RAISE EXCEPTION 'replay changed administrator configuration'; END IF;
END $$; ROLLBACK;""")
print('ORIGINAL_SKILL_RANGES_POSTGRES_PASS', flush=True)
print('FANTASY_ORIGINAL_RANGES_PG_DATA='+str(data), flush=True)
