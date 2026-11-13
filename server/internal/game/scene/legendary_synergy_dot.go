package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math"
	"math/big"
)

// Snapshot version zero is legacy. No login-time conversion is performed.
func synergyScale64(n, mul, div int64) int64 {
	if div <= 0 {
		return 0
	}
	if n == 0 || mul == 0 {
		return 0
	}
	var x big.Int
	x.Mul(big.NewInt(n), big.NewInt(mul))
	x.Quo(&x, big.NewInt(div))
	if !x.IsInt64() {
		if x.Sign() < 0 {
			return math.MinInt64
		}
		return math.MaxInt64
	}
	return x.Int64()
}
func synergyQualifyingDOT(d domain.StatusDef) bool {
	if !d.DamageOverTime() || d.IntervalSec <= 0 || d.DurationSec <= 0 || d.Permanent || d.DamageSourceTrap && d.SourceSkillID == 0 && d.SkillSnapshotID == 0 {
		return false
	}
	if !d.SkillApplied && d.SourceSkillID == 0 && d.SkillSnapshotID == 0 && d.LegendaryPowerID == "" {
		return false
	}
	if d.ManaToHealthRate > 0 {
		return false
	}
	for _, v := range d.HPChanges {
		if v.Value < 0 && v.Mode != domain.ModeAbsolute {
			return false
		}
	}
	for _, v := range d.Affixes {
		if v.Attr == domain.AttrHPRegen && v.Value < 0 && v.Mode != domain.ModeAbsolute {
			return false
		}
	}
	return true
}
func warriorSynergyDOTHandler() legendaryHandler {
	h := warriorSkill(10002, 200, 0, 0, 0)
	h.AfterHit = synergyProfessionDOT
	return h
}
func synergyProfessionDOT(c *legendaryContext) {
	if c.Owner == nil || c.Target == nil || !c.Target.Alive() || c.Hit.Flag.Has(event.DamageMiss) || c.Blow.Origin == domain.DamageDOTOrigin || c.Blow.ProcDepth > 0 && !c.IsChild || !c.Scene.equipmentProcEnemyTarget(c.Owner, c.Target) {
		return
	}
	skill, pct, physical, name := domain.SkillID(10002), int32(300), true, "流血"
	switch c.Power.ID {
	case "JK-A01":
		skill, pct = 10102, 360
	case "YS-A03":
		skill, pct, physical, name = 10319, 420, false, "灼光"
	case "ZS-A01":
	default:
		return
	}
	if c.Skill != skill {
		return
	}
	// Per root/target, including multi-hit and child casts: one admitted snapshot.
	if c.RootID != 0 {
		st := c.State(c.Target.ID, "dot-root")
		if st.RootID == c.RootID {
			return
		}
		st.RootID = c.RootID
	}
	pct = c.RollValue("dot_total_pct", pct)
	amount := max(int64(0), int64(c.Attack(!physical))) * int64(pct) / 100
	if amount <= 0 {
		return
	}
	d := domain.StatusDef{ID: legendaryMeleeStatusID(c, "synergy-dot"), Level: 1, Name: name, Desc: c.Power.Effect, TypeKnown: true, Type: domain.StatusBad, DurationSec: 6, IntervalSec: 1, LegendaryPowerID: c.Power.ID, LegendarySourceUID: c.SourceUID, SkillApplied: true, SourceSkillID: int32(skill), Icon: legendaryMeleeIcon(c), IconKnown: true, SynergyDOTVersion: 1, SynergyDOTPhysical: physical, SynergyDOTNew: true, ClassSetPoisonBonusTotal: amount}
	c.Scene.applyStatusDef(c.Target, d, c.Owner.ID)
}

