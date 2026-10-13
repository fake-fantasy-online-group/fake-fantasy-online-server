-- Preserve fractional high-rank skill gains across saves and reconnects.
-- Existing characters begin with no prepaid credit. Replays never reset it.
ALTER TABLE characters
  ADD COLUMN IF NOT EXISTS skill_rounding_credits JSONB NOT NULL DEFAULT '{}'::jsonb;

DO $skill_rounding$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='characters'::regclass
      AND conname='characters_skill_rounding_object'
  ) THEN
    ALTER TABLE characters ADD CONSTRAINT characters_skill_rounding_object
      CHECK (jsonb_typeof(skill_rounding_credits) = 'object');
  END IF;
END
$skill_rounding$;

COMMENT ON COLUMN characters.skill_rounding_credits IS
  'Per-skill fractional mana/cooldown credit; n/d already paid, 0 <= n < d; validated by the domain on load and save.';
