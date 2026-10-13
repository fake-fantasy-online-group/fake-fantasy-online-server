#!/usr/bin/env python3
"""Verify migration77 in a new disposable, socket-free PostgreSQL cluster.

Optionally copy an idle disposable fixture through migration76 with --seed-dir.
The input cluster is never opened or changed. The output remains at migration77
so the production Go loader can verify both historical and active rule versions.
"""
import argparse
import os
from pathlib import Path
import re
import shutil
import subprocess

from validate_daoist_postgres import concatenate_sources, write_single_user_commands

ROOT = Path(__file__).resolve().parents[3]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--work-dir', required=True, type=Path)
parser.add_argument('--seed-dir', type=Path)
args = parser.parse_args()
work = args.work_dir.resolve()
work.mkdir(parents=True, exist_ok=False)
bins = Path(os.environ['PG_BIN'])
data = work / 'data'
if args.seed_dir:
    seed = args.seed_dir.resolve()
    if not (seed / 'PG_VERSION').is_file() or (seed / 'postmaster.pid').exists():
        raise SystemExit('refusing missing or running/stale-postmaster test cluster')
    shutil.copytree(seed, data)
else:
    command = [str(bins / 'initdb'), '-D', str(data), '--no-locale', '-E', 'UTF8',
               '-U', 'validator', '-A', 'trust']
    if os.environ.get('PG_SHARE'):
        command += ['-L', os.environ['PG_SHARE']]
    subprocess.run(command, check=True, stdout=(work / 'init.log').open('w'),
                   stderr=subprocess.STDOUT)


def run(label, sql=None, paths=None, expected_error=None):
    raw, commands, log = [work / (label + suffix)
                          for suffix in ('.source.sql', '.sql', '.log')]
    with raw.open('wb') as out:
        if paths:
            # Production concatenates all migration fragments in one transaction.
            out.write(b'BEGIN;\n')
            concatenate_sources(paths, out)
            out.write(b'\nCOMMIT;\n')
        else:
            out.write(sql.encode())
    with raw.open() as source, commands.open('w') as out:
        write_single_user_commands(source, out)
    raw.unlink()
    with commands.open() as source, log.open('w') as out:
        result = subprocess.run([str(bins / 'postgres'), '--single', '-j', '-D',
                                 str(data), 'postgres'], stdin=source, stdout=out,
                                stderr=subprocess.STDOUT)
    errors = []
    with log.open() as source:
        for line in source:
            if re.search(r'\b(?:ERROR|FATAL|PANIC):', line):
                errors.append(line.strip())
    assert result.returncode == 0, (label, result.returncode, errors[:3])
    if expected_error:
        assert any(expected_error in error for error in errors), (label, errors[:3])
        assert all(expected_error in error or 'current transaction is aborted' in error
                   for error in errors), (label, errors[:3])
    else:
        assert not errors, (label, errors[:3])
    commands.unlink()
    print('PASS', label, flush=True)


def clone(version_table, children, source, target, changes='{}'):
    """Copy a complete unpublished version without bypassing immutable guards."""
    statements = [f"""INSERT INTO {version_table}
 SELECT (jsonb_populate_record(NULL::{version_table},to_jsonb(r)||
 '{{"version":{target},"published":false}}'::jsonb||'{changes}'::jsonb)).*
 FROM {version_table} r WHERE version={source};"""]
    for table in children:
        statements.append(f"""INSERT INTO {table}
 SELECT (jsonb_populate_record(NULL::{table},to_jsonb(r)||
 '{{"version":{target}}}'::jsonb)).* FROM {table} r WHERE version={source};""")
    return '\n'.join(statements)


loot_children = ['game_loot_rune_pool', 'game_loot_socket_levels', 'game_loot_grades']
rift_children = ['game_rift_class_rules', 'game_rift_ability_rules']
loot_table, rift_table = 'game_loot_rule_versions', 'game_rift_rule_versions'
tables = [loot_table, *loot_children, rift_table, *rift_children]
body = (ROOT / 'server/migrations/000000_init-77.sql').read_text()
if not args.seed_dir:
    run('fresh-through76', paths=[ROOT / f'server/migrations/000000_init-{i}.sql'
                                 for i in range(77)])

unmigrated = """DO $$ BEGIN
 IF EXISTS(SELECT FROM pg_attribute WHERE attrelid='game_loot_rune_pool'::regclass
           AND attname='weight' AND NOT attisdropped) OR
    EXISTS(SELECT FROM game_loot_rule_versions WHERE version<>1) OR
    EXISTS(SELECT FROM game_rift_rule_versions WHERE version NOT IN(1,2)) OR
    (SELECT version FROM game_loot_rule_active WHERE id=1)<>1 OR
    (SELECT version FROM game_rift_rule_active WHERE id=1)<>2 OR
    NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=2 AND published
               AND loot_version=1 AND floor_population_multiplier=2)
 THEN RAISE EXCEPTION 'fixture must be unchanged through migration76'; END IF;
END $$;"""
run('require-unmigrated', sql=unmigrated)
run('atomic-rollback', sql='BEGIN;\n' + body + """
DO $$ BEGIN RAISE EXCEPTION 'deliberate rune weight rollback'; END $$; COMMIT;
""", expected_error='deliberate rune weight rollback')
run('rollback-left-no-changes', sql=unmigrated)

