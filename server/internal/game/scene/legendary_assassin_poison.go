package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func legendaryAssassinPoisonStatus(c *legendaryContext, d *domain.StatusDef) {
	legendaryAssassinAmplifyPoison(d)
}
func legendaryAssassinAmplifyPoison(d *domain.StatusDef) {
	if d == nil || d.ID != domain.StatusPoison || !d.ClassSetPoison || d.LegendaryPoisonAmplified {
		return
	}
	d.LegendaryPoisonAmplified = true
	d.LegendaryModifiers.DamageTickBP += 10000
}
func (s *Scene) legendaryAssassinPreparePoison(source *entity.Entity, d *domain.StatusDef) {
	if legendaryAssassinHas(s, source, "CK-A09") {
		legendaryAssassinAmplifyPoison(d)
	}
}
func legendaryAssassinNativePoisonHit(c *legendaryContext) {
	if (c.Skill != 10205 && c.Skill != 10206) || c.Target == nil || !c.Target.Alive() || c.Hit.Flag.Has(event.DamageMiss) || c.Owner.Status == nil {
		return
	}
	count := 0
	for _, h := range c.Hits {
		if h.src == c.Owner.ID && h.dst == c.Target.ID && h.blow.SkillID == int32(c.Skill) && !h.ev.Flag.Has(event.DamageMiss) {
			count++
		}
	}
	if count > 1 {
		return
	}
	// Promote the native quenching carrier's application chance for this native
	// skill hit only. Never mutate the real buff or change ordinary-attack odds.
	source := *c.Owner
	source.Status = c.Owner.Status.Clone()
	found := false
	c.Owner.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == domain.StatusPoisonHit && a.Def.PoisonOnHitChance > 0 {
			d := a.Def
			d.PoisonOnHitChance = 100
			source.Status.Apply(d, c.Scene.tick, c.Owner.ID)
			found = true
		}
	})
	if found {
		c.Scene.applyCombatTriggeredStatus(&source, c.Target, domain.StatusPoison)
	}
}

func (s *Scene) legendaryAssassinDeath(dead *entity.Entity) {
	if dead == nil || dead.Status == nil {
		return
	}
	// Capture poison before native death cleanup discards it. Killer is irrelevant:
	// this belongs to the original poison owner, not the last damage dealer.
	var poisons []domain.Active
	dead.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == domain.StatusPoison && a.Def.ClassSetPoison && !a.Def.LegendaryPoisonCopy && !a.Def.LegendaryPoisonSpread && a.ExpireAt > s.tick {
			poisons = append(poisons, a)
		}
	})
	for _, poison := range poisons {
		owner := s.entities[poison.Source]
		s.forEachLegendary(owner, func(c *legendaryContext) {
			if c.Power.ID != "CK-A08" {
				return
			}
			if h := legendaryHandlers[c.Power.ID].OwnerPoisonDeath; h != nil && dead.Status.MarkLegendaryPoisonSpread(c.Owner.ID) {
				h(c, poison, dead)
			}
		})
	}
}
func legendaryAssassinPoisonTickPower(c *legendaryContext, a domain.Active, target *entity.Entity) int64 {
	if a.Def.IntervalSec <= 0 {
		return 0
	}
	count := int64((a.ExpireAt-max(a.NextTickAt, c.Scene.tick))/domain.Ticks(int(a.Def.IntervalSec)*1000)) + 1
	if count <= 0 {
		return 0
	}
	return c.Scene.legendaryPeriodicAmount(a, -classSetPoisonRemaining(a, c.Scene.tick, target.MaxHP)/count) * -1
}
func legendaryAssassinCopyPoison(c *legendaryContext, target *entity.Entity, poison domain.Active) {
	if target == nil || poison.Def.LegendaryPoisonCopy || poison.ExpireAt <= c.Scene.tick {
		return
	}
	incoming := poison
	if current, ok := legendaryAssassinNativePoison(c, target); ok {
		// One poison: select potency and remaining clock independently, never add two.
		if legendaryAssassinPoisonTickPower(c, current, target) > legendaryAssassinPoisonTickPower(c, poison, target) {
			incoming = current
		}
		incoming.ExpireAt = max(poison.ExpireAt, current.ExpireAt)
	}
	incoming.Def.LegendaryPoisonCopy = true
	if c.Scene.applyStatusDef(target, incoming.Def, c.Owner.ID) {
		target.Status.RestoreTransferredStatusBalance(c.Owner.ID, incoming)
		c.Scene.pushStatusSnapshots(target)
		c.Visual("stage-01", target) // One flash only after the copy committed.
	}
}

