package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"testing"
	"time"
)

func wearEquipmentV5(t *testing.T, s *Scene, p *entity.Entity, id domain.ItemID, uid int64) domain.Stack {
	t.Helper()
	d, ok := domain.UniqueForBase(id, catalogfixture.EndgameCatalog())
	if !ok {
		t.Fatal(id)
	}
	if s.items == nil {
		s.items = map[domain.ItemID]domain.ItemDef{}
	}
	def := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Name: d.BaseName, InstanceKind: domain.ItemInstanceEquipment, InventoryTab: 2, InventoryTabKnown: true, Equip: &domain.EquipDef{Slot: d.Slot, Durable: 100}}
	s.items[id] = def
	st := domain.Stack{UID: uid, Item: id, Count: 1, InstanceKind: domain.ItemInstanceEquipment, Durability: 100, MaxDurability: 100}
	if !domain.ApplyUnique(&st, def, func(int) int { return 2000 }) {
		t.Fatal("unique fixture")
	}
	p.Player.Char.Level = 100
	p.Player.Char.Base = domain.Base{STR: 1000, VIT: 1000, INT: 1000, SPI: 1000, AGI: 1000, DEX: 1000}
	p.Player.Worn.Set(domain.EquipSlot(d.Slot), st)
	return st
}
func cacheElementV5(s *Scene, p *entity.Entity, stats domain.EquipmentCombatStats) {
	c := s.equipmentEffects(p)
	c.stats = stats
	s.equipmentState().cache[p.ID] = c
}
func TestEquipmentElementV5ActualStrikeOrdering(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "攻击", 100, 100)
	join(t, s, 2, 200, "防御", 110, 100)
	a, b := s.players[1], s.players[2]
	// Monster source uses its own supplied offense snapshot; no owner borrowing.
	m := putMonster(s, 900, 10000, domain.Stats{MAtk: 1000}, 100, 100)
	b.HP, b.MaxHP = 2000, 2000
	b.Stats = domain.Stats{}
	var offense, defense domain.EquipmentCombatStats
	offense.Elements[0] = domain.ElementCombatStats{DamageBP: 3000, PierceBP: 1000}
	defense.Elements[0] = domain.ElementCombatStats{ResistBP: 4000, MitigationBP: 1500, AbsorbBP: 1000}
	cacheElementV5(s, b, defense)
	ev, _, _ := s.resolveStrikeStatuses(m, b, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire, EquipmentSnapshot: true, EquipmentOffense: offense})
	if ev.Amount != 696 || ev.AbsorbHeal != 40 || b.HP != 1344 || ev.DstHP != 1304 {
		t.Fatalf("%+v hp%d", ev, b.HP)
	}
	// All elements share one real rolling restoration quota.
	b.HP = 2000
	defense.Elements[1] = defense.Elements[0]
	cacheElementV5(s, b, defense)
	offense.Elements[1] = offense.Elements[0]
	ev, _, _ = s.resolveStrikeStatuses(m, b, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtIce, EquipmentSnapshot: true, EquipmentOffense: offense})
	if ev.AbsorbHeal != 0 {
		t.Fatal("quota per element", ev)
	}
	b.HP = 1
	ev, _, _ = s.resolveStrikeStatuses(m, b, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
	if b.HP != 0 || !ev.Flag.Has(event.DamageFatal) || ev.AbsorbHeal != 0 {
		t.Fatal("absorbed resurrection", ev)
	}
	_ = a
}
func TestEquipmentAbsorbV5ShieldReflectionAndOrigins(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	p.HP, p.MaxHP = 1000, 1000
	p.Stats = domain.Stats{}
	var stats domain.EquipmentCombatStats
	stats.Elements[0].AbsorbBP = 2000
	cacheElementV5(s, p, stats)
	m := putMonster(s, 900, 1000, domain.Stats{MAtk: 100}, 110, 100)
	for _, origin := range []domain.DamageOrigin{domain.DamageProcOrigin, domain.DamageAuraOrigin, domain.DamageReflectionOrigin, domain.DamageDOTOrigin, domain.DamageEnvironmentOrigin} {
		p.HP = 1000
		ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire, Origin: origin})
		if ev.Amount != 100 || ev.AbsorbHeal != 0 || p.HP != 900 {
			t.Fatal(origin, ev)
		}
	}
}
func TestEquipmentProcV5ActualRootFixedRankAndCooldown(t *testing.T) {
	s := newCombatScene(t)
	clock := time.Unix(1700000000, 0)
	s.now = func() time.Time { return clock }
	sink := join(t, s, 1, 100, "术士", 100, 100)
	p := s.players[1]
	legacy := wearEquipmentV5(t, s, p, 1306, 9001)
	// This regression asserts the frozen V5 proc payload and its exact damage.
	legacy.Endgame.UniqueContentVersion = 5
	p.Player.Worn.Set(domain.SlotTwoHand, legacy)
	p.Stats = domain.Stats{MAtk: 100}
	p.HP, p.MaxHP = 1000, 1000
	proc := domain.SkillDef{ID: 10401, Level: 10, HurtType: domain.HurtFire, Dist: 450, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 200}, Statuses: []domain.StatusApplication{{ID: domain.StatusFreeze, Level: 1}}}
	s.skills = skillTable(proc)
	p.Player.Char.Skills = domain.Learned{10401: 20}
	ids := []domain.EntityID{}
	for i := 0; i < 20; i++ {
		id := domain.EntityID(900 + i)
		putMonster(s, id, 10000, domain.Stats{}, 110+float64(i), 100)
		ids = append(ids, id)
	}
	// Choose deterministic independent proc stream success for the next root.
	root := s.equipmentState().sequence + 1
	for s.equipmentProcRoll(p.ID, "firebolt_echo", root) >= 1000 {
		s.equipmentSeed++
	}
	s.casts = append(s.casts, pendingCast{src: p.ID, def: domain.SkillDef{ID: 10402, HurtType: domain.HurtFire, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}, targets: ids, primary: ids[10], visualTarget: ids[10]})
	s.stepCasts()
	s.flush()
	count := 0
	for _, e := range sink.take() {
		if d, ok := e.(event.DamageDealt); ok && d.Origin == domain.DamageProcOrigin {
			count++
			if d.Amount != 200 || d.Dst != ids[10] || d.ProcDepth != 1 {
				t.Fatal(d)
			}
		}
	}
	if count != 1 {
		t.Fatalf("proc count %d across20targets", count)
	}
	if s.entities[ids[10]].Status != nil && s.entities[ids[10]].Status.Count() > 0 {
		t.Fatal("proc copied control")
	}
	old := p.Player.Char.EquipmentProcCooldowns["effect:firebolt_echo"]
	if old != clock.UnixMilli()+4000 {
		t.Fatal(old)
	}
	st := p.Player.Worn.At(domain.SlotTwoHand)
	p.Player.Worn.Set(domain.SlotTwoHand, domain.Stack{})
	_ = s.equipmentEffects(p)
	p.Player.Worn.Set(domain.SlotTwoHand, st)
	procDef := s.equipmentEffects(p).effects[0].Effect.Proc
	if procDef == nil {
		for _, a := range s.equipmentEffects(p).effects {
			if a.Effect.Proc != nil {
				procDef = a.Effect.Proc
			}
		}
	}
	if procDef == nil || s.reserveEquipmentProc(p, procDef, 999) {
		t.Fatal("reequip reset cooldown")
	}
	snap := s.snapshotOf(p)
	s.exec(Leave{ID: p.ID})
	other := newCombatScene(t)
	other.now = s.now
	other.items = s.items
	other.exec(Enter{ID: 50, Char: snap.Char, Bag: snap.Bag, Worn: snap.Worn, Sink: &fakeSink{}})
	if other.players[50].Player.Char.EquipmentProcCooldowns["effect:firebolt_echo"] != old {
		t.Fatal("reconnect reset cooldown")
	}
}
func TestEquipmentProcV5DeadAndSecondaryNeverTrigger(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	wearEquipmentV5(t, s, p, 2509, 9100)
	p.HP, p.MaxHP = 10, 1000
	p.Stats = domain.Stats{}
	m := putMonster(s, 900, 1000, domain.Stats{MinAtk: 100, MaxAtk: 100}, 110, 100)
	done := s.beginEquipmentRoot(m.ID, p.ID, domain.EquipmentCombatStats{})
	ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
	done()
	if !ev.Flag.Has(event.DamageFatal) || p.HP != 0 || len(p.Player.Char.EquipmentProcCooldowns) > 0 {
		t.Fatal("death proc", ev)
	}
	p.HP = 200
	done = s.beginEquipmentRoot(m.ID, p.ID, domain.EquipmentCombatStats{})
	s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true, Origin: domain.DamageProcOrigin, ProcDepth: 1})
	done()
	if len(p.Player.Char.EquipmentProcCooldowns) > 0 {
		t.Fatal("recursive proc")
	}
}
func TestEquipmentAuraV5PartyRangeFallbackAOIAndTeardown(t *testing.T) {
	s := newCombatScene(t)
	ownerSink := join(t, s, 1, 100, "来源", 100, 100)
	remoteSink := join(t, s, 2, 200, "队友", 120, 100)
	join(t, s, 3, 300, "弱来源", 130, 100)
	p, q, r := s.players[1], s.players[2], s.players[3]
	p.Player.Party, q.Player.Party, r.Player.Party = 1, 1, 1
	wearFormationAuraTest(t, s, p, 16, 9001)
	wearFormationAuraTest(t, s, r, 16, 9002)
	aura := formationAuraTest(t, 16)
	s.stepEquipmentAuras()
	s.flush()
	if got := s.equipmentDefense(q, false).PhysicalMitigationBP; got != aura.Value {
		t.Fatal(got)
	}
	if got := s.equipmentDefense(q, true).PhysicalMitigationBP; got != aura.Value/2 {
		t.Fatal(got)
	}
	if g := s.equipmentState().auras[q.ID][aura.Family]; g.owner != p.ID {
		t.Fatal("unstable tie", g)
	}
	foundOwn, foundRemote := false, false
	for _, e := range ownerSink.take() {
		if b, ok := e.(event.BuffSnapshot); ok {
			for _, a := range b.Buffs {
				if a.Name == aura.VisualKey {
					foundOwn = true
				}
			}
		}
	}
	for _, e := range remoteSink.take() {
		if b, ok := e.(event.EntityStatusEffect); ok && b.Who == p.ID && b.Effect == aura.VisualKey && b.DurationMS > 0 {
			foundRemote = true
		}
	}
	if !foundOwn || !foundRemote {
		t.Fatal("missing owner/observer aura", foundOwn, foundRemote)
	}
	s.sendEntityStatusEffects(q.ID, p)
	s.flush()
	if len(remoteSink.take()) == 0 {
		t.Fatal("AOI replay empty")
	}
	p.HP = 0
	s.stepEquipmentAuras()
	if s.equipmentState().auras[q.ID][aura.Family].owner != r.ID {
		t.Fatal("no surviving source fallback")
	}
	q.Player.Party = 0
	s.stepEquipmentAuras()
	if len(s.equipmentState().auras[q.ID]) != 0 {
		t.Fatal("left-party buff")
	}
	q.Player.Party = 1
	s.tick += 5
	s.stepEquipmentAuras()
	s.aoi.Move(q, domain.Pos{MapID: 7, X: 1000, Y: 1000})
	s.tick += 5
	s.stepEquipmentAuras()
	if len(s.equipmentState().auras[q.ID]) != 0 {
		t.Fatal("out of radius")
	}
	s.exec(Leave{ID: r.ID})
	if len(s.equipmentState().auras) > 0 {
		t.Fatal("disconnect aura residue")
	}
}
func TestGrowthCapabilityV5CombatCannotBypassSession(t *testing.T) {
	s := newCombatScene(t)
	s.requireGrowthV5 = true
	join(t, s, 1, 100, "旧客户端", 100, 100)
	p := s.players[1]
	m := putMonster(s, 900, 1000, sureHit, 110, 100)
	p.Stats = sureHit
	hp := p.HP
	ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
	if ev.Amount != 0 || p.HP != hp || s.validMonsterTarget(p) {
		t.Fatal("unnegotiated combat", ev)
	}
	p.Player.EndgameGrowthV5 = true
	ev, _, _ = s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
	if ev.Amount <= 0 {
		t.Fatal("negotiation did not unlock")
	}
}

