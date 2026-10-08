package domain

// 副本。
//
// 数据出处：
//
//	assets/client/pkg/map/pworld.def       69 个副本定义：进出点、限时、实例数上限
//	assets/client/dungeon_entrances.json   入口：哪个 NPC 送你进哪个副本
//
// ⚠️ **`00-不可逆决策.md` 里"Timeout 全是 900 秒"这句话是错的。**
// 实测 69 条：900 秒 58 条、0 七条、36000 三条、600 一条。
// 而 `MaxInst` **只有 3 条有值**（幻想小岛那三个，都是 2），其余 66 条没这一项。
// 所以"读 pworld.def 的 MaxInst"也不成立 —— 绝大多数副本的实例数上限得服务端定。

const (
	// DefaultDungeonTimeoutSec 没写 Timeout 时的限时。
	//
	// 取 900 秒是因为 69 条里 58 条就是 900 —— 用众数当默认值，
	// 比拍一个新数字更贴近原作。Timeout=0 的那 7 条按"不限时"处理。
	DefaultDungeonTimeoutSec = 900

	// DefaultMaxInstances 没写 MaxInst 时同一副本最多开几个实例。
	//
	// ⚠️ **服务端定的** —— 69 条里只有 3 条有这一项。
	// 上限存在的意义不是还原原作，是防止一张图被开出无限个实例把内存吃光。
	DefaultMaxInstances = 8

	// EmptyInstanceGraceSec 新实例尚未进入过玩家时，等待首个传送交接的最长时间。
	//
	// Router 先创建场景、源场景随后才投 Enter；这个内部空窗不能把刚创建的副本删掉。
	// 一旦真正进过人，之后人数归零就立即销毁，不再使用这个宽限。
	EmptyInstanceGraceSec = 60

	// DungeonPartyGraceSec 失去组队资格后允许在副本内暂留的时间。
	// 中途恢复组队则取消清退；无资格状态下下线由存档边界直接落出口。
	DungeonPartyGraceSec = 60
)

// DungeonID 是副本定义号（pworld.def 的 PworldID）。
type DungeonID int32

// DungeonDef 是一个副本的定义。
type DungeonDef struct {
	ID   DungeonID
	Name string

	// Enter 是进去之后的落点。
	Enter Pos
	// Exit 是出来之后的落点（限时到了、或者主动离开）。
	Exit Pos

	// TimeoutSec 限时。0 表示不限时（69 条里有 7 条如此）。
	TimeoutSec int32
	// MaxInstances 同时最多开几个实例。
	MaxInstances int32
	// Solo 按角色创建专属实例，不要求组队，也不与队友共享。
	Solo bool
}

// DungeonOwner 将角色与队伍分开编码，避免两种 ID 数值相同而串副本。
type DungeonOwner struct {
	Party     PartyID
	Character CharID
}

func (o DungeonOwner) Valid() bool {
	return (o.Party != 0 && o.Character == 0) || (o.Party == 0 && o.Character > 0)
}

func (d DungeonDef) Owner(character CharID, party PartyID) DungeonOwner {
	if d.Solo {
		return DungeonOwner{Character: character}
	}
	return DungeonOwner{Party: party}
}

// Timeout 返回限时对应的帧数。0 表示不限时。
func (d DungeonDef) Timeout() Tick {
	if d.TimeoutSec <= 0 {
		return 0
	}
	return Ticks(int(d.TimeoutSec) * 1000)
}

// InstanceCap 返回实例数上限，没配就用默认。
func (d DungeonDef) InstanceCap() int32 {
	if d.MaxInstances <= 0 {
		return DefaultMaxInstances
	}
	return d.MaxInstances
}

// DungeonTable 是全部副本定义，按**进入的地图 id** 索引。
//
// 按地图 id 而不是 PworldID：玩家点的是"进这个副本"，服务端要开的是"那张图的实例"。
// 一个地图 id 对应一个副本定义。
type DungeonTable map[int32]DungeonDef

type DungeonEncounterKey struct {
	MapID   int32
	Trigger MonsterID
}

// DungeonEncounterTable 定义“某怪死亡后生成下一阶段”的副本首领链。
// 位置继承触发实体，避免数据库复制一份与地图落位可能漂移的坐标。
type DungeonEncounterTable map[DungeonEncounterKey]MonsterID

// DungeonEntrance 是一个副本入口：某个 NPC 能送你进某个副本。
type DungeonEntrance struct {
	NPC   string
	Name  string
	MapID int32
	Enter Pos
}

// EntranceTable 是全部入口，按 NPC 名字分组。
//
// 按 NPC 分组是因为查询方向就是这个：玩家点了一个 NPC，要知道他能送去哪些副本。
type EntranceTable map[string][]DungeonEntrance
