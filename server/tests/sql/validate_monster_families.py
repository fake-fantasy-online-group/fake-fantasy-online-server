#!/usr/bin/env python3
"""Migration71 audit in a new disposable PostgreSQL copy (never a running DB)."""
import argparse
import csv
import os
from pathlib import Path
import re
import shutil
import subprocess
from validate_daoist_postgres import write_single_user_commands

ROOT = Path(__file__).resolve().parents[3]
p = argparse.ArgumentParser()
p.add_argument('--work-dir', type=Path, required=True)
p.add_argument('--seed-dir', type=Path, required=True, help='idle disposable cluster through migration68')
a = p.parse_args()
w = a.work_dir.resolve()
w.mkdir(parents=True, exist_ok=False)
seed = a.seed_dir.resolve()
if (seed/'postmaster.pid').exists():
    raise SystemExit('refusing running/stale-postmaster fixture')
data = w/'data'
shutil.copytree(seed, data)
bins = Path(os.environ['PG_BIN'])


def run(label, sql, expected_error=None):
    inp, log = w/(label+'.sql'), w/(label+'.log')
    import io
    with inp.open('w') as target:
        write_single_user_commands(io.StringIO(sql), target)
    with inp.open() as source, log.open('w') as out:
        result = subprocess.run([str(bins/'postgres'), '--single', '-j', '-D', str(data), 'postgres'], stdin=source, stdout=out, stderr=subprocess.STDOUT)
    errors = [x.strip() for x in log.read_text().splitlines() if re.search(r'\b(?:ERROR|FATAL|PANIC):', x)]
    if expected_error:
        assert any(expected_error in e for e in errors), (label, errors[:3])
    else:
        assert result.returncode == 0 and not errors, (label, errors[:3])
    inp.unlink()
    print('PASS', label, flush=True)

body = (ROOT/'server/migrations/000000_init-71.sql').read_text()
ddl = body[:body.index('DO $monster_family_import$')]
run('require-clean-fixture', "DO $$ BEGIN IF to_regclass('game_monster_families') IS NOT NULL THEN RAISE EXCEPTION 'already migrated'; END IF; END $$;")
run('rollback-on-failure', 'BEGIN;\n'+body+"\nDO $$ BEGIN RAISE EXCEPTION 'deliberate rollback'; END $$; COMMIT;", 'deliberate rollback')
run('rollback-is-atomic', "DO $$ BEGIN IF to_regclass('game_monster_families') IS NOT NULL OR EXISTS(SELECT FROM game_skill_mechanism_sources WHERE mechanic='target_monster_family') THEN RAISE EXCEPTION 'rollback leaked'; END IF; END $$;")
run('missing-skill-anchor-rejected', 'BEGIN; DELETE FROM gamedata.ov_skilldesc WHERE skill_id=10312 AND skill_level=2;\n'+body+'\nCOMMIT;', 'missing original Exorcism anchors')
run('preserve-preexisting-admin-config-and-source-identity', 'BEGIN;\n'+ddl+"""
INSERT INTO game_monster_families VALUES (1025,'other','admin','admin','admin','');
INSERT INTO game_skill_target_families VALUES (10312,'other');
UPDATE game_monsters SET name='管理员自定义同ID' WHERE id=1003;
UPDATE game_monsters SET sprite=sprite+1 WHERE id=1202;
UPDATE game_monsters SET kind='采集物' WHERE id=1206;
"""+body+"""
DO $$ BEGIN
 IF (SELECT family FROM game_monster_families WHERE monster_id=1025)<>'other' OR
    (SELECT count(*) FROM game_skill_target_families WHERE skill_id=10312 AND family='other')<>1 OR
    (SELECT count(*) FROM game_skill_target_families WHERE skill_id=10312)<>1 OR
    EXISTS(SELECT FROM game_monster_families WHERE monster_id IN (1003,1202,1206)) OR
    EXISTS(SELECT FROM game_monster_family_sources WHERE monster_id IN (1025,1003,1202,1206) AND imported) OR
    EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10312 AND mechanic='target_monster_family' AND imported)
 THEN RAISE EXCEPTION 'administrator configuration/source identity not preserved'; END IF;
END $$; ROLLBACK;
""")
tables = ['game_monsters','gamedata.ov_skilldesc','gamedata.ov_exceptdesc','gamedata.ov_exceptdetail','gamedata.ov_exceptdetail_entry','gamedata.ov_exceptdetail_effect_entry','game_skill_effects','game_skill_statuses','game_custom_statuses','game_custom_status_affixes','game_skill_growth_policy','game_skill_progression','game_skill_synergies','characters','character_skills']
for i, table in enumerate(tables):
    run('snapshot-'+str(i), f'CREATE TABLE before_family_{i} AS SELECT to_jsonb(t) AS row FROM {table} t;')
