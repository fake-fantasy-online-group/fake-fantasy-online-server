-- Rift admission belongs to an existing native NPC. Keep its original appearance,
-- script, greeting and services intact, and keep ordinary trial guards separate.
CREATE TABLE IF NOT EXISTS game_rift_entrances (
    map_id INT NOT NULL REFERENCES map_defs(id),
    map_file TEXT NOT NULL,
    npc_id INT NOT NULL,
    PRIMARY KEY (map_id, npc_id),
    FOREIGN KEY (map_file, npc_id) REFERENCES map_npcs(map_file, npc_id)
);

-- 龙城 / 八仙使者: original row, not a second spawned or renamed NPC.
INSERT INTO game_rift_entrances (map_id, map_file, npc_id)
VALUES (7, 'qw0007', 83)
ON CONFLICT (map_id, npc_id) DO NOTHING;
