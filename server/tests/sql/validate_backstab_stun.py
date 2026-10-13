#!/usr/bin/env python3
"""Validate migration73 on an explicitly owned, idle disposable cluster in place.

No cluster copy, listener or production connection. The caller must supply an
unmigrated test cluster through72; it is left at73 for the actual Go loaders.
"""
import argparse
import os
from pathlib import Path
import re
import subprocess
from validate_daoist_postgres import write_single_user_commands

ROOT = Path(__file__).resolve().parents[3]
p = argparse.ArgumentParser()
p.add_argument('--work-dir', type=Path, required=True)
p.add_argument('--data-dir', type=Path, required=True, help='owned idle disposable cluster through72; modified in place')
a = p.parse_args()
w, data = a.work_dir.resolve(), a.data_dir.resolve()
w.mkdir(parents=True, exist_ok=False)
if not (data/'PG_VERSION').is_file() or (data/'postmaster.pid').exists():
    raise SystemExit('refusing missing or running/stale-postmaster test cluster')
bins = Path(os.environ['PG_BIN'])


def run(label, sql, expected_error=None):
    import io
    inp, log = w/(label+'.sql'), w/(label+'.log')
    with inp.open('w') as out:
        write_single_user_commands(io.StringIO(sql), out)
    with inp.open() as source, log.open('w') as out:
        result = subprocess.run([str(bins/'postgres'), '--single', '-j', '-D', str(data), 'postgres'], stdin=source, stdout=out, stderr=subprocess.STDOUT)
    errors = [x.strip() for x in log.read_text().splitlines() if re.search(r'\b(?:ERROR|FATAL|PANIC):', x)]
    if expected_error:
        assert any(expected_error in e for e in errors), (label, errors[:3])
    else:
        assert result.returncode == 0 and not errors, (label, errors[:3])
    print('PASS', label, flush=True)


body = (ROOT/'server/migrations/000000_init-73.sql').read_text()
empty = """DO $$ BEGIN
 IF EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10201 AND mechanic='backstab_stun') OR
 EXISTS(SELECT FROM game_skill_passive_trigger_statuses WHERE passive_skill_id=10201) OR
 (SELECT count(*) FROM gamedata.ov_skilldesc WHERE skill_id=10201 AND skill_level BETWEEN 1 AND 5
 AND desc_=format('背刺时20%%的几率不会中止隐身状态，%s%%几率击晕对手。',skill_level*5))<>5
 THEN RAISE EXCEPTION 'fixture already migrated or changed'; END IF;
END $$;"""
run('require-unmigrated', empty)
run('rollback-on-failure', 'BEGIN;\n'+body+"\nDO $$ BEGIN RAISE EXCEPTION 'deliberate rollback'; END $$; COMMIT;", 'deliberate rollback')
run('rollback-left-no-changes', empty)
for label, change, error in [
    ('missing-passive-rank', 'DELETE FROM gamedata.ov_skilldesc WHERE skill_id=10201 AND skill_level=2;', 'missing native Advanced Backstab'),
    ('missing-target-rank', 'DELETE FROM gamedata.ov_skilldesc WHERE skill_id=10209 AND skill_level=2;', 'missing native Advanced Backstab'),
    ('changed-stun-duration', 'UPDATE gamedata.ov_exceptdetail_entry SET last_time=4 WHERE row_no=4 AND idx=26;', 'native three-second pure stun'),
    ('missing-stun-control', 'DELETE FROM gamedata.ov_exceptdetail_effect_entry WHERE row_no=4 AND idx=26 AND attr_id=83;', 'native three-second pure stun'),
    ('added-stun-penalty', 'INSERT INTO gamedata.ov_exceptdetail_effect_entry(row_no,idx,idx2,op_type,attr_id,mode,prob,value) VALUES(4,26,99,1,19,1,100,-50);', 'native three-second pure stun'),
    ('changed-stun-icon', 'UPDATE gamedata.ov_exceptdesc SET level_small_maps[27]=0 WHERE type=1002;', 'native three-second pure stun'),
    ('custom-status-conflict', "INSERT INTO game_custom_statuses(status_id,status_level,name,description,status_type,duration_sec) VALUES(1002,27,'custom','custom',1,3);", 'native three-second pure stun'),
]:
    run('reject-'+label, 'BEGIN;\n'+change+'\n'+body+'\nCOMMIT;', error)
