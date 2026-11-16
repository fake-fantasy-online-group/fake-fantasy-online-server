-- Existing databases: apply atomically with a separately authorized maintenance process.
-- Schema only: no player data is read or changed. Idempotent (IF NOT EXISTS).
BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='60s';

-- 000131 client_log_reports: index of player /report client-log uploads.
-- Files live under the gameserver -client-log-dir; rows older than 14 days are
-- deleted by the gameserver retention task together with their files.
CREATE TABLE IF NOT EXISTS client_log_reports (
 id bigserial PRIMARY KEY,
 report_no text NOT NULL UNIQUE,
 account_id bigint NOT NULL,
 char_id bigint NOT NULL,
 char_name text NOT NULL,
 map_id integer NOT NULL DEFAULT 0,
 client_version text NOT NULL DEFAULT '',
 platform text NOT NULL DEFAULT '',
 level text NOT NULL CHECK (level IN ('error','warn','info')),
 range_minutes integer NOT NULL CHECK (range_minutes BETWEEN 1 AND 1440),
 note text NOT NULL DEFAULT '',
 size_bytes bigint NOT NULL CHECK (size_bytes BETWEEN 1 AND 2097152),
 truncated boolean NOT NULL DEFAULT false,
 sha256 text NOT NULL,
 file_path text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS client_log_reports_account_created_idx ON client_log_reports(account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS client_log_reports_char_created_idx ON client_log_reports(char_name, created_at DESC);
CREATE INDEX IF NOT EXISTS client_log_reports_created_idx ON client_log_reports(created_at);

COMMIT;
