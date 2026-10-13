-- User decision 2026-10-13: Gangqi Sword (10102) knockback uses the existing
-- Thunder Bomb (10223) rule. This is an explicit project rule, not a claim that
-- Thunder Bomb's configured 100 world units were recovered from the old client.
-- Preserve Gangqi's native damage, text, costs, range and five authored ranks.
-- Copy the donor's per-rank knockback through the existing generic effect path;
-- no skill-ID runtime branch or protocol extension is needed.
-- Backstab's approved 100% base stun chance is NOT activated here: its stun
-- duration is still unspecified, and an arbitrary existing status level is not
-- evidence of the original duration. See the accompanying rules report.
DO $gangqi_knockback$
BEGIN
 INSERT INTO game_skill_mechanism_sources(skill_id,mechanic,source_url,source_revision,imported)
  VALUES(10102,'knockback','server/migrations/000000_init-33.sql',
   'User decision 2026-10-13: same as existing Thunder Bomb 10223; original damage retained',false)
  ON CONFLICT DO NOTHING;
 IF NOT FOUND THEN RETURN; END IF;
 -- Any existing explicit effects belong to the administrator. Do not merge an
 -- incomplete automatic effect chain into those settings or overwrite them.
 IF EXISTS(SELECT FROM game_skill_effects WHERE skill_id=10102) THEN RETURN; END IF;
 IF (SELECT count(*) FROM gamedata.ov_skilldesc
     WHERE skill_id=10102 AND skill_level BETWEEN 1 AND 5 AND prof=2
       AND skill_type=1 AND desc_=format('造成%s%%伤害，震退敌人。',127+3*skill_level))<>5 THEN
  RAISE EXCEPTION 'missing or changed original Gangqi damage anchors';
 END IF;
 IF (SELECT count(*) FROM game_skill_effects
     WHERE skill_id=10223 AND skill_level BETWEEN 1 AND 5
       AND effect_kind='knockback')<>5 OR
    (SELECT count(DISTINCT skill_level) FROM game_skill_effects
     WHERE skill_id=10223 AND skill_level BETWEEN 1 AND 5
       AND effect_kind='knockback' AND flat_value>0 AND chance_bp=10000
       AND power_pct=0 AND max_resource_pct=0 AND NOT guaranteed_hit
       AND effect_target='target' AND requires_previous_landed
       AND weapon_durability_pct=0 AND loss_refund_pct=0)<>5 THEN
  RAISE EXCEPTION 'missing or invalid Thunder Bomb knockback anchors';
 END IF;
 INSERT INTO game_skill_effects(skill_id,skill_level,effect_order,effect_kind,power_pct)
  SELECT 10102,lv,1,'damage',127+3*lv FROM generate_series(1,5) lv;
 INSERT INTO game_skill_effects(skill_id,skill_level,effect_order,effect_kind,
   flat_value,chance_bp,effect_target,requires_previous_landed)
  SELECT 10102,skill_level,2,effect_kind,flat_value,chance_bp,effect_target,requires_previous_landed
  FROM game_skill_effects
  WHERE skill_id=10223 AND skill_level BETWEEN 1 AND 5 AND effect_kind='knockback';
 UPDATE game_skill_mechanism_sources SET imported=true
  WHERE skill_id=10102 AND mechanic='knockback';
END
$gangqi_knockback$;
