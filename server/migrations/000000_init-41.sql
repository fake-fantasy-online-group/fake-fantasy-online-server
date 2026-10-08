-- 任务表中部分 NPC 名被压缩成短名，地图实体使用完整名。
-- 只收录在 map_npcs 中能唯一匹配、且人物身份一致的26个映射。
CREATE TABLE IF NOT EXISTS game_task_npc_aliases (
    task_id      INT PRIMARY KEY REFERENCES game_tasks(id) ON DELETE CASCADE,
    original_npc TEXT NOT NULL CHECK (original_npc<>''),
    resolved_npc TEXT NOT NULL CHECK (resolved_npc<>''),
    evidence     TEXT NOT NULL CHECK (evidence<>''),
    CHECK (original_npc<>resolved_npc)
);

INSERT INTO game_task_npc_aliases(task_id,original_npc,resolved_npc,evidence) VALUES
    (32,'药衡','跑货商人药衡','map_npcs 唯一后缀匹配'),
    (33,'药衡','跑货商人药衡','map_npcs 唯一后缀匹配'),
    (34,'药衡','跑货商人药衡','map_npcs 唯一后缀匹配'),
    (80,'伊源','少女伊源','map_npcs 唯一后缀匹配'),
    (127,'哈亚图','监城官哈亚图','map_npcs 唯一后缀匹配'),
    (231,'古老的石碑','石碑','试炼副本内 map_npcs 唯一实体名'),
    (232,'古老的石碑','石碑','飞升副本内 map_npcs 唯一实体名'),
    (308,'左慈','巧匠左慈','map_npcs 唯一后缀匹配'),
    (309,'元藏几','仙人元藏几','map_npcs 唯一后缀匹配'),
    (313,'元藏几','仙人元藏几','map_npcs 唯一后缀匹配'),
    (356,'安期生','仙人安期生','map_npcs 唯一后缀匹配'),
    (357,'安期生','仙人安期生','map_npcs 唯一后缀匹配'),
    (407,'蓟子训','铁拐门人蓟子训','map_npcs 唯一后缀匹配'),
    (408,'蓟子训','铁拐门人蓟子训','map_npcs 唯一后缀匹配'),
    (409,'蓟子训','铁拐门人蓟子训','map_npcs 唯一后缀匹配'),
    (411,'寒山子','果老门人寒山子','map_npcs 唯一后缀匹配'),
    (412,'寒山子','果老门人寒山子','map_npcs 唯一后缀匹配'),
    (413,'寒山子','果老门人寒山子','map_npcs 唯一后缀匹配'),
    (414,'寒山子','果老门人寒山子','map_npcs 唯一后缀匹配'),
    (416,'萼绿华','仙姑门人萼绿华','map_npcs 唯一后缀匹配'),
    (417,'萼绿华','仙姑门人萼绿华','map_npcs 唯一后缀匹配'),
    (418,'孟歧','镖师孟歧','map_npcs 唯一后缀匹配'),
    (419,'裴谌','钟离门人裴谌','map_npcs 唯一后缀匹配'),
    (421,'许栖岩','吕仙门人许栖岩','map_npcs 唯一后缀匹配'),
    (422,'许栖岩','吕仙门人许栖岩','map_npcs 唯一后缀匹配'),
    (423,'许栖岩','吕仙门人许栖岩','map_npcs 唯一后缀匹配')
ON CONFLICT(task_id) DO UPDATE SET
    original_npc=EXCLUDED.original_npc,resolved_npc=EXCLUDED.resolved_npc,evidence=EXCLUDED.evidence;

UPDATE game_tasks t SET npc=a.resolved_npc
  FROM game_task_npc_aliases a WHERE t.id=a.task_id AND t.npc=a.original_npc;
UPDATE game_task_flows f SET publisher=a.resolved_npc
  FROM game_task_npc_aliases a
 WHERE f.task_id=a.task_id AND f.publisher=a.original_npc;
UPDATE game_task_flow_steps s SET target_npc=a.resolved_npc
  FROM game_task_npc_aliases a
 WHERE s.task_id=a.task_id AND s.target_npc=a.original_npc;

DO $$
DECLARE n INT;
BEGIN
    SELECT count(*) INTO n FROM game_task_npc_aliases;
    IF n<>26 THEN RAISE EXCEPTION '任务NPC别名应为26条，实得%',n; END IF;
    SELECT count(*) INTO n FROM game_task_npc_aliases a
     WHERE NOT EXISTS(SELECT 1 FROM map_npcs p WHERE p.name=a.resolved_npc)
        OR NOT EXISTS(SELECT 1 FROM game_task_flows f WHERE f.task_id=a.task_id AND f.publisher=a.resolved_npc)
        OR EXISTS(SELECT 1 FROM game_task_flow_steps s
                   WHERE s.task_id=a.task_id AND s.target_npc=a.original_npc);
    IF n<>0 THEN RAISE EXCEPTION '任务NPC别名有%条未完全落位',n; END IF;
