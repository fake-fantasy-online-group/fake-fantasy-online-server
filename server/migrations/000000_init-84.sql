-- Portable Zhoutian Ding: authored content in existing item/task configuration.
-- Keep the original chamber doorway and all other NPCs unchanged.
ALTER TABLE game_items ADD COLUMN IF NOT EXISTS use_action TEXT NOT NULL DEFAULT '';
ALTER TABLE game_tasks ADD COLUMN IF NOT EXISTS requires_trial_completion BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE game_tasks ADD COLUMN IF NOT EXISTS recovery_item INTEGER NOT NULL DEFAULT 0;
ALTER TABLE game_tasks ADD COLUMN IF NOT EXISTS recovery_lore TEXT NOT NULL DEFAULT '';
ALTER TABLE game_task_flow_steps DROP CONSTRAINT IF EXISTS game_task_flow_steps_kind_check;
ALTER TABLE game_task_flow_steps ADD CONSTRAINT game_task_flow_steps_kind_check
 CHECK (kind IN ('talk','collect','kill','deliver','exchange','turnin','rift_complete'));

DO $$
DECLARE
 tool CONSTANT integer := 1900028301;
 quest CONSTANT integer := 1900030001;
 icon integer;
BEGIN
 IF EXISTS(SELECT FROM game_items WHERE id=tool AND (name<>'周天鼎' OR use_action<>'cube'))
 OR EXISTS(SELECT FROM gamedata.ov_item WHERE (index=tool OR row_no=tool) AND (index IS DISTINCT FROM tool OR row_no IS DISTINCT FROM tool OR name IS DISTINCT FROM '周天鼎'))
 OR EXISTS(SELECT FROM gamedata.ov_desc WHERE (index=tool OR row_no=tool) AND (index IS DISTINCT FROM tool OR row_no IS DISTINCT FROM tool))
 OR EXISTS(SELECT FROM game_equipment WHERE id=tool)
 OR EXISTS(SELECT FROM game_tasks WHERE id=quest AND name<>'问鼎周天') THEN
  RAISE EXCEPTION 'portable cube item/task identity collision';
 END IF;
 IF NOT EXISTS(SELECT FROM map_npcs WHERE map_file='qw0007' AND npc_id=73 AND name='巧匠左慈' AND script='N611FY') THEN
  RAISE EXCEPTION 'portable cube requires the original Longcheng artisan';
 END IF;
 SELECT small_map INTO icon FROM gamedata.ov_desc WHERE index=9515 ORDER BY row_no LIMIT 1;
 IF icon IS NULL OR icon<=0 THEN RAISE EXCEPTION 'native cauldron item icon missing'; END IF;
 INSERT INTO game_items(id,name,price,level,raw,use_action)
 VALUES(tool,'周天鼎',0,0,'\x','cube') ON CONFLICT(id) DO NOTHING;
 INSERT INTO gamedata.ov_item SELECT (jsonb_populate_record(NULL::gamedata.ov_item,
 jsonb_build_object('row_no',tool,'index',tool,'name','周天鼎','script','','category',0,'query_type',0,
 'level',0,'level_need',0,'can_pile',0,'can_mail',0,'can_deal',0,'can_buy',0,'can_drop',1,
 'weight',0,'buy_price',0,'sell_price',0,'self_use',1,'other_use',0,'use_waste',0))).*
 WHERE NOT EXISTS(SELECT FROM gamedata.ov_item WHERE index=tool OR row_no=tool);
 INSERT INTO gamedata.ov_desc SELECT (jsonb_populate_record(NULL::gamedata.ov_desc,
 jsonb_build_object('row_no',tool,'index',tool,'avatar','','category',0,'type',0,'level',0,'level_need',0,
 'desc_','巧匠左慈修复的随身古鼎。永久绑定，放在背包中右击问鼎，使用不会消耗；遗失可向左慈补领。',
 'small_map',icon,'big_map',icon,'can_pile',0,'can_mail',0,'can_deal',0,'can_buy',0,'can_drop',1,
 'weight',0,'sell_price',0,'self_use',1,'other_use',0))).*
 WHERE NOT EXISTS(SELECT FROM gamedata.ov_desc WHERE index=tool OR row_no=tool);
 INSERT INTO game_tasks(id,name,npc,map,level_min,level_max,descr,reward,requires_trial_completion,recovery_item,recovery_lore)
 VALUES(quest,'问鼎周天','试炼小仙','四村',0,0,
 '试炼功成，你所得的洞天符钥也该派上用场了。这符钥能开启演劫洞天，可邀同队道友一道历练。传说洞天中的劫气散尽，便会留下净煞尘；龙城的巧匠左慈一直在寻能驾驭此物的人。你且入洞天走上一遭，完整破关之后去见她，她自有东西交给你。',
 '永久绑定的周天鼎一尊',TRUE,tool,
 E'旧器、废篆入鼎，或可还作星髓；仙遗化去，还会留下八仙余韵。\n星髓能引星开窍，遇净煞尘，又有凝成星篆的机缘。\n三枚相同星篆相聚，或能换见同象别宿。\n有些器物还藏着未醒的仙韵，须得淬炼才显；已有仙韵的，也能借余韵重铸。只是重铸有高有低，莫把好物随意丢进来。')
 ON CONFLICT(id) DO NOTHING;
 INSERT INTO game_task_rewards(task_id) VALUES(quest) ON CONFLICT DO NOTHING;
 INSERT INTO game_task_reward_items(task_id,seq,item_id,qty) VALUES(quest,1,tool,1) ON CONFLICT DO NOTHING;
 INSERT INTO game_task_flows(layer,task_id,task_name,publisher) VALUES('base',quest,'问鼎周天','试炼小仙') ON CONFLICT DO NOTHING;
 INSERT INTO game_task_flow_steps(layer,task_id,step_no,target_npc,kind,text,say,done)
 VALUES('base',quest,1,'巧匠左慈','rift_complete',
 '接取后完整通关一次任意层演劫洞天，再前往龙城拜访巧匠左慈。不要求限时，无须额外交付符钥或净煞尘。',
 '看来小仙没看错人，你果真从演劫洞天回来了。这尊周天鼎，是我依残谱修复的旧物。鼎中自有一方天地，能散器中星髓，也能借星髓重炼器物。如今便交你保管。将它随身带着，在行囊里右击便可问鼎；若不慎遗失，再来找我。',TRUE) ON CONFLICT DO NOTHING;
 DELETE FROM map_npcs WHERE map_file='qw0506' AND npc_id=1 AND script='Nzhoutianding';
END $$;
