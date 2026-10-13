-- Close the previously unresolved theft identity, not the imported Youlong
-- damage placeholder. The 50/56/62/68/74% gate is supported by the old local
-- description and 2006 QQ Fantasy records; the user selected the monster's
-- existing drop pool on 2026-10-13. Relative-weight selection, one item per
-- success, a shared one-success-per-monster-life limit, zero damage, and ground
-- delivery are explicit project rules, not newly authenticated historic facts.
-- Keep mana, cooldown, cast time, distance, prerequisites, and icon untouched.
-- After the probability reaches its bound at rank10, use the already-approved
-- post-cap mana/cooldown continuation. Only the untouched all-zero policy is
-- eligible; original ranks1..10 and administrator policies remain unchanged.
DO $monster_loot_theft$
BEGIN
 INSERT INTO game_skill_mechanism_sources(skill_id,mechanic,source_url,source_revision,imported)
 VALUES(10212,'monster_loot_theft',
  'https://fo.17173.com/character/character_3.html',
  'Old local overview plus 2006 QQ Fantasy sources: gate 50% +6 points/rank. User 2026-10-13: pick one from monster drop pool. Lifecycle, weights, zero damage, ground delivery and approved post-cap resource growth at rank10 are project rules.',false)
 ON CONFLICT DO NOTHING;
 IF NOT FOUND THEN RETURN; END IF;

 -- Import only the known unresolved five-rank placeholder as one unit. A
 -- previously customized row/effect/status is preserved, including on replay.
 IF EXISTS (SELECT 1 FROM game_skill_effects WHERE skill_id=10212)
 OR EXISTS (SELECT 1 FROM game_skill_statuses WHERE skill_id=10212)
 OR (SELECT count(*) FROM gamedata.ov_skilldesc
      WHERE skill_id=10212 AND skill_level BETWEEN 1 AND 5
       AND name='游龙掌'
       AND desc_='造成'||(100+(skill_level-1)*3)::text||'%的伤害。') <> 5
 THEN RETURN; END IF;

 INSERT INTO game_skill_effects
  (skill_id,skill_level,effect_order,effect_kind,flat_value,chance_bp)
 SELECT 10212,rank,1,'steal_item',1,5000+(rank-1)*600
 FROM generate_series(1,5) rank;

 UPDATE gamedata.ov_skilldesc
 SET name='偷窃术',
     desc_='有'||(50+(skill_level-1)*6)::text||'%的几率从目标身上偷取一件物品。每只怪物仅可成功偷取一次。'
 WHERE skill_id=10212 AND skill_level BETWEEN 1 AND 5;

 UPDATE game_skill_growth_policy SET efficiency_cap_rank=10
 WHERE skill_id=10212 AND attribute=0 AND attribute_cap_rank=0
   AND efficiency_cap_rank=0 AND range_cap_rank=0 AND protection_cap_rank=0
   AND attack_cap_rank=0 AND cardinality(attack_targets)=0;

 UPDATE game_skill_mechanism_sources SET imported=true
 WHERE skill_id=10212 AND mechanic='monster_loot_theft';
END
$monster_loot_theft$;
