-- Permanent trial qualification and acceptance-bounded rift quest evidence.
ALTER TABLE characters ADD COLUMN IF NOT EXISTS trial_completed_once BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE character_quests ADD COLUMN IF NOT EXISTS accepted_at_ms BIGINT NOT NULL DEFAULT 0 CHECK (accepted_at_ms >= 0);
ALTER TABLE character_quests ADD COLUMN IF NOT EXISTS rift_completion_run_id TEXT NOT NULL DEFAULT '' CHECK (length(rift_completion_run_id) <= 64);

-- Preserve provable legacy completions, including a completed trial that was
-- replaced by a new active repeatable quest. Never infer from keys/inventory,
-- a mere admission, an uncompleted run, or elapsed wall time.
UPDATE characters c SET trial_completed_once = TRUE
WHERE NOT c.trial_completed_once AND (
    c.trial_daily_completed > 0 OR c.trial_cycle > 0
    OR EXISTS (
        SELECT 1 FROM character_quests q JOIN game_trial_rules r ON r.quest_id = q.task_id
        WHERE q.char_id = c.id AND q.state = 2
    )
    OR EXISTS (
        SELECT 1 FROM endgame_runs r
        WHERE r.state = 'trial.completed'
          AND r.payload->>'Kind' = 'trial'
          AND r.payload->>'Version' IN ('1', '2')
          AND r.payload->'Keys'->>c.id::text ~ '^[1-9][0-9]*$'
          AND (r.payload->>'Version' = '1' OR jsonb_typeof(r.payload->'Grants'->c.id::text) = 'object')
    )
);
