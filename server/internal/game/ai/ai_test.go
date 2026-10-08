package ai

import (
	"math"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// 决策是纯函数, 所以可以表驱动地把所有分支盖满, 不需要起场景。

// 树妖的真实参数(game_monsters 1001): 移速 110。视野/追击是列默认值 300/600。
func treeParams() Params {
	return Params{ViewDist: 300, TraceDist: 600, AttackDist: MeleeRange,
		MoveSpeed: 110, Aggressive: true}
}

func at(x, y float64) domain.Pos { return domain.Pos{MapID: 14, X: x, Y: y} }

func alive(p domain.Pos) *Target { return &Target{Pos: p, Alive: true} }

func TestDecide(t *testing.T) {
	home := at(1000, 1000)
	cases := []struct {
		name      string
		self      domain.Pos
		target    *Target
		canAttack bool
		params    func(Params) Params
		want      Action
		why       string
	}{
		{
			name: "没目标且在家", self: home, want: Hold,
			why: "站着不动就好, 索敌是场景的事",
		},
		{
			name: "没目标但离出生点远了", self: at(1400, 1000), want: Hold,
			why: "脱战位置就是新的活动位置, 不回出生点重置",
		},
		{
			name: "没目标, 离家一点点", self: at(1008, 1000), want: Hold,
			why: "有容差, 免得在出生点附近来回抖",
		},
		{
			name: "目标死了", self: home, target: &Target{Pos: home, Alive: false},
			want: DropTarget, why: "别对着尸体挥空",
		},
		{
			name: "远离出生点但目标仍在身边", self: at(1700, 1000), target: alive(at(1740, 1000)),
			want: Hold, why: "出生点不参与脱战判定",
		},
		{
			name: "目标跑太远", self: home, target: alive(at(1700, 1000)),
			want: DropTarget, why: "他跑出追击半径就放弃",
		},
		{
			name: "够得着且冷却好了", self: home, target: alive(at(1050, 1000)),
			canAttack: true, want: Strike, why: "50 < 近战距离 75",
		},
		{
			name: "够得着但在冷却", self: home, target: alive(at(1050, 1000)),
			canAttack: false, want: Hold, why: "站着等, 不要一边打一边贴脸挪动",
		},
		{
			name: "刚好在近战距离边缘", self: home, target: alive(at(1000+MeleeRange, 1000)),
			canAttack: true, want: Strike, why: "边界是闭区间",
		},
		{
			name: "差一点够不着", self: home, target: alive(at(1000+MeleeRange+1, 1000)),
			canAttack: true, want: Chase, why: "够不着就先走过去",
		},
		{
			name: "够不着且会走", self: home, target: alive(at(1200, 1000)),
			want: Chase, why: "",
		},
		{
			name: "够不着但天生不会动", self: home, target: alive(at(1200, 1000)),
			params: func(p Params) Params { p.MoveSpeed = 0; return p },
			want:   Hold, why: "食人花那类怪只能干瞪眼(全服 263 只移速为 0)",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := treeParams()
			if c.params != nil {
				p = c.params(p)
			}
			got := Decide(Input{Params: p, Self: c.self, Home: home,
				Target: c.target, CanAttack: c.canAttack})
			if got != c.want {
				t.Fatalf("期望 %v 实际 %v — %s", c.want, got, c.why)
			}
		})
	}
}

// 追击只看怪与当前目标的距离，不以出生点为牵引中心。
func TestTraceDistanceOnlyAppliesToCurrentTarget(t *testing.T) {
	home := at(0, 0)
	p := treeParams()

	// 我在家, 他跑到 700(>600)
	if got := Decide(Input{Params: p, Self: home, Home: home, Target: alive(at(700, 0))}); got != DropTarget {
		t.Errorf("目标跑出追击半径应脱战, 实际 %v", got)
	}
	// 我被牵到 700, 他就在我旁边：继续交战，不回出生点
	if got := Decide(Input{Params: p, Self: at(700, 0), Home: home, Target: alive(at(710, 0))}); got != Hold {
		t.Errorf("远离出生点但目标仍在身边时应继续交战, 实际 %v", got)
	}
	// 两个都在范围内 -> 正常追
	if got := Decide(Input{Params: p, Self: at(300, 0), Home: home, Target: alive(at(500, 0))}); got != Chase {
		t.Errorf("都在范围内应继续追, 实际 %v", got)
	}
}

// ── 索敌 ──

func TestCanAcquire(t *testing.T) {
	p := treeParams()
	home, self := at(0, 0), at(0, 0)

	if !CanAcquire(p, self, home, at(200, 0)) {
		t.Error("视野内(200<300)的玩家应该被盯上")
	}
	if CanAcquire(p, self, home, at(400, 0)) {
		t.Error("视野外(400>300)不该被盯上")
	}

	// 不主动的怪永远不主动索敌 —— 但它挨打了照样还手, 那条路径不走这里
	passive := p
	passive.Aggressive = false
	if CanAcquire(passive, self, home, at(10, 0)) {
		t.Error("不主动的怪不该主动索敌")
	}

	// 被牵到远离出生点的位置后，仍能在当前位置索敌。
	if !CanAcquire(p, at(590, 0), home, at(700, 0)) {
		t.Error("远离出生点后仍应能对附近玩家上仇恨")
	}
}

// ── 移动 ──

// 一帧走 移速 × 100ms。树妖 110/秒 = 一帧 11 个单位。
func TestStepTowardOneFrame(t *testing.T) {
	got := StepToward(at(0, 0), at(1000, 0), 110)
	if math.Abs(got.X-11) > 1e-9 || got.Y != 0 {
		t.Fatalf("一帧应走 11 个单位, 实际 (%.4f, %.4f)", got.X, got.Y)
	}
}

// 走过头要落在目标点上, 不能冲过去。
func TestStepTowardDoesNotOvershoot(t *testing.T) {
	got := StepToward(at(0, 0), at(3, 4), 1000) // 一帧能走 100, 距离只有 5
	if got.X != 3 || got.Y != 4 {
		t.Fatalf("应正好落在目标点, 实际 (%.4f, %.4f)", got.X, got.Y)
	}
}

// 斜着走的步长也该是 11, 不能变成 11√2。
func TestStepTowardDiagonalKeepsSpeed(t *testing.T) {
	start := at(0, 0)
	got := StepToward(start, at(1000, 1000), 110)
	if d := math.Hypot(got.X-start.X, got.Y-start.Y); math.Abs(d-11) > 1e-9 {
		t.Fatalf("斜向步长应仍是 11, 实际 %.4f — 没做归一化", d)
	}
}

func TestStepTowardStationary(t *testing.T) {
	start := at(5, 5)
	if got := StepToward(start, at(1000, 1000), 0); got != start {
		t.Fatal("移速 0 的怪不该动")
	}
	if got := StepToward(start, start, 110); got != start {
		t.Fatal("已经在目标点上不该动, 也不该除以 0")
	}
}

// 地图 id 要跟着走, 不能在移动时被抹掉。
func TestStepTowardKeepsMapID(t *testing.T) {
	got := StepToward(domain.Pos{MapID: 14, X: 0, Y: 0}, domain.Pos{MapID: 14, X: 100, Y: 0}, 110)
	if got.MapID != 14 {
		t.Fatalf("地图 id 丢了: %d", got.MapID)
	}
}
