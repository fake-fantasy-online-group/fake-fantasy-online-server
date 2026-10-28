package domain

import (
	"strconv"
	"strings"
)

// SpiritSoulEffects adds runtime equipment effects to the legacy public effects.
// Its result is a projection, never written back to the character's base CSV.
func SpiritSoulEffects(base string, worn *EquipSet) string {
	names := []string{}
	seen := map[string]bool{}
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	for _, name := range strings.Split(base, ",") {
		add(name)
	}
	if worn != nil {
		worn.Each(func(slot EquipSlot, st Stack) {
			ap, ok := SpiritAppearanceForItem(st.SpiritAppearance)
			def, defined := SpiritDefinitionForKind(ap.Kind)
			if !ok || !defined || !def.AcceptsSlot(slot) || ap.EffectID <= 0 {
				return
			}
			name := "TFX" + strconv.Itoa(int(ap.EffectID))
			// These original PKG assets exist only with the _20 suffix.
			if ap.EffectID >= 8055 && ap.EffectID <= 8063 {
				name += "_20"
			}
			add(name)
		})
	}
	return strings.Join(names, ",")
}
