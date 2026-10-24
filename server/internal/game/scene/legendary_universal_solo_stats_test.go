package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"math/rand"
	"testing"
)

func TestLegendaryUniversalSoloEquipmentStats(t *testing.T) {
	for _, id := range []string{"TY-D01", "TY-D02", "TY-D03", "TY-D04", "TY-U09", "TY-U20"} {
		t.Run(id, func(t *testing.T) {
			s, p, _ := universalSoloFixture(t, id, domain.Warrior, domain.Learned{})
			var slot domain.EquipSlot
			for _, x := range []domain.EquipSlot{domain.SlotWeapon, domain.SlotBody} {
				if p.Player.Worn.At(x).Item != 0 {
					slot = x
				}
			}
			st := p.Player.Worn.At(slot)
			it := s.items[st.Item]
			it.Equip.Bonus = domain.Stats{Def: 100, MDef: 100, MaxHP: 100, MaxMP: 100, MoveSpeed: 100}
			s.items[st.Item] = it
			p.Status.Apply(domain.StatusDef{ID: 190985, DurationSec: 60, Affixes: []domain.Affix{{Attr: domain.AttrDef, Value: 100}, {Attr: domain.AttrMDef, Value: 100}, {Attr: domain.AttrMaxHP, Value: 100}, {Attr: domain.AttrMaxMP, Value: 100}, {Attr: domain.AttrMoveSpeed, Value: 100}}}, s.tick, p.ID)
			s.refreshStats(p)
			enhanced := p.Stats
			speed := s.playerMoveSpeedPX(p)
			snap := s.attributeSnapshot(p)
			if snap.Current[4] != p.MaxHP || snap.Current[6] != p.MaxMP || snap.Current[9]*clientPixelsPerMoveSpeed != speed {
				t.Fatal("actual attribute projection inconsistent")
			}
			st.Legendary = domain.LegendaryInstance{}
			p.Player.Worn.Set(slot, st)
			s.refreshStats(p)
			base := p.Stats
			switch id {
			case "TY-D01":
				if enhanced.Def-base.Def != 40 {
					t.Fatal("equipment-only defense", enhanced.Def, base.Def)
				}
			case "TY-D02":
				if enhanced.MDef-base.MDef != 40 {
					t.Fatal("equipment-only magic defense", enhanced.MDef, base.MDef)
				}
			case "TY-D03":
				if enhanced.MaxHP-base.MaxHP != 30 || p.MaxHP != base.MaxHP {
					t.Fatal("maximum HP lifecycle")
				}
			case "TY-D04":
				if enhanced.MaxMP-base.MaxMP != 40 || p.MaxMP != base.MaxMP {
					t.Fatal("maximum MP lifecycle")
				}
			case "TY-U20":
				if speed-s.playerMoveSpeedPX(p) != 40*clientPixelsPerMoveSpeed {
					t.Fatal("equipment movement consumer", speed, s.playerMoveSpeedPX(p))
				}
			case "TY-U09":
				want := universalPct(characterInnateStats(p.Player.Char).MoveSpeed, 20)
				if enhanced.MoveSpeed-base.MoveSpeed != want {
					t.Fatal("base-only movement", enhanced.MoveSpeed, base.MoveSpeed, want)
				}
			}
			if universalShieldValue(p) != 0 {
				t.Fatal("stat bonus created a shield")
			}
		})
	}
}

func TestLegendaryUniversalSoloDebuffMagnitude(t *testing.T) {
	for _, id := range []string{"TY-A14", "TY-D14", "TY-D15", "TY-U08"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := universalSoloFixture(t, id, domain.Healer, domain.Learned{})
			s.refreshStats(p)
			dst, src := p, m
			if id == "TY-A14" {
				join(t, s, 2, 102, "status recipient", 130, 100)
				dst = s.players[2]
				src = p
			}
			before := dst.Stats
			d := domain.StatusDef{ID: 190986, SourceSkillID: 10310, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad, ChanceBP: 10000, ChanceExplicit: true}
			switch id {
			case "TY-A14", "TY-U08":
				d.Affixes = []domain.Affix{{Attr: domain.AttrMoveSpeed, Mode: domain.ModePercent, Value: -40}}
			case "TY-D14":
				d.Affixes = []domain.Affix{{Attr: domain.AttrDef, Value: -10}, {Attr: domain.AttrMDef, Value: -10}}
			case "TY-D15":
				d.Affixes = []domain.Affix{{Attr: domain.AttrAtkSpeed, Mode: domain.ModePercent, Value: 50}}
			}
			if !s.applyStatusDef(dst, d, src.ID) {
				t.Fatal("native debuff rejected")
			}
			a := universalSoloActive(t, dst, d.ID)
			if a.ExpireAt-s.tick != domain.Ticks(10000) {
				t.Fatal("magnitude changed duration")
			}
			switch id {
			case "TY-A14":
				if s.playerMoveSpeedPX(dst) != before.MoveSpeed*40/100*clientPixelsPerMoveSpeed {
					t.Fatal("source slow movement", s.playerMoveSpeedPX(dst), before.MoveSpeed)
				}
			case "TY-U08":
				if s.playerMoveSpeedPX(dst) != before.MoveSpeed*76/100*clientPixelsPerMoveSpeed {
					t.Fatal("receiver slow movement", s.playerMoveSpeedPX(dst), before.MoveSpeed)
				}
			case "TY-D14":
				if before.Def-dst.Stats.Def != 6 || before.MDef-dst.Stats.MDef != 6 {
					t.Fatal("defense reduction consumer", before, dst.Stats)
				}
			case "TY-D15":
				dst.DidBasicAttack(s.tick)
				ready := dst.CapturePlayerRuntime(s.tick).AttackReadyIn
				want := domain.Tick(1000 * 100 / int64(dst.Stats.PlayerAttackSpeedPercent()) / domain.TickMS)
				if ready != want || dst.Stats.AtkSpeedPct != -30 {
					t.Fatal("attack slowdown cadence", ready, want, dst.Stats.AtkSpeedPct)
				}
			}
			s.tick = a.ExpireAt
			s.stepStatus()
			if dst.Stats.MoveSpeed != before.MoveSpeed || dst.Stats.Def != before.Def || dst.Stats.MDef != before.MDef || dst.Stats.AtkSpeedPct != before.AtkSpeedPct {
				t.Fatal("expired debuff left modified stats")
			}
		})
	}
}

