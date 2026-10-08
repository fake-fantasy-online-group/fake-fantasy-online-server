// Package scene 是并发模型的核心: **一个场景 = 一个 goroutine, 独占该场景内所有实体。**
//
// 外部想改变场景里的任何东西, 只能往 mailbox 投 Command; 场景每帧排空 mailbox 串行执行。
// 因此场景内的一切都是单线程的 —— 写玩法逻辑时完全不用想并发。
//
// 三条纪律(违反其一, 这套模型就白做了, 见 docs/架构/00-不可逆决策.md):
//
//  1. 不阻塞     场景 goroutine 里不查库、不发网络、不等锁。要落盘就丢给 Saver 排队
//  2. 不外泄指针  *Entity 不许离开场景。往外传一律转成 id 或值拷贝
//  3. 不裸 panic  一帧 panic 只能毁这一帧, 不能带走整张图, 更不能带走全服
package scene

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"runtime/debug"
	"sync"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

// Saver 是场景的存档出口(依赖倒置: 场景不认识 store)。
//
// **实现必须立即返回** —— 内部排队, 由别的 goroutine 真正写库。
// 在这里做同步 IO 会让一次慢查询卡住整张图的所有人, 那是这套架构最容易犯也最难查的错。
type Saver interface {
	Save(domain.Snapshot)
}

// Config 建场景所需的一切。
type Config struct {
	Trial         *domain.TrialRules
	DungeonParty  func(domain.CharID) domain.DungeonParty
	DungeonNotify func(string, string)

	PickupRange int32
	ID          domain.SceneID
	CellSize    float64 // AOI 格边长, 0 用默认 300
	Mailbox     int     // 命令队列容量, 0 用默认 4096
	Saver       Saver   // nil 表示不落盘(测试用)
	Log         *slog.Logger

	// SaveEvery 是脏数据批量落盘的间隔(帧)。0 用默认 30 秒。
	// 关键操作(交易/仓库/摆摊)不走这里, 它们必须在场景外同步落盘后才回包。
	SaveEvery domain.Tick

	// PlayerAttackHitDelay 是从收到玩家普攻包到服务端命中回包的精确延迟。
	// 正式客户端先发 0x100c、随后才启动攻击姿势；0 会让本地低延迟回包赶在
	// 首个动画画面前跳字。仅保留给旧测试夹具；正式服使用 CombatTiming。
	PlayerAttackHitDelay time.Duration
	// CombatTiming 是 PostgreSQL 加载的普通攻击命中时序。远程普攻按释放瞬间
	// 距离/速度计算；近战仍使用动作命中点延迟。
	CombatTiming domain.CombatTimingRule
	// MonsterDialogs 是按客户端 RES_SCENE_* 编号索引的怪物原始台词。
	MonsterDialogs *domain.MonsterDialogs

	// Frames 是外部节拍源。nil 则场景自建 ticker。
	//
	// 两个用处: 测试里手动推帧(不用 sleep, 精确到帧); 将来几百个场景共用一个
	// ticker 而不是各开一个 —— 371 张图各起一个 time.Ticker 是白扔的开销。
	Frames <-chan time.Time

	// Portals 是本图的传送门。踩上去就走人。
	Portals []domain.Portal
	// Transports 是 NPC TransList 对应的权威目的地配置。
	Transports domain.TransportTable
	// Shops 是 NPC SellList 对应的权威库存配置。
	Shops domain.ShopTable

	// Revive 是玩家死亡后选择回城复活时的落点。零值表示原地复活
	// (测试与没接传送时用)。死亡本身不会自动触发这个落点。
	//
	// MVP 定的规则是"副本死亡和野外一样, 选择复活后回城"
	// (docs/MVP边界.md 第四节)。
	Revive ReviveAt
	// Home 是活着的玩家使用回城物品时的目标；与死亡复活是否启用独立，
	// 回城点所在地图本身也需要能把玩家送回固定坐标。
	Home ReviveAt

	// Alloc 是实体 id 分配器, 造掉落物要用。
	// 不给的话 Router 会在建完场景后回填自己的那个 —— 一个区服共用一个分配器。
	Alloc *domain.EntityAlloc

	// Items 是物品/装备模板表, Loot 是怪物掉落表。任一为 nil 则不产出掉落。
	Items          map[domain.ItemID]domain.ItemDef
	Loot           map[domain.MonsterID][]domain.DropEntry
	EquipmentRoll  domain.EquipmentRollTable
	BossFixedDrops domain.BossFixedDropTable
	ItemIcons      []domain.ItemIconMapping
	ItemIconsKnown bool

	// Skills 是技能表。nil 表示放不了技能(测试用)。
	Skills domain.SkillTable
	// Statuses 是状态表。nil 表示施加不了状态(测试用)。
	Statuses domain.StatusTable
	// StatusOverlay 是状态叠加矩阵。nil 时保留旧测试的单实例语义。
	StatusOverlay domain.StatusOverlay
	// Quests 是任务表。nil 表示接不了任务(测试用)。
	Quests domain.QuestTable
	// Works 是打工定义, Stamina 是耐力规则。
	Works          domain.WorkTable
	Stamina        domain.StaminaRule
	NianliCreation domain.NianliCreationTable
	Crafts         domain.CraftTable
	Refines        domain.RefineTable
	Sockets        domain.SocketTable
	Washes         domain.WashTable
	WarehouseRule  domain.WarehouseRule
	Wardrobes      domain.WardrobeTable
	WardrobeRule   domain.WardrobeRule
	HairRules      domain.HairRules
	StallOpenItems domain.StallOpenItems
	Rack           domain.RackCatalog
	DepositRule    domain.DepositRule
	// RequireWorkGather 让生产场景只在收到客户端 0x1025 后结算。测试夹具省略时
	// 保留纯时钟推进，便于不依赖协议层验证结算公式。
	RequireWorkGather bool
	// Rest 是坐下休息的周期恢复规则。零值表示数据库配置未加载，玩法关闭。
	Rest domain.RestRule
	// Death 是普通场景死亡时扣除经验和随身金钱的规则。零值表示不扣除。
	Death domain.DeathRule
	// Penalty 是跨级打怪的等级差惩罚。零值表示不启用（经验与掉率不递减）。
	Penalty domain.LevelPenalty
	// Now 给"今天是哪一天"用(耐力每日回满)。nil 则用 time.Now。
	Now func() time.Time
	// Pets 是宠物种族表, PetLevels 是宠物经验曲线。任一为 nil 则抓不了宠。
	Pets      domain.PetTable
	PetLevels *domain.PetLevelTable
	// PetFoods 是宠物食物表。nil 则喂不了。
	PetFoods          map[domain.ItemID]domain.PetFood
	PetPrefixes       domain.PetPrefixTable
	PetSkills         domain.PetSkillTable
	PetLearningItems  map[domain.ItemID]domain.PetLearningItem
	PetRule           domain.PetRule
	RideInviteSeconds int32
	// SkipNPCSpawn 进图时不下发 NPC 落位包。
	//
	// **只为排查**: NPC 包(0x8046)的字段格式没对好时客户端会**闪退**,
	// 而且服务端毫无异常。关掉它就能把"进不进得去世界"与"NPC 包对不对"分开验。
	SkipNPCSpawn bool

	// Dungeons 是副本表, 按进入的地图 id 索引。
	Dungeons domain.DungeonTable
	// DungeonEncounters 是副本首领死亡后的阶段生成链。
	DungeonEncounters domain.DungeonEncounterTable
	// Dungeon 非 nil 表示**这个场景本身就是一个副本实例**, 会限时、会自己关。
	Dungeon *domain.DungeonDef
	// NPCs 是本图的 NPC 落位。建场景时铺进去, 与刷怪同理。
	NPCs []domain.NPCSpawn

	// Levels 是经验曲线。nil 表示不结算经验(测试用)。
	Levels *domain.LevelTable
	// Defs 是怪物模板表, 击杀结算时回查经验值。nil 则不给经验。
	Defs spawn.Defs

	// Spawner 管本图的怪。nil 表示这张图不刷怪(主城、测试)。
	//
	// 建场景时就把怪铺满, 而不是等第一个玩家进来 —— 否则先进图的人会看到空场,
	// 而且怪的重生节奏会跟着"谁先进图"漂移。
	Spawner *spawn.Spawner
	// Walkable 是客户端 QQF MASK 的可行走判定。nil 只用于不带地图资源的测试；
	// 正式场景加载不到碰撞图时由装配层注入“全部不可走”，宁可停掉游荡也不能进水。
	Walkable func(domain.Pos) bool

	// Seed 是本场景随机数的种子。0 = 按时间取。
	//
	// 每个场景一条独立的随机流, 是为了**可复现**: 出了「这一刀怎么会打出这个数」
	// 的问题, 拿同样的种子加同样的命令序列就能重放。测试一律显式给种子。
	Seed int64
}

