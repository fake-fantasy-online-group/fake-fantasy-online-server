package scene

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
)

func incomingNativeCatalog(t *testing.T, s *Scene) {
	t.Helper()
	// Preserve the frozen original ranks, then apply the current authoritative
	// restoration delta, not the obsolete status anchors alone. Later skill
	// migrations 69-74 do not replace these defense/mirror/focus status values.
	raw, err := testfixture.ReadMigrationJSON("000000_init-62.sql", "$skill_config$")
	if err != nil {
		t.Fatal(err)
	}
	var seed struct {
		States       []domain.StatusDef
		Applications []struct {
			SkillID domain.SkillID
			Rank    int32
			App     domain.StatusApplication
		}
	}
	if err := json.Unmarshal(raw, &seed); err != nil {
		t.Fatal(err)
	}
	for _, d := range seed.States {
		s.statuses[domain.StatusKey{ID: d.ID, Level: d.Level}] = d
	}
	for _, a := range seed.Applications {
		key := domain.SkillKey{ID: a.SkillID, Level: a.Rank}
		if d, ok := s.skills[key]; ok {
			d.Statuses = []domain.StatusApplication{a.App}
			s.skills[key] = d
		}
	}
	for key, d := range s.skills {
		for i := range d.Statuses {
			d.Statuses[i].SourceSkill = d.ID
			d.Statuses[i].ChanceBP = domain.EffectiveStatusChanceBP(d.Statuses[i].ChanceBP, d.Statuses[i].ChanceExplicit)
			d.Statuses[i].ChanceExplicit = true
		}
		s.skills[key] = d
	}
}

func incomingEnablePower(s *Scene, p, m *entity.Entity, id string) {
	good := domain.StatusDef{ID: 190901, DurationSec: 30, TypeKnown: true, Type: domain.StatusGood}
	bad := domain.StatusDef{ID: 190902, DurationSec: 30, TypeKnown: true, Type: domain.StatusBad}
	switch id {
	case "SS-D01":
		good.SourceSkillID = 10425
	case "SS-D02":
		good.SourceSkillID = 10435
	case "SS-D03":
		good.SourceSkillID = 10417
	case "SS-D05":
		good.NextStatusDurationPct = 50
	case "SS-D09":
		bad.ID = 1036
		bad.SourceSkillID = 10402
	case "YS-D05":
		good.SourceSkillID = 10303
		s.casts = []pendingCast{{src: p.ID, def: domain.SkillDef{ID: 10301}, releaseAtWall: s.now().Add(time.Minute), windup: &castWindup{}}}
	case "YS-D06":
		s.casts = []pendingCast{{src: p.ID, def: domain.SkillDef{ID: 10304}, releaseAtWall: s.now().Add(time.Minute), windup: &castWindup{}}}
	case "YS-D08":
		s.casts = []pendingCast{{src: p.ID, def: domain.SkillDef{ID: 10305}, releaseAtWall: s.now().Add(time.Minute), windup: &castWindup{}}}
	case "YS-D10":
		bad.SourceSkillID = 10325
	}
	if good.SourceSkillID != 0 || good.NextStatusDurationPct > 0 {
		p.Status.Apply(good, s.tick, p.ID)
	}
	if bad.SourceSkillID != 0 {
		m.Status.Apply(bad, s.tick, p.ID)
	}
}