for label, change, error in [
    ('duplicate-level', 'UPDATE gamedata.ov_item SET level_need=12 WHERE index=1900028002;',
     '28 equal-weight runes'),
    ('missing-level', 'UPDATE gamedata.ov_item SET level_need=0 WHERE index=1900028001;',
     '28 equal-weight runes'),
    ('missing-rune', 'DELETE FROM game_endgame_rune_rules WHERE item_id=1900028001;',
     '28 equal-weight runes'),
    ('occupied-loot2', clone(loot_table, loot_children, 1, 2,
                            '{"unique_base_ppb":999}') +
     'UPDATE game_loot_rule_versions SET published=true WHERE version=2;',
     'loot rules version 2 is already occupied'),
    ('occupied-loot2-children', clone(loot_table, loot_children, 1, 2) +
     'UPDATE game_loot_rule_versions SET published=true WHERE version=2;',
     'loot rules version 2 must contain exactly'),
    ('occupied-rift3', clone(rift_table, rift_children, 2, 3) +
     'UPDATE game_rift_rule_versions SET published=true WHERE version=3;',
     'rift rules version 3 is already occupied'),
]:
    run('reject-' + label, sql='BEGIN;\n' + change + '\n' + body + '\nCOMMIT;',
        expected_error=error)
run('rejections-left-no-changes', sql=unmigrated)

# Rank by actual physical-card levels, even if catalog IDs are out of order.
run('rank-by-level-not-id', sql="""BEGIN;
UPDATE gamedata.ov_item SET level_need=CASE index WHEN 1900028001 THEN 14 ELSE 12 END
 WHERE index IN(1900028001,1900028002);
""" + body + """
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM game_loot_rune_pool WHERE version=2 AND seq=1
               AND item_id=1900028002 AND weight=129) OR
    NOT EXISTS(SELECT FROM game_loot_rune_pool WHERE version=2 AND seq=2
               AND item_id=1900028001 AND weight=125)
 THEN RAISE EXCEPTION 'rune weight order ignored actual physical-card levels'; END IF;
END $$; ROLLBACK;
""")

future = (clone(loot_table, loot_children, 1, 99) +
          'UPDATE game_loot_rule_versions SET published=true WHERE version=99;\n' +
          clone(rift_table, rift_children, 2, 99, '{"loot_version":99}') +
          'UPDATE game_rift_rule_versions SET published=true WHERE version=99;\n'
          'UPDATE game_loot_rule_active SET version=99 WHERE id=1;\n'
          'UPDATE game_rift_rule_active SET version=99 WHERE id=1;\n')
preserve_future = """DO $$ BEGIN
 IF (SELECT version FROM game_loot_rule_active WHERE id=1)<>99 OR
    (SELECT version FROM game_rift_rule_active WHERE id=1)<>99 OR
    NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=99 AND loot_version=99)
 THEN RAISE EXCEPTION 'migration replaced newer active versions'; END IF;
END $$;"""
run('first-import-preserves-future-active', sql='BEGIN;\n' + future + '\n' + body +
    '\n' + preserve_future + '\nROLLBACK;')

for table in tables:
    run('snapshot-' + table, sql=f'CREATE TABLE before_rune77_{table} AS '
        f'SELECT to_jsonb(r) AS row FROM {table} r;')
