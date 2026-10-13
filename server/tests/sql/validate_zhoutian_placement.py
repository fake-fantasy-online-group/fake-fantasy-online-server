#!/usr/bin/env python3
"""Socket-free PostgreSQL migration75 checks against original native map rows.
Creates its own disposable cluster; never opens a listener or a production DB.
"""
import argparse
import io
import os
from pathlib import Path
import re
import subprocess
from validate_daoist_postgres import write_single_user_commands

ROOT = Path(__file__).resolve().parents[3]
p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--work-dir', type=Path, required=True)
a = p.parse_args()
w = a.work_dir.resolve()
w.mkdir(parents=True, exist_ok=False)
data = w/'data'
bins = Path(os.environ['PG_BIN'])
init = [str(bins/'initdb'), '-D', str(data), '--no-locale', '--encoding=UTF8', '--auth=trust']
if os.environ.get('PG_SHARE'):
    init += ['-L', os.environ['PG_SHARE']]
subprocess.run(init, stdout=(w/'initdb.log').open('w'), stderr=subprocess.STDOUT, check=True)

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

original = (ROOT/'server/migrations/000000_init-0.sql').read_text()
seed = []
for name in ['map_defs', 'map_npcs', 'map_teleports']:
    match = re.search(r'CREATE TABLE IF NOT EXISTS '+name+r' \(.*?\n\);', original, re.S)
    assert match, name
    seed.append(match.group())
seed.append('ALTER TABLE map_teleports ADD COLUMN poly INTEGER[];')
# Keep every native row, including unrelated maps, so conservation is tested.
for name in ['map_defs', 'map_npcs', 'map_teleports']:
    blocks = re.findall(r'INSERT INTO "'+name+r'" .*?ON CONFLICT.*?;', original, re.S)
    assert blocks, name
    seed.extend(blocks)
seed.extend(re.findall(r'UPDATE map_teleports SET (?:poly|script)=.*?;', original, re.S))
seed.append('CREATE TABLE before_npcs AS TABLE map_npcs; CREATE TABLE before_maps AS TABLE map_defs; CREATE TABLE before_doors AS TABLE map_teleports;')
run('native-source-seed', '\n'.join(seed))
body = (ROOT/'server/migrations/000000_init-75.sql').read_text()
empty = """DO $$ BEGIN
 IF EXISTS(SELECT FROM map_npcs WHERE script='Nzhoutianding') OR
 NOT EXISTS(SELECT FROM map_teleports WHERE map_file='qw0506' AND proc_id=1 AND to_x=8898 AND to_y=6707 AND poly IS NULL)
 THEN RAISE EXCEPTION 'fixture was changed before import'; END IF;
END $$;"""
run('require-original-state', empty)
run('atomic-rollback', 'BEGIN;\n'+body+"\nDO $$ BEGIN RAISE EXCEPTION 'deliberate rollback'; END $$; COMMIT;", 'deliberate rollback')
run('rollback-preserved-original', empty)
for label, change, error in [
    ('missing-map', 'DELETE FROM map_defs WHERE id=506;', 'original Longcheng and chamber maps'),
    ('missing-zuoci', "DELETE FROM map_npcs WHERE map_file='qw0007' AND npc_id=73;", 'original Zuoci placement'),
    ('changed-door', "UPDATE map_teleports SET map_to=505 WHERE map_file='qw0007' AND proc_id=5;", 'original reversible chamber doorway'),
    ('occupied-slot', "INSERT INTO map_npcs(map_file,npc_id,sprite,name,script,x,y) VALUES('qw0506',1,620,'Other','Nother',10,10);", 'NPC slot is already occupied'),
]:
    run('reject-'+label, 'BEGIN;\n'+change+'\n'+body+'\nCOMMIT;', error)
run('guards-preserved-original', empty)
run('first-import', 'BEGIN;\n'+body+'\nCOMMIT;')
run('repeat-import', 'BEGIN;\n'+body+'\nCOMMIT;')
run('exact-native-delta', """DO $$ BEGIN
 IF (SELECT count(*) FROM map_npcs WHERE script='Nzhoutianding')<>1 OR
 NOT EXISTS(SELECT FROM map_npcs WHERE map_file='qw0506' AND npc_id=1 AND sprite=903 AND name='周天鼎' AND script='Nzhoutianding' AND x=620 AND y=430 AND dir=67 AND sell_list=0 AND trans_list=0 AND NOT hide) OR
 NOT EXISTS(SELECT FROM map_teleports WHERE map_file='qw0007' AND proc_id=5 AND map_to=506 AND to_x=498 AND to_y=541 AND back_x=9330 AND back_y=6099 AND poly=ARRAY[9250,5980,9385,6060,9250,6140,9115,6060]) OR
 NOT EXISTS(SELECT FROM map_teleports WHERE map_file='qw0506' AND proc_id=1 AND map_to=7 AND to_x=9330 AND to_y=6099 AND back_x=498 AND back_y=541 AND poly=ARRAY[583,580,630,620,675,591,642,550])
 THEN RAISE EXCEPTION 'wrong chamber placement or return doorway'; END IF;
 IF EXISTS((SELECT * FROM before_maps EXCEPT ALL SELECT * FROM map_defs) UNION ALL (SELECT * FROM map_defs EXCEPT ALL SELECT * FROM before_maps)) THEN RAISE EXCEPTION 'original map definitions changed'; END IF;
 IF EXISTS((SELECT * FROM before_npcs EXCEPT ALL SELECT * FROM map_npcs WHERE NOT(map_file='qw0506' AND npc_id=1)) UNION ALL (SELECT * FROM map_npcs WHERE NOT(map_file='qw0506' AND npc_id=1) EXCEPT ALL SELECT * FROM before_npcs)) THEN RAISE EXCEPTION 'original NPCs changed'; END IF;
 IF EXISTS((SELECT * FROM before_doors WHERE NOT((map_file='qw0007' AND proc_id=5) OR (map_file='qw0506' AND proc_id=1)) EXCEPT ALL SELECT * FROM map_teleports WHERE NOT((map_file='qw0007' AND proc_id=5) OR (map_file='qw0506' AND proc_id=1))) UNION ALL (SELECT * FROM map_teleports WHERE NOT((map_file='qw0007' AND proc_id=5) OR (map_file='qw0506' AND proc_id=1)) EXCEPT ALL SELECT * FROM before_doors WHERE NOT((map_file='qw0007' AND proc_id=5) OR (map_file='qw0506' AND proc_id=1)))) THEN RAISE EXCEPTION 'unrelated doorway changed'; END IF;
 IF EXISTS(SELECT FROM before_doors b JOIN map_teleports t USING(map_file,proc_id) WHERE (to_jsonb(b)-ARRAY['to_x','to_y','back_x','back_y','poly']) IS DISTINCT FROM (to_jsonb(t)-ARRAY['to_x','to_y','back_x','back_y','poly'])) THEN RAISE EXCEPTION 'door semantics changed'; END IF;
END $$;""")
run('replay-preserves-npc-admin-placement', "BEGIN; UPDATE map_npcs SET x=610,y=420 WHERE script='Nzhoutianding';\n"+body+"\nDO $$ BEGIN IF NOT EXISTS(SELECT FROM map_npcs WHERE script='Nzhoutianding' AND x=610 AND y=420) THEN RAISE EXCEPTION 'replay overwrote NPC placement'; END IF; END $$; ROLLBACK;")
print('ZHOUTIAN_PLACEMENT_POSTGRES_PASS', flush=True)
