#!/usr/bin/env python3
"""Validate migration 95 using real, isolated PostgreSQL native-table fixtures.

Uses the same --work-dir / PG_BIN / PG_SHARE arguments as the 93/94 validators.
Importing the V1 validator first proves the complete native -> 93 -> 94 path.
No server socket is opened and no existing database is contacted.
"""
import json
import os
from pathlib import Path
from validate_class_set_v1_restore import run, ROOT
from validate_class_set_upgrade import work

body = Path(os.environ.get('MIGRATION95_SQL', str(ROOT / 'server/migrations/000000_init-95.sql'))).read_text()
names = ['ov_card', 'ov_card_entry', 'ov_excesuit', 'ov_excesuit_entry']
for name in names:
    run('thresholds-snapshot-' + name,
        f'CREATE TABLE before95_{name} AS SELECT to_jsonb(t) AS row FROM gamedata.{name} t;')


def unchanged(prefix):
    clauses = []
    for name in names:
        clauses.append(f'''EXISTS(
 (SELECT row FROM {prefix}_{name} EXCEPT ALL SELECT to_jsonb(t) FROM gamedata.{name} t)
 UNION ALL
 (SELECT to_jsonb(t) FROM gamedata.{name} t EXCEPT ALL SELECT row FROM {prefix}_{name}))''')
    return "DO $$ BEGIN IF " + ' OR '.join(clauses) + " THEN RAISE EXCEPTION 'migration was not atomic/idempotent'; END IF; END $$;"


def reject(label, mutation, error, snapshot='before95'):
    run(label, 'BEGIN;\n' + mutation + '\n' + body + '\nCOMMIT;', error)
    run(label + '-rollback', unchanged(snapshot))


# Put target collisions in the last class, proving rollback also removes all
# writes already attempted for earlier classes, not only the colliding row.
clone = '''INSERT INTO gamedata.ov_card
 SELECT (jsonb_populate_record(NULL::gamedata.ov_card,
 to_jsonb(c) || jsonb_build_object('row_no',950052,'index',950052,'item_id',0))).*
 FROM gamedata.ov_card c WHERE c.index=930052;'''
reject('thresholds-reject-source-value',
       'UPDATE gamedata.ov_card_entry SET value=1 WHERE row_no=940051 AND idx=1;',
       'source bonus conflict')
reject('thresholds-reject-source-extra',
       'INSERT INTO gamedata.ov_card_entry VALUES(940051,7,0,1,19,0,100,1);',
       'unexpected source card')
reject('thresholds-reject-missing-entry',
       'DELETE FROM gamedata.ov_excesuit_entry WHERE row_no=4 AND idx=0;',
       'unexpected entry shape')
reject('thresholds-reject-foreign-reference',
       'UPDATE gamedata.ov_excesuit_entry SET ref_id=13135 WHERE row_no=4 AND idx=0;',
       'unexpected entry shape')
reject('thresholds-reject-foreign-threshold',
       'UPDATE gamedata.ov_excesuit_entry SET seq=7 WHERE row_no=4 AND idx=1;',
       'unexpected entry shape')
reject('thresholds-reject-mixed-state', '''
 DELETE FROM gamedata.ov_excesuit_entry WHERE row_no=0 AND idx=0;
 UPDATE gamedata.ov_excesuit_entry SET ref_id=CASE idx WHEN 1 THEN 950011 WHEN 2 THEN 950012 ELSE ref_id END,
 seq=CASE idx WHEN 1 THEN 6 WHEN 2 THEN 8 ELSE 4 END WHERE row_no=0;''',
       'mixed migration states')
reject('thresholds-reject-card-identity', clone + '''
 INSERT INTO gamedata.ov_card
 SELECT (jsonb_populate_record(NULL::gamedata.ov_card,
 to_jsonb(c) || jsonb_build_object('row_no',950099))).*
 FROM gamedata.ov_card c WHERE c.index=950052;''',
       'private card identity conflict')
reject('thresholds-reject-shared-target', clone +
       'UPDATE gamedata.ov_excesuit_entry SET ref_id=950052 WHERE row_no=10 AND idx=0;',
       'private card reference conflict')
reject('thresholds-reject-card-metadata', clone +
       'UPDATE gamedata.ov_card SET item_id=123 WHERE row_no=950052;',
       'private card conflict')
reject('thresholds-reject-orphan-effects',
       'INSERT INTO gamedata.ov_card_entry VALUES(950052,0,0,1,31,0,100,540);',
       'orphan private effects')
