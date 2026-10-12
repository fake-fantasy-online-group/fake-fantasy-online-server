#!/usr/bin/env python3
"""Audit unique equipment icon identities without a database or artwork import.

  python3 docs/daoist-endgame/catalog/audit_icons.py --check
  python3 docs/daoist-endgame/catalog/audit_icons.py --write
  python3 docs/daoist-endgame/catalog/audit_icons.py --check \
      --resource-root /path/to/icon-review --resource-report /tmp/icon-report.json

The optional resource check decodes extracted PNG pixels with Pillow. It proves
that those reviewed package extractions contain readable, nonempty sprites; it
does not prove a user's complete installation or semantic correctness of every
sprite. Missing PNGs are absent from the supplied extraction, not globally.
No game assets are copied into the repository.
"""
from __future__ import annotations

import argparse
import collections
import hashlib
import json
from pathlib import Path
import sys

sys.dont_write_bytecode = True
import generate as catalog

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]
MANIFEST = HERE / 'equipment_icon_manifest.json'
TABLES = {'ov_arm', 'ov_desc', 'game_equipment', 'ov_item_icon_special', 'ov_petgrow'}
SLOT_NAMES = {
    1: 'face', 2: 'head', 3: 'necklace', 4: 'one_hand_weapon', 5: 'shield',
    6: 'gloves', 7: 'ring', 8: 'clothing', 9: 'shoes', 10: 'back',
    11: 'magic_treasure', 13: 'two_hand_weapon', 14: 'earring', 15: 'bangle',
    16: 'belt', 17: 'beast_accessory_17', 18: 'beast_accessory_18',
    19: 'beast_accessory_19', 20: 'bravery_badge', 21: 'faith_badge',
    22: 'loyalty_badge', 23: 'excellence_badge', 24: 'wisdom_badge', 25: 'wild_badge',
}
VISUAL_REVIEW = {
    'scope': 'Actual extracted q21 sprite pixels inspected on 2026-10-11; this is a bounded representative visual review, not a claim that every alias or installed resource was visually verified.',
    'shoe_sprites': [2101, 2102, 2110, 22086, 22090, 22094, 22161, 22239, 22317, 22395, 30041, 40330],
    'category_representatives': {
        '1': 2701, '2': 2001, '3': 2501, '4': 1001, '5': 1102,
        '6': 2401, '7': 2551, '8': 2801, '9': 2101, '10': 2601,
        '13': 1501, '14': 37336, '15': 37335, '16': 37334,
        '17': 37314, '18': 37315, '19': 37316, '20': 57001,
        '21': 57002, '22': 57003, '23': 57004, '24': 57005, '25': 57006,
    },
    'findings': 'Representative shapes match their native categories: face glasses, helmet, necklace, sword, shield, gloves, ring, torso, boots, wings, two-handed sword, earrings, bracelet, belt, beast ornaments, and badges. Sprite 1001 is a sword reference and is not assigned to any unique shoe base.',
    'external_contact_sheets': ['equipment-slots-review.png', 'equipment-extra-slots-review.png'],
}


