package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"fmt"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func growthAttributePlayer(t *testing.T, profession int32) (*Scene, *entity.Entity) {
	t.Helper()
	s, p := behaviorAcceptancePlayer(t, profession)
	p.Player.Char.Base = domain.Base{STR: 100, VIT: 100, INT: 100, SPI: 100, AGI: 100, DEX: 100}
	p.Player.Char.Employed, p.Player.Char.EmploymentKnown = true, true
	s.refreshStats(p)
	return s, p
}

func growthBaseAttribute(b domain.Base, attr int32) int32 {
	switch attr {
	case domain.AttrVIT:
		return b.VIT
	case domain.AttrINT:
		return b.INT
	case domain.AttrSPI:
		return b.SPI
	}
	return 0
}

func TestSkillGrowthAttributesAllBoundariesV7(t *testing.T) {
	cases := []struct {
		id        domain.SkillID
		cap, attr int32
	}{
		{10024, 4, domain.AttrVIT}, {10222, 10, domain.AttrVIT}, {10229, 11, domain.AttrVIT}, {10230, 11, domain.AttrVIT},
		{10314, 17, domain.AttrINT}, {10415, 17, domain.AttrINT}, {10416, 5, domain.AttrINT}, {10329, 5, domain.AttrINT}, {10420, 5, domain.AttrINT},
		{10306, 5, domain.AttrSPI}, {10316, 5, domain.AttrSPI}, {10318, 5, domain.AttrSPI}, {10433, 5, domain.AttrSPI}, {10218, 7, domain.AttrSPI},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.id), func(t *testing.T) {
			ident, _ := domain.V5SkillIdentity(tc.id, catalogfixture.SkillProgressionCatalog())
			s, p := growthAttributePlayer(t, ident.Profession)
			original := p.Player.Char.Base
			for _, rank := range []int32{0, 1, 2, 3, 4, 5, tc.cap, tc.cap + 1, 20, 21, 100} {
				p.Player.Char.Skills = domain.Learned{tc.id: min(rank, 20)}
				s.skillRawCache = map[domain.EntityID]skillRawCacheEntry{p.ID: {key: skillRawFingerprint(p), levels: domain.Learned{tc.id: rank}}}
				src := domain.StatSource{Base: original, Level: 100, Worn: p.Player.Worn, Defs: s.itemDef, Passive: s.passiveStatAffixesIncludingGrowth(p, newEquipmentRequirementResolution(), false)}
				without, _ := domain.Compute(src)
				src.Passive = s.passiveStatAffixes(p)
				with, stats := domain.Compute(src)
				want := max(int32(0), rank-tc.cap)
				if got := growthBaseAttribute(with, tc.attr) - growthBaseAttribute(without, tc.attr); got != want {
					t.Fatalf("rank%d attr%d got%d want%d", rank, tc.attr, got, want)
				}
				derived := domain.DeriveFromBase(with)
				if tc.attr == domain.AttrVIT && stats.MaxHP != derived.MaxHP || tc.attr == domain.AttrSPI && (stats.MaxMP != derived.MaxMP || stats.MDef != derived.MDef) {
					t.Fatalf("growth did not use primary-stat pipeline: base%+v stats%+v", with, stats)
				}
				if p.Player.Char.Base != original {
					t.Fatal("growth mutated permanent base")
				}
				// A passive with two capped axes still contributes one point per rank.
				if tc.id == 10314 && rank == 20 && with.INT-original.INT != 3 {
					t.Fatal("casting/interruption double counted", with.INT)
				}
			}
			p.Player.Char.Race = domain.Race(ident.Profession % 5)
			s.skillRawCache = nil
			if got := s.passiveStatAffixes(p); len(got) > 0 {
				t.Fatalf("wrong profession retained growth: %+v", got)
			}
			p.Player.Char.Race = domain.Race(ident.Profession - 1)
			p.Player.Char.Skills = domain.Learned{}
			s.skillRawCache = nil
			if got := s.passiveStatAffixes(p); len(got) > 0 {
				t.Fatalf("unlearned skill retained growth: %+v", got)
			}
		})
	}
}

