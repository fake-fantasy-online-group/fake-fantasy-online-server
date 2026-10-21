package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// Use the approved V1 set-aura radius, retaining the equipment-aura cadence. Zero
// MaxPlayers means all eligible party members, not a new five-person limit.
func classSetAura(suit int32) (domain.EquipmentAura, bool) {
	a := domain.EquipmentAura{FixedRank: 1, RadiusPX: 300, RefreshMS: 500, LeaseMS: 1000, MaxPlayers: 0, PVPScaleBP: 10000}
	switch suit {
	case 1:
		a.ID, a.Modifier, a.Value, a.VisualKey = "class_set_1", "class_set_crit_rate_bp", 500, "套装光环·刺客"
	case 2:
		a.ID, a.Modifier, a.Value, a.VisualKey = "class_set_2", "class_set_magic_mitigation_bp", 800, "套装光环·药师"
	case 3:
		a.ID, a.Modifier, a.Value, a.VisualKey = "class_set_3", "class_set_crit_damage_bp", 2000, "套装光环·术士"
	case 4:
		a.ID, a.Modifier, a.Value, a.VisualKey = "class_set_4", "class_set_physical_mitigation_bp", 800, "套装光环·剑客"
	case 5:
		a.ID, a.Modifier, a.Value, a.VisualKey = "class_set_5", "class_set_max_hp_bp", 1500, "套装光环·战士"
	default:
		return domain.EquipmentAura{}, false
	}
	a.Family = a.ID
	return a, true
}
func classSetAuraID(a domain.EquipmentAura) int32 {
	for i := int32(1); i <= 5; i++ {
		expected, _ := classSetAura(i)
		if a == expected {
			return i
		}
	}
	return 0
}
func (s *Scene) classSetAuraSources(p *entity.Entity) []equipmentAuraSource {
	for suit := int32(1); suit <= 5; suit++ {
		if s.classSetPieces(p, suit) >= classSetSecondSkillPieces {
			a, _ := classSetAura(suit)
			return []equipmentAuraSource{{owner: p.ID, uid: -int64(930000 + suit), party: p.Player.Party, effect: a}}
		}
	}
	return nil
}
func (s *Scene) classSetAuraValue(p *entity.Entity, modifier string) int32 {
	if p == nil {
		return 0
	}
	for _, g := range s.equipmentState().auras[p.ID] {
		if g.effect.Modifier == modifier && classSetAuraID(g.effect) != 0 && s.equipmentAuraGrantValid(p, g) {
			return g.effect.Value
		}
	}
	return 0
}

// Stat compilation consumes membership already validated by stepEquipmentAuras.
// It must not recursively resolve source equipment while preparing attributes.
func classSetAuraStatKey(grants map[string]equipmentAuraGrant) (key [2]int32) {
	for _, g := range grants {
		switch g.effect.Modifier {
		case "class_set_max_hp_bp":
			key[0] = g.effect.Value
		case "class_set_crit_rate_bp":
			key[1] = g.effect.Value
		}
	}
	return
}
func (s *Scene) classSetAuraStats(p *entity.Entity, stats domain.Stats) domain.Stats {
	key := classSetAuraStatKey(s.equipmentState().auras[p.ID])
	stats.MaxHP = clampI32(int64(stats.MaxHP) * (10000 + int64(key[0])) / 10000)
	stats.CritRate = clampI32(int64(stats.CritRate) + int64(key[1]))
	stats.MCritRate = clampI32(int64(stats.MCritRate) + int64(key[1]))
	return stats
}
func (s *Scene) classSetAuraIncomingDamage(p *entity.Entity, magic bool, amount int32) int32 {
	key := "class_set_physical_mitigation_bp"
	if magic {
		key = "class_set_magic_mitigation_bp"
	}
	return clampI32(int64(amount) * (10000 - int64(s.classSetAuraValue(p, key))) / 10000)
}
func classSetAuraDescription(a domain.EquipmentAura, source string) string {
	label := map[string]string{"class_set_max_hp_bp": "最大生命 +15%", "class_set_physical_mitigation_bp": "物理伤害减免8%", "class_set_magic_mitigation_bp": "魔法伤害减免8%（包括无元素魔法）", "class_set_crit_rate_bp": "物理/法术暴击率各 +5个百分点", "class_set_crit_damage_bp": "物理/法术暴击伤害各 +20个百分点"}[a.Modifier]
	return fmt.Sprintf("%s套装光环：%s。来源：%s；范围%d，惠及自身与同队玩家，无额外人数上限。同职业不叠加，不同职业互相叠加；来源死亡、脱装、破损、离线或离开同队范围时移除。", classSetAuraDisplayName(a), label, source, a.RadiusPX)
}

// Commands in one mailbox drain can change party, range or worn durability
// before accepting a cast. Never freeze a stale stat aura into that cast.
// Newly entering recipients still use the existing spatial-refresh cadence.
func (s *Scene) syncClassSetAuraStatsForSnapshot(p *entity.Entity) {
	if p == nil || s.equipment == nil {
		return
	}
	for _, g := range s.equipment.auras[p.ID] {
		if (g.effect.Modifier == "class_set_max_hp_bp" || g.effect.Modifier == "class_set_crit_rate_bp") && !s.equipmentAuraGrantValid(p, g) {
			s.stepEquipmentAuras()
			return
		}
	}
}

// One mailbox drain may contain a party/movement change followed by an item
// or combat command. Prune invalid stat memberships before the next consumer;
// do not perform a full source scan when all existing grants are still valid.
func (s *Scene) syncInvalidClassSetAuraStats() {
	if s.equipment == nil {
		return
	}
	for id, grants := range s.equipment.auras {
		for _, g := range grants {
			if (g.effect.Modifier == "class_set_max_hp_bp" || g.effect.Modifier == "class_set_crit_rate_bp") && !s.equipmentAuraGrantValid(s.entities[id], g) {
				s.stepEquipmentAuras()
				return
			}
		}
	}
}

// Display labels are separate from the stable effect key consumed by clients.
func classSetAuraDisplayName(a domain.EquipmentAura) string {
	switch classSetAuraID(a) {
	case 1:
		return "夜色拥抱"
	case 2:
		return "信仰之翼"
	case 3:
		return "哲理之石"
	case 4:
		return "胜利王座"
	case 5:
		return "勇气之刃"
	}
	return a.VisualKey
}
