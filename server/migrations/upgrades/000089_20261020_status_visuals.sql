-- Incremental upgrade 89, 2026-10-20: fill only missing status visuals.
-- Execute this file only with psql -v ON_ERROR_STOP=1, after an authorized
-- backup/maintenance check. Never run the full init set on an existing DB.
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
-- Status visual mappings, 2026-10-20. Presentation only: no status identity,
-- rank, icon, description, mechanics, duration, player data or resource changes.
-- The explicit existing-database counterpart is
-- upgrades/000089_20261020_status_visuals.sql. Do not replay all init fragments.
CREATE TABLE IF NOT EXISTS game_status_visuals (
    status_id INTEGER NOT NULL,
    expected_name TEXT NOT NULL DEFAULT '',
    effect_name TEXT NOT NULL DEFAULT '',
    use_effect_name_for_self BOOLEAN NOT NULL DEFAULT FALSE,
    position_mode SMALLINT NOT NULL DEFAULT -1 CHECK (position_mode BETWEEN -1 AND 3),
    PRIMARY KEY (status_id, expected_name)
);

COMMENT ON TABLE game_status_visuals IS
    'Optional exact status ID/name presentation mapping; absent, empty or unmatched mappings retain legacy projection.';
COMMENT ON COLUMN game_status_visuals.expected_name IS
    'Exact runtime status name, including rank-specific names; never a wildcard.';
COMMENT ON COLUMN game_status_visuals.effect_name IS
    'Existing client effect resource key; empty means inactive, not hide the original status.';
COMMENT ON COLUMN game_status_visuals.use_effect_name_for_self IS
    'Use effect_name for the legacy self Buff title/aura field; false retains the original title.';
COMMENT ON COLUMN game_status_visuals.position_mode IS
    '-1 inherits legacy positioning; 0..3 explicitly select the client anchor.';

-- Keep the identity assertions and the compare-and-set stable. The upgrade
-- wrapper bounds lock and statement waits; the empty-db initializer owns its
-- transaction. No source table is modified.
LOCK TABLE gamedata.ov_exceptdesc, gamedata.ov_exceptdetail,
    gamedata.ov_exceptdetail_entry, game_custom_statuses,
    game_rift_rule_versions IN SHARE MODE;
LOCK TABLE game_status_visuals IN SHARE ROW EXCLUSIVE MODE;

DO $status_visual_seed$
DECLARE
    seed RECORD;
    actual_ranks INTEGER[];
    actual_versions INTEGER[];
    previous game_status_visuals%ROWTYPE;
    current_value game_status_visuals%ROWTYPE;
    had_previous BOOLEAN;
    fillable BOOLEAN;
    changed INTEGER;
    filled INTEGER := 0;
    checked INTEGER := 0;
