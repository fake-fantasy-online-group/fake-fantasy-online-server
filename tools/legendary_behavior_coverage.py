#!/usr/bin/env python3
"""Verify the per-power behavior-test manifest against completed go test JSON.

This is a traceability/pass gate, not a proof that an assertion is meaningful.
Reviewers must inspect the actual consumer and independent fixture. Registration,
text, modifier-only and combination tests are not independent behavior evidence.
Go statement/branch coverage is deliberately not inferred here.
"""

import argparse
import json
import re
from collections import Counter, defaultdict
from pathlib import Path


def related(a, b):
    return a == b or a.startswith(b + "/") or b.startswith(a + "/")


def read_results(paths):
    runs = []
    for path in paths:
        tests, packages = defaultdict(list), defaultdict(list)
        with Path(path).open() as stream:
            for line in stream:
                try:
                    event = json.loads(line)
                except json.JSONDecodeError:
                    continue  # go build diagnostics are not test/pass events.
                if not isinstance(event, dict):
                    continue
                action, package = event.get("Action"), event.get("Package")
                if action not in {"pass", "fail", "skip"} or not package:
                    continue
                if event.get("Test"):
                    tests[(package, event["Test"])].append(action)
                else:
                    packages[package].append(action)
        runs.append((str(path), tests, packages))
    return runs


def validate(repo, catalog, manifests, result_paths):
    errors, rows = [], {}
    ids = [row["id"] for row in catalog]
    expected = set(ids)
    if len(expected) != len(ids):
        errors.append("catalog contains duplicate power IDs")
    module = re.search(r"^module\s+(\S+)", (repo / "server/go.mod").read_text(), re.M).group(1)
    runs = read_results(result_paths)
    combinations, groups = [], []
    for manifest_path in manifests:
        document = json.loads(Path(manifest_path).read_text())
        if document.get("schema_version") != 1:
            errors.append(f"{manifest_path}: expected schema_version 1")
        groups.append(document.get("group", str(manifest_path)))
        for entry in document.get("powers", []):
            power = entry.get("power_id")
            if power not in expected:
                errors.append(f"unknown power ID: {power}")
                continue
            if power in rows:
                errors.append(f"duplicate manifest power ID: {power}")
                continue
            rows[power] = entry
        combinations.extend(document.get("combination_tests", []))

    def package_name(value):
        if value.startswith("./"):
            return module + "/" + value[2:]
        return value

    def ref_key(ref):
        return package_name(ref.get("package", "")), ref.get("test", "")

    def check_reference(ref, label):
        package, test = ref_key(ref)
        path = (repo / ref.get("file", "")).resolve()
        if not path.is_relative_to(repo.resolve()) or not path.name.endswith("_test.go") or not path.is_file():
            errors.append(f"{label}: missing/invalid test source {ref.get('file')}")
            return False
        top = test.split("/")[0]
        if not re.fullmatch(r"Test\w+", top) or not re.search(r"^func\s+" + re.escape(top) + r"\s*\(", path.read_text(), re.M):
            errors.append(f"{label}: top-level test {top!r} not declared in {ref.get('file')}")
            return False
        expected_package = module + "/" + str(path.parent.relative_to(repo / "server")) if path.is_relative_to(repo / "server") else ""
        if package != expected_package:
            errors.append(f"{label}: package does not match test file: {package}")
            return False
        for _, tests, packages in runs:
            if tests.get((package, test)) != ["pass"] or packages.get(package) != ["pass"]:
                continue
            # A parent can pass only if its selected subtree has no skip/fail.
            # Missing, filtered-out, interrupted and cached metadata cannot pass.
            bad_child = any(p == package and name.startswith(test + "/") and any(a != "pass" for a in actions)
                            for (p, name), actions in tests.items())
            if not bad_child:
                return True
        errors.append(f"{label}: no exact non-skipped pass in a completed successful package: {package}:{test}")
        return False

    combo_keys = [ref_key(ref) for ref in combinations]
    combo_names = {ref.get("test", "") for ref in combinations}
    for ref in combinations:
        check_reference(ref, "combination")
    owners = {}
    covered, pending = [], []
    for power in ids:
        entry = rows.get(power)
        start_errors = len(errors)
        if entry is None:
            errors.append(f"missing power: {power}")
            pending.append(power)
            continue
        if entry.get("status") != "covered":
            errors.append(f"{power}: status is not covered")
        refs = entry.get("solo_tests", [])
        clauses = entry.get("effect_clauses", [])
        if not refs:
            errors.append(f"{power}: no solo behavior tests")
        if not clauses:
            errors.append(f"{power}: no effect clauses/assertions")
        if not any(clause.get("kind", "solo") == "solo" for clause in clauses):
            errors.append(f"{power}: no solo main-effect clause")
        names = {ref.get("test", "") for ref in refs}
        for clause in clauses:
            if not clause.get("clause", "").strip() or not clause.get("assertion", "").strip():
                errors.append(f"{power}: empty effect clause/behavior assertion")
            kind = clause.get("kind", "solo")
            allowed = combo_names if kind == "combination" else names
            if kind not in {"solo", "combination"}:
                errors.append(f"{power}: unsupported clause kind {kind}")
            if not clause.get("tests") or not set(clause["tests"]).issubset(allowed):
                errors.append(f"{power}: {kind} clause must reference its exact {kind} test names")
        for ref in refs:
            key = ref_key(ref)
            if any(key[0] == package and related(key[1], test) for package, test in combo_keys):
                errors.append(f"{power}: solo/combination test overlap: {key[1]}")
            # One aggregate test spanning several powers is not a solo test.
            for (package, name), other in owners.items():
                if other != power and key[0] == package and related(key[1], name):
                    errors.append(f"{power}: solo test overlaps power {other}: {key[1]}")
            owners[key] = power
            check_reference(ref, power)
        (covered if len(errors) == start_errors else pending).append(power)
    return {
        "schema_version": 1,
        "metric": "independent_power_behavior_test_manifest_pass_coverage",
        "catalog_total": len(ids),
        "mapped_total": len(rows),
        "qualified_pass_total": len(covered),
        "coverage_percent": round(100 * len(covered) / len(ids), 4) if ids else 0,
        "groups": groups,
        "counts_by_prefix": dict(Counter(power.split("-")[0] for power in covered)),
        "solo_test_references": sum(len(rows[p].get("solo_tests", [])) for p in covered),
        "unique_solo_test_references": len({ref_key(ref) for p in covered for ref in rows[p].get("solo_tests", [])}),
        "combination_test_references": len(combinations),
        "unique_combination_test_references": len(set(combo_keys)),
        "unique_combination_top_level_test_groups": len({(package, test.split("/")[0]) for package, test in combo_keys}),
        "combination_tests_count_towards_power_coverage": False,
        "covered_ids": covered,
        "pending_ids": pending,
        "errors": errors,
        "gate_passed": not errors and len(covered) == len(ids),
        "semantic_review_required": True,
        "limitations": [
            "This gate verifies catalog completeness, distinct solo references, source identity and actual non-skipped pass evidence, not semantic assertion sufficiency.",
            "A reviewer must reject catalog/registration/text/modifier-only tests and inspect real consumers, clause completeness and legal single-power fixtures.",
            "Independent per-power test-case coverage is not Go statement/branch coverage, all-combination coverage, production deployment or client visual-effect verification."
        ],
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument("--manifest", type=Path, action="append", required=True)
    parser.add_argument("--test-json", type=Path, action="append", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    repo = args.repo.resolve()
    catalog = json.loads((repo / "server/internal/domain/legendary_powers.json").read_text())
    result = validate(repo, catalog, args.manifest, args.test_json)
    rendered = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(rendered)
    print(rendered, end="")
    return 0 if result["gate_passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