func TestLegendaryUniversalSoloPoisonCarrier(t *testing.T) {
	for _, tc := range []struct {
		name   string
		chance int32
		immune bool
		want   bool
	}{{"bonus", 20, false, true}, {"cap", 80, false, true}, {"none", 0, false, false}, {"immune", 80, true, false}} {
		t.Run("TY-A23/"+tc.name, func(t *testing.T) {
			s, p, m := universalSoloFixture(t, "TY-A23", domain.Assassin, domain.Learned{})
			carrier := domain.StatusDef{ID: 1018, SourceSkillID: 10211, DurationSec: 60, PoisonOnHitChance: tc.chance, PoisonOnHitLevel: 21, SourceEffectHitKnown: true}
			if !s.applyStatusDef(p, carrier, p.ID) {
				t.Fatal("poison carrier missing")
			}
			if tc.immune {
				m.Status.Apply(domain.StatusDef{ID: 190987, DurationSec: 60, BlockHarmful: true}, s.tick, m.ID)
			}
			// Source seed chosen between original20% and enhanced60% final contest.
			seed := int64(0)
			for ; seed < 1000; seed++ {
				r := rand.New(rand.NewSource(seed)).Float64()
				if r >= .2 && r < .6 {
					break
				}
			}
			s.rng = rand.New(rand.NewSource(seed))
			got := s.applyCombatTriggeredStatus(p, m, domain.StatusPoison)
			if got != tc.want || m.Status.Has(domain.StatusPoison) != tc.want {
				t.Fatal("triggered poison consumer", got, tc.want)
			}
			if tc.want {
				a := universalSoloActive(t, m, domain.StatusPoison)
				hp := m.HP
				s.tickStatus(m, a)
				if m.HP >= hp {
					t.Fatal("triggered poison never damaged")
				}
			}
		})
	}
}

func TestLegendaryUniversalSoloTrapRadius(t *testing.T) {
	for _, id := range []string{"TY-A04", "TY-A21", "TY-A25"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := universalSoloFixture(t, id, domain.Assassin, domain.Learned{10213: 1})
			at := domain.Pos{MapID: p.Pos.MapID, X: 150, Y: 100}
			r := float64(100)
			if id == "TY-A04" {
				r = 140
			}
			if id == "TY-A21" {
				r = 150
			}
			if id == "TY-A25" {
				st := p.Player.Worn.At(domain.SlotWeapon)
				it := s.items[st.Item]
				it.Equip.Bonus = domain.Stats{MinAtk: 100, MaxAtk: 100}
				s.items[st.Item] = it
			}
			s.aoi.Move(m, domain.Pos{MapID: m.Pos.MapID, X: at.X + r - 1, Y: 100})
			outside := putMonster(s, 901, 10000, domain.Stats{}, at.X+r+1, 100)
			s.monsters[outside.ID] = outside
			assassinRegressionCast(t, s, p, UseSkill{Skill: 10213, At: &at})
			if len(s.traps) != 1 {
				t.Fatal("trap count")
			}
			s.stepTraps()
			want := int32(200)
			if id == "TY-A25" {
				want = 300
			}
			universalSoloDamage(t, s, m.ID, 10213, want, 1)
			if len(s.traps) != 0 || outside.HP != 10000 {
				t.Fatal("trap radius/consumption")
			}
		})
	}
}

func TestLegendaryUniversalSoloConservation(t *testing.T) {
	s, p, tr := uniqueTestGear(t, "skill_economy")
	s.skills, s.statuses = nativeRepairTables(t)
	p.Player.Char.Race = domain.Warrior
	p.Player.Char.Employed = true
	p.Player.Char.EmploymentKnown = true
	p.Player.Char.SkillAllocationVersion = domain.SkillFormulaVersion
	p.Player.Char.Skills = domain.Learned{10002: 1}
	warriorEquip(t, s, p, "TY-A24", domain.SlotBody)
	p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
	p.MP = 1000
	p.MaxMP = 1000
	m := putMonster(s, 900, 10000, domain.Stats{}, 130, 100)
	before := p.MP
	assassinRegressionCast(t, s, p, UseSkill{Skill: 10002, Target: m.ID})
	want := (int32(8)*(10000-tr.PowerBP) + 9999) / 10000
	if before-p.MP != want {
		t.Fatal("ordinary equipment reduction actual cost", before-p.MP, want)
	}
	universalSoloDamage(t, s, m.ID, 10002, 130*(10000+2*tr.PowerBP)/10000, 1)
	s.tick += 100
	p.Look.AtkDist = 100
	s.outbox = nil
	universalSoloAttack(t, s, p, m, combat.Blow{GuaranteedHit: true})
	universalSoloDamage(t, s, m.ID, 0, 100, 1)
	if len(domain.ActiveLegendaryPowers(s.functionalWorn(p), s.itemDef, p.Player.Char)) != 1 {
		t.Fatal("not solo")
	}
}

// Output-shaped movement and defense bonuses are asserted at the authoritative
// entity and client snapshot consumers, not by calling their modifier handlers.
