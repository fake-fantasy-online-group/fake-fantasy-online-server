#!/usr/bin/env python3
"""Read-only checks for the fixed V6 27-item showcase plan."""
import importlib.util
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
HERE = ROOT / "docs/daoist-endgame/showcase"
spec = importlib.util.spec_from_file_location("showcase_generator", HERE / "generate_showcase.py")
generator = importlib.util.module_from_spec(spec)
spec.loader.exec_module(generator)


class ShowcaseManifestTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.manifest = generator.build_manifest()

    def test_generated_artifacts_are_current(self):
        for name, expected in generator.outputs().items():
            self.assertEqual((HERE / name).read_text(), expected, name)

    def test_reviewed_v6_selection(self):
        expected = {
            42: [(2707,), (2049,), (2509,), (2410,), (2590,), (2895, 2896), (2108,), (2604,), (1810,)],
            183: [(2707,), (2059,), (2506,), (2409,), (2558,), (2829, 2830), (2108,), (2602,), (1310,)],
            206: [(2707,), (2056,), (2509,), (2409,), (2559,), (2899, 2900), (2108,), (2601,), (1310,)],
        }
        actual = {s["character_id"]: [tuple(c["base_id"] for c in i["candidates"]) for i in s["items"]]
                  for s in self.manifest["loadouts"]}
        self.assertEqual(actual, expected)
        self.assertEqual(self.manifest["grant_count"], 27)

    def test_slots_channels_and_sex_branches(self):
        for s in self.manifest["loadouts"]:
            self.assertEqual(sorted(i["slot"] for i in s["items"]), [1, 2, 3, 6, 7, 8, 9, 10, 13])
            self.assertEqual({i["channel"] for i in s["items"] if i["kind"] == "formation"}, {"战阵", "守阵", "辅阵"} if s["character_id"] == 206 else {"守阵", "辅阵"})
            body = next(i for i in s["items"] if i["slot"] == 8)
            self.assertEqual([(c["equipment_sex_restriction"], c["character_gender_when_selected"])
                              for c in body["candidates"]], [(1, 1), (2, 0)])

    def test_exact_rune_order_and_zero_natural_bases(self):
        _, formations, runes, _, _ = generator.load_sources()
        socket_count = 0
        for s in self.manifest["loadouts"]:
            for i in s["items"]:
                if i["kind"] != "formation":
                    self.assertEqual(i["socket_count"], 0)
                    continue
                recipe = formations[i["formation_id"]]
                self.assertEqual([r["rune_item_id"] for r in i["sockets"]], recipe["runes"])
                self.assertEqual(i["socket_count"], len(recipe["runes"]))
                self.assertEqual(i["unique_id"], 0)
                socket_count += i["socket_count"]
                for c in i["candidates"]:
                    self.assertEqual(c["natural_affix_count"], 0)
                    self.assertEqual(c["fixed_affixes"], [])
                    self.assertEqual(c["suit_id"], 0)
                    self.assertFalse(c["unknown_innate_properties"])
                for r in i["sockets"]:
                    self.assertEqual(r["fixed_card_affixes"], runes[r["rune_item_id"]]["affixes"])
        self.assertEqual(socket_count, 34)
        self.assertEqual(self.manifest["embedded_rune_instance_count"], socket_count)
        self.assertEqual(self.manifest["total_new_item_instances_if_executed"], 27 + socket_count)

    def test_max_roll_does_not_multiply_fixed_stats(self):
        assassin = self.manifest["loadouts"][0]
        weapon = next(i for i in assassin["items"] if i["slot"] == 13)
        self.assertEqual(weapon["kind"], "unique")
        self.assertEqual(weapon["unique_id"], 1810)
        for loadout in self.manifest["loadouts"]:
            for item in loadout["items"]:
                d = item["catalog_definition"]
                source = d.get("Affixes", d.get("affixes", []))
                variables = d.get("VariableAffixes", d.get("variableAffixes", []))
                variable_ids = {v.get("Attr", v.get("attr")) for v in variables}
                actual = {a["Attr"]: a["Value"] for a in item["max_roll_authored_affixes"]}
                for a in source:
                    attr = a.get("Attr", a.get("attr"))
                    if attr not in variable_ids:
                        self.assertEqual(actual[attr], a.get("Value", a.get("value")))
        for s in self.manifest["loadouts"]:
            for i in s["items"]:
                self.assertEqual(i["content_version"], 6)
                self.assertEqual(i.get("unique_roll_bp", i.get("formation_roll_bp")), 12000)
                self.assertEqual(i["refine_level"], 0)
                self.assertEqual(i["rolled_affix_count"], 0)

    def test_prepared_read_only_scope(self):
        self.assertEqual(self.manifest["status"], "prepared_v6_pending_review_no_grant")
        policy = self.manifest["policy"]
        self.assertTrue(policy["preserve_existing_items"])
        self.assertFalse(policy["auto_equip"])
        self.assertFalse(policy["change_character_level_skills_stats_or_free_points"])
        compact = json.loads((HERE / "grant-plan.json").read_text())
        self.assertEqual(compact["grant_count"], 27)
        self.assertEqual(len(compact["loadouts"]), 3)


if __name__ == "__main__":
    unittest.main()
