-- Restore the approved V1 additions: 2-piece flat additions x10; percentage additions retain V1 values and former
-- 4-piece combat bonuses at the new 8-piece tier. Run after 93, not instead.
-- Shared cards are never changed; the five 8-piece rows get private clones.
DO $class_set_v1$
DECLARE r RECORD; suit_row INTEGER; src_card JSONB; desired JSONB; actual JSONB;
 card_id INTEGER; current_ref INTEGER; original_attr INTEGER; original_value INTEGER;
BEGIN
 FOR r IN SELECT * FROM (VALUES (1,19,34),(2,17,25),(3,17,15),(4,19,20),(5,19,16)) AS v(suit_id,attr,value) LOOP
  SELECT row_no INTO STRICT suit_row FROM gamedata.ov_excesuit WHERE id=r.suit_id;
  SELECT ref_id INTO STRICT current_ref FROM gamedata.ov_excesuit_entry WHERE row_no=suit_row AND idx=1 AND seq=8;
  SELECT to_jsonb(c) INTO STRICT src_card FROM gamedata.ov_card c WHERE c.index=current_ref;
  IF NOT EXISTS(SELECT FROM gamedata.ov_card_entry WHERE row_no=(src_card->>'row_no')::int AND idx=0 AND op_type=1 AND attr_id=r.attr AND mode=0 AND prob=100 AND value=r.value)
  THEN RAISE EXCEPTION 'set V1 original 8-piece bonus changed for %',r.suit_id; END IF;
  card_id:=940000+r.suit_id*10+1;
  IF current_ref<>card_id AND (SELECT count(*) FROM gamedata.ov_card_entry WHERE row_no=(src_card->>'row_no')::int)<>1 THEN RAISE EXCEPTION 'set V1 shared native bonus has unexpected rows'; END IF;
  desired:=src_card || jsonb_build_object('row_no',card_id,'index',card_id,'item_id',0);
  IF (SELECT count(*) FROM gamedata.ov_card WHERE row_no=card_id OR index=card_id)>1 THEN RAISE EXCEPTION 'set V1 private identity conflict'; END IF;
  SELECT to_jsonb(c) INTO actual FROM gamedata.ov_card c WHERE row_no=card_id OR index=card_id;
  IF NOT FOUND THEN INSERT INTO gamedata.ov_card SELECT (jsonb_populate_record(NULL::gamedata.ov_card,desired)).*;
  ELSIF actual IS DISTINCT FROM desired THEN RAISE EXCEPTION 'set V1 private card conflict'; END IF;
  INSERT INTO gamedata.ov_card_entry(row_no,idx,res0,op_type,attr_id,mode,prob,value)
   VALUES(card_id,0,0,1,r.attr,0,100,r.value) ON CONFLICT(row_no,idx) DO NOTHING;
  IF NOT EXISTS(SELECT FROM gamedata.ov_card_entry WHERE row_no=card_id AND idx=0 AND res0=0 AND op_type=1 AND attr_id=r.attr AND mode=0 AND prob=100 AND value=r.value)
  THEN RAISE EXCEPTION 'set V1 native bonus conflict'; END IF;
  UPDATE gamedata.ov_excesuit_entry SET ref_id=card_id WHERE row_no=suit_row AND idx=1 AND seq=8;
 END LOOP;
 FOR r IN SELECT * FROM (VALUES
 (1,2,1,1,0,300),
 (1,2,2,9,0,180),
 (2,2,1,3,0,160),
 (2,2,2,5,0,120),
 (2,2,3,36,1,10),
 (3,2,1,3,0,180),
 (3,2,2,61,0,120),
 (4,2,1,1,0,240),
 (4,2,2,5,0,240),
 (4,2,3,19,1,10),
 (5,2,1,1,0,300),
 (5,2,2,5,0,180),
 (5,2,3,25,1,-8),
 (1,8,1,1000101,0,1800),
 (1,8,2,1000102,0,1800),
 (1,8,3,1000104,0,1500),
 (2,8,1,1000108,0,2500),
 (2,8,2,1000105,0,1500),
 (2,8,3,1000113,0,1000),
 (3,8,1,1000106,0,1800),
 (3,8,2,1000107,0,1800),
 (3,8,3,1000110,0,800),
 (3,8,4,1000111,0,800),
 (3,8,5,66,0,300),
 (4,8,1,1000101,0,1600),
 (4,8,2,1000103,0,800),
 (4,8,3,1000110,0,600),
 (4,8,4,1000111,0,600),
 (4,8,5,1000112,0,600),
 (4,8,6,1000113,0,600),
 (5,8,1,1000101,0,1800),
 (5,8,2,1000103,0,800)
 ) AS v(suit_id,pieces,entry_index,attr,mode,value) LOOP
  card_id:=CASE WHEN r.pieces=2 THEN 930000+r.suit_id*10 ELSE 940000+r.suit_id*10+1 END;
  IF NOT EXISTS(SELECT FROM gamedata.ov_excesuit s JOIN gamedata.ov_excesuit_entry e ON e.row_no=s.row_no
    WHERE s.id=r.suit_id AND e.seq=r.pieces AND e.ref_id=card_id)
   OR NOT EXISTS(SELECT FROM gamedata.ov_card WHERE row_no=card_id AND index=card_id AND item_id=0)
  THEN RAISE EXCEPTION 'set V1 expected private card missing'; END IF;
  INSERT INTO gamedata.ov_card_entry(row_no,idx,res0,op_type,attr_id,mode,prob,value)
   VALUES(card_id,r.entry_index,0,1,r.attr,r.mode,100,r.value) ON CONFLICT(row_no,idx) DO NOTHING;
  IF NOT EXISTS(SELECT FROM gamedata.ov_card_entry WHERE row_no=card_id AND idx=r.entry_index
   AND res0=0 AND op_type=1 AND attr_id=r.attr AND mode=r.mode AND prob=100 AND value=r.value)
  THEN RAISE EXCEPTION 'set V1 bonus conflict %/%/%',r.suit_id,r.pieces,r.entry_index; END IF;
 END LOOP;
 FOR r IN SELECT * FROM (VALUES (1,34,3,4),(2,36,4,4),(3,36,3,6),(4,34,4,7),(5,34,4,3)) AS v(suit_id,hpmp_attr,two_count,eight_count) LOOP
  IF NOT EXISTS(SELECT FROM gamedata.ov_card_entry WHERE row_no=930000+r.suit_id*10 AND idx=0
     AND res0=0 AND op_type=1 AND attr_id=r.hpmp_attr AND mode=0 AND prob=100 AND value=2500)
   OR (SELECT count(*) FROM gamedata.ov_card_entry WHERE row_no=930000+r.suit_id*10)<>r.two_count
   OR (SELECT count(*) FROM gamedata.ov_card_entry WHERE row_no=940000+r.suit_id*10+1)<>r.eight_count
  THEN RAISE EXCEPTION 'set V1 unexpected private card entries for %',r.suit_id; END IF;
 END LOOP;
END $class_set_v1$;
