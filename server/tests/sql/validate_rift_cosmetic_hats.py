#!/usr/bin/env python3
"""Execute migration105 in a new disposable PostgreSQL cluster, never production.
PG_BIN must name a trusted PostgreSQL bin directory. The focused schema uses
actual native pools/face-back base metadata and the production immutable-rule
trigger. This is not a full initializer or production database validation.
Example: PG_BIN=/usr/lib/postgresql/17/bin python3 server/tests/sql/validate_rift_cosmetic_hats.py --work-dir /tmp/rift-hat-pg
"""
import argparse
import io
import json
import os
from pathlib import Path
import re
import subprocess
from validate_daoist_postgres import write_single_user_commands

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--work-dir', required=True, type=Path)
a = p.parse_args()
root = Path(__file__).resolve().parents[3]
work = a.work_dir.resolve()
work.mkdir(parents=True, exist_ok=True)
data = work / 'cluster'
if data.exists():
    raise SystemExit('refusing to use an existing cluster directory')
bin = Path(os.environ['PG_BIN'])
subprocess.run([str(bin / 'initdb'), '-D', str(data), '--no-sync', '--auth=trust',
                '--no-locale', '--encoding=UTF8'], check=True, stdout=subprocess.PIPE)
body = (root / 'server/migrations/000000_init-105.sql').read_text()
f = json.loads((root / 'server/internal/game/scene/testdata/rift_native_equipment_pools.json').read_text())
excluded = json.loads(body.split('$excluded$')[1])
ids = {x['id'] for x in excluded}
fallback_ids = list(range(2601, 2611)) + list(range(2701, 2708))
fallback_items = [x for x in f['Items'] if x.get('Equip', {}).get('Slot') in (1, 10)]
assert sorted(x['ID'] for x in fallback_items) == fallback_ids, 'native base fixture drifted'
assert f['SlotFallbacks'] == fallback_ids, 'exported fallback fixture drifted'
normal_gates = ['newbie', 'warrior', 'swordman', 'stabber', 'druggist', 'magician']
sr_gates = ['sr_warrior', 'sr_swordman', 'sr_stabber', 'sr_druggist', 'sr_magician']


def lit(x):
    return "'" + json.dumps(x, ensure_ascii=False).replace("'", "''") + "'::jsonb"


def check(sql, message):
    return "DO $$ BEGIN IF " + sql + " THEN RAISE EXCEPTION '" + message + "'; END IF; END $$;"


def run(label, sql, error=None):
    command = io.StringIO()
    write_single_user_commands(io.StringIO(sql), command)
    r = subprocess.run([str(bin / 'postgres'), '--single', '-j', '-D', str(data), 'postgres'],
                       input=command.getvalue(), text=True, stdout=subprocess.PIPE,
                       stderr=subprocess.STDOUT)
    (work / (label + '.log')).write_text(r.stdout)
    errors = [line for line in r.stdout.splitlines() if re.search(r'\b(?:ERROR|FATAL|PANIC):', line)]
    assert r.returncode == 0, (label, r.returncode, errors)
    if error:
        assert any(error in line for line in errors), (label, errors)
        assert all(error in line or 'current transaction is aborted' in line for line in errors), (label, errors)
    else:
        assert not errors, (label, errors)
    print('PASS', label, flush=True)


def source_item(item_id, name, slot, level_req, **native):
    columns = ','.join(native)
    values = ','.join(str(v) for v in native.values())
    escaped_name = name.replace("'", "''")
    return (f"INSERT INTO game_equipment VALUES({item_id},'{escaped_name}',{slot},{level_req});"
            f'INSERT INTO gamedata.ov_arm(index,{columns}) VALUES({item_id},{values});\n')


