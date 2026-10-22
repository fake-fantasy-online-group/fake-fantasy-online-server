"""Offline SQL contract/identity regression. This is NOT PostgreSQL execution."""
import copy
import json
import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
BASE = (ROOT / 'migrations/000000_init-0.sql').read_text()
SEED = (ROOT / 'migrations/000000_init-103.sql').read_text()
UPGRADE = (ROOT / 'migrations/upgrades/000103_20261022_native_portal_geometry.sql').read_text()
APPROVED = json.loads((ROOT / 'internal/testfixture/riftportals/approved.json').read_text())
IDENTITY = ['map_file', 'proc_id', 'map_to', 'to_x', 'to_y', 'to_dir', 'back_x', 'back_y', 'back_dir']

def original_routes():
    rows = {}
    for m in re.finditer(r"\('([^']+)', (-?\d+), (-?\d+), (-?\d+), (-?\d+), (-?\d+), (-?\d+), (-?\d+), (-?\d+), NULL\)", BASE):
        v = [m[1]] + list(map(int, m.groups()[1:]))
        r = dict(zip(IDENTITY, v), poly=None, script='', transition_owner='server')
        rows[(r['map_file'], r['proc_id'])] = r
    for m in re.finditer(r"UPDATE map_teleports SET poly=ARRAY\[([^\]]+)\]\s+WHERE map_file='([^']+)' AND proc_id=(\d+) AND map_to=(\d+) AND to_x=(\d+) AND to_y=(\d+);", BASE):
        key = m[2], int(m[3])
        r = rows.get(key)
        if r and [r[k] for k in ['map_to','to_x','to_y']] == list(map(int,m.groups()[3:])):
            r['poly'] = list(map(int,m[1].split(',')))
    return rows


def repair(before):
    """Independent atomic model; rejects divergence instead of guessing."""
    result = copy.deepcopy(before)
    for approved in APPROVED:
        key = approved['map_file'], approved['proc_id']
        row = result.get(key)
        if not row or any(row[k] != approved[k] for k in IDENTITY) or row['script'] != '':
            raise ValueError('route identity mismatch')
        if row['poly'] is not None and row['poly'] != approved['poly']:
            raise ValueError('geometry conflict')
        if row['poly'] is None:
            row['poly'] = approved['poly'].copy()
            row['transition_owner'] = 'client'
    return result


ORIGINAL = original_routes()


