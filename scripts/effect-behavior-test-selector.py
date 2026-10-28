#!/usr/bin/env python3
"""Print a Go -run expression selecting effect-owned tests, regardless of name prefix.
Run from server repository root. This selector does not certify assertions or coverage.
Example (module directory): go test ./internal/game/scene -run "$(python3 ../scripts/effect-behavior-test-selector.py)"
"""
import json
import pathlib
import re
root = pathlib.Path(__file__).resolve().parents[1]
prefixes = ('legendary_', 'class_set', 'combat_causal', 'status_causal',
            'equipment_', 'special_weapon_', 'unique_', 'loot_context_v5',
            'named_equipment_', 'unified_effect_')
names = set()
for path in (root / 'server/internal/game/scene').glob('*test.go'):
    if path.name.startswith(prefixes):
        names.update(re.findall(r'^func (Test\w+)\(', path.read_text(), re.M))
# Explicit shared-consumer links may live outside equipment-owned files.
# Their evidence is reviewed separately; merely listing a name is not coverage.
manifest = root / 'docs/special-weapon-behavior-coverage.json'
if manifest.exists():
    def collect(value):
        if isinstance(value, str):
            names.update(re.findall(r'\bTest[A-Za-z0-9_]+', value))
        elif isinstance(value, list):
            for item in value:
                collect(item)
        elif isinstance(value, dict):
            for item in value.values():
                collect(item)
    collect(json.loads(manifest.read_text()))
# This AOI/map admission sweep is unrelated to effect semantics and tested separately.
names.discard('TestRiftAllAuditedMapsSnapshotAndMemberHandoff')
print('^(' + '|'.join(sorted(names)) + ')$')
