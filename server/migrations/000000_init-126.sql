SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='30s';
LOCK TABLE gamedata.ov_item_icon_special IN SHARE ROW EXCLUSIVE MODE;
DO $power_icons$
BEGIN
 IF (SELECT count(*) FROM game_items WHERE
   (id=1900028302 AND use_action='power_codex') OR
   (id=1900028303 AND use_action='power_scroll'))<>2 OR
   (SELECT count(*) FROM gamedata.ov_desc WHERE
   (index=1900028302 AND small_map=12143 AND big_map=12143) OR
   (index=1900028303 AND small_map=39440 AND big_map=39440))<>2 THEN
  RAISE EXCEPTION 'Power icon prerequisite identity or source mapping mismatch';
 END IF;
 IF EXISTS(SELECT FROM gamedata.ov_item_icon_special WHERE
   (row_no IN(1900028302,1900028303) OR arm_id IN(1900028302,1900028303)) AND
   (row_no=1900028302 AND arm_id=1900028302 AND icon_small='12143' AND big_special_id='12143' OR
    row_no=1900028303 AND arm_id=1900028303 AND icon_small='39440' AND big_special_id='39440') IS NOT TRUE) THEN
  RAISE EXCEPTION 'Power icon mapping collision; refusing overwrite';
 END IF;
 INSERT INTO gamedata.ov_item_icon_special(row_no,big_special_id,arm_id,icon_small)
 VALUES(1900028302,'12143',1900028302,'12143'),(1900028303,'39440',1900028303,'39440')
 ON CONFLICT(row_no) DO NOTHING;
 IF (SELECT count(*) FROM gamedata.ov_item_icon_special WHERE arm_id IN(1900028302,1900028303))<>2 THEN
  RAISE EXCEPTION 'Power icon mapping verification failed';
 END IF;
END $power_icons$;
