package domain

// ElementCombatStats retains raw over-cap resistance until attacker-specific
// penetration is subtracted. All channels are separate from legacy flat resist.
type ElementCombatStats struct{ DamageBP, ResistBP, PierceBP, MitigationBP, AbsorbBP int32 }
type EquipmentCombatStats struct {
	Elements             [4]ElementCombatStats
	PhysicalMitigationBP int32
}

func AggregateEquipmentCombat(effects []ActiveEquipmentEffect) EquipmentCombatStats {
	var out EquipmentCombatStats
	for _, a := range effects {
		e := a.Effect
		i := ElementIndex(e.Element)
		if i < 0 {
			continue
		}
		x := &out.Elements[i]
		add := func(dst *int32) { *dst = SaturateInt32(int64(*dst) + int64(e.Value)) }
		switch e.Kind {
		case "element_damage_bp":
			add(&x.DamageBP)
		case "element_resist_bp":
			add(&x.ResistBP)
		case "enemy_element_pierce_bp":
			add(&x.PierceBP)
		case "element_mitigation_bp":
			add(&x.MitigationBP)
		case "element_absorb_bp":
			add(&x.AbsorbBP)
		}
	}
	return out
}
func ElementOutgoingDamage(amount, existingBP, elementBP int32, pvp bool) int32 {
	if pvp {
		elementBP /= 2
	}
	bp := min(int64(10000), max(int64(0), int64(existingBP)+int64(elementBP)))
	return max(int32(1), ApplyUniqueBP(amount, int32(bp)))
}
func ElementResistanceDamage(amount, rawResist, pierce int32, pvp bool) int32 {
	cap := int32(3000)
	if pvp {
		pierce /= 2
		cap = 1500
	}
	pierce = min(cap, max(int32(0), pierce))
	r := int32(max(int64(-5000), min(int64(7500), int64(rawResist)-int64(pierce))))
	return max(int32(1), ApplyUniqueBP(amount, -r))
}
func ElementMitigationDamage(amount, existingBP, elementBP int32) int32 {
	bp := int32(max(int64(0), min(int64(4500), int64(existingBP)+int64(elementBP))))
	return max(int32(1), ApplyUniqueBP(amount, -bp))
}

type ElementAbsorbResult struct{ Damage, Absorbed, Heal, HP int32 }

// ResolveElementAbsorb computes a single final HP transaction. Lethal damage
// never heals; shield-zero and 1-damage packets cannot manufacture restoration.
func ResolveElementAbsorb(hp, maxHP, damage, absorbBP, healQuota int32, pvp bool) ElementAbsorbResult {
	if damage <= 0 || hp <= 0 {
		return ElementAbsorbResult{HP: hp}
	}
	cap := int32(2000)
	if pvp {
		cap = 1000
	}
	absorbBP = min(cap, max(int32(0), absorbBP))
	absorbed := int32(int64(damage) * int64(absorbBP) / 10000)
	dealt := damage - absorbed
	remaining := max(int64(0), int64(hp)-int64(dealt))
	heal := int32(0)
	if remaining > 0 {
		heal = int32(max(int64(0), min(int64(absorbed), min(int64(maxHP)-remaining, int64(healQuota)))))
	}
	return ElementAbsorbResult{dealt, absorbed, heal, int32(remaining + int64(heal))}
}

// DamageOrigin zero means a normal accepted action for legacy callers.
type DamageOrigin string

const (
	DamageNormalOrigin      DamageOrigin = ""
	DamageProcOrigin        DamageOrigin = "proc"
	DamageAuraOrigin        DamageOrigin = "aura"
	DamageReflectionOrigin  DamageOrigin = "reflection"
	DamageDOTOrigin         DamageOrigin = "dot"
	DamageEnvironmentOrigin DamageOrigin = "environment"
	DamageCounterOrigin     DamageOrigin = "counter"
)

// EquipmentAbsorbRestore is a short-lived restoration receipt carried across scene transfer.
type EquipmentAbsorbRestore struct {
	AtMS   int64
	Amount int32
}
