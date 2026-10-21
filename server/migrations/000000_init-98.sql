-- Retire unspawned same-name/special dungeon sources; preserve other drops.
-- Runtime consumes materialized drops only. Fresh init runs 39 then this correction.
DO $clean_dungeon_granite$
BEGIN
 LOCK TABLE game_monster_drops,game_dungeon_drop_templates IN SHARE ROW EXCLUSIVE MODE;
 -- Match the complete provenance string, never an item ID or monster range alone.
 DELETE FROM game_monster_drops d
 WHERE d.item_id=4126 AND d.item_src ~ '^dungeon_template:[0-9]+:small$'
 AND NOT EXISTS(SELECT FROM map_monster_spawns s
  WHERE s.monster::TEXT=split_part(d.item_src,':',2) AND s.map_file LIKE 'qw%');
 -- Retire invalid source selection only. Do not regenerate the other copied rows.
 DELETE FROM game_dungeon_drop_templates t WHERE t.role='small'
 AND NOT EXISTS(SELECT FROM map_monster_spawns s WHERE s.monster=t.source_monster_id AND s.map_file LIKE 'qw%');
END $clean_dungeon_granite$;

