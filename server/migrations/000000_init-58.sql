-- Effect-contest ratings are not probabilities: resistance and vulnerability
-- continue above 100. Do not modify historical migrations or source artwork.
ALTER TABLE game_skill_passive_status_resists
  DROP CONSTRAINT IF EXISTS game_skill_passive_status_resists_resist_pct_check;
ALTER TABLE game_skill_passive_status_resists
  ADD CONSTRAINT game_skill_passive_status_resists_resist_pct_check CHECK (resist_pct > 0);
ALTER TABLE game_custom_statuses
  DROP CONSTRAINT IF EXISTS game_custom_statuses_physical_damage_taken_pct_check,
  DROP CONSTRAINT IF EXISTS game_custom_statuses_magic_damage_taken_pct_check;
ALTER TABLE game_custom_statuses
  ADD CONSTRAINT game_custom_statuses_physical_damage_taken_pct_check CHECK (physical_damage_taken_pct >= 0),
  ADD CONSTRAINT game_custom_statuses_magic_damage_taken_pct_check CHECK (magic_damage_taken_pct >= 0);
-- Stored chances are explicit, including zero. The application model retains
-- a separate legacy-omitted sentinel only for old saves/in-memory definitions.
ALTER TABLE game_skill_statuses DROP CONSTRAINT IF EXISTS game_skill_statuses_chance_bp_check;
ALTER TABLE game_skill_statuses ADD CONSTRAINT game_skill_statuses_chance_bp_check CHECK (chance_bp BETWEEN 0 AND 10000);
ALTER TABLE game_skill_passive_trigger_statuses DROP CONSTRAINT IF EXISTS game_skill_passive_trigger_statuses_chance_bp_check;
ALTER TABLE game_skill_passive_trigger_statuses ADD CONSTRAINT game_skill_passive_trigger_statuses_chance_bp_check CHECK (chance_bp BETWEEN 0 AND 10000);
ALTER TABLE game_monster_skill_effects DROP CONSTRAINT IF EXISTS game_monster_skill_effects_chance_bp_check;
ALTER TABLE game_monster_skill_effects ADD CONSTRAINT game_monster_skill_effects_chance_bp_check CHECK (chance_bp BETWEEN 0 AND 10000);