// ReviveAt 是复活点。Scene 为零值时表示不传送, 原地站起来。
type ReviveAt struct {
	Scene domain.SceneID
	Pos   domain.Pos
}

// Enabled 报告是否配了真正的复活点。
func (r ReviveAt) Enabled() bool { return r.Scene.MapID != 0 }

// Scene 是一张地图或一个副本实例。
//
// 除 mailbox / quit / log 外的所有字段, **只有场景 goroutine 能碰**。
type Scene struct {
	trial         *domain.TrialRules
	trialLevel    int32
	trialCleared  bool
	dungeonParty  func(domain.CharID) domain.DungeonParty
	dungeonNotify func(string, string)

	pickupRange int32
	id          domain.SceneID
	mailbox     chan Command
	quit        chan struct{}
	done        chan struct{} // Run 已退出；与 quit 分开，ctx 取消也会关闭它
	log         *slog.Logger
	saver       Saver
	// postMu 把“确认仍接客 + 入 mailbox”与 Stop 的封门做成同一个顺序点。
	// 锁只覆盖内存状态与非阻塞 send，绝不跨 IO。
	postMu  sync.RWMutex
	stopped bool
	// handoffs 是别的场景已经交出所有权的 Enter。它与普通玩法 mailbox 分开：
	// 入队只持 postMu 做一次 append，既不会因玩法流量满而丢，也不会阻塞源 actor
	// 等目标帧（双向传送时那会死锁）。目标每帧总在普通命令前消费它。
	handoffs []Enter

	// ── 以下仅场景 goroutine 访问, 无需锁 ──
	tick                 domain.Tick
	entities             map[domain.EntityID]*entity.Entity
	players              map[domain.EntityID]*entity.Entity // entities 的子集; 广播只遍历它
	tradeSettlements     []*pendingTradeSettlement
	tradeDeferred        map[domain.EntityID][]func()
	tradeLeaves          map[domain.EntityID]Leave
	tradeTeleports       map[domain.EntityID]Teleport
	monsters             map[domain.EntityID]*entity.Entity // 同上; stepAI 只遍历它, 不用扫全场
	ground               map[domain.EntityID]*entity.Entity // 同上; 地上的掉落物
	pets                 map[domain.EntityID]*entity.Entity // 同上; stepPets 只遍历它
	traps                map[domain.EntityID]*entity.Entity // 刺客放置、等待触发的一次性陷阱
	aoi                  *AOI
	timer                timers
	outbox               []pending
	mapLoads             map[domain.EntityID]*mapLoad
	attacks              []pendingAttack                        // 已受理、等待动画命中帧的攻击
	casts                []pendingCast                          // 本帧待结算的技能
	hits                 map[domain.EntityID]*combat.HitTracker // 每个攻击者的命中步进器
	rng                  *rand.Rand                             // 场景私有, 单线程用, 可复现
	aiRng                *rand.Rand                             // AI 游荡独立随机流，不扰动伤害/掉落序列
	playerAttackHitDelay time.Duration                          // 旧测试夹具兼容的固定命中延迟
	combatTiming         domain.CombatTimingRule                // 正式普通攻击命中时序
	monsterDialogs       *domain.MonsterDialogs                 // 怪物各事件的原始台词
	spawner              *spawn.Spawner                         // nil = 这张图不刷怪
	walkable             func(domain.Pos) bool                  // 怪物游荡和局部追击寻路的 MASK 判定
	portals              []domain.Portal                        // 本图的传送门
	transports           domain.TransportTable                  // NPC 传送菜单
	shops                domain.ShopTable                       // NPC 商店库存
	// portalBlocked 记录“刚被服务端传送到门区内”的玩家。只要仍处于任意
	// 门区，后续移动包都不能再次触门；完全离开后才重新武装边沿触发。
	portalBlocked map[domain.EntityID]struct{}
	// moveWindows 只记真实客户端 0x1017 的短周期位移额度。传送和离场会清掉，
	// 不进角色存档，也不跨场景交接。
	moveWindows        map[domain.EntityID]playerMoveWindow
	stallBrowsers      map[domain.EntityID]map[domain.EntityID]struct{}
	levels             *domain.LevelTable               // 经验曲线
	defs               spawn.Defs                       // 怪物模板, 结算经验时回查
	items              map[domain.ItemID]domain.ItemDef // 物品模板
	itemIcons          []domain.ItemIconMapping         // 0x804d 特殊图标覆盖
	itemIconsKnown     bool
	skills             domain.SkillTable    // 技能定义
	statuses           domain.StatusTable   // 状态定义
	statusOverlay      domain.StatusOverlay // 状态叠加矩阵
	quests             domain.QuestTable    // 任务定义
	dungeons           domain.DungeonTable  // 副本表
	works              domain.WorkTable     // 打工定义
	nianliCreation     domain.NianliCreationTable
	requireWorkGather  bool               // 收益是否必须由 0x1025 触发
	crafts             domain.CraftTable  // 合成配方
	refines            domain.RefineTable // 精炼配方
	sockets            domain.SocketTable // 打孔/镶嵌配方
	washes             domain.WashTable   // 洗练配方
	warehouseRule      domain.WarehouseRule
	wardrobes          domain.WardrobeTable
	wardrobeRule       domain.WardrobeRule
	hairRules          domain.HairRules
	stallOpenItems     domain.StallOpenItems
	rack               domain.RackCatalog
	depositRule        domain.DepositRule
	petDefs            domain.PetTable                  // 宠物种族表
	petLvls            *domain.PetLevelTable            // 宠物经验曲线
	petFoods           map[domain.ItemID]domain.PetFood // 宠物食物
	petPrefixes        domain.PetPrefixTable
	petSkills          domain.PetSkillTable
	petLearnItems      map[domain.ItemID]domain.PetLearningItem
	petPPAiCap         int32
	rideInviteSeconds  int32
	petRule            domain.PetRule
	skipNPC            bool                // 排查用: 不下发 NPC 落位
	stamina            domain.StaminaRule  // 耐力规则
	rest               domain.RestRule     // 坐下休息规则
	death              domain.DeathRule    // 玩家死亡损失规则
	penalty            domain.LevelPenalty // 跨级打怪的等级差惩罚
	now                func() time.Time    // 取当前时间(耐力每日回满用)
	dungeon            *domain.DungeonDef  // 非 nil = 本场景是副本实例
	dungeonElapsedBase domain.Tick         // 本层创建前副本已消耗的帧数
	everOccupied       bool                // 已有玩家真正进入过；单层副本人数归零后立即销毁
	dungeonEncounters  domain.DungeonEncounterTable
	encounterDone      map[domain.MonsterID]bool

	// emptySince 是这个实例空掉的那一帧。0 表示现在有人。
	emptySince     domain.Tick
	lootTable      map[domain.MonsterID][]domain.DropEntry // 掉落表
	equipmentRoll  domain.EquipmentRollTable
	bossFixedDrops domain.BossFixedDropTable
	alloc          *domain.EntityAlloc // 造掉落物实体用; 由 Router 回填
	revive         ReviveAt            // 死亡后送回哪
	home           ReviveAt            // 活着时使用回城物品的落点
	router         *Router             // 跨场景投递; 由 Router 建完场景后回填

	saveEvery domain.Tick
	frames    <-chan time.Time // 外部节拍源; nil 则 Run 自建 ticker
	panics    int              // 连续 panic 帧数; 连着崩说明状态已经坏了, 该让 Router 收场
}

