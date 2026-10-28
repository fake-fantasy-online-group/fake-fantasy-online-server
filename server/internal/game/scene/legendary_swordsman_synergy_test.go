package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func swordEquip(t *testing.T, s *Scene, p *entity.Entity, id string, slot domain.EquipSlot) {
	t.Helper()
	item := domain.ItemID(910000 + int(slot))
	s.items[item] = domain.ItemDef{ID: item, Equip: &domain.EquipDef{Slot: int32(slot), LevelReq: 100, Durable: 100, Need: domain.Requirement{Professions: 2}}}
	p.Player.Worn.Set(slot, domain.Stack{UID: int64(item), Item: item, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: id}})
}
func swordCast(t *testing.T, s *Scene, p, target *entity.Entity, def domain.SkillDef) []domain.EntityID {
	t.Helper()
	def.TargetEnemy = true
	finish := s.beginEquipmentRoot(p.ID, target.ID, domain.EquipmentCombatStats{})
	root := s.equipmentState().root
	root.def = def
	root.center = p.Pos
	ids := s.legendarySelectTargets(p, def, target.ID, []domain.EntityID{target.ID})
	ids = s.legendaryImpactTargets(p, def, target.ID, ids)
	root.targets = ids
	for _, id := range ids {
		s.applySkill(p, s.entities[id], s.legendaryTargetDefinition(p, def, s.entities[id], target.ID, ids), 100, false)
	}
	s.flushLegendarySwordRestore()
	finish()
	return ids
}
func swordDualDef(id domain.SkillID) domain.SkillDef {
	restore := domain.SkillEffect{Kind: domain.EffectRestoreHP, Target: domain.EffectTargetCaster, SourceAttackPct: 20, RequiresPreviousLanded: true}
	if id == 10119 {
		restore.Kind = domain.EffectRestoreMP
	}
	return domain.SkillDef{ID: id, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 150, GuaranteedHit: true}, restore}}
}
func swordShield(p *entity.Entity) (amount int32, permanent bool) {
	if p.Status != nil {
		p.Status.EachEffective(func(a domain.Active) {
			if a.Def.LegendaryPowerID == "JK-D03" {
				amount = a.ShieldLeft
				permanent = a.ExpireAt == 0
			}
		})
	}
	return
}
func TestLegendarySwordsmanBloodSoulStandaloneAndCombinations(t *testing.T) {
	for _, skill := range []domain.SkillID{10104, 10119} {
		for _, pieces := range []int{1, 2, 3} {
			t.Run(string(rune(skill))+string(rune('0'+pieces)), func(t *testing.T) {
				s, p, m := legendaryMeleeFixture(t, "JK-A09")
				p.HP = 9990
				p.MaxMP = 1000
				p.MP = 990
				if pieces >= 2 {
					swordEquip(t, s, p, "JK-A11", domain.SlotBody)
				}
				if pieces == 3 {
					swordEquip(t, s, p, "JK-D03", domain.SlotHead)
				}
				for i := 0; i < 5; i++ {
					putMonster(s, domain.EntityID(901+i), 10000, domain.Stats{}, 140+float64(i), 100)
				}
				ids := swordCast(t, s, p, m, swordDualDef(skill))
				if len(ids) != 6 {
					t.Fatalf("targets=%v", ids)
				}
				wantRestore := int32(20)
				if pieces >= 2 {
					wantRestore = 60
				}
				pool := p.HP
				if skill == 10119 {
					pool = p.MP
				}
				maximum := p.MaxHP
				if skill == 10119 {
					maximum = p.MaxMP
				}
				if pool != maximum {
					t.Fatal("missing native restore", pool)
				}
				shield, permanent := swordShield(p)
				if pieces == 3 {
					if shield != wantRestore-10 || !permanent {
						t.Fatalf("shield=%d permanent=%v", shield, permanent)
					}
				} else if shield != 0 {
					t.Fatal("unearned shield")
				}
				if m.HP != 100000-172 {
					t.Fatal("native damage115%", m.HP)
				}
				if len(s.legendaryState().states) != 0 {
					t.Fatal("dual swords introduced temporal state", s.legendaryState().states)
				}
			})
		}
	}
	for _, id := range []string{"JK-A11", "JK-D03"} {
		t.Run(id+"-single", func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, id)
			p.HP = p.MaxHP
			swordCast(t, s, p, m, swordDualDef(10104))
			shield, permanent := swordShield(p)
			if id == "JK-D03" && (shield != 20 || !permanent) {
				t.Fatalf("standalone shield=%d", shield)
			}
		})
	}
}
func TestLegendarySwordsmanRestoreActualHitsAndNonRecursive(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-A11")
	p.HP = 9000
	other := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
	finish := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
	r := s.equipmentState().root
	r.def = swordDualDef(10104)
	// Duplicate hits of one enemy count once; an admitted child hitting another enemy contributes independently.
	r.hits = []equipmentHit{{src: p.ID, dst: m.ID, blow: combat.Blow{SkillID: 10104}}, {src: p.ID, dst: m.ID, blow: combat.Blow{SkillID: 10104}}, {src: p.ID, dst: other.ID, blow: combat.Blow{SkillID: 10104, ProcDepth: 1}}}
	if !s.queueLegendarySwordRestore(p, p, r.def, 20, false) {
		t.Fatal("not queued")
	}
	s.queueLegendarySwordRestore(p, p, r.def, 999, false)
	s.flushLegendarySwordRestore()
	s.flushLegendarySwordRestore()
	if p.HP != 9040 {
		t.Fatal("distinct target restoration or duplicate flush incorrect", p.HP)
	}
	finish()
	s.legendaryState().depth = 1
	if s.queueLegendarySwordRestore(p, p, swordDualDef(10104), 20, false) {
		t.Fatal("recursive restore queued")
	}
	s.legendaryState().depth = 0
}
func TestLegendarySwordsmanBoundStrikeAndSourceDefense(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-A05")
	swordEquip(t, s, p, "JK-A24", domain.SlotBody)
	swordEquip(t, s, p, "JK-D06", domain.SlotHead)
	rootDef := domain.StatusDef{ID: 800111, Level: 1, DurationSec: 6, BlockMove: true, TypeKnown: true, Type: domain.StatusBad, EffectHitBP: 100000000}
	stunDef := domain.StatusDef{ID: domain.StatusStun, Level: 1, DurationSec: 6, Control: domain.ControlStun, TypeKnown: true, Type: domain.StatusBad, EffectHitBP: 100000000}
	s.statuses = statusTable(rootDef, stunDef)
	q := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
	d := domain.SkillDef{ID: 10111, CooldownMS: 120000, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}, Statuses: []domain.StatusApplication{{ID: rootDef.ID, Level: 1}}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.CooldownMS != 24000 {
		t.Fatal(d.CooldownMS)
	}
	ids := swordCast(t, s, p, m, d)
	if len(ids) != 2 || !swordRooted(q, p.ID) {
		t.Fatal("group native root absent", ids)
	}
	d.ID = 10126
	d.CooldownMS = 120000
	d.Statuses = []domain.StatusApplication{{ID: domain.StatusStun, Level: 1}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	if d.CooldownMS != 18000 {
		t.Fatal(d.CooldownMS)
	}
	before := q.HP
	ids = swordCast(t, s, p, m, d)
	if len(ids) != 2 || q.HP != before-180 || q.Status.Controlled() == domain.ControlStun {
		t.Fatal("root expansion copied stun or damage failed", ids, q.HP)
	}
	s.legendaryCastReleased(p, domain.SkillDef{ID: 10111}, q.ID, nil)
	var c legendaryContext
	for _, ctx := range s.legendaryContexts(p, q, combat.Blow{}, event.DamageDealt{}) {
		if ctx.Power.ID == "JK-D06" {
			c = ctx
		}
	}
	if n := legendaryHandlers["JK-D06"].BeforeIncoming(&c, 100); n != 65 {
		t.Fatal("missing bind window defense", n)
	}
	q.Status = domain.NewStatusSet()
	q.Status.Apply(rootDef, s.tick, 999)
	if n := legendaryHandlers["JK-D06"].BeforeIncoming(&c, 100); n != 65 {
		t.Fatal("window incorrectly depends on target control", n)
	}
	c.NowMS += 8000
	if n := legendaryHandlers["JK-D06"].BeforeIncoming(&c, 100); n != 100 {
		t.Fatal("expired window remained", n)
	}
}
func TestLegendarySwordsmanGatherConcentrateProximity(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-A16")
	swordEquip(t, s, p, "JK-A15", domain.SlotBody)
	swordEquip(t, s, p, "JK-D05", domain.SlotHead)
	m.Pos.X = 220
	s.aoi.Move(m, m.Pos)
	q := putMonster(s, 901, 10000, domain.Stats{}, 230, 100)
	d := domain.SkillDef{ID: 10121, HurtType: domain.HurtPhysical, Radius: 100, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}}
	ids := swordCast(t, s, p, m, d)
	if len(ids) != 2 || sqDist(p.Pos, q.Pos) > 50*50 {
		t.Fatalf("gather targets=%v positions %v", ids, q.Pos)
	}
	// 100*1.3*1.6=208 each; sum416 =>145 concentrated on highest current HP.
	if q.HP != 10000-208 || m.HP != 100000-208-145 {
		t.Fatal("concentrated native actual sum", m.HP, q.HP)
	}
	c := legendaryContext{Scene: s, Owner: p, Target: q}
	if n := legendaryHandlers["JK-D05"].BeforeIncoming(&c, 100); n != 60 {
		t.Fatal(n)
	}
	q.Pos.X = p.Pos.X + 76
	if n := legendaryHandlers["JK-D05"].BeforeIncoming(&c, 100); n != 100 {
		t.Fatal(n)
	}
	if len(s.legendaryState().states) != 0 {
		t.Fatal("whirlwind retained old ring/timer")
	}
}
func TestLegendarySwordsmanKeepHealSuppressionAndShieldBreak(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-A10")
	m.HP = 90000
	legendaryMeleeStrike(s, p, m, 10104)
	hp, maxHP, mp := m.HP, m.MaxHP, m.MP
	ev := combat.Restore(p, m, 100, 10104)
	if ev.Amount != 35 || m.HP != hp+35 || m.MaxHP != maxHP || m.MP != mp {
		t.Fatal("antiheal changed wrong resource", ev, m)
	}
	// The same central boundary is used by life steal and periodic heals.
	ev = combat.Restore(m, m, 100, 0)
	if ev.Amount != 35 {
		t.Fatal("lifesteal bypassed suppression", ev.Amount)
	}
	m.Status.Expire(s.tick + domain.Ticks(8000))
	if ev = combat.Restore(p, m, 100, 10104); ev.Amount != 100 {
		t.Fatal("antiheal outlived eight seconds", ev.Amount)
	}
	s, p, m = legendaryMeleeFixture(t, "JK-A14")
	m.Status = domain.NewStatusSet()
	m.Status.Apply(domain.StatusDef{ID: 900001, Level: 1, DurationSec: 10, DamageShield: 1000}, s.tick, m.ID)
	hp = m.HP
	legendaryMeleeStrike(s, p, m, 10113)
	left := int32(0)
	m.Status.EachEffective(func(a domain.Active) { left += a.ShieldLeft })
	if left != 780 || m.HP != hp {
		t.Fatal("shield break spilled to life or coefficient wrong", left, m.HP)
	}
	m.Status = domain.NewStatusSet()
	evDamage := legendaryMeleeStrike(s, p, m, 10113)
	if evDamage.Amount != 150 {
		t.Fatal("unshielded damage", evDamage.Amount)
	}
}
func TestLegendarySwordsmanConcentrateExcludesOverkill(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-A15")
	q := putMonster(s, 901, 10, domain.Stats{}, 140, 100)
	finish := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
	s.equipmentState().root.def = domain.SkillDef{ID: 10121}
	for _, dst := range []*entity.Entity{m, q} {
		s.resolveStrikeStatuses(p, dst, &combat.HitTracker{}, combat.Blow{SkillID: 10121, SkillPct: 100, GuaranteedHit: true})
	}
	finish()
	// Real HP loss is160+10, not160+160;35% is59, not112.
	if m.HP != 100000-160-59 {
		t.Fatal("overkill inflated concentrated slash", m.HP)
	}
}
func TestLegendarySwordsmanGatherImmunityAndRadius(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-A16")
	m.Pos.X = 220
	s.aoi.Move(m, m.Pos)
	m.Status = domain.NewStatusSet()
	m.Status.Apply(domain.StatusDef{ID: 900001, Level: 1, DurationSec: 10, SuppressHarmful: true}, s.tick, m.ID)
	q := putMonster(s, 901, 10000, domain.Stats{}, 170, 100)
	q.Status = domain.NewStatusSet()
	q.Status.Apply(domain.StatusDef{ID: 900001, Level: 1, DurationSec: 10, SuppressHarmful: true}, s.tick, q.ID)
	d := domain.SkillDef{ID: 10121, HurtType: domain.HurtPhysical, Radius: 100, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}}
	ids := swordCast(t, s, p, m, d)
	if len(ids) != 1 || ids[0] != q.ID || m.Pos.X != 220 || m.HP != 100000 || q.HP != 9870 {
		t.Fatal("immune target teleported or out-of-radius damage", ids, m.Pos, m.HP, q.HP)
	}
}
func TestLegendarySwordsmanShieldCapRefreshNoExpiry(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-D03")
	power, _ := domain.LegendaryPower("JK-D03")
	c := legendaryContext{Scene: s, Owner: p, Target: m, Power: power, SourceUID: 991, Skill: 10104}
	swordOverflowShield(&c, 0, 99999)
	a, permanent := swordShield(p)
	if a != 1600 || !permanent {
		t.Fatal(a, permanent)
	}
	p.Status.AbsorbDamage(1000)
	swordOverflowShield(&c, 0, 100)
	a, _ = swordShield(p)
	if a != 600 {
		t.Fatal("lower refresh stacked or replaced higher remaining", a)
	}
	p.MaxHP = 10000
	swordOverflowShield(&c, 0, 900)
	a, _ = swordShield(p)
	if a != 900 {
		t.Fatal("higher refresh did not replace", a)
	}
	p.Status.Expire(s.tick + domain.Ticks(1000000))
	a, _ = swordShield(p)
	if a != 900 {
		t.Fatal("permanent shield acquired timer", a)
	}
}
func TestLegendarySwordsmanStandaloneAndTwoPieceControls(t *testing.T) {
	for _, pieces := range []int{1, 2} {
		s, p, m := legendaryMeleeFixture(t, "JK-A24")
		q := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
		root := domain.StatusDef{ID: 800111, Level: 1, DurationSec: 6, BlockMove: true, TypeKnown: true, Type: domain.StatusBad}
		for _, e := range []*entity.Entity{m, q} {
			e.Status = domain.NewStatusSet()
			e.Status.Apply(root, s.tick, 999)
		}
		if pieces == 2 {
			swordEquip(t, s, p, "JK-D06", domain.SlotBody)
		}
		d := domain.SkillDef{ID: 10126, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}}
		ids := swordCast(t, s, p, m, d)
		if len(ids) != 2 || q.HP != 9820 {
			t.Fatal("standalone strongstrike rejected legal other-source root", ids, q.HP)
		}
	}
	s, p, m := legendaryMeleeFixture(t, "JK-A11")
	p.HP = 9000
	swordCast(t, s, p, m, swordDualDef(10104))
	if p.HP != 9030 {
		t.Fatal("standalone A11 not150%", p.HP)
	}
	s, p, m = legendaryMeleeFixture(t, "JK-A16")
	swordEquip(t, s, p, "JK-D05", domain.SlotBody)
	m.Pos.X = 220
	s.aoi.Move(m, m.Pos)
	swordCast(t, s, p, m, domain.SkillDef{ID: 10121, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}})
	c := legendaryContext{Scene: s, Owner: p, Target: m}
	if n := legendaryHandlers["JK-D05"].BeforeIncoming(&c, 100); n != 60 {
		t.Fatal("two-piece gather did not enable proximity defense", n)
	}
}
func TestLegendarySwordsmanSilenceAndArmorRefresh(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-A13")
	st := domain.StatusDef{ID: domain.StatusSilence, Level: 1, DurationSec: 6, Control: domain.ControlSilence, TypeKnown: true, Type: domain.StatusBad, EffectHitBP: 100000000}
	s.statuses = statusTable(st)
	for i := 0; i < 5; i++ {
		putMonster(s, domain.EntityID(901+i), 10000, domain.Stats{}, 140+float64(i), 100)
	}
	d := domain.SkillDef{ID: 10113, CooldownMS: 120000, HurtType: domain.HurtPhysical, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}, Statuses: []domain.StatusApplication{{ID: st.ID, Level: 1}}}
	s.legendaryPrepareSkill(p, &d, m.ID)
	ids := swordCast(t, s, p, m, d)
	if d.CooldownMS != 24000 || len(ids) != 6 {
		t.Fatal("group silence shape/cooldown", d.CooldownMS, ids)
	}
	for _, id := range ids {
		if s.entities[id].Status.Controlled() != domain.ControlSilence {
			t.Fatal("native silence not retained", id)
		}
	}
	s, p, m = legendaryMeleeFixture(t, "JK-A18")
	legendaryMeleeStrike(s, p, m, 10122)
	legendaryMeleeStrike(s, p, m, 10122)
	count := 0
	m.Status.EachEffective(func(a domain.Active) {
		if a.Def.LegendaryPowerID == "JK-A18" {
			count++
			if a.Def.Affixes[0].Value != -30 || a.ExpireAt != s.tick+domain.Ticks(8000) {
				t.Fatal(a)
			}
		}
	})
	if count != 1 {
		t.Fatal("armor stacked", count)
	}
}

func TestLegendarySwordsmanGatherAdmissionDoesNotMoveOrRequireInside100(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "JK-A16")
	m.Pos.X = 220
	s.aoi.Move(m, m.Pos)
	before := m.Pos
	d := domain.SkillDef{ID: 10121, Kind: domain.SkillArea, TargetEnemy: true, Radius: 100}
	ids := s.legendarySelectTargets(p, d, 0, nil)
	if len(ids) != 1 || ids[0] != m.ID || m.Pos != before {
		t.Fatal("pull admission rejected outer target or moved before commit", ids, m.Pos)
	}
}
