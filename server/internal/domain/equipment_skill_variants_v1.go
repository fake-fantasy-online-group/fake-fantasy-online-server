package domain

// EquipmentSkillVariantsV1 gates both execution and the optional wire metadata.
const EquipmentSkillVariantsV1 = "endgame.skill-variants.v1"

type EquipmentSkillVariant struct {
	EquipmentRevision uint64  `json:"-"`
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	SourceUID         int64   `json:"sourceUID,string"`
	Item              ItemID  `json:"item"`
	Description       string  `json:"description"`
	MaxTargets        int32   `json:"maxTargets"`
	SkillID           SkillID `json:"-"`
	Profession        int32   `json:"-"`
}

// IDs define complete reviewed finite actions, never client-authored formulas.
func EquipmentSkillVariantByID(id string) (EquipmentSkillVariant, bool) {
	v := EquipmentSkillVariant{ID: id}
	switch id {
	case "sword_gale_fan":
		v.Name, v.SkillID, v.Profession, v.MaxTargets = "拂岳", 10102, 2, 4
		v.Description = "罡气剑改为90度、半径100的近身扇面，最多4敌；首目标全额伤害与震退，其余60%伤害且不震退。"
	case "warrior_piercing_lance":
		v.Name, v.SkillID, v.Profession, v.MaxTargets = "贯阵", 10017, 1, 4
		v.Description = "飞翔之枪改为向前240距离、48宽的贯穿枪线，最多4敌，每敌80%伤害。"
	case "assassin_bomb_fuse":
		v.Name, v.SkillID, v.Profession, v.MaxTargets = "藏雷引线", 10223, 3, 1
		v.Description = "雷火弹有效命中后，引爆落点160距离内自己最近一枚爆弹；爆炸半径至少160，在原位置结算，最多6敌，不连锁引爆。"
	case "healer_guarded_mend":
		v.Name, v.SkillID, v.Profession, v.MaxTargets = "护生签", 10301, 4, 1
		v.Description = "治愈术按实际缺血确定治疗预算，即时恢复70%，1秒后同一目标恢复50%；每目标仅一份，满血不存量，死亡或换装取消后段。"
	case "warlock_flash_fire":
		v.Name, v.SkillID, v.Profession, v.MaxTargets = "掠火", 10401, 5, 1
		v.Description = "火弹术无需吟唱，保留实际法力消耗；冷却至少1.2秒，可快发后走位，换装不刷新剩余冷却。"
	default:
		return EquipmentSkillVariant{}, false
	}
	return v, true
}
