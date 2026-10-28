BEGIN;
-- Nine-type spirit appearance availability. Original 72 ov_combine recipes
-- are untouched. This supplies a new furnace route for 18 golden exchanges
-- and 2 dark upgrades; it does not pretend these routes existed in ov_combine.
-- Ingredients: assets.fenfentu.org/db/shapes.json lingShapes golden rows;
-- real IDs: original game_descs (not website imgMap icon IDs).
-- Dark certificate 36322 explicitly upgrades an existing golden glove shape;
-- therefore the golden ingredients are not charged again.
-- No fee is specified by the recovered exchange data: these new exchanges
-- charge materials only, rather than inventing a currency amount.
-- Two equivalent glove routes also accept original certificate 36205; the
-- first pair accepts 27353. Both originals explicitly describe golden exchange.
-- Item/instance state, NPCs, ring spirits and 21 unsupported cloth shapes are
-- not migrated. Successful exchanges produce exactly one appearance item.
CREATE TEMP TABLE spirit_appearance_recipe_stage (LIKE game_craft_extra_recipes INCLUDING ALL) ON COMMIT DROP;
INSERT INTO spirit_appearance_recipe_stage(row_no,product_item_id,cost,need_lifeskill_id,prof_need,prof_max,prof_gain,disappear_rate,success_rate,skill_level_need,mat_count,mat_1,mat_2,mat_3,mat_4,mat_5,out_qty,show_type) VALUES
(5600,14919,0,0,0,0,0,0,100,0,3,6589641,1346762,101576,0,0,1,0),
(5601,14923,0,0,0,0,0,0,100,0,3,6589641,1346762,101576,0,0,1,0),
(5602,42007,0,0,0,0,0,0,100,0,3,6580954,1338075,92889,0,0,1,0),
(5603,42011,0,0,0,0,0,0,100,0,3,6580954,1338075,92889,0,0,1,0),
(5604,42654,0,0,0,0,0,0,100,0,3,6589920,1347041,101847,0,0,1,0),
(5605,42655,0,0,0,0,0,0,100,0,3,6589920,1347041,101847,0,0,1,0),
(5606,42982,0,0,0,0,0,0,100,0,3,6589953,1347074,101883,0,0,1,0),
(5607,42983,0,0,0,0,0,0,100,0,3,6589953,1347074,101883,0,0,1,0),
(5608,14908,0,0,0,0,0,0,100,0,3,6589638,1346759,101573,0,0,1,0),
(5609,14909,0,0,0,0,0,0,100,0,3,6589638,1346759,101573,0,0,1,0),
(5610,42643,0,0,0,0,0,0,100,0,3,6589918,1347039,101562,0,0,1,0),
(5611,42644,0,0,0,0,0,0,100,0,3,6589918,1347039,101562,0,0,1,0),
(5612,25104,0,0,0,0,0,0,100,0,3,6589823,1346944,91743,0,0,1,0),
(5613,25105,0,0,0,0,0,0,100,0,3,6589823,1346944,91743,0,0,1,0),
(5614,43192,0,0,0,0,0,0,100,0,3,6590017,1347138,101949,0,0,1,0),
(5615,43193,0,0,0,0,0,0,100,0,3,6590017,1347138,101949,0,0,1,0),
(5616,13707,0,0,0,0,0,0,100,0,3,6557830,1314951,91744,0,0,1,0),
(5617,13708,0,0,0,0,0,0,100,0,3,6557830,1314951,91744,0,0,1,0),
(5618,42685,0,0,0,0,0,0,100,0,2,107543,101858,0,0,0,1,0),
(5619,42686,0,0,0,0,0,0,100,0,2,107547,101858,0,0,0,1,0),
(5620,42007,0,0,0,0,0,0,100,0,3,6580954,1338075,101741,0,0,1,0),
(5621,42011,0,0,0,0,0,0,100,0,3,6580954,1338075,101741,0,0,1,0);
DO $guard$
BEGIN
 IF EXISTS (SELECT 1 FROM spirit_appearance_recipe_stage s JOIN gamedata.ov_combine o ON o.row_no=s.row_no) THEN
  RAISE EXCEPTION 'spirit appearance recipe collides with original recipe';
 END IF;
 IF EXISTS (SELECT 1 FROM spirit_appearance_recipe_stage s JOIN game_craft_extra_recipes e ON e.row_no=s.row_no WHERE to_jsonb(e) IS DISTINCT FROM to_jsonb(s)) THEN
  RAISE EXCEPTION 'spirit appearance recipe ID already belongs to different data';
 END IF;
END $guard$;
INSERT INTO game_craft_extra_recipes(row_no,product_item_id,cost,need_lifeskill_id,prof_need,prof_max,prof_gain,disappear_rate,success_rate,skill_level_need,mat_count,mat_1,mat_2,mat_3,mat_4,mat_5,out_qty,show_type) SELECT row_no,product_item_id,cost,need_lifeskill_id,prof_need,prof_max,prof_gain,disappear_rate,success_rate,skill_level_need,mat_count,mat_1,mat_2,mat_3,mat_4,mat_5,out_qty,show_type FROM spirit_appearance_recipe_stage ON CONFLICT (row_no) DO NOTHING;
COMMIT;