func TestEquipmentAbsorbV5ShieldsAndFullReflectCommit(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	p.HP, p.MaxHP = 1000, 1000
	p.Stats = domain.Stats{}
	m := putMonster(s, 900, 1000, domain.Stats{MAtk: 100}, 110, 100)
	var stats domain.EquipmentCombatStats
	stats.Elements[0].AbsorbBP = 2000
	p.Status = domain.NewStatusSet()
	p.Status.Apply(domain.StatusDef{ID: domain.StatusDamageShield, Level: 1, DamageShield: 1000, DurationSec: 100}, s.tick, p.ID)
	cacheElementV5(s, p, stats)
	ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
	if ev.Amount != 0 || ev.AbsorbHeal != 0 || p.HP != 1000 {
		t.Fatal("shield fed heal", ev)
	}
	p.Status = domain.NewStatusSet()
	p.Status.Apply(domain.StatusDef{ID: 1011, Level: 1, MagicFullReflectChancePct: 100, DurationSec: 100}, s.tick, p.ID)
	cacheElementV5(s, p, stats)
	ev, raw, reflection := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
	if ev.Amount != 0 || ev.AbsorbHeal != 0 || reflection != 100 || p.HP != 1000 {
		t.Fatal("reflection absorbed", ev, reflection)
	}
	s.afterStrikeStatuses(m, p, ev, raw, reflection)
	if m.HP != 900 {
		t.Fatal(m.HP)
	}
}
func TestEquipmentAuraV5PetBrokenReequipAndTransfer(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "来源", 100, 100)
	join(t, s, 2, 200, "队友", 120, 100)
	p, q := s.players[1], s.players[2]
	p.Player.Party, q.Player.Party = 1, 1
	st := wearFormationAuraTest(t, s, p, 24, 9010)
	aura := formationAuraTest(t, 24)
	pet := &entity.Entity{ID: 800, Kind: domain.KindPet, HP: 100, MaxHP: 100, Pos: domain.Pos{MapID: 7, X: 125, Y: 100}, Pet: &entity.Pet{Owner: q.ID}}
	q.Player.Pet = pet.ID
	s.entities[pet.ID] = pet
	s.pets[pet.ID] = pet
	s.aoi.Enter(pet)
	s.stepEquipmentAuras()
	if v := s.equipmentDefense(pet, false).Elements[0].ResistBP; v != aura.Value {
		t.Fatal("activepet did not gain aura", v)
	}
	d := s.items[st.Item]
	st.Durability = 0
	p.Player.Worn.Set(domain.EquipSlot(d.Equip.Slot), st)
	s.stepEquipmentAuras()
	if len(s.equipmentState().auras) > 0 {
		t.Fatal("broken source aura")
	}
	st.Durability = 100
	p.Player.Worn.Set(domain.EquipSlot(d.Equip.Slot), st)
	s.stepEquipmentAuras()
	if s.equipmentDefense(pet, false).Elements[0].ResistBP != aura.Value {
		t.Fatal("repair missed")
	}
	s.aoi.Leave(p)
	delete(s.entities, p.ID)
	delete(s.players, p.ID)
	s.removeEquipmentEntity(p.ID)
	if len(s.equipmentState().auras) > 0 {
		t.Fatal("transfer leaked source")
	}
}
func TestEquipmentProcV5SafePVPAndHealScaling(t *testing.T) {
	s := newCombatScene(t)
	clock := time.Unix(1700000000, 0)
	s.now = func() time.Time { return clock }
	join(t, s, 1, 100, "治疗者", 100, 100)
	join(t, s, 2, 200, "敌方", 110, 100)
	p, q := s.players[1], s.players[2]
	wearEquipmentV5(t, s, p, 2509, 9003)
	p.HP, p.MaxHP = 200, 1000
	p.Stats = domain.Stats{}
	q.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100}
	if s.equipmentProcEnemyTarget(p, q) {
		t.Fatal("proc silently enabled PvP")
	}
	q.Player.Party, p.Player.Party = 1, 1
	if s.equipmentProcEnemyTarget(p, q) {
		t.Fatal("friendly proc")
	}
	q.Player.Party, p.Player.Party = 0, 0
	s.skills = skillTable(domain.SkillDef{ID: 10301, Level: 5, Effect: domain.SkillEffect{Kind: domain.EffectHeal, HealFlat: 200}})
	root := s.equipmentState().sequence + 1
	for s.equipmentProcRoll(p.ID, "emergency_heal", root) >= 2000 {
		s.equipmentSeed++
	}
	finish := s.beginEquipmentRoot(q.ID, p.ID, domain.EquipmentCombatStats{})
	ev, _, _ := s.resolveStrikeStatuses(q, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
	if ev.Amount != 100 {
		t.Fatal(ev)
	}
	finish()
	if p.HP != 100 || len(p.Player.Char.EquipmentProcCooldowns) > 0 {
		t.Fatal("safe map accepted offensive PvP trigger")
	}
	var active domain.ActiveEquipmentEffect
	for _, a := range s.equipmentEffects(p).effects {
		if a.Effect.Proc != nil {
			active = a
			break
		}
	}
	s.executeEquipmentProc(p, equipmentProcCandidate{active: active, owner: p.ID, pvp: true, hit: equipmentHit{src: q.ID, dst: p.ID, ev: ev}})
	if p.HP != 170 {
		t.Fatal("PvP-caused self-heal lost35percent coefficient", p.HP)
	}
}
func TestEquipmentProcV5RandomIndependentAndCooldownBounds(t *testing.T) {
	a, b := newCombatScene(t), newCombatScene(t)
	join(t, a, 1, 100, "甲", 100, 100)
	join(t, b, 1, 100, "甲", 100, 100)
	for i := uint64(0); i < 100; i++ {
		a.equipmentProcRoll(1, "example", i)
	}
	if a.rng.Int63() != b.rng.Int63() {
		t.Fatal("proc roll consumed combat/loot stream")
	}
	p := a.players[1]
	zero := &domain.EquipmentProc{ID: "zero", SharedGroup: "zero", ICDMS: 4000, ChanceBP: 0}
	if a.reserveEquipmentProc(p, zero, 1) {
		t.Fatal("zero proc")
	}
	full := &domain.EquipmentProc{ID: "full", SharedGroup: "full", ICDMS: 4000, ChanceBP: 10000}
	if !a.reserveEquipmentProc(p, full, 1) {
		t.Fatal("100percent failed")
	}
	if !a.reserveEquipmentProc(p, &domain.EquipmentProc{ID: "other", SharedGroup: "other", ICDMS: 4000, ChanceBP: 10000}, 1) {
		t.Fatal("independent proc blocked by obsolete global window")
	}
}