END $$;

ALTER TABLE game_task_disabled
    ADD COLUMN IF NOT EXISTS disabled_reason TEXT NOT NULL DEFAULT '';

-- 两处武器师匠奖励按建角时职业路线发放；初行者的路线仍为0..4。
-- 任务原文给出加工3级，ov_arm中四件幻念武器名称与ID一一对应；药师/术士共用杖。
CREATE TABLE IF NOT EXISTS game_task_profession_rewards (
    task_id INT NOT NULL REFERENCES game_tasks(id),
    race SMALLINT NOT NULL CHECK (race BETWEEN 0 AND 4),
    item_id INT NOT NULL REFERENCES game_equipment(id),
    refine_level INT NOT NULL DEFAULT 0 CHECK (refine_level BETWEEN 0 AND 10),
    PRIMARY KEY(task_id,race)
);
INSERT INTO game_task_profession_rewards(task_id,race,item_id,refine_level) VALUES
    (86,0,8003,3),(86,1,8001,3),(86,2,8004,3),(86,3,8002,3),(86,4,8002,3),
    (90,0,8003,3),(90,1,8001,3),(90,2,8004,3),(90,3,8002,3),(90,4,8002,3)
ON CONFLICT(task_id,race) DO UPDATE SET item_id=EXCLUDED.item_id,refine_level=EXCLUDED.refine_level;
DELETE FROM game_task_disabled WHERE task_id IN (86,90);

-- 小米的问题：原任务奖励明确含3支小瓶法力药水，对应唯一的小号法力药水3013。
INSERT INTO game_task_reward_items(task_id,seq,item_id,qty) VALUES (82,1,3013,3)
ON CONFLICT(task_id,seq) DO UPDATE SET item_id=EXCLUDED.item_id,qty=EXCLUDED.qty;
UPDATE game_task_disabled SET disabled_reason='神奇的粽子属于活动任务，当前不开放'
 WHERE task_id=303;

UPDATE game_task_disabled SET disabled_reason=CASE task_id
    WHEN 501 THEN '新年活动触发时间、好友邀请与祝福之花奖励尚未闭合'
    WHEN 503 THEN '普天同庆活动时段、场景播放与祝福之花奖励尚未闭合'
    ELSE disabled_reason END
WHERE disabled_reason='';

WITH chosen AS (
    SELECT t.id,t.name,t.reward,f.layer,f.publisher
      FROM game_tasks t
      JOIN LATERAL (
          SELECT layer,publisher FROM game_task_flows f
           WHERE f.task_id=t.id ORDER BY (layer='verified') DESC LIMIT 1
      ) f ON TRUE
), blockers AS (
    SELECT c.id,'发布NPC无地图落位: '||c.publisher AS reason
      FROM chosen c WHERE NOT EXISTS(SELECT 1 FROM map_npcs n WHERE n.name=c.publisher)
    UNION ALL
    SELECT c.id,'步骤NPC无地图落位: '||s.target_npc
      FROM chosen c JOIN game_task_flow_steps s ON s.task_id=c.id AND s.layer=c.layer
     WHERE NOT EXISTS(SELECT 1 FROM map_npcs n WHERE n.name=s.target_npc)
    UNION ALL
    SELECT c.id,'击杀目标无刷怪点: '||string_agg(DISTINCT k.monster_id::TEXT,',' ORDER BY k.monster_id::TEXT)
      FROM chosen c JOIN game_task_flow_kills k ON k.task_id=c.id AND k.layer=c.layer
     WHERE NOT EXISTS(SELECT 1 FROM map_monster_spawns p WHERE p.monster=k.monster_id)
     GROUP BY c.id
    UNION ALL
    SELECT c.id,'奖励文本非空，但结构化经验/名誉/金钱/物品奖励均为空'
      FROM chosen c LEFT JOIN game_task_rewards r ON r.task_id=c.id
     WHERE c.id NOT IN (400,401,402,403,404)
       AND NOT EXISTS(SELECT 1 FROM game_task_exchange_rules x WHERE x.task_id=c.id)
       AND NOT EXISTS(SELECT 1 FROM game_task_profession_rewards p WHERE p.task_id=c.id)
       AND COALESCE(r.exp,0)=0 AND COALESCE(r.honor,0)=0
       AND COALESCE(r.gold,0)=0 AND COALESCE(r.silver,0)=0 AND COALESCE(r.copper,0)=0
       AND NOT EXISTS(SELECT 1 FROM game_task_reward_items ri WHERE ri.task_id=c.id)
       AND NOT EXISTS(SELECT 1 FROM game_task_flow_step_items x
                       WHERE x.task_id=c.id AND x.layer=c.layer AND x.phase='give')
       AND c.reward NOT IN ('','无')
), grouped AS (
    SELECT id,string_agg(DISTINCT reason,'; ' ORDER BY reason) AS reason
      FROM blockers GROUP BY id
)
INSERT INTO game_task_disabled(task_id,task_name,disabled_reason)
SELECT g.id,t.name,g.reason FROM grouped g JOIN game_tasks t ON t.id=g.id
ON CONFLICT(task_id) DO UPDATE SET
    task_name=EXCLUDED.task_name,
    disabled_reason=CASE WHEN game_task_disabled.disabled_reason=''
                         THEN EXCLUDED.disabled_reason
                         ELSE game_task_disabled.disabled_reason END;

