#!/usr/bin/env python3
"""Socket-free real PostgreSQL checks. PG_BIN must name trusted PostgreSQL bin.
Creates a new disposable cluster below --work-dir; never targets an existing DB.
Example: PG_BIN=/usr/lib/postgresql/17/bin python3 server/tests/sql/validate_daoist_postgres.py --work-dir /tmp/daoist-pg-check
"""
import argparse
import os
from pathlib import Path
import re
import subprocess

REPO = Path(__file__).resolve().parents[3]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--work-dir', required=True, type=Path)
    args = parser.parse_args()
    work = args.work_dir.resolve()
    if work.exists():
        parser.error('--work-dir must not exist (a fresh disposable directory is required)')
    bins = Path(os.environ['PG_BIN'])
    work.mkdir(parents=True)
    data = work / 'data'
    subprocess.run([str(bins / 'initdb'), '-D', str(data), '--no-locale', '-E', 'UTF8', '-U', 'validator', '-A', 'trust'], check=True, stdout=(work / 'initdb.log').open('w'), stderr=subprocess.STDOUT)

    def run(database, label, sql):
        # -j uses a genuinely empty line as the command delimiter. Whitespace-only
        # lines keep complete DO/function blocks intact; comments and newlines stay.
        body = '\n'.join(line if line else ' ' for line in sql.split('\n')) + '\n\n'
        result = subprocess.run([str(bins / 'postgres'), '--single', '-j', '-D', str(data), database], input=body, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        (work / f'{label}.log').write_text(result.stdout)
        # The standalone backend can exit zero even after SQL errors.
        if result.returncode or re.search(r'\b(?:ERROR|FATAL|PANIC):', result.stdout):
            raise RuntimeError(f'{label} failed; see {work / (label + ".log")}')
        print(f'PASS {label}', flush=True)
        return result.stdout

    # CREATE DATABASE cannot share an implicit transaction with a second command.
    run('postgres', 'create-fresh', 'CREATE DATABASE fresh_test;')
    run('postgres', 'create-upgrade', 'CREATE DATABASE upgrade_test;')
    fragments = [REPO / f'server/migrations/000000_init-{i}.sql' for i in range(51)]
    init = ''.join(p.read_text() for p in fragments)
    run('fresh_test', 'fresh-init-0-through-50', 'BEGIN;\n' + init + '\nCOMMIT;')
    assertions = (REPO / 'server/tests/sql/daoist_endgame_assertions.sql').read_text()
    result = run('fresh_test', 'fresh-assertions', assertions)
    assert 'DAOIST_ENDGAME_SQL_PASS' in result
    baseline = ''.join(p.read_text() for p in fragments[:50])
    run('upgrade_test', 'baseline-init-0-through-49', 'BEGIN;\n' + baseline + '\nCOMMIT;')
    run('upgrade_test', 'seed-existing-character', "INSERT INTO accounts(id,username) VALUES(930000001,'pre_upgrade'); INSERT INTO characters(id,account_id,slot,name,race,map_id,pos_x,pos_y) VALUES(930000001,930000001,0,'pre_upgrade',1,7,10,20);")
    migration = (REPO / 'docs/migrations/20261011_daoist_endgame_v1.sql').read_text()
    # Exercise an already-installed original v1 schema, not only a pre-v1 DB.
    run('upgrade_test', 'seed-original-unique-only-schema', """
CREATE TABLE equipment_endgame_state (
 uid BIGINT PRIMARY KEY REFERENCES item_instances(uid) ON DELETE CASCADE,
 revision BIGINT NOT NULL CHECK(revision >= 0),
 unique_id INTEGER NOT NULL CHECK(unique_id > 0),
 unique_roll_bp INTEGER NOT NULL CHECK(unique_roll_bp BETWEEN 8000 AND 12000)
);
INSERT INTO item_instances(uid,item_id,instance_kind) VALUES(930000009,32001,1);
INSERT INTO equipment_endgame_state VALUES(930000009,9,32001,9000);
""")
    run('upgrade_test', 'upgrade-first', migration)
    run('upgrade_test', 'seed-endgame-persistence', """
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM characters WHERE id=930000001 AND trial_key_reward=0 AND trial_completion_id='') THEN RAISE EXCEPTION 'upgrade defaults missing'; END IF;
END $$;
UPDATE characters SET trial_key_reward=7,trial_completion_id='persisted-completion' WHERE id=930000001;
INSERT INTO item_instances(uid,item_id,instance_kind) VALUES(930000001,32001,1);
INSERT INTO equipment_instances(uid,refine_level) VALUES(930000001,4);
INSERT INTO equipment_endgame_state VALUES(930000001,2,32001,11000);
INSERT INTO item_instances(uid,item_id,instance_kind) VALUES(930000002,1001,1);
INSERT INTO equipment_instances(uid,socket_count) VALUES(930000002,2);
INSERT INTO equipment_endgame_state VALUES(930000002,2,0,0);
INSERT INTO character_endgame_state VALUES(930000001,3,'{"keys":7}');
INSERT INTO endgame_receipts VALUES(930000001,'persisted-request','claim',repeat('c',64),3,'{"ok":true}');
INSERT INTO endgame_runs VALUES('persisted-run',2,'completed','{"result":"won"}');
CREATE TABLE validation_schema_before AS SELECT md5(string_agg(table_name||column_name||data_type||is_nullable||coalesce(column_default,''),',' ORDER BY table_name,ordinal_position)) AS fingerprint FROM information_schema.columns WHERE table_schema='public' AND (table_name LIKE '%endgame%' OR table_name='characters');
""")
    run('upgrade_test', 'upgrade-second', migration)
    # Every run() starts a new standalone process and reads the on-disk database.
    run('upgrade_test', 'restart-persistence-idempotence', """
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM characters WHERE id=930000001 AND trial_key_reward=7 AND trial_completion_id='persisted-completion') THEN RAISE EXCEPTION 'trial persistence'; END IF;
 IF NOT EXISTS(SELECT FROM equipment_endgame_state WHERE uid=930000001 AND revision=2 AND unique_roll_bp=11000) THEN RAISE EXCEPTION 'overlay persistence'; END IF;
 IF NOT EXISTS(SELECT FROM equipment_endgame_state WHERE uid=930000002 AND revision=2 AND unique_id=0 AND unique_roll_bp=0) THEN RAISE EXCEPTION 'ordinary revision persistence'; END IF;
 IF NOT EXISTS(SELECT FROM equipment_endgame_state WHERE uid=930000009 AND revision=9 AND unique_id=32001 AND unique_roll_bp=9000) THEN RAISE EXCEPTION 'original overlay preserved'; END IF;
 IF NOT EXISTS(SELECT FROM equipment_instances WHERE uid=930000001 AND refine_level=4) THEN RAISE EXCEPTION 'native item persistence'; END IF;
 IF NOT EXISTS(SELECT FROM character_endgame_state WHERE char_id=930000001 AND revision=3 AND payload->>'keys'='7') THEN RAISE EXCEPTION 'state persistence'; END IF;
 IF NOT EXISTS(SELECT FROM endgame_receipts WHERE char_id=930000001 AND request_id='persisted-request' AND response->>'ok'='true') THEN RAISE EXCEPTION 'receipt persistence'; END IF;
 IF NOT EXISTS(SELECT FROM endgame_runs WHERE run_id='persisted-run' AND revision=2 AND state='completed' AND payload->>'result'='won') THEN RAISE EXCEPTION 'run persistence'; END IF;
 IF (SELECT fingerprint FROM validation_schema_before) IS DISTINCT FROM (SELECT md5(string_agg(table_name||column_name||data_type||is_nullable||coalesce(column_default,''),',' ORDER BY table_name,ordinal_position)) FROM information_schema.columns WHERE table_schema='public' AND (table_name LIKE '%endgame%' OR table_name='characters')) THEN RAISE EXCEPTION 'second upgrade changed schema'; END IF;
END $$;
SELECT 'DAOIST_ENDGAME_RESTART_PASS';
""")
    result = run('upgrade_test', 'upgrade-assertions', assertions)
    assert 'DAOIST_ENDGAME_SQL_PASS' in result
    print('All SQL checks passed. pgx/network/concurrent-lock tests were NOT run.')


if __name__ == '__main__':
    main()
