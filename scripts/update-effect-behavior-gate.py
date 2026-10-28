#!/usr/bin/env python3
"""Refresh execution evidence only. A PASS is not semantic assertion review.
Usage: update-effect-behavior-gate.py TEST_JSONL COVERPROFILE
The curated manifest maps semantics; do not infer effects from arbitrary ID mentions.
"""
import collections
import hashlib
import json
import pathlib
import re
import subprocess
import sys
root = pathlib.Path(__file__).resolve().parents[1]
log, cover = map(pathlib.Path, sys.argv[1:3])
events = []
for line in log.read_text().splitlines():
    try:
        events.append(json.loads(line))
    except ValueError:
        pass
passed = {e['Test'] for e in events if e.get('Action') == 'pass' and e.get('Test')}
failures = [e.get('Test', '(package)') for e in events if e.get('Action') == 'fail']
package_pass = any(e.get('Action') == 'pass' and not e.get('Test') for e in events)
path = root / 'docs/legendary-effect-behavior-gate.json'
report = json.loads(path.read_text())
def ran(name):
    return name in passed or any(t.startswith(name + '/') for t in passed)
for effect in report['effects']:
    for test in effect['tests']:
        test['executed_pass'] = ran(test['name'])
    effect['status'] = 'behavior_witness_exists' if any(t['executed_pass'] for t in effect['tests']) else 'unverified'
for group in report['set_behavior_groups']:
    group['executed_pass'] = ran(group['test'])
for group in report['interaction_test_families']:
    group['executed_pass'] = all(ran(t) for t in group['tests'])
counts = collections.defaultdict(lambda: [0, 0])
for line in cover.read_text().splitlines():
    m = re.match(r'(.+):\d+\.\d+,\d+\.\d+ (\d+) (\d+)', line)
    if not m:
        continue
    file = m[1].rsplit('/', 1)[-1]
    if file.startswith(('legendary_', 'class_set', 'combat_causal', 'status_causal', 'equipment_', 'special_weapon_', 'unique_')) and 'zz_' not in file:
        n, hits = int(m[2]), int(m[3])
        counts[file][0] += n * (hits > 0)
        counts[file][1] += n
covered, total = map(sum, zip(*counts.values()))
report['statement_coverage'] = {'snapshot': 'latest combined run', 'covered': covered, 'total': total,
                              'percent': round(covered * 100 / total, 2),
                              'scope': 'legendary, class-set, causal, equipment, special-weapon, unique production families; excludes zz registry',
                              'by_file': {f: dict(covered=a, total=b) for f, (a, b) in counts.items()}}
report['latest_combined_execution'] = {'head': subprocess.check_output(['git', '-C', str(root), 'rev-parse', 'HEAD'], text=True).strip(),
    'package_pass': package_pass, 'passing_test_nodes': len(passed), 'failures': failures,
    'log_sha256': hashlib.sha256(log.read_bytes()).hexdigest(), 'cover_sha256': hashlib.sha256(cover.read_bytes()).hexdigest(),
    'command': 'go test ./internal/game/scene -run "$(python3 ../scripts/effect-behavior-test-selector.py)" -count=1 -timeout=180s -coverprofile=PROFILE -json'}
report['executed_effect_witnesses'] = sum(e['status'] == 'behavior_witness_exists' for e in report['effects'])
for row in report.get('legacy_unique_traits', []):
    row['executed_pass'] = ran(row['test'])
for row in report.get('shared_equipment_families', []):
    row['executed_pass'] = bool(row.get('tests')) and all(ran(t) for t in row['tests'])
for row in report.get('normalized_runtime_definitions', []):
    row['executed_pass'] = bool(row.get('behavior_tests')) and all(ran(t) for t in row['behavior_tests'])
report['runtime_definitions_with_passing_consumers'] = sum(row.get('executed_pass', False) for row in report.get('normalized_runtime_definitions', []))
# Human semantic review and current-feature positive evidence remain independent gates.
report['gate'] = 'AWAITING_UNIFIED_ARCHITECTURE_AND_SEMANTIC_REVIEW' if package_pass and not failures else 'FAILED_COMBINED_EXECUTION'
path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
print(json.dumps({k: report[k] for k in ('gate', 'executed_effect_witnesses', 'latest_combined_execution')}, indent=2))
