SET statement_timeout=45000;

BEGIN READ ONLY;

WITH bases AS MATERIALIZED (
 SELECT DISTINCT ON(a.index) a.index AS id,a.name,a.level AS tier,a.level_need AS level,a.no_type_drop,a.is_fly_arm,d.type,
 CASE a.query_type WHEN 1 THEN '长枪' WHEN 2 THEN '长剑' WHEN 3 THEN '双手剑' WHEN 4 THEN '法杖' WHEN 5 THEN '双刃' WHEN 6 THEN '暗器' WHEN 7 THEN '盾牌' WHEN 8 THEN '服装' WHEN 9 THEN '头盔' WHEN 10 THEN '鞋子' WHEN 11 THEN '手套' WHEN 12 THEN '戒指' WHEN 13 THEN '背包' WHEN 14 THEN '项链' WHEN 15 THEN '面具' WHEN 49 THEN '耳环' WHEN 51 THEN '腰带' ELSE CASE a.position_ WHEN 114 THEN '耳环' WHEN 116 THEN '腰带' ELSE '' END END AS kind
 FROM gamedata.ov_arm a JOIN game_equipment eq ON eq.id=a.index
 JOIN LATERAL (SELECT type FROM gamedata.ov_desc d WHERE d.index=a.index ORDER BY row_no LIMIT 1) d ON true
 WHERE a.index<>0 ORDER BY a.index,a.row_no
), wild_spawns AS MATERIALIZED (
 SELECT DISTINCT ON (m.id) md.id AS map_id,md.name AS map_name,s.spawn_id,m.id AS base_monster,m.name AS base_name,m.elite_id
 FROM map_monster_spawns s JOIN map_defs md ON md.file=s.map_file
 JOIN game_monsters m ON m.id=s.monster AND m.level>0 AND m.hp>0
 JOIN game_monster_ai_templates ai ON ai.id=m.ai_template
 WHERE NOT md.is_city AND NOT EXISTS(SELECT FROM game_dungeons gd WHERE gd.map_id=md.id) ORDER BY m.id,md.id,s.spawn_id
), reachable AS MATERIALIZED (
 SELECT map_id,map_name,spawn_id,base_monster,base_name,base_monster AS monster,'direct' AS route FROM wild_spawns
 UNION ALL SELECT s.map_id,s.map_name,s.spawn_id,s.base_monster,s.base_name,e.id,'elite_replacement' FROM wild_spawns s
 JOIN game_monsters b ON b.id=s.base_monster AND b.kind='普通'
 JOIN game_monsters e ON e.id=s.elite_id AND e.kind='精英' AND e.level>0 AND e.hp>0
 JOIN game_monster_ai_templates ai ON ai.id=e.ai_template
), drop_rows AS MATERIALIZED (SELECT r.*,d.seq,d.rate,d.item_id,d.kind_name,d.lv_min,d.lv_max,m.name AS monster_name FROM reachable r JOIN game_monsters m ON m.id=r.monster JOIN game_monster_drops d ON d.monster_id=r.monster WHERE d.rate>0 AND d.rate<=100), paths AS (
 SELECT DISTINCT ON(b.id) b.*,r.map_id,r.map_name,r.spawn_id,r.base_monster,r.base_name,r.monster,r.monster_name,r.route,r.seq,r.rate,r.item_id,r.kind_name,r.lv_min,r.lv_max
 FROM bases b JOIN drop_rows r ON (r.item_id=b.id OR r.item_id=0 AND r.kind_name=b.kind AND b.tier BETWEEN r.lv_min AND r.lv_max)
 WHERE b.no_type_drop=0 AND b.level<=60 AND b.is_fly_arm=0
 AND EXISTS(SELECT FROM game_equipment_roll_options p WHERE p.equipment_level=b.tier AND (p.equipment_type=b.type OR p.equipment_type=0))
 ORDER BY b.id,r.route,r.map_id,r.monster,r.spawn_id,r.seq
), unique_ids AS(SELECT base_id FROM game_endgame_definitions WHERE owner_kind='unique' AND content_version=6)
SELECT json_build_object('paths',(SELECT json_agg(p ORDER BY id) FROM paths p),
 'unique_without_path',(SELECT json_agg(u.base_id) FROM unique_ids u WHERE NOT EXISTS(SELECT FROM paths p WHERE p.id=u.base_id)),
 'path_without_unique',(SELECT json_agg(p.id) FROM paths p WHERE NOT EXISTS(SELECT FROM unique_ids u WHERE u.base_id=p.id)),
 'unique_count',(SELECT count(*) FROM unique_ids), 'path_count',(SELECT count(*) FROM paths)) AS evidence;

ROLLBACK;

