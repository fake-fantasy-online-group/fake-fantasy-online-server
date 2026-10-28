-- Nine dynamic equipment spirits. Ring souls are deliberately unchanged.
-- Existing fixed FusedSoul IDs remain valid; no old item is rerolled on migration.
ALTER TABLE item_instances DROP CONSTRAINT IF EXISTS item_instances_instance_kind_check;
ALTER TABLE item_instances ADD CONSTRAINT item_instances_instance_kind_check CHECK(instance_kind IN (1,2,3,4));
CREATE TABLE IF NOT EXISTS spirit_instances (
 uid BIGINT PRIMARY KEY REFERENCES item_instances(uid) ON DELETE CASCADE,
 state JSONB NOT NULL CHECK(jsonb_typeof(state)='object' AND octet_length(state::text)<8192)
);
CREATE TABLE IF NOT EXISTS equipment_spirit_fusions (
 equipment_uid BIGINT PRIMARY KEY REFERENCES item_instances(uid) ON DELETE CASCADE,
 spirit_uid BIGINT UNIQUE REFERENCES spirit_instances(uid),
 appearance_item_id INT NOT NULL DEFAULT 0 CHECK(appearance_item_id>=0),
 CHECK(spirit_uid IS NULL OR spirit_uid<>equipment_uid),
 CHECK(spirit_uid IS NOT NULL OR appearance_item_id>0)
);