seed = """
CREATE SCHEMA gamedata;
CREATE TABLE game_equipment(id INTEGER PRIMARY KEY,name TEXT,slot INTEGER,level_req INTEGER);
CREATE TABLE gamedata.ov_arm(
 index INTEGER PRIMARY KEY,level_need INTEGER,level INTEGER,sex INTEGER DEFAULT 0,
 query_type INTEGER,no_type_drop INTEGER DEFAULT 0,position_ INTEGER,
 newbie INTEGER DEFAULT 0,warrior INTEGER DEFAULT 0,swordman INTEGER DEFAULT 0,
 stabber INTEGER DEFAULT 0,druggist INTEGER DEFAULT 0,magician INTEGER DEFAULT 0,
 sr_warrior INTEGER DEFAULT 0,sr_swordman INTEGER DEFAULT 0,sr_stabber INTEGER DEFAULT 0,
 sr_druggist INTEGER DEFAULT 0,sr_magician INTEGER DEFAULT 0);
CREATE TABLE game_equip_extra(arm_id INTEGER,seq INTEGER,attr_id INTEGER,value INTEGER,mode INTEGER DEFAULT 0,PRIMARY KEY(arm_id,seq));
-- Start at the pre-105 schema so the migration must add a nullable column.
CREATE TABLE game_rift_rule_versions(version INTEGER PRIMARY KEY,published BOOLEAN,equipment_pools JSONB,elite_rules JSONB,floor_layout_version INTEGER,guardian_recent_window INTEGER,normal_boss_equipment INTEGER,unchanged_sentinel JSONB);
CREATE TABLE game_rift_rule_active(id INTEGER PRIMARY KEY,version INTEGER REFERENCES game_rift_rule_versions);
CREATE TABLE game_rift_class_rules(version INTEGER REFERENCES game_rift_rule_versions,class INTEGER,hp_bp INTEGER,damage_bp INTEGER,PRIMARY KEY(version,class));
CREATE TABLE game_rift_ability_rules(version INTEGER REFERENCES game_rift_rule_versions,id INTEGER,name TEXT,affix TEXT,warning TEXT,shape TEXT,radius INTEGER,inner_radius INTEGER,length INTEGER,delay_ms BIGINT,damage_pct INTEGER,PRIMARY KEY(version,id));
"""
for x in excluded:
    seed += source_item(x['id'], x['name'], 2, 1, level_need=1, level=5,
                        sex=x['sex'], query_type=9, no_type_drop=0, position_=102)
# Preserve native wear/resource levels and class gates, not synthetic level-one bases.
for x in fallback_items:
    equip = x['Equip']
    need = equip['Need']
    assert not equip['AscensionOnly'] and not equip['NoTypeDrop'] and not equip.get('Affixes')
    assert equip['Position'] == {1: 101, 10: 110}[equip['Slot']]
    gates = dict(zip(normal_gates[1:], [int(bool(need['Professions'] & (1 << n))) for n in range(5)]))
    seed += source_item(x['ID'], x['Name'], equip['Slot'], equip['LevelReq'],
                        level_need=need['Level'], level=equip['ResourceLevel'], sex=need['Sex'],
                        query_type={1: 15, 10: 13}[equip['Slot']], position_=equip['Position'],
                        no_type_drop=int(equip['NoTypeDrop']), newbie=int(need['BeginnerAllowed']), **gates)

# Every decoy differs from a valid base by one independent eligibility condition.
# Fixed-affix presence excludes the entire base, even if its value is zero.
decoys = [
    ('no-type-drop', {}, {'no_type_drop': 1}),
    ('costume-position', {}, {'position_': 201}),
    ('wrong-query-type', {}, {'query_type': 13}),
    ('wrong-back-position', {'slot': 10}, {'query_type': 13, 'position_': 101}),
    ('wrong-slot', {'slot': 2}, {}),
    ('negative-resource', {}, {'level': -1}),
    ('high-equipment-wear', {'level_req': 61}, {}),
    ('high-native-wear', {}, {'level_need': 61}),
    ('negative-equipment-wear', {'level_req': -1}, {}),
    ('negative-native-wear', {}, {'level_need': -1}),
    ('fixed-affix', {}, {}),
]
for gate in sr_gates:
    decoys.append(('ascension-only-' + gate, {}, {'newbie': 0, gate: 1}))
for n, (name, equipment, native) in enumerate(decoys, 900001):
    values = dict(level_need=6, level=1, query_type=15, position_=101, no_type_drop=0, newbie=1)
    values.update(native)
    seed += source_item(n, name, equipment.get('slot', 1), equipment.get('level_req', 6), **values)
    if name == 'fixed-affix':
        seed += f'INSERT INTO game_equip_extra(arm_id,seq,attr_id,value) VALUES({n},1,1,0);'
# Require both catalog rows, not merely a matching row on either side of the join.
seed += "INSERT INTO game_equipment VALUES(900100,'missing native source',1,6);"
seed += 'INSERT INTO gamedata.ov_arm(index,level_need,level,query_type,position_,newbie) VALUES(900101,6,1,15,101,1);'
catalog = """jsonb_build_object(
 'equipment',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM game_equipment e),
 'native',(SELECT jsonb_agg(to_jsonb(a) ORDER BY index) FROM gamedata.ov_arm a),
 'fixed',(SELECT jsonb_agg(to_jsonb(x) ORDER BY arm_id,seq) FROM game_equip_extra x))"""
seed += 'CREATE TABLE focused_catalog_snapshot AS SELECT ' + catalog + ' AS snapshot;'
for version, layout in [(9, 5), (10, 6)]:
    seed += f"INSERT INTO game_rift_rule_versions VALUES({version},true,{lit(f['Pools'])},'{{\"keep\":true}}',{layout},3,10,'{{\"keep\":[1,2,3]}}');"
    seed += f'INSERT INTO game_rift_class_rules SELECT {version},i,10000,10000 FROM generate_series(0,3)i;'
    seed += f"INSERT INTO game_rift_ability_rules VALUES({version},0,'ability','affix','warning','circle',1,2,3,4,5);"
