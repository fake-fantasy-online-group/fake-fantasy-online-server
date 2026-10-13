-- Published rule versions are immutable. Operators add a new version, then
-- change the separate active selector. Old runs bind to their saved version.
CREATE TABLE IF NOT EXISTS game_loot_rule_versions (
 version INTEGER PRIMARY KEY CHECK(version>0), published BOOLEAN NOT NULL DEFAULT false,
 min_rune_level INTEGER NOT NULL,
 unique_base_ppb BIGINT NOT NULL,
 unique_mf_boost BIGINT NOT NULL,
 unique_mf_divisor BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS game_loot_rule_active (id INTEGER PRIMARY KEY CHECK(id=1), version INTEGER NOT NULL REFERENCES game_loot_rule_versions(version));
CREATE TABLE IF NOT EXISTS game_loot_rune_pool (version INTEGER REFERENCES game_loot_rule_versions(version),seq INTEGER CHECK(seq>0),item_id INTEGER NOT NULL REFERENCES game_items(id),PRIMARY KEY(version,seq),UNIQUE(version,item_id));
CREATE TABLE IF NOT EXISTS game_loot_socket_levels (version INTEGER REFERENCES game_loot_rule_versions(version),min_level INTEGER CHECK(min_level>0),cap INTEGER NOT NULL CHECK(cap BETWEEN 0 AND 5),PRIMARY KEY(version,min_level));
CREATE TABLE IF NOT EXISTS game_loot_grades (version INTEGER REFERENCES game_loot_rule_versions(version),grade INTEGER CHECK(grade BETWEEN 0 AND 3),rune_rate_bp INTEGER NOT NULL CHECK(rune_rate_bp BETWEEN 0 AND 10000),socket_weights BIGINT[] NOT NULL CHECK(array_length(socket_weights,1)=6),PRIMARY KEY(version,grade));
CREATE TABLE IF NOT EXISTS game_rift_rule_versions (
 version INTEGER PRIMARY KEY CHECK(version>0), published BOOLEAN NOT NULL DEFAULT false,
 loot_version INTEGER NOT NULL,
 max_tier INTEGER NOT NULL,
 progress_max INTEGER NOT NULL,
 normal_progress INTEGER NOT NULL,
 globe_progress INTEGER NOT NULL,
 blue_orbs INTEGER NOT NULL,
 gold_orbs INTEGER NOT NULL,
 timed_ms BIGINT NOT NULL,
 hard_ms BIGINT NOT NULL,
 reconnect_ms BIGINT NOT NULL,
 death_penalty_ms BIGINT NOT NULL,
 presence_bp INTEGER NOT NULL,
 tier_jump_seconds INTEGER NOT NULL,
 hp_growth_bp INTEGER NOT NULL,
 damage_growth_bp INTEGER NOT NULL,
 party_hpbp INTEGER NOT NULL,
 equipment INTEGER NOT NULL,
 runes INTEGER NOT NULL,
 dust_base INTEGER NOT NULL,
 dust_tier_divisor INTEGER NOT NULL,
 unique_base_chance INTEGER NOT NULL,
 unique_per_tier INTEGER NOT NULL,
 unique_cap INTEGER NOT NULL,
 guaranteed_unique_tier INTEGER NOT NULL,
 timely_equipment INTEGER NOT NULL,
 timely_runes INTEGER NOT NULL,
 timely_dust INTEGER NOT NULL,
 experience_divisor INTEGER NOT NULL,
 dust_item INTEGER NOT NULL,
 name TEXT NOT NULL,
 boss_name TEXT NOT NULL,
 exit_name TEXT NOT NULL,
 exit_greeting TEXT NOT NULL,
 exit_sprite TEXT NOT NULL,
 exit_portrait TEXT NOT NULL,
 npc_model INTEGER NOT NULL,
 ability_max_concurrent INTEGER NOT NULL,
 ability_range INTEGER NOT NULL,
 ability_recovery_ms BIGINT NOT NULL,
 ability_start_ms BIGINT NOT NULL,
 ability_jitter_ms BIGINT NOT NULL,
 ability_reset_ms BIGINT NOT NULL,
 ability_unavailable_ms BIGINT NOT NULL,
 ability_cooldown_ms BIGINT NOT NULL,
 ability_class_reduction_ms BIGINT NOT NULL,
 ability_boss_damage_pct INTEGER NOT NULL,
 visual_skill INTEGER NOT NULL,
 visual_skill_level INTEGER NOT NULL,
 FOREIGN KEY(loot_version) REFERENCES game_loot_rule_versions(version),
 FOREIGN KEY(dust_item) REFERENCES game_items(id)
);
CREATE TABLE IF NOT EXISTS game_rift_rule_active (id INTEGER PRIMARY KEY CHECK(id=1), version INTEGER NOT NULL REFERENCES game_rift_rule_versions(version));
CREATE TABLE IF NOT EXISTS game_rift_class_rules (version INTEGER REFERENCES game_rift_rule_versions(version),class INTEGER CHECK(class BETWEEN 0 AND 3),hp_bp INTEGER NOT NULL CHECK(hp_bp>0),damage_bp INTEGER NOT NULL CHECK(damage_bp>0),PRIMARY KEY(version,class));
CREATE TABLE IF NOT EXISTS game_rift_ability_rules (version INTEGER REFERENCES game_rift_rule_versions(version),id INTEGER CHECK(id BETWEEN 0 AND 2),name TEXT NOT NULL,affix TEXT NOT NULL,warning TEXT NOT NULL,shape TEXT NOT NULL CHECK(shape IN ('circle','ring','line')),radius INTEGER NOT NULL,inner_radius INTEGER NOT NULL,length INTEGER NOT NULL,delay_ms BIGINT NOT NULL,damage_pct INTEGER NOT NULL,PRIMARY KEY(version,id));
DO $migration$
DECLARE l JSONB := $loot_rules$
{
  "Version": 1,
  "MinRuneLevel": 20,
  "UniqueBasePPB": 10000000,
  "UniqueMFBoost": 2,
  "UniqueMFDivisor": 250,
  "RunePool": [
    1900028001,
    1900028002,
    1900028003,
    1900028004,
    1900028005,
    1900028006,
    1900028007,
    1900028008,
    1900028009,
    1900028010,
    1900028011,
    1900028012,
    1900028013,
    1900028014,
    1900028015,
    1900028016,
    1900028017,
    1900028018,
    1900028019,
    1900028020,
    1900028021,
    1900028022,
    1900028023,
    1900028024,
    1900028025,
    1900028026,
    1900028027,
    1900028028
  ],
  "SocketLevels": [
    {
      "MinLevel": 10,
      "Cap": 2
    },
    {
      "MinLevel": 20,
      "Cap": 3
    },
    {
      "MinLevel": 40,
      "Cap": 4
    },
    {
      "MinLevel": 50,
      "Cap": 5
    }
  ],
  "Grades": [
    {
      "Grade": 0,
      "RuneRateBP": 60,
      "SocketWeights": [
        72,
        21,
        6,
        1,
        0,
        0
      ]
    },
    {
      "Grade": 1,
      "RuneRateBP": 800,
      "SocketWeights": [
        45,
        25,
        18,
        9,
        3,
        0
      ]
    },
    {
      "Grade": 2,
      "RuneRateBP": 2000,
      "SocketWeights": [
        25,
        25,
        23,
        17,
        8,
        2
      ]
    },
    {
      "Grade": 3,
      "RuneRateBP": 0,
      "SocketWeights": [
        15,
        20,
        25,
        22,
        13,
        5
      ]
    }
  ]
}
$loot_rules$::jsonb;
 r JSONB := $rift_rules$
{
  "Version": 1,
  "LootVersion": 1,
  "MaxTier": 150,
  "ProgressMax": 10000,
  "NormalProgress": 40,
  "GlobeProgress": 200,
  "BlueOrbs": 1,
  "GoldOrbs": 4,
  "TimedMS": 900000,
  "HardMS": 1200000,
  "ReconnectMS": 120000,
  "DeathPenaltyMS": 5000,
  "PresenceBP": 5000,
  "TierJumpSeconds": 180,
  "HPGrowthBP": 10800,
  "DamageGrowthBP": 10400,
  "PartyHPBP": 6000,
  "Equipment": 1,
  "Runes": 1,
  "DustBase": 6,
  "DustTierDivisor": 5,
  "UniqueBaseChance": 20,
  "UniquePerTier": 2,
  "UniqueCap": 60,
  "GuaranteedUniqueTier": 10,
  "TimelyEquipment": 1,
  "TimelyRunes": 1,
  "TimelyDust": 4,
  "ExperienceDivisor": 10,
  "DustItem": 1900028202,
  "Name": "演劫洞天",
  "BossName": "镇境劫影",
  "ExitName": "归元道使",
  "ExitGreeting": "劫影已散，尘缘未了。且收好洞天所得，贫道送诸位归还人间。",
  "ExitSprite": "npc630",
  "ExitPortrait": "N630",
  "NPCModel": 630,
  "AbilityMaxConcurrent": 4,
  "AbilityRange": 400,
  "AbilityRecoveryMS": 650,
  "AbilityStartMS": 3000,
  "AbilityJitterMS": 3000,
  "AbilityResetMS": 2000,
  "AbilityUnavailableMS": 3000,
  "AbilityCooldownMS": 11000,
  "AbilityClassReductionMS": 2000,
  "AbilityBossDamagePct": 20,
  "VisualSkill": 10402,
  "VisualSkillLevel": 1,
  "Classes": [
    {
      "Class": 0,
      "HPBP": 10000,
      "DamageBP": 10000
    },
    {
      "Class": 1,
      "HPBP": 40000,
      "DamageBP": 12000
    },
    {
      "Class": 2,
      "HPBP": 80000,
      "DamageBP": 14000
    },
    {
      "Class": 3,
      "HPBP": 400000,
      "DamageBP": 15000
    }
  ],
  "Abilities": [
    {
      "ID": 0,
      "Name": "烈焰爆裂",
      "Affix": "烈焰",
      "Warning": "烈焰将至，离开落点！",
      "Shape": "circle",
      "Radius": 105,
      "InnerRadius": 0,
      "Length": 0,
      "DelayMS": 1500,
      "DamagePct": 140
    },
    {
      "ID": 1,
      "Name": "震环冲击",
      "Affix": "震环",
      "Warning": "震环将至，贴近内圈或退出外圈！",
      "Shape": "ring",
      "Radius": 220,
      "InnerRadius": 85,
      "Length": 0,
      "DelayMS": 1800,
      "DamagePct": 160
    },
    {
      "ID": 2,
      "Name": "裂地斩",
      "Affix": "裂地",
      "Warning": "裂地将至，向两侧闪避！",
      "Shape": "line",
      "Radius": 38,
      "InnerRadius": 0,
      "Length": 380,
      "DelayMS": 1400,
      "DamagePct": 150
    }
  ]
}
$rift_rules$::jsonb;
BEGIN
 IF NOT EXISTS(SELECT 1 FROM game_loot_rule_versions WHERE version=1) THEN
 INSERT INTO game_loot_rule_versions(version,min_rune_level,unique_base_ppb,unique_mf_boost,unique_mf_divisor) VALUES ((l->>'Version')::integer,(l->>'MinRuneLevel')::integer,(l->>'UniqueBasePPB')::bigint,(l->>'UniqueMFBoost')::bigint,(l->>'UniqueMFDivisor')::bigint);
 INSERT INTO game_loot_rune_pool SELECT 1,ordinality::int,value::text::int FROM jsonb_array_elements(l->'RunePool') WITH ORDINALITY;
 INSERT INTO game_loot_socket_levels SELECT 1,(v->>'MinLevel')::int,(v->>'Cap')::int FROM jsonb_array_elements(l->'SocketLevels') v;
 INSERT INTO game_loot_grades SELECT 1,(v->>'Grade')::int,(v->>'RuneRateBP')::int,ARRAY(SELECT x::text::bigint FROM jsonb_array_elements(v->'SocketWeights') x) FROM jsonb_array_elements(l->'Grades') v;
 UPDATE game_loot_rule_versions SET published=true WHERE version=1;
 END IF;
 IF NOT EXISTS(SELECT 1 FROM game_rift_rule_versions WHERE version=1) THEN
 INSERT INTO game_rift_rule_versions(version,loot_version,max_tier,progress_max,normal_progress,globe_progress,blue_orbs,gold_orbs,timed_ms,hard_ms,reconnect_ms,death_penalty_ms,presence_bp,tier_jump_seconds,hp_growth_bp,damage_growth_bp,party_hpbp,equipment,runes,dust_base,dust_tier_divisor,unique_base_chance,unique_per_tier,unique_cap,guaranteed_unique_tier,timely_equipment,timely_runes,timely_dust,experience_divisor,dust_item,name,boss_name,exit_name,exit_greeting,exit_sprite,exit_portrait,npc_model,ability_max_concurrent,ability_range,ability_recovery_ms,ability_start_ms,ability_jitter_ms,ability_reset_ms,ability_unavailable_ms,ability_cooldown_ms,ability_class_reduction_ms,ability_boss_damage_pct,visual_skill,visual_skill_level) VALUES ((r->>'Version')::integer,(r->>'LootVersion')::integer,(r->>'MaxTier')::integer,(r->>'ProgressMax')::integer,(r->>'NormalProgress')::integer,(r->>'GlobeProgress')::integer,(r->>'BlueOrbs')::integer,(r->>'GoldOrbs')::integer,(r->>'TimedMS')::bigint,(r->>'HardMS')::bigint,(r->>'ReconnectMS')::bigint,(r->>'DeathPenaltyMS')::bigint,(r->>'PresenceBP')::integer,(r->>'TierJumpSeconds')::integer,(r->>'HPGrowthBP')::integer,(r->>'DamageGrowthBP')::integer,(r->>'PartyHPBP')::integer,(r->>'Equipment')::integer,(r->>'Runes')::integer,(r->>'DustBase')::integer,(r->>'DustTierDivisor')::integer,(r->>'UniqueBaseChance')::integer,(r->>'UniquePerTier')::integer,(r->>'UniqueCap')::integer,(r->>'GuaranteedUniqueTier')::integer,(r->>'TimelyEquipment')::integer,(r->>'TimelyRunes')::integer,(r->>'TimelyDust')::integer,(r->>'ExperienceDivisor')::integer,(r->>'DustItem')::integer,r->>'Name',r->>'BossName',r->>'ExitName',r->>'ExitGreeting',r->>'ExitSprite',r->>'ExitPortrait',(r->>'NPCModel')::integer,(r->>'AbilityMaxConcurrent')::integer,(r->>'AbilityRange')::integer,(r->>'AbilityRecoveryMS')::bigint,(r->>'AbilityStartMS')::bigint,(r->>'AbilityJitterMS')::bigint,(r->>'AbilityResetMS')::bigint,(r->>'AbilityUnavailableMS')::bigint,(r->>'AbilityCooldownMS')::bigint,(r->>'AbilityClassReductionMS')::bigint,(r->>'AbilityBossDamagePct')::integer,(r->>'VisualSkill')::integer,(r->>'VisualSkillLevel')::integer);
 INSERT INTO game_rift_class_rules SELECT 1,(v->>'Class')::int,(v->>'HPBP')::int,(v->>'DamageBP')::int FROM jsonb_array_elements(r->'Classes') v;
 INSERT INTO game_rift_ability_rules SELECT 1,(v->>'ID')::int,v->>'Name',v->>'Affix',v->>'Warning',v->>'Shape',(v->>'Radius')::int,(v->>'InnerRadius')::int,(v->>'Length')::int,(v->>'DelayMS')::bigint,(v->>'DamagePct')::int FROM jsonb_array_elements(r->'Abilities') v;
 UPDATE game_rift_rule_versions SET published=true WHERE version=1;
 END IF;
 INSERT INTO game_loot_rule_active VALUES(1,1) ON CONFLICT(id) DO NOTHING;
 INSERT INTO game_rift_rule_active VALUES(1,1) ON CONFLICT(id) DO NOTHING;
END $migration$;
CREATE OR REPLACE FUNCTION guard_published_endgame_rules() RETURNS trigger LANGUAGE plpgsql AS $guard$
DECLARE v INTEGER; frozen BOOLEAN;
BEGIN
 v := CASE WHEN TG_OP='DELETE' THEN OLD.version ELSE NEW.version END;
 IF TG_TABLE_NAME IN ('game_loot_rule_versions','game_rift_rule_versions') THEN
  IF TG_OP<>'INSERT' AND OLD.published THEN RAISE EXCEPTION 'Published rule versions are immutable; create a new version'; END IF;
 ELSE
  IF TG_TABLE_NAME LIKE 'game_loot_%' THEN SELECT published INTO frozen FROM game_loot_rule_versions WHERE version=v;
  ELSE SELECT published INTO frozen FROM game_rift_rule_versions WHERE version=v; END IF;
  IF frozen THEN RAISE EXCEPTION 'Published rule children are immutable; create a new version'; END IF;
  IF TG_OP='UPDATE' AND OLD.version<>NEW.version THEN RAISE EXCEPTION 'Cannot move rule rows between versions'; END IF;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
END $guard$;
DROP TRIGGER IF EXISTS immutable_published_rules ON game_loot_rule_versions;
CREATE TRIGGER immutable_published_rules BEFORE INSERT OR UPDATE OR DELETE ON game_loot_rule_versions FOR EACH ROW EXECUTE FUNCTION guard_published_endgame_rules();
DROP TRIGGER IF EXISTS immutable_published_rules ON game_loot_rune_pool;
CREATE TRIGGER immutable_published_rules BEFORE INSERT OR UPDATE OR DELETE ON game_loot_rune_pool FOR EACH ROW EXECUTE FUNCTION guard_published_endgame_rules();
DROP TRIGGER IF EXISTS immutable_published_rules ON game_loot_socket_levels;
CREATE TRIGGER immutable_published_rules BEFORE INSERT OR UPDATE OR DELETE ON game_loot_socket_levels FOR EACH ROW EXECUTE FUNCTION guard_published_endgame_rules();
DROP TRIGGER IF EXISTS immutable_published_rules ON game_loot_grades;
CREATE TRIGGER immutable_published_rules BEFORE INSERT OR UPDATE OR DELETE ON game_loot_grades FOR EACH ROW EXECUTE FUNCTION guard_published_endgame_rules();
DROP TRIGGER IF EXISTS immutable_published_rules ON game_rift_rule_versions;
CREATE TRIGGER immutable_published_rules BEFORE INSERT OR UPDATE OR DELETE ON game_rift_rule_versions FOR EACH ROW EXECUTE FUNCTION guard_published_endgame_rules();
DROP TRIGGER IF EXISTS immutable_published_rules ON game_rift_class_rules;
CREATE TRIGGER immutable_published_rules BEFORE INSERT OR UPDATE OR DELETE ON game_rift_class_rules FOR EACH ROW EXECUTE FUNCTION guard_published_endgame_rules();
DROP TRIGGER IF EXISTS immutable_published_rules ON game_rift_ability_rules;
CREATE TRIGGER immutable_published_rules BEFORE INSERT OR UPDATE OR DELETE ON game_rift_ability_rules FOR EACH ROW EXECUTE FUNCTION guard_published_endgame_rules();
