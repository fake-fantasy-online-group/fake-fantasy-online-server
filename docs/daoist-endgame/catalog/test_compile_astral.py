"""Mutation tests for authored compiler boundaries; no runtime output is written."""
import copy,json,unittest
import compile_astral as compiler

class AstralCompilerTest(unittest.TestCase):
    def setUp(self):
        self.doc=json.loads(compiler.SOURCE.read_text())
        # Mutation cases start with a known-valid effect budget, so a bad
        # unrelated source row cannot make their negative assertions pass.
        for r in self.doc['formations']:
            r['powerTier']='transition'
            r['affixes']=[{'attr':a,'value':20,'mode':0} for a in (1,5,19,34,47)]
    def test_current_source(self):
        self.assertIn('astralRuneCatalog',compiler.compile_catalog(json.loads(compiler.SOURCE.read_text())))
    def test_five_or_more_extra_attributes(self):
        for tier in sorted(compiler.POWER_TIERS):
            doc=copy.deepcopy(self.doc);r=doc['formations'][0];r['powerTier']=tier
            r['affixes'] += [{'attr':a,'value':20,'mode':0} for a in (7,9,17)]
            r['affixes'].append({'attr':25,'value':-1,'mode':1})
            self.assertIn('astralRecipeCatalog',compiler.compile_catalog(doc))
    def test_runes_traits_and_reserved_skills_cannot_pad_four_benefits(self):
        doc=copy.deepcopy(self.doc);r=doc['formations'][0]
        r['affixes']=r['affixes'][:4]+[{'attr':47,'value':-20,'mode':0}]
        r['traits']=[{'kind':'basic_focus','powerBP':2000,'thresholdPct':0}]
        r['reservedSkills']=[{'scope':'all','target':'','levels':3}]
        with self.assertRaisesRegex(AssertionError,'at least five beneficial'):compiler.compile_catalog(doc)
    def test_duplicate_attributes_and_percentage_aliases(self):
        for additions,reason in [([{'attr':1,'value':20,'mode':0}],'duplicate extra Attr'),
                                 ([{'attr':1,'value':20,'mode':1}],'duplicate extra Attr'),
                                 ([{'attr':29,'value':20,'mode':1},{'attr':31,'value':20,'mode':1}],'duplicate percentage attack')]:
            doc=copy.deepcopy(self.doc);doc['formations'][0]['affixes']+=additions
            with self.subTest(additions=additions),self.assertRaisesRegex(AssertionError,reason):compiler.compile_catalog(doc)
    def test_unsupported_formation_modes(self):
        for attr,value,mode in [(38,10,0),(40,100,0),(25,-100,0),(23,5,1),(66,5,1),(1,0,0),(1,True,0),(1,20,False),(1,2147483648,0),(69,20,1),(70,20,1),(118,20,1)]:
            doc=copy.deepcopy(self.doc);doc['formations'][0]['affixes'][0]={'attr':attr,'value':value,'mode':mode}
            with self.subTest(affix=doc['formations'][0]['affixes'][0]),self.assertRaisesRegex(AssertionError,'unsupported extra affix'):compiler.compile_catalog(doc)
    def test_legacy_tiers_are_rejected(self):
        for tier in ('niche','specialist','chase'):
            doc=copy.deepcopy(self.doc);doc['formations'][0]['powerTier']=tier
            with self.subTest(tier=tier),self.assertRaises(AssertionError):compiler.compile_catalog(doc)
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
