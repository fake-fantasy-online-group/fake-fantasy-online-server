package domain

import "strings"

// NPCRole 是客户端 NpcChatter 认识的环境闲聊分组名。
type NPCRole string

const (
	NPCRoleCommon    NPCRole = "common"
	NPCRoleVendor    NPCRole = "vendor"
	NPCRoleTransport NPCRole = "transport"
	NPCRoleStorage   NPCRole = "storage"
	NPCRoleQuest     NPCRole = "quest"
	NPCRoleGuide     NPCRole = "guide"
	NPCRoleHeal      NPCRole = "heal"
)

// RoleOfNPC 按 NPC 的真实功能字段优先、名字次之确定闲聊类别。
func RoleOfNPC(name string, sell, trans int32) NPCRole {
	switch {
	case sell > 0:
		return NPCRoleVendor
	case trans > 0 || containsAny(name, "传送", "飞空艇"):
		return NPCRoleTransport
	case strings.Contains(name, "仓库"):
		return NPCRoleStorage
	case containsAny(name, "任务", "委托", "传令"):
		return NPCRoleQuest
	case containsAny(name, "指引", "向导", "导师", "教头", "新手"):
		return NPCRoleGuide
	case containsAny(name, "医生", "医师", "药师", "治疗"):
		return NPCRoleHeal
	default:
		return NPCRoleCommon
	}
}

// DefaultNPCGreeting 是没有原版无条件 Hints 时的稳定兜底，不从全服文本池抽签。
func DefaultNPCGreeting(role NPCRole) string {
	switch role {
	case NPCRoleVendor:
		return "看看需要些什么吧。"
	case NPCRoleTransport:
		return "想去别的地方吗？我可以帮你。"
	case NPCRoleStorage:
		return "需要存取物品吗？"
	case NPCRoleQuest:
		return "我这里有些事情需要你帮忙。"
	case NPCRoleGuide:
		return "有什么不明白的地方，可以问我。"
	case NPCRoleHeal:
		return "需要治疗吗？"
	default:
		return "有什么可以帮你的吗？"
	}
}

func containsAny(s string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(s, value) {
			return true
		}
	}
	return false
}
