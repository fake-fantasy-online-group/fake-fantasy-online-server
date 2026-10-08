package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 击杀结算: 经验与升级。
//
// 数值口径见 docs/怪物数值-定案.md —— **经验跟血量走不跟等级走**
// (经验 ≈ 0.052 × 血量^1.30, R²=0.956; 二元回归里血量偏系数 0.926、等级只有 0.633)。
// 不过我们不用那个拟合式, 因为粉粉兔给了 2174 只怪的**全量真值**, 直接用真值即可。
// 那个公式是留给"以后要配新怪"的时候用的。
//
// ⚠️ **没有等级差惩罚。** 大等级差打低级怪照样给满经验 —— 不是我们觉得该这样,
// 是**客户端数据里没有任何等级差修正的痕迹**, 而凭空发明一条会直接改变练级路线。
// 要加的话那是个平衡决策, 得单独定, 不能混在这里悄悄塞进去。

// awardKill 把击杀回报结给凶手。
//
// 只结给**最后一击的人**。原作有"怪物所有权 8 秒"的说法(单源, 见 docs/战斗公式.md),
// 但那说的是抢怪判定, 不是经验分配; 组队分经验也还没做。
// 现在这样简单且不会错 —— 等组队接进来时这里会变成"结给仇恨列表"。
func (s *Scene) awardKill(dead *entity.Entity, killer domain.EntityID) {
	if dead.Monster == nil || killer == 0 {
		return // 玩家死了不给谁经验; PVP 的回报是另一回事
	}
	// 宠物补的最后一刀算主人的。**不做这个转换的话**, 凶手 id 是宠物的实体 id,
	// 在 players 里查不到, 于是这一只怪的经验就凭空蒸发了 ——
	// 而且越是靠宠物打的玩家越吃亏, 正好跟宠物系统的意图相反。
	if pet, ok := s.pets[killer]; ok && pet.Pet != nil {
		killer = pet.Pet.Owner
	}
	p, ok := s.players[killer]
	if !ok || p.Player == nil {
		return // 凶手已经走了
	}
	exp := s.monsterExp(dead)
	if exp <= 0 {
		return
	}
	// 组队优先: 分掉了就不再给击杀者单独结算一遍。组队内每个成员按
	// **自己的等级**各自应用跨级惩罚 —— 大号带小号打低级怪, 大号被惩罚,
	// 小号拿满, 不会出现"全队共用击杀者倍率"的偏差。
	if s.splitKillExp(p, exp, dead.Monster.TypeID) {
		return
	}
	exp = s.expAfterPenalty(exp, p.Player.Char.Level, dead.Monster.TypeID)
	s.grantExp(p, exp)
	// 宠物那份是**额外的**, 不从主人这份里扣(见 domain.SharedExp)
	s.grantPetExp(p, exp)
}

// expAfterPenalty 按玩家等级对一次击杀经验应用跨级等级差惩罚。
//
// 倍率取的是这只怪的模板等级, 不是实体上的值 —— 与 monsterExp 同源。
// 惩罚未启用(零值配置)或拿不到模板时原样返回。
func (s *Scene) expAfterPenalty(exp int64, playerLevel int32, monster domain.MonsterID) int64 {
	if !s.penalty.Enabled() {
		return exp
	}
	d, ok := s.defs.Def(monster)
	if !ok {
		return exp
	}
	mult := s.penalty.ExpMultBP(playerLevel, d.Level)
	if mult >= 10000 {
		return exp
	}
	return exp * int64(mult) / 10000
}

// monsterExp 取这只怪的经验值。
//
// 实体上没存经验(那是模板的属性, 不是这一只的), 所以回查配置。
func (s *Scene) monsterExp(m *entity.Entity) int64 {
	if s.defs == nil || m.Monster == nil {
		return 0
	}
	d, ok := s.defs.Def(m.Monster.TypeID)
	if !ok {
		return 0
	}
	return d.Exp
}

// grantExp 加经验, 处理连升, 把该发的事件都发出去。
//
// 下行顺序有真实流量约束: **权威属性 0x8007 → EXP 飘字 0x800c → 文本 0x8008**。
// 后两包只做表现，不修改客户端角色状态；因此哪怕没升级，也必须先推属性快照。
func (s *Scene) grantExp(p *entity.Entity, exp int64) {
	s.grantExpWithBonus(p, exp, true)
}

// grantExpExact 给固定经验道具和 GM/运维入口使用：仍走与正常经验相同的
// 升级、属性刷新和存档链路，但不叠加经验倍率；到顶仍沿用溢出丢弃规则。
func (s *Scene) grantExpExact(p *entity.Entity, exp int64) {
	s.grantExpWithBonus(p, exp, false)
}

func (s *Scene) grantExpWithBonus(p *entity.Entity, exp int64, applyBonus bool) {
	bonus := p.Status.ExpBonusPct()
	if itemBonus := p.Player.ExperienceBonusPct(s.tick); itemBonus > bonus {
		bonus = itemBonus
	}
	if applyBonus && bonus != 0 {
		exp += exp * int64(bonus) / 100
	}
	if applyBonus && exp > 0 {
		exp += s.petExperienceBonus(p, exp)
	}
	ch := p.Player.Char
	res := ch.AddExp(s.levels, exp, domain.LevelCap)
	p.Player.MarkDirty()

	if res.Levels > 0 {
		// 升级: 先把成长加到六维上, 再重推二级属性
		ch.ApplyLevelUps(res.FromLevel, res.ToLevel)
		p.Level = ch.Level

		// 升级也走完整装备/套装管线，不能把已激活加成覆盖成裸装属性。
		// refreshStats 只补上限变化量，保持原有“不因升级回满”的口径。
		s.refreshStats(p)
	}

	// 真实击杀顺序：先权威状态，再表现包。
	s.emitTo(p.ID, s.attributeSnapshot(p))
	s.emitTo(p.ID, event.ExpGained{Who: p.ID, Delta: exp, Total: ch.Exp})

	if res.Levels == 0 {
		return
	}

	// 升级事实仍广播给周围人；目前没有证实的独立升级特效包，协议层不会猜。
	s.emit(event.LevelUp{Who: p.ID, Level: ch.Level})
	// 0x8007 不含技能点；升级获得技能点后必须刷新 0x8015，否则技能面板会一直
	// 保留升级前的余额，直到玩家重登或主动升级一次技能。
	s.emitTo(p.ID, s.skillSnapshot(p))
	// 等级和成长六维同时影响装备提示的需求颜色。
	s.pushInventory(p)
	s.refreshChangeSetTooltips(p)

	s.log.Info("升级", "char", ch.Name, "从", res.FromLevel, "到", res.ToLevel,
		"血上限", p.MaxHP, "自由点", ch.FreePoints, "技能点", ch.SkillPoints)
	if res.Overflowed {
		s.log.Info("已达等级上限, 溢出经验丢弃", "char", ch.Name, "上限", domain.LevelCap)
	}
}
