-- Synthetic data only. Load ONLY into a newly created disposable test database.

-- This fixture is not a game-data export or a release catalog.

CREATE SCHEMA gamedata;

CREATE TABLE gamedata.ov_item (
  buy_price bigint NOT NULL DEFAULT 0,
  can_buy bigint NOT NULL DEFAULT 0,
  can_deal bigint NOT NULL DEFAULT 0,
  can_drop bigint NOT NULL DEFAULT 0,
  can_pile bigint NOT NULL DEFAULT 0,
  category bigint NOT NULL DEFAULT 0,
  index bigint NOT NULL DEFAULT 0,
  level bigint NOT NULL DEFAULT 0,
  name text NOT NULL DEFAULT '',
  row_no bigint NOT NULL DEFAULT 0,
  sell_price bigint NOT NULL DEFAULT 0,
  weight bigint NOT NULL DEFAULT 0,
  private_script text NOT NULL DEFAULT 'PRIVATE_SCRIPT_SENTINEL',
  hidden_recipe text NOT NULL DEFAULT 'PRIVATE_RECIPE_SENTINEL',
  gmlevel bigint NOT NULL DEFAULT 999
);

CREATE TABLE gamedata.ov_arm (
  agi bigint NOT NULL DEFAULT 0,
  agi_need bigint NOT NULL DEFAULT 0,
  buy_price bigint NOT NULL DEFAULT 0,
  can_buy bigint NOT NULL DEFAULT 0,
  can_deal bigint NOT NULL DEFAULT 0,
  can_drop bigint NOT NULL DEFAULT 0,
  def bigint NOT NULL DEFAULT 0,
  dex bigint NOT NULL DEFAULT 0,
  dex_need bigint NOT NULL DEFAULT 0,
  druggist bigint NOT NULL DEFAULT 0,
  duration bigint NOT NULL DEFAULT 0,
  flee bigint NOT NULL DEFAULT 0,
  hit_rate bigint NOT NULL DEFAULT 0,
  index bigint NOT NULL DEFAULT 0,
  int_ bigint NOT NULL DEFAULT 0,
  int_need bigint NOT NULL DEFAULT 0,
  level_need bigint NOT NULL DEFAULT 0,
  luk bigint NOT NULL DEFAULT 0,
  luk_need bigint NOT NULL DEFAULT 0,
  m_resist bigint NOT NULL DEFAULT 0,
  magician bigint NOT NULL DEFAULT 0,
  matk bigint NOT NULL DEFAULT 0,
  max_atk bigint NOT NULL DEFAULT 0,
  max_weight bigint NOT NULL DEFAULT 0,
  mdef bigint NOT NULL DEFAULT 0,
  min_atk bigint NOT NULL DEFAULT 0,
  name text NOT NULL DEFAULT '',
  newbie bigint NOT NULL DEFAULT 0,
  p_resist bigint NOT NULL DEFAULT 0,
  query_type bigint NOT NULL DEFAULT 0,
  row_no bigint NOT NULL DEFAULT 0,
  sell_price bigint NOT NULL DEFAULT 0,
  sex bigint NOT NULL DEFAULT 0,
  spi bigint NOT NULL DEFAULT 0,
  spi_need bigint NOT NULL DEFAULT 0,
  sr_druggist bigint NOT NULL DEFAULT 0,
  sr_magician bigint NOT NULL DEFAULT 0,
  sr_stabber bigint NOT NULL DEFAULT 0,
  sr_swordman bigint NOT NULL DEFAULT 0,
  sr_warrior bigint NOT NULL DEFAULT 0,
  stabber bigint NOT NULL DEFAULT 0,
  str bigint NOT NULL DEFAULT 0,
  str_need bigint NOT NULL DEFAULT 0,
  swordman bigint NOT NULL DEFAULT 0,
  vit bigint NOT NULL DEFAULT 0,
  vit_need bigint NOT NULL DEFAULT 0,
  warrior bigint NOT NULL DEFAULT 0,
  weight bigint NOT NULL DEFAULT 0,
  private_script text NOT NULL DEFAULT 'PRIVATE_SCRIPT_SENTINEL',
  hidden_recipe text NOT NULL DEFAULT 'PRIVATE_RECIPE_SENTINEL',
  gmlevel bigint NOT NULL DEFAULT 999
);

CREATE TABLE gamedata.ov_desc (index bigint, row_no bigint, small_map bigint, desc_ text);

CREATE TABLE gamedata.ov_card (index bigint, item_id bigint);

CREATE TABLE public.game_items (id bigint PRIMARY KEY, price bigint);

CREATE TABLE public.game_equipment (id bigint PRIMARY KEY, price bigint);

CREATE TABLE public.game_equip_extra (arm_id bigint, attr_name text, attr_id bigint, value bigint, mode bigint, seq bigint);

CREATE TABLE public.accounts (password text);

CREATE TABLE public.characters (name text);

CREATE TABLE public.game_drops (monster_name text);

CREATE TABLE public.map_npcs (script text);

CREATE TABLE public.hidden_recipes (recipe text);

INSERT INTO public.accounts VALUES ('PRIVATE_PASSWORD_SENTINEL');

INSERT INTO public.characters VALUES ('PRIVATE_CHARACTER_SENTINEL');

INSERT INTO public.game_drops VALUES ('PRIVATE_DROP_SENTINEL');

INSERT INTO public.map_npcs VALUES ('PRIVATE_NPC_SCRIPT_SENTINEL');

INSERT INTO public.hidden_recipes VALUES ('PRIVATE_HIDDEN_RECIPE_SENTINEL');

INSERT INTO gamedata.ov_item (index,row_no,name,category,level,buy_price,sell_price,weight,can_pile,can_buy,can_deal,can_drop) VALUES (900001,1,'Synthetic potion',0,1,99,8,1,1,1,1,1),(900001,2,'Duplicate ignored',0,1,999,8,1,1,1,1,1),(900003,3,'Synthetic card',100,5,10,2,1,1,1,1,1),(900004,4,'   ',0,0,0,0,0,0,0,0,0);

INSERT INTO gamedata.ov_arm (index,row_no,name,query_type,level_need,buy_price,sell_price,weight,can_buy,can_deal,can_drop,min_atk,max_atk,matk,def,mdef,duration,str,str_need,luk,sex,warrior,sr_swordman) VALUES (900002,1,'Synthetic sword',2,30,888,3,10,1,1,1,11,22,7,6,4,18000,12,10,-2,1,1,1);

INSERT INTO public.game_items VALUES (900001,20);

INSERT INTO public.game_equipment VALUES (900002,100);

INSERT INTO gamedata.ov_card VALUES (900003,900003);

INSERT INTO gamedata.ov_desc VALUES (900001,1,11,'Synthetic test item'),(900001,2,99,'Duplicate description ignored'),(900002,1,22,'Synthetic test equipment');

INSERT INTO public.game_equip_extra VALUES (900002,'Synthetic affix',1,5,1,1);

CREATE ROLE public_catalog_fixture_reader LOGIN;

GRANT USAGE ON SCHEMA gamedata TO public_catalog_fixture_reader;

GRANT SELECT ON gamedata.ov_item,gamedata.ov_arm,gamedata.ov_desc,gamedata.ov_card,public.game_items,public.game_equipment,public.game_equip_extra TO public_catalog_fixture_reader;

ALTER ROLE public_catalog_fixture_reader SET default_transaction_read_only = on;