class NativePortal103Tests(unittest.TestCase):
    def test_sql_seed_upgrade_and_exact_approved_routes(self):
        self.assertIn(SEED, UPGRADE)
        self.assertTrue(UPGRADE.startswith('-- Apply'))
        self.assertTrue(UPGRADE.rstrip().endswith('COMMIT;'))
        self.assertIn('BEGIN;', UPGRADE)
        self.assertIn('pg_advisory_xact_lock(1936224119,103)', UPGRADE)
        sql_rows = re.findall(r"^   \((\d+),'([^']+)',(\d+),(-?\d+),(-?\d+),(-?\d+),(-?\d+),(-?\d+),(-?\d+),(-?\d+),ARRAY\[([^\]]+)\]\)[,]?$", SEED, re.M)
        self.assertEqual(len(sql_rows),len(APPROVED))
        for actual, expected in zip(sql_rows, APPROVED):
            self.assertEqual(int(actual[0]),expected['map_id'])
            self.assertEqual(actual[1],expected['map_file'])
            self.assertEqual(list(map(int,actual[2:10])),[expected[k] for k in IDENTITY[1:]])
            self.assertEqual(list(map(int,actual[10].split(','))),expected['poly'])
        self.assertEqual(len({(r['map_file'],r['proc_id']) for r in APPROVED}),len(APPROVED))
        for field in IDENTITY:
            self.assertIn(f't.{field}=r.{field}', SEED)
        for guard in ["t.script=''",'FOR UPDATE','previous.poly IS NOT NULL','previous.poly IS DISTINCT FROM r.poly',
                      "AND t.poly IS NULL", "to_jsonb(after_row)-'poly'-'transition_owner'",
                      'Native portal repair postcondition failed',f'IF checked <> {len(APPROVED)} THEN']:
            self.assertIn(guard,SEED)
        self.assertEqual(re.findall(r'UPDATE\s+(\w+)\s+\w+\s+SET\s+([^\n]+)',SEED),
                         [('map_teleports',"poly=r.poly,transition_owner='client'")])
        for forbidden in ['DELETE ', 'TRUNCATE ', 'schema_migrations','character_','game_endgame_','DROP TABLE']:
            self.assertNotIn(forbidden,SEED)

    def test_every_repair_targets_missing_geometry_and_preserves_destinations(self):
        before=copy.deepcopy(ORIGINAL)
        after=repair(before)
        self.assertEqual(len(before),994)
        approved_keys={(r['map_file'],r['proc_id']) for r in APPROVED}
        for key,old in before.items():
            new=after[key]
            if key not in approved_keys:
                self.assertEqual(new,old)
                continue
            self.assertIsNone(old['poly'])
            self.assertGreaterEqual(len(new['poly']),6)
            self.assertEqual(len(new['poly'])%2,0)
            self.assertEqual(new['transition_owner'],'client')
            self.assertEqual({k:v for k,v in old.items() if k not in ['poly','transition_owner']},
                             {k:v for k,v in new.items() if k not in ['poly','transition_owner']})
        self.assertEqual(repair(after),after)

    def test_all_168_candidate_native_gates_match_repaired_database(self):
        gates=json.loads((ROOT/'internal/testfixture/riftportals/all-native-gates.json').read_text())
        self.assertEqual(len(gates),354)
        self.assertEqual(len({g['map_id'] for g in gates}),168)
        self.assertEqual(len({(g['map_file'],g['proc_id']) for g in gates}),354)
        after=repair(copy.deepcopy(ORIGINAL))
        for gate in gates:
            self.assertEqual(after[(gate['map_file'],gate['proc_id'])]['poly'],gate['poly'])
            for k in ['source_hash','link_hash','mask_hash']:
                self.assertRegex(gate[k],r'^[0-9a-f]{64}$')

    def test_offline_native_proof_is_bound_to_handler_and_sql_fixtures(self):
        proof=ROOT/'tests/native_portal_geometry'
        source_repairs=json.loads((proof/'approved-recovery-rows.json').read_text())
        by_key={(r['map_id'],r['proc_id']):r for r in source_repairs}
        self.assertEqual(set(by_key),{(r['map_id'],r['proc_id']) for r in APPROVED})
        for row in APPROVED:
            src=by_key[(row['map_id'],row['proc_id'])]
            self.assertEqual([row[k] for k in IDENTITY],[src[k] for k in IDENTITY])
            self.assertEqual(row['poly'],src['verifiedPoly'])
            self.assertEqual(row['native_prc_sha256'],src['nativePRCSha256'])
            self.assertEqual(row['native_link_sha256'],src['nativeLINKSha256'])
        handler=json.loads((ROOT/'internal/testfixture/riftportals/all-native-gates.json').read_text())
        source_gates=json.loads((proof/'all-native-gates-manifest.json').read_text())
        native={(r['map_id'],r['proc_id']):r for r in source_gates}
        self.assertEqual(set(native),{(r['map_id'],r['proc_id']) for r in handler})
        for row in handler:
            src=native[(row['map_id'],row['proc_id'])]
            for key in ['map_file','poly','point','source_hash','link_hash','mask_hash']:
                self.assertEqual(row[key],src[key])

    def test_replay_keeps_explicit_owner_of_existing_geometry(self):
        rows=repair(copy.deepcopy(ORIGINAL))
        for r in rows.values():
            if r['poly'] is not None:r['transition_owner']='server'
        self.assertEqual(repair(rows),rows)

    def test_modified_identity_or_geometry_fails_without_partial_changes(self):
        for approved in APPROVED:
            key=approved['map_file'],approved['proc_id']
            for field in ['map_to','to_x','to_y','to_dir','back_x','back_y','back_dir','script','poly']:
                with self.subTest(map=approved['map_id'],proc=approved['proc_id'],field=field):
                    rows=copy.deepcopy(ORIGINAL)
                    rows[key][field] = 'custom' if field=='script' else [1,2,3,4,5,6] if field=='poly' else rows[key][field]+1
                    unchanged=copy.deepcopy(rows)
                    with self.assertRaises(ValueError):repair(rows)
                    self.assertEqual(rows,unchanged)

    def test_map36_exact_native_gate_identity(self):
        gates=[r for r in APPROVED if r['map_id']==36]
        self.assertEqual([(r['proc_id'],r['map_to']) for r in gates],[(2,77),(3,35),(4,78)])
        self.assertEqual(gates[0]['poly'],[3234,2092,3323,2169,3419,2102,3346,2030])
        self.assertEqual(gates[1]['poly'],[11420,2223,11518,2288,11600,2219,11502,2163])
        self.assertEqual(gates[2]['poly'],[8967,6208,9057,6281,9173,6211,9105,6144])

if __name__=='__main__':unittest.main()
