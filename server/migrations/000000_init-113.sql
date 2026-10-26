-- Homogeneous astral rune stacks retain their physical card UID/snapshot.
-- Counts already live in location tables. Do not rewrite any player inventory.
DO $rune_stacks$
DECLARE rune_id INTEGER;
BEGIN
  -- NULL means a legacy snapshot: preserve its historical root-flag fallback.
  -- New writes retain the complete card snapshot independently of root flags.
  ALTER TABLE card_instances ADD COLUMN IF NOT EXISTS snapshot_bound BOOLEAN;
  ALTER TABLE card_instances ADD COLUMN IF NOT EXISTS snapshot_locked BOOLEAN;
  FOR rune_id IN SELECT value::integer FROM jsonb_array_elements_text(
    $rune_stack_ids$[1900028001,1900028002,1900028003,1900028004,1900028005,1900028006,1900028007,1900028008,1900028009,1900028010,1900028011,1900028012,1900028013,1900028014,1900028015,1900028016,1900028017,1900028018,1900028019,1900028020,1900028021,1900028022,1900028023,1900028024,1900028025,1900028026,1900028027,1900028028]$rune_stack_ids$::jsonb)
  LOOP
    IF (SELECT count(*) FROM gamedata.ov_item WHERE index=rune_id AND row_no=rune_id AND can_pile IN (0,1)) <> 1
      OR (SELECT count(*) FROM gamedata.ov_desc WHERE index=rune_id AND row_no=rune_id AND can_pile IN (0,1)) <> 1
      OR (SELECT count(*) FROM gamedata.ov_card WHERE item_id=rune_id AND index=rune_id AND row_no=rune_id) <> 1
      OR NOT EXISTS (SELECT FROM gamedata.ov_card_entry WHERE row_no=rune_id)
      OR NOT EXISTS (SELECT FROM game_items WHERE id=rune_id) THEN
      RAISE EXCEPTION 'astral rune stack migration requires intact rune definition %', rune_id;
    END IF;
    UPDATE gamedata.ov_item SET can_pile=1 WHERE index=rune_id AND row_no=rune_id;
    UPDATE gamedata.ov_desc SET can_pile=1 WHERE index=rune_id AND row_no=rune_id;
  END LOOP;
END $rune_stacks$;
