#!/usr/bin/env python3
"""Run migration 93 twice against isolated native-table PostgreSQL fixtures.
Socket-free single-user mode; never connects to an existing database.
PG_BIN and PG_SHARE must refer to an already available trusted PostgreSQL install.
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
a = p.parse_args()
work = a.work_dir.resolve()
work.mkdir(parents=True, exist_ok=False)
data = work / 'data'
bins = Path(os.environ['PG_BIN'])
subprocess.run([str(bins/'initdb'), '-D', str(data), '-L', os.environ['PG_SHARE'], '--no-locale', '-E', 'UTF8'], check=True, stdout=(work/'init.log').open('w'), stderr=subprocess.STDOUT)

def run(label, sql, expected_error=None):
    inp, log = work/(label+'.sql'), work/(label+'.log')
    with inp.open('w') as f:
        write_single_user_commands(io.StringIO(sql), f)
    with inp.open() as source, log.open('w') as out:
        result = subprocess.run([str(bins/'postgres'), '--single', '-j', '-D', str(data), 'postgres'], stdin=source, stdout=out, stderr=subprocess.STDOUT)
    errors = [x for x in log.read_text().splitlines() if re.search(r'\b(?:ERROR|FATAL|PANIC):', x)]
    if expected_error:
        assert any(expected_error in x for x in errors), (label, errors[:3])
    else:
        assert result.returncode == 0 and not errors, (label, errors[:3])
    print('PASS', label, flush=True)

names = ['ov_card','ov_card_entry','ov_excesuit','ov_excesuit_entry']
schema = (ROOT/'server/migrations/000000_init-0.sql').read_text()
source = ''.join((ROOT/f'server/migrations/000000_init-{i}.sql').read_text() for i in range(26,33))
fixture = ['CREATE SCHEMA gamedata;']
for name in names:
    fixture.append(re.search(r'CREATE TABLE IF NOT EXISTS gamedata\.'+name+r' \([^;]+;',schema).group())
    fixture.extend(m.group() for m in re.finditer(r'INSERT INTO "gamedata"\."'+name+r'"[^;]+;',source))
run('native-fixture', '\n'.join(fixture))
for name in names:
    run('snapshot-'+name, f'CREATE TABLE before_{name} AS SELECT to_jsonb(t) AS row FROM gamedata.{name} t;')
body = (ROOT/'server/migrations/000000_init-93.sql').read_text()
run('reject-missing-source', 'BEGIN; DELETE FROM gamedata.ov_excesuit_entry WHERE row_no=0 AND idx=2;\n'+body+'\nCOMMIT;', 'bonus reference changed')
run('first-migration', 'BEGIN;\n'+body+'\nCOMMIT;')
run('second-migration', 'BEGIN;\n'+body+'\nCOMMIT;')
run('verify-isolation-and-values', """DO $$ BEGIN
 IF (SELECT count(*) FROM gamedata.ov_card WHERE row_no>=930000)<>10 OR
    (SELECT count(*) FROM gamedata.ov_card_entry WHERE row_no>=930000)<>10 OR
    EXISTS(SELECT FROM gamedata.ov_card WHERE row_no>=930000 AND (item_id<>0 OR index<>row_no))
 THEN RAISE EXCEPTION 'private bonus card count/item identity changed'; END IF;
 IF EXISTS((SELECT row FROM before_ov_card EXCEPT ALL SELECT to_jsonb(t) FROM gamedata.ov_card t WHERE row_no<930000)
 UNION ALL (SELECT to_jsonb(t) FROM gamedata.ov_card t WHERE row_no<930000 EXCEPT ALL SELECT row FROM before_ov_card)) OR
 EXISTS((SELECT row FROM before_ov_card_entry EXCEPT ALL SELECT to_jsonb(t) FROM gamedata.ov_card_entry t WHERE row_no<930000)
 UNION ALL (SELECT to_jsonb(t) FROM gamedata.ov_card_entry t WHERE row_no<930000 EXCEPT ALL SELECT row FROM before_ov_card_entry)) OR
 EXISTS((SELECT row FROM before_ov_excesuit EXCEPT ALL SELECT to_jsonb(t) FROM gamedata.ov_excesuit t)
 UNION ALL (SELECT to_jsonb(t) FROM gamedata.ov_excesuit t EXCEPT ALL SELECT row FROM before_ov_excesuit)) OR
 EXISTS((SELECT row FROM before_ov_excesuit_entry WHERE (row->>'row_no')::int NOT BETWEEN 0 AND 4 EXCEPT ALL SELECT to_jsonb(t) FROM gamedata.ov_excesuit_entry t WHERE row_no NOT BETWEEN 0 AND 4)
 UNION ALL (SELECT to_jsonb(t) FROM gamedata.ov_excesuit_entry t WHERE row_no NOT BETWEEN 0 AND 4 EXCEPT ALL SELECT row FROM before_ov_excesuit_entry WHERE (row->>'row_no')::int NOT BETWEEN 0 AND 4))
 THEN RAISE EXCEPTION 'unrelated native rows changed'; END IF;
 IF (SELECT count(*) FROM gamedata.ov_excesuit s JOIN gamedata.ov_excesuit_entry x ON x.row_no=s.row_no
 JOIN gamedata.ov_card c ON c.index=x.ref_id JOIN gamedata.ov_card_entry e ON e.row_no=c.row_no
 JOIN (VALUES
 (1,2,34,2500),(1,4,31,240),(1,6,23,500),(1,8,19,34),
 (2,2,36,2500),(2,4,15,200),(2,6,66,500),(2,8,17,25),
 (3,2,36,2500),(3,4,15,250),(3,6,66,500),(3,8,17,15),
 (4,2,34,2500),(4,4,31,480),(4,6,23,500),(4,8,19,20),
 (5,2,34,2500),(5,4,31,540),(5,6,23,500),(5,8,19,16)
 ) expected(id,pieces,attr,value) ON expected.id=s.id AND expected.pieces=x.seq
 AND expected.attr=e.attr_id AND expected.value=e.value)<>20
 THEN RAISE EXCEPTION 'new 2/4/6/8 bonuses differ'; END IF;
END $$;""")
run('reject-private-card-collision', 'BEGIN; UPDATE gamedata.ov_card_entry SET value=1 WHERE row_no=930010;\n'+body+'\nCOMMIT;', 'effects changed')
print('PASS class-set migration 93: exact five-set scope, 2/4/6/8 reordering, x10 flat stats, replay, collision guards and transaction rollback')
