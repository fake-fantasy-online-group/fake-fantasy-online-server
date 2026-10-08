package domain

import "sync/atomic"

// 标识与时间 —— 全服共用的词汇表。放在 domain 是因为每一层都要说这几个词,
// 而 domain 是唯一所有层都能 import 的地方(见 docs/架构/10-分层规范.md)。
//
// 三种 id 必须分清, 混用是 MMO 里最常见的一类脏 bug:
//
//	EntityID   运行时。进场景时分配, 离场即失效。**下行包里的就是它。**
//	CharID     数据库主键。永久。只在存档/加载/离线查询时出现。
//	MonsterID  配置表主键(game_monsters.id)。是"哪种怪", 不是"哪一只"。
//
// 抓包实测印证了这个分离: 同一只怪重生后 0x800f 里的 id 变了 ——
// 那个 id 是实体的, 不是配置的。

// EntityID 是运行时实体标识, 下行协议直接使用。
type EntityID uint32

// CharID 是角色的数据库主键。
type CharID int64

// MonsterID 是怪物配置表主键(game_monsters.id)。
type MonsterID int32

// EntityID 分段。高位段能让人一眼从日志里看出这是什么东西,
// 也保证玩家/怪物/NPC/掉落物即使各自分配也不会撞号。
const (
	entitySegSize = 1 << 26 // 每段 6700 万, 够用

	segPlayer = 0 * entitySegSize
	segNPC    = 1 * entitySegSize
	segMonter = 2 * entitySegSize
	segDrop   = 3 * entitySegSize
	// 宠物是第五段。它既不是玩家(不占连接、不存运行时会话)也不是怪
	// (不由刷怪器管、死了不重生、有主人), 混进任何一段都会让
	// "遍历 monsters 做 AI" 这类代码顺手把它当成敌人。
	segPet = 4 * entitySegSize
	// 陷阱使用独立分段。它有自己的客户端实体协议和隐藏/侦测生命周期，
	// 不能冒充怪物，否则会被 AI、仇恨和死亡重生逻辑误处理。
	segTrap = 5 * entitySegSize
)

// EntityKind 是实体类别。**权威来源是实体自己的字段**, 下面那个由 id 反推的
// SegKind 只用于日志与断言 —— 业务判类别请查实体, 别从 id 猜。
type EntityKind uint8

const (
	KindPlayer EntityKind = iota
	KindNPC
	KindMonster
	KindDrop
	KindPet
	KindTrap
	KindUnknown
)

// SegKind 由 id 所在分段反推类别。仅供日志/断言。
func (id EntityID) SegKind() EntityKind {
	switch uint32(id) / entitySegSize {
	case 0:
		return KindPlayer
	case 1:
		return KindNPC
	case 2:
		return KindMonster
	case 3:
		return KindDrop
	case 4:
		return KindPet
	case 5:
		return KindTrap
	}
	return KindUnknown
}

// EntityAlloc 分配 EntityID。**全服一个**, 不是每场景一个。
//
// 为什么全局而不是场景内唯一: 场景内唯一省不下什么(一次原子加而已),
// 却会让日志里的 id 变得有歧义 —— 排查"实体 4711 怎么了"时得先问"哪张图的 4711"。
// 传送时实体带着 id 换场景也更简单。
type EntityAlloc struct {
	next [6]atomic.Uint32
}

// NewEntityAlloc 建一个分配器。各段从 1 开始, 0 保留为"无实体"。
func NewEntityAlloc() *EntityAlloc { return &EntityAlloc{} }

func (a *EntityAlloc) alloc(seg int, base uint32) EntityID {
	n := a.next[seg].Add(1)
	// 段内回绕: 到顶就从 1 重来。段容量 6700 万, 按每秒 1000 次分配算要 19 小时才绕一圈,
	// 而实体存活时间以分钟计, 回绕时旧 id 早已失效。
	if n >= entitySegSize {
		a.next[seg].Store(1)
		n = 1
	}
	return EntityID(base + n)
}

func (a *EntityAlloc) Player() EntityID  { return a.alloc(0, segPlayer) }
func (a *EntityAlloc) NPC() EntityID     { return a.alloc(1, segNPC) }
func (a *EntityAlloc) Monster() EntityID { return a.alloc(2, segMonter) }
func (a *EntityAlloc) Drop() EntityID    { return a.alloc(3, segDrop) }
func (a *EntityAlloc) Pet() EntityID     { return a.alloc(4, segPet) }
func (a *EntityAlloc) Trap() EntityID    { return a.alloc(5, segTrap) }

// ── 时间 ──

// TickMS 是一个逻辑帧的毫秒数。**所有游戏内时长都用帧表达, 不用墙钟。**
// 100ms 的粒度对本作够用: 攻击间隔 1500ms、移速 160 单位/秒, 玩家感知不到。
const TickMS = 100

// Tick 是逻辑帧号, 场景内单调递增。
type Tick uint64

// Ticks 把毫秒换算成帧数, 向上取整 —— 宁可晚一帧也不能早, 早了就是"冷却没到就能放"。
func Ticks(ms int) Tick {
	if ms <= 0 {
		return 0
	}
	return Tick((ms + TickMS - 1) / TickMS)
}

// Millis 把帧数换回毫秒(只用于日志与对外展示)。
func (t Tick) Millis() int64 { return int64(t) * TickMS }

// ── 场景 ──

// SceneID 标识一个场景实例。
//
// Instance == 0 是常驻大陆图(一张地图一个场景);
// Instance > 0 是副本实例(同一张地图可以同时存在多个, pworld.def 给了实例数上限)。
type SceneID struct {
	MapID    int32
	Instance uint32
}

// Persistent 报告这是不是常驻图(非副本)。
func (s SceneID) Persistent() bool { return s.Instance == 0 }
