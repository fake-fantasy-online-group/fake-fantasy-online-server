"""Static SQL contract and independent transactional model; NOT a PostgreSQL test."""
import copy
import json
import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
SQL = (ROOT / 'migrations/upgrades/000102_20261022_existing_class_sets_true.sql').read_text()
MANIFEST = json.loads((ROOT / 'internal/domain/dungeon_equipment_manifest.json').read_text())
MAP = dict((int(a), int(b)) for a, b in re.findall(r'^ \((\d+),(\d+)\)[,;]$', SQL, re.M))
LOCATIONS = ['character_items', 'character_equips', 'character_change_set_items',
             'character_warehouse_items', 'character_mail_attachments',
             'game_family_stash_items', 'character_stall_items']


def convert(state):
    """Model only the approved transformation and fail-closed transaction boundary."""
    if state['done']:
        return copy.deepcopy(state)
    out = copy.deepcopy(state)
    owned = [p for p in out['positions'] if not p.get('buy_order')]
    targets = [p for p in owned if p['item_id'] in MAP]
    for p in targets:
        uid = p['uid']
        if uid <= 0 or sum(x['uid'] == uid for x in owned) != 1:
            raise ValueError('invalid ownership')
        fact = out['facts'].get(uid)
        if not fact or fact['item_id'] != p['item_id'] or uid not in out['equipment']:
            raise ValueError('fact mismatch')
    for p in targets:
        out['facts'][p['uid']]['item_id'] = MAP[p['item_id']]
        p['item_id'] = MAP[p['item_id']]
    out['done'] = True
    return out


class Conversion102Tests(unittest.TestCase):
    def fixture(self):
        state = dict(done=False, positions=[], facts={}, equipment={}, history={'item_id': 8401},
                     pending_rift={'item_id': 8401}, socket_cards={'uid': 999, 'item_id': 7001})
        for n, (loc, old) in enumerate((loc, old) for loc in LOCATIONS for old in MAP):
            uid = n + 1
            state['positions'].append(dict(location=loc, uid=uid, item_id=old, refine=17, sockets=[999], bound=True))
            state['facts'][uid] = dict(uid=uid, item_id=old, instance_kind=1, created_source='original', updated_at='unchanged')
            state['equipment'][uid] = dict(uid=uid, refine=17, socket_instance_uids=[999], durability=321, rolled_affixes=[{'value': 5}])
        return state

    def test_exact_mapping_and_sql_contract(self):
        expected = {i for s in MANIFEST['sets'] for i in s['members']}
        self.assertEqual(len(MAP), 47)
        self.assertEqual(set(MAP), expected)
        self.assertTrue(all(b == a + 1800000000 for a, b in MAP.items()))
        self.assertTrue(set(MAP).isdisjoint(w['base_id'] for w in MANIFEST['weapons']))
        self.assertEqual(re.findall(r'UPDATE\s+(\S+)\s+p?i?\s*SET\s+(\w+)', SQL), [('item_instances', 'item_id'), ('%I', 'item_id')])
        for loc in LOCATIONS:
            self.assertIn("SELECT '" + loc + "'::text", SQL)
        self.assertIn('h.stall_type=0', SQL)
        self.assertIn('IN ACCESS EXCLUSIVE MODE NOWAIT', SQL)
        self.assertIn("to_jsonb(i)-'item_id'", SQL)
        self.assertIn('to_jsonb(e)<>f.equipment_before', SQL)
        self.assertTrue(SQL.rstrip().endswith('COMMIT;'))
        init = (ROOT / 'migrations/000000_init-102.sql').read_text()
        self.assertNotIn('UPDATE ', init)
        self.assertIn("VALUES ('class-set-true-20261022',0)", init)

    def test_all_locations_preserve_every_other_field(self):
        before = self.fixture()
        after = convert(before)
        self.assertEqual(after['equipment'], before['equipment'])
        for key in ['history', 'pending_rift', 'socket_cards']:
            self.assertEqual(after[key], before[key])
        for a, b in zip(after['positions'], before['positions']):
            self.assertEqual(a['item_id'], MAP[b['item_id']])
            self.assertEqual({k:v for k,v in a.items() if k != 'item_id'}, {k:v for k,v in b.items() if k != 'item_id'})
        for uid, fact in after['facts'].items():
            self.assertEqual({k:v for k,v in fact.items() if k != 'item_id'}, {k:v for k,v in before['facts'][uid].items() if k != 'item_id'})

    def test_once_marker_does_not_convert_future_normal_drops(self):
        once = convert(self.fixture())
        once['positions'].append(dict(location='character_items', uid=10000, item_id=8401))
        self.assertEqual(convert(once), once)

    def test_non_sets_and_purchase_requests_untouched(self):
        before = self.fixture()
        before['positions'] += [dict(uid=10001, item_id=1022), dict(uid=0, item_id=8401, buy_order=True)]
        after = convert(before)
        self.assertEqual(after['positions'][-2:], before['positions'][-2:])

    def test_bad_ownership_or_facts_abort_without_mutation(self):
        for failure in ['zero', 'duplicate', 'missing', 'mismatch']:
            with self.subTest(failure=failure):
                state = self.fixture()
                if failure == 'zero': state['positions'][0]['uid'] = 0
                if failure == 'duplicate': state['positions'].append(copy.deepcopy(state['positions'][0]))
                if failure == 'missing': del state['equipment'][1]
                if failure == 'mismatch': state['facts'][1]['item_id'] = 1022
                before = copy.deepcopy(state)
                with self.assertRaises(ValueError): convert(state)
                self.assertEqual(state, before)


if __name__ == '__main__':
    unittest.main()
