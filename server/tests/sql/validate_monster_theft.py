#!/usr/bin/env python3
"""Exercise theft migration70 in a copied idle PostgreSQL cluster, never live DB."""
import argparse
import os
from pathlib import Path
import re
import shutil
import subprocess
from validate_daoist_postgres import write_single_user_commands

ROOT = Path(__file__).resolve().parents[3]
p = argparse.ArgumentParser()
p.add_argument('--work-dir', type=Path, required=True)
p.add_argument('--seed-dir', type=Path, required=True, help='idle disposable cluster through68')
p.add_argument('--reuse-test-baseline', action='store_true', help='use existing work/data explicitly restored to an unmigrated test baseline')
a = p.parse_args()
w = a.work_dir.resolve()
w.mkdir(parents=True, exist_ok=a.reuse_test_baseline)
seed = a.seed_dir.resolve()
if (seed/'postmaster.pid').exists():
    raise SystemExit('refusing running/stale-postmaster cluster')
data = w/'data'
if not a.reuse_test_baseline:
    shutil.copytree(seed, data)
elif (data/'postmaster.pid').exists():
    raise SystemExit('refusing running/stale-postmaster target')
postgres = Path(os.environ['PG_BIN'])/'postgres'
body = (ROOT/'server/migrations/000000_init-70.sql').read_text()


def run(label, sql, expected_error=None):
    source, inp, log = [w/(label+suffix) for suffix in ('.source.sql', '.sql', '.log')]
    source.write_text(sql)
    with source.open() as src, inp.open('w') as dst:
        write_single_user_commands(src, dst)
    with inp.open() as src, log.open('w') as dst:
        result = subprocess.run([str(postgres), '--single', '-j', '-D', str(data), 'postgres'], stdin=src, stdout=dst, stderr=subprocess.STDOUT)
    errors = [x.strip() for x in log.read_text().splitlines() if re.search(r'\b(?:ERROR|FATAL|PANIC):', x)]
    if expected_error:
        assert any(expected_error in e for e in errors), (label, errors[:3])
    else:
        assert result.returncode == 0 and not errors, (label, errors[:3])
    source.unlink()
    inp.unlink()
    print('PASS', label, flush=True)


empty = """DO $$ BEGIN
 IF EXISTS(SELECT FROM game_skill_effects WHERE skill_id=10212) OR
 EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10212 AND mechanic='monster_loot_theft')
 THEN RAISE EXCEPTION 'seed already modified'; END IF;
END $$;"""
run('require-original-seed', empty)
run('failed-transaction', 'BEGIN;\n'+body+"\nDO $$ BEGIN RAISE EXCEPTION 'deliberate rollback'; END $$; COMMIT;", 'deliberate rollback')
run('failure-preserved-originals', empty)
for label, change, assertion in [
    ('existing-effect', "INSERT INTO game_skill_effects(skill_id,skill_level,effect_order,effect_kind,power_pct) VALUES(10212,1,1,'damage',777);", "(SELECT count(*) FROM game_skill_effects WHERE skill_id=10212 AND power_pct=777)<>1 OR (SELECT count(*) FROM game_skill_effects WHERE skill_id=10212)<>1"),
    ('existing-text', "UPDATE gamedata.ov_skilldesc SET desc_='administrator text' WHERE skill_id=10212 AND skill_level=3;", "NOT EXISTS(SELECT FROM gamedata.ov_skilldesc WHERE skill_id=10212 AND skill_level=3 AND desc_='administrator text') OR EXISTS(SELECT FROM game_skill_effects WHERE skill_id=10212)"),
]:
    run('preserve-'+label, 'BEGIN;\n'+change+'\n'+body+"\nDO $$ BEGIN IF "+assertion+" OR EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10212 AND mechanic='monster_loot_theft' AND imported) THEN RAISE EXCEPTION 'overwrote administrator configuration'; END IF; END $$; ROLLBACK;")

tables = ['game_monster_drops', 'game_boss_fixed_equipment_pool', 'game_skill_statuses', 'game_skill_progression', 'game_skill_growth_policy', 'game_skill_synergies', 'characters', 'character_skills']
for i, table in enumerate(tables):
    run('snapshot-'+str(i), f'DROP TABLE IF EXISTS before_theft_{i}; CREATE TABLE before_theft_{i} AS SELECT to_jsonb(t) AS row FROM {table} t;')