seed += 'INSERT INTO game_rift_rule_active VALUES(1,10);'
s63 = (root / 'server/migrations/000000_init-63.sql').read_text()
seed += s63[s63.index('CREATE OR REPLACE FUNCTION guard_published_endgame_rules()'):s63.index('DROP TRIGGER IF EXISTS immutable_published_rules')]
for table in ['game_rift_rule_versions', 'game_rift_class_rules', 'game_rift_ability_rules']:
    seed += f'CREATE TRIGGER immutable_published_rules BEFORE INSERT OR UPDATE OR DELETE ON {table} FOR EACH ROW EXECUTE FUNCTION guard_published_endgame_rules();'
run('focused-native-fixture', seed)
expected = [dict(p, Items=[i for i in p['Items'] if i not in ids]) for p in f['Pools']]
fallback_assertion = check('(SELECT equipment_slot_fallbacks FROM game_rift_rule_versions WHERE version=11) IS DISTINCT FROM ' + lit(fallback_ids), 'wrong frozen slot fallback bases')
assertions = check('(SELECT equipment_pools FROM game_rift_rule_versions WHERE version=11) IS DISTINCT FROM ' + lit(expected), 'wrong filtered pools')
assertions += fallback_assertion
assertions += check("(SELECT to_jsonb(r)-'version'-'equipment_pools'-'equipment_slot_fallbacks' FROM game_rift_rule_versions r WHERE version=10) IS DISTINCT FROM (SELECT to_jsonb(r)-'version'-'equipment_pools'-'equipment_slot_fallbacks' FROM game_rift_rule_versions r WHERE version=11)", 'unrelated rule changed')
for version in (9, 10):
    assertions += check(f'(SELECT equipment_pools FROM game_rift_rule_versions WHERE version={version}) IS DISTINCT FROM ' + lit(f['Pools']), 'old version changed')
    assertions += check(f'(SELECT equipment_slot_fallbacks FROM game_rift_rule_versions WHERE version={version}) IS NOT NULL', 'old version gained fallback')
for column in ['class,hp_bp,damage_bp', 'id,name,affix,warning,shape,radius,inner_radius,length,delay_ms,damage_pct']:
    table = 'game_rift_class_rules' if column.startswith('class') else 'game_rift_ability_rules'
    assertions += check(f'EXISTS((SELECT {column} FROM {table} WHERE version=10 EXCEPT ALL SELECT {column} FROM {table} WHERE version=11) UNION ALL (SELECT {column} FROM {table} WHERE version=11 EXCEPT ALL SELECT {column} FROM {table} WHERE version=10))', 'child mismatch')
unchanged_catalog = check('(' + catalog + ') IS DISTINCT FROM (SELECT snapshot FROM focused_catalog_snapshot)', 'catalog changed')
run('publish-replay-preserve', 'BEGIN;' + body + body + assertions + unchanged_catalog + check('(SELECT version FROM game_rift_rule_active WHERE id=1)<>11', 'active selector mismatch') + 'ROLLBACK;')
run('fresh-v9-client-gate', 'BEGIN;UPDATE game_rift_rule_active SET version=9;' + body + assertions + unchanged_catalog + check('(SELECT version FROM game_rift_rule_active WHERE id=1)<>9', 'client gate bypassed') + 'ROLLBACK;')
run('future-selector', "BEGIN;INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,to_jsonb(r)||'{\"version\":12}'::jsonb)).* FROM game_rift_rule_versions r WHERE version=10;UPDATE game_rift_rule_active SET version=12;" + body + assertions + check('(SELECT version FROM game_rift_rule_active WHERE id=1)<>12', 'future selector rolled back') + 'ROLLBACK;')
run('occupied-v11', 'BEGIN;INSERT INTO game_rift_rule_versions SELECT (jsonb_populate_record(NULL::game_rift_rule_versions,to_jsonb(r)||\'{"version":11,"published":false}\'::jsonb)).* FROM game_rift_rule_versions r WHERE version=10;' + body + 'COMMIT;', 'rift11 is occupied')
run('disabled-guard', 'BEGIN;ALTER TABLE game_rift_class_rules DISABLE TRIGGER immutable_published_rules;' + body + 'COMMIT;', 'requires enabled immutable rule guards')
run('identity-drift', "BEGIN;UPDATE game_equipment SET name='ordinary hat' WHERE id=2037;" + body + 'COMMIT;', 'source identity mismatch')
run('rollback-after-publication', 'BEGIN;' + body + "DO $$ BEGIN RAISE EXCEPTION 'deliberate rollback'; END $$;COMMIT;", 'deliberate rollback')
run('rollback-no-leak', check('EXISTS(SELECT FROM game_rift_rule_versions WHERE version=11) OR (SELECT version FROM game_rift_rule_active WHERE id=1)<>10', 'rollback leaked') + check("EXISTS(SELECT FROM information_schema.columns WHERE table_schema='public' AND table_name='game_rift_rule_versions' AND column_name='equipment_slot_fallbacks')", 'rollback leaked schema'))