func (s *Scene) synergySnapshotDOT(owner *entity.Entity, d *domain.StatusDef) {
	if owner == nil || owner.Player == nil || d == nil || d.LegendaryModifiers.Applied || !synergyQualifyingDOT(*d) {
		return
	}
	growth := false
	s.forEachLegendary(owner, func(c *legendaryContext) {
		if c.Power.ID == "TY-A18" {
			growth = true
		}
	})
	if !growth {
		return
	}
	d.SynergyDOTVersion = 1
	if d.ClassSetPoison && d.ClassSetPoisonBonusTotal > 0 {
		return
	} // Native 10P poison is already above the floor.
	st := s.equipmentAttackStats(owner)
	p := int64(st.MAtk)
	if d.SynergyDOTPhysical || d.ID == domain.StatusPoison {
		p = (int64(st.MinAtk) + int64(st.MaxAtk)) / 2
	}
	per100 := d.PeriodicDamageFraction100
	if d.DurationSec > 0 {
		per100 += synergyScale64(d.ClassSetPoisonBonusTotal, int64(d.IntervalSec)*100, int64(d.DurationSec))
	}
	for _, v := range d.HPChanges {
		per100 -= int64(v.Value) * 100
	}
	for _, v := range d.Affixes {
		if v.Attr == domain.AttrHPRegen {
			per100 -= int64(v.Value) * 100
		}
	}
	floor100 := synergyScale64(max(int64(0), p)*4, int64(d.IntervalSec)*100, 6)
	if floor100 > per100 {
		d.HPChanges = append([]domain.ResourceChange(nil), d.HPChanges...)
		for i := range d.HPChanges {
			if d.HPChanges[i].Value < 0 {
				d.HPChanges[i].Value = 0
			}
		}
		d.Affixes = append([]domain.Affix(nil), d.Affixes...)
		for i := range d.Affixes {
			if d.Affixes[i].Attr == domain.AttrHPRegen && d.Affixes[i].Value < 0 {
				d.Affixes[i].Value = 0
			}
		}
		d.PeriodicDamageFraction100 = 0
		d.ClassSetPoisonBonusTotal = synergyScale64(max(int64(0), p)*4, int64(d.DurationSec), 6)
	}
}
func synergyNativeHit(c *legendaryContext) bool {
	return c.Owner != nil && c.Target != nil && c.Skill != 0 && !c.IsBasic && !c.Def.LegendaryBasic && !c.IsChild && c.RootID != 0 && c.Blow.Origin == domain.DamageNormalOrigin && c.Blow.ProcDepth == 0 && !c.Blow.LegendaryPayload && !c.Hit.Flag.Has(event.DamageMiss) && c.Scene.equipmentProcEnemyTarget(c.Owner, c.Target)
}
func synergyOnce(c *legendaryContext, phase string) bool {
	st := c.State(0, phase)
	if st.RootID == c.RootID {
		return false
	}
	st.RootID = c.RootID
	return true
}
func synergySkillProc(c *legendaryContext) {
	if !synergyNativeHit(c) || !synergyOnce(c, "skill-proc-root") || c.Scene.rng.Intn(100) >= 25 || !c.Ready(4000) {
		return
	}
	n := domain.SaturateInt32(int64(c.Attack(c.Blow.Magic)) * int64(c.RollValue("proc_pct", 800)) / 100)
	if c.Scene.equipmentPVP(c.Owner, c.Target) {
		n /= 4
	}
	c.Damage(c.Target, n, c.Blow.Magic, false)
}
func synergySkillShield(c *legendaryContext) {
	if !synergyNativeHit(c) || !synergyOnce(c, "skill-shield-root") || !c.Ready(4000) {
		return
	}
	n := domain.SaturateInt32(int64(c.Owner.MaxHP) * int64(c.RollValue("benefit", 20)) / 100)
	c.Scene.applyStatusDef(c.Owner, domain.StatusDef{ID: legendaryMeleeStatusID(c, "skill-shield"), Level: 1, Name: "护势", Desc: c.Power.Effect, TypeKnown: true, Type: domain.StatusGood, DurationSec: 4, DamageShield: n, LegendaryPowerID: c.Power.ID, LegendarySourceUID: c.SourceUID, Icon: legendaryMeleeIcon(c), IconKnown: true}, c.Owner.ID)
}
func synergyReductionBP(c *legendaryContext, bp int32) int32 {
	if c != nil && c.Scene != nil && (c.Blow.PeriodicPVP || c.Scene.equipmentPVP(c.Owner, c.Target)) {
		return bp * 60 / 100
	}
	return bp
}

