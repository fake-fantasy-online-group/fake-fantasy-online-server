#!/usr/bin/env python3
"""Real PostgreSQL migration88/explicit upgrade checks in disposable clusters.

PG_BIN must contain trusted postgres/initdb binaries. No DSN, network, production
database or psql settings are used. --work-dir must not exist. Optional --seed-dir
is an idle disposable migration87 cluster, copied before any SQL is executed.
"""
import argparse
import copy
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess

from validate_daoist_postgres import concatenate_sources, write_single_user_commands

ROOT = Path(__file__).resolve().parents[3]
MIGRATIONS = ROOT / 'server/migrations'
FLAGS = ('aggressive', 'call_help', 'flee', 'pickup')
DEFAULTS = dict(flee_hp_percent=20, flee_duration_ms=3000, flee_cooldown_ms=10000)
EXPECTED_COUNTS = [68, 23, 15, 27]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--work-dir', type=Path, required=True)
    parser.add_argument('--seed-dir', type=Path)
    args = parser.parse_args()
    work = args.work_dir.resolve()
    work.mkdir(parents=True, exist_ok=False)
    bins = Path(os.environ['PG_BIN']).resolve()
    baseline = work / 'baseline'
    report = {'engine': subprocess.check_output([str(bins / 'postgres'), '--version'], text=True).strip(),
              'scope': 'Real PostgreSQL, socket-free single-user mode; no pgx/network/concurrent-session certification',
              'source_sha256': {}, 'checks': []}
    for path in [*(MIGRATIONS / f'000000_init-{i}.sql' for i in range(89)),
                 MIGRATIONS / 'upgrades/20261019_monster_behavior.sql']:
        report['source_sha256'][str(path.relative_to(ROOT))] = hashlib.sha256(path.read_bytes()).hexdigest()

    def run(data, label, sql=None, paths=None, expected_error=None, verbatim=False):
        if (data / 'postmaster.pid').exists():
            raise RuntimeError('refusing an active or stale-postmaster cluster')
        inp, log = work / (label + '.sql'), work / (label + '.log')
        if paths:
            raw = work / (label + '.source.sql')
            with raw.open('wb') as out:
                out.write(b'BEGIN;\n')
                concatenate_sources(paths, out)
                out.write(b'\nCOMMIT;\n')
            with raw.open() as source, inp.open('w') as target:
                write_single_user_commands(source, target)
            raw.unlink()
        elif verbatim:
            # These two complete migration files contain no single-user input
            # delimiter inside a literal/DO body. Preserve even dollar quoting
            # for their first application, testing the original SQL syntax.
            inp.write_text(sql)
        else:
            with inp.open('w') as target:
                write_single_user_commands(io.StringIO(sql), target)
        with inp.open() as source, log.open('w') as out:
            result = subprocess.run([str(bins / 'postgres'), '--single', '-j', '-D', str(data), 'postgres'],
                                    stdin=source, stdout=out, stderr=subprocess.STDOUT)
        errors = [line for line in log.read_text().splitlines() if re.search(r'\b(?:ERROR|FATAL|PANIC):', line)]
        if expected_error:
            assert result.returncode == 0 and any(expected_error in e for e in errors), (label, errors[:3])
            assert all(expected_error in e or 'current transaction is aborted' in e for e in errors), (label, errors[:3])
        else:
            assert result.returncode == 0 and not errors, (label, errors[:3])
        inp.unlink()

    def passed(label):
        report['checks'].append(label)
        print('PASS', label, flush=True)

    def export(data, label, query):
        target = work / (label + '.jsonl')
        escaped = str(target).replace("'", "''")
        run(data, label, f"COPY (SELECT to_jsonb(r) FROM ({query}) r) TO '{escaped}';")
        # COPY text escapes backslashes, tabs and newlines in each JSON record.
        return [json.loads(line.replace('\\\\', '\\')) for line in target.read_text().splitlines()]

    def rows(data, label, table):
        return {row['id']: row for row in export(data, label, f'SELECT * FROM {table} ORDER BY id')}

    def fingerprints(data, label):
        target = work / (label + '.jsonl')
        escaped = str(target).replace("'", "''")
        run(data, label, """
CREATE TEMP TABLE audit_fingerprints(name text, row_count bigint, sha256 text);
DO $$ DECLARE r record; BEGIN
 FOR r IN SELECT n.nspname, c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE c.relkind='r' AND n.nspname IN ('public','gamedata')
 LOOP
  EXECUTE format('INSERT INTO audit_fingerprints SELECT %L,count(*),encode(sha256(convert_to(COALESCE(string_agg(h, '''' ORDER BY h),''''),''UTF8'')),''hex'') FROM (SELECT encode(sha256(convert_to(to_jsonb(t)::text,''UTF8'')),''hex'') h FROM %I.%I t) hashes', r.nspname||'.'||r.relname,r.nspname,r.relname);
 END LOOP;
END $$;
""" + f"COPY (SELECT to_jsonb(t) FROM audit_fingerprints t ORDER BY name) TO '{escaped}';")
        return {r['name']: r for r in map(json.loads, target.read_text().splitlines())}

    if args.seed_dir:
        seed = args.seed_dir.resolve()
        if (seed / 'postmaster.pid').exists() or not (seed / 'PG_VERSION').is_file():
            raise RuntimeError('seed must be an idle disposable PostgreSQL cluster')
        shutil.copytree(seed, baseline)
        report['baseline'] = 'Copied idle disposable migration87 seed; source is never modified'
    else:
        with (work / 'initdb.log').open('w') as log:
            subprocess.run([str(bins / 'initdb'), '-D', str(baseline), '--no-locale', '-E', 'UTF8',
                            '-U', 'validator', '-A', 'trust'], check=True, stdout=log, stderr=subprocess.STDOUT)
        run(baseline, 'initialize-0-through-87', paths=[MIGRATIONS / f'000000_init-{i}.sql' for i in range(88)])
        report['baseline'] = 'Empty cluster initialized transactionally with the complete, ordered 0–87 source SQL'
    run(baseline, 'require-old-schema', """DO $$ BEGIN
 IF EXISTS (SELECT FROM information_schema.columns WHERE table_name='game_monsters' AND column_name IN ('flee','pickup'))
 OR EXISTS (SELECT FROM information_schema.columns WHERE table_name='game_monster_ai_templates' AND column_name LIKE 'flee_%')
 THEN RAISE EXCEPTION 'fixture must precede migration88'; END IF;
END $$;""")
    approved = {r['id']: r for r in json.loads((ROOT / 'server/internal/data/testdata/monster-behavior-approved.json').read_text())}
    before_monsters = rows(baseline, 'baseline-monsters', 'game_monsters')
    before_ai = rows(baseline, 'baseline-ai', 'game_monster_ai_templates')
    before_tables = fingerprints(baseline, 'baseline-all-tables')
    scoped = lambda r: 1 <= r['level'] <= 60 and r['hp'] > 0 and r['kind'] in ('普通', '精英', 'BOSS')
    for identity, decision in approved.items():
        assert before_monsters[identity]['name'] == decision['name'] and scoped(before_monsters[identity]), identity
    report['fixture'] = {'tables': len(before_tables), 'monsters': len(before_monsters),
                         'scoped_monsters': sum(map(scoped, before_monsters.values())), 'approved_identities': len(approved),
                         'above_level_60': sum(r['level'] > 60 for r in before_monsters.values()),
                         'noncombat_or_placeholder': sum(r['kind'] not in ('普通', '精英', 'BOSS') or r['level'] <= 0 or r['hp'] <= 0 for r in before_monsters.values())}
    passed('Original database rows independently match every approved identity and scope')
    expected_monsters = copy.deepcopy(before_monsters)
    for identity, row in expected_monsters.items():
        row.update(flee=False, pickup=False)
        if scoped(row):
            row.update({flag: approved.get(identity, {}).get(flag, False) for flag in FLAGS})
    expected_ai = {identity: dict(row, **DEFAULTS) for identity, row in before_ai.items()}
    seed_sql = (MIGRATIONS / '000000_init-88.sql').read_text()
    upgrade_sql = (MIGRATIONS / 'upgrades/20261019_monster_behavior.sql').read_text()
    assert seed_sql in upgrade_sql

    for mode, migration in [('init88', 'BEGIN;\n' + seed_sql + '\nCOMMIT;'), ('upgrade', upgrade_sql)]:
        data = work / mode
        shutil.copytree(baseline, data)
        # The guard fails AFTER the DDL and UPDATE; prove both roll back.
        run(data, mode + '-damage', "UPDATE game_monsters SET name='intentional-test-drift' WHERE id=1002;")
        damaged_tables = fingerprints(data, mode + '-damaged-before')
        run(data, mode + '-reject-old-schema-drift', migration, expected_error='approved identity/scope mismatch')
        assert fingerprints(data, mode + '-damaged-after') == damaged_tables
        run(data, mode + '-assert-ddl-rollback', """DO $$ BEGIN
 IF EXISTS (SELECT FROM information_schema.columns WHERE table_name='game_monsters' AND column_name IN ('flee','pickup'))
 OR EXISTS (SELECT FROM information_schema.columns WHERE table_name='game_monster_ai_templates' AND column_name LIKE 'flee_%')
 THEN RAISE EXCEPTION 'DDL leaked from failed migration'; END IF;
END $$; UPDATE game_monsters SET name='食人花' WHERE id=1002;""")
        passed(mode + ': failure rolls back all DDL and all table data')
        run(data, mode + '-first-apply', migration, verbatim=True)
        actual = rows(data, mode + '-first-monsters', 'game_monsters')
        assert actual == expected_monsters
        assert rows(data, mode + '-first-ai', 'game_monster_ai_templates') == expected_ai
        counts = [sum(row[flag] for row in actual.values() if scoped(row)) for flag in FLAGS]
        assert counts == EXPECTED_COUNTS, counts
        after_tables = fingerprints(data, mode + '-first-all-tables')
        unrelated = lambda tables: {k: v for k, v in tables.items() if k not in ('public.game_monsters', 'public.game_monster_ai_templates')}
        assert unrelated(after_tables) == unrelated(before_tables)
        passed(mode + ': counts 68/23/15/27; exact per-row diff; all unrelated tables unchanged')
        for repeat in range(2):
            run(data, f'{mode}-repeat-{repeat}', migration)
        assert fingerprints(data, mode + '-replayed-all-tables') == after_tables
        passed(mode + ': repeated application is exactly idempotent')
        # Pre-existing new columns and custom flee tuning are preserved outside
        # the approved data scope. Boundaries are separate rather than conflated.
        run(data, mode + '-boundary-setup', """
UPDATE game_monster_ai_templates SET flee_hp_percent=35,flee_duration_ms=5000,flee_cooldown_ms=12000 WHERE id='melee';
INSERT INTO game_monsters(id,sprite,name,raw,level,hp,kind,aggressive,call_help,flee,pickup) VALUES
 (900001,1,'unlisted-at60','',60,100,'精英',true,true,true,true),
 (900002,1,'above60','',61,100,'普通',true,true,true,true),
 (900003,1,'noncombat','',60,100,'场景物件',true,true,true,true),
 (900004,1,'level-zero','',0,100,'普通',true,true,true,true),
 (900005,1,'hp-zero','',60,0,'普通',true,true,true,true),
 (900006,1,'unlisted-at1','',1,100,'BOSS',true,true,true,true);
""")
        boundary = rows(data, mode + '-boundaries-before', 'game_monsters')
        for identity in (900001, 900006):
            boundary[identity].update({flag: False for flag in FLAGS})
        custom_ai = rows(data, mode + '-custom-ai-before', 'game_monster_ai_templates')
        run(data, mode + '-boundary-apply', migration)
        assert rows(data, mode + '-boundaries-after', 'game_monsters') == boundary
        assert rows(data, mode + '-custom-ai-after', 'game_monster_ai_templates') == custom_ai
        passed(mode + ': level1/60 inclusive; >60/noncombat/zero-level/zero-HP excluded; custom AI preserved')
        for label, mutation in [('name', "name='renamed'"), ('level', 'level=61'), ('hp', 'hp=0'), ('kind', "kind='场景物件'")]:
            run(data, mode + '-drift-' + label, 'UPDATE game_monsters SET ' + mutation + ' WHERE id=1002; UPDATE game_monsters SET aggressive=false WHERE id=1003;')
            drifted = rows(data, mode + '-drift-before-' + label, 'game_monsters')
            run(data, mode + '-reject-' + label, migration, expected_error='approved identity/scope mismatch')
            assert rows(data, mode + '-drift-after-' + label, 'game_monsters') == drifted
            old = before_monsters[1002]
            run(data, mode + '-restore-' + label, f"UPDATE game_monsters SET name='食人花',level={old['level']},hp={old['hp']},kind='{old['kind']}' WHERE id=1002;")
        passed(mode + ': renamed/out-of-level/zero-HP/noncombat approved identity rejects atomically')
        run(data, mode + '-missing-approved-identity', """
CREATE TEMP TABLE saved_monster AS SELECT * FROM game_monsters WHERE id=12176;
CREATE TEMP TABLE saved_family AS SELECT * FROM game_monster_families WHERE monster_id=12176;
DELETE FROM game_monsters WHERE id=12176;
UPDATE game_monsters SET aggressive=false WHERE id=1003;
""" + migration + """
DO $$ BEGIN
 IF EXISTS(SELECT FROM game_monsters WHERE id=12176) OR
    (SELECT aggressive FROM game_monsters WHERE id=1003)
 THEN RAISE EXCEPTION 'missing identity migration did not roll back'; END IF;
END $$;
INSERT INTO game_monsters SELECT * FROM saved_monster;
INSERT INTO game_monster_families SELECT * FROM saved_family;
""", expected_error='approved identity/scope mismatch')
        passed(mode + ': deleted approved identity rejects without a partial behavior update')
        # Roll back constraint probes, keeping the final cluster usable by the
        # real Go loader. Every invalid value must be rejected by PostgreSQL.
        for column, values in [('flee_hp_percent', [0, 100]), ('flee_duration_ms', [0, -1]), ('flee_cooldown_ms', [0, -1])]:
            for value in values:
                run(data, f'{mode}-constraint-{column}-{value}', f"BEGIN; UPDATE game_monster_ai_templates SET {column}={value} WHERE id='melee'; COMMIT;", expected_error='violates check constraint')
        passed(mode + ': PostgreSQL rejects all six invalid flee configuration boundaries')
        run(data, mode + '-restore-clean-fixture', "DELETE FROM game_monsters WHERE id BETWEEN 900001 AND 900006; UPDATE game_monster_ai_templates SET flee_hp_percent=20,flee_duration_ms=3000,flee_cooldown_ms=10000 WHERE id='melee';")
        run(data, mode + '-final-replay', migration)
        assert fingerprints(data, mode + '-final-all-tables') == after_tables
    report['passed'] = True
    (work / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print('MONSTER_BEHAVIOR_POSTGRES_PASS', flush=True)
    print('FANTASY_MONSTER_BEHAVIOR_PG_DATA=' + str(work / 'upgrade'), flush=True)


if __name__ == '__main__':
    main()
