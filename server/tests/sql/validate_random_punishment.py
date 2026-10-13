#!/usr/bin/env python3
"""Validate project-designed random statuses on an isolated migration68 copy."""
import argparse
import os
from pathlib import Path
import re
import shutil
import subprocess
from validate_daoist_postgres import write_single_user_commands

ROOT = Path(__file__).resolve().parents[3]
p = argparse.ArgumentParser()
p.add_argument('--work-dir', required=True, type=Path)
p.add_argument('--seed-dir', required=True, type=Path)
a = p.parse_args()
w = a.work_dir.resolve()
w.mkdir(parents=True, exist_ok=False)
seed, data = a.seed_dir.resolve(), w / 'data'
if (seed / 'postmaster.pid').exists():
    raise SystemExit('refusing running/stale-postmaster fixture')
shutil.copytree(seed, data)
bins = Path(os.environ['PG_BIN'])

def run(label, sql, expected=None):
    raw, inp, log = [w / (label+x) for x in ('.source.sql', '.sql', '.log')]
    raw.write_text(sql)
    with raw.open() as src, inp.open('w') as dst:
        write_single_user_commands(src, dst)
    raw.unlink()
    with inp.open() as src, log.open('w') as out:
        r = subprocess.run([str(bins/'postgres'), '--single', '-j', '-D', str(data), 'postgres'], stdin=src, stdout=out, stderr=subprocess.STDOUT)
    errors = [x.strip() for x in log.read_text().splitlines() if re.search(r'\b(?:ERROR|FATAL|PANIC):', x)]
    assert (any(expected in e for e in errors) if expected else r.returncode == 0 and not errors), (label, errors[:3])
    inp.unlink()
    print('PASS', label, flush=True)

body = (ROOT/'server/migrations/000000_init-72.sql').read_text()
run('require-unmigrated', "DO $$ BEGIN IF EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10313 AND mechanic='random_status_pool') THEN RAISE EXCEPTION 'fixture already migrated'; END IF; END $$;")
run('rollback', 'BEGIN;\n'+body+"\nDO $$ BEGIN RAISE EXCEPTION 'deliberate rollback'; END $$; COMMIT;", 'deliberate rollback')
run('rollback-clean', "DO $$ BEGIN IF EXISTS(SELECT FROM information_schema.columns WHERE table_name='game_skill_statuses' AND column_name='random_group') OR EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10313 AND mechanic='random_status_pool') THEN RAISE EXCEPTION 'rollback leaked'; END IF; END $$;")
run('preserve-preexisting-admin', "BEGIN; INSERT INTO game_skill_statuses(skill_id,skill_level,status_order,status_id,status_level,chance_bp,duration_sec) VALUES(10313,1,1,1008,2,1234,17);\n"+body+"\nDO $$ BEGIN IF (SELECT count(*) FROM game_skill_statuses WHERE skill_id=10313)<>1 OR NOT EXISTS(SELECT FROM game_skill_statuses WHERE skill_id=10313 AND chance_bp=1234 AND duration_sec=17 AND random_group=0) OR EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10313 AND mechanic='random_status_pool' AND imported) THEN RAISE EXCEPTION 'admin config overwritten'; END IF; END $$; ROLLBACK;")
run('missing-root-fails-closed', "BEGIN; DELETE FROM game_custom_statuses WHERE status_id=1034 AND status_level=31;\n"+body+'\nCOMMIT;', 'pure-root status definitions missing')
run('augmented-root-fails-closed', "BEGIN; UPDATE game_custom_statuses SET damage_shield=100 WHERE status_id=1034 AND status_level=31;\n"+body+'\nCOMMIT;', 'pure-root status definitions missing')
tables = ['gamedata.ov_skilldesc', 'gamedata.ov_exceptdesc', 'gamedata.ov_exceptdetail', 'gamedata.ov_exceptdetail_entry', 'gamedata.ov_exceptdetail_effect_entry', 'game_custom_statuses', 'game_custom_status_affixes', 'game_skill_effects', 'game_skill_area_rules', 'characters', 'character_skills']
for i, table in enumerate(tables):
    run('snapshot-'+str(i), f'CREATE TABLE before_random72_{i} AS SELECT to_jsonb(t) row FROM {table} t;')
run('snapshot-other-apps', "CREATE TABLE before_random72_apps AS SELECT to_jsonb(t) row FROM game_skill_statuses t;")
run('first-import', 'BEGIN;\n'+body+'\nCOMMIT;')
run('repeat-import', 'BEGIN;\n'+body+'\nCOMMIT;')
for i, table in enumerate(tables):
    run('preserve-'+str(i), f"DO $$ BEGIN IF EXISTS((SELECT row FROM before_random72_{i} EXCEPT ALL SELECT to_jsonb(t) FROM {table} t) UNION ALL (SELECT to_jsonb(t) FROM {table} t EXCEPT ALL SELECT row FROM before_random72_{i})) THEN RAISE EXCEPTION 'original rows changed'; END IF; END $$;")
run('preserve-other-apps', "DO $$ BEGIN IF EXISTS((SELECT row FROM before_random72_apps EXCEPT ALL SELECT to_jsonb(t)-'random_group'-'random_weight' FROM game_skill_statuses t WHERE skill_id<>10313) UNION ALL (SELECT to_jsonb(t)-'random_group'-'random_weight' FROM game_skill_statuses t WHERE skill_id<>10313 EXCEPT ALL SELECT row FROM before_random72_apps)) THEN RAISE EXCEPTION 'other skill changed'; END IF; END $$;")
run('pool-shape', """DO $$ BEGIN
 IF (SELECT count(*) FROM game_skill_statuses WHERE skill_id=10313)<>20 OR
    EXISTS(SELECT FROM game_skill_statuses WHERE skill_id=10313 AND (chance_bp<>4000 OR random_group<>1 OR random_weight<>1 OR target_type<>'any' OR configuration_version<>72 OR status_id NOT IN (1006,1007,1008,1034) OR duration_sec<>skill_level+CASE WHEN status_id=1034 THEN 2 ELSE 5 END OR status_level<>skill_level+CASE WHEN status_id=1034 THEN 30 WHEN status_id=1008 THEN 1 ELSE 0 END)) OR
    NOT EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10313 AND mechanic='random_status_pool' AND imported)
 THEN RAISE EXCEPTION 'wrong random pool'; END IF;
END $$;""")
run('replay-preserves-admin-edits-deletes', "BEGIN; UPDATE game_skill_statuses SET chance_bp=1234 WHERE skill_id=10313 AND skill_level=1; DELETE FROM game_skill_statuses WHERE skill_id=10313 AND skill_level=2 AND status_id=1008;\n"+body+"\nDO $$ BEGIN IF (SELECT count(*) FROM game_skill_statuses WHERE skill_id=10313 AND skill_level=1 AND chance_bp=1234)<>4 OR EXISTS(SELECT FROM game_skill_statuses WHERE skill_id=10313 AND skill_level=2 AND status_id=1008) THEN RAISE EXCEPTION 'replay changed administrator configuration'; END IF; END $$; ROLLBACK;")
for label, clause in [('negative-group','random_group=-1'), ('zero-weight','random_weight=0'), ('unpaired-weight','random_group=0')]:
    run('reject-'+label, f'BEGIN; UPDATE game_skill_statuses SET {clause} WHERE skill_id=10313; COMMIT;', 'check constraint')
print('RANDOM_PUNISHMENT_POSTGRES_PASS', flush=True)
print('FANTASY_RANDOM_PUNISHMENT_PG_DATA='+str(data), flush=True)