run('first-import', 'BEGIN;\n'+body+'\nCOMMIT;')
run('repeat-import', 'BEGIN;\n'+body+'\nCOMMIT;')
run('classifications-and-combat-boundaries', """DO $$ BEGIN
 IF (SELECT count(*) FROM game_monster_families)<>1278 OR
    (SELECT count(*) FROM game_monster_family_sources WHERE imported)<>1278 OR
    (SELECT count(*) FROM game_monster_families WHERE family='undead')<>90 OR
    (SELECT count(*) FROM game_monster_families WHERE family='demon')<>87 OR
    (SELECT count(*) FROM game_monster_families WHERE family='unknown')<>244 OR
    (SELECT count(*) FROM game_monster_families WHERE family='other')<>857 OR
    EXISTS(SELECT FROM game_monster_families f JOIN game_monsters m ON m.id=f.monster_id WHERE m.kind NOT IN ('普通','精英','BOSS') OR m.level<=0 OR m.hp<=0) OR
    (SELECT count(*) FROM game_skill_target_families WHERE skill_id=10312 AND family IN ('undead','demon'))<>2 OR
    EXISTS(SELECT FROM game_skill_target_families WHERE skill_id<>10312)
 THEN RAISE EXCEPTION 'classification/target scope mismatch'; END IF;
 IF (SELECT family FROM game_monster_families WHERE monster_id=1207)<>'undead' OR
    (SELECT family FROM game_monster_families WHERE monster_id=6080)<>'unknown' OR
    (SELECT family FROM game_monster_families WHERE monster_id=6067)<>'demon' OR
    (SELECT family FROM game_monster_families WHERE monster_id=31009)<>'unknown' OR
    (SELECT family FROM game_monster_families WHERE monster_id=31736)<>'other' OR
    (SELECT family FROM game_monster_families WHERE monster_id=4131)<>'other' OR
    (SELECT family FROM game_monster_families WHERE monster_id=6016)<>'demon'
 THEN RAISE EXCEPTION 'name/elite-template reuse exception mismatch'; END IF;
END $$;""")
for i, table in enumerate(tables):
    run('preserve-'+str(i), f"DO $$ BEGIN IF EXISTS((SELECT row FROM before_family_{i} EXCEPT ALL SELECT to_jsonb(t) FROM {table} t) UNION ALL (SELECT to_jsonb(t) FROM {table} t EXCEPT ALL SELECT row FROM before_family_{i})) THEN RAISE EXCEPTION 'unrelated data changed: {table}'; END IF; END $$;")
run('replay-preserves-admin-edits-and-deletions', """BEGIN;
UPDATE game_monster_families SET family='other',evidence='admin edit' WHERE monster_id=1025;
DELETE FROM game_monster_families WHERE monster_id=1003;
DELETE FROM game_skill_target_families WHERE skill_id=10312 AND family='demon';
"""+body+"""
DO $$ BEGIN
 IF (SELECT family FROM game_monster_families WHERE monster_id=1025)<>'other' OR
 EXISTS(SELECT FROM game_monster_families WHERE monster_id=1003) OR
 EXISTS(SELECT FROM game_skill_target_families WHERE skill_id=10312 AND family='demon') OR
 (SELECT count(*) FROM game_skill_target_families WHERE skill_id=10312 AND family='undead')<>1
 THEN RAISE EXCEPTION 'replay changed edits or resurrected deleted mapping'; END IF;
END $$; ROLLBACK;
""")
run('invalid-monster-family-rejected', "BEGIN; UPDATE game_monster_families SET family='probably_undead' WHERE monster_id=1001; COMMIT;", 'violates check constraint')
run('unknown-target-family-rejected', "BEGIN; INSERT INTO game_skill_target_families VALUES(10312,'unknown'); COMMIT;", 'violates check constraint')
# Export committed DB, then compare every row to the hand-reviewed audit artifact.
export=w/'classified.tsv'
run('export-actual-db', f"COPY (SELECT m.id,m.name,m.kind,f.family,f.lineage,f.evidence_source,f.evidence,f.recommendation FROM game_monsters m JOIN game_monster_families f ON f.monster_id=m.id ORDER BY m.id) TO '{export}' WITH CSV HEADER DELIMITER E'\\t';")
with (ROOT/'docs/daoist-endgame/monster-family-audit-2026-10-13.tsv').open() as f:
    audit = {r['id']:r for r in csv.DictReader(f,delimiter='\t') if r['family']!='excluded'}
with export.open() as f:
    actual=list(csv.DictReader(f,delimiter='\t'))
assert len(actual)==len(audit)==1278
for r in actual:
    assert all(audit[r['id']][k]==v for k,v in r.items()), r
print('MONSTER_FAMILY_POSTGRES_PASS')
print('FANTASY_MONSTER_FAMILIES_PG_DATA='+str(data))
