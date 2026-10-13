#!/usr/bin/env python3
"""Verify migration65 in a new disposable PostgreSQL cluster, without sockets.

Optionally copy an idle disposable fixture through migration64 with --seed-dir.
The input cluster is never opened or changed by this script.
"""
import argparse
import os
from pathlib import Path
import re
import shutil
import subprocess

from validate_daoist_postgres import concatenate_sources, write_single_user_commands

ROOT = Path(__file__).resolve().parents[3]
parser = argparse.ArgumentParser()
parser.add_argument('--work-dir', required=True, type=Path)
parser.add_argument('--seed-dir', type=Path)
args = parser.parse_args()
work = args.work_dir.resolve()
work.mkdir(parents=True, exist_ok=False)
bins = Path(os.environ['PG_BIN'])
data = work / 'data'
if args.seed_dir:
    seed = args.seed_dir.resolve()
    if (seed / 'postmaster.pid').exists():
        raise SystemExit('refusing to copy a running PostgreSQL cluster')
    shutil.copytree(seed, data)
else:
    subprocess.run([str(bins / 'initdb'), '-D', str(data), '--no-locale', '-E', 'UTF8',
                    '-U', 'validator', '-A', 'trust'], check=True,
                   stdout=(work / 'init.log').open('w'), stderr=subprocess.STDOUT)


def run(label, sql=None, paths=None, expected_error=None):
    raw, commands, log = [work / (label + suffix) for suffix in ('.source.sql', '.sql', '.log')]
    with raw.open('wb') as out:
        if paths:
            # The production init command executes every fragment in one
            # transaction; earlier migrations use ON COMMIT DROP temp tables.
            out.write(b'BEGIN;\n')
            concatenate_sources(paths, out)
            out.write(b'\nCOMMIT;\n')
        else:
            out.write(sql.encode())
    with raw.open() as source, commands.open('w') as out:
        write_single_user_commands(source, out)
    raw.unlink()
    with commands.open() as source, log.open('w') as out:
        result = subprocess.run([str(bins / 'postgres'), '--single', '-j', '-D', str(data),
                                 'postgres'], stdin=source, stdout=out, stderr=subprocess.STDOUT)
    errors = [line.strip() for line in log.read_text().splitlines()
              if re.search(r'\b(?:ERROR|FATAL|PANIC):', line)]
    if expected_error:
        assert any(expected_error in error for error in errors), (label, errors[:3])
    else:
        assert result.returncode == 0 and not errors, (label, errors[:3])
    commands.unlink()
    print('PASS', label, flush=True)


if not args.seed_dir:
    run('fresh-through64', paths=[ROOT / f'server/migrations/000000_init-{i}.sql' for i in range(65)])
body = (ROOT / 'server/migrations/000000_init-65.sql').read_text()
run('require-unmigrated-copy', sql="""DO $$ BEGIN
 IF EXISTS(SELECT FROM pg_attribute WHERE attrelid='map_teleports'::regclass
           AND attname='transition_owner' AND NOT attisdropped)
 THEN RAISE EXCEPTION 'fixture already migrated'; END IF;
END $$;""")
run('failed-transaction-rolls-back', sql='BEGIN;\n'+body+"""
DO $$ BEGIN RAISE EXCEPTION 'deliberate ownership migration failure'; END $$;
COMMIT;
""", expected_error='deliberate ownership migration failure')
run('failure-left-schema-unchanged', sql="""DO $$ BEGIN
 IF EXISTS(SELECT FROM pg_attribute WHERE attrelid='map_teleports'::regclass
           AND attname='transition_owner' AND NOT attisdropped)
 THEN RAISE EXCEPTION 'failed migration persisted owner'; END IF;
END $$;""")
run('changed-key-unknown-and-script-conservative', sql="""BEGIN;
UPDATE map_teleports SET to_x=to_x+1 WHERE map_file='qw0919' AND proc_id=1;
UPDATE map_teleports SET poly=ARRAY[1,1,2,1,2,2] WHERE map_file='qw0919' AND proc_id=2;
UPDATE map_teleports SET script='admin_script' WHERE map_file='qw0007' AND proc_id=9;
INSERT INTO map_teleports(map_file,proc_id,map_to,to_x,to_y,poly)
 VALUES('qw0919',77777,920,10789,5108,ARRAY[1,1,2,1,2,2]);
"""+body+"""
DO $$ BEGIN
 IF (SELECT count(*) FROM map_teleports WHERE transition_owner='client')<>454
 THEN RAISE EXCEPTION 'changed routes classified as client'; END IF;
 IF EXISTS(SELECT FROM map_teleports WHERE transition_owner<>'server' AND
 ((map_file='qw0919' AND proc_id IN(1,2,77777)) OR (map_file='qw0007' AND proc_id=9)))
 THEN RAISE EXCEPTION 'changed/unknown route not server'; END IF;
END $$;
ROLLBACK;
""")
run('first-migration-no-route-mutations', sql="""BEGIN;
CREATE TEMP TABLE before_portals AS SELECT to_jsonb(t) AS row FROM map_teleports t;
"""+body+"""
DO $$ BEGIN
 IF EXISTS((SELECT row FROM before_portals EXCEPT ALL SELECT to_jsonb(t)-'transition_owner' FROM map_teleports t)
 UNION ALL (SELECT to_jsonb(t)-'transition_owner' FROM map_teleports t EXCEPT ALL SELECT row FROM before_portals))
 THEN RAISE EXCEPTION 'original route fields changed'; END IF;
 IF (SELECT count(*) FROM map_teleports WHERE transition_owner='client')<>457 OR
    (SELECT count(*) FROM map_teleports WHERE transition_owner='server')<>542
 THEN RAISE EXCEPTION 'owner coverage'; END IF;
 IF EXISTS(SELECT FROM map_teleports WHERE (proc_id=90001 OR script<>'') AND transition_owner<>'server')
 THEN RAISE EXCEPTION 'server/script routes changed owner'; END IF;
END $$;
COMMIT;
""")
run('replay-preserves-explicit-admin-choices', sql="""BEGIN;
UPDATE map_teleports SET transition_owner='server',to_x=to_x+99 WHERE map_file='qw0919' AND proc_id=2;
INSERT INTO map_teleports(map_file,proc_id,map_to,to_x,to_y,poly)
 VALUES('qw0919',77777,920,10789,5108,ARRAY[1,1,2,1,2,2]);
DO $$ BEGIN
 IF (SELECT transition_owner FROM map_teleports WHERE map_file='qw0919' AND proc_id=77777)<>'server'
 THEN RAISE EXCEPTION 'new row not conservative'; END IF;
END $$;
UPDATE map_teleports SET transition_owner='client' WHERE map_file='qw0919' AND proc_id=77777;
"""+body+"""
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM map_teleports WHERE map_file='qw0919' AND proc_id=2 AND transition_owner='server' AND to_x=10888)
 OR NOT EXISTS(SELECT FROM map_teleports WHERE map_file='qw0919' AND proc_id=77777 AND transition_owner='client')
 THEN RAISE EXCEPTION 'replay overwrote administrator choices'; END IF;
END $$;
ROLLBACK;
""")
run('reject-invalid-owner', sql="BEGIN; UPDATE map_teleports SET transition_owner='guess' WHERE map_file='qw0919'; COMMIT;",
    expected_error='check constraint')
run('repeat-clean', sql='BEGIN;\n'+body+'\nCOMMIT;')
print('PORTAL_OWNERSHIP_POSTGRES_PASS', flush=True)
print('Read-only loader: FANTASY_PORTAL_OWNER_PG_DATA='+str(data), flush=True)