func TestEquipmentProcV5AcceptedSkillSnapshotsAndCancelledSource(t *testing.T) {
	s := newCombatScene(t)
	clock := time.Unix(1700000000, 0)
	s.now = func() time.Time { return clock }
	sink := join(t, s, 1, 100, "术士", 100, 100)
	p := s.players[1]
	wearEquipmentV5(t, s, p, 1306, 9001)
	p.Player.Char.Race = domain.Warlock
	p.Player.Char.Skills = domain.Learned{10402: 1}
	p.Stats = domain.Stats{MAtk: 100}
	p.HP, p.MaxHP, p.MP, p.MaxMP = 1000, 1000, 10000, 10000
	base := domain.SkillDef{ID: 10402, Level: 1, Kind: domain.SkillSingle, Prof: 5, HurtType: domain.HurtFire, TargetEnemy: true, Dist: 450, PrepareMS: 100, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
	fixed := base
	fixed.ID = 10401
	fixed.Level = 10
	fixed.Effect.DamagePct = 200
	fixed.PrepareMS = 0
	s.skills = skillTable(base, fixed)
	m := putMonster(s, 900, 10000, domain.Stats{}, 110, 100)
	root := s.equipmentState().sequence + 1
	for s.equipmentProcRoll(p.ID, "firebolt_echo", root) >= 1000 {
		s.equipmentSeed++
	}
	var stats domain.EquipmentCombatStats
	stats.Elements[0].DamageBP = 1000
	cacheElementV5(s, p, stats)
	s.exec(UseSkill{ID: p.ID, Skill: base.ID, Target: m.ID})
	if len(s.casts) != 1 {
		t.Fatalf("accepted command failed %v", sink.take())
	}
	// New elemental offense is frozen when cast accepted; losing the source still
	// cancels any pending proc instead of allowing an unequipped skill discharge.
	p.Player.Worn = domain.NewEquipSet()
	p.Stats.MAtk = 10000 // Accepted projectile must not inherit newly equipped stats.
	clock = clock.Add(time.Second)
	s.stepCastsAt(clock, false)
	s.flush()
	if m.HP != 9890 {
		t.Fatalf("accepted offense snapshot changed on unequip: %d", m.HP)
	}
	for _, e := range sink.take() {
		if d, ok := e.(event.DamageDealt); ok && d.Origin == domain.DamageProcOrigin {
			t.Fatal("unequipped proc fired", d)
		}
	}
}

func TestExplicitZeroFlatCarrierV5ActualStrike(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "高攻控制", 100, 100)
	p := s.players[1]
	p.Stats = domain.Stats{MinAtk: 100000, MaxAtk: 100000, MAtk: 100000}
	m := putMonster(s, 900, 1000, domain.Stats{MDef: 1000, MagicResist: 1000, PhysResist: 1000}, 110, 100)
	for _, magic := range []bool{false, true} {
		m.HP = 1000
		ev, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{Magic: magic, GuaranteedHit: true, SkillPctNumerator: 0, SkillPctDenominator: 1, FlatDamage: 1})
		if ev.Amount != 1 || m.HP != 999 {
			t.Fatal("flat carrier inherited full attack/minimum baseline", magic, ev)
		}
	}
}