BEGIN
    FOR seed IN
        SELECT * FROM (VALUES
            -- Existing, live client aliases. Preserve self HUD titles AND legacy
            -- anchors (-1); in particular 春风 keeps its original orbit anchor.
            -- Source: ov_exceptdesc + positive-duration ov_exceptdetail_entry.
            (1023, '补充生命', '补血', FALSE, -1, 'native', ARRAY[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21]),
            (1024, '补充法力', '补魔法', FALSE, -1, 'native', ARRAY[1,2,3,4,5,6,7,8,9,10,11]),
            (1028, '瞬发', '鹰眼', FALSE, -1, 'native', ARRAY[1,2,3,4,5]),
            (1036, '燃烧', '燃烧1', FALSE, -1, 'native', ARRAY[1,2,3,4,5,6]),
            (1043, '春风', '补血', FALSE, -1, 'native', ARRAY[1,2,3]),

            -- Approved missing native states. Same-ID differently named ranks
            -- (notably 1154/8 and 1178/3..6) are intentionally not mapped.
            (1018, '使毒', 'TFX045_30', TRUE, 3, 'native', ARRAY[1,2,3,4,5,6,7,8,9,10]),
            (1152, '完全回避', '钢铁', TRUE, 1, 'native', ARRAY[1,2,3,4,5,6]),
            (1154, '物抗上升', '防御增加', TRUE, 1, 'native', ARRAY[1,2,3,4,5,6,7]),
            (1178, '金甲之佑', '钢铁', TRUE, 1, 'native', ARRAY[1,2]),
            (1208, '大地状态', '地之盾', TRUE, 1, 'native', ARRAY[1,2]),

            -- Approved custom statuses. Source: game_custom_statuses seeded
            -- by init-32/33 and restored in init-62.
            -- 衰弱 is the original blue weak glyph, NOT 防御下降.
            (10009, '破防之枪', '衰弱', TRUE, 1, 'custom', ARRAY[1,2,3,4,5]),
            (10012, '破甲', '降幅', TRUE, 1, 'custom', ARRAY[1,2,3,4,5]),
            (10018, '冲锋', '加速', TRUE, 0, 'custom', ARRAY[1,2,3,4,5]),
            (10019, '全力投掷', '减速', TRUE, 0, 'custom', ARRAY[1,2,3,4,5]),
            (10111, '移动受限', '金缚', TRUE, 1, 'custom', ARRAY[1,2,3,4,5]),
            (10115, '火字诀', '强化', TRUE, 1, 'custom', ARRAY[1,2,3,4,5]),
            (10129, '致残', '衰弱', TRUE, 1, 'custom', ARRAY[1,2,3,4,5]),
            (10303, '回春术', '补血', TRUE, 0, 'custom', ARRAY[1,2,3,4,5]),
            (10321, '防御术', '钢铁', TRUE, 1, 'custom', ARRAY[1,2,3,4,5]),
            (10327, '现形', '鹰眼', TRUE, 0, 'custom', ARRAY[1,2,3,4,5]),
            (10427, '冰系掌握', '减速', TRUE, 0, 'custom', ARRAY[1,2,3,4,5]),
            (12034, '冲锋', '加速', TRUE, 0, 'custom', ARRAY[1,2,3,4,5,6]),
            (12050, '破魂之墙', '伤害吸收', TRUE, 0, 'custom', ARRAY[1]),
            (12086, '魔法免疫', '伤害吸收', TRUE, 0, 'custom', ARRAY[1,5]),
            (12524, '冰火双盾', '伤害吸收', TRUE, 0, 'custom', ARRAY[1]),

            -- Runtime-only rank-1 identities in applyRiftEliteBuff; their names
            -- come from immutable published rift6/7 elite_rules, not a skill row.
            (1900101, '金身', '钢铁', TRUE, 1, 'rift', ARRAY[1]),
            (1900102, '敏捷', '加速', TRUE, 0, 'rift', ARRAY[1])
        ) AS approved(status_id, expected_name, effect_name,
                      use_effect_name_for_self, position_mode, source_kind, ranks)
    LOOP
        -- Fail closed on a missing/renamed/rank-changed source. Native naming
        -- deliberately mirrors LoadStatuses, including its per-rank fallback.
        -- A custom replacement cannot silently stand in for an approved native
        -- source, and a custom source cannot collide with an unreplaced native.
        IF seed.source_kind = 'native' THEN
            SELECT array_agg(e.except_level::INTEGER ORDER BY e.except_level)
            INTO actual_ranks
            FROM gamedata.ov_exceptdetail d
            JOIN gamedata.ov_exceptdetail_entry e ON e.row_no = d.row_no
            LEFT JOIN gamedata.ov_exceptdesc x ON x.type = d.type
            WHERE d.type = seed.status_id AND e.last_time > 0
              AND COALESCE(to_jsonb(x)->>('lv' || e.except_level::TEXT || '_name'),
                           x.lv1_name, '') = seed.expected_name
              AND NOT EXISTS (
                  SELECT FROM game_custom_statuses c
                  WHERE c.status_id = d.type AND c.status_level = e.except_level
              );
        ELSIF seed.source_kind = 'custom' THEN
            SELECT array_agg(c.status_level ORDER BY c.status_level)
            INTO actual_ranks
            FROM game_custom_statuses c
            WHERE c.status_id = seed.status_id AND c.name = seed.expected_name
              AND c.duration_sec > 0
              AND c.replace_native = EXISTS (
                  SELECT FROM gamedata.ov_exceptdetail d
                  JOIN gamedata.ov_exceptdetail_entry e ON e.row_no = d.row_no
                  WHERE d.type = c.status_id AND e.except_level = c.status_level
                    AND e.last_time > 0
              );
        ELSIF seed.source_kind = 'rift' THEN
            SELECT array_agg(r.version ORDER BY r.version)
            INTO actual_versions
            FROM game_rift_rule_versions r
            CROSS JOIN LATERAL jsonb_array_elements(r.elite_rules->'Abilities') a
            WHERE r.version IN (6, 7) AND r.published
              AND a->>'Name' = seed.expected_name
              AND (
                  (seed.status_id = 1900101 AND a->>'ID' = '0'
                   AND a->>'Mechanism' = 'golden_body' AND a->>'Trigger' = 'active')
                  OR
                  (seed.status_id = 1900102 AND a->>'ID' = '5'
                   AND a->>'Mechanism' = 'agility' AND a->>'Trigger' = 'passive')
              );
            IF actual_versions IS DISTINCT FROM ARRAY[6,7] THEN
                RAISE EXCEPTION 'Status visual source mismatch for %/%: expected one exact runtime ability in published rift versions 6 and 7, got %',
                    seed.status_id, seed.expected_name, actual_versions;
            END IF;
            actual_ranks := ARRAY[1];
        ELSE
            RAISE EXCEPTION 'Unknown status visual source kind: %', seed.source_kind;
        END IF;

        IF actual_ranks IS DISTINCT FROM seed.ranks THEN
            RAISE EXCEPTION 'Status visual source mismatch for %/%: expected % ranks %, got %. Verify the source version; do not manufacture status rows.',
                seed.status_id, seed.expected_name, seed.source_kind, seed.ranks, actual_ranks;
        END IF;

        SELECT * INTO previous FROM game_status_visuals
        WHERE status_id = seed.status_id AND expected_name = seed.expected_name;
        had_previous := FOUND;
        fillable := NOT had_previous OR
            (previous.effect_name = '' AND NOT previous.use_effect_name_for_self
             AND previous.position_mode = -1);

        -- This is an atomic whole-mapping fill, never a field-by-field merge:
        -- nonempty effects, custom self-title choices, custom positions and
        -- partial configurations are all preserved byte-for-byte.
        INSERT INTO game_status_visuals AS existing
            (status_id, expected_name, effect_name, use_effect_name_for_self, position_mode)
        VALUES
            (seed.status_id, seed.expected_name, seed.effect_name,
             seed.use_effect_name_for_self, seed.position_mode)
        ON CONFLICT (status_id, expected_name) DO UPDATE
        SET effect_name = EXCLUDED.effect_name,
            use_effect_name_for_self = EXCLUDED.use_effect_name_for_self,
            position_mode = EXCLUDED.position_mode
        WHERE existing.effect_name = '' AND NOT existing.use_effect_name_for_self
          AND existing.position_mode = -1;
        GET DIAGNOSTICS changed = ROW_COUNT;
        filled := filled + changed;

        SELECT * INTO current_value FROM game_status_visuals
        WHERE status_id = seed.status_id AND expected_name = seed.expected_name;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'Status visual postcondition missing row for %/%',
                seed.status_id, seed.expected_name;
        END IF;
        IF fillable THEN
            IF ROW(current_value.effect_name, current_value.use_effect_name_for_self,
                   current_value.position_mode)
               IS DISTINCT FROM
               ROW(seed.effect_name, seed.use_effect_name_for_self, seed.position_mode) THEN
                RAISE EXCEPTION 'Status visual fill postcondition failed for %/%',
                    seed.status_id, seed.expected_name;
            END IF;
        ELSIF current_value IS DISTINCT FROM previous THEN
            RAISE EXCEPTION 'Status visual preservation postcondition failed for %/%',
                seed.status_id, seed.expected_name;
        END IF;
        checked := checked + 1;
    END LOOP;

    IF checked <> 27 THEN
        RAISE EXCEPTION 'Expected 27 approved status visual identities, checked %', checked;
    END IF;
    RAISE NOTICE 'Status visuals: % source identities checked; % absent/neutral mappings filled; % configured mappings preserved',
        checked, filled, checked - filled;
END
$status_visual_seed$;

-- Intentionally absent: 1032 隐身 (dedicated visibility), 1157 状态抗性 and
-- 1173 伤害反射 (unimplemented mechanics). Also do not invent status IDs for
-- unused old global aliases: 魔镜术, 仙佑符, 灵聚之术符, 神返咒符, 砺金之咒符,
-- 坚甲之术符, 灵护之术符, 御法之术符, 裂刃枪法符, 野性之赐符, 心与剑随符,
-- 凌虚步法符, 灵印法符. No exact runtime status source uses those 13 names.
COMMIT;
