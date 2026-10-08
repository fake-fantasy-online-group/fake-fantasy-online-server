// Package ai 是怪物行为的**决策核心**：给定一只怪的处境，回答"这一帧该干什么"。
//
// 刻意做成纯函数：不碰实体、不碰场景、不发事件、不掷随机数。
// 执行（真的走一步、真的出手、真的广播）在 scene 那边，本包只出决定。
// 这样一整套追击/脱战逻辑可以用几十个表驱动用例覆盖，不需要起场景。
//
// ⚠️ **本包的参数有一半没有原作数据。**
// 逐条标注在 Params 上。凡是标了「服务端定」的，将来要调就直接调，
// 不用担心违背原作 —— 因为原作那部分我们根本没拿到。
package ai

import (
	"math"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Action 是这一帧的决定。
type Action uint8

const (
	Hold       Action = iota // 原地不动
	Chase                    // 朝目标挪一步
	Strike                   // 够得着且冷却好了, 出手
	GoHome                   // 兼容旧枚举；当前规则不会产生回出生点动作
	DropTarget               // 目标不成立了, 脱战
)

func (a Action) String() string {
	switch a {
	case Chase:
		return "追击"
	case Strike:
		return "出手"
	case GoHome:
		return "回家"
	case DropTarget:
		return "脱战"
	}
	return "待机"
}

// 距离常量。单位是地图世界坐标（客户端像素）。
const (
	// MeleeRange 近战够得着的距离。
	// **[实证]** 取自 ov_arm.atk_dist 的众数: 578 件有值的武器里 412 件是 75。
	// 另外 79 件是 450（远程武器）, 85 件 100, 2 件 200。
	MeleeRange = 75

	// RangedRange 远程攻击距离。**[实证]** 同上, 远程武器一律 450。
	//
	// ⚠️ 目前**怪物一律按近战处理**。法系怪理应用这个值, 但 game_monsters
	// 没有"近战/远程"这一列可查（combat_type 有"法系"但那说的是伤害类型）,
	// 硬套会让所有法系怪隔着半屏放风筝, 影响太大, 等有依据再接。
	RangedRange = 450
)

// Params 是一只怪的行为参数。
type Params struct {
	// ViewDist 索敌半径。
	//
	// ⚠️ **服务端定**。正式值来自 PostgreSQL AI 模板；客户端的 .lst 与粉粉兔
	// 都没有这一项。当前主动模板统一为 400。
	ViewDist float64

	// TraceDist 当前目标离自己超过这么远时放弃追击。
	//
	// ⚠️ **服务端定**。同上，当前主动模板统一为 600。
	// 它只结束当前追击，不会让怪物回出生点；怪会留在脱战位置继续活动。
	TraceDist float64

	// AttackDist 够得着的距离。见 MeleeRange。
	AttackDist float64

	// MoveSpeed 移速（单位/秒）。**[实证]** 来自粉粉兔, 是真数据。
	// **0 表示这只怪天生不会动**（食人花那类）, 全服 263 只 60 级内的怪如此。
	MoveSpeed float64

	// Aggressive 是否主动找人打。
	//
	// 推导值，不是原始数据：精英/BOSS 或等级不低于 15 的怪物视为主动怪。
	// 规则参考公开玩家攻略，实际行为可能因怪物而异。
	Aggressive bool
}

// Target 是当前目标的处境。nil 表示没有目标。
type Target struct {
	Pos   domain.Pos
	Alive bool
}

// Input 是做一次决策要看的全部东西。
type Input struct {
	Params    Params
	Self      domain.Pos
	Home      domain.Pos // 仅保留数据兼容；决策不以出生点为活动中心
	Target    *Target
	CanAttack bool // 攻击冷却好了没
}

// Decide 决定这一帧干什么。纯函数, 同样的输入永远同样的输出。
//
// 判定顺序是有讲究的, 从"最该立刻停手的情况"往下排:
//
//	目标死了      → 立刻脱战, 别对着尸体挥空
//	目标跑太远    → 脱战
//	够得着        → 冷却好了就打, 没好就等（不要一边打一边贴脸挪动）
//	够不着        → 会走就追, 不会走就干瞪眼
func Decide(in Input) Action {
	p := in.Params

	if in.Target == nil {
		// 没目标就待机；场景游走逻辑会从当前位置安排下一段移动。
		return Hold
	}

	if !in.Target.Alive {
		return DropTarget
	}
	// 目标离得太远就结束本次追击，但绝不回出生点重置。
	if dist(in.Self, in.Target.Pos) > p.TraceDist {
		return DropTarget
	}

	if dist(in.Self, in.Target.Pos) <= p.AttackDist {
		if in.CanAttack {
			return Strike
		}
		return Hold // 冷却里就站着, 别抖
	}

	if p.MoveSpeed <= 0 {
		return Hold // 天生不会动的怪, 够不着就只能干瞪眼
	}
	return Chase
}

// CanAcquire 报告这只怪现在能不能去抓一个新目标。
//
// 不主动的怪**不是永远不还手** —— 被打了照样打回来, 那条路径是场景在结算伤害时
// 直接把仇恨挂上去, 不走这里。这个函数只管"主动去找人"。
func CanAcquire(p Params, self, _ domain.Pos, cand domain.Pos) bool {
	if !p.Aggressive {
		return false
	}
	if dist(self, cand) > p.ViewDist {
		return false
	}
	return true
}

// StepToward 返回朝 dst 走一帧之后的位置。
//
// 一帧走 MoveSpeed × 100ms。走过头就直接落在目标点上 —— 不做插值,
// 因为客户端自己会平滑, 服务端只需要位置是对的（容差 Snap 260 / Tol 40）。
func StepToward(self, dst domain.Pos, speed float64) domain.Pos {
	if speed <= 0 {
		return self
	}
	dx, dy := dst.X-self.X, dst.Y-self.Y
	d := math.Hypot(dx, dy)
	step := speed * float64(domain.TickMS) / 1000
	if d <= step || d == 0 {
		out := self
		out.X, out.Y = dst.X, dst.Y
		return out
	}
	out := self
	out.X += dx / d * step
	out.Y += dy / d * step
	return out
}

func dist(a, b domain.Pos) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }
