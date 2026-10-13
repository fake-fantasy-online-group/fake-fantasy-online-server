import copy,json,unittest
import compile_v6
class V6CompilerTest(unittest.TestCase):
 @classmethod
 def setUpClass(cls):cls.rows=json.loads((compile_v6.HERE/'v6/authored_catalog.json').read_text())
 def reject(self,fn):
  rows=copy.deepcopy(self.rows);fn(rows)
  with self.assertRaises((AssertionError,KeyError)):compile_v6.compile_catalog(rows)
 def test_exact_scope(self):self.assertEqual(sum(map(len,compile_v6.compile_catalog(self.rows).values())),259)
 def test_name_drift(self):self.reject(lambda x:x[0].update(Name='new identity'))
 def test_lore_drift(self):self.reject(lambda x:x[0].update(Lore='template'))
 def test_tier_inflation(self):self.reject(lambda x:x[0].update(PowerTier='godlike'))
 def test_unique_aura(self):self.reject(lambda x:x[0]['TypedEffects'].append({'kind':'aura'}))
 def test_missing_base(self):self.reject(lambda x:x.pop())
 def test_only_four_static_lines(self):self.reject(lambda x:x[0].update(Affixes=x[0]['Affixes'][:4]))
 def test_global_attack_percent_duplicate(self):self.reject(lambda x:x[0]['Affixes'].extend([{'Attr':29,'Value':20,'Mode':1},{'Attr':31,'Value':20,'Mode':1}]))
if __name__=='__main__':unittest.main()
