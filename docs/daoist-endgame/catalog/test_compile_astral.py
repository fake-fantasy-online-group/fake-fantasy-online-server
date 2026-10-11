"""Mutation tests for authored compiler boundaries; no runtime output is written."""
import copy,json,unittest
import compile_astral as compiler

class AstralCompilerTest(unittest.TestCase):
    def setUp(self):
        self.doc=json.loads(compiler.SOURCE.read_text())
    def test_current_source(self):
        self.assertIn('astralRuneCatalog',compiler.compile_catalog(self.doc))
    def test_unsupported_rune_modes(self):
        for index,affix in [(0,{'attr':25,'value':-100,'mode':0}), (0,{'attr':38,'value':10,'mode':0}), (0,{'attr':40,'value':100,'mode':0}), (27,{'attr':38,'value':10,'mode':1}), (27,{'attr':38,'value':-1,'mode':0})]:
            with self.subTest(index=index,affix=affix):
                doc=copy.deepcopy(self.doc);doc['runes'][index]['affixes']=[affix]
                with self.assertRaises(AssertionError):compiler.compile_catalog(doc)
    def test_supported_rune_redefinition_needs_versioned_migration(self):
        for key,value in [('id',1900028999),('name','另一张星篆'),('affixes',[{'attr':1,'value':4,'mode':0}])]:
            with self.subTest(key=key):
                doc=copy.deepcopy(self.doc);doc['runes'][0][key]=value
                with self.assertRaises(AssertionError):compiler.compile_catalog(doc)
    def test_unknown_or_inert_formation_traits(self):
        for trait,professions in [({'kind':'not_a_trait','powerBP':100,'thresholdPct':0},31),({'kind':'fire_mastery','powerBP':100,'thresholdPct':0},4),({'kind':'swift_cast','powerBP':100,'thresholdPct':0,'skillID':10301,'skillName':'治愈术'},8),({'kind':'skill_focus','powerBP':100,'thresholdPct':0,'skillName':'冒名'},31)]:
            with self.subTest(trait=trait):
                doc=copy.deepcopy(self.doc);doc['formations'][0]['traits']=[trait];doc['formations'][0]['professions']=professions
                with self.assertRaises(AssertionError):compiler.compile_catalog(doc)
if __name__=='__main__':unittest.main()