func TestSkillGrowthAttributesResourcesAndPanelV7(t *testing.T) {
	for _, tc := range []struct {
		profession int32
		id         domain.SkillID
		cap        int32
		attrSlot   int
	}{{1, 10024, 4, 11}, {4, 10306, 5, 14}, {5, 10416, 5, 13}} {
		t.Run(fmt.Sprint(tc.id), func(t *testing.T) {
			s, p := growthAttributePlayer(t, tc.profession)
			original := p.Player.Char.Base
			p.Player.Char.Skills = domain.Learned{tc.id: tc.cap}
			s.refreshStats(p)
			baseHP, baseMP := p.MaxHP, p.MaxMP
			p.HP, p.MP = 1, 0
			for i := 0; i < 3; i++ {
				p.Player.Char.Skills[tc.id] = 20
				s.refreshStats(p)
				want := int32(100 + 20 - tc.cap)
				if got := p.Player.Char.Attrs[tc.attrSlot]; got != want {
					t.Fatalf("panel=%d want%d", got, want)
				}
				if p.HP != 1 || p.MP != 0 {
					t.Fatalf("skill increase granted resources: HP%d MP%d", p.HP, p.MP)
				}
				total := p.Stats
				s.refreshStats(p)
				if p.Stats != total || p.HP != 1 || p.MP != 0 {
					t.Fatal("repeated refresh stacked growth")
				}
				p.Player.Char.Skills[tc.id] = tc.cap
				s.refreshStats(p)
				if p.MaxHP != baseHP || p.MaxMP != baseMP || p.HP != 1 || p.MP != 0 {
					t.Fatal("rank decrease retained growth or resources")
				}
			}
			p.Player.Char.Skills[tc.id] = 20
			s.refreshStats(p)
			p.HP, p.MP = p.MaxHP, p.MaxMP
			p.Player.Char.Skills = domain.Learned{}
			s.refreshStats(p)
			if p.HP > p.MaxHP || p.MP > p.MaxMP || p.HP < 0 || p.MP < 0 {
				t.Fatal("unlearning did not clamp resources")
			}
			p.HP = 0
			p.Player.Char.Skills[tc.id] = 20
			s.refreshStats(p)
			if p.HP != 0 {
				t.Fatal("skill growth revived dead character")
			}
			p.HP = 1 // ordinary resurrection baseline remains stable through refresh
			s.refreshStats(p)
			if p.HP != 1 {
				t.Fatal("resurrection refresh granted growth health")
			}
			if p.Player.Char.Base != original {
				t.Fatal("base changed")
			}
		})
	}
}

func TestReplacedPassiveCompensationsRemovedV7(t *testing.T) {
	for _, rank := range []int32{5, 20, 21, 100} {
		s, p := growthAttributePlayer(t, 3)
		p.Player.Char.Skills = domain.Learned{10222: min(rank, 20), 10229: min(rank, 20), 10230: min(rank, 20)}
		levels := domain.Learned{10222: rank, 10229: rank, 10230: rank}
		s.skillRawCache = map[domain.EntityID]skillRawCacheEntry{p.ID: {key: skillRawFingerprint(p), levels: levels}}
		for _, kind := range []domain.PassiveModifierKind{domain.PassiveTrapHalfDamageChance, domain.PassiveIncomingMagicHalfChance} {
			if got := s.passiveHalfDamage(p, kind, 101); got != 51 {
				t.Fatalf("rank%d half-damage deepened to %d", rank, got)
			}
		}
		blow := combat.Blow{SkillPct: 100}
		s.prepareBasicAttackStatuses(p, &blow)
		if blow.SkillPct != 100 {
			t.Fatal("dark weapon recovery still boosts basic damage", blow.SkillPct)
		}
		p.Player.Char.Race = domain.Warlock
		d, _ := s.skills.Get(10401, rank)
		before := d.ManaCostReductionRanks
		s.applyPassiveSkillModifiersWithLevels(p, &d, domain.Learned{10415: rank, 10416: rank})
		if d.ManaCostReductionRanks != before {
			t.Fatal("cast passive still gives overflow mana savings")
		}
		p.Player.Char.Race = domain.Warrior
		d, _ = s.skills.Get(10003, rank)
		before = d.ManaCostReductionRanks
		s.applyPassiveSkillModifiersWithLevels(p, &d, domain.Learned{10024: rank})
		if d.ManaCostReductionRanks != before {
			t.Fatal("maintenance still gives overflow mana savings")
		}
		resist, _ := s.skills.Get(10008, rank)
		for _, r := range resist.PassiveResists {
			if r.ProgressionRestoreMP != 0 {
				t.Fatal("resistance still rewards MP")
			}
		}
	}
}

