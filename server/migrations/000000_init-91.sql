-- Explicit species alliances for assistance only. No broad Family/Kind/model inference.
-- Empty is deliberately conservative: same monster template ID only.
ALTER TABLE game_monsters ADD COLUMN IF NOT EXISTS help_group TEXT NOT NULL DEFAULT '';
COMMENT ON COLUMN game_monsters.help_group IS 'Reviewed assistance species group; empty means exact monster template only. Never infer from combat family, rank, sprite, or name.';
