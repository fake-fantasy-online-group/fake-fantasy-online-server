#!/usr/bin/env python3
"""Validate migration74 in an explicitly owned idle disposable cluster through73.
No listener, production connection or cluster copy. Leaves this fixture at74.
"""
import argparse
import io
import os
from pathlib import Path
import re
import subprocess
from validate_daoist_postgres import write_single_user_commands

ROOT = Path(__file__).resolve().parents[3]
p = argparse.ArgumentParser()
p.add_argument('--work-dir', type=Path, required=True)
p.add_argument('--data-dir', type=Path, required=True)
a = p.parse_args()
w, data = a.work_dir.resolve(), a.data_dir.resolve()
w.mkdir(parents=True, exist_ok=False)
if not (data/'PG_VERSION').is_file() or (data/'postmaster.pid').exists():
    raise SystemExit('refusing missing or running/stale-postmaster test cluster')
bins = Path(os.environ['PG_BIN'])

def run(label, sql, expected_error=None):
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

body = (ROOT/'server/migrations/000000_init-74.sql').read_text()
empty = """DO $$ BEGIN
 IF to_regclass('game_skill_synergy_update_history') IS NOT NULL OR
 (SELECT count(*) FROM game_skill_synergies WHERE rank_mode='hard')<>70 OR
 (SELECT count(*) FROM game_skill_synergies WHERE rank_mode='effective_raw')<>58
 THEN RAISE EXCEPTION 'fixture already migrated or changed'; END IF;
END $$;"""
run('require-unmigrated', empty)
run('rollback-on-failure', 'BEGIN;\n'+body+"\nDO $$ BEGIN RAISE EXCEPTION 'deliberate rollback'; END $$; COMMIT;", 'deliberate rollback')
run('rollback-left-no-changes', empty)
for label,change,error in [
 ('missing-edge','DELETE FROM game_skill_synergies WHERE edge_order=1;','complete skill configuration'),
 ('missing-learning-level','DELETE FROM gamedata.ov_skilldesc WHERE skill_id=10401 AND skill_level=1;','original beneficiary learning levels'),
 ('invalid-learning-level','UPDATE gamedata.ov_skilldesc SET level_need=0 WHERE skill_id=10401 AND skill_level=1;','original beneficiary learning levels'),
]:
    run('reject-'+label,'BEGIN;\n'+change+'\n'+body+'\nCOMMIT;',error)
run('rejections-left-no-changes', empty)
run('preserve-existing-admin-coefficient', "BEGIN; UPDATE game_skill_synergies SET per_point_bp=175 WHERE target_skill_id=10401 AND rank_mode='hard';\n"+body+"""
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM game_skill_synergies WHERE target_skill_id=10401 AND rank_mode='hard' AND per_point_bp=175) OR
 (SELECT updated_edges FROM game_skill_synergy_update_history WHERE configuration_version=74)<>69
 THEN RAISE EXCEPTION 'administrator coefficient overwritten'; END IF;
END $$; ROLLBACK;""")
tables=['gamedata.ov_skilldesc','gamedata.ov_exceptdesc','gamedata.ov_exceptdetail','gamedata.ov_exceptdetail_entry','gamedata.ov_exceptdetail_effect_entry','game_custom_statuses','game_custom_status_affixes','game_skill_passive_modifiers','game_skill_passive_trigger_statuses','game_skill_statuses','game_skill_effects','game_skill_growth_policy','game_skill_progression','game_skill_prerequisite_overrides','game_skill_configuration_manifest','characters','character_skills']
for i,table in enumerate(tables):
    run('snapshot-'+str(i),f'CREATE TABLE before_synergy74_{i} AS SELECT to_jsonb(t) AS row FROM {table} t;')
run('snapshot-synergies','CREATE TABLE before_synergy74_edges AS SELECT * FROM game_skill_synergies;')
run('first-import','BEGIN;\n'+body+'\nCOMMIT;')
run('repeat-import','BEGIN;\n'+body+'\nCOMMIT;')
run('exact-reviewed-delta',"""DO $$ BEGIN
 IF (SELECT updated_edges FROM game_skill_synergy_update_history WHERE configuration_version=74)<>70 OR
 (SELECT count(*) FROM game_skill_synergies WHERE rank_mode='effective_training')<>70 OR
 (SELECT count(*) FROM game_skill_synergies WHERE rank_mode='effective_raw')<>58 OR
 EXISTS(SELECT FROM game_skill_synergies GROUP BY target_skill_id HAVING count(DISTINCT source_skill_id)>3)
 THEN RAISE EXCEPTION 'wrong coverage or source cardinality'; END IF;
 IF EXISTS(
 WITH expected AS (SELECT b.edge_order,b.target_skill_id,b.source_skill_id,
 CASE WHEN b.rank_mode='hard' THEN 'effective_training' ELSE b.rank_mode END rank_mode,
 CASE WHEN b.rank_mode='hard' AND b.effect='heal_flat' THEN 'heal_power' ELSE b.effect END effect,
 CASE WHEN b.rank_mode='hard' THEN CASE WHEN d.level_need<30 THEN 300 WHEN d.level_need<50 THEN 400 WHEN d.level_need<70 THEN 500 ELSE 600 END ELSE b.per_point_bp END per_point_bp
 FROM before_synergy74_edges b JOIN gamedata.ov_skilldesc d ON d.skill_id=b.target_skill_id AND d.skill_level=1)
 (SELECT * FROM expected EXCEPT ALL SELECT * FROM game_skill_synergies)
 UNION ALL (SELECT * FROM game_skill_synergies EXCEPT ALL SELECT * FROM expected))
 THEN RAISE EXCEPTION 'unapproved synergy delta'; END IF;
END $$;""")
for i,table in enumerate(tables):
    run('preserve-'+str(i),f"DO $$ BEGIN IF EXISTS((SELECT row FROM before_synergy74_{i} EXCEPT ALL SELECT to_jsonb(t) FROM {table} t) UNION ALL (SELECT to_jsonb(t) FROM {table} t EXCEPT ALL SELECT row FROM before_synergy74_{i})) THEN RAISE EXCEPTION 'unrelated data changed: {table}'; END IF; END $$;")
run('replay-preserves-admin-edit-and-deletion',"""BEGIN;
UPDATE game_skill_synergies SET per_point_bp=350 WHERE target_skill_id=10401 AND rank_mode='effective_training';
DELETE FROM game_skill_synergies WHERE target_skill_id=10402 AND rank_mode='effective_training';
"""+body+"""
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM game_skill_synergies WHERE target_skill_id=10401 AND rank_mode='effective_training' AND per_point_bp=350) OR
 EXISTS(SELECT FROM game_skill_synergies WHERE target_skill_id=10402 AND rank_mode='effective_training')
 THEN RAISE EXCEPTION 'replay overwrote administrator configuration'; END IF;
END $$; ROLLBACK;""")
print('EQUIPMENT_SYNERGY_POSTGRES_PASS',flush=True)
print('FANTASY_SKILL_CONFIG_PG_DATA='+str(data),flush=True)
