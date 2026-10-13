-- Persist the 32 endgame base items in the original item configuration path.
-- Atomic and repeatable: existing exact rows are retained; conflicting IDs or
-- row numbers abort the whole statement instead of overwriting any definition.
-- Apply this fragment alone for an existing database after backup/review. The
-- migrate command remains empty-database-only. No player/instance rows change.
-- These are authored server additions; game_items.raw is empty, not fabricated
-- client binary data. Rune effects use the original ov_card_entry loader.
DO $migration$
DECLARE
  definitions CONSTANT jsonb := $endgame_items$
[
  {"id": 1900028001, "name": "星篆·角木蛟", "level": 12, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 26018, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 1, "value": 3, "mode": 0}]},
  {"id": 1900028002, "name": "星篆·亢金龙", "level": 14, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4332, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 5, "value": 3, "mode": 0}]},
  {"id": 1900028003, "name": "星篆·氐土貉", "level": 16, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4134, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 69, "value": 2, "mode": 0}]},
  {"id": 1900028004, "name": "星篆·房日兔", "level": 17, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4141, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 34, "value": 3, "mode": 1}]},
  {"id": 1900028005, "name": "星篆·心月狐", "level": 19, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4121, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 23, "value": 100, "mode": 0}]},
  {"id": 1900028006, "name": "星篆·尾火虎", "level": 21, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4102, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 47, "value": 3, "mode": 0}]},
  {"id": 1900028007, "name": "星篆·箕水豹", "level": 23, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4122, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 19, "value": 4, "mode": 0}]},
  {"id": 1900028008, "name": "星篆·斗木獬", "level": 24, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4118, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 15, "value": 3, "mode": 0}]},
  {"id": 1900028009, "name": "星篆·牛金牛", "level": 26, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4313, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 17, "value": 4, "mode": 0}]},
  {"id": 1900028010, "name": "星篆·女土蝠", "level": 28, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4158, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 61, "value": 3, "mode": 0}]},
  {"id": 1900028011, "name": "星篆·虚日鼠", "level": 30, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4304, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 36, "value": 3, "mode": 1}]},
  {"id": 1900028012, "name": "星篆·危月燕", "level": 32, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4117, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 118, "value": 3, "mode": 0}]},
  {"id": 1900028013, "name": "星篆·室火猪", "level": 33, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4132, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 70, "value": 2, "mode": 0}]},
  {"id": 1900028014, "name": "星篆·壁水貐", "level": 35, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4112, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 34, "value": 24, "mode": 0}]},
  {"id": 1900028015, "name": "星篆·奎木狼", "level": 37, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4115, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 9, "value": 3, "mode": 0}]},
  {"id": 1900028016, "name": "星篆·娄金狗", "level": 39, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4131, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 7, "value": 3, "mode": 0}]},
  {"id": 1900028017, "name": "星篆·胃土雉", "level": 40, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4345, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 13, "value": 4, "mode": 0}]},
  {"id": 1900028018, "name": "星篆·昴日鸡", "level": 42, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4142, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 23, "value": 100, "mode": 0}]},
  {"id": 1900028019, "name": "星篆·毕月乌", "level": 44, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 39439, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 47, "value": 3, "mode": 1}]},
  {"id": 1900028020, "name": "星篆·觜火猴", "level": 46, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4136, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 25, "value": -2, "mode": 1}]},
  {"id": 1900028021, "name": "星篆·参水猿", "level": 48, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4350, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 19, "value": 3, "mode": 1}]},
  {"id": 1900028022, "name": "星篆·井木犴", "level": 49, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4137, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 3, "value": 3, "mode": 0}]},
  {"id": 1900028023, "name": "星篆·鬼金羊", "level": 51, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 27653, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 66, "value": 100, "mode": 0}]},
  {"id": 1900028024, "name": "星篆·柳土獐", "level": 53, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 3583, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 17, "value": 3, "mode": 1}]},
  {"id": 1900028025, "name": "星篆·星日马", "level": 55, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4314, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 15, "value": 3, "mode": 1}]},
  {"id": 1900028026, "name": "星篆·张月鹿", "level": 56, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4344, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 36, "value": 24, "mode": 0}]},
  {"id": 1900028027, "name": "星篆·翼火蛇", "level": 58, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4116, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 27, "value": 2, "mode": 1}]},
  {"id": 1900028028, "name": "星篆·轸水蚓", "level": 60, "description": "按孔位顺序镶入星篆，可结成宿阵。", "icon": 4338, "weight": 1, "stackable": false, "can_mail": true, "can_trade": true, "effects": [{"attr": 38, "value": 1, "mode": 0}]},
  {"id": 1900028101, "name": "洞天符钥", "level": 0, "description": "开启演劫洞天的钥匙。试炼功成所得，每次消耗一枚，可邀同队道友同行。", "icon": 3387, "weight": 0, "stackable": true, "can_mail": false, "can_trade": false},
  {"id": 1900028201, "name": "星髓", "level": 0, "description": "周天鼎解器归元所得，用于开窍、凝篆与淬器。", "icon": 26017, "weight": 0, "stackable": true, "can_mail": false, "can_trade": false},
  {"id": 1900028202, "name": "净煞尘", "level": 0, "description": "演劫洞天凝结的清净劫尘，用于周天鼎炼化。", "icon": 26214, "weight": 0, "stackable": true, "can_mail": false, "can_trade": false},
  {"id": 1900028203, "name": "八仙余韵", "level": 0, "description": "仙遗化韵留下的道意，用于返本重铸与炼真守一。", "icon": 26804, "weight": 0, "stackable": true, "can_mail": false, "can_trade": false}
]
$endgame_items$::jsonb;
  item record;
  expected_item gamedata.ov_item%ROWTYPE;
  expected_desc gamedata.ov_desc%ROWTYPE;
  expected_card gamedata.ov_card%ROWTYPE;
  expected_effect gamedata.ov_card_entry%ROWTYPE;
  effect record;
  expected_canonical game_items%ROWTYPE;