CREATE OR REPLACE VIEW game_task_audit AS
WITH chosen AS (
    SELECT t.id,t.name,t.reward,f.layer,f.publisher,
           (SELECT count(*) FROM game_task_flow_steps s
             WHERE s.task_id=t.id AND s.layer=f.layer)::INT AS step_count
      FROM game_tasks t
      JOIN LATERAL (
          SELECT layer,publisher FROM game_task_flows f
           WHERE f.task_id=t.id ORDER BY (layer='verified') DESC LIMIT 1
      ) f ON TRUE
), reward_state AS (
    SELECT c.id,CASE
      WHEN c.id IN (400,401,402,403,404) THEN 'career'
      WHEN EXISTS(SELECT 1 FROM game_task_profession_rewards p WHERE p.task_id=c.id) THEN 'profession_equipment'
      WHEN EXISTS(SELECT 1 FROM game_task_exchange_rules x WHERE x.task_id=c.id) THEN 'exchange'
      WHEN EXISTS(SELECT 1 FROM game_task_flow_step_items x
                   WHERE x.task_id=c.id AND x.layer=c.layer AND x.phase='give') THEN 'step_give'
      WHEN COALESCE(r.exp,0)<>0 OR COALESCE(r.honor,0)<>0 OR COALESCE(r.gold,0)<>0
        OR COALESCE(r.silver,0)<>0 OR COALESCE(r.copper,0)<>0
        OR EXISTS(SELECT 1 FROM game_task_reward_items ri WHERE ri.task_id=c.id) THEN 'structured'
      WHEN c.reward IN ('','无') THEN 'none'
      ELSE 'missing' END AS state
      FROM chosen c LEFT JOIN game_task_rewards r ON r.task_id=c.id
)
SELECT c.id AS task_id,c.name,d.task_id IS NULL AS enabled,c.layer,c.publisher,
       c.step_count,r.state AS reward_state,COALESCE(d.disabled_reason,'') AS disabled_reason
  FROM chosen c JOIN reward_state r ON r.id=c.id
  LEFT JOIN game_task_disabled d ON d.task_id=c.id;

DO $$
DECLARE total INT; enabled_count INT; disabled_count INT; bad INT;
BEGIN
    SELECT count(*),count(*) FILTER(WHERE enabled),count(*) FILTER(WHERE NOT enabled)
      INTO total,enabled_count,disabled_count FROM game_task_audit;
    IF total<>366 OR enabled_count<>267 OR disabled_count<>99 THEN
        RAISE EXCEPTION '任务台账数量错误 total=% enabled=% disabled=%',total,enabled_count,disabled_count;
    END IF;
    SELECT count(*) INTO bad FROM game_task_audit
     WHERE step_count<=0 OR (NOT enabled AND disabled_reason='') OR (enabled AND reward_state='missing');
    IF bad<>0 THEN RAISE EXCEPTION '任务台账有%条缺步骤、禁用原因或奖励',bad; END IF;
END $$;

CREATE TABLE game_announcement_settings (
 id SMALLINT PRIMARY KEY CHECK(id=1), speed INT NOT NULL CHECK(speed>0),
 interval_sec INT NOT NULL CHECK(interval_sec>0), enabled BOOL NOT NULL DEFAULT TRUE,
 period_min INT NOT NULL CHECK(period_min>0)
);
INSERT INTO game_announcement_settings VALUES(1,60,10,TRUE,5);
CREATE TABLE game_announcements (
 id INT PRIMARY KEY CHECK(id>0), text TEXT NOT NULL DEFAULT '', enabled BOOL NOT NULL DEFAULT TRUE
);
INSERT INTO game_announcements VALUES(1,'欢迎来到QQ吊想',TRUE);
CREATE TABLE game_horn_items (
 item_id INT PRIMARY KEY REFERENCES game_items(id),
 tier SMALLINT NOT NULL CHECK(tier BETWEEN 1 AND 255),
 skin SMALLINT NOT NULL DEFAULT 0 CHECK(skin BETWEEN 0 AND 255)
);
-- 普通大喇叭使用默认展示款，特殊外观款不在本批启用。
INSERT INTO game_horn_items VALUES(10144,1,0),(10145,1,0);
INSERT INTO game_chat_channel_rules(channel,scope,cooldown_ms,max_shares) VALUES(5,'world',10000,4);