// The direct/proc/child rows exercise the production damage consumer, not the
// handler. DOT and reflection use their real separate production consumers.
// PvP is disabled, so proc/child-to-player rows are explicit strike-boundary
// tests, not claims that a player can currently attack another player.
func TestLegendaryIncomingAllDamageConsumerMatrix(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want int32
	}{
		{"SS-D01", 720}, {"SS-D02", 450}, {"SS-D03", 250}, {"SS-D05", 500}, {"SS-D06", 650}, {"SS-D09", 600}, {"SS-D10", 650},
		{"YS-D05", 750}, {"YS-D06", 600}, {"YS-D08", 400}, {"YS-D10", 600},
	} {
		for _, kind := range []string{"native", "proc", "child", "set_proc", "dot", "proc_dot", "reflection"} {
			t.Run(tc.id+"/"+kind, func(t *testing.T) {
				s, c := legendaryCasterFixture(t, tc.id)
				p, m := c.Owner, c.Target
				p.Stats = domain.Stats{}
				m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000}
				incomingEnablePower(s, p, m, tc.id)
				magic := tc.id == "SS-D03"
				blow := combat.Blow{SkillID: 10402, GuaranteedHit: true, Magic: magic}
				switch kind {
				case "proc", "set_proc":
					blow.Origin = domain.DamageProcOrigin
					blow.ProcDepth = 1
				case "child":
					blow.Origin = domain.DamageProcOrigin
					blow.ProcDepth = 1
					blow.LegendaryChild = true
				}
				if kind == "set_proc" {
					s.equipmentState().classSetTriggered = 1
				}
				before := p.HP
				switch kind {
				case "dot", "proc_dot":
					d := domain.StatusDef{ID: 199900, SourceSkillID: 10402, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -1000}}}
					if kind == "proc_dot" {
						d.LegendaryPowerID = "test-payload"
					}
					s.tickStatus(p, domain.Active{Source: m.ID, Def: d})
				case "reflection":
					s.emitReflectedDamage(m, p, 1000, magic)
				default:
					s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, blow)
				}
				if got := before - p.HP; got != tc.want {
					t.Fatalf("actual HP loss %d want %d", got, tc.want)
				}
				if s.legendaryState().depth != 0 {
					t.Fatal("recursion guard leaked")
				}
			})
		}
	}
}

func TestLegendaryIncomingCombinationShieldAndManaOnce(t *testing.T) {
	for _, kind := range []string{"native", "proc", "child", "dot", "reflection"} {
		t.Run(kind, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, "SS-D01")
			p, m := c.Owner, c.Target
			incomingEquipPower(t, s, p, "SS-D01", domain.SlotBody)
			incomingEquipPower(t, s, p, "SS-D09", domain.SlotHead)
			incomingEquipPower(t, s, p, "TY-D11", domain.SlotGlove)
			if n := len(domain.ActiveLegendaryPowers(s.functionalWorn(p), s.itemDef, p.Player.Char)); n != 3 {
				t.Fatalf("invalid build %d", n)
			}
			incomingEnablePower(s, p, m, "SS-D01")
			incomingEnablePower(s, p, m, "SS-D09")
			p.Stats = domain.Stats{}
			m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
			p.Status.Apply(domain.StatusDef{ID: 199901, DurationSec: 30, DamageShield: 100, PhysicalDamageReductionPct: 20, MagicDamageReductionPct: 20}, s.tick, p.ID)
			s.refreshStats(p)
			p.Stats = domain.Stats{}
			p.HP, p.MP = p.MaxHP, p.MaxMP
			before, mp := p.HP, p.MP
			b := combat.Blow{SkillID: 10002, GuaranteedHit: true}
			switch kind {
			case "proc":
				b.Origin = domain.DamageProcOrigin
				b.ProcDepth = 1
			case "child":
				b.Origin = domain.DamageProcOrigin
				b.ProcDepth = 1
				b.LegendaryChild = true
			}
			switch kind {
			case "dot":
				s.tickStatus(p, domain.Active{Source: m.ID, Def: domain.StatusDef{ID: 199902, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -1000}}}})
			case "reflection":
				s.emitReflectedDamage(m, p, 1000, false)
			default:
				s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, b)
			}
			// Native20%, 72%,60%, then mana30%, and finally shield once.
			// Reflection shares resource absorption, not direct-only reductions.
			want, spent := int32(142), int32(103)
			if before-p.HP != want || mp-p.MP != spent {
				t.Fatalf("HP=%d MP=%d want %d/%d", before-p.HP, mp-p.MP, want, spent)
			}
			if universalShieldValue(p) != 0 {
				t.Fatal("shield not settled exactly once")
			}
		})
	}
}

