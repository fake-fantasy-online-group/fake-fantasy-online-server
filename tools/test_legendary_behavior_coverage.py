import json
import tempfile
import unittest
from pathlib import Path

from legendary_behavior_coverage import validate


class BehaviorCoverageGateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        source = self.root / "server/internal/game/scene"
        source.mkdir(parents=True)
        (self.root / "server/go.mod").write_text("module example/server\n")
        (source / "behavior_test.go").write_text("package scene\nfunc TestBehavior(t *testing.T) {}\nfunc TestCombo(t *testing.T) {}\n")
        self.ref = {"package": "./internal/game/scene", "file": "server/internal/game/scene/behavior_test.go", "test": "TestBehavior/TY-A01"}
        self.row = {"power_id": "TY-A01", "status": "covered", "solo_tests": [self.ref], "effect_clauses": [{"clause": "actual effect", "assertion": "actual consumer result differs from baseline", "tests": [self.ref["test"]]}]}
        self.manifest = {"schema_version": 1, "group": "test", "powers": [self.row], "combination_tests": []}
        self.events = [self.event("pass", self.ref["test"]), self.event("pass")]

    def event(self, action, test=None):
        result = {"Action": action, "Package": "example/server/internal/game/scene"}
        if test:
            result["Test"] = test
        return result

    def run_gate(self, catalog=None):
        manifest, results = self.root / "manifest.json", self.root / "tests.jsonl"
        manifest.write_text(json.dumps(self.manifest))
        results.write_text("".join(json.dumps(event) + "\n" for event in self.events))
        return validate(self.root, catalog or [{"id": "TY-A01"}], [manifest], [results])

    def test_exact_behavior_reference_passes(self):
        result = self.run_gate()
        self.assertTrue(result["gate_passed"])
        self.assertEqual(result["coverage_percent"], 100)

    def test_skipped_or_missing_or_failed_test_does_not_count(self):
        for action in ["skip", "fail", "run"]:
            with self.subTest(action=action):
                self.events[0]["Action"] = action
                result = self.run_gate()
                self.assertFalse(result["gate_passed"])
                self.assertEqual(result["qualified_pass_total"], 0)

    def test_incomplete_or_failed_package_does_not_count(self):
        for action in ["skip", "fail", "run"]:
            with self.subTest(action=action):
                self.events[-1]["Action"] = action
                self.assertFalse(self.run_gate()["gate_passed"])

    def test_child_skip_rejects_parent_reference(self):
        self.events.insert(1, self.event("skip", self.ref["test"] + "/boundary"))
        self.assertFalse(self.run_gate()["gate_passed"])

    def test_combination_reference_cannot_be_counted_as_solo(self):
        self.manifest["combination_tests"] = [dict(self.ref)]
        self.assertFalse(self.run_gate()["gate_passed"])

    def test_unknown_missing_and_duplicate_ids_fail(self):
        self.assertFalse(self.run_gate([{"id": "TY-A01"}, {"id": "TY-A02"}])["gate_passed"])
        self.manifest["powers"].append(dict(self.row))
        self.assertFalse(self.run_gate()["gate_passed"])
        self.manifest["powers"][-1]["power_id"] = "TY-A99"
        self.assertFalse(self.run_gate()["gate_passed"])

    def test_same_aggregate_test_cannot_cover_two_powers(self):
        other = dict(self.row, power_id="TY-A02")
        self.manifest["powers"].append(other)
        self.assertFalse(self.run_gate([{"id": "TY-A01"}, {"id": "TY-A02"}])["gate_passed"])

    def test_source_and_package_identity_are_checked(self):
        self.ref["package"] = "./internal/domain"
        self.assertFalse(self.run_gate()["gate_passed"])
        self.ref["package"] = "./internal/game/scene"
        self.ref["file"] = "server/internal/game/scene/missing_test.go"
        self.assertFalse(self.run_gate()["gate_passed"])

    def test_clause_assertion_and_exact_mapping_required(self):
        self.row["effect_clauses"][0]["tests"] = ["TestCombo"]
        self.assertFalse(self.run_gate()["gate_passed"])
        self.row["effect_clauses"] = []
        self.assertFalse(self.run_gate()["gate_passed"])

    def test_explicit_combination_clause_stays_separate(self):
        combo = dict(self.ref, test="TestCombo")
        self.manifest["combination_tests"] = [combo]
        self.events.insert(1, self.event("pass", "TestCombo"))
        self.row["effect_clauses"].append({"kind": "combination", "clause": "paired modifier", "assertion": "two-power interaction", "tests": ["TestCombo"]})
        result = self.run_gate()
        self.assertTrue(result["gate_passed"])
        self.assertEqual(result["qualified_pass_total"], 1)
        self.assertEqual(result["combination_test_references"], 1)
        self.row["effect_clauses"] = self.row["effect_clauses"][1:]
        self.assertFalse(self.run_gate()["gate_passed"])

    def test_repeated_combo_reference_is_not_an_extra_unique_case(self):
        combo = dict(self.ref, test="TestCombo")
        self.manifest["combination_tests"] = [combo, dict(combo)]
        self.events.insert(1, self.event("pass", "TestCombo"))
        result = self.run_gate()
        self.assertTrue(result["gate_passed"])
        self.assertEqual(result["combination_test_references"], 2)
        self.assertEqual(result["unique_combination_test_references"], 1)
        self.assertEqual(result["unique_combination_top_level_test_groups"], 1)
        self.assertEqual(result["unique_solo_test_references"], 1)


if __name__ == "__main__":
    unittest.main()