// Copy scales frozen base and remainders, never re-enters application modifiers.
func synergyScaleActive(a domain.Active, bp int64) domain.Active {
	d := &a.Def
	d.HPChanges = append([]domain.ResourceChange(nil), d.HPChanges...)
	for i := range d.HPChanges {
		if d.HPChanges[i].Value < 0 && d.HPChanges[i].Mode == domain.ModeAbsolute {
			d.PeriodicDamageFraction100 -= int64(d.HPChanges[i].Value) * 100
			d.HPChanges[i].Value = 0
		}
	}
	d.Affixes = append([]domain.Affix(nil), d.Affixes...)
	for i := range d.Affixes {
		if d.Affixes[i].Attr == domain.AttrHPRegen && d.Affixes[i].Value < 0 && d.Affixes[i].Mode == domain.ModeAbsolute {
			d.PeriodicDamageFraction100 -= int64(d.Affixes[i].Value) * 100
			d.Affixes[i].Value = 0
		}
	}
	d.PeriodicDamageFraction100 = synergyScale64(d.PeriodicDamageFraction100, bp, 10000)
	d.ClassSetPoisonBonusTotal = synergyScale64(d.ClassSetPoisonBonusTotal, bp, 10000)
	a.PeriodicDamageRemainder100 = synergyScale64(a.PeriodicDamageRemainder100, bp, 10000)
	a.ClassSetPoisonRemainder = synergyScale64(a.ClassSetPoisonRemainder, bp, 10000)
	return a
}
func synergyFrozenBalance(a domain.Active, now domain.Tick, maxHP int32) int64 {
	if a.Def.IntervalSec <= 0 || a.NextTickAt == 0 || a.ExpireAt <= now || a.NextTickAt > a.ExpireAt {
		return 0
	}
	interval := domain.Ticks(int(a.Def.IntervalSec) * 1000)
	if interval == 0 {
		return 0
	}
	ticks := new(big.Int).SetUint64(uint64((a.ExpireAt-max(a.NextTickAt, now))/interval) + 1)
	var per, balance, part big.Int
	for _, v := range a.Def.HPChanges {
		per.Sub(&per, big.NewInt(int64(resourceDelta(v.Value, v.Mode, maxHP))))
	}
	for _, v := range a.Def.Affixes {
		if v.Attr == domain.AttrHPRegen {
			per.Sub(&per, big.NewInt(int64(resourceDelta(v.Value, v.Mode, maxHP))))
		}
	}
	if per.Sign() > 0 {
		balance.Mul(&per, ticks)
	}
	if a.Def.PeriodicDamageFraction100 > 0 {
		part.Mul(big.NewInt(a.Def.PeriodicDamageFraction100), ticks)
		part.Add(&part, big.NewInt(a.PeriodicDamageRemainder100))
		part.Quo(&part, big.NewInt(100))
		balance.Add(&balance, &part)
	}
	if count := a.Def.DurationSec / a.Def.IntervalSec; count > 0 && a.Def.ClassSetPoisonBonusTotal > 0 {
		part.Mul(big.NewInt(a.Def.ClassSetPoisonBonusTotal), ticks)
		part.Add(&part, big.NewInt(a.ClassSetPoisonRemainder))
		part.Quo(&part, big.NewInt(int64(count)))
		balance.Add(&balance, &part)
	}
	bp := 10000 + int64(min(int32(50000), max(int32(0), a.Def.LegendaryModifiers.DamageTickBP)))
	balance.Mul(&balance, big.NewInt(bp))
	balance.Quo(&balance, big.NewInt(10000))
	if !balance.IsInt64() {
		return math.MaxInt64
	}
	return max(int64(0), balance.Int64())
}