func TestLegendaryIncomingRiftHazardNotBasic(t *testing.T) {
	s, m, p, _ := riftAbilityFixture(t, riftEruption)
	p.Player.Char.Level = 100
	p.Player.Char.Employed = true
	p.Player.Char.EmploymentKnown = true
	h := &endgameRiftHazard{Source: m.ID, SourceLevel: m.Level, Attack: domain.Stats{MinAtk: 100, MaxAtk: 100}, Pos: p.Pos}
	p.HP = 10000
	s.hitRiftHazard(h, p, 100)
	baseline := 10000 - p.HP
	warriorEquip(t, s, p, "TY-D06", domain.SlotBody)
	p.HP = 10000
	s.hitRiftHazard(h, p, 100)
	equipped := 10000 - p.HP
	if baseline <= 0 || equipped != baseline {
		t.Fatalf("aura hazard is not basic: %d -> %d", baseline, equipped)
	}
	p.Stats = domain.Stats{}
	m.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100}
	ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
	if ev.Amount != 75 {
		t.Fatalf("native monster basic must still reduce: %d", ev.Amount)
	}
	ev, _, _ = s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{SkillID: 999001, GuaranteedHit: true})
	if ev.Amount != 100 {
		t.Fatalf("monster skill is not basic: %d", ev.Amount)
	}
}

func TestLegendaryIncomingActualPayloadAndChildReflection(t *testing.T) {
	for _, kind := range []string{"payload", "child"} {
		t.Run(kind, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, "SS-D10")
			p, m := c.Owner, c.Target
			p.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000}
			p.Player.Char.Skills = domain.Learned{}
			m.Status.Apply(domain.StatusDef{ID: 199903, DurationSec: 30, MagicFullReflectChancePct: 100}, s.tick, m.ID)
			// Both actors may reflect; the returned reflection must not reflect again.
			p.Status.Apply(domain.StatusDef{ID: 199904, DurationSec: 30, MagicFullReflectChancePct: 100}, s.tick, p.ID)
			before, targetBefore := p.HP, m.HP
			c.Skill = 10401
			if kind == "payload" {
				c.Damage(m, 1000, true, false)
			} else {
				d := domain.SkillDef{ID: 10401, Level: 1, Kind: domain.SkillSingle, TargetEnemy: true, HurtType: domain.HurtFire, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
				if !c.ChildSkill(m.ID, d, legendaryChildOptions{BaseDamage: 1000, TargetOnly: true, SuppressGeneration: true}) {
					t.Fatal("child rejected")
				}
			}
			if before-p.HP != 650 || m.HP != targetBefore {
				t.Fatalf("reflection damage %d, target %d", before-p.HP, targetBefore-m.HP)
			}
			if s.legendaryState().depth != 0 || s.legendaryState().child != nil {
				t.Fatal("nested state leaked")
			}
		})
	}
}

func TestLegendaryIncomingMirrorProcCapAndNoReflectionLoop(t *testing.T) {
	for _, kind := range []string{"native", "proc", "child", "set_proc"} {
		t.Run(kind, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, "SS-A26")
			legendaryCasterEquipBuild(t, s, c.Owner, "SS-A26", "SS-D03")
			p, m := c.Owner, c.Target
			p.Stats = domain.Stats{}
			m.Stats = domain.Stats{MAtk: 3000}
			p.Status.Apply(domain.StatusDef{ID: 1011, SourceSkillID: 10417, DurationSec: 30, MagicFullReflectChancePct: 100}, s.tick, p.ID)
			m.Status.Apply(domain.StatusDef{ID: 1011, DurationSec: 30, MagicFullReflectChancePct: 100}, s.tick, m.ID)
			b := combat.Blow{SkillID: 10401, Magic: true, HurtType: domain.HurtFire}
			if kind != "native" {
				b.Origin = domain.DamageProcOrigin
				b.ProcDepth = 1
			}
			if kind == "child" {
				b.LegendaryChild = true
			}
			if kind == "set_proc" {
				s.equipmentState().classSetTriggered = 1
			}
			before := m.HP
			ev, raw, reflected := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, b)
			s.afterStrikeStatuses(m, p, ev, raw, reflected)
			if ev.Amount != 0 || reflected != 1800 || before-m.HP != 1800 || p.HP != 10000 {
				t.Fatalf("mirror cap/loop ev=%+v reflected=%d loss=%d", ev, reflected, before-m.HP)
			}
		})
	}
}