// pending 是一条待投递的事件。广播中心格在**产生事件时**就取好, 因为到帧末
// 冲刷时主体可能已经离场了(典型: 死亡消失事件)。
type pending struct {
	ev event.Event
	to domain.EntityID // 非 0 = 只发这个人; 0 = 发给同地图全部玩家
	// except 在广播时跳过这个人。移动事件用它跳过自己 —— 客户端已经本地移动过了,
	// 再收一条自己的移动会把画面拽回去。
	except domain.EntityID
}

// pendingAttack 是已经受理、等着在动画命中帧统一结算的一次出手。
// 只存 id 不存指针 —— 从收到命令到结算之间隔着半帧, 那个实体可能已经没了。
type pendingAttack struct {
	src, dst      domain.EntityID
	blow          combat.Blow
	resolveAt     domain.Tick // 怪物/宠物及兼容测试使用的主帧时刻
	resolveAtWall time.Time   // 玩家攻击的精确命中时刻；零值表示跟随主帧
	reserved      bool        // true 表示受理时已经占用冷却，结算时不能再起算一次
	counter       bool        // 被动反击；MISS 后不能再递归触发另一轮反击
}

const (
	defaultMailbox   = 4096
	maxPanicsInARow  = 3
	defaultSaveEvery = 300 // 帧。300 × 100ms = 30 秒

	// corpseTicks 尸体停留多久才从场上移除。3 秒 —— 够客户端播完倒地动画,
	// 也够玩家看清自己打死了什么。掉落拾取窗口另算, 不由它决定。
	corpseTicks = 30
)

