"""Player-facing formation names omit the category prefix; rune identities stay full."""
import json
from pathlib import Path
import unittest


class FormationNameTest(unittest.TestCase):
    def test_all_authored_formation_names_are_unprefixed_and_unique(self):
        root = Path(__file__).resolve().parents[3]
        source = json.loads((root / "server/docs/endgame/astral_runes.json").read_text())
        rows = source["formations"]
        self.assertEqual([row["id"] for row in rows], list(range(1, 41)))
        names = [row["name"] for row in rows]
        self.assertEqual(len(set(names)), 40)
        for row in rows:
            with self.subTest(formation=row["id"]):
                self.assertTrue(row["name"])
                self.assertFalse(row["name"].startswith("宿阵"))
        baseline = json.loads((root / "docs/daoist-endgame/catalog/rune_identity_baseline.json").read_text())
        self.assertEqual(source["runes"], baseline)
        self.assertTrue(all(row["name"].startswith("星篆·") for row in source["runes"]))


if __name__ == "__main__":
    unittest.main()