func TestLegendaryIncomingNativeShieldMagnitudeConsumer(t *testing.T) {
	for _, skill := range []domain.SkillID{10425, 10435} {
		t.Run(fmt.Sprint(skill), func(t *testing.T) {
			s, c := legendaryCasterFixture(t, "SS-D03")
			incomingNativeCatalog(t, s)
			p, m := c.Owner, c.Target
			legendaryCasterCastNow(t, s, p, skill, p.ID)
			var applied domain.StatusDef
			p.Status.EachEffective(func(a domain.Active) {
				if a.Def.SkillSnapshotID == skill {
					applied = a.Def
				}
			})
			if applied.PhysicalDamageReductionPct != 45 {
				t.Fatalf("native shield not strengthened: %+v", applied)
			}
			if skill == 10425 && applied.PhysicalReflectPct != 7 {
				t.Fatal("native fire reflection not strengthened", applied.PhysicalReflectPct)
			}
			if skill == 10435 && applied.FreezeAttackerChance != 60 {
				t.Fatal("native ice chance not strengthened", applied.FreezeAttackerChance)
			}
			p.Stats = domain.Stats{}
			m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
			p.HP = p.MaxHP
			ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
			if ev.Amount != 550 {
				t.Fatalf("shield field never reached real physical consumer: %d", ev.Amount)
			}
		})
	}
}

func TestLegendaryIncomingNativeBuffMagnitudeCombinations(t *testing.T) {
	t.Run("defense_focus_and_flat_bonus", func(t *testing.T) {
		s, c := legendaryCasterFixture(t, "YS-U02")
		incomingNativeCatalog(t, s)
		legendaryCasterEquipBuild(t, s, c.Owner, "YS-U02", "YS-A26", "YS-D04")
		legendaryCasterCastNow(t, s, c.Owner, 10322, c.Owner.ID)
		legendaryCasterCastNow(t, s, c.Owner, 10321, c.Owner.ID)
		var d domain.StatusDef
		c.Owner.Status.EachEffective(func(a domain.Active) {
			if a.Def.SkillSnapshotID == 10321 {
				d = a.Def
			}
		})
		// Native 10% * (1+70%+50%) + the separate35-point D04 grant.
		if d.PhysicalDamageReductionPct != 57 || d.MagicDamageReductionPct != 57 {
			t.Fatalf("wrong shared/flat buckets: %+v", d)
		}
		s.legendaryFinalizeStatus(&d)
		if d.PhysicalDamageReductionPct != 57 {
			t.Fatal("finalization compounded defense")
		}
		c.Owner.Stats = domain.Stats{}
		c.Target.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000}
		for _, magic := range []bool{false, true} {
			c.Owner.HP = c.Owner.MaxHP
			ev, _, _ := s.resolveStrikeStatuses(c.Target, c.Owner, &combat.HitTracker{}, combat.Blow{SkillID: 999001, Magic: magic, GuaranteedHit: true})
			if ev.Amount != 430 {
				t.Fatalf("defense consumer magic=%v got=%d", magic, ev.Amount)
			}
		}
	})
	for _, tc := range []struct {
		skill domain.SkillID
		field string
		want  int32
	}{{10330, "reflect", 15}, {10322, "duration", 75}, {10315, "haste", -37}} {
		t.Run(tc.field, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, "YS-U02")
			incomingNativeCatalog(t, s)
			legendaryCasterCastNow(t, s, c.Owner, 10322, c.Owner.ID)
			legendaryCasterCastNow(t, s, c.Owner, tc.skill, c.Owner.ID)
			var d domain.StatusDef
			c.Owner.Status.EachEffective(func(a domain.Active) {
				if a.Def.SkillSnapshotID == tc.skill {
					d = a.Def
				}
			})
			got := d.PhysicalReflectPct
			if tc.field == "duration" {
				got = d.NextStatusDurationPct
			}
			if tc.field == "haste" {
				for _, a := range d.Affixes {
					if a.Attr == domain.AttrAtkSpeed {
						got = a.Value
					}
				}
			}
			if got != tc.want {
				t.Fatalf("native %s magnitude=%d want%d", tc.field, got, tc.want)
			}
			if tc.field == "reflect" {
				c.Owner.Stats = domain.Stats{}
				c.Owner.HP = c.Owner.MaxHP
				c.Target.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
				before := c.Target.HP
				ev, raw, reflected := s.resolveStrikeStatuses(c.Target, c.Owner, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
				s.afterStrikeStatuses(c.Target, c.Owner, ev, raw, reflected)
				if before-c.Target.HP != 150 {
					t.Fatalf("native reflect consumer=%d", before-c.Target.HP)
				}
			}
		})
	}
}