run('snapshot-physical-cards', sql="""CREATE TABLE before_rune77_cards AS
 SELECT to_jsonb(r) AS row FROM gamedata.ov_item r WHERE index BETWEEN 1900028001 AND 1900028028;
""")
run('first-import', sql='BEGIN;\n' + body + '\nCOMMIT;')
run('repeat-import', sql='BEGIN;\n' + body + '\nCOMMIT;')
run('exact-weight-and-version-contract', sql="""DO $$ BEGIN
 IF (SELECT version FROM game_loot_rule_active WHERE id=1)<>2 OR
    (SELECT version FROM game_rift_rule_active WHERE id=1)<>3 OR
    (SELECT count(*) FROM game_loot_rune_pool WHERE version=2)<>28 OR
    (SELECT sum(weight) FROM game_loot_rune_pool WHERE version=2)<>2100 OR
    EXISTS(SELECT FROM game_loot_rune_pool WHERE version=2 AND weight<>133-4*seq) OR
    NOT EXISTS(SELECT FROM game_loot_rune_pool p JOIN gamedata.ov_item i ON i.index=p.item_id
               WHERE p.version=2 AND p.seq=28 AND p.weight=21 AND i.level_need=60) OR
    EXISTS(SELECT FROM game_loot_rune_pool WHERE version=1 AND weight<>1) OR
    EXISTS(SELECT FROM (SELECT i.level_need,lag(i.level_need) OVER(ORDER BY p.seq) AS previous
                        FROM game_loot_rune_pool p JOIN gamedata.ov_item i ON i.index=p.item_id
                        WHERE p.version=2) ranked WHERE level_need<=previous) OR
    (SELECT to_jsonb(r)-'version' FROM game_loot_rule_versions r WHERE version=1)
     IS DISTINCT FROM (SELECT to_jsonb(r)-'version' FROM game_loot_rule_versions r WHERE version=2) OR
    (SELECT to_jsonb(r)-ARRAY['version','loot_version'] FROM game_rift_rule_versions r WHERE version=2)
     IS DISTINCT FROM (SELECT to_jsonb(r)-ARRAY['version','loot_version'] FROM game_rift_rule_versions r WHERE version=3) OR
    NOT EXISTS(SELECT FROM game_rift_rule_versions WHERE version=3 AND published
                AND loot_version=2 AND floor_population_multiplier=2)
 THEN RAISE EXCEPTION 'weighted pool/version contract changed'; END IF;
END $$;""")
for table in tables:
    version_filter = 'version=1' if table.startswith('game_loot_') else 'version IN(1,2)'
    projection = "to_jsonb(r)-'weight'" if table == 'game_loot_rune_pool' else 'to_jsonb(r)'
    run('preserve-' + table, sql=f"""DO $$ BEGIN
 IF EXISTS((SELECT row FROM before_rune77_{table}
            EXCEPT ALL SELECT {projection} FROM {table} r WHERE {version_filter})
 UNION ALL (SELECT {projection} FROM {table} r WHERE {version_filter}
            EXCEPT ALL SELECT row FROM before_rune77_{table}))
 THEN RAISE EXCEPTION 'historical rules changed: {table}'; END IF;
END $$;""")
run('preserve-physical-cards', sql="""DO $$ BEGIN
 IF EXISTS((SELECT row FROM before_rune77_cards EXCEPT ALL SELECT to_jsonb(r)
             FROM gamedata.ov_item r WHERE index BETWEEN 1900028001 AND 1900028028)
 UNION ALL (SELECT to_jsonb(r) FROM gamedata.ov_item r WHERE index BETWEEN 1900028001 AND 1900028028
            EXCEPT ALL SELECT row FROM before_rune77_cards))
 THEN RAISE EXCEPTION 'physical rune definitions changed'; END IF;
END $$;""")
run('published-versions-still-immutable', sql="""DO $$ DECLARE command TEXT; BEGIN
 FOREACH command IN ARRAY ARRAY[
  'UPDATE game_loot_rune_pool SET weight=2 WHERE version=1 AND seq=1',
  'UPDATE game_loot_rune_pool SET weight=130 WHERE version=2 AND seq=1',
  'DELETE FROM game_loot_rune_pool WHERE version=2 AND seq=1',
  'UPDATE game_loot_rule_versions SET min_rune_level=21 WHERE version=2',
  'UPDATE game_rift_rule_versions SET loot_version=2 WHERE version=2',
  'UPDATE game_rift_rule_versions SET loot_version=1 WHERE version=3',
  'UPDATE game_rift_class_rules SET hp_bp=999 WHERE version=3',
  'DELETE FROM game_rift_ability_rules WHERE version=3'
 ] LOOP
  BEGIN
   EXECUTE command;
   RAISE EXCEPTION 'published mutation unexpectedly succeeded';
  EXCEPTION WHEN OTHERS THEN
   IF SQLERRM NOT LIKE '%immutable%' THEN RAISE; END IF;
  END;
 END LOOP;
END $$;""")
run('replay-preserves-future-active', sql='BEGIN;\n' + future + '\n' + body + '\n' +
    preserve_future + '\nROLLBACK;')
run('weight-default', sql='BEGIN;\n' + clone(loot_table, [], 1, 99) + """
INSERT INTO game_loot_rune_pool(version,seq,item_id) VALUES(99,1,1900028001);
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM game_loot_rune_pool WHERE version=99 AND weight=1)
 THEN RAISE EXCEPTION 'legacy default weight missing'; END IF;
END $$; ROLLBACK;
""")
for label, weight in [('zero', '0'), ('negative', '-1'), ('too-large', '1000001'), ('null', 'NULL')]:
    run('reject-weight-' + label, sql='BEGIN;\n' + clone(loot_table, [], 1, 99) +
        f'INSERT INTO game_loot_rune_pool(version,seq,item_id,weight) VALUES(99,1,1900028001,{weight}); COMMIT;',
        expected_error='not-null constraint' if weight == 'NULL' else 'check constraint')
run('final-clean-replay', sql='BEGIN;\n' + body + '\nCOMMIT;')
print('RUNE_LOOT_WEIGHTS_POSTGRES_PASS', flush=True)
print('FANTASY_RUNE_WEIGHTS_PG_DATA=' + str(data), flush=True)