// New 建一个场景。**不启动** —— 启动由 Router 负责。
func New(cfg Config) *Scene {
	if cfg.Mailbox <= 0 {
		cfg.Mailbox = defaultMailbox
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.SaveEvery <= 0 {
		cfg.SaveEvery = defaultSaveEvery
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Stamina.Max <= 0 {
		cfg.Stamina = domain.DefaultStaminaRule()
	}
	if !cfg.CombatTiming.Valid() && cfg.PlayerAttackHitDelay > 0 {
		cfg.CombatTiming.MeleeHitDelayMS = int32(cfg.PlayerAttackHitDelay / time.Millisecond)
	}
	if cfg.Seed == 0 {
		// 混进地图与实例号, 免得同一时刻建出来的几个场景共用一条随机流
		cfg.Seed = time.Now().UnixNano() ^
			(int64(cfg.ID.MapID) << 32) ^ int64(cfg.ID.Instance)
	}
	s := &Scene{
		id:                   cfg.ID,
		mailbox:              make(chan Command, cfg.Mailbox),
		quit:                 make(chan struct{}),
		done:                 make(chan struct{}),
		log:                  cfg.Log.With("map", cfg.ID.MapID, "inst", cfg.ID.Instance),
		saver:                cfg.Saver,
		entities:             map[domain.EntityID]*entity.Entity{},
		players:              map[domain.EntityID]*entity.Entity{},
		monsters:             map[domain.EntityID]*entity.Entity{},
		ground:               map[domain.EntityID]*entity.Entity{},
		pets:                 map[domain.EntityID]*entity.Entity{},
		traps:                map[domain.EntityID]*entity.Entity{},
		aoi:                  newAOI(cfg.CellSize),
		hits:                 map[domain.EntityID]*combat.HitTracker{},
		rng:                  rand.New(rand.NewSource(cfg.Seed)),
		aiRng:                rand.New(rand.NewSource(cfg.Seed ^ 0x5deece66d)),
		playerAttackHitDelay: cfg.PlayerAttackHitDelay,
		combatTiming:         cfg.CombatTiming,
		monsterDialogs:       cfg.MonsterDialogs,
		saveEvery:            cfg.SaveEvery,
		frames:               cfg.Frames,
		spawner:              cfg.Spawner,
		trial:                cfg.Trial, dungeonParty: cfg.DungeonParty, dungeonNotify: cfg.DungeonNotify,
		walkable:          cfg.Walkable,
		portals:           cfg.Portals,
		transports:        cfg.Transports,
		shops:             cfg.Shops,
		portalBlocked:     map[domain.EntityID]struct{}{},
		moveWindows:       map[domain.EntityID]playerMoveWindow{},
		stallBrowsers:     map[domain.EntityID]map[domain.EntityID]struct{}{},
		revive:            cfg.Revive,
		home:              cfg.Home,
		levels:            cfg.Levels,
		defs:              cfg.Defs,
		items:             cfg.Items,
		itemIcons:         append([]domain.ItemIconMapping(nil), cfg.ItemIcons...),
		itemIconsKnown:    cfg.ItemIconsKnown,
		skills:            cfg.Skills,
		statuses:          cfg.Statuses,
		statusOverlay:     cfg.StatusOverlay,
		quests:            cfg.Quests,
		dungeons:          cfg.Dungeons,
		dungeonEncounters: cfg.DungeonEncounters,
		encounterDone:     make(map[domain.MonsterID]bool),
		dungeon:           cfg.Dungeon,
		works:             cfg.Works,
		nianliCreation:    cfg.NianliCreation,
		requireWorkGather: cfg.RequireWorkGather,
		crafts:            cfg.Crafts,
		refines:           cfg.Refines,
		sockets:           cfg.Sockets,
		washes:            cfg.Washes,
		warehouseRule:     cfg.WarehouseRule,
		wardrobes:         cfg.Wardrobes,
		wardrobeRule:      cfg.WardrobeRule,
		hairRules:         cfg.HairRules,
		stallOpenItems:    cfg.StallOpenItems,
		rack:              cfg.Rack,
		depositRule:       cfg.DepositRule,
		petDefs:           cfg.Pets,
		petLvls:           cfg.PetLevels,
		petFoods:          cfg.PetFoods,
		petPrefixes:       cfg.PetPrefixes,
		petSkills:         cfg.PetSkills,
		petLearnItems:     cfg.PetLearningItems,
		petRule:           cfg.PetRule,
		rideInviteSeconds: cfg.RideInviteSeconds,
		pickupRange:       cfg.PickupRange,
		skipNPC:           cfg.SkipNPCSpawn,
		stamina:           cfg.Stamina,
		rest:              cfg.Rest,
		death:             cfg.Death,
		penalty:           cfg.Penalty,
		now:               cfg.Now,
		lootTable:         cfg.Loot,
		equipmentRoll:     cfg.EquipmentRoll,
		bossFixedDrops:    cfg.BossFixedDrops,
		alloc:             cfg.Alloc,
	}
	for _, item := range cfg.Items {
		if item.PetAffectionPP != nil {
			s.petPPAiCap = item.PetAffectionPP.Limit
		}
	}
	s.populate()
	s.placeNPCs(cfg.NPCs)
	return s
}

// placeNPCs 把 NPC 铺进场景。
//
// NPC 是**实体**而不是一批静态落位包 —— 要它是实体的理由只有一个:
// 接任务要能判"你站在发布这个任务的 NPC 面前"。
// 它们不动、不打人、不会死, 但要能被看见、被靠近。
func (s *Scene) placeNPCs(list []domain.NPCSpawn) {
	if len(list) == 0 {
		return
	}
	// **没有分配器就报出来, 不要静默返回。**
	// 这里静默过一次: Builder 忘了传 Alloc, 而 Router 是建完场景才回填的,
	// 于是全服 NPC 一个都没落下去 —— 没有任何日志, 表现只是"城里没人"。
	if s.alloc == nil {
		s.log.Error("没有实体分配器, NPC 落位被跳过", "本该落位", len(list))
		return
	}
	for _, n := range list {
		e := &entity.Entity{
			ID: s.alloc.NPC(), Kind: domain.KindNPC, Name: n.Name,
			Pos: n.Pos, Dir: uint8(n.Dir),
			Look: domain.Look{ModelID: n.Sprite},
			// sprite 的资源名是 "npc" + 模型号(见 data.NpcEntry.Index 的注释)。
			// 立绘使用数据库中核实过的资源名；未配置时保持为空。
			NPC: &entity.NPCInfo{
				Sprite:   fmt.Sprintf("npc%d", n.Sprite),
				Portrait: n.Portrait,
				Script:   n.Script,
				Sell:     n.Sell,
				Trans:    n.Trans,
				Role:     n.Role,
				Greeting: n.Greeting,
			},
		}
		s.entities[e.ID] = e
		s.aoi.Enter(e)
	}
	s.log.Info("NPC 落位", "数量", len(list))
}

// populate 开场把怪铺满。
//
// 在 New 里做而不是在第一帧做: 场景一旦存在就该是完整的世界, 不该有"还没长出怪"
// 的中间态。这时还没有玩家, 所以不发任何出场事件 —— 玩家进图时自然会看到它们。
func (s *Scene) populate() {
	if s.spawner == nil {
		return
	}
	for _, e := range s.spawner.Initial() {
		s.initializeMonster(e)
		s.entities[e.ID] = e
		s.monsters[e.ID] = e
		s.aoi.Enter(e)
	}
	if n := len(s.entities); n > 0 {
		s.log.Info("刷怪完成", "怪数", n, "刷怪点", s.spawner.Count())
	}
}

// ID 返回场景标识。可从任意 goroutine 调用(建后不变)。
func (s *Scene) ID() domain.SceneID { return s.id }

// MonsterDialogLines 返回指定原始 scene/group 的台词。目录在建场景后
// 只读，结果是副本，可从任意 goroutine 查询。
func (s *Scene) MonsterDialogLines(monster domain.MonsterID, scene, group uint8) []string {
	return s.monsterDialogs.Lines(monster, scene, group)
}

// Post 从任意 goroutine 投递命令。
//
// **非阻塞**: 队列满就丢弃并计数。宁可丢一条移动, 也不能让一个慢客户端
// 或者一个刷包的外挂把整张图的循环拖住。
func (s *Scene) Post(c Command) bool {
	s.postMu.RLock()
	defer s.postMu.RUnlock()
	if s.stopped {
		return false
	}
	select {
	case s.mailbox <- c:
		return true
	default:
		s.log.Warn("mailbox 满, 丢弃命令", "cmd", c.CmdName())
		return false
	}
}

// PostHandoff 接受一条跨场景 Enter 所有权。它是常数时间、非阻塞的内存入队；
// Stop 与本方法共用 postMu，因此返回 true 后该 Enter 必会由正常帧、关服排空或
// panic 终结路径中的一个接管，不能落在两个场景之间。
func (s *Scene) PostHandoff(c Enter) bool {
	s.postMu.Lock()
	defer s.postMu.Unlock()
	if s.stopped {
		return false
	}
	s.handoffs = append(s.handoffs, c)
	return true
}

// PostLifecycle 投递不能丢的生命周期或已落库状态同步命令（Leave/RenamePlayer）。
//
// 普通玩法命令在 mailbox 满时必须立即失败，避免一个刷包客户端阻塞网关；但 Leave
// 若同样被丢，场景会永久留下玩家实体，后续重登者会收到旧人的 0x8023。生命周期
// 投递因此允许调用方等到下一个场景帧腾出队列空间。场景已经停止时明确返回 false；
// shutdown 会保存并清空全部实体，此时无需也不可能再投 Leave。
func (s *Scene) PostLifecycle(c Command) bool {
	for {
		// 不持读锁等待队列空间。否则 mailbox 满且场景异常退出时，Stop 会等
		// 这把读锁，而此后已无人腾位置，形成关服死锁。
		s.postMu.RLock()
		if s.stopped {
			s.postMu.RUnlock()
			return false
		}
		select {
		case s.mailbox <- c:
			s.postMu.RUnlock()
			return true
		default:
			s.postMu.RUnlock()
		}
		select {
		case <-s.done:
			return false
		case <-time.After(time.Millisecond):
		}
	}
}

// Run 跑场景循环, 阻塞到 ctx 取消或 Stop。由 Router 在独立 goroutine 里调。
func (s *Scene) Run(ctx context.Context) {
	defer func() {
		// 任何退出路径（包括连续 panic）都必须先原子封门，再宣布 done。
		s.Stop()
		close(s.done)
	}()
	frames := s.frames
	if frames == nil {
		tk := time.NewTicker(domain.TickMS * time.Millisecond)
		defer tk.Stop()
		frames = tk.C
	}
	s.log.Info("场景启动")

	for {
		// 玩家命中表现与技能弹道需要小于 100ms 的精确相位，但整个世界仍保持
		// 100ms 逻辑帧。定时器只唤醒同一个场景 actor，不并发碰世界状态。
		var hitTimer *time.Timer
		var hitC <-chan time.Time
		if due, ok := s.nextTimedCombatAt(); ok {
			wait := time.Until(due)
			if wait < 0 {
				wait = 0
			}
			hitTimer = time.NewTimer(wait)
			hitC = hitTimer.C
		}
		select {
		case <-ctx.Done():
			stopTimer(hitTimer)
			s.shutdownGracefully("ctx")
			return
		case <-s.quit:
			stopTimer(hitTimer)
			s.shutdownGracefully("stop")
			return
		case now := <-hitC:
			if !s.hitFrame(now) {
				s.Stop()
				s.finalizePendingEnters()
				s.shutdown("panic")
				return
			}
		case <-frames:
			stopTimer(hitTimer)
			if !s.frame() {
				s.Stop()
				// 连续 panic 说明当前状态不可再执行任意业务命令；但封门前已经
				// 接受、尚未落地的 Enter 自带角色租约，直接丢弃会永久封住重登。
				// 只终结这些未入场所有权，其余命令不再执行。
				s.finalizePendingEnters()
				s.shutdown("panic")
				return
			}
		}
	}
}

func stopTimer(t *time.Timer) {
	if t == nil || t.Stop() {
		return
	}
	select {
	case <-t.C:
	default:
	}
}

// nextTimedCombatAt 返回最早的普通攻击、技能吟唱结束或技能命中时刻。
// 只有场景 actor 调用，无需锁。
func (s *Scene) nextTimedCombatAt() (time.Time, bool) {
	var earliest time.Time
	for _, at := range s.attacks {
		if at.resolveAtWall.IsZero() {
			continue
		}
		if earliest.IsZero() || at.resolveAtWall.Before(earliest) {
			earliest = at.resolveAtWall
		}
	}
	for _, cast := range s.casts {
		if cast.windup != nil && cast.windup.interrupted {
			continue
		}
		if !cast.released && !cast.releaseAtWall.IsZero() {
			if earliest.IsZero() || cast.releaseAtWall.Before(earliest) {
				earliest = cast.releaseAtWall
			}
			continue
		}
		if cast.resolveAtWall.IsZero() {
			continue
		}
		if earliest.IsZero() || cast.resolveAtWall.Before(earliest) {
			earliest = cast.resolveAtWall
		}
	}
	return earliest, !earliest.IsZero()
}

// hitFrame 是主逻辑帧之间的精确时序相位。它仍在场景 Run goroutine 内执行；
// 只处理到期的定时攻击、吟唱结束和技能命中，并按原事件顺序 flush，
// 不推进 AI、状态或重生计时。
func (s *Scene) hitFrame(now time.Time) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			s.panics++
			s.log.Error("场景命中相位 panic", "tick", s.tick, "panic", r,
				"连续次数", s.panics, "stack", string(debug.Stack()))
			ok = s.panics < maxPanicsInARow
		}
	}()
	s.stepCastsAt(now, true)
	s.stepCombatAt(now, true)
	s.flush()
	s.panics = 0
	return true
}

// shutdownGracefully 先执行已经被 mailbox 接受的命令，再做最终快照。
//
// 网关返回只能证明 Session.OnClose 已把 Leave 送进队列，不代表场景
// 已执行它。若 Stop 分支直接 shutdown，同一队列里 Leave 前面的穿戴、
// 拾取或移动也会被略过，关服快照就回退到动作前。
func (s *Scene) shutdownGracefully(reason string) {
	// ctx 取消分支未必经过 Router.Drain/Stop；先关 quit，统一封禁新 Post。
	s.Stop()
	func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("关服排空 mailbox 时 panic", "panic", r,
					"stack", string(debug.Stack()))
				// drainForShutdown 已经取出的 Enter 由 execTracked 终结；但一次
				// panic 会中断正常排空，队列后面仍可能有已经被 Post 接受、尚未
				// 落地的 Enter。场景已经封门，不能再执行其余业务命令，只把这些
				// Enter 的完整加载快照交回最终写回并关闭对应连接，避免永久泄漏
				// 角色租约。已经注册的玩家仍由下面的 shutdown 保存最新状态。
				s.finalizePendingEnters()
			}
		}()
		s.drainForShutdown()
	}()
	s.shutdown(reason)
}