reject('thresholds-reject-target-value', clone +
       'INSERT INTO gamedata.ov_card_entry VALUES(950052,0,0,1,31,0,100,541);',
       'private bonus conflict')
reject('thresholds-reject-target-extra', clone +
       'INSERT INTO gamedata.ov_card_entry VALUES(950052,7,0,1,19,0,100,1);',
       'private bonus conflict')

run('thresholds-first-migration', 'BEGIN;\n' + body + '\nCOMMIT;')
for name in names:
    run('thresholds-after-snapshot-' + name,
        f'CREATE TABLE after95_{name} AS SELECT to_jsonb(t) AS row FROM gamedata.{name} t;')
run('thresholds-second-migration', 'BEGIN;\n' + body + '\nCOMMIT;')
run('thresholds-identical-replay', unchanged('after95'))

expected = []
def add(suit, pieces, rows):
    expected.extend((suit, pieces, *row) for row in rows)
for suit, attack, native in [(1,240,(19,34)), (2,200,(17,25)), (3,250,(17,15)), (4,480,(19,20)), (5,540,(19,16))]:
    add(suit,4,[(66 if suit in (2,3) else 23,0,500)])
    add(suit,6,[(36 if suit in (2,3) else 34,0,2500),(native[0],0,native[1])])
    add(suit,8,[(15 if suit in (2,3) else 31,0,attack)])
add(5,6,[(1,0,300),(5,0,180),(25,1,-8)])
add(4,6,[(1,0,240),(5,0,240),(19,1,10)])
add(1,6,[(1,0,300),(9,0,180)])
add(3,6,[(3,0,180),(61,0,120)])
add(2,6,[(3,0,160),(5,0,120),(36,1,10)])
add(5,8,[(1000101,0,1800),(1000103,0,800)])
add(4,8,[(1000101,0,1600),(1000103,0,800),(1000110,0,600),(1000111,0,600),(1000112,0,600),(1000113,0,600)])
add(1,8,[(1000101,0,1800),(1000102,0,1800),(1000104,0,1500)])
add(3,8,[(1000106,0,1800),(1000107,0,1800),(1000110,0,800),(1000111,0,800),(66,0,300)])
add(2,8,[(1000108,0,2500),(1000105,0,1500),(1000113,0,1000)])
assert len(expected) == 52
rows = ','.join('(' + ','.join(map(str, row)) + ')' for row in expected)
run('thresholds-full-approved-matrix', '''
CREATE TEMP VIEW expected_threshold_bonus AS SELECT * FROM (VALUES ''' + rows + ''') x(suit,pieces,attr,mode,value);
CREATE TEMP VIEW actual_threshold_bonus AS SELECT s.id suit,x.seq pieces,e.attr_id attr,e.mode,e.value
 FROM gamedata.ov_excesuit s JOIN gamedata.ov_excesuit_entry x ON x.row_no=s.row_no
 JOIN gamedata.ov_card c ON c.index=x.ref_id JOIN gamedata.ov_card_entry e ON e.row_no=c.row_no WHERE s.id BETWEEN 1 AND 5;
DO $$ BEGIN
 IF EXISTS((SELECT * FROM expected_threshold_bonus EXCEPT ALL SELECT * FROM actual_threshold_bonus)
 UNION ALL (SELECT * FROM actual_threshold_bonus EXCEPT ALL SELECT * FROM expected_threshold_bonus))
 THEN RAISE EXCEPTION 'approved final 52-row threshold matrix differs'; END IF;
 IF (SELECT count(*) FROM gamedata.ov_card WHERE row_no>=950000)<>10 OR
    (SELECT count(*) FROM gamedata.ov_card_entry WHERE row_no>=950000)<>47 OR
    (SELECT count(*) FROM gamedata.ov_excesuit s JOIN gamedata.ov_excesuit_entry e ON e.row_no=s.row_no WHERE s.id BETWEEN 1 AND 5)<>15 OR
    EXISTS(SELECT FROM gamedata.ov_excesuit s JOIN gamedata.ov_excesuit_entry e ON e.row_no=s.row_no WHERE s.id BETWEEN 1 AND 5 AND e.seq NOT IN(4,6,8))
 THEN RAISE EXCEPTION 'final cards/entries/thresholds differ'; END IF;
 IF EXISTS(SELECT FROM gamedata.ov_card_entry e JOIN gamedata.ov_card c ON c.row_no=e.row_no
 WHERE c.row_no>=950000 AND (e.res0<>0 OR e.op_type<>1 OR e.prob<>100 OR c.item_id<>0 OR c.index<>c.row_no OR e.idx NOT BETWEEN 0 AND 7))
 THEN RAISE EXCEPTION 'private card semantics differ'; END IF;
 -- The unchanged production LEFT JOIN loader must see no missing/null effects.
 IF EXISTS(SELECT FROM gamedata.ov_excesuit s JOIN gamedata.ov_excesuit_entry x ON x.row_no=s.row_no
 LEFT JOIN gamedata.ov_card c ON c.index=x.ref_id LEFT JOIN gamedata.ov_card_entry e ON e.row_no=c.row_no
 WHERE s.id BETWEEN 1 AND 5 AND (c.row_no IS NULL OR e.row_no IS NULL OR e.attr_id IS NULL OR e.mode IS NULL OR e.prob IS NULL OR e.value IS NULL OR e.op_type IS NULL))
 THEN RAISE EXCEPTION 'native loader would encounter an incomplete card'; END IF;
 IF EXISTS((SELECT row FROM before95_ov_card EXCEPT ALL SELECT to_jsonb(t) FROM gamedata.ov_card t WHERE row_no<950000)
 UNION ALL (SELECT to_jsonb(t) FROM gamedata.ov_card t WHERE row_no<950000 EXCEPT ALL SELECT row FROM before95_ov_card)) OR
 EXISTS((SELECT row FROM before95_ov_card_entry EXCEPT ALL SELECT to_jsonb(t) FROM gamedata.ov_card_entry t WHERE row_no<950000)
 UNION ALL (SELECT to_jsonb(t) FROM gamedata.ov_card_entry t WHERE row_no<950000 EXCEPT ALL SELECT row FROM before95_ov_card_entry)) OR
 EXISTS((SELECT row FROM before95_ov_excesuit EXCEPT ALL SELECT to_jsonb(t) FROM gamedata.ov_excesuit t)
 UNION ALL (SELECT to_jsonb(t) FROM gamedata.ov_excesuit t EXCEPT ALL SELECT row FROM before95_ov_excesuit)) OR
 EXISTS((SELECT row FROM before95_ov_excesuit_entry WHERE (row->>'row_no')::int NOT BETWEEN 0 AND 4 EXCEPT ALL SELECT to_jsonb(t) FROM gamedata.ov_excesuit_entry t WHERE row_no NOT BETWEEN 0 AND 4)
 UNION ALL (SELECT to_jsonb(t) FROM gamedata.ov_excesuit_entry t WHERE row_no NOT BETWEEN 0 AND 4 EXCEPT ALL SELECT row FROM before95_ov_excesuit_entry WHERE (row->>'row_no')::int NOT BETWEEN 0 AND 4))
 THEN RAISE EXCEPTION 'migration 95 changed source/shared cards or unrelated sets'; END IF;
END $$;''')
reject('thresholds-reject-replay-value',
       'UPDATE gamedata.ov_card_entry SET value=1 WHERE row_no=950052 AND idx=1;',
       'private bonus conflict', 'after95')
