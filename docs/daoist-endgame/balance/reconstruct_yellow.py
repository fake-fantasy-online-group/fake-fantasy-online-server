#!/usr/bin/env python3
"""Rebuild exact migration39 yellow affix benchmarks, without a database.
P50/P90 are conditional on the specific affix and mode, not whole-item quantiles.
"""
import argparse
import collections
import json
import math
import pathlib
import source_parser as g
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--output', type=pathlib.Path, default=pathlib.Path(__file__).with_name('yellow_affix_benchmarks.json'))
args = parser.parse_args()
t = g.read_tables()
arms = g.first_index(t['ov_arm'])
descs = g.first_index(t['ov_desc'])
actual_keys = {(a['level'], descs[i]['type']) for i, a in arms.items() if i and a['no_type_drop'] == 0 and (1 <= a['level'] <= 17) and (i in descs) and (descs[i]['type'] in g.PARTS)}
cards = {r['row_no']: r for r in t['ov_card'] if (5001 <= r['index'] <= 5997 or 13141 <= r['index'] <= 13191) and r['category'] == 0 and (r['fun_type'] == 0)}
entries = [e for e in t['ov_card_entry'] if e['row_no'] in cards and e['op_type'] == 1 and (e['prob'] == 100) and (e['mode'] in (0, 1)) and (e['attr_id'] in g.ATTRS)]
pools = collections.defaultdict(list)
for e in entries:
    c = cards[e['row_no']]
    for level in range(max(1, c['min_level']), min(17, c['max_level']) + 1):
        for typ, part in g.PARTS.items():
            if c[part]:
                pools[level, typ, e['attr_id'], e['mode']].append(dict(e, card_id=c['index'], card_source=c['_source']))
summary = {}
for k, opts in pools.items():
    if k[:2] not in actual_keys:
        continue
    vals = [abs(o['value']) for o in opts]
    lo = min(vals)
    hi = max(vals)
    scores = [1 if hi == lo else math.exp(-(math.log(1 + (v - lo) / (hi - lo)) - (math.log(2) + 0.45 ** 2)) ** 2 / (2 * 0.45 ** 2)) / (1 + (v - lo) / (hi - lo)) for v in vals]
    modes = len({kk[3] for kk in pools if kk[:3] == k[:3]})
    weights = [max(1, math.floor(1000000 * x / sum(scores) / modes + 0.5)) for x in scores]
    denom = sum(weights)
    ordered = sorted(zip(vals, weights))
    cumulative = 0
    qs = {}
    for v, w in ordered:
        cumulative += w
        for q in [0.1, 0.5, 0.9]:
            if q not in qs and cumulative >= q * denom:
                qs[q] = v
    # Magnitude quantiles match migration39's abs(value) curve. Keep the sign
    # explicit so a negative attack-speed modifier is not mistaken for slowing.
    signs = {-1 if o['value'] < 0 else 1 for o in opts}
    assert len(signs) == 1, ('mixed signs require separate handling', k)
    sign = signs.pop()
    summary[k] = {'min': lo, 'max': hi, 'mean': round(sum((v * w for v, w in zip(vals, weights))) / denom, 3), 'p10': qs[0.1], 'p50': qs[0.5], 'p90': qs[0.9], 'value_sign': sign, 'options': len(opts), 'totalweight': denom, 'first_source': opts[0]['_source'], 'last_source': opts[-1]['_source']}
rows = [{**dict(zip(['tier', 'type', 'attr', 'mode'], k)), **v} for k, v in sorted(summary.items())]
assert len({(r['tier'], r['type']) for r in rows}) == 135
assert sum((r['options'] for r in rows)) == 16583
args.output.write_text(json.dumps(rows, ensure_ascii=False, indent=2) + '\n')
print(f'Verified 135 actual tier/type keys, 16583 weighted options; wrote {args.output}')
