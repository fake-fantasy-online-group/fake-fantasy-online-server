CREATE TABLE game_monsters(id INT PRIMARY KEY,level INT,kind TEXT);
CREATE TABLE map_monster_spawns(map_file TEXT,monster INT);
CREATE TABLE game_monster_drops(monster_id INT,seq INT,rate REAL,item_id INT,item_src TEXT,PRIMARY KEY(monster_id,seq));
CREATE TABLE game_dungeon_drop_templates(monster_id INT PRIMARY KEY,role TEXT,source_monster_id INT);
INSERT INTO game_monsters VALUES(1014,24,'普通'),(5007,24,'普通'),(5066,25,'普通'),(3049,24,'精英'),(3050,25,'精英'),(3111,50,'普通'),(4044,25,'精英');
INSERT INTO map_monster_spawns VALUES('qw0028',1014),('qw1234',3111),('pw20701',3049),('pw20701',3050);
INSERT INTO game_dungeon_drop_templates VALUES(3049,'small',5007),(3050,'small',5066),(8000,'small',1014),(3080,'boss',4044);
INSERT INTO game_monster_drops VALUES
 (3049,8,40,4126,'dungeon_template:5007:small'),(3050,8,40,4126,'dungeon_template:5066:small'),
 (3049,9,60,3034,'dungeon_template:5007:small'),(3049,10,1,1025,'dungeon_template:5007:small'),
 (3049,11,1,0,'dungeon_template:5007:small'),
 (5007,0,20,4126,'item'),(5066,0,20,4126,'item'),(3111,0,20,4126,'item'),
 (8000,1,20,4126,'dungeon_template:3111:small'),(3080,1,20,4126,'dungeon_template:4044:boss'),
 (1014,0,37.5,4115,'item');
CREATE TEMP TABLE expected_drops AS SELECT * FROM game_monster_drops WHERE NOT(monster_id IN(3049,3050) AND seq=8);
CREATE TEMP TABLE expected_templates AS SELECT * FROM game_dungeon_drop_templates WHERE monster_id NOT IN(3049,3050);
\ir ../../migrations/upgrades/000098_20261021_dungeon_drop_source_scope.sql
\ir ../../migrations/upgrades/000098_20261021_dungeon_drop_source_scope.sql
\ir ../../migrations/000000_init-98.sql
DO $$ BEGIN
 IF EXISTS((TABLE game_monster_drops EXCEPT TABLE expected_drops) UNION ALL (TABLE expected_drops EXCEPT TABLE game_monster_drops)) THEN RAISE EXCEPTION 'unapproved drop change'; END IF;
 IF EXISTS((TABLE game_dungeon_drop_templates EXCEPT TABLE expected_templates) UNION ALL (TABLE expected_templates EXCEPT TABLE game_dungeon_drop_templates)) THEN RAISE EXCEPTION 'wrong source retirement'; END IF;
END $$;