func TestLegendaryIncomingSetAuraAndShieldCombination(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-D10")
	incomingEquipPower(t, s, c.Owner, "SS-D10", domain.SlotBody)
	incomingEquipPower(t, s, c.Owner, "SS-D06", domain.SlotHead)
	c.Owner.Player.Party = 1
	partySetMember(t, s, 2, 4)
	s.stepEquipmentAuras()
	p, m := c.Owner, c.Target
	p.Status.Apply(domain.StatusDef{ID: 199906, DurationSec: 30, PhysicalDamageReductionPct: 20, DamageShield: 100}, s.tick, p.ID)
	s.refreshStats(p)
	p.Stats = domain.Stats{}
	p.HP = p.MaxHP
	m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
	if s.classSetAuraValue(p, "class_set_physical_mitigation_bp") != 800 {
		t.Fatal("actual party set aura missing")
	}
	before := p.HP
	ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
	// 1000 * native80% * actual set92% * power65% * power65% - shield100.
	if ev.Amount != 210 || before-p.HP != 210 {
		t.Fatalf("set/power/shield double settlement: ev%d HP%d", ev.Amount, before-p.HP)
	}
}

func TestLegendaryIncomingWarriorBucketNotReapplied(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-D05")
	warriorEquip(t, s, p, "ZS-D07", domain.SlotHead)
	p.Stats = domain.Stats{}
	m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
	for _, b := range []combat.Blow{{GuaranteedHit: true}, {Origin: domain.DamageProcOrigin, ProcDepth: 1, LegendaryChild: true, GuaranteedHit: true}} {
		p.HP = p.MaxHP
		ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, b)
		if ev.Amount != 150 {
			t.Fatalf("warrior40+45 additive bucket must run once: %d", ev.Amount)
		}
	}
}

func incomingEquipPower(t *testing.T, s *Scene, p *entity.Entity, id string, slot domain.EquipSlot) {
	t.Helper()
	item := domain.ItemID(992000 + slot)
	s.items[item] = domain.ItemDef{ID: item, Equip: &domain.EquipDef{Slot: int32(slot), LevelReq: 100, Durable: 100, Need: domain.Requirement{Professions: 1 << uint8(p.Player.Char.Race)}}}
	p.Player.Worn.Set(slot, domain.Stack{UID: int64(item), Item: item, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: id}})
	power, _ := domain.LegendaryPower(id)
	if !domain.LegendaryPowerEligible(power, s.items[item]) {
		t.Fatalf("invalid test slot %s/%v", id, slot)
	}
}