run('rejections-left-no-changes', empty)
run('preserve-admin-trigger', """BEGIN;
INSERT INTO game_skill_passive_trigger_statuses(passive_skill_id,passive_skill_level,trigger_order,target_skill_id,status_id,status_level,chance_bp,duration_sec)
VALUES(10201,1,1,10209,1002,27,4321,7);
"""+body+"""
DO $$ BEGIN
 IF (SELECT count(*) FROM game_skill_passive_trigger_statuses WHERE passive_skill_id=10201)<>1 OR
 NOT EXISTS(SELECT FROM game_skill_passive_trigger_statuses WHERE passive_skill_id=10201 AND chance_bp=4321 AND duration_sec=7) OR
 EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10201 AND mechanic='backstab_stun' AND imported) OR
 EXISTS(SELECT FROM gamedata.ov_skilldesc WHERE skill_id=10201 AND desc_ LIKE '%3秒%')
 THEN RAISE EXCEPTION 'existing administrator trigger changed'; END IF;
END $$; ROLLBACK;""")
run('preserve-admin-description', """BEGIN;
UPDATE gamedata.ov_skilldesc SET desc_='Administrator reviewed text' WHERE skill_id=10201 AND skill_level=2;
"""+body+"""
DO $$ BEGIN
 IF EXISTS(SELECT FROM game_skill_passive_trigger_statuses WHERE passive_skill_id=10201) OR
 NOT EXISTS(SELECT FROM gamedata.ov_skilldesc WHERE skill_id=10201 AND skill_level=2 AND desc_='Administrator reviewed text') OR
 EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10201 AND mechanic='backstab_stun' AND imported)
 THEN RAISE EXCEPTION 'administrator description changed'; END IF;
END $$; ROLLBACK;""")
tables = ['gamedata.ov_skilldesc', 'game_skill_passive_trigger_statuses', 'gamedata.ov_exceptdesc', 'gamedata.ov_exceptdetail', 'gamedata.ov_exceptdetail_entry', 'gamedata.ov_exceptdetail_effect_entry', 'game_custom_statuses', 'game_custom_status_affixes', 'game_skill_passive_modifiers', 'game_skill_statuses', 'game_skill_effects', 'game_skill_growth_policy', 'game_skill_progression', 'game_skill_synergies', 'characters', 'character_skills']
for i, table in enumerate(tables):
    run('snapshot-'+str(i), f'CREATE TABLE before_backstab73_{i} AS SELECT to_jsonb(t) AS row FROM {table} t;')
run('first-import', 'BEGIN;\n'+body+'\nCOMMIT;')
run('repeat-import', 'BEGIN;\n'+body+'\nCOMMIT;')
run('exact-five-trigger-rows', """DO $$ BEGIN
 IF (SELECT count(*) FROM game_skill_passive_trigger_statuses WHERE passive_skill_id=10201)<>5 OR
 (SELECT count(*) FROM game_skill_passive_trigger_statuses WHERE passive_skill_id=10201 AND passive_skill_level BETWEEN 1 AND 5
 AND trigger_order=1 AND target_skill_id=10209 AND status_id=1002 AND status_level=27 AND chance_bp=10000 AND duration_sec=3)<>5 OR
 NOT EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10201 AND mechanic='backstab_stun' AND imported)
 THEN RAISE EXCEPTION 'wrong backstab trigger rows'; END IF;
 IF EXISTS((SELECT row FROM before_backstab73_1 EXCEPT ALL SELECT to_jsonb(t) FROM game_skill_passive_trigger_statuses t WHERE passive_skill_id<>10201)
 UNION ALL (SELECT to_jsonb(t) FROM game_skill_passive_trigger_statuses t WHERE passive_skill_id<>10201 EXCEPT ALL SELECT row FROM before_backstab73_1))
 THEN RAISE EXCEPTION 'unrelated trigger changed'; END IF;
END $$;""")
run('only-approved-description-numbers-change', """DO $$ BEGIN
 IF EXISTS(
 WITH expected AS (SELECT CASE WHEN (row->>'skill_id')::int=10201 AND (row->>'skill_level')::int BETWEEN 1 AND 5
 THEN jsonb_set(row,'{desc_}',to_jsonb('背刺时20%的几率不会中止隐身状态，100%几率击晕对手3秒。'::text)) ELSE row END row FROM before_backstab73_0)
 (SELECT row FROM expected EXCEPT ALL SELECT to_jsonb(t) FROM gamedata.ov_skilldesc t)
 UNION ALL (SELECT to_jsonb(t) FROM gamedata.ov_skilldesc t EXCEPT ALL SELECT row FROM expected))
 THEN RAISE EXCEPTION 'unapproved original skill change'; END IF;
END $$;""")
for i, table in enumerate(tables[2:], 2):
    run('preserve-'+str(i), f"DO $$ BEGIN IF EXISTS((SELECT row FROM before_backstab73_{i} EXCEPT ALL SELECT to_jsonb(t) FROM {table} t) UNION ALL (SELECT to_jsonb(t) FROM {table} t EXCEPT ALL SELECT row FROM before_backstab73_{i})) THEN RAISE EXCEPTION 'unrelated data changed: {table}'; END IF; END $$;")
run('replay-preserves-admin-edits-and-deletions', """BEGIN;
UPDATE game_skill_passive_trigger_statuses SET chance_bp=2345,duration_sec=4 WHERE passive_skill_id=10201 AND passive_skill_level=1;
DELETE FROM game_skill_passive_trigger_statuses WHERE passive_skill_id=10201 AND passive_skill_level=2;
UPDATE gamedata.ov_skilldesc SET desc_='Later administrator text' WHERE skill_id=10201 AND skill_level=3;
"""+body+"""
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM game_skill_passive_trigger_statuses WHERE passive_skill_id=10201 AND passive_skill_level=1 AND chance_bp=2345 AND duration_sec=4) OR
 EXISTS(SELECT FROM game_skill_passive_trigger_statuses WHERE passive_skill_id=10201 AND passive_skill_level=2) OR
 NOT EXISTS(SELECT FROM gamedata.ov_skilldesc WHERE skill_id=10201 AND skill_level=3 AND desc_='Later administrator text')
 THEN RAISE EXCEPTION 'replay changed administrator configuration'; END IF;
END $$; ROLLBACK;""")
print('BACKSTAB_STUN_POSTGRES_PASS', flush=True)
print('FANTASY_BACKSTAB_STUN_PG_DATA='+str(data), flush=True)