// drainForShutdown 排空封门前已接受的命令。Stop 已关闭 quit，新的
// Scene.Post 会失败；Router.Drain 也已封禁 Ensure，所以这里可以安全读到空，
// 不会被新流量无限补满。
func (s *Scene) drainForShutdown() {
	for {
		enter, ok := s.popHandoff()
		if !ok {
			break
		}
		s.execTracked(enter)
	}
	for {
		select {
		case c := <-s.mailbox:
			// 关服时不再发起新的跨场景所有权交接。Router 已封门，
			// 若执行 onTeleport，玩家会先从源场景删除，再因目标
			// Post 失败而两边都不保存。保留在源场景才能被最终快照。
			if cmd, ok := c.(Teleport); ok && cmd.To != s.id {
				s.log.Debug("关服时保留玩家在原场景", "id", cmd.ID, "to", cmd.To)
				continue
			}
			s.execTracked(c)
		default:
			return
		}
	}
}

// Stop 请求停止。可重复调用。
func (s *Scene) Stop() {
	s.postMu.Lock()
	if !s.stopped {
		s.stopped = true
		close(s.quit)
	}
	s.postMu.Unlock()
}

// frame 推进一帧。返回 false 表示这张图已经救不回来了, 该收摊。
//
// panic 在这里被兜住而不是逃到 Run 外面 —— 一帧崩掉是可以接受的损失,
// 整张图消失不是。但连着崩 maxPanicsInARow 帧就说明状态本身坏了,
// 这时候继续跑只会把错误写进存档, 不如让 Router 重建。
func (s *Scene) frame() (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			s.panics++
			s.log.Error("场景帧 panic", "tick", s.tick, "panic", r,
				"连续次数", s.panics, "stack", string(debug.Stack()))
			ok = s.panics < maxPanicsInARow
		}
	}()
	s.drain()
	s.step()
	s.panics = 0
	return true
}

// drain 一次排空已交接 Enter 与普通 mailbox。
//
// 为什么要排空而不是在 select 里一条条收: 同一帧到达的命令一起结算, 结果就与
// 到达顺序无关(除了显式排序的部分), 这是"可复现"的前提。
// 循环有上界, 否则一个疯狂刷包的客户端能让这里永远出不去。
func (s *Scene) drain() {
	for {
		enter, ok := s.popHandoff()
		if !ok {
			break
		}
		s.execTracked(enter)
	}
	for i := 0; i < cap(s.mailbox); i++ {
		select {
		case c := <-s.mailbox:
			s.execTracked(c)
		default:
			return
		}
	}
}

// popHandoff 每次只取一个：若处理某个 Enter panic，后续所有权仍留在队列里，
// 下一帧可重试或由 finalizePendingEnters 逐个终结，不会因批量换出切片而遗失。
func (s *Scene) popHandoff() (Enter, bool) {
	s.postMu.Lock()
	defer s.postMu.Unlock()
	if len(s.handoffs) == 0 {
		return Enter{}, false
	}
	c := s.handoffs[0]
	s.handoffs[0] = Enter{}
	s.handoffs = s.handoffs[1:]
	if len(s.handoffs) == 0 {
		s.handoffs = nil
	}
	return c, true
}

// execTracked 保留原来的 panic 传播语义，但保证一个尚未注册实体的 Enter 即使
// 自己在构造阶段 panic，也会交出最终快照并断开对应客户端，不把 lease 遗失在栈上。
func (s *Scene) execTracked(c Command) {
	defer func() {
		if r := recover(); r != nil {
			if enter, ok := c.(Enter); ok {
				s.finalizeUnentered(enter)
			}
			panic(r)
		}
	}()
	s.exec(c)
}

// finalizePendingEnters 仅用于场景因连续 panic 放弃恢复之后。此时执行普通命令可能
// 继续破坏状态；未入场 Enter 的加载快照却仍是完整所有权，可以安全直接写回。
func (s *Scene) finalizePendingEnters() {
	for {
		enter, ok := s.popHandoff()
		if !ok {
			break
		}
		s.finalizeUnentered(enter)
	}
	for {
		select {
		case c := <-s.mailbox:
			if enter, ok := c.(Enter); ok {
				s.finalizeUnentered(enter)
			}
		default:
			return
		}
	}
}

func (s *Scene) finalizeUnentered(cmd Enter) {
	// onEnter 若已把实体注册进场景，shutdown 会从实体上的 FinalSaver 保存更
	// 新状态；这里不能用进入前快照覆盖它。
	if _, exists := s.entities[cmd.ID]; exists {
		return
	}
	if cmd.FinalSaver != nil && cmd.Char != nil {
		cmd.FinalSaver.Save(domain.Snapshot{
			Char: cmd.Char, Bag: cmd.Bag, Worn: cmd.Worn, ChangeSet: cmd.ChangeSet,
			Warehouse: cmd.Warehouse,
			Wardrobe:  cmd.Wardrobe,
			Stall:     cmd.Stall,
		}.Clone())
	}
	if cmd.Sink != nil {
		cmd.Sink.Close()
	}
}

// step 推进一帧的游戏逻辑。**顺序是固定的, 不能随便调。**
func (s *Scene) step() {
	s.finishReadyTrades(false)
	s.tick++
	if s.dungeon == nil && s.spawner != nil && s.tick%spawn.EliteChanceInterval == 0 {
		s.spawner.RecordMinute()
	}
	s.timer.advance(s.tick)     // 1. 冷却/刷怪/重生/尸体移除
	s.stepStatus()              // 2. 状态: 周期触发与到期。**必须在任何人行动之前**
	s.stepRest()                // 3. 坐下休息: 周期恢复生命
	s.stepWork()                // 4. 打工: 结算与扣耐力
	s.stepNianli()              // 5. 念力: 只累计在线时间
	s.stepPets()                // 6. 宠物: 跟随主人、推进出战期饥饿与信赖
	s.stepAI()                  // 7. 怪物寻敌、移动、决定出手
	s.stepTraps()               // 8. 怪物踏入后触发上一帧已经落地的陷阱
	s.stepCasts()               // 9. 结算本帧全部技能
	s.stepCombat()              // 10. 结算本帧全部普通攻击
	s.checkDungeonEligibility() // 11. 副本退队宽限到期时只清退本人
	s.flush()                   // 12. 按 AOI 把本帧事件广播出去
	s.checkInstanceLife()       // 13. 副本实例: 限时到 / 空了就收摊

	if s.tick%s.saveEvery == 0 {
		s.saveDirty()
	}
}

// stepCombat 结算本帧收到的全部攻击。
//
// 攒到这里统一算, 而不是收到命令就算, 是为了**公平**: 同一帧内谁的包先到不影响结果,
// 网络好的客户端占不到便宜。这也是逻辑帧模型的意义所在。
func (s *Scene) stepCombat() {
	s.stepCombatAt(s.now(), false)
}

