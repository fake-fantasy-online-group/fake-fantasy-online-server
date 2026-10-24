package scene

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Bounded migration-derived consumer fixture, not a PostgreSQL loader claim:
// init-32 assigns monster 1099 the level-one 12007 skill and explicitly maps its
// only effect to the same-level native 1009 slow. The native skill has no windup
// or authored distance, so the loader uses the monster's basic-attack reach.
func TestLegendarySwordsmanMovementImmunityMonsterSlowCast(t *testing.T) {
	sql, err := os.ReadFile("../../../migrations/000000_init-32.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []string{"(1099,0,'enemy',12007,1,'脚部狙击')", "(12007,0,0,'status',1009,'same',0,0)"} {
		if !strings.Contains(string(sql), row) {
			t.Fatal("native source mapping drift", row)
		}
	}
	for _, equipped := range []bool{false, true} {
		t.Run(fmt.Sprint(equipped), func(t *testing.T) {
			powers := []string{}
			if equipped {
				powers = append(powers, "JK-U08", "TY-U08", "TY-D09")
			}
			s, p, m := nativeRepairFixture(t, domain.Swordsman, domain.Learned{}, powers...)
			nativeRepairSet(t, s, p, 4, 4)
			s.refreshStats(p)
			baseSpeed := p.Stats.MoveSpeed
			p.Stats.StatusResist = 0
			m.Monster.TypeID, m.Monster.Target = 1099, p.ID
			m.Monster.SkillReady = map[domain.SkillID]domain.Tick{}
			m.Monster.SkillCasts = map[domain.SkillID]int32{}
			rule := domain.MonsterSkillRule{Enabled: true, Skill: domain.SkillDef{ID: 12007, Name: "脚部狙击", Level: 1, Kind: domain.SkillSingle, TargetEnemy: true,
				Statuses: []domain.StatusApplication{{ID: domain.StatusSlow, Level: 1, SourceSkill: 12007, ChanceBP: 10000, ChanceExplicit: true}}}}
			rule.Skill.Dist = m.Monster.AI.BasicAttackDist
			s.startMonsterSkill(m, rule, []domain.EntityID{p.ID})
			if len(s.casts) != 1 {
				t.Fatal("native slow cast not queued")
			}
			s.stepCasts()
			if len(s.casts) != 0 || m.Monster.SkillCasts[12007] != 1 {
				t.Fatal("monster cast did not settle")
			}
			if p.Status.Has(domain.StatusSlow) == equipped {
				t.Fatal("monster slow immunity not functional", equipped)
			}
			if equipped && p.Stats.MoveSpeed != baseSpeed || !equipped && p.Stats.MoveSpeed >= baseSpeed {
				t.Fatal("actual movement stat mismatch", equipped, baseSpeed, p.Stats.MoveSpeed)
			}
		})
	}
}

func TestLegendarySwordsmanMovementImmunityEquipmentLifecycle(t *testing.T) {
	for _, powers := range [][]string{{"JK-U08"}, {"JK-U08", "TY-U08"}, {"JK-U08", "TY-U08", "TY-D09"}} {
		for _, setPieces := range []int{0, 2, 4} {
			for _, key := range []domain.StatusKey{{ID: 1009, Level: 1}, {ID: 10111, Level: 1}, {ID: 1034, Level: 41}} {
				t.Run(fmt.Sprintf("%v/set%d/status%v", powers, setPieces, key), func(t *testing.T) {
					s, p, m := nativeRepairFixture(t, domain.Swordsman, domain.Learned{}, powers...)
					if setPieces > 0 {
						nativeRepairSet(t, s, p, 4, setPieces)
					}
					weapon := p.Player.Worn.At(domain.SlotWeapon)
					if weapon.Legendary.PowerID != "JK-U08" {
						t.Fatal("immunity not in its authorized weapon slot")
					}
					p.Player.Worn.Set(domain.SlotWeapon, domain.Stack{})
					s.refreshStats(p)
					d, ok := s.statuses.Get(key.ID, key.Level)
					if !ok {
						t.Fatal("native movement state absent", key)
					}
					d.EffectHitBP = 100000000 // Isolate receiving semantics, not random resistance.
					if !s.applyStatusDef(p, d, m.ID) || !p.Status.Has(key.ID) {
						t.Fatal("unequipped native baseline failed")
					}
					p.Player.Worn.Set(domain.SlotWeapon, weapon)
					s.refreshStats(p) // Same equipment-change path used by equip/unequip.
					if p.Status.Has(key.ID) || p.Status.BlocksMovement() || !canMove(p) {
						t.Fatal("existing movement penalty survived equip")
					}
					if s.applyStatusDef(p, d, m.ID) || p.Status.Has(key.ID) {
						t.Fatal("new movement penalty bypassed immunity")
					}
					p.Player.Worn.Set(domain.SlotWeapon, domain.Stack{})
					s.refreshStats(p)
					if p.Status.Has(key.ID) {
						t.Fatal("unequip resurrected a cleansed state")
					}
					if !s.applyStatusDef(p, d, m.ID) || !p.Status.Has(key.ID) {
						t.Fatal("immunity survived unequip")
					}
				})
			}
		}
	}
}

