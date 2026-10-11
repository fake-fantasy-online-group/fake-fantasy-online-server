#!/usr/bin/env python3
"""Original unique vs coherent5-line P90 yellow: formula fixtures, not DPS.
Frozen inputs retain the pre-redesign catalogue for regression comparison.
"""
import json
import pathlib
import collections
HERE = pathlib.Path(__file__).resolve().parent
f = json.loads((HERE / 'baseline_fixture_inputs.json').read_text())
A = {r['index']: r for r in f['arm']}
D = {r['index']: r for r in f['desc']}
U = {r['BaseID']: r for r in f['unique']}
P = {(r['tier'], r['type'], r['attr'], r['mode']): r for r in json.loads((HERE / 'yellow_affix_benchmarks.json').read_text())}
starts = [[12, 24, 4, 4, 4, 4], [8, 20, 6, 6, 8, 6], [4, 16, 6, 4, 12, 12], [6, 16, 6, 12, 4, 4], [6, 12, 12, 16, 4, 4]]
odds = [[2, 5, 0, 1, 1, 1], [2, 4, 0, 1, 1, 1], [1, 3, 0, 1, 3, 2], [1, 3, 1, 2, 1, 1], [0, 3, 2, 3, 1, 1]]
evens = [[2, 5, 0, 1, 1, 1], [2, 4, 0, 2, 1, 2], [1, 3, 0, 1, 3, 2], [2, 3, 2, 2, 1, 1], [0, 3, 2, 3, 1, 1]]
idx = {1: 0, 5: 1, 3: 2, 61: 3, 7: 4, 9: 5}

def natural(race, lv):
    b = starts[race].copy()
    for n in range(2, lv + 1):
        b = [x + y for x, y in zip(b, evens[race] if n % 2 == 0 else odds[race])]
    return b

def fixture(race, lv, iid, affixes):
    a = A[iid]
    b = natural(race, lv)
    points = 2 * (lv - 1)
    for i, key in enumerate(['str_need', 'vit_need', 'int_need', 'spi_need', 'agi_need', 'dex_need']):
        delta = max(0, a[key] - b[i])
        b[i] += delta
        points -= delta
    assert points >= 0, (race, lv, iid, points)
    b[0 if race < 3 else 2] += points
    for i, key in enumerate(['str', 'vit', 'int_', 'spi', 'agi', 'dex']):
        b[i] += a[key]
    basepct = collections.Counter()
    flat = collections.Counter()
    pct = collections.Counter()
    for af in affixes:
        k, v, m = (af['Attr'], af['Value'], af['Mode'])
        if k in idx:
            if m == 0:
                b[idx[k]] += v
            else:
                basepct[idx[k]] += v
        elif m == 0:
            flat[k] += v
        else:
            pct[k] += v
    b = [x + int(x * basepct[i] / 100) for i, x in enumerate(b)]
    out = {29: b[0] // 2 + a['min_atk'] + flat[29] + flat[47], 31: b[0] // 2 + a['max_atk'] + flat[31] + flat[47], 15: b[2] // 2 + a['matk'] + flat[15], 19: b[5] // 2 + a['hit_rate'] + flat[19], 34: b[1] * 9 + flat[34], 36: b[3] * 9 + flat[36], 13: b[4] // 2 + a['def'] + flat[13], 17: b[3] // 2 + a['mdef'] + flat[17]}
    for k, v in list(out.items()):
        out[k] = v + int(v * pct[47 if k in [29, 31] else k] / 100)
    out['avg_atk'] = (out[29] + out[31]) / 2
    return out

def aff(attr, v, mode=0):
    return {'Attr': attr, 'Value': v, 'Mode': mode}
for lv, n in [(30, 6), (54, 10)]:
    for race, prefix in [(0, 1600), (1, 1000), (2, 1800), (3, 1300), (4, 1300)]:
        iid = prefix + n
        a = A[iid]
        k = (a['level'], D[iid]['type'])
        p90 = lambda attr, mode=0: P[*k, attr, mode]['p90']
        if race < 3:
            yellow = [aff(47, p90(47, 1), 1), aff(29, p90(29)), aff(31, p90(31)), aff(1, p90(1)), aff(19, p90(19, 1), 1)]
        else:
            yellow = [aff(15, p90(15, 1), 1), aff(3, p90(3)), aff(61, p90(61)), aff(34, p90(34)), aff(17, p90(17))]
        base = fixture(race, lv, iid, [])
        y = fixture(race, lv, iid, yellow)
        u = fixture(race, lv, iid, U[iid]['Affixes'])
        converted = fixture(race, lv, iid, yellow + U[iid]['Affixes'])
        key = 'avg_atk' if race < 3 else 15
        print(lv, race, iid, a['name'], 'base=', base[key], 'yellow=', y[key], 'unique=', u[key], 'Y HP/MP/hit=', y[34], y[36], y[19], 'U HP/MP/hit=', u[34], u[36], u[19], 'converted=', converted[key], 'Uaff=', U[iid]['Affixes'], 'Utraits=', U[iid]['Traits'])
