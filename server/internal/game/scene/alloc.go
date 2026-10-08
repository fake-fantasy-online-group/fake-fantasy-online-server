package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// onAllocPoints 加点。
//
// 客户端发的是**六维增量**(EquipDlg.pend[] 累加数组), 不是绝对值。验证:
// ① 六项增量都 >= 0(不能洗点) ② 消耗 = 六项增量之和, 且 <= FreePoints。
//
// wire[5](d7[5])业务含义未证明, 这里**不拒也不消费** —— 理由见下面的注释。
func (s *Scene) onAllocPoints(cmd AllocPoints) {
	p, ok := s.players[cmd.ID]
	if !ok || !p.Alive() {
		return
	}
	reject := func(r event.RejectReason) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "AllocPoints", Reason: r})
	}

	ch := p.Player.Char
	old := ch.Base

	// 验证 ① 各维增量不能为负(不能洗点)
	if cmd.STR < 0 || cmd.VIT < 0 || cmd.INT < 0 ||
		cmd.SPI < 0 || cmd.AGI < 0 || cmd.DEX < 0 {
		reject(event.RejectInvalid)
		return
	}

	// wire[5] = d7[5]: 阶段 1 审计未证明业务含义(positional)。
	// 2026-08-14 真机实测发现客户端确实会写非零值(实测 93)——
	// 原"从不写"结论与实际不符,已记入定义与实测不符.md。
	// 当前暂时忽略该字段(不拒也不消费),后续闭合后再决定是否校验。
	_ = cmd.Slot5

	// 消耗 = 六项增量之和
	spent := cmd.STR + cmd.VIT + cmd.INT + cmd.SPI + cmd.AGI + cmd.DEX
	if spent == 0 {
		return // 空加点, 无需处理
	}

	// 验证 ③ 消耗 <= FreePoints
	if spent > ch.FreePoints {
		reject(event.RejectNotEnough)
		return
	}

	// 应用变更(旧值 + 增量)
	ch.Base = domain.Base{
		STR: old.STR + cmd.STR, VIT: old.VIT + cmd.VIT, INT: old.INT + cmd.INT,
		SPI: old.SPI + cmd.SPI, AGI: old.AGI + cmd.AGI, DEX: old.DEX + cmd.DEX,
	}
	ch.FreePoints -= spent
	p.Player.MarkDirty()

	// 体质/精神变了 → 血量/法力上限变 → 重算属性并补满(仿原服: 加点后立即补满)
	s.refreshStats(p)
	p.HP, p.MP = p.MaxHP, p.MaxMP
	s.emitTo(p.ID, s.attributeSnapshot(p))
	// 加点可能满足装备门槛，刷新服务端生成的逐行需求颜色。
	s.pushInventory(p)
	s.refreshChangeSetTooltips(p)

	s.log.Debug("加点", "char", ch.Name, "消耗", spent, "剩余", ch.FreePoints,
		"新六维", ch.Base)
}
