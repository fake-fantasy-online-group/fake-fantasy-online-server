-- Upgrade only the five native class sets. Shared card rows stay untouched;
-- each changed set bonus receives its own non-item card definition.
-- Stable entry indexes make replay safe: no multiplying values or rotating
-- thresholds a second time. New thresholds are 2,8,4,6 for original 2,4,6,8.
DO $class_set_upgrade$
DECLARE r RECORD; source_card JSONB; desired_card JSONB; actual_card JSONB;
 source_row INTEGER; suit_row INTEGER; target_card INTEGER; affected INTEGER;
BEGIN
 FOR r IN SELECT * FROM (VALUES
 (1,0,13135,34,250,2500),(1,2,5018,31,24,240),
 (2,0,13140,36,250,2500),(2,2,13151,15,20,200),
 (3,0,13140,36,250,2500),(3,2,13156,15,25,250),
 (4,0,13135,34,250,2500),(4,2,5304,31,48,480),
 (5,0,13135,34,250,2500),(5,2,5306,31,54,540)
 ) AS v(suit_id,entry_index,original_card,attr_id,original_value,new_value) LOOP
  SELECT row_no INTO STRICT suit_row FROM gamedata.ov_excesuit WHERE id=r.suit_id;
  SELECT row_no,to_jsonb(c) INTO STRICT source_row,source_card
    FROM gamedata.ov_card c WHERE c.index=r.original_card;
  IF (SELECT count(*) FROM gamedata.ov_card_entry WHERE row_no=source_row)<>1 OR
     NOT EXISTS(SELECT FROM gamedata.ov_card_entry WHERE row_no=source_row AND idx=0
       AND op_type=1 AND attr_id=r.attr_id AND mode=0 AND prob=100 AND value=r.original_value) THEN
   RAISE EXCEPTION 'class set % original card % changed',r.suit_id,r.original_card;
  END IF;
  target_card:=930000+r.suit_id*10+r.entry_index;
  IF NOT EXISTS(SELECT FROM gamedata.ov_excesuit_entry WHERE row_no=suit_row
       AND idx=r.entry_index AND ref_id IN(r.original_card,target_card)) THEN
   RAISE EXCEPTION 'class set % bonus reference changed',r.suit_id;
  END IF;
  desired_card:=source_card || jsonb_build_object('row_no',target_card,'index',target_card,'item_id',0);
  IF (SELECT count(*) FROM gamedata.ov_card c WHERE c.row_no=target_card OR c.index=target_card)>1 THEN
   RAISE EXCEPTION 'class set private card % has conflicting identities',target_card;
  END IF;
  SELECT to_jsonb(c) INTO actual_card FROM gamedata.ov_card c
    WHERE c.row_no=target_card OR c.index=target_card;
  IF NOT FOUND THEN
   INSERT INTO gamedata.ov_card SELECT (jsonb_populate_record(NULL::gamedata.ov_card,desired_card)).*;
  ELSIF actual_card IS DISTINCT FROM desired_card THEN
   RAISE EXCEPTION 'class set private card % is occupied',target_card;
  END IF;
  INSERT INTO gamedata.ov_card_entry(row_no,idx,res0,op_type,attr_id,mode,prob,value)
   VALUES(target_card,0,0,1,r.attr_id,0,100,r.new_value)
   ON CONFLICT(row_no,idx) DO NOTHING;
  IF (SELECT count(*) FROM gamedata.ov_card_entry WHERE row_no=target_card)<>1 OR
    NOT EXISTS(SELECT FROM gamedata.ov_card_entry WHERE row_no=target_card AND idx=0
      AND res0=0 AND op_type=1 AND attr_id=r.attr_id AND mode=0 AND prob=100 AND value=r.new_value) THEN
   RAISE EXCEPTION 'class set private card % effects changed',target_card;
  END IF;
  UPDATE gamedata.ov_excesuit_entry SET ref_id=target_card
   WHERE row_no=suit_row AND idx=r.entry_index;
 END LOOP;
 UPDATE gamedata.ov_excesuit_entry x SET seq=CASE x.idx WHEN 0 THEN 2 WHEN 1 THEN 8 WHEN 2 THEN 4 WHEN 3 THEN 6 END
 FROM gamedata.ov_excesuit s WHERE s.row_no=x.row_no AND s.id BETWEEN 1 AND 5 AND x.idx BETWEEN 0 AND 3;
 GET DIAGNOSTICS affected=ROW_COUNT;
 IF affected<>20 THEN RAISE EXCEPTION 'class set upgrade requires twenty native bonuses'; END IF;
END $class_set_upgrade$;