reject('thresholds-reject-replay-extra',
       'INSERT INTO gamedata.ov_card_entry VALUES(950052,7,0,1,19,0,100,1);',
       'private bonus conflict', 'after95')
run('thresholds-post-rollback-replay', 'BEGIN;\n' + body + '\nCOMMIT;')
run('thresholds-final-exact-state', unchanged('after95'))
print('PASS migration 95: exact 52-row 4/6/8 matrix, 10 private cards/47 effects, native loader completeness, replay, source/shared isolation and atomic collision rollback')

# Export the actual migrated rows for runtime/tooltip acceptance tests.
fixture_path = work / 'class-set-thresholds.json'
run('thresholds-export-runtime-fixture', """COPY (SELECT jsonb_agg(jsonb_build_object(
 'suit_id',s.id,'seq',x.seq,'attr_id',e.attr_id,'mode',e.mode,'prob',e.prob,'value',e.value)
 ORDER BY s.row_no,x.seq,x.idx,e.idx)
 FROM gamedata.ov_excesuit s JOIN gamedata.ov_excesuit_entry x ON x.row_no=s.row_no
 JOIN gamedata.ov_card c ON c.index=x.ref_id JOIN gamedata.ov_card_entry e ON e.row_no=c.row_no
 WHERE s.id BETWEEN 1 AND 5) TO '""" + str(fixture_path).replace("'", "''") + "';")
assert len(json.loads(fixture_path.read_text())) == 52
print('Runtime fixture:', fixture_path)
