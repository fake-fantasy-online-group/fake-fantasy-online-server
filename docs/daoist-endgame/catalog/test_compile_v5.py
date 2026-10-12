import copy,json,unittest
import compile_v5
class V5CompilerTest(unittest.TestCase):
 @classmethod
 def setUpClass(cls):
  cls.proposal=json.loads((compile_v5.HERE/'v5/assignment.json').read_text())['items']
  cls.stats=json.loads((compile_v5.HERE/'v5/authored_stats.json').read_text())
 def reject(self,change):
  rows=copy.deepcopy(self.stats);change(rows)
  with self.assertRaises((AssertionError,KeyError)):compile_v5.compile_catalog(self.proposal,rows)
 def test_exact_scope_and_tiers(self):
  groups=compile_v5.compile_catalog(self.proposal,self.stats)
  self.assertEqual(sum(map(len,groups.values())),259)
 def test_missing_identity(self):self.reject(lambda x:x.pop())
 def test_four_static_properties(self):self.reject(lambda x:x[0].update(Affixes=x[0]['Affixes'][:4]))
 def test_free_advanced_property(self):self.reject(lambda x:x[0]['TypedEffects'].append({'kind':'magic_find_pct','value':20}))
 def test_unapproved_mf_value(self):self.reject(lambda x:next(r for r in x if r['BaseID']==1002)['TypedEffects'][0].update(value=30))
 def test_unapproved_proc_rank(self):self.reject(lambda x:next(r for r in x if r['BaseID']==1306)['TypedEffects'][0]['proc'].update(fixedRank=20))
 def test_unapproved_skill_scope(self):self.reject(lambda x:next(r for r in x if r['BaseID']==2409)['SkillBonuses'][0].update(scope='class',profession=5))
 def test_aura_pet_scope(self):self.reject(lambda x:next(r for r in x if r['BaseID']==1107)['TypedEffects'][0]['aura'].update(includeActivePet=False))
if __name__=='__main__':unittest.main()
