"""Prove all rune icon changes from native SQL, without a database or artwork.

Run: python3 -m unittest discover -s docs/daoist-endgame -p test_rune_monster_icons.py
Actual external pixel/hash verification remains in validate_icons.py.
"""
import json
from pathlib import Path
import re
import sys
import unittest

FOLDER = Path(__file__).resolve().parent
ROOT = FOLDER.parents[1]
sys.path.insert(0, str(FOLDER / "balance"))
import source_parser


class RuneMonsterIcons(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.proof = json.loads((FOLDER / "rune-monster-icons.json").read_text())
        cls.assets = json.loads((FOLDER / "icon-assets.json").read_text())
        previous_tables = source_parser.TABLES
        try:
            source_parser.TABLES = {"game_monsters", "game_monster_drops", "ov_item", "ov_desc"}
            cls.tables = source_parser.read_tables()
        finally:
            source_parser.TABLES = previous_tables
        cls.monsters = {row["id"]: row for row in cls.tables["game_monsters"]}
        cls.items = source_parser.first_index(cls.tables["ov_item"])
        cls.descriptions = source_parser.first_index(cls.tables["ov_desc"])
        cls.drops = {(row["monster_id"], row["seq"]): row for row in cls.tables["game_monster_drops"]}

    def assertSourceFields(self, expected, actual):
        self.assertEqual(expected, {key: actual[key] for key in expected})

    def test_every_rune_is_audited_and_runtime_matches(self):
        runes = json.loads((ROOT / "server/docs/endgame/astral_runes.json").read_text())["runes"]
        rows = self.proof["runes"]
        self.assertEqual(len(rows), 28)
        self.assertEqual([(r["rune_id"], r["rune_name"]) for r in rows], [(r["id"], r["name"]) for r in runes])
        self.assertEqual(self.proof["source_counts"], {table: len(rows) for table, rows in self.tables.items()})
        self.assertEqual(self.assets["rune_provenance"], "rune-monster-icons.json")
        expected = [row["sprite"] for row in rows]
        self.assertEqual(self.assets["runes"], expected)
        source = (ROOT / "server/migrations/000000_init-59.sql").read_text()
        items = json.loads(source.split("$endgame_items$")[1])
        self.assertEqual([row["icon"] for row in items[:28]], expected)

    def test_verified_monster_drop_collectible_sprite_chain(self):
        verified = [r for r in self.proof["runes"] if r["status"] == "verified_monster_collectible"]
        self.assertEqual(len(verified), self.proof["verified_count"])
        self.assertEqual(len(verified), 14)
        for row in verified:
            with self.subTest(rune=row["rune_name"]):
                monster = self.monsters[row["monster"]["id"]]
                drop = self.drops[(row["drop"]["monster_id"], row["drop"]["seq"])]
                item = self.items[drop["item_id"]]
                desc = self.descriptions[drop["item_id"]]
                self.assertSourceFields(row["monster"], monster)
                self.assertSourceFields(row["drop"], drop)
                self.assertSourceFields(row["collectible"], item)
                self.assertSourceFields(row["description"], desc)
                self.assertEqual(row["animal"], row["rune_name"][-1])
                self.assertIn(row["animal"], monster["name"])
                self.assertEqual(monster["id"], drop["monster_id"])
                self.assertEqual(drop["item_src"], "item")
                self.assertGreater(float(drop["rate"]), 0)
                self.assertEqual(drop["item_name"], item["name"])
                self.assertEqual(item["category"], 0)
                self.assertEqual(item["query_type"], 27)
                self.assertEqual((item["self_use"], item["other_use"]), (0, 0))
                self.assertEqual(row["sprite"], desc["small_map"])
                self.assertNotEqual(row["sprite"], row["previous_sprite"])

    def test_related_collectibles_are_not_misrepresented_as_direct_drops(self):
        related = [r for r in self.proof["runes"] if r["status"] == "related_collectible_substitute"]
        self.assertEqual(len(related), self.proof["related_substitute_count"])
        self.assertEqual(len(related), 14)
        self.assertEqual(self.proof["pending_count"], 0)
        for row in self.proof["runes"]:
            terms = row.get("name_search_terms", [row["animal"]])
            matches = [m for m in self.tables["game_monsters"] if any(term in m["name"] for term in terms)]
            self.assertEqual(row["name_match_count"], len(matches))
            if row["status"] != "related_collectible_substitute":
                continue
            self.assertNotEqual(row["sprite"], row["previous_sprite"])
            self.assertTrue(row["reason"])
            self.assertNotIn("drop", row)
            self.assertNotIn("monster", row)
            self.assertEqual(row["direct_drop_audit"]["status"], "not_established")
            self.assertTrue(row["direct_drop_audit"]["reason"])
            item = self.items[row["collectible"]["index"]]
            desc = self.descriptions[item["index"]]
            self.assertSourceFields(row["collectible"], item)
            self.assertSourceFields(row["description"], desc)
            self.assertEqual((item["category"], item["self_use"], item["other_use"]), (0, 0, 0))
            self.assertEqual(row["sprite"], desc["small_map"])
            self.assertEqual(row["candidates"], [{k: m[k] for k in ("id", "name", "sprite", "_source")} for m in matches])

    def test_crow_alias_candidates_have_only_rejected_generic_drop_evidence(self):
        row = next(r for r in self.proof["runes"] if r["animal"] == "乌")
        self.assertEqual(row["name_search_terms"], ["乌", "鸦"])
        self.assertEqual(row["status"], "related_collectible_substitute")
        self.assertEqual(row["direct_drop_audit"]["status"], "not_established")
        self.assertEqual(row["sprite"], 39439)
        evidence = row["candidate_drop_evidence"]
        crow_ids = [m["id"] for m in self.tables["game_monsters"] if "鸦" in m["name"]]
        self.assertEqual([e["monster_id"] for e in evidence], crow_ids)
        self.assertTrue({31724, 31784}.issubset(crow_ids))
        for candidate in evidence:
            mid = candidate["monster_id"]
            self.assertIn("鸦", self.monsters[mid]["name"])
            self.assertEqual(candidate["drops"], [d for d in self.tables["game_monster_drops"] if d["monster_id"] == mid])
            self.assertTrue(candidate["decision"])
            self.assertTrue(candidate["drops"])
            if mid in (31724, 31784):
                self.assertTrue(any(d["item_id"] == 4126 for d in candidate["drops"]))

    def test_all_primary_icons_have_distinct_normalized_visible_pixels(self):
        primary = self.assets["runes"] + [self.assets["key"]] + self.assets["materials"]
        self.assertEqual(len(primary), 32)
        self.assertEqual(len(set(primary)), 32)
        assets = {row["sprite"]: row for row in self.assets["assets"]}
        for mode in ("q21", "classic"):
            for key in ("pixel_sha256", "visible_pixel_sha256"):
                hashes = [assets[sprite]["variants"][mode][key] for sprite in primary]
                self.assertTrue(all(re.fullmatch(r"[0-9a-f]{64}", value) for value in hashes))
                self.assertEqual(len(set(hashes)), 32)

    def test_user_examples_and_rabbit_sprite_alias(self):
        by_animal = {r["animal"]: r for r in self.proof["runes"]}
        for animal, monster, item, sprite in [("狼", 1014, 4115, 4115), ("蚓", 1127, 4338, 4338)]:
            row = by_animal[animal]
            self.assertEqual((row["monster"]["id"], row["collectible"]["index"], row["sprite"]), (monster, item, sprite))
        self.assertEqual(self.descriptions[4101]["small_map"], by_animal["猴"]["sprite"])
        self.assertEqual(by_animal["兔"]["sprite"], 4141)
        self.assertNotEqual(by_animal["兔"]["sprite"], by_animal["猴"]["sprite"])


if __name__ == "__main__":
    unittest.main()