func growthAllSkillGear(s *Scene) (domain.ItemDef, domain.Stack) {
	item := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 2409, Equip: &domain.EquipDef{Slot: int32(domain.SlotGlove), Durable: 100}}
	s.items = map[domain.ItemID]domain.ItemDef{item.ID: item}
	st := domain.NewStack(item, 1)
	st.UID = 992409
	st.Endgame = domain.EndgameItemState{UniqueID: 2409, UniqueRollBP: 10000, UniqueContentVersion: domain.UniqueContentVersion}
	return item, st
}

func TestSkillGrowthRealSoftRanksAndEquipmentResourcesV7(t *testing.T) {
	for _, tc := range []struct {
		prof int32
		id   domain.SkillID
		cap  int32
	}{{1, 10024, 4}, {4, 10306, 5}, {5, 10415, 17}} {
		t.Run(fmt.Sprint(tc.id), func(t *testing.T) {
			s, p := growthAttributePlayer(t, tc.prof)
			_, gear := growthAllSkillGear(s)
			p.Player.Char.Skills = domain.Learned{tc.id: tc.cap}
			s.refreshStats(p)
			hard := p.Player.Char.Skills.Clone()
			p.HP, p.MP = 500, 400
			for n := 0; n < 3; n++ {
				oldHP, oldMP := p.HP, p.MP
				_, before, gBefore := s.computeSkillGrowthStats(p, s.functionalWorn(p))
				p.Player.Worn.Set(domain.SlotGlove, gear)
				s.refreshStats(p)
				rank := s.rawSkillLevels(p)[tc.id]
				if rank != tc.cap+1 || p.Player.Char.Skills[tc.id] != hard[tc.id] {
					t.Fatalf("soft rank=%d hard=%d", rank, p.Player.Char.Skills[tc.id])
				}
				_, after, gAfter := s.computeSkillGrowthStats(p, s.functionalWorn(p))
				// Existing equipment resources still behave exactly as before; only new
				// skill growth is removed from the delta, including percentage scaling.
				wantHP := min(after.MaxHP, max(int32(1), oldHP+(after.MaxHP-gAfter.MaxHP)-(before.MaxHP-gBefore.MaxHP)))
				wantMP := min(after.MaxMP, max(int32(0), oldMP+(after.MaxMP-gAfter.MaxMP)-(before.MaxMP-gBefore.MaxMP)))
				if p.HP != wantHP || p.MP != wantMP {
					t.Fatalf("gear+growth delta HP/MP=%d/%d want%d/%d", p.HP, p.MP, wantHP, wantMP)
				}
				p.Player.Worn.Set(domain.SlotGlove, domain.Stack{})
				s.refreshStats(p)
				if s.rawSkillLevels(p)[tc.id] != tc.cap || p.Player.SkillGrowthMaxHP != 0 || p.Player.SkillGrowthMaxMP != 0 {
					t.Fatal("unequip retained soft growth")
				}
				if p.HP != oldHP || p.MP != oldMP {
					t.Fatalf("gear cycle minted resources %d/%d ->%d/%d", oldHP, oldMP, p.HP, p.MP)
				}
			}
			p.Player.Char.Skills = domain.Learned{}
			p.Player.Worn.Set(domain.SlotGlove, gear)
			s.refreshStats(p)
			if s.rawSkillLevels(p)[tc.id] != 0 || p.Player.SkillGrowthMaxHP != 0 || p.Player.SkillGrowthMaxMP != 0 {
				t.Fatal("soft item learned an unlearned growth skill")
			}
		})
	}
}

