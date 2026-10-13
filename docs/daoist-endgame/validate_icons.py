#!/usr/bin/env python3
"""Validate source mappings and actual, externally supplied reviewed icon files.

The resource root must contain loose item/item/itemNNNN.gso files, extracted by
an authorized resource owner. This tool never downloads or publishes artwork.
"""
import argparse, hashlib, json, re
from pathlib import Path

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--resource-root', type=Path, required=True)
parser.add_argument('--mode', choices=('q21', 'classic'), required=True)
args = parser.parse_args()
folder = Path(__file__).resolve().parent
manifest = json.loads((folder / 'icon-assets.json').read_text())
source = (folder.parents[1] / 'server/migrations/000000_init-59.sql').read_text()
items = json.loads(source.split('$endgame_items$')[1])
assert [row['icon'] for row in items[:28]] == manifest['runes'], 'rune map differs from pixel review'
assert [row['icon'] for row in items[28:]] == [manifest['key']] + manifest['materials'], 'key/material map differs from pixel review'
primary = manifest['runes'] + [manifest['key']] + manifest['materials']
assert len(primary) == len(set(primary)) == 32, 'primary icons must remain distinct'
assert len(set(manifest['fallbacks'])) == 11, 'fallback semantic groups collapsed'
assets = {row['sprite']: row for row in manifest['assets']}
assert set(primary + manifest['fallbacks']) == set(assets), 'review coverage mismatch'
assert len({assets[id]['variants'][args.mode]['pixel_sha256'] for id in primary}) == 32, 'duplicate primary artwork'
assert len({assets[id]['variants'][args.mode]['visible_pixel_sha256'] for id in primary}) == 32, 'duplicate visible primary artwork after cropping transparent borders'
errors = []
for sprite, row in assets.items():
    path = args.resource_root / row['path']
    reviewed = row['variants'][args.mode]
    if not path.is_file():
        errors.append(str(path) + ': missing')
        continue
    data = path.read_bytes()
    if len(data) != reviewed['bytes'] or hashlib.sha256(data).hexdigest() != reviewed['sha256']:
        errors.append(str(path) + ': differs from reviewed ' + reviewed['package'])
if errors:
    parser.exit(1, '\n'.join(errors) + '\nFAIL: resource availability/identity was not established.\n')
print(f"PASS: {len(assets)} actual {args.mode} sprites; 32 distinct primary icons including normalized visible pixels and 11 semantic fallback groups; source map matches review.")
