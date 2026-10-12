-- V5 battle skill allocation reset. Incremental and idempotent; no rank multiplication.
-- A before-image remains available for a human-readable old build and audit.
ALTER TABLE characters ADD COLUMN IF NOT EXISTS skill_allocation_version INTEGER NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS character_skill_migrations (
 char_id BIGINT NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
 version INTEGER NOT NULL,
 before_image JSONB NOT NULL,
 before_checksum TEXT NOT NULL,
 earned_points INTEGER NOT NULL,
 applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(char_id, version)
);
DO $$
DECLARE invalid_count BIGINT;
BEGIN
 SELECT count(*) INTO invalid_count FROM characters
 WHERE skill_allocation_version NOT IN (0,5) OR (skill_allocation_version=0 AND (level<1 OR level>100 OR skill_points<0));
 IF invalid_count>0 THEN RAISE EXCEPTION 'V5 skill migration blocked: % invalid character budgets/versions; audit before retry',invalid_count; END IF;
 SELECT count(*) INTO invalid_count FROM character_skills s JOIN characters c ON c.id=s.char_id
 WHERE c.skill_allocation_version=0 AND s.skill_id<>11005 AND s.skill_id BETWEEN 10001 AND 10435
 AND (s.level<0 OR s.level>5 OR (s.skill_id/100-100)<>c.race);
 IF invalid_count>0 THEN RAISE EXCEPTION 'V5 skill migration blocked: % invalid or cross-profession legacy ranks',invalid_count; END IF;
 SELECT count(*) INTO invalid_count FROM characters c WHERE c.skill_allocation_version=0 AND NOT c.employed
 AND COALESCE((SELECT SUM(GREATEST(s.level,0)) FROM character_skills s WHERE s.char_id=c.id AND s.skill_id IN(13601,13602)),0)>GREATEST(0,c.level-1);
 IF invalid_count>0 THEN RAISE EXCEPTION 'V5 skill migration blocked: % starter allocations exceed earned points',invalid_count; END IF;
END $$;
INSERT INTO character_skill_migrations(char_id,version,before_image,before_checksum,earned_points)
SELECT c.id,5,j.image,md5(j.image::text),GREATEST(0,LEAST(c.level,100)-1)
FROM characters c CROSS JOIN LATERAL (
 SELECT jsonb_build_object('level',c.level,'unspent',c.skill_points,'employed',c.employed,'skills',
 COALESCE((SELECT jsonb_agg(jsonb_build_object('id',s.skill_id,'rank',s.level) ORDER BY s.skill_id) FROM character_skills s WHERE s.char_id=c.id),'[]'::jsonb)) AS image
) j WHERE c.skill_allocation_version=0 ON CONFLICT(char_id,version) DO NOTHING;
DELETE FROM character_skills s USING characters c
WHERE c.id=s.char_id AND c.skill_allocation_version=0
AND ((s.skill_id BETWEEN 10001 AND 10435) OR (c.employed AND s.skill_id IN(13601,13602)));
UPDATE characters c SET skill_points=GREATEST(0,LEAST(c.level,100)-1)-
 CASE WHEN NOT c.employed THEN COALESCE((SELECT SUM(GREATEST(s.level,0)) FROM character_skills s WHERE s.char_id=c.id AND s.skill_id IN(13601,13602)),0) ELSE 0 END,
 skill_allocation_version=5 WHERE c.skill_allocation_version=0;
ALTER TABLE characters ALTER COLUMN skill_allocation_version SET DEFAULT 5;
COMMENT ON COLUMN characters.skill_allocation_version IS '5: level-derived battle hard-point budget; equipment ranks are never persisted here';