func synergySpread(c *legendaryContext, a domain.Active) {
	if c.Owner == nil || c.Target == nil || a.Source != c.Owner.ID || !synergyQualifyingDOT(a.Def) || a.Def.SynergyDOTCopy || a.Def.LegendaryPoisonCopy || a.Def.SynergySpreadNext > c.Scene.tick || a.ExpireAt <= c.Scene.tick {
		return
	}
	next := c.Scene.tick + domain.Ticks(2000)
	c.Target.Status.TransformSourceStatus(a.Def.ID, a.Source, func(x domain.Active) domain.Active { x.Def.SynergySpreadNext = next; return x })
	// DueTicks already advances the stored clock; copy only the remaining schedule.
	c.Target.Status.EachEffective(func(x domain.Active) {
		if x.Def.ID == a.Def.ID && x.Source == a.Source {
			a = x
		}
	})
	copy := synergyScaleActive(a, int64(c.RollValue("copy_bp", 5000)))
	copy.Def.SynergyDOTCopy = true
	copy.Def.SynergyDOTVersion = 1
	copy.Def.LegendaryModifiers.Applied = true
	count := 0
	for _, target := range c.Enemies(c.Target.Pos, 180, 0) {
		if target.ID == c.Target.ID {
			continue
		}
		if count >= 6 {
			break
		}
		incoming := copy
		exists := false
		var old domain.Active
		if target.Status != nil {
			target.Status.EachEffective(func(x domain.Active) {
				if x.Source == a.Source && x.Def.ID == a.Def.ID {
					old = x
					exists = true
				}
			})
		}
		if exists {
			incoming.ExpireAt = max(incoming.ExpireAt, old.ExpireAt)
			if synergyTickPower(old, c.Scene.tick, target.MaxHP) >= synergyTickPower(incoming, c.Scene.tick, target.MaxHP) {
				incoming.Def = old.Def
			}
			incoming.NextTickAt = old.NextTickAt
		}
		if c.Scene.applyStatusDef(target, incoming.Def, a.Source) {
			target.Status.RestoreTransferredStatusBalance(a.Source, incoming)
			c.Scene.pushStatusSnapshots(target)
			count++
		}
	}
}
func synergyCashout(c *legendaryContext) {
	if !synergyNativeHit(c) || c.Target.Status == nil || !synergyOnce(c, "cash-root") {
		return
	}
	var originals []domain.Active
	c.Target.Status.EachEffective(func(a domain.Active) {
		if a.Source == c.Owner.ID && synergyQualifyingDOT(a.Def) && !a.Def.SynergyDOTCopy && !a.Def.LegendaryPoisonCopy {
			originals = append(originals, a)
		}
	})
	if len(originals) == 0 || !c.Ready(1000) {
		return
	}
	type payload struct {
		amount   int32
		physical bool
		skill    int32
		hurt     int32
	}
	var due []payload
	for _, a := range originals {
		var consumed int64
		c.Target.Status.TransformSourceStatus(a.Def.ID, c.Owner.ID, func(current domain.Active) domain.Active {
			before := synergyFrozenBalance(current, c.Scene.tick, c.Target.MaxHP)
			after := synergyScaleActive(current, 10000-int64(c.RollValue("consume_bp", 3500)))
			consumed = max(int64(0), before-synergyFrozenBalance(after, c.Scene.tick, c.Target.MaxHP))
			return after
		})
		if consumed > 0 {
			due = append(due, payload{domain.SaturateInt32(consumed), a.Def.SynergyDOTPhysical, a.Def.SourceSkillID, c.Scene.legendaryPeriodicBlow(a).HurtType})
		}
	}
	c.Scene.pushStatusSnapshots(c.Target)
	for _, p := range due {
		for _, target := range c.Enemies(c.Target.Pos, 160, 0) {
			// This is one terminal settlement, not an applied DOT or another skill cast.
			c.Scene.tickStatus(target, domain.Active{Source: c.Owner.ID, Def: domain.StatusDef{ID: legendaryMeleeStatusID(c, fmt.Sprintf("cash-%t", p.physical)), LegendaryPowerID: c.Power.ID, SynergyDOTVersion: 1, SynergyDOTCopy: true, SynergyDOTNew: true, SynergyDOTPhysical: p.physical, SourceSkillID: p.skill, SynergyDOTHurtType: p.hurt, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -p.amount, Mode: domain.ModeAbsolute}}}})
		}
	}
}

func synergySelectivePower(id string) bool {
	switch id {
	case "TY-D05", "TY-D06", "TY-D07", "TY-D08", "TY-D10", "TY-D17", "TY-D19":
		return true
	}
	return false
}
func (s *Scene) synergyPoolReduction(src, dst *entity.Entity, blow combat.Blow, hit event.DamageDealt) int32 {
	if src == nil || dst == nil || src.ID == dst.ID || sameParty(src, dst) {
		return 0
	}
	var bp int64
	for _, c := range s.legendaryIncomingContexts(src, dst, blow, hit) {
		if blow.Origin == domain.DamageDOTOrigin && !synergySelectivePower(c.Power.ID) {
			continue
		}
		// D07 remains direct-only, while D08 applies to magic ticks as before.
		if c.Power.ID == "TY-D07" && blow.Origin == domain.DamageDOTOrigin {
			continue
		}
		c.Blow = blow
		c.Hit = hit
		if h := legendaryHandlers[c.Power.ID].ModifyIncoming; h != nil {
			bp += 10000 - int64(h(&c, 10000))
		}
	}
	return int32(min(int64(7000), max(int64(0), bp)))
}

func synergyTickPower(a domain.Active, now domain.Tick, maxHP int32) int64 {
	a.ExpireAt = max(now, a.NextTickAt)
	if a.ExpireAt <= now {
		a.ExpireAt = now + 1
	}
	return synergyFrozenBalance(a, now, maxHP)
}