func TestEquipmentAbsorbV5TransferRetainsRollingQuota(t *testing.T) {
	a, b := newCombatScene(t), newCombatScene(t)
	clock := time.Unix(1700000000, 0)
	a.now = func() time.Time { return clock }
	b.now = a.now
	join(t, a, 1, 100, "来源", 100, 100)
	join(t, b, 2, 200, "目标", 100, 100)
	p, q := a.players[1], b.players[2]
	p.HP, p.MaxHP = 1000, 1000
	q.MaxHP = 1000
	first := a.equipmentAbsorb(p, 100, 2000, false)
	if first.Heal != 20 {
		t.Fatal(first)
	}
	p.HP = first.HP
	runtime := p.CapturePlayerRuntime(a.tick)
	q.RestorePlayerRuntime(runtime, b.tick)
	second := b.equipmentAbsorb(q, 100, 2000, false)
	if second.Heal != 0 {
		t.Fatal("map transfer reset shared restoration quota", second)
	}
	q.Player.EquipmentAbsorbRestores[0].Amount = 1
	if p.Player.EquipmentAbsorbRestores[0].Amount != 20 {
		t.Fatal("runtime quota alias")
	}
	clock = clock.Add(time.Second)
	third := b.equipmentAbsorb(q, 100, 2000, false)
	if third.Heal != 20 {
		t.Fatal("rolling quota did not expire", third)
	}
}
func TestEquipmentProcV5FatalPrimaryDoesNotRetarget(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "术士", 100, 100)
	p := s.players[1]
	legacy := wearEquipmentV5(t, s, p, 1306, 9010)
	// This regression asserts the frozen V5 proc payload and its exact damage.
	legacy.Endgame.UniqueContentVersion = 5
	p.Player.Worn.Set(domain.SlotTwoHand, legacy)
	p.Stats = domain.Stats{MAtk: 100}
	p.HP, p.MaxHP = 1000, 1000
	s.skills = skillTable(domain.SkillDef{ID: 10401, Level: 10, HurtType: domain.HurtFire, Dist: 450, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 200}})
	primary := putMonster(s, 900, 1, domain.Stats{}, 110, 100)
	other := putMonster(s, 901, 1000, domain.Stats{}, 120, 100)
	root := s.equipmentState().sequence + 1
	for s.equipmentProcRoll(p.ID, "firebolt_echo", root) >= 1000 {
		s.equipmentSeed++
	}
	finish := s.beginEquipmentRoot(p.ID, primary.ID, s.equipmentOffense(p))
	s.resolveStrikeStatuses(p, primary, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
	s.resolveStrikeStatuses(p, other, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
	finish()
	if primary.HP != 0 || other.HP != 900 || p.Player.Char.EquipmentProcCooldowns["effect:firebolt_echo"] == 0 {
		t.Fatal("dead primary illegally retargeted/refunded CD", other.HP, p.Player.Char.EquipmentProcCooldowns)
	}
}

func BenchmarkEquipmentV5CachedLookup(b *testing.B) {
	s := New(Config{Seed: 1})
	p := &entity.Entity{ID: 1, Kind: domain.KindPlayer, Player: &entity.Player{Char: &domain.Character{Level: 1}, Worn: domain.NewEquipSet()}}
	s.equipmentEffects(p)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.equipmentEffects(p)
	}
}

