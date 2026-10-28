-- UI review: presentation-only book metadata and buy-only wing spirit vendor.
-- Native icon source: init-28 row 5486, index 10757, 封印的涅磐之书.
DO $ui_review$
BEGIN
 IF (SELECT count(*) FROM game_items WHERE
     (id=1900028302 AND name='万象秘典' AND use_action='power_codex') OR
     (id=1900028303 AND name='威能卷轴' AND use_action='power_scroll'))<>2 THEN
  RAISE EXCEPTION 'UI review requires power codex item identities from 118';
 END IF;
 IF EXISTS(SELECT FROM game_equipment WHERE id IN(1900028302,1900028303)) OR
    (SELECT count(*) FROM gamedata.ov_item WHERE
      (index=1900028302 AND row_no=1900028302 AND name='万象秘典') OR
      (index=1900028303 AND row_no=1900028303 AND name='威能卷轴'))<>2 OR
    EXISTS(SELECT FROM gamedata.ov_item WHERE
      (index IN(1900028302,1900028303) OR row_no IN(1900028302,1900028303)) AND index IS DISTINCT FROM row_no) OR
    (SELECT count(*) FROM gamedata.ov_desc WHERE index IN(1900028302,1900028303) AND row_no=index)<>2 OR
    EXISTS(SELECT FROM gamedata.ov_desc WHERE
      (index IN(1900028302,1900028303) OR row_no IN(1900028302,1900028303)) AND index IS DISTINCT FROM row_no) THEN
  RAISE EXCEPTION 'UI review power codex metadata identity collision or missing prerequisite';
 END IF;
 IF NOT EXISTS(SELECT FROM gamedata.ov_desc WHERE index=10757 AND row_no=5486
     AND name_2='封印的涅磐之书' AND small_map=12143 AND big_map=12143) THEN
  RAISE EXCEPTION 'UI review requires verified native nirvana book icon 12143';
 END IF;
 IF NOT EXISTS(SELECT FROM map_npcs WHERE map_file='qw0007' AND npc_id=257
     AND name='羽灵仙子' AND script='Nyuling1' AND sell_list=6203) OR
    NOT EXISTS(SELECT FROM game_shops WHERE shop_id=6203 AND source_kind='inferred'
     AND source_note='spirit sealed purchase 10 gold' AND NOT allow_repair AND stock_source_shop_id IS NULL) OR
    (SELECT count(*) FROM game_shop_items WHERE shop_id=6203)<>1 OR
    NOT EXISTS(SELECT FROM game_shop_items WHERE shop_id=6203 AND slot=0 AND item_id=47501) OR
    EXISTS(SELECT FROM map_npcs WHERE sell_list=6203 AND NOT(map_file='qw0007' AND npc_id=257)) THEN
  RAISE EXCEPTION 'UI review requires exclusive native wing spirit shop from 122';
 END IF;
 UPDATE gamedata.ov_desc SET small_map=12143,big_map=12143,
   desc_='巧匠左慈赠予的秘典，铭记分解暗金时留下的威能。右击翻阅。' WHERE index=1900028302;
 UPDATE gamedata.ov_desc SET
   desc_='记录一项威能的卷轴。右击后选择暗金装备或羽翼，将此卷所载威能附着其上。' WHERE index=1900028303;
 -- Existing onSellItem enforces AllowSell; keep SellList for 10-gold purchases.
 UPDATE game_shops SET allow_sell=false WHERE shop_id=6203;
END $ui_review$;
