#!/usr/bin/env python3
"""Validate migration69 in a fresh/copied idle, socket-free PostgreSQL cluster."""
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
p.add_argument('--seed-dir', type=Path, help='idle disposable cluster through migration68')
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
    raw, inp, log = [w/(label+suffix) for suffix in ('.source.sql', '.sql', '.log')]
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


if not a.seed_dir:
    run('fresh-through68', paths=[ROOT/f'server/migrations/000000_init-{i}.sql' for i in range(69)])
body = (ROOT/'server/migrations/000000_init-69.sql').read_text()
empty = """DO $$ BEGIN
 IF EXISTS(SELECT FROM game_skill_effects WHERE skill_id=10102) OR
 EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10102 AND mechanic='knockback')
 THEN RAISE EXCEPTION 'fixture already imported or changed'; END IF;
END $$;"""
run('require-unmigrated', sql=empty)
run('rollback-on-failure', sql='BEGIN;\n'+body+"\nDO $$ BEGIN RAISE EXCEPTION 'deliberate rollback'; END $$; COMMIT;", expected_error='deliberate rollback')
run('rollback-left-no-changes', sql=empty)
for label, change, error in [
    ('missing-original-rank', 'DELETE FROM gamedata.ov_skilldesc WHERE skill_id=10102 AND skill_level=2;', 'missing or changed original Gangqi'),
    ('changed-original-damage', "UPDATE gamedata.ov_skilldesc SET desc_='造成999%伤害，震退敌人。' WHERE skill_id=10102 AND skill_level=2;", 'missing or changed original Gangqi'),
    ('missing-donor', "DELETE FROM game_skill_effects WHERE skill_id=10223 AND skill_level=3 AND effect_kind='knockback';", 'missing or invalid Thunder Bomb'),
    ('duplicate-donor', "INSERT INTO game_skill_effects(skill_id,skill_level,effect_order,effect_kind,flat_value,requires_previous_landed) VALUES(10223,3,3,'knockback',100,true);", 'missing or invalid Thunder Bomb'),
    ('zero-distance', "UPDATE game_skill_effects SET flat_value=0 WHERE skill_id=10223 AND effect_kind='knockback';", 'missing or invalid Thunder Bomb'),
    ('unguarded-donor', "UPDATE game_skill_effects SET requires_previous_landed=false WHERE skill_id=10223 AND effect_kind='knockback';", 'missing or invalid Thunder Bomb'),
]:
    run('reject-'+label, sql='BEGIN;\n'+change+'\n'+body+'\nCOMMIT;', expected_error=error)
run('rejections-left-no-changes', sql=empty)
run('preserve-preexisting-admin-effects', sql="""BEGIN;
INSERT INTO game_skill_effects(skill_id,skill_level,effect_order,effect_kind,power_pct)
VALUES(10102,1,1,'damage',177);
"""+body+"""
DO $$ BEGIN
 IF (SELECT count(*) FROM game_skill_effects WHERE skill_id=10102)<>1 OR
 NOT EXISTS(SELECT FROM game_skill_effects WHERE skill_id=10102 AND power_pct=177) OR
 EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10102 AND mechanic='knockback' AND imported)
 THEN RAISE EXCEPTION 'administrator effects changed'; END IF;
END $$; ROLLBACK;""")
run('copy-configured-donor-distance', sql="""BEGIN;
UPDATE game_skill_effects SET flat_value=137 WHERE skill_id=10223 AND effect_kind='knockback';
"""+body+"""
DO $$ BEGIN
 IF (SELECT count(*) FROM game_skill_effects WHERE skill_id=10102 AND effect_kind='knockback' AND flat_value=137)<>5
 THEN RAISE EXCEPTION 'donor parameter was hardcoded'; END IF;
END $$; ROLLBACK;""")
tables = ['gamedata.ov_skilldesc', 'gamedata.ov_exceptdesc', 'gamedata.ov_exceptdetail', 'gamedata.ov_exceptdetail_entry', 'gamedata.ov_exceptdetail_effect_entry', 'game_skill_statuses', 'game_custom_statuses', 'game_custom_status_affixes', 'game_skill_passive_trigger_statuses', 'game_skill_passive_modifiers', 'game_skill_growth_policy', 'game_skill_progression', 'game_skill_synergies', 'characters', 'character_skills']
for i, table in enumerate(tables):
    run('snapshot-'+str(i), sql=f'CREATE TABLE before_gangqi_{i} AS SELECT to_jsonb(t) AS row FROM {table} t;')