func TestEquipmentAbsorbV5ElementTrapAndSelfDamage(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	p.HP, p.MaxHP = 1000, 1000
	p.Stats = domain.Stats{MAtk: 100}
	m := putMonster(s, 900, 1000, domain.Stats{MAtk: 100}, 110, 100)
	var stats domain.EquipmentCombatStats
	stats.Elements[0].AbsorbBP = 2000
	cacheElementV5(s, p, stats)
	ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire, Trap: true})
	if ev.Amount != 80 || ev.AbsorbHeal != 20 || p.HP != 940 {
		t.Fatal("tagged direct trap bypassed absorb", ev)
	}
	p.HP = 1000
	p.Player.EquipmentAbsorbRestores = nil
	ev, _, _ = s.resolveStrikeStatuses(p, p, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
	if ev.Amount != 100 || ev.AbsorbHeal != 0 {
		t.Fatal("selfdamage generated absorption", ev)
	}
}

func TestEquipmentProcV5RespectsNativeSkillImmunity(t *testing.T) {
	s := newCombatScene(t)
	sink := join(t, s, 1, 100, "触发者", 100, 100)
	p := s.players[1]
	wearEquipmentV5(t, s, p, 1306, 9001)
	p.Stats = domain.Stats{MAtk: 100}
	p.HP, p.MaxHP = 1000, 1000
	m := putMonster(s, 900, 1000, domain.Stats{}, 110, 100)
	s.defs = spawn.MapDefs{338: domain.MonsterDef{ImmuneSkills: map[domain.SkillID]struct{}{10401: {}}}}
	s.skills = skillTable(domain.SkillDef{ID: 10401, Level: 10, HurtType: domain.HurtFire, Dist: 450, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 200}})
	var active domain.ActiveEquipmentEffect
	for _, a := range s.equipmentEffects(p).effects {
		if a.Effect.Proc != nil {
			active = a
			break
		}
	}
	s.executeEquipmentProc(p, equipmentProcCandidate{active: active, owner: p.ID, hit: equipmentHit{src: p.ID, dst: m.ID}})
	s.flush()
	if m.HP != 1000 {
		t.Fatal("proc bypassed native skill immunity", m.HP)
	}
	miss := false
	for _, raw := range sink.take() {
		if ev, ok := raw.(event.DamageDealt); ok && ev.SkillID == 10401 && ev.Flag.Has(event.DamageMiss) {
			miss = true
		}
	}
	if !miss {
		t.Fatal("missing immune miss event")
	}
}