def audit():
    catalog.TABLES = TABLES
    tables = catalog.read_tables()
    descriptors = catalog.first_index(tables['ov_desc'])
    equipment = catalog.first_index(tables['game_equipment'], 'id')
    # The native loader sends ascending row_no; a later explicit override wins
    # in the receiving dictionary. Descriptor selection instead chooses the
    # earliest row_no, exactly like LoadItemsVerbose's DISTINCT ON query.
    specials = {r['arm_id']: r for r in sorted(
        tables['ov_item_icon_special'], key=lambda r: r['row_no'])}
    descriptor_counts = collections.Counter(r['index'] for r in tables['ov_desc'])
    current_files = sorted((ROOT / 'server/internal/domain').glob('unique_catalog_*.json'))
    legacy_files = sorted((ROOT / 'server/internal/domain/legacy_unique_v4').glob('*.json'))
    unique_files = legacy_files + current_files
    # Visual aliases outlive generation eligibility: preserve all old owners.
    unique_rows = list({r['BaseID']:r for p in unique_files for r in json.loads(p.read_text())}.values())
    errors, rows = [], []
    seen = set()
    for unique in sorted(unique_rows, key=lambda r: r['BaseID']):
        iid = unique['BaseID']
        if iid in seen or unique['ID'] != iid:
            errors.append(f'invalid or repeated unique base identity {iid}')
        seen.add(iid)
        desc, equip, special = descriptors.get(iid), equipment.get(iid), specials.get(iid)
        if desc is None or equip is None:
            errors.append(f'unique base {iid} lacks original descriptor or equipment row')
            continue
        if equip['slot'] != unique['Slot']:
            errors.append(f'unique base {iid} slot differs from loaded equipment')
        small = desc['small_map']
        try:
            effective = int(special['icon_small']) if special else small
        except (TypeError, ValueError):
            effective = 0
        if not isinstance(small, int) or small <= 0 or effective <= 0:
            errors.append(f'unique base {iid} has invalid source icon mapping')
        rows.append({
            'base_id': iid, 'base_name': unique['BaseName'], 'slot': equip['slot'],
            'descriptor_small': small, 'descriptor_big': desc['big_map'],
            'effective_small': effective, 'descriptor_row_no': desc['row_no'],
            'descriptor_rows_for_base': descriptor_counts[iid],
            'descriptor_source': desc['_source'],
            'special_override_source': special['_source'] if special else None,
        })
    slots = []
    for slot in sorted({e['slot'] for e in equipment.values()}):
        selected = [r for r in rows if r['slot'] == slot]
        slots.append({
            'slot': slot, 'category': SLOT_NAMES.get(slot, 'unknown'),
            'all_equipment_count': sum(e['slot'] == slot for e in equipment.values()),
            'unique_count': len(selected),
            'descriptor_nonidentity_count': sum(r['descriptor_small'] != r['base_id'] for r in selected),
            'special_override_count': sum(r['special_override_source'] is not None for r in selected),
        })
    pets = {r['pet_id'] for r in tables['ov_petgrow'] if r['pet_id'] > 0}
    overlap = set(specials) & seen
    pairs = len(specials) + len(pets) + len(seen - set(specials)) + 32
    payload = 2 + pairs * 8
    plaintext = 8 + 2 + payload
    packet = {
        'pair_count': pairs, 'payload_bytes': payload,
        'opcode_and_payload_bytes': payload + 2,
        'encrypted_frame_bytes_including_prefix': (plaintext // 16 + 1) * 16 + 52,
        'pair_limit': 65535, 'wire_frame_limit_bytes': 1048576,
    }
    if pairs > packet['pair_limit'] or packet['encrypted_frame_bytes_including_prefix'] > packet['wire_frame_limit_bytes']:
        errors.append('combined icon mapping packet exceeds native wire limits')
    sources = {r['descriptor_source'].split(':')[0] for r in rows}
    sources.update(r['special_override_source'].split(':')[0] for r in rows if r['special_override_source'])
    sources.update(str(p.relative_to(ROOT)) for p in unique_files)
    result = {
        'schema_version': 1,
        'scope': 'Appearance aliases for V5 and frozen V4-owned uniques; generation remains limited to259 approved bases.',
        'new_generation_count': 259,
        'selection': {
            'descriptor': 'First ov_desc row by numeric row_no for each index (DISTINCT ON index ORDER BY index,row_no).',
            'special': 'ov_item_icon_special.icon_small overrides descriptor small_map; last row_no wins.',
            'existing_runtime_override': 'Existing explicit runtime ItemIconMapping wins over newly published unique-base mapping.',
        },
        'unique_count': len(rows), 'all_equipment_count': len(equipment),
        'descriptor_nonidentity_count': sum(r['descriptor_small'] != r['base_id'] for r in rows),
        'special_override_count': len(specials), 'special_unique_overlap_count': len(overlap),
        'new_unique_override_count': len(seen - set(specials)), 'pet_carrier_count': len(pets),
        'endgame_material_rune_key_count': 32, 'combined_packet': packet,
        'source_sha256': {p: hashlib.sha256((ROOT / p).read_bytes()).hexdigest() for p in sorted(sources)},
        'category_counts': slots, 'source_errors': errors, 'mappings': rows,
        'visual_review': VISUAL_REVIEW,
    }
    return result


def check_resources(manifest, root):
    try:
        from PIL import Image
    except ImportError as exc:
        raise SystemExit('--resource-root requires Pillow for actual pixel decoding') from exc
    folders = [root / 'res_05diff.pkg-icons', root / 'res_q21_001.pkg-icons']
    if not any(p.is_dir() for p in folders):
        raise SystemExit('resource root lacks both supported package extraction directories')
    ids = sorted({r['effective_small'] for r in manifest['mappings']})
    present, absent, invalid = {}, [], []
    for icon in ids:
        matches = [folder / f'{icon}.png' for folder in folders if (folder / f'{icon}.png').is_file()]
        if not matches:
            absent.append(icon)
            continue
        checks = []
        for path in matches:
            try:
                with Image.open(path) as image:
                    image.load()
                    rgba = image.convert('RGBA')
                    if rgba.getchannel('A').getbbox() is None:
                        raise ValueError('all pixels are transparent')
                    checks.append({
                        'file': str(path.relative_to(root)), 'dimensions': list(image.size),
                        'sha256': hashlib.sha256(path.read_bytes()).hexdigest(),
                        'decoded_nonempty_pixels': True,
                    })
            except Exception as exc:
                invalid.append({'icon': icon, 'file': str(path.relative_to(root)), 'error': str(exc)})
        present[str(icon)] = checks
    return {
        'scope': 'Only supplied res_05diff and res_q21_001 extracted PNGs; absence is not global resource absence. Pixel decoding does not establish semantic correctness.',
        'distinct_effective_sprite_count': len(ids), 'present_sprite_count': len(present),
        'absent_from_supplied_extractions': absent, 'invalid_pixel_files': invalid,
        'bases_absent_from_supplied_extractions': [r['base_id'] for r in manifest['mappings'] if r['effective_small'] in absent],
        'decoded_sprites': present,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument('--write', action='store_true', help='regenerate the metadata-only checked-in manifest')
    mode.add_argument('--check', action='store_true', help='verify byte-exact agreement with the checked-in manifest')
    parser.add_argument('--resource-root', type=Path)
    parser.add_argument('--resource-report', type=Path, help='write optional local pixel-verification report')
    args = parser.parse_args()
    if args.resource_report and not args.resource_root:
        parser.error('--resource-report requires --resource-root')
    result = audit()
    if result['source_errors']:
        raise SystemExit('\n'.join(result['source_errors']))
    body = json.dumps(result, ensure_ascii=False, indent=2) + '\n'
    if args.write:
        MANIFEST.write_text(body)
    if args.check and (not MANIFEST.exists() or MANIFEST.read_text() != body):
        raise SystemExit('equipment icon manifest differs; inspect source changes before using --write')
    print(f"Verified {result['unique_count']} unique bases across {sum(s['unique_count'] > 0 for s in result['category_counts'])} slots; "
          f"{result['descriptor_nonidentity_count']} nonidentity descriptors; "
          f"{result['special_unique_overlap_count']} preserved special overrides.")
    print('Combined native packet:', json.dumps(result['combined_packet'], sort_keys=True))
    if args.resource_root:
        report = check_resources(result, args.resource_root)
        if args.resource_report:
            args.resource_report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
        print('Resource check:', json.dumps({k: v for k, v in report.items() if k != 'decoded_sprites'}, ensure_ascii=False))
        if report['invalid_pixel_files']:
            raise SystemExit('one or more supplied sprite files failed pixel decoding')


if __name__ == '__main__':
    main()
