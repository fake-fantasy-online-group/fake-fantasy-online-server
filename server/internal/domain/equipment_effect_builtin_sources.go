package domain

// These producers predate the registry. Keep their causal tokens stable so
// persisted status ancestry remains valid across the unified-table migration.
func init() {
	for _, row := range []struct {
		id, trigger, text string
		source            EquipmentEffectSourceKind
	}{
		{"native:life-leech", "hit", "命中恢复生命", EffectSourceNative},
		{"native:mana-leech", "hit", "命中恢复法力", EffectSourceNative},
		{"set:dragon-stats", "modifier", "套装属性修正", EffectSourceSet},
		{"set:missing-health", "modifier", "套装属性修正", EffectSourceSet},
		{"set:melee-mitigation", "modifier", "套装属性修正", EffectSourceSet},
		{"set:unruly-release", "modifier", "套装属性修正", EffectSourceSet},
		{"set:element-window", "modifier", "套装属性修正", EffectSourceSet},
		{"set:card-bonuses", "modifier", "套装属性修正", EffectSourceSet},
		{"set:poison-snapshot", "status", "套装毒伤快照", EffectSourceSet},
		{"set:blood-cost", "cost", "套装技能生命消耗", EffectSourceSet},
		{"set:party-aura", "aura", "套装队伍光环", EffectSourceSet},
		{"set:periodic-mana", "periodic", "套装周期恢复法力", EffectSourceSet},
		{"set:dragon-steal", "hit", "套装伤害恢复生命", EffectSourceSet},
		{"set:poison-burst", "poison", "套装毒伤爆发", EffectSourceSet},
		{"set:healing-copy", "heal", "套装治疗复制", EffectSourceSet},
		{"set:opposite-spell", "cast", "套装追加异系法术", EffectSourceSet},
	} {
		RegisterEquipmentEffectDefinition(EquipmentEffectDefinition{row.id, row.source, row.trigger, row.text})
	}
}
func RuntimeBuiltinEquipmentEffect(id string, producer bool) RuntimeEquipmentEffect {
	d, _ := EquipmentEffectDefinitionByID(id)
	return RuntimeEquipmentEffect{DefinitionID: d.ID, ID: id, SourceKind: d.SourceKind, Trigger: d.Trigger, Description: d.Description, Producer: producer}
}
