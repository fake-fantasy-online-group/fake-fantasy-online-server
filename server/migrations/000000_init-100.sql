-- User-approved beginner/quest special weapons; no character backfill.
-- Same-route alternatives are BOTH granted because completion has no selection UI.
ALTER TABLE game_task_profession_rewards DROP CONSTRAINT IF EXISTS game_task_profession_rewards_pkey;
ALTER TABLE game_task_profession_rewards ADD PRIMARY KEY(task_id,race,item_id);
INSERT INTO game_task_profession_rewards(task_id,race,item_id,refine_level) VALUES
 (85,0,1618,0),
 (85,1,1019,0),
 (85,1,1518,0),
 (85,2,1818,0),
 (85,2,1920,0),
 (85,3,1318,0),
 (85,4,1318,0),
 (87,0,1618,0),
 (87,1,1019,0),
 (87,1,1518,0),
 (87,2,1818,0),
 (87,2,1920,0),
 (87,3,1318,0),
 (87,4,1318,0),
 (36,0,1619,0),
 (36,1,1020,0),
 (36,1,1519,0),
 (36,2,1819,0),
 (36,2,1921,0),
 (36,3,1319,0),
 (36,4,1319,0),
 (37,0,1619,0),
 (37,1,1020,0),
 (37,1,1519,0),
 (37,2,1819,0),
 (37,2,1921,0),
 (37,3,1319,0),
 (37,4,1319,0),
 (42,0,1620,0),
 (42,1,1021,0),
 (42,1,1520,0),
 (42,2,1820,0),
 (42,2,1922,0),
 (42,3,1320,0),
 (42,4,1320,0)
ON CONFLICT(task_id,race,item_id) DO UPDATE SET refine_level=EXCLUDED.refine_level;
UPDATE game_tasks SET reward = reward || '；追加本职业利器（剑客与刺客各两种武器，其他职业一柄）'
 WHERE id IN (85,87,36,37,42) AND reward NOT LIKE '%追加本职业利器%';