// stepCombatAt 在主帧或精确命中相位结算攻击。timedOnly=true 时保留怪物/宠物
// 的主帧攻击，避免 50ms 表现定时器改变它们原有的攻速节奏。
func (s *Scene) stepCombatAt(now time.Time, timedOnly bool) {
	if len(s.attacks) == 0 {
		return
	}
	originalLen := len(s.attacks)
	waiting := s.attacks[:0]
	counters := make([]pendingAttack, 0, 1)
	for _, at := range s.attacks {
		if !at.resolveAtWall.IsZero() {
			if now.Before(at.resolveAtWall) {
				waiting = append(waiting, at)
				continue
			}
		} else if timedOnly || at.resolveAt > s.tick {
			waiting = append(waiting, at)
			continue
		}
		src, dst := s.entities[at.src], s.entities[at.dst]
		// 从收到命令到动画命中帧之间，双方都可能已经不在了 —— 所以存 id 不存指针。
		if src == nil || dst == nil || !src.Alive() || !dst.Alive() {
			continue
		}
		// 辅助宠物不属于战斗单位。即便这里残留了一条旧队列或异常请求，
		// 也不能让宠物造成或承受伤害。
		if src.Kind == domain.KindPet || dst.Kind == domain.KindPet {
			continue
		}
		if !at.reserved {
			src.DidBasicAttack(s.tick)
		}

		tr := s.hits[src.ID]
		if tr == nil {
			tr = &combat.HitTracker{}
			s.hits[src.ID] = tr
		}
		ev, rawDamage, fullMagicReflect := s.resolveStrikeStatuses(src, dst, tr, at.blow)
		// 伤害数值是**平衡性问题的唯一入口**: 打不死怪时, 光看"有没有伤害包"
		// 分不出是没打中还是每下只有三十点(实测 63 级角色因为六维是空的,
		// 按一级战力在打 12 级怪)。
		s.log.Debug("命中", "攻", src.Name, "受", dst.Name,
			"伤害", ev.Amount, "剩余", ev.DstHP, "标志", ev.Flag)
		s.emit(ev)
		if ev.Flag.Has(event.DamageMiss) && !at.counter {
			if counter, ok := s.passiveCounterPending(dst, src); ok {
				counters = append(counters, counter)
			}
		}
		s.afterStrikeStatuses(src, dst, ev, rawDamage, fullMagicReflect)
		s.consumeAttackDurability(src)
		s.consumeHitDurability(dst, ev)
		s.emitAttributesIfPlayer(dst)
		// 挨打就还手: 不主动的怪靠这条上仇恨, 它们不会主动找人但一定回击
		s.aggro(dst, src.ID)
		if ev.Flag.Has(event.DamageFatal) {
			s.onDeath(dst, src.ID)
		}
	}
	for i := len(waiting); i < originalLen; i++ {
		s.attacks[i] = pendingAttack{}
	}
	s.attacks = waiting
	s.attacks = append(s.attacks, counters...)
}

func (s *Scene) passiveCounterPending(defender, attacker *entity.Entity) (pendingAttack, bool) {
	if s == nil || s.skills == nil || defender == nil || attacker == nil ||
		defender.Player == nil || defender.Player.Char == nil || !defender.Alive() ||
		!attacker.Alive() || !canDealDamage(defender) {
		return pendingAttack{}, false
	}
	distance := defender.Look.AtkDist
	if distance <= 0 {
		distance = 75
	}
	if sqDist(defender.Pos, attacker.Pos) > float64(distance)*float64(distance) {
		return pendingAttack{}, false
	}
	ch := defender.Player.Char
	for skill, level := range ch.Skills {
		passive, ok := s.skills.Get(skill, level)
		if !ok || passive.Kind != domain.SkillPassive || passive.PassiveCounter == nil ||
			!ch.AllowsSkill(passive.ID, passive.Prof) {
			continue
		}
		rule := passive.PassiveCounter
		if rule.RequiredEquipType != 0 && !s.hasFunctionalEquipType(defender, rule.RequiredEquipType) {
			continue
		}
		if int32(s.rng.Intn(10000)) >= rule.ChanceBP {
			continue
		}
		return pendingAttack{
			src: defender.ID, dst: attacker.ID,
			blow:      combat.Blow{SkillPct: rule.DamagePct},
			resolveAt: s.tick + 1, reserved: true, counter: true,
		}, true
	}
	return pendingAttack{}, false
}

// onDeath 处理一次死亡。
//
// 死亡与「从视野里消失」是两件事: 死亡是**战斗结果**(要结算经验/掉落/仇恨),
// 消失是**视野事件**(只影响渲染)。所以先发 EntityDied, 尸体停留若干帧后才移除。
func (s *Scene) onDeath(dead *entity.Entity, killer domain.EntityID) {
	if dead.Monster != nil {
		s.monsterSpeak(dead, domain.MonsterSceneDie)
	} else if dead.Kind == domain.KindPlayer {
		if victor := s.monsters[killer]; victor != nil && victor.Alive() {
			s.monsterSpeak(victor, domain.MonsterSceneDefeat)
		}
	}
	s.endSharedRide(dead)
	s.clearCastWindup(dead.ID)
	if dead.Kind == domain.KindPlayer {
		s.consumeDeathDurability(dead)
	}
	// 死亡会终止本次战斗产生的状态，只保留跨死亡继续计时的经验增益。
	// 用结构化经验加成语义判断，不硬编码双倍/三倍经验状态号，避免以后新增
	// 经验卡或活动经验 Buff 时又漏进死亡清理。
	if dead.Kind == domain.KindPlayer && dead.Status != nil {
		s.statusesRemoved(dead, dead.Status.RemoveWhere(func(def domain.StatusDef) bool {
			return def.ExperienceBonusPct <= 0
		}))
	}
	s.emit(event.EntityDied{
		ID: dead.ID, Kind: dead.Kind, Killer: killer,
		AllowItemRevive:  dead.Kind == domain.KindPlayer && s.hasReviveItem(dead),
		AllowSkillRevive: dead.Kind == domain.KindPlayer,
	})
	if dead.Kind == domain.KindPlayer {
		s.recordPlayerDeathLoss(dead)
	}

	// 打它的那些怪要脱战, 否则会一直对着尸体挥空
	for _, m := range s.monsters {
		if m.Monster != nil && m.Monster.Target == dead.ID {
			s.dropTarget(m)
		}
	}

	// 打死它的人不该还锁着一个死目标 —— 不清的话下次换目标时会被当成同一场交战,
	// 累加器不重置, 首击标志也不会再出现。
	if tr := s.hits[killer]; tr != nil && tr.Target() == dead.ID {
		tr.Reset()
	}

	// 玩家死了不能像怪那样从场上抹掉 —— 那个实体还连着一条 TCP 连接。
	// 也不能在服务端起自动复活计时器：客户端收到 0x801a(alive=0)
	// 后会保持发黑跪地的死亡态并显示复活选择，只有它随后发来的
	// 0x1018 才有权让角色回城站起。
	if dead.Kind == domain.KindPlayer {
		dead.Player.StopResting()
		s.onPetOwnerDeath(dead)
		return
	}

	// 临时召唤物不代表刷怪点产出，不提供经验、任务进度或掉落。
	if dead.Monster == nil || dead.Monster.Summoner == 0 {
		// 结算击杀回报：任务专属收集品直入背包；普通掉落仍生成地面实体。
		s.recordQuestKill(dead, killer)
		s.grantQuestDrops(dead, killer)
		s.awardKill(dead, killer)
		s.rollLoot(dead, killer)
		s.requestFirstKillTip(dead, killer)
		s.advanceDungeonEncounter(dead)
		s.updateTrialClear()
	}

	id := dead.ID
	s.timer.after(s.tick, corpseTicks, func() {
		e, ok := s.entities[id]
		if !ok || e.Alive() {
			return // 已经被移走, 或者已经复活了
		}
		s.emit(event.EntityDespawned{ID: id, Kind: e.Kind, Reason: event.DespawnDeath})
		s.flush() // 立刻冲刷: 下面就把它摘掉了, 留到帧末就发不出去
		s.aoi.Leave(e)
		delete(s.entities, id)
		delete(s.players, id)
		delete(s.monsters, id)
		delete(s.hits, id)
		s.scheduleRespawn(e, true)
	})
}

func (s *Scene) hasReviveItem(p *entity.Entity) bool {
	_, _, ok := s.reviveItem(p)
	return ok
}

// scheduleRespawn 尸体清掉之后, 把那个刷怪点排进重生队列。
//
// 计时从**尸体消失**算起而不是从死亡算起, 这样"尸体停留 + 重生间隔"是相加的,
// 玩家不会看到尸体旁边突然冒出一只活的同款。
func (s *Scene) scheduleRespawn(dead *entity.Entity, recordKill bool) {
	if s.trial.IsMap(s.id.MapID) {
		return
	}
	if s.spawner == nil || dead.Monster == nil || dead.Monster.SpawnID == 0 || dead.Monster.Summoner != 0 {
		return
	}
	point := dead.Monster.SpawnID
	s.spawner.Release(point)
	// 副本怪只在实例创建时铺一次。死亡或技能驱散后永久离场，
	// 不进野外场景的重生定时器，也不累计下一只光圈精英的概率。
	if s.dungeon != nil {
		return
	}
	if recordKill {
		s.spawner.RecordKill(point)
	}

	d := spawn.RespawnTicks(dead.Monster.Kind)
	if d == 0 {
		return // 摆设与采集物不重生
	}
	s.timer.after(s.tick, d, func() {
		e := s.spawner.Respawn(point, uint8(s.rng.Intn(100)))
		if e == nil {
			return // 点位被占了, 或者场景重建过
		}
		s.initializeMonster(e)
		s.entities[e.ID] = e
		s.monsters[e.ID] = e
		s.aoi.Enter(e)
		// 0x800f 没有另一条“刷新包”；同一个 elite 字节的最高位是客户端
		// MonsterEntity.eliteFresh。只在这次重生广播置位，触发原生刷新表现。
		appeared := s.spawnEvent(e)
		appeared.Fresh = true
		s.emit(appeared)
		s.monsterSpeak(e, domain.MonsterSceneNormal)
	})
}

