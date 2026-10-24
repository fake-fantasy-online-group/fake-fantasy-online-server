package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func sourceLevelLootRules(t *testing.T) *domain.LootRuleTable {
	t.Helper()
	rule := *lootRulesFixture()
	rule.UniqueBasePPB = domain.LootProbabilityScaleV5
	rule.UniqueMFBoost = 0
	rules, err := domain.NewLootRuleTable(rule.Version, []domain.LootRuleSet{rule}, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func TestLegendarySourceLevelNaturalKillAndFixedRows(t *testing.T) {
	fixture, def := legendaryDropFixture(t)
	def.Equip.Slot = int32(domain.SlotFace)
	def.Equip.LevelReq = 18
	def.Equip.Need = domain.Requirement{Level: 18}
	for _, fixed := range []bool{false, true} {
		s, id := lootScene(t, []domain.DropEntry{{Item: def.ID, RatePct: 100, RollAffixes: true}})
		s.items[def.ID] = def
		s.equipmentRoll = fixture.equipmentRoll
		s.lootRules = sourceLevelLootRules(t)
		dead := s.entities[id]
		dead.Level = 43
		if fixed {
			dead.Monster.Kind = domain.MonsterBoss
			s.lootTable[dead.Monster.TypeID] = nil
			s.bossFixedDrops = domain.BossFixedDropTable{{MapID: s.id.MapID, Monster: dead.Monster.TypeID}: {Minimum: 1, Pool: []domain.BossFixedDropChoice{{Item: def.ID, Weight: 1, RollAffixes: true}}}}
		}
		aboveWear := false
		for i := 0; i < 64; i++ {
			s.tick = domain.Tick(i)
			s.rollLootWithContextV5(dead, domain.LootContextV5{Version: 5})
			if len(s.ground) != 1 {
				t.Fatal("original route count changed", fixed, len(s.ground))
			}
			for id, drop := range s.ground {
				st := drop.Drop.Stack
				p, ok := domain.LegendaryPower(st.Legendary.PowerID)
				if !ok || st.Endgame.SourceLevel != 43 || p.MinLevel > 43 || domain.EquipmentRequiredLevel(def, st) != 18 {
					t.Fatal("wrong source/wear in kill path", fixed, st)
				}
				aboveWear = aboveWear || p.MinLevel > 18
				s.removeLoot(id, 0)
			}
		}
		if !aboveWear {
			t.Fatal("real kill/fixed row never produced above-wear power", fixed)
		}
	}
}

func TestLegendarySourceLevelTheftIgnoresPlayerLevelAndLearnedSkills(t *testing.T) {
	fixture, base := legendaryDropFixture(t)
	for _, tc := range []struct{ source, wear int32 }{{43, 18}, {12, 60}} {
		def := base
		e := *base.Equip
		def.Equip = &e
		e.Slot = int32(domain.SlotFace)
		e.LevelReq = tc.wear
		e.Need = domain.Requirement{Level: tc.wear}
		s, src, dst, _ := theftTestScene(t, 10000)
		s.items[def.ID] = def
		s.equipmentRoll = fixture.equipmentRoll
		s.lootRules = sourceLevelLootRules(t)
		s.lootTable[dst.Monster.TypeID] = []domain.DropEntry{{Item: def.ID, RatePct: 100, RollAffixes: true}}
		dst.Level = tc.source
		skill := s.skills[domain.SkillKey{ID: 10212, Level: 1}]
		aboveWear := false
		for i := 0; i < 64; i++ {
			var first domain.LegendaryInstance
			for _, playerLevel := range []int32{1, 100} {
				src.Level, src.Player.Char.Level = playerLevel, playerLevel
				src.Player.Char.Skills = domain.Learned{} // The drop does not consult a learned-power skill gate.
				s.tick = domain.Tick(i)
				dst.Monster.TheftSuccesses = 0
				if !s.stealMonsterItem(src, dst, skill, skill.Effects[0]) {
					t.Fatal("theft fixture failed")
				}
				drop := onlyTheftDrop(t, s)
				st := drop.Drop.Stack
				p, ok := domain.LegendaryPower(st.Legendary.PowerID)
				if !ok || st.Endgame.SourceLevel != tc.source || p.MinLevel > tc.source || domain.EquipmentRequiredLevel(def, st) != tc.wear {
					t.Fatal("theft source/wear wrong", tc, st)
				}
				if playerLevel == 1 {
					first = st.Legendary
				} else if first != st.Legendary {
					t.Fatal("player level changed power/affix/tier roll")
				}
				aboveWear = aboveWear || p.MinLevel > tc.wear
				s.removeLoot(drop.ID, 0)
			}
		}
		if tc.source > tc.wear && !aboveWear {
			t.Fatal("theft low-wear slot omitted higher source powers")
		}
	}
}

func TestLegendarySourceLevelRealSkillThresholdWithoutLearnedGate(t *testing.T) {
	skills, _ := realV5SkillFixtures(t)
	skill, ok := skills.Get(10002, 1)
	if !ok || skill.LevelNeed != 10 {
		t.Fatal("real skill threshold fixture changed", skill.LevelNeed)
	}
	p, ok := domain.LegendaryPower("ZS-A01")
	if !ok || p.MinLevel != skill.LevelNeed {
		t.Fatal("approved threshold not preserved")
	}
	_, def := legendaryDropFixture(t)
	def.Equip.LevelReq = 1
	for _, source := range []int32{skill.LevelNeed - 1, skill.LevelNeed, 43} {
		if got := domain.LegendaryPowerEligibleAtLevel(p, def, source); got != (source >= skill.LevelNeed) {
			t.Fatal("real skill source boundary mismatch", source)
		}
	}
}
