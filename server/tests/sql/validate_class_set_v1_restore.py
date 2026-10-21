#!/usr/bin/env python3
"""Validate approved V1 restoration after migration 93 in isolated PostgreSQL.
Uses the same --work-dir / PG_BIN / PG_SHARE arguments as validate_class_set_upgrade.
"""
import os
from pathlib import Path
from validate_class_set_upgrade import run, ROOT

body = Path(os.environ.get('MIGRATION94_SQL', str(ROOT/'server/migrations/000000_init-94.sql'))).read_text()
run('v1-first-migration', 'BEGIN;\n'+body+'\nCOMMIT;')
run('v1-second-migration', 'BEGIN;\n'+body+'\nCOMMIT;')
expected=[]
def add(suit,pieces,rows):
    expected.extend((suit,pieces,*row) for row in rows)
for suit,attack,original8 in [(1,240,(19,34)),(2,200,(17,25)),(3,250,(17,15)),(4,480,(19,20)),(5,540,(19,16))]:
    add(suit,2,[(36 if suit in (2,3) else 34,0,2500)])
    add(suit,4,[(15 if suit in (2,3) else 31,0,attack)])
    add(suit,6,[(66 if suit in (2,3) else 23,0,500)])
    add(suit,8,[(original8[0],0,original8[1])])
add(5,2,[(1,0,300),(5,0,180),(25,1,-80)])
add(4,2,[(1,0,240),(5,0,240),(19,1,100)])
add(1,2,[(1,0,300),(9,0,180)])
add(3,2,[(3,0,180),(61,0,120)])
add(2,2,[(3,0,160),(5,0,120),(36,1,100)])
add(5,8,[(1000101,0,1800),(1000103,0,800)])
add(4,8,[(1000101,0,1600),(1000103,0,800),(1000110,0,600),(1000111,0,600),(1000112,0,600),(1000113,0,600)])
add(1,8,[(1000101,0,1800),(1000102,0,1800),(1000104,0,1500)])
add(3,8,[(1000106,0,1800),(1000107,0,1800),(1000110,0,800),(1000111,0,800),(66,0,300)])
add(2,8,[(1000108,0,2500),(1000105,0,1500),(1000113,0,1000)])
assert len(expected)==52
rows=','.join('('+','.join(map(str,row))+')' for row in expected)
run('v1-full-approved-matrix', '''CREATE TEMP VIEW expected_bonus AS SELECT * FROM (VALUES '''+rows+''') x(suit,pieces,attr,mode,value);
CREATE TEMP VIEW actual_bonus AS SELECT s.id suit,x.seq pieces,e.attr_id attr,e.mode,e.value
 FROM gamedata.ov_excesuit s JOIN gamedata.ov_excesuit_entry x ON x.row_no=s.row_no
 JOIN gamedata.ov_card c ON c.index=x.ref_id JOIN gamedata.ov_card_entry e ON e.row_no=c.row_no WHERE s.id BETWEEN 1 AND 5;
DO $$ BEGIN
 IF EXISTS((SELECT * FROM expected_bonus EXCEPT ALL SELECT * FROM actual_bonus)
 UNION ALL (SELECT * FROM actual_bonus EXCEPT ALL SELECT * FROM expected_bonus))
 THEN RAISE EXCEPTION 'approved V1 matrix differs'; END IF;
 IF (SELECT count(*) FROM gamedata.ov_card WHERE row_no>=930000)<>15 OR
 (SELECT count(*) FROM gamedata.ov_card_entry WHERE row_no>=930000)<>47
 THEN RAISE EXCEPTION 'private cards/entries differ'; END IF;
 IF EXISTS(SELECT FROM gamedata.ov_card_entry e JOIN gamedata.ov_card c ON c.row_no=e.row_no
 WHERE c.row_no>=930000 AND (e.op_type<>1 OR e.prob<>100 OR c.item_id<>0 OR c.index<>c.row_no))
 THEN RAISE EXCEPTION 'private entry semantics differ'; END IF;
 IF EXISTS((SELECT row FROM before_ov_card EXCEPT ALL SELECT to_jsonb(t) FROM gamedata.ov_card t WHERE row_no<930000)
 UNION ALL (SELECT to_jsonb(t) FROM gamedata.ov_card t WHERE row_no<930000 EXCEPT ALL SELECT row FROM before_ov_card)) OR
 EXISTS((SELECT row FROM before_ov_card_entry EXCEPT ALL SELECT to_jsonb(t) FROM gamedata.ov_card_entry t WHERE row_no<930000)
 UNION ALL (SELECT to_jsonb(t) FROM gamedata.ov_card_entry t WHERE row_no<930000 EXCEPT ALL SELECT row FROM before_ov_card_entry)) OR
 EXISTS((SELECT row FROM before_ov_excesuit EXCEPT ALL SELECT to_jsonb(t) FROM gamedata.ov_excesuit t)
 UNION ALL (SELECT to_jsonb(t) FROM gamedata.ov_excesuit t EXCEPT ALL SELECT row FROM before_ov_excesuit)) OR
 EXISTS((SELECT row FROM before_ov_excesuit_entry WHERE (row->>'row_no')::int NOT BETWEEN 0 AND 4 EXCEPT ALL SELECT to_jsonb(t) FROM gamedata.ov_excesuit_entry t WHERE row_no NOT BETWEEN 0 AND 4)
 UNION ALL (SELECT to_jsonb(t) FROM gamedata.ov_excesuit_entry t WHERE row_no NOT BETWEEN 0 AND 4 EXCEPT ALL SELECT row FROM before_ov_excesuit_entry WHERE (row->>'row_no')::int NOT BETWEEN 0 AND 4))
 THEN RAISE EXCEPTION 'shared cards or unrelated suits changed'; END IF;
END $$;''')
print('PASS migration 94: all 52 approved bonus rows, 15 private cards/47 private entries, replay and shared-data isolation')
run('v1-reject-bonus-collision', 'BEGIN; UPDATE gamedata.ov_card_entry SET value=1 WHERE row_no=940011 AND idx=1;\n'+body+'\nCOMMIT;', 'bonus conflict')
run('v1-reject-missing-two-piece', 'BEGIN; UPDATE gamedata.ov_excesuit_entry SET ref_id=1 WHERE row_no=0 AND seq=2;\n'+body+'\nCOMMIT;', 'expected private card missing')
run('v1-after-rollback-replay', 'BEGIN;\n'+body+'\nCOMMIT;')