// ── 事件产出 ──

const attributeArraySlots = 37

const (
	// 正式客户端 0x8007 handler 的直接落点：attrs[25..28] 依次写入
	// CharData.nianli/maxNianli/naili/maxNaili。
	clientAttrNianli    = 25
	clientAttrMaxNianli = 26
	clientAttrNaili     = 27
	clientAttrMaxNaili  = 28
	clientAttrCritRate  = 35
	clientAttrMCritRate = 36
)

// attributeSnapshot 组装客户端可消费的权威角色属性状态。
//
// 0x8007 的数组下标与装备词条 attr_id 是两套完全不同的编号。这里只覆盖
// 客户端 EquipDlg 已经静态闭合的下标；其余位置继续继承角色兼容数组，绝不
// 把 domain.Stats 的字段顺序直接抄进去。原服穿戴前后的真实 0x8007 证明：
// Current 与 Total 都要写入装备/状态结算后的六维和战斗值；两者的区别是
// 当前 HP/MP 与其上限等可消耗状态，不是“裸装/穿装”两层。
func (s *Scene) attributeSnapshot(e *entity.Entity) event.StatsChanged {
	ch := e.Player.Char
	baseMoveSpeed := characterBaseMoveSpeed(ch)
	n := attributeArraySlots
	if len(ch.Attrs) > n {
		n = len(ch.Attrs)
	}
	current := make([]int32, n)
	copy(current, ch.Attrs)
	current[0] = ch.Level
	current[1] = nonnegativeI32(ch.Exp)
	if s.levels != nil {
		current[2] = nonnegativeI32(s.levels.Need(ch.Level))
	}
	base := ch.EffectiveBase()
	finalBase, _ := domain.Compute(domain.StatSource{
		Base: base, Level: ch.Level, Innate: domain.Stats{MoveSpeed: baseMoveSpeed},
		Worn: s.functionalWorn(e), Defs: s.itemDef, Status: e.Status,
	})
	writeKnownClientAttributes(current, finalBase, e.Stats)
	current[24] = ch.FreePoints
	current[clientAttrNianli] = ch.EffectiveNianli()
	current[clientAttrMaxNianli] = domain.DefaultNianli
	current[clientAttrNaili] = ch.Stamina
	current[clientAttrMaxNaili] = s.stamina.Max
	current[3], current[4] = e.HP, e.MaxHP
	current[5], current[6] = e.MP, e.MaxMP
	total := append([]int32(nil), current...)
	total[3] = e.MaxHP
	total[4] = e.MaxHP
	total[5] = e.MaxMP
	total[6] = e.MaxMP

	// 0x8003/0x8004 共用 Character.Attrs；同步这一份，保证下次进图与选角页
	// 看到的等级/经验/HP/MP不再停留在旧抓包值。
	ch.Attrs = append([]int32(nil), current...)
	// attrs 同时承担历史兼容存档和当前下行快照。存档必须留基础移速，否则
	// Buff 期间的 150 会成为下一次重算基线，状态到期后也降不回 100。
	ch.Attrs[9] = baseMoveSpeed
	// 0x8012 只有骑乘状态和坐骑模型，不带速度。本人的实际移动仍由
	// 0x8007 的 A_MOVESPEED 驱动，因此骑乘期间要把服务端算出的像素速度
	// 投影回客户端属性单位。客户端换算系数是 2，奇数像素速度向下取整，
	// 保证客户端不会比服务端权威上限快 1px/s。这只改下行副本，不污染存档。
	if e.Player.Riding {
		moveAttr := s.playerMoveSpeedPX(e) / clientPixelsPerMoveSpeed
		if moveAttr > 0 {
			current[9], total[9] = moveAttr, moveAttr
		}
	}
	return event.StatsChanged{Who: e.ID, Current: current, Total: total}
}

func characterBaseMoveSpeed(ch *domain.Character) int32 {
	if ch != nil && len(ch.Attrs) > 9 && ch.Attrs[9] > 0 {
		return ch.Attrs[9]
	}
	return domain.DefaultPlayerMoveSpeed
}

func characterInnateStats(ch *domain.Character) domain.Stats {
	return domain.Stats{MoveSpeed: characterBaseMoveSpeed(ch)}
}

// writeKnownClientAttributes 只写客户端常量已经证明的属性数组位置：
// A_ATK=7、A_DEF=8、A_MOVESPEED=9、六维=10..14/21、A_MATK=15、A_MDEF=16、
// A_HIT=17、A_MINATK=34、物理/魔法暴击率=35/36。7 是攻击上限，34 是攻击下限。
// A_PTS=24 是角色剩余点数，由 attributeSnapshot 直接写 Character.FreePoints。
func writeKnownClientAttributes(dst []int32, base domain.Base, stats domain.Stats) {
	dst[7] = stats.MaxAtk
	dst[8] = stats.Def
	dst[9] = stats.MoveSpeed
	dst[10] = base.STR
	dst[11] = base.VIT
	dst[12] = base.AGI
	dst[13] = base.INT
	dst[14] = base.SPI
	dst[15] = stats.MAtk
	dst[16] = stats.MDef
	dst[17] = stats.Hit
	dst[21] = base.DEX
	dst[34] = stats.MinAtk
	dst[33] = stats.PlayerAttackSpeedPercent()
	// 实机已确认客户端面板仍接收万分比，并在显示层自行除以100：
	// 下发1500显示15%；若服务端先除会错误显示0.15%。
	dst[clientAttrCritRate] = stats.CritRate
	dst[clientAttrMCritRate] = stats.MCritRate
}

func (s *Scene) emitAttributesIfPlayer(e *entity.Entity) {
	if e != nil && e.Kind == domain.KindPlayer && e.Player != nil {
		s.emitTo(e.ID, s.attributeSnapshot(e))
	}
}

// spawnEvent 生成某实体在**当前帧**的公开出场视图。
//
// 掉落保护必须在这里算剩余时间：同一件东西刚掉出来时是 30 秒，玩家晚些走进
// 视野时只能收到当时尚余的时间。把原始 OwnerUntil 交给协议层会泄漏逻辑帧模型，
// 把固定 30 秒塞进 Entity.Spawned 又会让后来者重新等满一轮。
func (s *Scene) spawnEvent(e *entity.Entity) event.EntitySpawned {
	ev := e.Spawned()
	if e.Player != nil && e.Player.Stall != nil && !e.Player.StallBusy {
		ev.Stall = &event.StallOwnerChanged{Owner: e.ID, Type: e.Player.Stall.Type, Name: e.Player.Stall.Name}
	}
	if e.Kind == domain.KindPlayer {
		ev.Invisible = entityInvisible(e)
		ev.RideAnchor, ev.RideSeat, ev.RideSeats = e.Player.RideAnchor, e.Player.RideSeat, s.rideSeats(e)
	}
	for _, st := range s.visibleStatuses(e) {
		ev.StatusIcons = append(ev.StatusIcons, st.icon)
	}
	if e.Kind != domain.KindDrop || e.Drop == nil {
		return ev
	}
	var protectionMS int64
	if e.Drop.Owner != 0 && s.tick < e.Drop.OwnerUntil {
		protectionMS = (e.Drop.OwnerUntil - s.tick).Millis()
	}
	const maxI32 = int64(1<<31 - 1)
	if protectionMS > maxI32 {
		protectionMS = maxI32
	}
	quality := uint8(0)
	if def, ok := s.items[e.Drop.Item]; ok {
		quality = equipmentQuality(def, e.Drop.Stack)
	}
	ev.Ground = &event.GroundItemView{
		Item: e.Drop.Item, Count: e.Drop.Count, Owner: e.Drop.Owner,
		ProtectionMS: int32(protectionMS), Quality: quality,
	}
	return ev
}