func TestLegendaryIncomingMirrorAmplifierInsideGenerationGuard(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-A26")
	incomingEquipPower(t, s, c.Owner, "SS-D03", domain.SlotBody)
	c.Owner.Stats = domain.Stats{}
	c.Target.Stats = domain.Stats{MAtk: 500}
	c.Owner.Status.Apply(domain.StatusDef{ID: 1011, SourceSkillID: 10417, DurationSec: 30, MagicFullReflectChancePct: 100}, s.tick, c.Owner.ID)
	s.legendaryState().depth = 1
	s.equipmentState().classSetTriggered = 1
	_, _, reflected := s.resolveStrikeStatuses(c.Target, c.Owner, &combat.HitTracker{}, combat.Blow{Origin: domain.DamageProcOrigin, ProcDepth: 1, Magic: true, HurtType: domain.HurtFire})
	if reflected != 900 {
		t.Fatalf("120%% * D03 150%% multiplier lost inside guard: %d", reflected)
	}
}

func TestLegendaryIncomingPoisonUsesMirrorMagicReduction(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-D03")
	incomingEnablePower(s, c.Owner, c.Target, "SS-D03")
	c.Owner.Stats.MDef, c.Owner.Stats.MagicResist = 0, 0
	before := c.Owner.HP
	s.tickStatus(c.Owner, domain.Active{Source: c.Target.ID, Def: domain.StatusDef{ID: domain.StatusPoison, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -1000}}}})
	if before-c.Owner.HP != 250 {
		t.Fatal("periodic magic bypassed mirror reduction")
	}
}

func TestLegendaryIncomingActualNativeFireShieldCombination(t *testing.T) {
	s, c := legendaryCasterFixture(t, "SS-D03")
	incomingNativeCatalog(t, s)
	incomingEquipPower(t, s, c.Owner, "SS-D01", domain.SlotHead)
	legendaryCasterCastNow(t, s, c.Owner, 10425, c.Owner.ID)
	p, m := c.Owner, c.Target
	p.Stats = domain.Stats{}
	p.HP = p.MaxHP
	m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
	ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
	if ev.Amount != 396 {
		t.Fatalf("native45%% shield + nearby28%% power got%d want396", ev.Amount)
	}
	before := p.HP
	s.emitReflectedDamage(m, p, 1000, false)
	if before-p.HP != 396 {
		t.Fatal("same shield/power pair not respected by reflection")
	}
	p.Player.Worn.Set(domain.SlotHead, domain.Stack{})
	p.HP = p.MaxHP
	ev, _, _ = s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
	if ev.Amount != 550 {
		t.Fatal("removed incoming modifier still active", ev.Amount)
	}
}

func TestLegendaryIncomingActualHealerWindupCombination(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-D05")
	incomingNativeCatalog(t, s)
	incomingEquipPower(t, s, c.Owner, "YS-D06", domain.SlotHead)
	legendaryCasterCastNow(t, s, c.Owner, 10303, c.Owner.ID)
	s.onUseSkill(UseSkill{ID: c.Owner.ID, Skill: 10304, Target: c.Owner.ID})
	if !s.legendaryCasterWinding(c.Owner, 10304) {
		t.Fatal("native group heal failed to enter real windup")
	}
	p, m := c.Owner, c.Target
	p.Stats = domain.Stats{}
	p.HP = p.MaxHP
	m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
	ev, raw, reflected := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
	s.afterStrikeStatuses(m, p, ev, raw, reflected)
	if ev.Amount != 450 || !s.legendaryCasterWinding(p, 10304) {
		t.Fatalf("native winding protection amount%d", ev.Amount)
	}
	before := p.HP
	s.tickStatus(p, domain.Active{Source: m.ID, Def: domain.StatusDef{ID: 199907, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -1000}}}})
	if before-p.HP != 450 {
		t.Fatal("real healing windup did not protect DOT", before-p.HP)
	}
	before = p.HP
	s.emitReflectedDamage(m, p, 1000, false)
	if before-p.HP != 450 {
		t.Fatal("real healing windup did not protect reflection", before-p.HP)
	}
}
