-- Durable outcomes for server-owned mail send/claim operations. Retain receipts
-- across process restarts; do not expire them while operation IDs can be retried.
CREATE TABLE IF NOT EXISTS mail_operation_receipts (
 operation_id VARCHAR(32) PRIMARY KEY,
 char_id BIGINT NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
 kind TEXT NOT NULL CHECK (kind IN ('send','claim')),
 digest VARCHAR(64) NOT NULL,
 committed BOOLEAN NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS mail_operation_receipts_char_idx ON mail_operation_receipts(char_id);
