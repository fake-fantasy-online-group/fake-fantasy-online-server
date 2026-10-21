-- Exact native schemas/rows extracted from init-0, init-2, init-24, init-32 and init-39.
-- Narrow rollback-only PostgreSQL fixture; no player data.
CREATE SCHEMA gamedata;
CREATE TABLE IF NOT EXISTS map_defs (
    id       INT  PRIMARY KEY,          -- 地图数字 id(协议里用这个, 如 7=龙城)
    file     TEXT NOT NULL,             -- 资源文件名(qw0007), 对应 map/<file>.npc 等
    name     TEXT NOT NULL,             -- 中文名(龙城)
    is_city  BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE IF NOT EXISTS map_monster_spawns (
    map_file  TEXT NOT NULL,
    spawn_id  INT  NOT NULL,            -- 图内刷新点编号
    monster   INT  NOT NULL,            -- index, 对应 game_monsters.id
    x         INT  NOT NULL,
    y         INT  NOT NULL,
    dir       INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (map_file, spawn_id)
);
CREATE TABLE IF NOT EXISTS game_equipment (
    id   INT  PRIMARY KEY,              -- 1001 铁剑
    name TEXT NOT NULL,
    raw  BYTEA NOT NULL                 -- 338B
);
CREATE TABLE IF NOT EXISTS game_monsters (
    id     INT  PRIMARY KEY,            -- 1001 树妖
    sprite INT  NOT NULL,               -- 模型号
    name   TEXT NOT NULL,
    raw    BYTEA NOT NULL               -- 76B
);
CREATE TABLE IF NOT EXISTS game_monster_drops (
    monster_id INT  NOT NULL,
    seq        INT  NOT NULL,
    rate       REAL NOT NULL,           -- 概率（百分比，如 31.365556）
    -- 具名物品
    item_id    INT  NOT NULL DEFAULT 0, -- 已按名字重映射到我方 ov_item/ov_arm 的 index
    item_name  TEXT NOT NULL DEFAULT '',
    item_src   TEXT NOT NULL DEFAULT '',-- item / arm / unmapped
    -- 随机装备
    kind_name  TEXT NOT NULL DEFAULT '',-- 长剑 / 法杖 / 头盔 …
    lv_min     INT  NOT NULL DEFAULT 0,
    lv_max     INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (monster_id, seq)
);
CREATE TABLE IF NOT EXISTS gamedata.ov_arm (
  row_no integer NOT NULL,
  index bigint,
  name text,
  script text,
  category smallint,
  position_ smallint,
  class smallint,
  ext_type smallint,
  level smallint,
  exce_suit_id bigint,
  number integer,
  min_atk integer,
  max_atk integer,
  atk_dist integer,
  def integer,
  matk integer,
  mdef integer,
  hit_rate integer,
  flee integer,
  p_resist integer,
  m_resist integer,
  max_weight integer,
  weight integer,
  sell_price bigint,
  buy_price bigint,
  buy_bf_honor bigint,
  level_need integer,
  str_need integer,
  vit_need integer,
  agi_need integer,
  int_need integer,
  dex_need integer,
  luk_need integer,
  str integer,
  vit integer,
  agi integer,
  int_ integer,
  dex integer,
  luk integer,
  can_pile smallint,
  can_buy smallint,
  can_deal smallint,
  can_drop smallint,
  can_bind smallint,
  can_not_lock smallint,
  enchase smallint,
  drop_lost smallint,
  use_waste smallint,
  offline_lost smallint,
  switch_lost smallint,
  unique_ smallint,
  duration integer,
  dragon smallint,
  insect smallint,
  machine smallint,
  immortal smallint,
  terran smallint,
  animal smallint,
  plant smallint,
  aquatic smallint,
  angel smallint,
  ghost smallint,
  be_dragon smallint,
  be_insect smallint,
  be_machine smallint,
  be_immortal smallint,
  be_terran smallint,
  be_animal smallint,
  be_plant smallint,
  be_aquatic smallint,
  be_angel smallint,
  be_ghost smallint,
  none_ smallint,
  water smallint,
  earth smallint,
  fire smallint,
  wind smallint,
  thunder smallint,
  sage smallint,
  dark smallint,
  be_none smallint,
  be_water smallint,
  be_earth smallint,
  be_fire smallint,
  be_wind smallint,
  be_thunder smallint,
  be_sage smallint,
  be_dark smallint,
  sex smallint,
  no_fight smallint,
  can_repair smallint,
  can_spec_repair smallint,
  disappear_if_zero smallint,
  no_limit_duration smallint,
  attack_consume integer,
  be_hit_consume integer,
  dead_consume integer,
  time_consume integer,
  repair_consume integer,
  spec_repair_consume integer,
  can_mail smallint,
  no_type_drop smallint,
  spi integer,
  spi_need integer,
  newbie smallint,
  warrior smallint,
  swordman smallint,
  stabber smallint,
  druggist smallint,
  magician smallint,
  sr_warrior smallint,
  sr_swordman smallint,
  sr_stabber smallint,
  sr_druggist smallint,
  sr_magician smallint,
  importance smallint,
  avt_ext_card integer,
  avt_ext_card_prob integer,
  init_hole integer,
  init_hole_prob integer,
  init_level integer,
  init_level_prob integer,
  hurt_type bigint,
  def_type bigint,
  required_honor bigint,
  binding_skill bigint,
  hurt_absorb bigint,
  magic_hurt_absorb integer,
  max_life_num integer,
  refine_limit integer,
  enchase_limit integer,
  next_arm bigint,
  upd_special smallint,
  is_fly_arm smallint,
  hand_pos smallint,
  attack_speed integer,
  merge_limit integer,
  max_merge_limit integer,
  five_attr smallint,
  res0 smallint,
  five_attr_value integer,
  res integer,
  attr_sum smallint,
  hole_prob1 smallint,
  hole_prob2 smallint,
  hole_prob3 smallint,
  gen_bind smallint,
  res1 bytea,
  un_bind bigint,
  trade_limit smallint,
  can_soul_to_arm smallint,
  can_upgrade_soul_arm smallint,
  can_upgrade_soul_all_type smallint,
  can_attach smallint,
  no_be_attach smallint,
  tai_ji_level_need integer,
  query_type integer,
  can_seal smallint,
  paid_drop_offset smallint,
  fixed_tax bigint,
  name_2 text,
  arm_level integer,
  PRIMARY KEY (row_no)
);
CREATE TABLE IF NOT EXISTS gamedata.ov_item (
  row_no integer NOT NULL,
  index bigint,
  name text,
  script text,
  category smallint,
  ext_type smallint,
  level smallint,
  can_pile smallint,
  can_buy smallint,
  can_deal smallint,
  can_drop smallint,
  can_not_lock smallint,
  weight integer,
  sell_price bigint,
  buy_price bigint,
  buy_bf_honor bigint,
  drop_lost smallint,
  drop_event smallint,
  use_waste smallint,
  offline_lost smallint,
  switch_lost smallint,
  unique_ smallint,
  enchase smallint,
  refine smallint,
  new_union smallint,
  revive smallint,
  fly smallint,
  home smallint,
  recall smallint,
  fate smallint,
  identify smallint,
  repair smallint,
  silence smallint,
  defense smallint,
  speed smallint,
  restore smallint,
  sleep smallint,
  poison smallint,
  only_ally_use smallint,
  res smallint,
  combine smallint,
  enchase_pos smallint,
  other_use smallint,
  self_use smallint,
  size integer,
  dir integer,
  volume integer,
  level_need integer,
  skill_id integer,
  conds bigint,
  can_mail smallint,
  no_type_drop smallint,
  importance smallint,
  required_honor bigint,
  binding_pet bigint,
  is_fly_card smallint,
  use_range smallint,
  trade_limit smallint,
  monster_use smallint,
  is_multi_hose smallint,
  message_type smallint,
  cd_type smallint,
  skill_id_2 bigint,
  is_time_item smallint,
  time_item_form bigint,
  time_item_day integer,
  time_item_pos smallint,
  time_item_can_use smallint,
  query_type integer,
  sing_time integer,
  can_seal smallint,
  paid_drop_offset smallint,
  fixed_tax bigint,
  name_2 text,
  skill_id_3 bigint,
  PRIMARY KEY (row_no)
);
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS price     BIGINT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS level_req INT    NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS atk_min INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS atk_max INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS defense INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS durable INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS slot       INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS tier       INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS magic_atk  INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS magic_def  INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS hit        INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS weight     INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS strength   INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS vitality   INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS agility    INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS wisdom     INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS dexterity  INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS spirit INT NOT NULL DEFAULT 0;
ALTER TABLE game_equipment ADD COLUMN IF NOT EXISTS max_load INT NOT NULL DEFAULT 0;
ALTER TABLE game_monsters ADD COLUMN kind TEXT;
INSERT INTO "map_defs" ("id", "file", "name", "is_city") VALUES
  (20600, 'pw20600', '纯阳宝塔', FALSE);
INSERT INTO "map_monster_spawns" ("map_file", "spawn_id", "monster", "x", "y", "dir") VALUES
  ('pw20600', 267, 3060, 1324, 4590, 67);
INSERT INTO "map_monster_spawns" ("map_file", "spawn_id", "monster", "x", "y", "dir") VALUES
  ('pw20600', 272, 3062, 7608, 2717, 67);
INSERT INTO "game_equipment" ("id", "name", "price", "level_req", "atk_min", "atk_max", "defense", "durable", "slot", "tier", "magic_atk", "magic_def", "hit", "strength", "vitality", "agility", "wisdom", "dexterity", "spirit", "max_load", "raw") VALUES
  (8005, '胜利之光', 316521, 33, 9, 97, 0, 45, 4, 99, 0, 0, 40, 62, 0, 62, 0, 0, 0, 300, decode('451f0000caa4c0fbd6aeb9e200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000402000063040000000000090061004b00000000000000280000000000000000002d000100000069d404000000000021003e0000003e00000000000000000000000000000000000000000101010100010100000000605400000000000000000000000000000000000000000000000000000000000000000000000000000101000001000000a00500002c010000010100000000000001000000000100000001e0136400ff1364004e1664000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000a0005000000000000000000000000000000000000000000000000000000000000000000010100000000000200010000000000','hex'));
INSERT INTO "game_equipment" ("id", "name", "price", "level_req", "atk_min", "atk_max", "defense", "durable", "slot", "tier", "magic_atk", "magic_def", "hit", "strength", "vitality", "agility", "wisdom", "dexterity", "spirit", "max_load", "raw") VALUES
  (8051, '胜利之光', 316521, 33, 15, 115, 0, 71, 13, 99, 0, 0, 40, 62, 0, 62, 0, 0, 0, 300, decode('731f0000caa4c0fbd6aeb9e200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000d040000630400000000000f0073004b000000000000002800000000000000000047000100000069d404000000000021003e0000003e00000000000000000000000000000000000000000101010100010100000000605400000000000000000000000000000000000000000000000000000000000000000000000000000101000001000000a00500002c0100000101000000000000010000000001000000017e1464008b1464004d1664000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000a0005000000000000000100000000000000000000000000000000000000000000000000010100000000000300010000000000','hex'));
INSERT INTO "game_equipment" ("id", "name", "price", "level_req", "atk_min", "atk_max", "defense", "durable", "slot", "tier", "magic_atk", "magic_def", "hit", "strength", "vitality", "agility", "wisdom", "dexterity", "spirit", "max_load", "raw") VALUES
  (8101, '信仰之光', 316521, 33, 7, 70, 0, 55, 13, 99, 49, 0, 33, 0, 0, 0, 45, 0, 0, 300, decode('a51f0000d0c5d1f6d6aeb9e200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000d10000063020000000000070046004b000000310000002100000000000000000037000100000069d404000000000021000000000000002d0000000000000000000000000000000000000101010100010100000000605400000000000000000000000000000000000000000000000000000000000000000000000000000101000001000000a00500002c0100000101000000000000000001000000000100010a146400a7146400c41c64000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000a0005000000000000000100000000000000000000000000000000000000000000000000010100000000000400010000000000','hex'));
INSERT INTO "game_equipment" ("id", "name", "price", "level_req", "atk_min", "atk_max", "defense", "durable", "slot", "tier", "magic_atk", "magic_def", "hit", "strength", "vitality", "agility", "wisdom", "dexterity", "spirit", "max_load", "raw") VALUES
  (8401, '夜色之息', 316521, 33, 16, 63, 0, 63, 13, 99, 0, 0, 62, 29, 0, 0, 0, 95, 0, 300, decode('d1200000d2b9c9abd6aecfa200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000d0800006301000000000010003f004b000000000000003e0000000000000000003f000100000069d404000000000021001d000000000000005f000000000000000000000000000000000101010100010100000000605400000000000000000000000000000000000000000000000000000000000000000000000000000101000001000000a00500002c0100000101000000000000000100000000010000017e1464008e146400961364000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000a0005000000000000000100000000000000000000000000000000000000000000000000010100000000000500010000000000','hex'));
INSERT INTO "game_equipment" ("id", "name", "price", "level_req", "atk_min", "atk_max", "defense", "durable", "slot", "tier", "magic_atk", "magic_def", "hit", "strength", "vitality", "agility", "wisdom", "dexterity", "spirit", "max_load", "raw") VALUES
  (8451, '夜色之息', 316521, 33, 15, 64, 0, 63, 13, 99, 0, 0, 62, 29, 0, 46, 0, 95, 0, 300, decode('03210000d2b9c9abd6aecfa200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000d200100630100000000000f004000c2010000000000003e0000000000000000003f000100000069d404000000000021001d0000002e0000005f000000000000000000000000000000000101010100010100000000605400000000000000000000000000000000000000000000000000000000000000000000000000000101000001000000a00500002c010000010100000000000000010000000001000001e01364008e1464009a1364000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000a0005000000000000000100000000000000000000000000000000000000000000000000010100000000000600010000000000','hex'));
INSERT INTO "game_monsters" ("id", "sprite", "name", "raw") VALUES
  (3060, 203, '彷徨的凶灵（头目）', decode('f40b0000cb000000e1dde1e5b5c4d0d7c1e9a3a8cdb7c4bfa3a90000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000','hex'));
INSERT INTO "game_monsters" ("id", "sprite", "name", "raw") VALUES
  (3062, 203, '嫉妒的凶灵（头目）', decode('f60b0000cb000000bcb5b6cab5c4d0d7c1e9a3a8cdb7c4bfa3a90000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000','hex'));
INSERT INTO "gamedata"."ov_arm" ("row_no", "index", "name", "script", "category", "position_", "class", "ext_type", "level", "exce_suit_id", "number", "min_atk", "max_atk", "atk_dist", "def", "matk", "mdef", "hit_rate", "flee", "p_resist", "m_resist", "max_weight", "weight", "sell_price", "buy_price", "buy_bf_honor", "level_need", "str_need", "vit_need", "agi_need", "int_need", "dex_need", "luk_need", "str", "vit", "agi", "int_", "dex", "luk", "can_pile", "can_buy", "can_deal", "can_drop", "can_bind", "can_not_lock", "enchase", "drop_lost", "use_waste", "offline_lost", "switch_lost", "unique_", "duration", "dragon", "insect", "machine", "immortal", "terran", "animal", "plant", "aquatic", "angel", "ghost", "be_dragon", "be_insect", "be_machine", "be_immortal", "be_terran", "be_animal", "be_plant", "be_aquatic", "be_angel", "be_ghost", "none_", "water", "earth", "fire", "wind", "thunder", "sage", "dark", "be_none", "be_water", "be_earth", "be_fire", "be_wind", "be_thunder", "be_sage", "be_dark", "sex", "no_fight", "can_repair", "can_spec_repair", "disappear_if_zero", "no_limit_duration", "attack_consume", "be_hit_consume", "dead_consume", "time_consume", "repair_consume", "spec_repair_consume", "can_mail", "no_type_drop", "spi", "spi_need", "newbie", "warrior", "swordman", "stabber", "druggist", "magician", "sr_warrior", "sr_swordman", "sr_stabber", "sr_druggist", "sr_magician", "importance", "avt_ext_card", "avt_ext_card_prob", "init_hole", "init_hole_prob", "init_level", "init_level_prob", "hurt_type", "def_type", "required_honor", "binding_skill", "hurt_absorb", "magic_hurt_absorb", "max_life_num", "refine_limit", "enchase_limit", "next_arm", "upd_special", "is_fly_arm", "hand_pos", "attack_speed", "merge_limit", "max_merge_limit", "five_attr", "res0", "five_attr_value", "res", "attr_sum", "hole_prob1", "hole_prob2", "hole_prob3", "gen_bind", "res1", "un_bind", "trade_limit", "can_soul_to_arm", "can_upgrade_soul_arm", "can_upgrade_soul_all_type", "can_attach", "no_be_attach", "tai_ji_level_need", "query_type", "can_seal", "paid_drop_offset", "fixed_tax", "name_2", "arm_level") VALUES
  (1262, 8005, '胜利之光', '', 4, 2, 0, 0, 99, 4, 0, 9, 97, 75, 0, 0, 0, 40, 0, 0, 0, 0, 45, 1, 316521, 0, 33, 62, 0, 62, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 0, 1, 1, 0, 0, 0, 0, 21600, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 1, 0, 1440, 0, 300, 0, 1, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, decode('000000','hex'), 0, 0, 1, 1, 0, 0, 0, 0, 2, 1, 0, 0, '胜利之光', 1123);
INSERT INTO "gamedata"."ov_arm" ("row_no", "index", "name", "script", "category", "position_", "class", "ext_type", "level", "exce_suit_id", "number", "min_atk", "max_atk", "atk_dist", "def", "matk", "mdef", "hit_rate", "flee", "p_resist", "m_resist", "max_weight", "weight", "sell_price", "buy_price", "buy_bf_honor", "level_need", "str_need", "vit_need", "agi_need", "int_need", "dex_need", "luk_need", "str", "vit", "agi", "int_", "dex", "luk", "can_pile", "can_buy", "can_deal", "can_drop", "can_bind", "can_not_lock", "enchase", "drop_lost", "use_waste", "offline_lost", "switch_lost", "unique_", "duration", "dragon", "insect", "machine", "immortal", "terran", "animal", "plant", "aquatic", "angel", "ghost", "be_dragon", "be_insect", "be_machine", "be_immortal", "be_terran", "be_animal", "be_plant", "be_aquatic", "be_angel", "be_ghost", "none_", "water", "earth", "fire", "wind", "thunder", "sage", "dark", "be_none", "be_water", "be_earth", "be_fire", "be_wind", "be_thunder", "be_sage", "be_dark", "sex", "no_fight", "can_repair", "can_spec_repair", "disappear_if_zero", "no_limit_duration", "attack_consume", "be_hit_consume", "dead_consume", "time_consume", "repair_consume", "spec_repair_consume", "can_mail", "no_type_drop", "spi", "spi_need", "newbie", "warrior", "swordman", "stabber", "druggist", "magician", "sr_warrior", "sr_swordman", "sr_stabber", "sr_druggist", "sr_magician", "importance", "avt_ext_card", "avt_ext_card_prob", "init_hole", "init_hole_prob", "init_level", "init_level_prob", "hurt_type", "def_type", "required_honor", "binding_skill", "hurt_absorb", "magic_hurt_absorb", "max_life_num", "refine_limit", "enchase_limit", "next_arm", "upd_special", "is_fly_arm", "hand_pos", "attack_speed", "merge_limit", "max_merge_limit", "five_attr", "res0", "five_attr_value", "res", "attr_sum", "hole_prob1", "hole_prob2", "hole_prob3", "gen_bind", "res1", "un_bind", "trade_limit", "can_soul_to_arm", "can_upgrade_soul_arm", "can_upgrade_soul_all_type", "can_attach", "no_be_attach", "tai_ji_level_need", "query_type", "can_seal", "paid_drop_offset", "fixed_tax", "name_2", "arm_level") VALUES
  (1301, 8051, '胜利之光', '', 13, 4, 0, 0, 99, 4, 0, 15, 115, 75, 0, 0, 0, 40, 0, 0, 0, 0, 71, 1, 316521, 0, 33, 62, 0, 62, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 0, 1, 1, 0, 0, 0, 0, 21600, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 1, 0, 1440, 0, 300, 0, 1, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 5, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, decode('000000','hex'), 0, 0, 1, 1, 0, 0, 0, 0, 3, 1, 0, 0, '胜利之光', 1123);
INSERT INTO "gamedata"."ov_arm" ("row_no", "index", "name", "script", "category", "position_", "class", "ext_type", "level", "exce_suit_id", "number", "min_atk", "max_atk", "atk_dist", "def", "matk", "mdef", "hit_rate", "flee", "p_resist", "m_resist", "max_weight", "weight", "sell_price", "buy_price", "buy_bf_honor", "level_need", "str_need", "vit_need", "agi_need", "int_need", "dex_need", "luk_need", "str", "vit", "agi", "int_", "dex", "luk", "can_pile", "can_buy", "can_deal", "can_drop", "can_bind", "can_not_lock", "enchase", "drop_lost", "use_waste", "offline_lost", "switch_lost", "unique_", "duration", "dragon", "insect", "machine", "immortal", "terran", "animal", "plant", "aquatic", "angel", "ghost", "be_dragon", "be_insect", "be_machine", "be_immortal", "be_terran", "be_animal", "be_plant", "be_aquatic", "be_angel", "be_ghost", "none_", "water", "earth", "fire", "wind", "thunder", "sage", "dark", "be_none", "be_water", "be_earth", "be_fire", "be_wind", "be_thunder", "be_sage", "be_dark", "sex", "no_fight", "can_repair", "can_spec_repair", "disappear_if_zero", "no_limit_duration", "attack_consume", "be_hit_consume", "dead_consume", "time_consume", "repair_consume", "spec_repair_consume", "can_mail", "no_type_drop", "spi", "spi_need", "newbie", "warrior", "swordman", "stabber", "druggist", "magician", "sr_warrior", "sr_swordman", "sr_stabber", "sr_druggist", "sr_magician", "importance", "avt_ext_card", "avt_ext_card_prob", "init_hole", "init_hole_prob", "init_level", "init_level_prob", "hurt_type", "def_type", "required_honor", "binding_skill", "hurt_absorb", "magic_hurt_absorb", "max_life_num", "refine_limit", "enchase_limit", "next_arm", "upd_special", "is_fly_arm", "hand_pos", "attack_speed", "merge_limit", "max_merge_limit", "five_attr", "res0", "five_attr_value", "res", "attr_sum", "hole_prob1", "hole_prob2", "hole_prob3", "gen_bind", "res1", "un_bind", "trade_limit", "can_soul_to_arm", "can_upgrade_soul_arm", "can_upgrade_soul_all_type", "can_attach", "no_be_attach", "tai_ji_level_need", "query_type", "can_seal", "paid_drop_offset", "fixed_tax", "name_2", "arm_level") VALUES
  (1302, 8101, '信仰之光', '', 13, 16, 0, 0, 99, 2, 0, 7, 70, 75, 0, 49, 0, 33, 0, 0, 0, 0, 55, 1, 316521, 0, 33, 0, 0, 0, 45, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 0, 1, 1, 0, 0, 0, 0, 21600, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 1, 0, 1440, 0, 300, 0, 1, 1, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 5, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, decode('000000','hex'), 0, 0, 1, 1, 0, 0, 0, 0, 4, 1, 0, 0, '信仰之光', 611);
INSERT INTO "gamedata"."ov_arm" ("row_no", "index", "name", "script", "category", "position_", "class", "ext_type", "level", "exce_suit_id", "number", "min_atk", "max_atk", "atk_dist", "def", "matk", "mdef", "hit_rate", "flee", "p_resist", "m_resist", "max_weight", "weight", "sell_price", "buy_price", "buy_bf_honor", "level_need", "str_need", "vit_need", "agi_need", "int_need", "dex_need", "luk_need", "str", "vit", "agi", "int_", "dex", "luk", "can_pile", "can_buy", "can_deal", "can_drop", "can_bind", "can_not_lock", "enchase", "drop_lost", "use_waste", "offline_lost", "switch_lost", "unique_", "duration", "dragon", "insect", "machine", "immortal", "terran", "animal", "plant", "aquatic", "angel", "ghost", "be_dragon", "be_insect", "be_machine", "be_immortal", "be_terran", "be_animal", "be_plant", "be_aquatic", "be_angel", "be_ghost", "none_", "water", "earth", "fire", "wind", "thunder", "sage", "dark", "be_none", "be_water", "be_earth", "be_fire", "be_wind", "be_thunder", "be_sage", "be_dark", "sex", "no_fight", "can_repair", "can_spec_repair", "disappear_if_zero", "no_limit_duration", "attack_consume", "be_hit_consume", "dead_consume", "time_consume", "repair_consume", "spec_repair_consume", "can_mail", "no_type_drop", "spi", "spi_need", "newbie", "warrior", "swordman", "stabber", "druggist", "magician", "sr_warrior", "sr_swordman", "sr_stabber", "sr_druggist", "sr_magician", "importance", "avt_ext_card", "avt_ext_card_prob", "init_hole", "init_hole_prob", "init_level", "init_level_prob", "hurt_type", "def_type", "required_honor", "binding_skill", "hurt_absorb", "magic_hurt_absorb", "max_life_num", "refine_limit", "enchase_limit", "next_arm", "upd_special", "is_fly_arm", "hand_pos", "attack_speed", "merge_limit", "max_merge_limit", "five_attr", "res0", "five_attr_value", "res", "attr_sum", "hole_prob1", "hole_prob2", "hole_prob3", "gen_bind", "res1", "un_bind", "trade_limit", "can_soul_to_arm", "can_upgrade_soul_arm", "can_upgrade_soul_all_type", "can_attach", "no_be_attach", "tai_ji_level_need", "query_type", "can_seal", "paid_drop_offset", "fixed_tax", "name_2", "arm_level") VALUES
  (1306, 8401, '夜色之息', '', 13, 8, 0, 0, 99, 1, 0, 16, 63, 75, 0, 0, 0, 62, 0, 0, 0, 0, 63, 1, 316521, 0, 33, 29, 0, 0, 0, 95, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 0, 1, 1, 0, 0, 0, 0, 21600, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 1, 0, 1440, 0, 300, 0, 1, 1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 5, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, decode('000000','hex'), 0, 0, 1, 1, 0, 0, 0, 0, 5, 1, 0, 0, '夜色之息', 355);
INSERT INTO "gamedata"."ov_arm" ("row_no", "index", "name", "script", "category", "position_", "class", "ext_type", "level", "exce_suit_id", "number", "min_atk", "max_atk", "atk_dist", "def", "matk", "mdef", "hit_rate", "flee", "p_resist", "m_resist", "max_weight", "weight", "sell_price", "buy_price", "buy_bf_honor", "level_need", "str_need", "vit_need", "agi_need", "int_need", "dex_need", "luk_need", "str", "vit", "agi", "int_", "dex", "luk", "can_pile", "can_buy", "can_deal", "can_drop", "can_bind", "can_not_lock", "enchase", "drop_lost", "use_waste", "offline_lost", "switch_lost", "unique_", "duration", "dragon", "insect", "machine", "immortal", "terran", "animal", "plant", "aquatic", "angel", "ghost", "be_dragon", "be_insect", "be_machine", "be_immortal", "be_terran", "be_animal", "be_plant", "be_aquatic", "be_angel", "be_ghost", "none_", "water", "earth", "fire", "wind", "thunder", "sage", "dark", "be_none", "be_water", "be_earth", "be_fire", "be_wind", "be_thunder", "be_sage", "be_dark", "sex", "no_fight", "can_repair", "can_spec_repair", "disappear_if_zero", "no_limit_duration", "attack_consume", "be_hit_consume", "dead_consume", "time_consume", "repair_consume", "spec_repair_consume", "can_mail", "no_type_drop", "spi", "spi_need", "newbie", "warrior", "swordman", "stabber", "druggist", "magician", "sr_warrior", "sr_swordman", "sr_stabber", "sr_druggist", "sr_magician", "importance", "avt_ext_card", "avt_ext_card_prob", "init_hole", "init_hole_prob", "init_level", "init_level_prob", "hurt_type", "def_type", "required_honor", "binding_skill", "hurt_absorb", "magic_hurt_absorb", "max_life_num", "refine_limit", "enchase_limit", "next_arm", "upd_special", "is_fly_arm", "hand_pos", "attack_speed", "merge_limit", "max_merge_limit", "five_attr", "res0", "five_attr_value", "res", "attr_sum", "hole_prob1", "hole_prob2", "hole_prob3", "gen_bind", "res1", "un_bind", "trade_limit", "can_soul_to_arm", "can_upgrade_soul_arm", "can_upgrade_soul_all_type", "can_attach", "no_be_attach", "tai_ji_level_need", "query_type", "can_seal", "paid_drop_offset", "fixed_tax", "name_2", "arm_level") VALUES
  (1307, 8451, '夜色之息', '', 13, 32, 1, 0, 99, 1, 0, 15, 64, 450, 0, 0, 0, 62, 0, 0, 0, 0, 63, 1, 316521, 0, 33, 29, 0, 46, 0, 95, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 0, 1, 1, 0, 0, 0, 0, 21600, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 1, 0, 1440, 0, 300, 0, 1, 1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 10, 5, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, decode('000000','hex'), 0, 0, 1, 1, 0, 0, 0, 0, 6, 1, 0, 0, '夜色之息', 355);
UPDATE game_monsters SET kind='BOSS';
INSERT INTO game_monster_drops(monster_id,seq,rate,item_id,item_name,item_src,kind_name,lv_min,lv_max) VALUES
(3060,0,27.777514,4458,'彷徨之翼','item','',0,0),
(3060,1,0.104166,8005,'胜利之光','arm','',0,0),
(3060,2,0.104166,8005,'胜利之光','arm','',0,0),
(3060,3,0.104166,8101,'信仰之光','arm','',0,0),
(3062,0,27.777514,4460,'嫉妒之翼','item','',0,0),
(3062,1,0.104166,8401,'夜色之息','arm','',0,0),
(3062,2,0.104166,8401,'夜色之息','arm','',0,0),
(3062,3,0.104166,8101,'信仰之光','arm','',0,0);
CREATE TABLE IF NOT EXISTS game_boss_fixed_equipment_pool (
    map_id      INT NOT NULL CHECK (map_id > 0),
    monster_id INT NOT NULL CHECK (monster_id > 0),
    seq         SMALLINT NOT NULL CHECK (seq > 0),
    minimum     SMALLINT NOT NULL CHECK (minimum BETWEEN 1 AND 2),
    item_id     INT NOT NULL CHECK (item_id > 0),
    rate_pct    DOUBLE PRECISION NOT NULL CHECK (rate_pct BETWEEN 0 AND 100),
    weight      BIGINT NOT NULL CHECK (weight > 0),
	roll_affixes BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (map_id,monster_id,seq),
    UNIQUE (map_id,monster_id,item_id)
);
WITH fixed_defs(map_id,monster_id,minimum) AS (VALUES
    (20600,3059,1),(20600,3060,1),(20600,3061,1),(20600,3062,1),(20600,3063,1),
    (20701,3080,1),(20701,3081,1),(20701,3082,2),
    (20801,3067,2),(20802,3071,2),(20803,3075,2),
    (20901,3115,1),(20901,3116,1),(20901,3117,1),
    (20902,3118,1),(20902,3119,1),
    (20903,3125,2),(20903,3126,2)
), grouped AS (
    SELECT f.map_id,f.monster_id,f.minimum,d.item_id,count(*)::BIGINT AS copies,
           CASE WHEN bool_or(d.rate>=100) THEN 100::DOUBLE PRECISION
                ELSE 100*(1-exp(sum(ln(1-d.rate::DOUBLE PRECISION/100)))) END AS rate_pct
      FROM fixed_defs f
      JOIN game_monster_drops d ON d.monster_id=f.monster_id AND d.item_id>0
      JOIN game_equipment e ON e.id=d.item_id
     GROUP BY f.map_id,f.monster_id,f.minimum,d.item_id
), numbered AS (
    SELECT *,row_number() OVER(PARTITION BY map_id,monster_id ORDER BY item_id)::SMALLINT seq
      FROM grouped
)
INSERT INTO game_boss_fixed_equipment_pool(map_id,monster_id,seq,minimum,item_id,rate_pct,weight,roll_affixes)
SELECT map_id,monster_id,seq,minimum,item_id,rate_pct,copies,FALSE FROM numbered
ON CONFLICT(map_id,monster_id,item_id) DO UPDATE SET
    minimum=EXCLUDED.minimum,rate_pct=EXCLUDED.rate_pct,weight=EXCLUDED.weight,
    roll_affixes=EXCLUDED.roll_affixes;

