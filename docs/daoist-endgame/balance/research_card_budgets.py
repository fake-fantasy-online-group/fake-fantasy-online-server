#!/usr/bin/env python3
"""Reproduce card/source-level anchors and honest same-slot yellow references.

Reads original SQL and eligible.csv only; never edits runtime/catalogue data.
Card source ranks are NOT wearer levels. Availability is not inferred from rows.
"""
from __future__ import annotations

import argparse
import collections
import csv
import json
import pathlib
import re

import source_parser as source

HERE = pathlib.Path(__file__).resolve().parent
CLASSES = ['warrior', 'swordman', 'stabber', 'druggist', 'magician']
ATTR_NAMES = {1: 'STR', 3: 'INT', 5: 'VIT', 7: 'AGI', 9: 'DEX', 61: 'SPI',
              13: 'Def', 15: 'MAtk', 17: 'MDef', 19: 'Hit', 23: 'physical_crit_bp',
              25: 'attack_interval_pct', 27: 'movement', 29: 'MinAtk', 31: 'MaxAtk',
              34: 'HP', 36: 'MP', 38: 'HP_regen_no_general_consumer',
              40: 'weight_no_capacity_enforcement', 47: 'both_attack_bounds',
              66: 'magic_crit_bp', 69: 'flat_physical_reduction',
              70: 'flat_magic_reduction', 118: 'status_resistance'}
GRADE_NAMES = {'残品': 1, '次品': 2, '凡品': 3, '良品': 4, '精品': 5,
               '珍品': 6, '至尊': 7}


def card_family(name):
    if m := re.fullmatch(r'(.+?)\+(\d+)', name):
        return m[1], 'named_plus_rank', int(m[2])
    if m := re.fullmatch(r'(残品|次品|凡品|良品|精品|珍品|至尊)_(.+)', name):
        return m[2], 'named_quality', GRADE_NAMES[m[1]]
    if m := re.fullmatch(r'(\[[^]]+\].+?)(?:【(\d+)星】)?', name):
        return m[1], 'named_stars', int(m[2] or 0)
    return name, 'unranked', None


def active(effect):
    a, m = effect['attr'], effect['mode']
    return (effect['op'] == 1 and effect['probability'] == 100 and
            a in ATTR_NAMES and a not in (38, 40) and m in (0, 1) and
            not (a in (23, 66) and m != 0) and not (a == 25 and m != 1))