func TestEquipmentTrapV5UsesLesserCastAndCurrentOffense(t *testing.T) {
	s := newCombatScene(t)
	s.alloc = domain.NewEntityAlloc()
	join(t, s, 1, 100, "陷阱施法者", 100, 100)
	p := s.players[1]
	p.HP, p.MaxHP = 1000, 1000
	m := putMonster(s, 900, 1000, domain.Stats{}, 110, 100)
	s.monsters[m.ID] = m
	def := domain.SkillDef{ID: 10214, Level: 1, HurtType: domain.HurtFire, Effects: []domain.SkillEffect{{Kind: domain.EffectPlaceTrap, TrapRadius: 100}, {Kind: domain.EffectDamage, DamagePct: 100}}}
	for _, tc := range []struct {
		current      int32
		currentBonus int32
		want         int32
	}{{10000, 5000, 110}, {50, 0, 50}} {
		m.HP = 1000
		p.Stats = domain.Stats{MAtk: 100}
		var offense domain.EquipmentCombatStats
		offense.Elements[0].DamageBP = 1000
		cacheElementV5(s, p, offense)
		s.placeTrap(p, def, domain.Pos{MapID: 7, X: 110, Y: 100}, 100, 0)
		p.Stats.MAtk = tc.current
		offense.Elements[0].DamageBP = tc.currentBonus
		cacheElementV5(s, p, offense)
		s.stepTraps()
		if m.HP != 1000-tc.want {
			t.Fatalf("current%d damage%d want%d", tc.current, 1000-m.HP, tc.want)
		}
		if len(s.traps) != 0 {
			t.Fatal("triggered trap not removed")
		}
	}
}