func init() {
	registerLegendarySynergyHandler("CK-A08", legendaryHandler{OwnerPoisonDeath: legendaryAssassinSpreadDeath})
}
func legendaryAssassinSpreadDeath(c *legendaryContext, poison domain.Active, dead *entity.Entity) {
	for _, target := range c.Enemies(dead.Pos, 200, 5) {
		legendaryAssassinCopyPoison(c, target, poison)
	}
}
func legendaryAssassinNativePoison(c *legendaryContext, target *entity.Entity) (domain.Active, bool) {
	var found domain.Active
	ok := false
	if target != nil && target.Status != nil {
		target.Status.EachEffective(func(a domain.Active) {
			if a.Def.ID == domain.StatusPoison && a.Def.ClassSetPoison && a.Source == c.Owner.ID && a.ExpireAt > c.Scene.tick {
				found = a
				ok = true
			}
		})
	}
	return found, ok
}

func init() {
	registerLegendarySynergyHandler("CK-A18", legendaryHandler{StatusApplied: legendaryAssassinFrostGuard, StatusCommitted: legendaryAssassinFrostPoison})
}
func legendaryAssassinFrostGuard(c *legendaryContext, d *domain.StatusDef) {
	if d.ID == domain.StatusFreeze && (d.SourceSkillID == 10216 || d.SkillSnapshotID == 10216) {
		d.LegendaryFreezePoisonOwner = c.Owner.ID
	}
}
func legendaryAssassinFrostPoison(c *legendaryContext, d *domain.StatusDef) {
	if d.ID != domain.StatusFreeze || d.LegendaryFreezePoisonOwner != c.Owner.ID || c.Target == nil || c.Owner.Player == nil || c.Owner.Player.Char.Skills[10211] <= 0 {
		return
	}
	// Both existing and virtual carriers belong to the accepted trap attack.
	// A later learned skill or stronger active enchantment cannot upgrade it.
	raw := c.Scene.rawSkillLevels(c.Owner)
	hard := c.Owner.Player.Char.Skills
	if c.Def.SnapshotRawRanks != nil {
		raw = lesserSkillLevels(c.Def.SnapshotRawRanks, raw)
		hard = lesserSkillLevels(c.Def.SnapshotHardRanks, hard)
	}
	if hard[10211] <= 0 {
		return
	}
	bounded, boundedOK := legendaryAssassinFrostPoisonCarrier(c, hard, raw)
	source := *c.Owner
	source.Status = c.Owner.Status.Clone()
	// Native poison's class-set bonus reads this panel, so use the same lesser
	// accepted/current attack stats as the trap's direct payload.
	source.Stats = c.Scene.equipmentAttackStats(c.Owner)
	found := false
	source.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == domain.StatusPoisonHit {
			carrier := a.Def
			if boundedOK {
				carrier.PoisonOnHitLevel = min(carrier.PoisonOnHitLevel, bounded.PoisonOnHitLevel)
				carrier.PoisonOnHitProgressionRanks = min(carrier.PoisonOnHitProgressionRanks, bounded.PoisonOnHitProgressionRanks)
				carrier.PoisonOnHitFirstLevel = min(carrier.PoisonOnHitFirstLevel, bounded.PoisonOnHitFirstLevel)
				carrier.PoisonOnHitFirstDurationSec = min(carrier.PoisonOnHitFirstDurationSec, bounded.PoisonOnHitFirstDurationSec)
				carrier.PoisonOnHitBaseDurationSec = min(carrier.PoisonOnHitBaseDurationSec, bounded.PoisonOnHitBaseDurationSec)
				carrier.EffectHitBP = min(carrier.EffectHitBP, bounded.EffectHitBP)
			}
			carrier.PoisonOnHitChance = 100
			source.Status.Apply(carrier, c.Scene.tick, c.Owner.ID)
			found = true
		}
	})
	if !found && boundedOK {
		source.Status.Apply(bounded, c.Scene.tick, c.Owner.ID)
		found = true
	}
	if found {
		c.Scene.applyCombatTriggeredStatus(&source, c.Target, domain.StatusPoison)
	}
}

func legendaryAssassinFrostPoisonCarrier(c *legendaryContext, hard, raw domain.Learned) (domain.StatusDef, bool) {
	if c.Scene.skills == nil {
		return domain.StatusDef{}, false
	}
	skill, ok := c.Scene.skills.Get(10211, raw[10211])
	if !ok {
		return domain.StatusDef{}, false
	}
	c.Scene.applyResolvedSkillModifiers(c.Owner, &skill, hard, raw)
	skill.SnapshotRawRanks, skill.SnapshotHardRanks = raw.Clone(), hard.Clone()
	for _, application := range skill.Statuses {
		if carrier, ok := c.Scene.resolveSkillStatusDef(skill, application, 0); ok && carrier.ID == domain.StatusPoisonHit {
			carrier.PoisonOnHitChance = 100
			// A new virtual native borrow is not an old saved buff with unknown
			// accuracy. Existing real carriers retain their stricter own snapshot.
			carrier.SourceEffectHitBP = c.Scene.sourceEffectHitBP(c.Owner)
			carrier.SourceEffectHitKnown = true
			return carrier, true
		}
	}
	return domain.StatusDef{}, false
}
