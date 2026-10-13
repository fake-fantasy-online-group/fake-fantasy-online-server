"""Mutation tests for the actual authored unique loader, without generation."""
import copy
import json
import pathlib
import tempfile
import unittest
from unittest.mock import patch

import generate as compiler


def good_affixes():
    return [{'Attr': attr, 'Value': -10 if attr == 25 else 20, 'Mode': 1 if attr == 25 else 0}
            for attr in (1, 5, 19, 25, 34)]


class UniqueExtraAffixContractTest(unittest.TestCase):
    def setUp(self):
        self.row = dict(BaseID=1016, Name='校场之枪', Lore='枪缨留着旧日的烟痕。', LoreRoot='原创',
                        Identity='近身枪', Benefit='五项可执行增益', Tradeoff='无施法专精',
                        PowerTier='transition', Affixes=good_affixes(), Traits=[], ReservedSkills=[])

    def load(self, row):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory)
            (path/'warrior.json').write_text(json.dumps([row], ensure_ascii=False))
            with patch.object(compiler, 'AUTHORED', path):
                return compiler.read_authored({1016: ({}, {}, {}, 1)})

    def test_five_or_more_beneficial_extras_and_optional_curses(self):
        for tier in sorted(compiler.POWER_TIERS):
            row = copy.deepcopy(self.row)
            row['PowerTier'] = tier
            row['Affixes'] += [{'Attr': 7, 'Value': 12, 'Mode': 0},
                              {'Attr': 9, 'Value': 14, 'Mode': 0},
                              {'Attr': 47, 'Value': 18, 'Mode': 1},
                              {'Attr': 17, 'Value': -8, 'Mode': 0},
                              {'Attr': 36, 'Value': -12, 'Mode': 0}]
            self.assertEqual(len(self.load(row)[1016]['Affixes']), 10)

    def test_four_benefits_cannot_be_padded_with_other_effects(self):
        row = copy.deepcopy(self.row)
        row['Affixes'] = row['Affixes'][:4]
        row['Traits'] = [{'Kind': 'basic_focus', 'PowerBP': 2000, 'ThresholdPct': 0}]
        row['ReservedSkills'] = [{'Scope': 'all', 'Target': '', 'Levels': 3}]
        row['Affixes'] += [{'Attr': 34, 'Value': -200, 'Mode': 0}]
        with self.assertRaisesRegex(AssertionError, 'at least five beneficial'):
            self.load(row)

    def test_duplicate_attribute_even_different_mode(self):
        for mode in (0, 1):
            row = copy.deepcopy(self.row)
            row['Affixes'].append({'Attr': 1, 'Value': 20, 'Mode': mode})
            with self.subTest(mode=mode), self.assertRaisesRegex(AssertionError, 'duplicate extra Attr'):
                self.load(row)

    def test_percentage_attack_aliases_are_not_extra_distinct_benefits(self):
        row = copy.deepcopy(self.row)
        row['Affixes'] += [{'Attr': 29, 'Value': 20, 'Mode': 1}, {'Attr': 47, 'Value': 20, 'Mode': 1}]
        with self.assertRaisesRegex(AssertionError, 'duplicate percentage attack'):
            self.load(row)

    def test_inactive_invalid_and_zero_affixes_are_rejected(self):
        for attr, value, mode in [(38, 20, 0), (40, 20, 0), (23, 20, 1), (66, 20, 1),
                                  (25, -20, 0), (1, 0, 0), (999, 20, 0), (1, 20, 2),
                                  (True, 20, 0), (1, True, 0), (1, 20, False),
                                  (1, 2.5, 0), (1, 2147483648, 0),
                                  (69, 20, 1), (70, 20, 1), (118, 20, 1)]:
            row = copy.deepcopy(self.row)
            row['Affixes'][0] = dict(Attr=attr, Value=value, Mode=mode)
            with self.subTest(affix=row['Affixes'][0]), self.assertRaisesRegex(AssertionError, 'unsupported extra affix'):
                self.load(row)

    def test_unique_lines_cannot_vanish_or_overflow_at_legal_potency(self):
        for value, reason in [(1, 'vanishes'), (-1, 'vanishes'), (2147483647, 'overflows'), (-2147483648, 'overflows')]:
            row = copy.deepcopy(self.row)
            row['Affixes'][0]['Value'] = value
            with self.subTest(value=value), self.assertRaisesRegex(AssertionError, reason):
                self.load(row)

    def test_legacy_tiers_are_rejected(self):
        for tier in ('niche', 'specialist', 'chase', ''):
            row = copy.deepcopy(self.row)
            row['PowerTier'] = tier
            with self.subTest(tier=tier), self.assertRaisesRegex(AssertionError, 'PowerTier'):
                self.load(row)


if __name__ == '__main__':
    unittest.main()
