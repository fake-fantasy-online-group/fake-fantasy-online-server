-- Final approved class-set thresholds after 93 and 94. Only the five native
-- class sets change. Existing cards (including 93/94 private cards) stay intact.
-- 2 pieces: runtime class skill only; no fake zero-valued numerical card.
-- 4 pieces: former 6-piece skill/aura plus the unchanged native 5pp critical.
-- 6 pieces: former 2-piece attributes plus the former 8-piece hit/magic defense.
-- 8 pieces: former 4-piece attack plus every former 8-piece added enhancement.
-- The runtime emits the 2/4-piece skill tooltips independently of these cards.
DO $class_set_thresholds$
DECLARE
 r RECORD; b RECORD; suit_row INTEGER; source_ref INTEGER; source_row INTEGER;
 target_card INTEGER; source_card JSONB; desired_card JSONB; actual_card JSONB;
 actual_entries JSONB; before_entries JSONB; after_entries JSONB;
 is_before BOOLEAN; first_is_before BOOLEAN;
BEGIN
 -- Validate all source effects before writing. Their original private cards
 -- remain available on replay, so a damaged source is never silently accepted.
 FOR b IN SELECT * FROM (VALUES
 (1,2,0,34,0,2500),(1,2,1,1,0,300),(1,2,2,9,0,180),
 (1,4,0,31,0,240),(1,6,0,23,0,500),
 (1,8,0,19,0,34),(1,8,1,1000101,0,1800),(1,8,2,1000102,0,1800),(1,8,3,1000104,0,1500),
 (2,2,0,36,0,2500),(2,2,1,3,0,160),(2,2,2,5,0,120),(2,2,3,36,1,10),
 (2,4,0,15,0,200),(2,6,0,66,0,500),
 (2,8,0,17,0,25),(2,8,1,1000108,0,2500),(2,8,2,1000105,0,1500),(2,8,3,1000113,0,1000),
 (3,2,0,36,0,2500),(3,2,1,3,0,180),(3,2,2,61,0,120),
 (3,4,0,15,0,250),(3,6,0,66,0,500),
 (3,8,0,17,0,15),(3,8,1,1000106,0,1800),(3,8,2,1000107,0,1800),(3,8,3,1000110,0,800),(3,8,4,1000111,0,800),(3,8,5,66,0,300),
 (4,2,0,34,0,2500),(4,2,1,1,0,240),(4,2,2,5,0,240),(4,2,3,19,1,10),
 (4,4,0,31,0,480),(4,6,0,23,0,500),
 (4,8,0,19,0,20),(4,8,1,1000101,0,1600),(4,8,2,1000103,0,800),(4,8,3,1000110,0,600),(4,8,4,1000111,0,600),(4,8,5,1000112,0,600),(4,8,6,1000113,0,600),
 (5,2,0,34,0,2500),(5,2,1,1,0,300),(5,2,2,5,0,180),(5,2,3,25,1,-8),
 (5,4,0,31,0,540),(5,6,0,23,0,500),
 (5,8,0,19,0,16),(5,8,1,1000101,0,1800),(5,8,2,1000103,0,800)
 ) AS v(suit_id,pieces,entry_index,attr,mode,value) LOOP
  source_ref:=CASE b.pieces
   WHEN 2 THEN 930000+b.suit_id*10
   WHEN 4 THEN 930000+b.suit_id*10+2
   WHEN 6 THEN CASE WHEN b.suit_id IN(2,3) THEN 13201 ELSE 13196 END
   WHEN 8 THEN 940000+b.suit_id*10+1 END;
  SELECT row_no INTO STRICT source_row FROM gamedata.ov_card WHERE index=source_ref;
  IF NOT EXISTS(SELECT FROM gamedata.ov_card_entry WHERE row_no=source_row
   AND idx=b.entry_index AND res0=0 AND op_type=1 AND attr_id=b.attr
   AND mode=b.mode AND prob=100 AND value=b.value) THEN
   RAISE EXCEPTION 'set thresholds source bonus conflict %/%/%',b.suit_id,b.pieces,b.entry_index;
  END IF;
 END LOOP;

 -- Require one complete known state, never repair a partial/foreign migration.
 FOR r IN SELECT * FROM (VALUES (1,3,4),(2,4,4),(3,3,6),(4,4,7),(5,4,3))
 AS v(suit_id,two_count,eight_count) LOOP
  SELECT row_no INTO STRICT suit_row FROM gamedata.ov_excesuit WHERE id=r.suit_id;
  source_ref:=CASE WHEN r.suit_id IN(2,3) THEN 13201 ELSE 13196 END;
  FOR b IN SELECT * FROM (VALUES
   (930000+r.suit_id*10,r.two_count),(930000+r.suit_id*10+2,1),
   (940000+r.suit_id*10+1,r.eight_count),(source_ref,1)
  ) AS v(ref,entry_count) LOOP
   SELECT row_no INTO STRICT source_row FROM gamedata.ov_card WHERE index=b.ref;
   IF (SELECT count(*) FROM gamedata.ov_card_entry WHERE row_no=source_row)<>b.entry_count
    OR (b.ref>=930000 AND NOT EXISTS(SELECT FROM gamedata.ov_card
     WHERE row_no=b.ref AND index=b.ref AND item_id=0)) THEN
    RAISE EXCEPTION 'set thresholds unexpected source card %',b.ref;
   END IF;
  END LOOP;
  SELECT jsonb_agg(jsonb_build_array(idx,ref_id,seq) ORDER BY idx) INTO actual_entries
   FROM gamedata.ov_excesuit_entry WHERE row_no=suit_row;
  before_entries:=jsonb_build_array(
   jsonb_build_array(0,930000+r.suit_id*10,2),
   jsonb_build_array(1,940000+r.suit_id*10+1,8),
   jsonb_build_array(2,930000+r.suit_id*10+2,4),
   jsonb_build_array(3,source_ref,6));
  after_entries:=jsonb_build_array(
   jsonb_build_array(1,950000+r.suit_id*10+1,6),
   jsonb_build_array(2,950000+r.suit_id*10+2,8),
   jsonb_build_array(3,source_ref,4));
  is_before:=actual_entries IS NOT DISTINCT FROM before_entries;
  IF NOT is_before AND actual_entries IS DISTINCT FROM after_entries THEN
   RAISE EXCEPTION 'set thresholds unexpected entry shape for %',r.suit_id;
  END IF;
  IF first_is_before IS NOT NULL AND first_is_before<>is_before THEN
   RAISE EXCEPTION 'set thresholds mixed migration states';
  END IF;
  first_is_before:=is_before;
 END LOOP;

 FOR r IN SELECT * FROM (VALUES (1,3,4),(2,4,4),(3,3,6),(4,4,7),(5,4,3))
 AS v(suit_id,two_count,eight_count) LOOP
  SELECT row_no INTO STRICT suit_row FROM gamedata.ov_excesuit WHERE id=r.suit_id;
  FOR b IN SELECT * FROM (VALUES
   (1,6,930000+r.suit_id*10,r.two_count+1),
   (2,8,930000+r.suit_id*10+2,r.eight_count)
  ) AS v(entry_index,pieces,source_ref,entry_count) LOOP
   target_card:=950000+r.suit_id*10+b.entry_index;
   IF EXISTS(SELECT FROM gamedata.ov_excesuit_entry WHERE ref_id=target_card
    AND (row_no<>suit_row OR idx<>b.entry_index)) THEN
    RAISE EXCEPTION 'set thresholds private card reference conflict %',target_card;
   END IF;
   SELECT to_jsonb(c) INTO STRICT source_card FROM gamedata.ov_card c WHERE index=b.source_ref;
   desired_card:=source_card || jsonb_build_object('row_no',target_card,'index',target_card,'item_id',0);
   IF (SELECT count(*) FROM gamedata.ov_card WHERE row_no=target_card OR index=target_card)>1 THEN
    RAISE EXCEPTION 'set thresholds private card identity conflict %',target_card;
   END IF;
   SELECT to_jsonb(c) INTO actual_card FROM gamedata.ov_card c WHERE row_no=target_card OR index=target_card;
   IF NOT FOUND THEN
    -- Orphan effects are not evidence that this private identity is ours.
    IF EXISTS(SELECT FROM gamedata.ov_card_entry WHERE row_no=target_card) THEN
     RAISE EXCEPTION 'set thresholds orphan private effects %',target_card;
    END IF;
    INSERT INTO gamedata.ov_card SELECT (jsonb_populate_record(NULL::gamedata.ov_card,desired_card)).*;
   ELSIF actual_card IS DISTINCT FROM desired_card THEN
    RAISE EXCEPTION 'set thresholds private card conflict %',target_card;
   END IF;
   -- 6 pieces copies the entire former 2-piece card, then its native hit/MDef.
   -- 8 pieces copies the attack card, then the added effects (not native hit/MDef).
   INSERT INTO gamedata.ov_card_entry(row_no,idx,res0,op_type,attr_id,mode,prob,value)
    SELECT target_card,e.idx,e.res0,e.op_type,e.attr_id,e.mode,e.prob,e.value
    FROM gamedata.ov_card_entry e WHERE e.row_no=(source_card->>'row_no')::int
    UNION ALL
    SELECT target_card,CASE WHEN b.pieces=6 THEN r.two_count ELSE e.idx END,
     e.res0,e.op_type,e.attr_id,e.mode,e.prob,e.value
    FROM gamedata.ov_card_entry e WHERE e.row_no=940000+r.suit_id*10+1
     AND ((b.pieces=6 AND e.idx=0) OR (b.pieces=8 AND e.idx>0))
    ON CONFLICT(row_no,idx) DO NOTHING;
   IF (SELECT count(*) FROM gamedata.ov_card_entry WHERE row_no=target_card)<>b.entry_count
    OR EXISTS(
     (SELECT idx,res0,op_type,attr_id,mode,prob,value FROM gamedata.ov_card_entry WHERE row_no=target_card)
     EXCEPT ALL
     (SELECT e.idx,e.res0,e.op_type,e.attr_id,e.mode,e.prob,e.value
      FROM gamedata.ov_card_entry e WHERE e.row_no=(source_card->>'row_no')::int
      UNION ALL
      SELECT CASE WHEN b.pieces=6 THEN r.two_count ELSE e.idx END,
       e.res0,e.op_type,e.attr_id,e.mode,e.prob,e.value
      FROM gamedata.ov_card_entry e WHERE e.row_no=940000+r.suit_id*10+1
       AND ((b.pieces=6 AND e.idx=0) OR (b.pieces=8 AND e.idx>0)))
    ) THEN
    RAISE EXCEPTION 'set thresholds private bonus conflict %',target_card;
   END IF;
   UPDATE gamedata.ov_excesuit_entry SET ref_id=target_card,seq=b.pieces
    WHERE row_no=suit_row AND idx=b.entry_index;
  END LOOP;
  UPDATE gamedata.ov_excesuit_entry SET seq=4 WHERE row_no=suit_row AND idx=3;
  DELETE FROM gamedata.ov_excesuit_entry WHERE row_no=suit_row AND idx=0;
 END LOOP;
END $class_set_thresholds$;