func TestSkillGrowthCannotSelfQualifyEquipmentV7(t *testing.T) {
	s, p := growthAttributePlayer(t, 1)
	item, gear := growthAllSkillGear(s)
	item.Equip.Need.Base.VIT = 101
	p.Player.Char.Skills = domain.Learned{10024: 4}
	p.Player.Worn.Set(domain.SlotGlove, gear)
	if got := s.functionalWorn(p).Count(); got != 0 {
		t.Fatalf("soft growth met its own equipment requirement: %d", got)
	}
	// Two individually unusable sources cannot bootstrap one another.
	ring := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 990001, Equip: &domain.EquipDef{Slot: int32(domain.SlotRing), Need: domain.Requirement{Base: domain.Base{VIT: 101}}, Base: domain.Base{VIT: 1}}}
	s.items[ring.ID] = ring
	p.Player.Worn.Set(domain.SlotRing, domain.NewStack(ring, 1))
	s.skillRawCache = nil
	if got := s.functionalWorn(p).Count(); got != 0 {
		t.Fatalf("mutually supporting growth gear activated: %d", got)
	}
	// A real hard-rank point does qualify it. Subsequent unlearning revokes both.
	p.Player.Char.Skills[10024] = 5
	s.refreshStats(p)
	if s.functionalWorn(p).Count() != 2 || s.rawSkillLevels(p)[10024] != 6 {
		t.Fatal("independently qualified skill growth rejected")
	}
	p.Player.Char.Skills[10024] = 4
	s.refreshStats(p)
	if s.functionalWorn(p).Count() != 0 || s.rawSkillLevels(p)[10024] != 4 || p.Player.Char.Attrs[11] != 100 {
		t.Fatal("removed hard growth left a self-supporting equipment cycle")
	}
}

func TestPersistentGrowthRebuildKeepsEffectHitSnapshotV7(t *testing.T) {
	for _, id := range []domain.SkillID{10211, 10435} {
		ident, _ := domain.V5SkillIdentity(id, catalogfixture.SkillProgressionCatalog())
		s, p := growthAttributePlayer(t, ident.Profession)
		p.Player.Char.Skills = domain.Learned{id: 20}
		def, _ := s.skills.Get(id, 20)
		def.SnapshotRawRanks = s.rawSkillLevels(p)
		def.SnapshotHardRanks = p.Player.Char.Skills.Clone()
		if len(def.Statuses) == 0 {
			t.Fatal("carrier lacks status", id)
		}
		old, ok := s.resolveSkillStatusDef(def, def.Statuses[0], 0)
		if !ok {
			t.Fatal("carrier did not resolve", id)
		}
		old.SourceEffectHitBP, old.SourceEffectHitKnown = 700, true
		old.Icon, old.SourceSkillID = 99, int32(id)
		p.Stats.EffectHitBP = 5000
		p.Player.Char.Skills[id] = 19
		next, keep, changed := s.resolvePersistentStatus(p, p, old)
		if !keep || !changed || next.SkillSnapshotRank != 19 || next.SourceEffectHitBP != 700 || !next.SourceEffectHitKnown || next.Icon != 99 {
			t.Fatalf("carrier snapshot changed: %+v", next)
		}
		if next.FreezeAttackerEffectHitBP > old.FreezeAttackerEffectHitBP || next.PoisonOnHitProgressionRanks > old.PoisonOnHitProgressionRanks {
			t.Fatal("decreased skill upgraded carrier payload")
		}
		p.Player.Char.Skills[id] = 20
		unchanged, keep, changed := s.resolvePersistentStatus(p, p, next)
		if !keep || changed || unchanged.SkillSnapshotRank != 19 {
			t.Fatal("releveling upgraded old snapshot")
		}
		p.Player.Char.Skills = domain.Learned{}
		if _, keep, _ := s.resolvePersistentStatus(p, p, next); keep {
			t.Fatal("unlearned source retained carrier")
		}
	}
}