BEGIN
  LOCK TABLE gamedata.ov_item, gamedata.ov_desc, gamedata.ov_card,
    gamedata.ov_card_entry, game_items, gamedata.ov_arm, game_equipment IN SHARE ROW EXCLUSIVE MODE;
  FOR item IN SELECT * FROM jsonb_to_recordset(definitions) AS d(
    id integer, name text, level integer, description text, icon integer,
    weight integer, stackable boolean, can_mail boolean, can_trade boolean, effects jsonb)
  LOOP
    IF EXISTS (SELECT 1 FROM gamedata.ov_arm WHERE index = item.id)
       OR EXISTS (SELECT 1 FROM game_equipment WHERE id = item.id) THEN
      RAISE EXCEPTION 'endgame item ID % collides with equipment', item.id;
    END IF;
    expected_item := jsonb_populate_record(NULL::gamedata.ov_item,
      jsonb_build_object('row_no', item.id, 'index', item.id, 'name', item.name,
        'script', '', 'category', 0, 'query_type', 0, 'level', item.level,
        'level_need', item.level, 'can_pile', item.stackable::integer,
        'can_mail', item.can_mail::integer, 'can_deal', item.can_trade::integer,
        'can_buy', 0, 'can_drop', 0, 'weight', item.weight, 'buy_price', 0,
        'sell_price', 0, 'self_use', 0, 'other_use', 0, 'use_waste', 0));
    expected_desc := jsonb_populate_record(NULL::gamedata.ov_desc,
      jsonb_build_object('row_no', item.id, 'index', item.id, 'avatar', '',
        'category', 0, 'type', 0, 'level', item.level, 'level_need', item.level,
        'desc_', item.description, 'small_map', item.icon, 'big_map', item.icon,
        'can_pile', item.stackable::integer, 'can_mail', item.can_mail::integer,
        'can_deal', item.can_trade::integer, 'can_buy', 0, 'can_drop', 0,
        'weight', item.weight, 'sell_price', 0, 'self_use', 0, 'other_use', 0));
    expected_canonical := jsonb_populate_record(NULL::game_items,
      jsonb_build_object('id', item.id, 'name', item.name, 'level', item.level,
        'price', 0, 'raw', '\x'));
    IF EXISTS (SELECT 1 FROM gamedata.ov_item t
        WHERE (t.index = item.id OR t.row_no = item.id)
          AND to_jsonb(t) IS DISTINCT FROM to_jsonb(expected_item))
       OR EXISTS (SELECT 1 FROM gamedata.ov_desc t
        WHERE (t.index = item.id OR t.row_no = item.id)
          AND to_jsonb(t) IS DISTINCT FROM to_jsonb(expected_desc))
       OR EXISTS (SELECT 1 FROM game_items t WHERE t.id = item.id
          AND to_jsonb(t) IS DISTINCT FROM to_jsonb(expected_canonical)) THEN
      RAISE EXCEPTION 'endgame base definition collision for item %', item.id;
    END IF;
    INSERT INTO gamedata.ov_item SELECT expected_item.* ON CONFLICT (row_no) DO NOTHING;
    INSERT INTO gamedata.ov_desc SELECT expected_desc.* ON CONFLICT (row_no) DO NOTHING;
    INSERT INTO game_items SELECT expected_canonical.* ON CONFLICT (id) DO NOTHING;
    IF NOT item.stackable THEN
      -- Physical identity and effects both follow the original card tables.
      expected_card := jsonb_populate_record(NULL::gamedata.ov_card,
        jsonb_build_object('row_no', item.id, 'index', item.id, 'item_id', item.id,
          'level_need', item.level, 'min_level', 0, 'max_level', 0));
      IF EXISTS (SELECT 1 FROM gamedata.ov_card t
          WHERE (t.index = item.id OR t.row_no = item.id OR t.item_id = item.id)
            AND to_jsonb(t) IS DISTINCT FROM to_jsonb(expected_card)) THEN
        RAISE EXCEPTION 'endgame rune card definition collision for item %', item.id;
      END IF;
      INSERT INTO gamedata.ov_card SELECT expected_card.* ON CONFLICT (row_no) DO NOTHING;
      IF EXISTS (SELECT 1 FROM gamedata.ov_card_entry
          WHERE row_no = item.id AND (idx < 0 OR idx >= jsonb_array_length(item.effects))) THEN
        RAISE EXCEPTION 'unexpected rune card effect for item %', item.id;
      END IF;
      FOR effect IN SELECT value, ordinality FROM jsonb_array_elements(item.effects) WITH ORDINALITY
      LOOP
        expected_effect := jsonb_populate_record(NULL::gamedata.ov_card_entry,
          jsonb_build_object('row_no', item.id, 'idx', effect.ordinality - 1,
            'res0', 0, 'op_type', 1, 'attr_id', (effect.value->>'attr')::integer,
            'mode', (effect.value->>'mode')::integer, 'prob', 100,
            'value', (effect.value->>'value')::integer));
        IF EXISTS (SELECT 1 FROM gamedata.ov_card_entry t
            WHERE t.row_no = item.id AND t.idx = effect.ordinality - 1
              AND to_jsonb(t) IS DISTINCT FROM to_jsonb(expected_effect)) THEN
          RAISE EXCEPTION 'endgame rune effect collision for item %', item.id;
        END IF;
        INSERT INTO gamedata.ov_card_entry SELECT expected_effect.*
          ON CONFLICT (row_no, idx) DO NOTHING;
      END LOOP;
    ELSIF EXISTS (SELECT 1 FROM gamedata.ov_card
        WHERE index = item.id OR row_no = item.id OR item_id = item.id) THEN
      RAISE EXCEPTION 'endgame stackable item % collides with socket card', item.id;
    END IF;
  END LOOP;
END
$migration$;