func TestLegendarySwordsmanMovementImmunityMixedStatusPreservation(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, powerCount := range []int{1, 3} {
			for _, id := range []domain.StatusID{domain.StatusStun, domain.StatusFreeze, domain.StatusPetrify, 81020} {
				t.Run(fmt.Sprintf("existing%v/powers%d/id%d", existing, powerCount, id), func(t *testing.T) {
					powers := []string{"JK-U08", "TY-U08", "TY-D09"}
					s, p, m := nativeRepairFixture(t, domain.Swordsman, domain.Learned{}, powers[:powerCount]...)
					nativeRepairSet(t, s, p, 4, 4)
					weapon := p.Player.Worn.At(domain.SlotWeapon)
					if existing {
						p.Player.Worn.Set(domain.SlotWeapon, domain.Stack{})
					}
					s.refreshStats(p)
					d := domain.StatusDef{ID: id, Level: 1, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad, EffectHitBP: 100000000, Control: domain.ControlStun, Immobile: 1,
						Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -50, Mode: domain.ModePercent}, {Attr: domain.AttrDef, Value: -20, Mode: domain.ModePercent}}, PhysicalDamageTakenPct: 30, IntervalSec: 2, HPChanges: []domain.ResourceChange{{Value: -7}}, DamageShield: 200}
					if id == 81020 {
						d.Control, d.BlockMove = domain.ControlNone, true
					}
					if !s.applyStatusDef(p, d, m.ID) {
						t.Fatal("mixed payload was discarded")
					}
					before := verifiedActive(t, p, id)
					if existing {
						s.tick += domain.Ticks(1000)
						p.Player.Worn.Set(domain.SlotWeapon, weapon)
						s.refreshStats(p)
					}
					got := verifiedActive(t, p, id)
					if got.Source != m.ID || got.ExpireAt != before.ExpireAt || got.NextTickAt != before.NextTickAt || got.ShieldLeft != before.ShieldLeft {
						t.Fatal("cleanse restarted or refilled mixed state", before, got)
					}
					if got.Def.Control != d.Control || got.Def.PhysicalDamageTakenPct != 30 || !reflect.DeepEqual(got.Def.HPChanges, d.HPChanges) || len(got.Def.Affixes) != 1 || got.Def.Affixes[0].Attr != domain.AttrDef {
						t.Fatal("unrelated mixed payload changed", got.Def)
					}
					if id == 81020 {
						if got.Def.BlockMove || got.Def.Immobile != 0 || !canMove(p) {
							t.Fatal("mixed root not removed")
						}
					} else if canMove(p) || got.Def.Immobile != 1 || p.Status.Controlled() != domain.ControlStun {
						t.Fatal("hard control incorrectly immunized")
					}
					if powerCount == 3 && (existing || id != 81020) && got.Def.LegendaryDurationMS != 7000 {
						t.Fatal("other control-duration power lost", got.Def.LegendaryDurationMS)
					}
					if d.Affixes[0].Value != -50 || len(d.Affixes) != 2 {
						t.Fatal("incoming source definition mutated")
					}
				})
			}
		}
	}
}

func TestLegendarySwordsmanMovementImmunityPreservesNativeAttackSlow(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Swordsman, domain.Learned{}, "JK-U08")
	for _, rank := range []int32{11, 15} {
		d, ok := s.statuses.Get(domain.StatusSlow, rank)
		if !ok || !d.AttackSpeedIsFrequency || len(d.Affixes) != 1 || d.Affixes[0].Attr != domain.AttrAtkSpeed {
			t.Fatal("native shackle fixture drift", d)
		}
		d.EffectHitBP = 100000000
		if !s.applyStatusDef(p, d, m.ID) {
			t.Fatal("attack slow mistaken for movement slow")
		}
		got := verifiedActive(t, p, d.ID)
		if !reflect.DeepEqual(got.Def.Affixes, d.Affixes) || !got.Def.AttackSpeedIsFrequency {
			t.Fatal("attack slow lost", got.Def)
		}
		p.Status.Remove(d.ID)
	}
}
