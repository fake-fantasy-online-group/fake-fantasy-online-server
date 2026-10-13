#!/usr/bin/env python3
"""Verify migration 59 in a newly created disposable PostgreSQL cluster only."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
from validate_daoist_postgres import concatenate_sources, write_single_user_commands

ROOT = Path(__file__).resolve().parents[3]
p = argparse.ArgumentParser()
p.add_argument('--work-dir', type=Path, required=True)
a = p.parse_args()
w = a.work_dir.resolve()
w.mkdir(parents=True, exist_ok=False)
bins = Path(os.environ['PG_BIN'])
data = w / 'data'
subprocess.run([str(bins / 'initdb'), '-D', str(data), '--no-locale', '-E', 'UTF8', '-U', 'validator', '-A', 'trust'], check=True, stdout=(w / 'init.log').open('w'), stderr=subprocess.STDOUT)


def run(label, sql=None, paths=None, expected_error=None):
    raw, inp, log = [w / (label + suffix) for suffix in ('.source.sql', '.sql', '.log')]
    with raw.open('wb') as f:
        if paths:
            concatenate_sources(paths, f)
        else:
            f.write(sql.encode())
    with raw.open() as source, inp.open('w') as target:
        write_single_user_commands(source, target)
    raw.unlink()
    with inp.open() as source, log.open('w') as out:
        result = subprocess.run([str(bins / 'postgres'), '--single', '-j', '-D', str(data), 'postgres'], stdin=source, stdout=out, stderr=subprocess.STDOUT)
    errors = [x.strip() for x in log.read_text().splitlines() if re.search(r'\b(?:ERROR|FATAL|PANIC):', x)]
    if expected_error:
        assert any(expected_error in e for e in errors), (label, errors[:3])
    else:
        assert result.returncode == 0 and not errors, (label, errors[:3])
    inp.unlink()
    print('PASS', label, flush=True)


run('fresh-through-58', paths=[ROOT / f'server/migrations/000000_init-{i}.sql' for i in range(59)])
patch = ROOT / 'server/migrations/000000_init-59.sql'
body = patch.read_text()
rows = json.loads(body.split('$endgame_items$')[1])
assert len(rows) == 32
ids = ','.join(str(r['id']) for r in rows)
# Real previously created card snapshots deliberately differ from today's base
# defaults. Upgrade must never rewrite their ID, UID, ownership or effect state.
run('seed-existing-owned-items', sql='''
INSERT INTO accounts(id,username) VALUES(930590001,'before_item_config');
INSERT INTO characters(id,account_id,slot,name,race,map_id,pos_x,pos_y)
 VALUES(930590001,930590001,0,'before_item_config',1,7,10,20);
INSERT INTO item_instances(uid,item_id,instance_kind,created_source)
 VALUES(930590001,1900028001,2,'before_item_config');
INSERT INTO card_instances(uid,effect_count,ops,attrs,modes,probabilities,effect_values)
 VALUES(930590001,1,ARRAY[1,0,0,0,0,0,0,0],ARRAY[1,0,0,0,0,0,0,0],
 ARRAY[0,0,0,0,0,0,0,0],ARRAY[100,0,0,0,0,0,0,0],ARRAY[37,0,0,0,0,0,0,0]);
INSERT INTO character_items(char_id,slot,uid,item_id,count)
 VALUES(930590001,100,930590001,1900028001,1),
 (930590001,101,0,1900028101,7),(930590001,102,0,1900028201,19);
INSERT INTO character_endgame_state VALUES(930590001,3,'{"keys":7}');
''')
tables = ['gamedata.ov_item', 'gamedata.ov_desc', 'gamedata.ov_card', 'gamedata.ov_card_entry', 'game_items', 'gamedata.ov_arm', 'game_equipment', 'accounts', 'characters', 'item_instances', 'card_instances', 'character_endgame_state', 'character_items']
for i, table in enumerate(tables):
    run('snapshot-' + str(i), sql=f'CREATE TABLE before_items_{i} AS SELECT to_jsonb(t) AS row FROM {table} t;')
# A collision at the last item proves earlier inserts are rolled back too.
for label, conflict in [
 ('canonical', "INSERT INTO game_items(id,name,raw) VALUES(1900028203,'collision','\\x');"),
 ('item-index', "INSERT INTO gamedata.ov_item(row_no,index,name) VALUES(1900028999,1900028203,'collision');"),
 ('item-rowno', "INSERT INTO gamedata.ov_item(row_no,index,name) VALUES(1900028203,1900028999,'collision');"),
 ('descriptor', "INSERT INTO gamedata.ov_desc(row_no,index,desc_) VALUES(1900028203,1900028203,'collision');"),
 ('equipment', "INSERT INTO game_equipment(id,name,raw) VALUES(1900028203,'collision','\\x');"),
 ('rune-card', "INSERT INTO gamedata.ov_card(row_no,index,item_id) VALUES(1900028999,1900028028,1900028028);"),
 ('rune-effect', "INSERT INTO gamedata.ov_card_entry(row_no,idx,value) VALUES(1900028028,0,999);"),
]:
    run('collision-' + label, sql='BEGIN;\n' + conflict + '\n' + body + '\nCOMMIT;\n', expected_error='colli')
    run('collision-atomic-' + label, sql=f"DO $$ BEGIN IF EXISTS(SELECT FROM game_items WHERE id IN ({ids})) THEN RAISE EXCEPTION 'partial base rows survived failed migration'; END IF; END $$;")
run('migration-59-first', paths=[patch])
run('migration-59-repeat', paths=[patch])
for i, table in enumerate(tables):
    key = 'index' if table in ['gamedata.ov_item', 'gamedata.ov_desc', 'gamedata.ov_card'] else 'row_no' if table == 'gamedata.ov_card_entry' else 'id' if table == 'game_items' else None
    where = f' WHERE {key} NOT IN ({ids})' if key else ''
    run('preserve-' + str(i), sql=f"DO $$ BEGIN IF EXISTS((SELECT row FROM before_items_{i} EXCEPT ALL SELECT to_jsonb(t) FROM {table} t{where}) UNION ALL (SELECT to_jsonb(t) FROM {table} t{where} EXCEPT ALL SELECT row FROM before_items_{i})) THEN RAISE EXCEPTION 'preexisting rows modified in {table}'; END IF; END $$;")
# Execute the application's actual search CTE, with name, id and level filters.
source = (ROOT / 'server/cmd/dispatch/catalog.go').read_text()
cte = source.split('const catalogCTE = `', 1)[1].split('`', 1)[0]
for label, condition, count in [('all32', f'id IN ({ids})', 32), ('runes28', "name LIKE '星篆·%'", 28), ('key-name', "name='洞天符钥'", 1), ('key-id', 'id=1900028101', 1), ('key-keyword', "id=1900028101 AND (name ILIKE '%钥匙%' OR description ILIKE '%钥匙%')", 1), ('level60', "name LIKE '星篆·%' AND level=60", 1)]:
    run('search-' + label, sql="DO $$ DECLARE n integer; BEGIN " + cte + f" SELECT count(*) INTO n FROM catalog WHERE {condition}; IF n <> {count} THEN RAISE EXCEPTION 'wrong search count %',n; END IF; END $$;")
run('base-card-semantics', sql=f'''DO $$ BEGIN
 IF (SELECT count(*) FROM gamedata.ov_card WHERE index BETWEEN 1900028001 AND 1900028028 AND index=item_id) <> 28 THEN RAISE EXCEPTION 'rune identities'; END IF;
 IF (SELECT count(*) FROM gamedata.ov_card_entry WHERE row_no BETWEEN 1900028001 AND 1900028028 AND idx=0 AND op_type=1 AND prob=100) <> 28 THEN RAISE EXCEPTION 'rune effects'; END IF;
 IF NOT EXISTS(SELECT FROM card_instances WHERE uid=930590001 AND effect_values[1]=37) THEN RAISE EXCEPTION 'legacy snapshot rewritten'; END IF;
 IF (SELECT count(*) FROM gamedata.ov_item WHERE index IN ({ids}) AND can_pile=1 AND can_deal=0 AND can_mail=0 AND level=0 AND weight=0) <> 4 THEN RAISE EXCEPTION 'materials/key flags'; END IF;
END $$;''')
print('ENDGAME_ITEM_DEFINITIONS_POSTGRES_PASS', flush=True)
print('Read-only Go loader test: FANTASY_ENDGAME_ITEMS_PG_DATA=' + str(data), flush=True)
