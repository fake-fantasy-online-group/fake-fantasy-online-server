package domain

const (
	SkillNianliCreationI   SkillID = 11008
	SkillNianliCreationII  SkillID = 11009
	SkillNianliCreationIII SkillID = 11010
)

func IsNianliCreationSkill(id SkillID) bool {
	return id >= SkillNianliCreationI && id <= SkillNianliCreationIII
}

type NianliCreationMaterial struct {
	Item ItemID
	Qty  int32
}

// NianliCreationRule 是念力造物的服务端结果配置。客户端只会上报技能号；
// 产物、念力与材料均由 PostgreSQL 决定。
type NianliCreationRule struct {
	Skill      SkillID
	Product    ItemID
	ProductQty int32
	NianliCost int32
	Materials  []NianliCreationMaterial
}

type NianliCreationTable map[SkillID]NianliCreationRule