run('snapshot-unrelated-effects', sql='CREATE TABLE before_gangqi_effects AS SELECT to_jsonb(t) AS row FROM game_skill_effects t;')
run('first-import', sql='BEGIN;\n'+body+'\nCOMMIT;')
run('repeat-import', sql='BEGIN;\n'+body+'\nCOMMIT;')
run('exact-scoped-effect-chain', sql="""DO $$ BEGIN
 IF (SELECT count(*) FROM game_skill_effects WHERE skill_id=10102)<>10 OR
 (SELECT count(*) FROM game_skill_effects WHERE skill_id=10102 AND effect_order=1 AND effect_kind='damage' AND power_pct=127+3*skill_level AND NOT guaranteed_hit AND NOT requires_previous_landed)<>5 OR
 (SELECT count(*) FROM game_skill_effects g JOIN game_skill_effects d USING(skill_level)
 WHERE g.skill_id=10102 AND d.skill_id=10223 AND g.effect_kind='knockback' AND d.effect_kind='knockback'
 AND to_jsonb(g)-'skill_id'=to_jsonb(d)-'skill_id')<>5 OR
 NOT EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10102 AND mechanic='knockback' AND imported)
 THEN RAISE EXCEPTION 'unexpected Gangqi effect configuration'; END IF;
 IF EXISTS(SELECT FROM game_skill_passive_trigger_statuses WHERE passive_skill_id=10201)
 THEN RAISE EXCEPTION 'unconfirmed backstab duration activated'; END IF;
 IF EXISTS((SELECT row FROM before_gangqi_effects EXCEPT ALL SELECT to_jsonb(t) FROM game_skill_effects t WHERE skill_id<>10102)
 UNION ALL (SELECT to_jsonb(t) FROM game_skill_effects t WHERE skill_id<>10102 EXCEPT ALL SELECT row FROM before_gangqi_effects))
 THEN RAISE EXCEPTION 'unrelated effects changed'; END IF;
END $$;""")
for i, table in enumerate(tables):
    run('preserve-'+str(i), sql=f"DO $$ BEGIN IF EXISTS((SELECT row FROM before_gangqi_{i} EXCEPT ALL SELECT to_jsonb(t) FROM {table} t) UNION ALL (SELECT to_jsonb(t) FROM {table} t EXCEPT ALL SELECT row FROM before_gangqi_{i})) THEN RAISE EXCEPTION 'unrelated data changed: {table}'; END IF; END $$;")
run('replay-preserves-admin-edits-and-deletions', sql="""BEGIN;
UPDATE game_skill_effects SET flat_value=123 WHERE skill_id=10102 AND skill_level=1 AND effect_kind='knockback';
DELETE FROM game_skill_effects WHERE skill_id=10102 AND skill_level=2 AND effect_kind='knockback';
UPDATE game_skill_effects SET flat_value=177 WHERE skill_id=10223 AND effect_kind='knockback';
"""+body+"""
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM game_skill_effects WHERE skill_id=10102 AND skill_level=1 AND effect_kind='knockback' AND flat_value=123) OR
 EXISTS(SELECT FROM game_skill_effects WHERE skill_id=10102 AND skill_level=2 AND effect_kind='knockback') OR
 (SELECT count(*) FROM game_skill_effects WHERE skill_id=10102 AND skill_level BETWEEN 3 AND 5 AND effect_kind='knockback' AND flat_value=100)<>3
 THEN RAISE EXCEPTION 'replay changed administrator configuration'; END IF;
END $$; ROLLBACK;""")
print('GANGQI_KNOCKBACK_POSTGRES_PASS', flush=True)
print('FANTASY_GANGQI_KNOCKBACK_PG_DATA='+str(data), flush=True)
