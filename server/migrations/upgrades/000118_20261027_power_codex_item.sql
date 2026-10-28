-- Existing databases: apply atomically.
BEGIN;
-- Item-based power archive: use existing packaged book/scroll icons, never copies.
ALTER TABLE equipment_endgame_state ADD COLUMN IF NOT EXISTS power_scroll_id TEXT NOT NULL DEFAULT '';
ALTER TABLE equipment_endgame_state ADD COLUMN IF NOT EXISTS power_scroll_roll_bp INTEGER NOT NULL DEFAULT 0;
DO $power_codex$
DECLARE
 book CONSTANT integer := 1900028302;
 scroll CONSTANT integer := 1900028303;
 quest CONSTANT integer := 1900030003;
 item_id integer; item_name text; action text; icon integer; source_id integer; description text;
BEGIN
 IF NOT EXISTS(SELECT FROM map_npcs WHERE map_file='qw0007' AND npc_id=73 AND name='巧匠左慈' AND script='N611FY') THEN
  RAISE EXCEPTION 'power codex requires original Longcheng artisan';
 END IF;
 IF EXISTS(SELECT FROM game_tasks WHERE id=quest AND name<>'万象留痕') THEN RAISE EXCEPTION 'power codex quest collision'; END IF;
 FOR item_id,item_name,action,source_id,description IN VALUES
  (book,'万象秘典','power_codex',10025,'巧匠左慈赠予的秘典，铭记分解暗金时留下的威能。右击翻阅，提取所得卷轴可附着于暗金装备。永久绑定，使用不消耗，遗失可向左慈补领。'),
  (scroll,'威能卷轴','power_scroll',25030,'记录一项威能的卷轴。右击后选择暗金装备，将此卷所载威能附着其上；成功后消耗卷轴。永久绑定，不可交易。')
 LOOP
  IF EXISTS(SELECT FROM game_items WHERE id=item_id AND (name<>item_name OR use_action<>action))
   OR EXISTS(SELECT FROM game_equipment WHERE id=item_id)
   OR EXISTS(SELECT FROM gamedata.ov_item WHERE (index=item_id OR row_no=item_id) AND (index IS DISTINCT FROM item_id OR row_no IS DISTINCT FROM item_id OR name IS DISTINCT FROM item_name))
   OR EXISTS(SELECT FROM gamedata.ov_desc WHERE (index=item_id OR row_no=item_id) AND (index IS DISTINCT FROM item_id OR row_no IS DISTINCT FROM item_id)) THEN
   RAISE EXCEPTION 'power codex item identity collision %',item_id;
  END IF;
  SELECT small_map INTO icon FROM gamedata.ov_desc WHERE index=source_id ORDER BY row_no LIMIT 1;
  IF icon IS NULL OR icon<=0 THEN RAISE EXCEPTION 'power codex native icon missing %',source_id; END IF;
  INSERT INTO game_items(id,name,price,level,raw,use_action) VALUES(item_id,item_name,0,0,'\x',action) ON CONFLICT(id) DO NOTHING;
  INSERT INTO gamedata.ov_item SELECT (jsonb_populate_record(NULL::gamedata.ov_item,jsonb_build_object(
   'row_no',item_id,'index',item_id,'name',item_name,'script','','category',0,'query_type',0,'level',0,'level_need',0,
   'can_pile',0,'can_mail',0,'can_deal',0,'can_buy',0,'can_drop',0,'weight',0,'buy_price',0,'sell_price',0,'self_use',1,'other_use',0,'use_waste',0))).*
   WHERE NOT EXISTS(SELECT FROM gamedata.ov_item WHERE index=item_id OR row_no=item_id);
  INSERT INTO gamedata.ov_desc SELECT (jsonb_populate_record(NULL::gamedata.ov_desc,jsonb_build_object(
   'row_no',item_id,'index',item_id,'avatar','','category',0,'type',0,'level',0,'level_need',0,'desc_',description,
   'small_map',icon,'big_map',icon,'can_pile',0,'can_mail',0,'can_deal',0,'can_buy',0,'can_drop',0,'weight',0,'sell_price',0,'self_use',1,'other_use',0))).*
   WHERE NOT EXISTS(SELECT FROM gamedata.ov_desc WHERE index=item_id OR row_no=item_id);
 END LOOP;
 INSERT INTO game_tasks(id,name,npc,map,level_min,level_max,descr,reward,recovery_item,recovery_lore)
 VALUES(quest,'万象留痕','巧匠左慈','龙城',0,0,'你分解暗金器物时散出的威能引起了巧匠左慈的注意。前往龙城，与她一叙。','永久绑定的万象秘典一册',book,'分解暗金所得威能，秘典皆会铭记；同一威能只保留更佳数值。提取不损收录，所成卷轴不可交易。') ON CONFLICT DO NOTHING;
 INSERT INTO game_task_rewards(task_id) VALUES(quest) ON CONFLICT DO NOTHING;
 INSERT INTO game_task_reward_items(task_id,seq,item_id,qty) VALUES(quest,1,book,1) ON CONFLICT DO NOTHING;
 INSERT INTO game_task_flows(layer,task_id,task_name,publisher) VALUES('base',quest,'万象留痕','巧匠左慈') ON CONFLICT DO NOTHING;
 INSERT INTO game_task_flow_steps(layer,task_id,step_no,target_npc,kind,text,say,done)
 VALUES('base',quest,1,'巧匠左慈','talk','前往龙城，与巧匠左慈交谈，领取万象秘典。','器虽散，威能未必随之消逝。这册秘典赠你，留住器中真意，日后自可移附新器。',TRUE) ON CONFLICT DO NOTHING;
END $power_codex$;

COMMIT;
