package scene

import (
	"fmt"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// Exercise the runtime consumers independently of SQL attribute/card bonuses.
// Every count is intentional: six/eight pieces retain both earlier skill tiers.
func TestClassSetFiveClassesSkillThresholdsZeroThroughEight(t *testing.T) {
	for _, tc := range []struct {
		suit     int32
		name     string
		modifier string
		value    int32
	}{
		{1, "assassin", "class_set_crit_rate_bp", 500},
		{2, "healer", "class_set_magic_mitigation_bp", 800},
		{3, "warlock", "class_set_crit_damage_bp", 2000},
		{4, "swordsman", "class_set_physical_mitigation_bp", 800},
		{5, "warrior", "class_set_max_hp_bp", 1500},
	} {
		for pieces := 0; pieces <= 8; pieces++ {
			t.Run(fmt.Sprintf("%s/%d", tc.name, pieces), func(t *testing.T) {
				lower, upper := pieces >= 2, pieces >= 4
				var s *Scene
				var p *entity.Entity
				switch tc.suit {
				case 1:
					var target *entity.Entity
					s, p, target = classPoisonScene(t, pieces)
					poison := s.statuses[domain.StatusKey{ID: domain.StatusPoison, Level: 21}]
					s.prepareClassSetPoison(p, domain.StatusDef{ID: 1018}, &poison)
					want := int64(0)
					if lower {
						want = 1000
					}
					if poison.ClassSetPoisonBonusTotal != want {
						t.Fatalf("poison attack bonus=%d want%d", poison.ClassSetPoisonBonusTotal, want)
					}
					// Existing poison has exactly 300 damage left, independent of
					// the incoming poison's lower-tier attack snapshot.
					old := poison
					old.ClassSetPoisonBonusTotal = 0
					target.Status.Apply(old, s.tick, p.ID)
					burst := int32(0)
					if upper {
						burst = 60
					}
					if got := s.classSetPoisonBurst(target, poison, p.ID); got != burst {
						t.Fatalf("poison burst=%d want%d", got, burst)
					}
				case 2:
					var targetID domain.EntityID
					s, p, targetID = casterSetFixture(t, 2, pieces)
					join(t, s, 2, 101, "ally", 120, 100)
					ally := s.entities[2]
					p.Player.Party, ally.Player.Party = 7, 7
					p.HP = 100
					s.casterSetCopyHeal(p, ally, 100, 10301)
					wantHP := int32(100)
					if lower {
						wantHP = 200
					}
					if p.HP != wantHP {
						t.Fatalf("copied healing HP=%d want%d", p.HP, wantHP)
					}
					p.MP = 0
					s.stepCasterSets()
					s.tick += domain.Ticks(3000)
					s.stepCasterSets()
					wantMP := int32(0)
					if lower {
						wantMP = 30
					}
					if p.MP != wantMP {
						t.Fatalf("three-second mana=%d want%d", p.MP, wantMP)
					}
					area := s.captureCasterSetArea(p, 治愈术, ally.ID, nil)
					if (area != nil) != upper {
						t.Fatalf("reciprocal area enabled=%v want%v", area != nil, upper)
					}
					target := s.entities[targetID]
					target.Stats.MDef = 0
					before := target.HP
					s.finishCasterSetCast(p, pendingCast{def: 治愈术, classSetArea: area}, []domain.EntityID{ally.ID})
					wantDamage := int32(0)
					if upper {
						wantDamage = 100
					}
					if got := before - target.HP; got != wantDamage {
						t.Fatalf("reciprocal bolt=%d want%d", got, wantDamage)
					}
				case 3:
					var targetID domain.EntityID
					s, p, targetID = casterSetFixture(t, 3, pieces)
					s.startCasterSetCast(p, 火弹术)
					wantDamage := int32(100)
					wantViews := 0
					if lower {
						wantDamage, wantViews = 150, 1
					}
					if got := s.casterSetOutgoingDamage(p, domain.HurtIce, 100); got != wantDamage {
						t.Fatalf("elemental damage=%d want%d", got, wantDamage)
					}
					if got := len(s.classSetCasterPresentation(p)); got != wantViews {
						t.Fatalf("elemental buff views=%d want%d", got, wantViews)
					}
					target := s.entities[targetID]
					target.Stats.MDef = 0
					before := target.HP
					s.finishCasterSetCast(p, pendingCast{def: 火球术}, []domain.EntityID{targetID})
					wantDamage = 0
					if upper {
						wantDamage = 208
					}
					if got := before - target.HP; got != wantDamage {
						t.Fatalf("generated firebolt=%d want%d", got, wantDamage)
					}
				case 4:
					s, p = meleeSetFixture(t, 4, pieces)
					s.classSetMeleeCastReleased(p, &domain.SkillDef{ID: classSetTaunt})
					wantStacks, wantIncoming := int32(0), int32(1000)
					if lower {
						wantStacks, wantIncoming = 1, 950
					}
					if got := s.classSetUnrulyStacks(p); got != wantStacks {
						t.Fatalf("unruly stacks=%d want%d", got, wantStacks)
					}
					if got := s.classSetMeleeIncomingDamage(p, 1000); got != wantIncoming {
						t.Fatalf("unruly incoming=%d want%d", got, wantIncoming)
					}
					wantScale := int32(10000)
					if upper {
						wantScale, wantStacks = 20000, 0
					}
					if got := s.classSetMeleeCastReleased(p, &domain.SkillDef{ID: classSetWhirlwind}); got != wantScale {
						t.Fatalf("whirlwind scale=%d want%d", got, wantScale)
					}
					if got := s.classSetUnrulyStacks(p); got != wantStacks {
						t.Fatalf("remaining stacks=%d want%d", got, wantStacks)
					}
				case 5:
					s, p = meleeSetFixture(t, 5, pieces)
					p.HP = p.MaxHP / 2
					wantDamage := int32(1000)
					if lower {
						wantDamage = 1500
					}
					if got := s.classSetMissingHealthDamage(p, 1000); got != wantDamage {
						t.Fatalf("missing-health damage=%d want%d", got, wantDamage)
					}
					if s.classSetDragonActive(p) != upper || (s.classSetAppearance(p, p.Player.Char.Appear).EquipView[0] == 1371) != upper {
						t.Fatal("dragon runtime/appearance threshold mismatch")
					}
					base := domain.Stats{MaxHP: 1000, MinAtk: 100, MaxAtk: 200, MoveSpeed: 100}
					wantStats := base
					if upper {
						wantStats = domain.Stats{MaxHP: 2000, MinAtk: 160, MaxAtk: 320, MoveSpeed: 120}
					}
					if got := s.classSetMeleeStats(p, s.functionalWorn(p), base); got != wantStats {
						t.Fatalf("dragon stat multipliers=%+v want%+v", got, wantStats)
					}
					if got := s.captureCombatSourceV5(p, false).Stats; got != p.Stats {
						t.Fatalf("combat source does not match panel: %+v != %+v", got, p.Stats)
					}
					preview := domain.SkillDef{MPCost: 200}
					s.applyResolvedSkillModifiers(p, &preview, p.Player.Char.Skills, p.Player.Char.Skills)
					wantPreview := int32(200)
					if upper {
						wantPreview = 0
					}
					if got := s.skillBehaviorPayload(p, preview).MPCost; got != wantPreview {
						t.Fatalf("skill MP preview=%d want%d", got, wantPreview)
					}
					p.HP, p.MP = 1000, 1000
					def := domain.SkillDef{MPCost: 200}
					s.classSetPaySkillCost(p, def)
					wantHP, wantMP := int32(1000), int32(800)
					if upper {
						wantHP, wantMP = 900, 1000
					}
					if p.HP != wantHP || p.MP != wantMP {
						t.Fatalf("skill cost HP/MP=%d/%d want%d/%d", p.HP, p.MP, wantHP, wantMP)
					}
				}
				if got := s.classSetPieces(p, tc.suit); got != int32(pieces) {
					t.Fatalf("functional count=%d want%d", got, pieces)
				}
				if got := len(s.classSetAuraSources(p)); (got == 1) != upper || got > 1 {
					t.Fatalf("aura sources=%d want enabled%v", got, upper)
				}
				p.Player.Party = 7
				s.stepEquipmentAuras()
				wantAura := int32(0)
				if upper {
					wantAura = tc.value
				}
				if got := s.classSetAuraValue(p, tc.modifier); got != wantAura {
					t.Fatalf("aura value=%d want%d", got, wantAura)
				}
				if upper {
					p.HP = 0
					if got := s.classSetAuraValue(p, tc.modifier); got != 0 {
						t.Fatalf("dead source retained aura=%d", got)
					}
					p.HP = 100
					p.Player.Worn = domain.NewEquipSet()
					if got := s.classSetAuraValue(p, tc.modifier); got != 0 {
						t.Fatalf("unequipped source retained aura=%d", got)
					}
				}
			})
		}
	}
}

func TestClassSetEveryAuraWithdrawsWhenFourthPieceBreaks(t *testing.T) {
	for suit := int32(1); suit <= 5; suit++ {
		t.Run(fmt.Sprint(suit), func(t *testing.T) {
			s := newCombatScene(t)
			p := partySetMember(t, s, 1, suit)
			s.stepEquipmentAuras()
			aura, _ := classSetAura(suit)
			if s.classSetPieces(p, suit) != 4 || s.classSetAuraValue(p, aura.Modifier) != aura.Value {
				t.Fatal("four-piece aura fixture is not active")
			}
			st := p.Player.Worn.At(domain.SlotGlove)
			st.Durability = 0
			p.Player.Worn.Set(domain.SlotGlove, st)
			if got := s.classSetPieces(p, suit); got != 3 {
				t.Fatalf("broken source count=%d want3", got)
			}
			if len(s.classSetAuraSources(p)) != 0 || s.classSetAuraValue(p, aura.Modifier) != 0 {
				t.Fatal("broken fourth piece retained aura before next scan")
			}
		})
	}
}
