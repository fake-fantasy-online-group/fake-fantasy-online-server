-- Additive legendary instance facts. No changes to legacy five-affix arrays,
-- native base/drop tables, dyes, or historical unique generations.
CREATE TABLE IF NOT EXISTS equipment_legendary_state (
 uid BIGINT PRIMARY KEY REFERENCES item_instances(uid) ON DELETE CASCADE,
 state JSONB NOT NULL CHECK (jsonb_typeof(state)='object' AND length(state->>'PowerID')>0 AND jsonb_array_length(state->'Affixes')=6 AND jsonb_array_length(state->'Bounds')=6)
);
