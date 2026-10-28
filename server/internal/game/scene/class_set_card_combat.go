package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// Read the activated native card rows, not a second table of hard-coded bonuses.
func (s *Scene) classSetCardCombatPayload(p *entity.Entity) (out domain.SuitCombatBonuses) {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return
	}
	worn := s.functionalWorn(p)
	seen := map[int32]bool{}
	worn.Each(func(_ domain.EquipSlot, st domain.Stack) {
		def, ok := s.itemDef(st.Item)
		if !ok || def.Equip == nil || def.Equip.Suit == nil {
			return
		}
		suit := def.Equip.Suit
		if seen[suit.ID] {
			return
		}
		seen[suit.ID] = true
		pieces := s.classSetPiecesInWorn(p, worn, suit.EnhancedClass)
		for _, b := range suit.Bonuses {
			if b.Pieces > pieces || b.DisabledReason != "" || b.Probability != 100 || b.Affix.Mode != domain.ModeAbsolute {
				continue
			}
			a := b.Affix
			add := func(dst *int32) { *dst = clampI32(int64(*dst) + int64(a.Value)) }
			switch a.Attr {
			case domain.AttrSuitPhysicalSkillBP:
				add(&out.PhysicalSkillBP)
			case domain.AttrSuitTrapDamageBP:
				add(&out.TrapDamageBP)
			case domain.AttrSuitPhysicalMitigationBP:
				add(&out.PhysicalMitigationBP)
			case domain.AttrSuitPhysicalCritDamageBP:
				add(&out.PhysicalCritDamageBP)
			case domain.AttrSuitHealingBP:
				add(&out.HealingBP)
			default:
				if a.Attr >= domain.AttrSuitFireDamageBP && a.Attr <= domain.AttrSuitDarkDamageBP {
					add(&out.DamageBP[a.Attr-domain.AttrSuitFireDamageBP])
				}
				if a.Attr >= domain.AttrSuitFireMitigationBP && a.Attr <= domain.AttrSuitDarkMitigationBP {
					add(&out.MitigationBP[a.Attr-domain.AttrSuitFireMitigationBP])
				}
			}
		}
	})
	return
}
func sourceSuitBonuses(source, current *domain.CombatSourceSnapshotV5) domain.SuitCombatBonuses {
	if source == nil {
		return domain.SuitCombatBonuses{}
	}
	out := source.SuitBonuses
	if current != nil {
		c := current.SuitBonuses
		out.PhysicalSkillBP = min(out.PhysicalSkillBP, c.PhysicalSkillBP)
		out.TrapDamageBP = min(out.TrapDamageBP, c.TrapDamageBP)
		out.PhysicalCritDamageBP = min(out.PhysicalCritDamageBP, c.PhysicalCritDamageBP)
		out.HealingBP = min(out.HealingBP, c.HealingBP)
		for i := range out.DamageBP {
			out.DamageBP[i] = min(out.DamageBP[i], c.DamageBP[i])
		}
	}
	return out
}
func (s *Scene) classSetCardHealing(src *entity.Entity, amount int32) int32 {
	if amount <= 0 || src == nil {
		return amount
	}
	source, current := s.combatSourceV5(src)
	bp := sourceSuitBonuses(source, current).HealingBP
	return clampI32(int64(amount) * (10000 + int64(bp)) / 10000)
}

// Only a status explicitly emitted by a trap gets the snapshotted trap bonus.
// Preserve sub-HP fractions with the existing periodic accumulator, not a new
// multiplier at every tick (which would also accidentally scale unrelated poison).
func scaleClassSetTrapStatus(d *domain.StatusDef, bp int32) {
	if d == nil || bp <= 0 || d.IntervalSec <= 0 || d.SuitTrapDamageBP != 0 {
		return
	}
	d.SuitTrapDamageBP = bp
	d.Affixes = append([]domain.Affix(nil), d.Affixes...)
	d.HPChanges = append([]domain.ResourceChange(nil), d.HPChanges...)
	fraction := d.PeriodicDamageFraction100 * (10000 + int64(bp)) / 10000
	scale := func(value *int32) {
		if *value >= 0 {
			return
		}
		wide := -int64(*value) * (10000 + int64(bp))
		*value = -clampI32(wide / 10000)
		fraction += (wide % 10000) / 100
	}
	for i := range d.Affixes {
		if d.Affixes[i].Attr == domain.AttrHPRegen && d.Affixes[i].Mode == domain.ModeAbsolute {
			scale(&d.Affixes[i].Value)
		}
	}
	for i := range d.HPChanges {
		if d.HPChanges[i].Mode == domain.ModeAbsolute {
			scale(&d.HPChanges[i].Value)
		}
	}
	// Convert whole fractional HP to a separate negative HP change.
	if whole := fraction / 100; whole > 0 {
		d.HPChanges = append(d.HPChanges, domain.ResourceChange{Value: -clampI32(whole)})
	}
	d.PeriodicDamageFraction100 = fraction % 100
}

func (s *Scene) classSetCardCombat(p *entity.Entity) domain.SuitCombatBonuses {
	out := domain.SuitCombatBonuses{}
	s.dispatchEquipmentEffect(p, domain.RuntimeBuiltinEquipmentEffect("set:card-bonuses", false), func() { out = s.classSetCardCombatPayload(p) })
	return out
}