def changed_source(value):
    return 'ALTER TABLE game_rift_rule_versions DISABLE TRIGGER immutable_published_rules; UPDATE game_rift_rule_versions SET equipment_pools=' + lit(value) + ' WHERE version=10; ALTER TABLE game_rift_rule_versions ENABLE TRIGGER immutable_published_rules;'


run('missing-reviewed-hat', 'BEGIN;' + changed_source([dict(p, Items=[i for i in p['Items'] if i != 2037]) for p in f['Pools']]) + body + 'COMMIT;', 'missing a reviewed cosmetic hat')
run('empty-tier', 'BEGIN;' + changed_source(f['Pools'] + [{'MonsterLevel': 100, 'SourceMonsters': [1], 'Items': [2037]}]) + body + 'COMMIT;', 'would empty an equipment tier')
for slot in (1, 10):
    run(f'missing-slot-{slot}-source', f'BEGIN;DELETE FROM game_equipment WHERE slot={slot};' + body + 'COMMIT;', 'requires normal face and backpack bases')
# Individual normal gates allow mixed normal/SR bases; all-zero normal and SR
# gates are unrestricted, not ascension-only. Exercise every gate independently.
for gate in normal_gates + ['unrestricted']:
    changes = {g: 0 for g in normal_gates + sr_gates}
    if gate != 'unrestricted':
        changes.update({gate: 1, 'sr_warrior': 1})
    assignments = ','.join(f'{g}={v}' for g, v in changes.items())
    run('normal-gate-' + gate, f'BEGIN;UPDATE gamedata.ov_arm SET {assignments} WHERE index BETWEEN 2601 AND 2707;' + body + assertions + 'ROLLBACK;')
# Inclusive wear/resource bounds must not accidentally discard level-zero or
# level-sixty ordinary bases. Resource tiers have no arbitrary upper cap.
for wear, resource in [(0, 0), (60, 99)]:
    run(f'eligible-wear-boundary-{wear}', f'BEGIN;UPDATE game_equipment SET level_req={wear} WHERE id=2601;UPDATE gamedata.ov_arm SET level_need={wear},level={resource} WHERE index=2601;' + body + assertions + 'ROLLBACK;')
# Editing the source after publication cannot alter the frozen membership.
run('frozen-source-membership', 'BEGIN;' + body + 'DELETE FROM game_equipment WHERE id=2707;' + fallback_assertion + 'ROLLBACK;')
run('replay-source-conflict', 'BEGIN;' + body + 'DELETE FROM game_equipment WHERE id=2707;' + body + 'COMMIT;', 'rift11 is occupied')
for table, column in [('game_rift_class_rules', 'damage_bp'), ('game_rift_ability_rules', 'damage_pct')]:
    run('replay-dependency-conflict-' + table, 'BEGIN;' + body + f'ALTER TABLE {table} DISABLE TRIGGER immutable_published_rules;UPDATE {table} SET {column}={column}+1 WHERE version=11;ALTER TABLE {table} ENABLE TRIGGER immutable_published_rules;' + body + 'COMMIT;', 'must preserve class and ability dependencies')
    run('published-v11-child-immutable-' + table, 'BEGIN;' + body + f'UPDATE {table} SET {column}={column}+1 WHERE version=11;COMMIT;', 'immutable')
run('published-v11-immutable', 'BEGIN;' + body + 'UPDATE game_rift_rule_versions SET normal_boss_equipment=11 WHERE version=11;COMMIT;', 'immutable')
run('published-v11-fallback-immutable', 'BEGIN;' + body + "UPDATE game_rift_rule_versions SET equipment_slot_fallbacks='[]'::jsonb WHERE version=11;COMMIT;", 'immutable')
run('published-v10-immutable', 'BEGIN;UPDATE game_rift_rule_versions SET normal_boss_equipment=11 WHERE version=10;COMMIT;', 'immutable')
run('published-v10-null-fallback-immutable', 'BEGIN;' + body + "UPDATE game_rift_rule_versions SET equipment_slot_fallbacks='[]'::jsonb WHERE version=10;COMMIT;", 'immutable')
print('All focused PostgreSQL migration checks passed. No live DB or full initializer was used.')
