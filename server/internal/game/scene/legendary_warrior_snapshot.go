package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// SnapshotNativeDamage consumes one accepted attack/critical roll even for an
// empty cast. Initial quake impacts and its field share this source-only value;
// every actual target subsequently supplies its own defenses and hit result.
func (c *legendaryContext) SnapshotNativeDamage() int32 {
	if c.Preview {
		return 0
	}
	s := c.Scene
	rt := s.legendaryState()
	if snap := rt.nativeDamageSnapshots[c.RootID]; snap != nil {
		return snap.Amount
	}
	if c.Owner == nil || c.RootID == 0 {
		return 0
	}
	neutral := &entity.Entity{ID: 0, Kind: domain.KindMonster, HP: 2147483647, MaxHP: 2147483647, Pos: c.Owner.Pos, Monster: &entity.Monster{}}
	var effect domain.SkillEffect
	found := false
	for _, e := range c.Def.DirectEffects() {
		if e.Kind == domain.EffectDamage {
			effect = e
			found = true
			break
		}
	}
	if !found {
		return 0
	}
	blow := combat.Blow{RootID: c.RootID, SkillID: int32(c.Skill), SkillPct: effect.DamagePct, SkillPctNumerator: effect.DamagePctNumerator, SkillPctDenominator: effect.DamagePctDenominator, BaseDamageFlat: effect.BaseDamageFlat, FlatDamage: effect.DamageFlat, HurtType: c.Def.HurtType, Area: c.Def.Area.Valid() || c.Def.Radius > 0, GuaranteedHit: true, GuaranteedCrit: effect.GuaranteedCrit, SourceOffense: c.SourceSnapshot}
	if c.SourceSnapshot != nil {
		blow.AttackStatsSnapshot = true
		blow.AttackStats = c.SourceSnapshot.Stats
		blow.EquipmentSnapshot = true
		blow.EquipmentOffense = c.SourceSnapshot.Equipment
	}
	blow.Magic = c.Def.HurtType != domain.HurtPhysical && c.Def.HurtType != domain.HurtNone
	blow = s.equipmentBlow(c.Owner, blow)
	s.legendaryModifyBlow(c.Owner, neutral, &blow)
	// No live target condition may turn a cast-wide source snapshot into an
	// implicit full-health/elite-target bonus.
	ev := combat.CalculateStrike(c.Owner, neutral, &combat.HitTracker{}, blow, s.rng)
	n := domain.SaturateInt32(int64(ev.Amount) + int64(effect.DamageFlat) + int64(s.sourcePetExtraV5(blow.SourceOffense, blow.CurrentSourceOffense, blow.Magic)))
	if blow.SourceOffense.StatusPresent || blow.CurrentSourceOffense != nil && blow.CurrentSourceOffense.StatusPresent {
		n = max(int32(1), domain.SaturateInt32(int64(n)+int64(sourceStatusFlatV5(blow.SourceOffense, blow.CurrentSourceOffense, blow.Magic))))
	}
	source, current := warriorSourceOnlyTraits(blow.SourceOffense), warriorSourceOnlyTraits(blow.CurrentSourceOffense)
	x := sourceUniqueEffectsV5(source, current, uniqueStrikeContext(c.Owner, neutral, blow, ev, neutral.HP))
	suit := sourceSuitBonuses(blow.SourceOffense, blow.CurrentSourceOffense)
	var suitDamage int32
	if !blow.Magic {
		suitDamage = suit.PhysicalSkillBP
	}
	element := domain.HurtElementIndex(blow.HurtType)
	var offense domain.ElementCombatStats
	if element >= 0 {
		offense = blow.EquipmentOffense.Elements[element]
		offense.DamageBP += suit.DamageBP[element]
	}
	buckets := domain.CombatDamageBuckets{DamageIncreaseBP: int64(x.DamageBP) + int64(suitDamage), PVPScaledDamageIncreaseBP: int64(offense.DamageBP), ScaledDamageIncreaseNumerator: s.equipmentAuraDamageNumerator(c.Owner, element, false), PayloadScaleBP: blow.PayloadScaleBP, PayloadScaleKnown: blow.PayloadScaleKnown}
	n = buckets.OutgoingDamage(n)
	n = s.casterSetOutgoingDamage(c.Owner, blow.HurtType, n)
	n = s.classSetMissingHealthDamage(c.Owner, n)
	if rt.nativeDamageSnapshots == nil {
		rt.nativeDamageSnapshots = map[uint64]*combat.NativeDamageSnapshot{}
	}
	rt.nativeDamageSnapshots[c.RootID] = &combat.NativeDamageSnapshot{Owner: c.Owner.ID, Amount: n, Critical: ev.Flag.Has(event.DamageCrit), SourceDamageIncreaseBP: int64(x.DamageBP) + int64(suitDamage) + int64(offense.DamageBP) + buckets.ScaledDamageIncreaseNumerator/10000}
	return n
}
func warriorSourceOnlyTraits(source *domain.CombatSourceSnapshotV5) *domain.CombatSourceSnapshotV5 {
	if source == nil {
		return nil
	}
	copy := *source
	copy.Traits = nil
	for _, t := range source.Traits {
		switch t.Kind {
		case "execution", "ambush", "elite_hunter":
			continue
		}
		copy.Traits = append(copy.Traits, t)
	}
	return &copy
}
