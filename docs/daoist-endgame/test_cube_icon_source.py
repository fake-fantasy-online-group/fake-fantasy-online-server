"""Verify the portable tool's alias against the original SQL, without game assets."""
import importlib.util
from pathlib import Path
import unittest


class CubeIconSourceTest(unittest.TestCase):
    def test_original_cauldron_alias_is_preserved_by_tool_migration(self):
        folder = Path(__file__).resolve().parent
        spec = importlib.util.spec_from_file_location(
            "cube_icon_source_parser", folder / "balance/source_parser.py")
        parser = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(parser)
        parser.TABLES = {"ov_desc"}
        original = parser.first_index(parser.read_tables()["ov_desc"])[9515]
        self.assertEqual(original["name_2"], "清神炉")
        self.assertEqual(original["small_map"], 9509)
        self.assertEqual(original["big_map"], 9509)
        sql = (parser.ROOT / "server/migrations/000000_init-84.sql").read_text()
        self.assertIn(
            "SELECT small_map INTO icon FROM gamedata.ov_desc WHERE index=9515 ORDER BY row_no LIMIT 1;",
            sql)
        self.assertIn("'small_map',icon,'big_map',icon", sql)
        # No newly authored sprite or copied loose art is required. Resolution
        # uses the existing package paths via the normal client icon loader.


if __name__ == "__main__":
    unittest.main()