func TestEquipmentV5AcceptedResourceConversionSnapshot(t *testing.T) {
	s := newCombatScene(t)
	clock := time.Unix(1700000000, 0)
	s.now = func() time.Time { return clock }
	sink := join(t, s, 1, 100, "资源施法者", 100, 100)
	p := s.players[1]
	p.Player.Char.Race = domain.Warlock
	p.Player.Char.Skills = domain.Learned{10402: 1}
	p.Stats = domain.Stats{MaxAtk: 100}
	p.HP, p.MaxHP, p.MP, p.MaxMP = 100, 1000, 100, 1000
	def := domain.SkillDef{ID: 10402, Level: 1, Kind: domain.SkillSingle, Prof: 5, TargetSelf: true, Dist: 450, PrepareMS: 100, Effects: []domain.SkillEffect{
		{Kind: domain.EffectRestoreHP, SourceAttackPct: 100},
		{Kind: domain.EffectRestoreMP, SourceAttackPct: 100},
		{Kind: domain.EffectDamageMP, SourceAttackPct: 50},
	}}
	s.skills = skillTable(def)
	s.exec(UseSkill{ID: p.ID, Skill: def.ID, Target: p.ID})
	if len(s.casts) != 1 {
		t.Fatalf("accepted command failed %v", sink.take())
	}
	p.Stats.MaxAtk = 10000
	clock = clock.Add(time.Second)
	s.stepCastsAt(clock, false)
	if p.HP != 200 || p.MP != 150 {
		t.Fatalf("resource payload used impact stats: hp%d mp%d", p.HP, p.MP)
	}
}
