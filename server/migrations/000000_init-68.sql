-- Restore the three original area radii whose imported numeric field conflicts
-- with the unchanged original description. This resolves project data conflicts;
-- it does not assert a measured Tencent client/runtime version.
-- War Cry / Taunt: original text and Tencent's old class guides, radius 100.
-- Old group Heaven's Punishment: original text and the matching old FF table,
-- radius 150; do not substitute the later single-target skill's parameters.
-- Existing range-growth policy continues from the corrected rank-five radius.
-- Reuse migration66's source ledger; never overwrite administrator values or
-- reapply a correction after its first import, even if a value later becomes 200.
DO $original_skill_radii$
DECLARE
 correction RECORD;
 changed INTEGER;
BEGIN
 FOR correction IN SELECT * FROM (VALUES
   (10028,100,'https://ffo.qq.com/new/newerData/zs.htm',
    'Original War Cry description + Tencent old warrior guide; checked 2026-10-13; guide version undated'),
   (10127,100,'https://ffo.qq.com/new/newerData/jk.htm',
    'Original Taunt description + Tencent old swordsman guide; checked 2026-10-13; guide version undated'),
   (10313,150,'https://ffo.17173.com/information/2_1%2C6.shtml',
    'Original level70 group Heaven''s Punishment description + matching old FF table; checked 2026-10-13; release version unverified')
 ) AS corrections(skill_id,radius,source_url,source_revision) LOOP
  INSERT INTO game_skill_mechanism_sources(skill_id,mechanic,source_url,source_revision,imported)
   VALUES(correction.skill_id,'original_radius',correction.source_url,correction.source_revision,false)
   ON CONFLICT DO NOTHING;
  IF NOT FOUND THEN CONTINUE; END IF;
  IF (SELECT count(*) FROM gamedata.ov_skilldesc
      WHERE skill_id=correction.skill_id AND skill_level BETWEEN 1 AND 5) <> 5 THEN
   RAISE EXCEPTION 'missing original radius anchors for skill %',correction.skill_id;
  END IF;
  UPDATE gamedata.ov_skilldesc SET radius=correction.radius
   WHERE skill_id=correction.skill_id AND skill_level BETWEEN 1 AND 5 AND radius=200;
  GET DIAGNOSTICS changed=ROW_COUNT;
  UPDATE game_skill_mechanism_sources SET imported=changed>0
   WHERE skill_id=correction.skill_id AND mechanic='original_radius';
 END LOOP;
END
$original_skill_radii$;
