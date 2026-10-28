package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"testing"
)

func TestLegendaryMeleeAdditionalUnequippedControls(t *testing.T) {
	t.Run("JK-A18", func(t *testing.T) {
		for _, equipped := range []bool{false, true} {
			s, p, m := swordsmanCoverageFixture(t, "JK-A18", equipped)
			m.Stats.Def = 100
			s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, Stats: m.Stats}}
			m.Status = domain.NewStatusSet()
			legendaryMeleeStrike(s, p, m, 10122)
			want := int32(100)
			if equipped {
				want = 70
			}
			if m.Stats.Def != want {
				t.Fatalf("equipped=%v armor=%d want%d", equipped, m.Stats.Def, want)
			}
		}
	})
	t.Run("JK-A20", func(t *testing.T) {
		for _, equipped := range []bool{false, true} {
			s, p, m := swordsmanCoverageFixture(t, "JK-A20", equipped)
			side := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
			swordCast(t, s, p, m, swordsmanCoverageDef(10116))
			want := int32(0)
			if equipped {
				want = 160
			}
			if 10000-side.HP != want {
				t.Fatalf("equipped=%v secondary damage=%d want%d", equipped, 10000-side.HP, want)
			}
		}
	})
}

// Slot order must not change the independent producer→native leech→pure
// healing-strength chain. The HP oracle is derived from each actual outgoing
// strike, not a previously observed total from one equipment ordering.
func TestLegendarySwordsmanLeechInteractionSlotOrders(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		powers := []string{"JK-A03", "TY-A17"}
		if reverse {
			powers = []string{"TY-A17", "JK-A03"}
		}
		s, p, m := meleeAuditNativeScene(t, domain.Swordsman, domain.Learned{10102: 1}, powers...)
		p.Status = domain.NewStatusSet()
		s.applyStatusDef(p, domain.StatusDef{ID: 190946, SkillApplied: true, Level: 1, DurationSec: 10, TypeKnown: true, Type: domain.StatusGood, LifeStealPct: 100, LifeStealChance: 100}, p.ID)
		p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
		p.HP = 1000
		s.outbox = nil
		swordCast(t, s, p, m, swordsmanCoverageDef(10102))
		want := int32(1000)
		hits := 0
		for _, msg := range s.outbox {
			if e, ok := msg.ev.(event.DamageDealt); ok && e.Src == p.ID && e.Dst == m.ID && e.Amount > 0 && !e.Flag.Has(event.DamageMiss) {
				want += e.Amount * 160 / 100
				hits++
			}
		}
		if hits != 2 || p.HP != want {
			t.Fatalf("reverse=%v hits=%d HP=%d want%d", reverse, hits, p.HP, want)
		}
	}
}

func TestLegendarySwordsmanOwnPureModifierInsideProducerLineage(t *testing.T) {
	for _, tc := range []struct {
		id                 string
		skill              domain.SkillID
		coefficient, bonus int32
	}{{"JK-A07", 10103, 45, 110}, {"JK-A22", 10123, 70, 120}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m := swordsmanCoverageFixture(t, tc.id, true)
			side := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
			ok, restore := s.enterCombatEffect(p.ID, tc.id)
			if !ok {
				t.Fatal("first producer entry denied")
			}
			before := m.HP
			hit, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{SkillID: int32(tc.skill), SkillPct: tc.coefficient, Origin: domain.DamageProcOrigin, ProcDepth: 1, GuaranteedHit: true, PrimaryTarget: m.ID})
			restore()
			want := tc.coefficient * tc.bonus / 100
			if hit.Amount != want || before-m.HP != want || side.HP != 10000 {
				t.Fatalf("own pure bonus must apply once without another producer wave: hit=%d loss=%d side=%d want=%d", hit.Amount, before-m.HP, side.HP, want)
			}
			if len(s.legendaryState().causalPath) != 0 {
				t.Fatal("producer scope leaked")
			}
		})
	}
}

func TestLegendarySwordsmanFreeBindConsumesNoMana(t *testing.T) {
	t.Run("JK-U06", func(t *testing.T) {
		for _, equipped := range []bool{false, true} {
			s, p, m := swordsmanCoverageFixture(t, "JK-U06", equipped)
			p.MP = 500
			d := swordsmanCoverageDef(10111)
			d.MPCost = 100
			d.Dist = 100
			swordsmanCoverageNativeCast(t, s, p, d, m.ID)
			want := int32(400)
			if equipped {
				want = 500
			}
			if p.MP != want {
				t.Fatalf("equipped=%v actual paid MP=%d want%d", equipped, p.MP, want)
			}
			if m.HP == m.MaxHP {
				t.Fatal("zero payment must still execute native attack")
			}
		}
	})
}