run('preserve-existing-growth-policy', 'BEGIN; UPDATE game_skill_growth_policy SET efficiency_cap_rank=11 WHERE skill_id=10212;\n'+body+"\nDO $$ BEGIN IF NOT EXISTS(SELECT FROM game_skill_growth_policy WHERE skill_id=10212 AND efficiency_cap_rank=11) THEN RAISE EXCEPTION 'administrator growth policy changed'; END IF; END $$; ROLLBACK;")
run('snapshot-skills', "DROP TABLE IF EXISTS before_theft_skills; DROP TABLE IF EXISTS before_theft_effects; CREATE TABLE before_theft_skills AS SELECT row_no,skill_id,skill_level,to_jsonb(t) AS row FROM gamedata.ov_skilldesc t; CREATE TABLE before_theft_effects AS SELECT to_jsonb(t) AS row FROM game_skill_effects t;")
run('first-import', 'BEGIN;\n'+body+'\nCOMMIT;')
run('repeat-import', 'BEGIN;\n'+body+'\nCOMMIT;')
run('exact-five-rank-pure-theft', """DO $$ BEGIN
 IF (SELECT count(*) FROM game_skill_effects WHERE skill_id=10212)<>5 OR
 (SELECT count(*) FROM game_skill_effects WHERE skill_id=10212 AND effect_order=1 AND effect_kind='steal_item' AND flat_value=1 AND power_pct=0 AND max_resource_pct=0 AND NOT guaranteed_hit AND NOT requires_previous_landed AND effect_target='target' AND weapon_durability_pct=0 AND loss_refund_pct=0 AND chance_bp=5000+(skill_level-1)*600)<>5 OR
 NOT EXISTS(SELECT FROM game_skill_mechanism_sources WHERE skill_id=10212 AND mechanic='monster_loot_theft' AND imported)
 THEN RAISE EXCEPTION 'wrong theft configuration'; END IF;
 IF NOT EXISTS(SELECT FROM game_skill_growth_policy WHERE skill_id=10212 AND efficiency_cap_rank=10 AND attribute=0 AND attribute_cap_rank=0 AND range_cap_rank=0 AND protection_cap_rank=0 AND attack_cap_rank=0 AND cardinality(attack_targets)=0) THEN RAISE EXCEPTION 'wrong theft post-cap growth policy'; END IF;
 IF EXISTS(SELECT FROM gamedata.ov_skilldesc t JOIN before_theft_skills b USING(row_no) WHERE CASE WHEN t.skill_id=10212 THEN to_jsonb(t)-'name'-'desc_' <> b.row-'name'-'desc_' ELSE to_jsonb(t)<>b.row END)
 THEN RAISE EXCEPTION 'unapproved raw fields changed'; END IF;
 IF (SELECT count(*) FROM gamedata.ov_skilldesc WHERE skill_id=10212 AND name='偷窃术' AND desc_ LIKE '有%的几率从目标身上偷取一件物品。每只怪物仅可成功偷取一次。')<>5
 THEN RAISE EXCEPTION 'theft name/rule description missing'; END IF;
 IF EXISTS((SELECT row FROM before_theft_effects EXCEPT ALL SELECT to_jsonb(t) FROM game_skill_effects t WHERE skill_id<>10212) UNION ALL (SELECT to_jsonb(t) FROM game_skill_effects t WHERE skill_id<>10212 EXCEPT ALL SELECT row FROM before_theft_effects))
 THEN RAISE EXCEPTION 'unrelated skill effects changed'; END IF;
END $$;""")
for i, table in enumerate(tables):
    if table == 'game_skill_growth_policy':
        run('only-approved-theft-growth-policy-change', f"DO $$ BEGIN IF EXISTS((SELECT CASE WHEN row->>'skill_id'='10212' THEN row-'efficiency_cap_rank' ELSE row END FROM before_theft_{i} EXCEPT ALL SELECT CASE WHEN skill_id=10212 THEN to_jsonb(t)-'efficiency_cap_rank' ELSE to_jsonb(t) END FROM {table} t) UNION ALL (SELECT CASE WHEN skill_id=10212 THEN to_jsonb(t)-'efficiency_cap_rank' ELSE to_jsonb(t) END FROM {table} t EXCEPT ALL SELECT CASE WHEN row->>'skill_id'='10212' THEN row-'efficiency_cap_rank' ELSE row END FROM before_theft_{i})) THEN RAISE EXCEPTION 'unrelated growth data changed'; END IF; END $$;")
        continue
    run('unchanged-'+table, f"DO $$ BEGIN IF EXISTS((SELECT row FROM before_theft_{i} EXCEPT ALL SELECT to_jsonb(t) FROM {table} t) UNION ALL (SELECT to_jsonb(t) FROM {table} t EXCEPT ALL SELECT row FROM before_theft_{i})) THEN RAISE EXCEPTION 'unrelated data changed'; END IF; END $$;")
run('preserve-admin-edits-deletions-on-replay', """BEGIN;
UPDATE game_skill_growth_policy SET efficiency_cap_rank=77 WHERE skill_id=10212;
UPDATE game_skill_effects SET chance_bp=1234 WHERE skill_id=10212 AND skill_level=1;
DELETE FROM game_skill_effects WHERE skill_id=10212 AND skill_level=2;
UPDATE gamedata.ov_skilldesc SET desc_='administrator text after import' WHERE skill_id=10212 AND skill_level=3;
"""+body+"""
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM game_skill_growth_policy WHERE skill_id=10212 AND efficiency_cap_rank=77) OR
 NOT EXISTS(SELECT FROM game_skill_effects WHERE skill_id=10212 AND skill_level=1 AND chance_bp=1234) OR
 EXISTS(SELECT FROM game_skill_effects WHERE skill_id=10212 AND skill_level=2) OR
 NOT EXISTS(SELECT FROM gamedata.ov_skilldesc WHERE skill_id=10212 AND skill_level=3 AND desc_='administrator text after import')
 THEN RAISE EXCEPTION 'replay reset administrator changes'; END IF;
END $$; ROLLBACK;""")
print('MONSTER_THEFT_POSTGRES_PASS', flush=True)
print('data='+str(data), flush=True)