def build():
    source.TABLES.update({'ov_item', 'game_equip_extra', 'game_equipment', 'ov_excesuit'})
    tables = source.read_tables()
    arms = source.first_index(tables['ov_arm'])
    descs = source.first_index(tables['ov_desc'])
    items = source.first_index(tables['ov_item'])
    equipment = source.first_index(tables['game_equipment'], 'id')
    entries = collections.defaultdict(list)
    for e in tables['ov_card_entry']:
        if e['op_type']:
            entries[e['row_no']].append(e)
    cards = []
    for c in sorted(tables['ov_card'], key=lambda r: r['index']):
        if not c['index'] or c['index'] != c['item_id']:
            continue
        iid = c['index']
        d, item = descs[iid], items[iid]
        name = item['name']
        family, rank_kind, rank = card_family(name)
        effects = []
        for e in sorted(entries[c['row_no']], key=lambda e: e['idx']):
            fx = dict(attr=e['attr_id'], attr_name=ATTR_NAMES.get(e['attr_id'], 'unmapped'),
                      value=e['value'], mode=e['mode'], op=e['op_type'],
                      probability=e['prob'], source=e['_source'])
            fx['active_general_equipment_stat'] = active(fx)
            effects.append(fx)
        cards.append(dict(card_id=iid, name=name, family=family, rank_kind=rank_kind,
                          named_rank=rank, descriptor_resource_level=d['level'],
                          item_resource_level=item['level'], item_use_level=item['level_need'],
                          card_level_need=c['level_need'], card_min_level=c['min_level'],
                          card_max_level=c['max_level'], category=c['category'],
                          fun_type=c['fun_type'], source_part_types=[t for t, p in source.PARTS.items() if c[p]],
                          effects=effects, card_source=c['_source'], item_source=item['_source'],
                          descriptor_source=d['_source']))
    assert len(cards) == 2683
    assert len({c['card_id'] for c in cards}) == len(cards)
    assert all(c['item_use_level'] == c['card_level_need'] == c['card_min_level'] == c['card_max_level'] == 0 for c in cards)
    family_groups = collections.defaultdict(list)
    for c in cards:
        for e in c['effects']:
            key = c['family'], c['rank_kind'], c['named_rank'], e['attr'], e['mode'], e['active_general_equipment_stat']
            family_groups[key].append((c, e))
    families = []
    for key, rows in sorted(family_groups.items(), key=lambda kv: str(kv[0])):
        fam, kind, rank, attr, mode, enabled = key
        vals = [e['value'] for _, e in rows]
        families.append(dict(family=fam, rank_kind=kind, named_rank=rank, attr=attr,
                             attr_name=ATTR_NAMES.get(attr, 'unmapped'), mode=mode,
                             active_general_equipment_stat=enabled,
                             min_value=min(vals), max_value=max(vals),
                             distinct_values=sorted(set(vals)), card_ids=[c['card_id'] for c, _ in rows],
                             descriptor_resource_levels=sorted({c['descriptor_resource_level'] for c, _ in rows}),
                             source_part_types=sorted({t for c, _ in rows for t in c['source_part_types']})))

    with (HERE.parent / 'catalog/eligible.csv').open() as f:
        eligible = list(csv.DictReader(f))
    benchmarks = json.loads((HERE / 'yellow_affix_benchmarks.json').read_text())
    pool_keys = {(r['tier'], r['type']) for r in benchmarks}
    extras = collections.defaultdict(list)
    unknown_innates = set()
    for e in tables['game_equip_extra']:
        if e['prob'] >= 100 and e['attr_name']:
            extras[e['arm_id']].append(dict(attr=e['attr_id'], value=e['value'], mode=e['mode'], source=e['_source']))
        else:
            unknown_innates.add(e['arm_id'])
    suit_ids = {}
    for suit in tables['ov_excesuit']:
        for i in range(1, 25):
            if iid := suit[f'arm{i}_id']:
                assert iid not in suit_ids, ('multiple source suits', iid)
                suit_ids[iid] = suit['id']
    rows = []
    for r in eligible:
        iid = int(r['base_id']); a, d = arms[iid], descs[iid]
        professions = [i for i, name in enumerate(CLASSES) if a[name] or a['sr_' + name]]
        native = (a['level'], d['type']) in pool_keys
        assert native == (r['can_roll_affix'] == 'True')
        rows.append(dict(base_id=iid, base_name=a['name'], wearer_level=a['level_need'],
                         resource_tier=a['level'], type=d['type'], slot=equipment[iid]['slot'], professions=professions,
                         beginner_allowed=bool(a['newbie']), profession_restricted=bool(professions or a['newbie']),
                         sex=a['sex'], native_yellow_pool=native,
                         natural_affix_count=len(extras[iid]), fixed_affixes=extras[iid],
                         unknown_innate_properties=iid in unknown_innates, suit_id=suit_ids.get(iid, 0),
                         max_random_lines=max(0, 5-len(extras[iid])),
                         intrinsic={k: a[k] for k in ('min_atk', 'max_atk', 'matk', 'def', 'mdef', 'hit_rate', 'str', 'vit', 'int_', 'spi', 'agi', 'dex')},
                         requirements={k: a[k] for k in ('str_need', 'vit_need', 'int_need', 'spi_need', 'agi_need', 'dex_need')},
                         source_arm=a['_source'], source_desc=d['_source']))
    native_rows = [r for r in rows if r['native_yellow_pool']]
    for r in rows:
        r['same_level_native_alternatives'] = {}
        r['lower_level_native_anchors'] = {}
        for prof in r['professions'] or range(5):
            opts = [n for n in native_rows if n['type'] == r['type'] and
                    (not n['professions'] or prof in n['professions']) and
                    (not n['sex'] or not r['sex'] or n['sex'] == r['sex'])]
            exact = [n['base_id'] for n in opts if n['wearer_level'] == r['wearer_level']]
            r['same_level_native_alternatives'][CLASSES[prof]] = exact
            lower = [n for n in opts if n['wearer_level'] < r['wearer_level']]
            anchor_level = max((n['wearer_level'] for n in lower), default=None)
            r['lower_level_native_anchors'][CLASSES[prof]] = dict(
                wearer_level=anchor_level,
                base_ids=[n['base_id'] for n in lower if n['wearer_level'] == anchor_level])
    assert len(rows) == 1619 and len(native_rows) == 298
    card_doc = dict(schema=1,
                    level_semantics='All physical card/item wearer-use levels are zero. Named rank and descriptor resource level are source metadata, not equip-level gates.',
                    acquisition_scope='Source records only; no claim of current drop availability, market price, or player access.',
                    slot_semantics='source_part_types reproduce ov_card flags. Current ordinary inlay does not enforce them; do not claim runtime slot restrictions.',
                    active_scope='Static stats only; conditional/unknown effects, ordinary HP regeneration and unenforced weight excluded from active budget.',
                    physical_source_card_count=len(cards), cards=cards)
    family_doc = dict(schema=1, level_semantics=card_doc['level_semantics'],
                      aggregation='Unweighted source ranges, not drop probabilities or value quantiles.', families=families)
    budget_doc = dict(schema=1, total_bases=len(rows), native_pool_bases=len(native_rows),
                      native_pool_keys=len(pool_keys),
                      reference_affix_file='yellow_affix_benchmarks.json',
                      comparison_contract='Equal refinement/socket investment. Native natural-roll yellow baseline; card families are separate strength anchors.',
                      missing_pool_policy='No invented same-base yellow. Exact same-wearer-level alternatives and strictly lower-level native anchors are explicitly separate.',
                      class_ids=dict(enumerate(CLASSES)), bases=rows)
    return {'native_card_inventory.json': card_doc,
            'native_card_family_benchmarks.json': family_doc,
            'native_equipment_budget_references.json': budget_doc}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    for filename, data in build().items():
        target = HERE / filename
        # Full provenance stays self-contained; compact deterministic JSON keeps
        # these generated fixtures small. Human explanations live in the MD file.
        text = json.dumps(data, ensure_ascii=False, separators=(',', ':')) + '\n'
        if args.check:
            assert target.read_text() == text, f'stale {target}'
        else:
            target.write_text(text)
        print(('Verified ' if args.check else 'Wrote ') + filename)


if __name__ == '__main__':
    main()