func nonnegativeI32(v int64) int32 {
	if v <= 0 {
		return 0
	}
	const max = int64(1<<31 - 1)
	if v > max {
		return int32(max)
	}
	return int32(v)
}

// emit 把事件广播给同地图所有玩家(含主体本人)。客户端实体生命周期以整张地图
// 为边界；AOI 只保留为服务端索敌、技能范围等空间查询索引。
func (s *Scene) emit(ev event.Event) {
	// 致死伤害的气泡先于客户端死亡表现入队；尸体虽仍短暂停在场上，
	// 若在 PlayDie 后才下发台词，客户端可能已隐藏其头顶节点。
	if hit, ok := ev.(event.DamageDealt); ok && hit.Flag.Has(event.DamageFatal) {
		if target := s.entities[hit.Dst]; target != nil {
			if target.Monster != nil {
				s.monsterSpeak(target, domain.MonsterSceneDie)
			} else if target.Kind == domain.KindPlayer {
				if victor := s.monsters[hit.Src]; victor != nil && victor.Alive() {
					s.monsterSpeak(victor, domain.MonsterSceneDefeat)
				}
			}
		}
	}
	// Only the attacker needs intermediate monster combat results. The genuine
	// lethal result remains a broadcast because it starts the native death animation.
	if hit, ok := ev.(event.DamageDealt); ok && !hit.Flag.Has(event.DamageFatal) {
		if target := s.entities[hit.Dst]; target != nil && target.Kind == domain.KindMonster {
			s.emitTo(hit.Src, ev)
			if hit.Amount > 0 && !hit.Flag.Has(event.DamageMiss) {
				s.monsterSpeak(target, domain.MonsterSceneBeHit)
			}
			return
		}
	}
	s.push(ev, 0, 0)
}

// emitExcept 广播但跳过某人。移动事件用它跳过移动者本人。
func (s *Scene) emitExcept(ev event.Event, except domain.EntityID) { s.push(ev, 0, except) }

// emitTo 只发给一个玩家。经验、属性变化、拒绝回执走这条。
func (s *Scene) emitTo(to domain.EntityID, ev event.Event) {
	s.outbox = append(s.outbox, pending{ev: ev, to: to})
}

func (s *Scene) push(ev event.Event, to, except domain.EntityID) {
	s.outbox = append(s.outbox, pending{ev: ev, to: to, except: except})
}

// flush 把本帧攒下的事件送出去。
//
// 攒到帧末再发, 而不是产生时立刻发, 有两个理由: 一是同一帧的事件到达客户端的
// 顺序确定; 二是将来要做移动合并/包合并时, 这里是唯一的下手点。
func (s *Scene) flush() {
	if len(s.outbox) == 0 {
		return
	}
	for _, p := range s.outbox {
		if p.to != 0 {
			if e := s.players[p.to]; e != nil && s.canDeliverWorldEvent(p.to, p.ev) {
				e.Player.Sink.Emit(p.ev)
			}
			continue
		}
		for _, o := range s.players {
			if o.ID != p.except && s.canDeliverWorldEvent(o.ID, p.ev) {
				o.Player.Sink.Emit(p.ev)
			}
		}
	}
	// 复用底层数组: 这个切片每帧都要重建, 不复用就是每帧一次分配 × 场景数
	s.outbox = s.outbox[:0]
}

// ── 存档 ──

// saveDirty 把有变更的在线角色交给存档队列。
//
// 注意是**交给队列**, 不是写库 —— Saver 的实现必须立即返回。
// 这里传的是克隆, 因为一旦把 *Character 交出去, 场景下一帧改它就和写库的 goroutine 打架了。
func (s *Scene) saveDirty() {
	if s.saver == nil {
		return
	}
	n := 0
	for _, e := range s.players {
		if !e.Player.TradeBusy && e.Player.TakeDirty() {
			s.saver.Save(s.snapshotOf(e))
			n++
		}
	}
	if n > 0 {
		s.log.Debug("批量存档", "tick", s.tick, "count", n)
	}
}

// snapshotOf 给一个玩家做存档快照。
//
// **必须是克隆**: 交出去之后场景下一帧还会继续改角色和背包,
// 直接传指针就是让写库的 goroutine 和场景 goroutine 读写同一份数据。
func (s *Scene) snapshotOf(e *entity.Entity) domain.Snapshot {
	snap := domain.Snapshot{Char: e.Player.Char, Bag: e.Player.Bag, Worn: e.Player.Worn,
		ChangeSet: e.Player.ChangeSet, Warehouse: e.Player.Warehouse,
		Wardrobe: e.Player.Wardrobe, Stall: e.Player.Stall}.Clone()
	snap.Char.SavedStatuses = domain.CloneSavedStatuses(e.Status.SaveForLogout(s.tick, time.Now(), e.ID))
	e.Player.SaveExperienceBoost(snap.Char, s.tick)
	// 出战宠物的 HP/MP 以场景实体为准。周期存档和关服快照都可能发生在宠物
	// 尚未摘除时，必须把运行值投影到克隆，且不能为此修改仍在推进的场景对象。
	if e.Player.Pet != 0 {
		if pet := s.entities[e.Player.Pet]; pet != nil && pet.Pet != nil && pet.Pet.Inst != nil {
			if inst := snap.Char.FindPet(pet.Pet.Inst.ID); inst != nil {
				inst.HP, inst.MP = pet.HP, pet.MP
				inst.Deployed = true
				inst.Riding = e.Player.Riding
			}
		}
	}
	// **位置以实体为准。**
	//
	// 玩家走路只动 AOI 里的实体(e.Pos), 角色记录(Char.Pos)在整个在线期间
	// 一动不动 —— 全服没有任何一处往 Char.Pos 写。不在这儿抄一下的话,
	// 存进库的永远是进图时那个坐标: 玩家走一整天, 下线再上线还站在原地。
	//
	// 而且这个 bug **完全不报错**: 存档确实跑了(日志里"批量存档 count=1"),
	// 只是存的是旧值。实测就是靠"客户端走到 9300, 库里一直是 9000"发现的。
	snap.Char.Pos, snap.Char.SceneInstance = s.persistedPosition(e)
	return snap
}

// shutdown 关场景: 无条件存全部在线角色, 再断开他们的连接。
//
// 顺序不能反 —— 先断连接的话, 会话层会以为是玩家掉线, 走另一套清理路径。
func (s *Scene) shutdown(reason string) {
	// WriteBack stays alive until all scenes have drained. Resolve each accepted
	// pair before saving final snapshots, even after normal Post has been closed.
	s.finishReadyTrades(true)
	s.log.Info("场景停止", "reason", reason, "tick", s.tick, "在线", len(s.players))
	for _, e := range s.players {
		s.closeStallForExit(e)
		e.Player.TakeDirty()
		saver := e.Player.FinalSaver
		if saver == nil {
			saver = s.saver
		}
		if saver != nil {
			saver.Save(s.snapshotOf(e))
		}
	}
	for _, e := range s.players {
		e.Player.Sink.Close()
	}
	s.entities = map[domain.EntityID]*entity.Entity{}
	s.players = map[domain.EntityID]*entity.Entity{}
	s.monsters = map[domain.EntityID]*entity.Entity{}
	s.ground = map[domain.EntityID]*entity.Entity{}
}

// ── 只读窥视(仅测试与 GM) ──

// Tick 返回当前帧号。只能在 Inspect 回调里调。
func (s *Scene) Tick() domain.Tick { return s.tick }

// PlayerCount 返回在线玩家数。只能在 Inspect 回调里调。
func (s *Scene) PlayerCount() int { return len(s.players) }

// EntityAt 按 id 取实体。只能在 Inspect 回调里调, 且**不许把返回的指针存到外面**。
func (s *Scene) EntityAt(id domain.EntityID) *entity.Entity { return s.entities[id] }
