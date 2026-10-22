-- Empty databases have no pre-release enhanced sets. Mark this one-time
-- conversion complete now, so future ordinary drops are never converted.
CREATE TABLE IF NOT EXISTS class_set_true_conversion (
 migration_key TEXT PRIMARY KEY CHECK (migration_key = 'class-set-true-20261022'),
 converted_instances BIGINT NOT NULL CHECK (converted_instances >= 0),
 completed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO class_set_true_conversion(migration_key,converted_instances) VALUES ('class-set-true-20261022',0) ON CONFLICT DO NOTHING;
