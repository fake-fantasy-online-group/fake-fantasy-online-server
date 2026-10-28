-- Apply only with the item-based power codex server/client (118), never alone
-- on the life-crafting-only branch. Recipe 900000 was code-only, not a DB row.
BEGIN;
DO $wing_scroll$
BEGIN
 IF NOT EXISTS(SELECT FROM game_items WHERE id=1900028302 AND use_action='power_codex')
 OR NOT EXISTS(SELECT FROM game_items WHERE id=1900028303 AND use_action='power_scroll') THEN
  RAISE EXCEPTION 'wing power scroll requires power codex migration 118';
 END IF;
 UPDATE game_tasks SET descr='周天鼎既已修复，也该让你识一识羽翼之道。去见龙城的幻翼域使者，他会赠你一副光之翼；今后升阶、换系可用周天鼎，融入威能请使用威能卷轴。' WHERE id=1900030002;
 UPDATE game_task_flow_steps SET say='这副光之翼先交你。洞天首通与反复历练皆能积羽，升阶、换系用周天鼎，融入威能请使用威能卷轴。' WHERE task_id=1900030002 AND target_npc='幻翼域使者';
 UPDATE gamedata.ov_desc SET desc_='巧匠左慈赠予的秘典，铭记分解暗金时留下的威能。右击翻阅，提取所得卷轴可附着于暗金装备或羽翼。永久绑定，使用不消耗，遗失可向左慈补领。' WHERE index=1900028302;
 UPDATE gamedata.ov_desc SET desc_='记录一项威能的卷轴。右击后选择暗金装备或羽翼，将此卷所载威能附着其上；羽翼仅覆盖同类威能，其他两类保留。成功后消耗卷轴。永久绑定，不可交易。' WHERE index=1900028303;
END $wing_scroll$;
COMMIT;
