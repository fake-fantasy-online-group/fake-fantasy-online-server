// gameserver —— 区服主程序。
//
// 这个文件**只做装配**: 读配置、连库、把各层接起来、按正确顺序关服。
// 一行业务逻辑都不该出现在这里 —— 见 docs/架构/10-分层规范.md 第六节。
//
// 装配顺序与关服顺序是对称的, 且关服顺序不能乱:
//
//	起: 存储 → 写回队列 → 场景路由 → 网关
//	关: 网关停止接客 → 场景存档退出 → 写回队列排空
//
// 反过来关就会丢档: 场景还在往队列里写, 队列已经关了。
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/data"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/online"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/party"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/social"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/trade"
	gnet "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/net"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/session"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/wire"
)

var (
	addr     = flag.String("addr", ":19000", "监听地址")
	privKey  = flag.String("priv", "internal/crypto/keys/server_private.pem", "服务端 RSA 私钥(自解握手)")
	seedFile = flag.String("seed", "", "可选: 初始角色种子(JSON), 供自测导入角色")
	dsn      = flag.String("dsn", os.Getenv("DATABASE_URL"), "PostgreSQL DSN(留空则用内存存储, 仅自测)")
	saveSec  = flag.Int("save", 30, "脏数据批量落盘间隔(秒)")
	drainSec = flag.Int("drain", 30, "关服时等待场景与写回队列的上限(秒)")
	debugLog = flag.Bool("debug", false, "打开调试日志")
	noNPC    = flag.Bool("no-npc", false, "不下发 NPC 初始场景数据")
)

func main() {
	os.Exit(run())
}

func run() int {
	flag.Parse()
	lvl := slog.LevelInfo
	if *debugLog {
		lvl = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── 存储: 有 DSN 用 PostgreSQL(生产), 否则内存(仅自测) ──
	var st store.Store
	if *dsn != "" {
		pg, err := store.Open(ctx, *dsn)
		if err != nil {
			log.Error("连接 Postgres 失败", "err", err)
			return 1
		}
		st = pg
		log.Info("存储: PostgreSQL", "dsn", maskDSN(*dsn))
	} else {
		st = store.NewMemory()
		log.Warn("存储: 内存(重启丢档, 仅自测)。生产请用 -dsn")
	}
	defer st.Close()
	seedChars(ctx, st, log)

	// ── 写回队列: 场景与数据库之间的缓冲, 保证场景 goroutine 永不等 IO ──
	wb := store.NewWriteBack(st, time.Second, log)
	// 全服只建一个角色 gate：租约从 Enter 前持续到离场最终快照成功提交。
	// 若每连接各建一份，新 TCP 秒连仍会绕过旧连接的写回屏障。
	charGate := store.NewCharacterGate(wb)
	// 写回循环不能共用网关的 signal context。收到停服信号后，场景会在
	// Router.Drain 中才交出最后一批在线存档；若 wb 也同时因 ctx.Done 退出，
	// 这批迟到快照就只会留在 pending，服务端重启后丢档。wb 的生命周期只由
	// 下面的 Close 结束，从而严格保证 gateway → router drain → writeback close。
	go wb.Run(context.Background())

	// ── 静态配置: 怪物模板/刷怪点/传送门, 开服一次性读进内存, 之后只读 ──
	parties := party.NewRegistry()
	onlineIdx := online.NewRegistry()
	world := loadWorldData(ctx, st, log)
	var staticQ data.Querier
	var mapCatalog *data.MapCatalog
	if pg, ok := st.(*store.Postgres); ok {
		staticQ = pg.ReadOnly()
		var err error
		mapCatalog, err = data.LoadMapCatalog(ctx, staticQ)
		if err == nil && mapCatalog.Len() == 0 {
			err = fmt.Errorf("map_defs 为空")
		}
		if err == nil {
			var stats data.NPCGreetingStats
			stats, err = data.AttachNPCGreetings(ctx, staticQ, mapCatalog, world.npcs)
			if err == nil {
				log.Info("NPC 专属问候就绪", "地图", stats.Maps, "NPC", stats.NPCs,
					"原版无条件问候", stats.Matched, "缺少原始落位", stats.NoSource)
			}
		}
		if err != nil {
			log.Error("加载地图运行资源失败，拒绝启动", "err", err)
			if closeErr := wb.Close(time.Second); closeErr != nil {
				log.Error("启动中止后关闭写回队列失败", "err", closeErr)
			}
			return 1
		}
	}
	if *dsn != "" && world.monsterDialogs == nil {
		log.Error("怪物台词目录未就绪，拒绝启动")
		if err := wb.Close(time.Second); err != nil {
			log.Error("启动中止后关闭写回队列失败", "err", err)
		}
		return 1
	}
	if *dsn != "" && (world.announcements.Speed <= 0 || world.pickupRange <= 0 || world.rideInviteSeconds <= 0 || world.petRule.MaxLifeSkills <= 0 || world.petRule.MaxFightSkills <= 0 || world.trial == nil || len(world.petSkills) == 0 || len(world.refines) == 0) {
		log.Error("玩法配置未就绪，拒绝启动")
		_ = wb.Close(time.Second)
		return 1
	}
	if *dsn != "" && len(world.mapReturns) == 0 {
		log.Error("地图回城配置未就绪，拒绝以错误回城目标启动")
		if err := wb.Close(time.Second); err != nil {
			log.Error("启动中止后关闭写回队列失败", "err", err)
		}
		return 1
	}
	if *dsn != "" && (len(world.dungeons) < 11 || dungeonGroupCount(world.dungeons) < 7 ||
		dungeonEntranceCount(world.transports) < 9 ||
		dungeonPortalCount(world.portals) < 1 || len(world.dungeonEncounters) < 3) {
		log.Error("第一版副本定义或客户端入口未就绪，拒绝以不可达副本状态启动",
			"副本", dungeonGroupCount(world.dungeons), "副本地图", len(world.dungeons),
			"NPC入口", dungeonEntranceCount(world.transports),
			"传送门入口", dungeonPortalCount(world.portals),
			"首领阶段", len(world.dungeonEncounters))
		if err := wb.Close(time.Second); err != nil {
			log.Error("启动中止后关闭写回队列失败", "err", err)
		}
		return 1
	}
	if *dsn != "" && (!world.equipmentRoll.Enabled() || world.colorProfileErr != nil) {
		log.Error("装备随机属性矩阵未就绪或染色档位配置错误，拒绝以全白板掉落状态启动",
			"err", world.colorProfileErr)
		if err := wb.Close(time.Second); err != nil {
			log.Error("启动中止后关闭写回队列失败", "err", err)
		}
		return 1
	}
	// Router 在写锁内串行调用 Builder，因此这份按静态地图号共享的懒加载缓存
	// 不需要额外锁；副本实例也不会重复吃一份 1~2MB 的碰撞网格。
	maskCache := make(map[int32]*data.CollisionMask)
	maskTried := make(map[int32]bool)
	// ── 场景路由: 按需建场景。371 张图不会全部常驻, 有人进才起 ──
	saveEvery := domain.Ticks(*saveSec * 1000)
	var router *scene.Router // Builder 里要用 router.Alloc(), 而它总在 router 赋值之后才被调用
	router = scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
		// 客户端把幻想小岛的线上地图号写成 60, 静态资源表却按内部
		// pworld 号 21000 索引。场景/存档/协议一律继续使用 60；只有
		// 装配静态资源时翻译一次，避免把资源内部编号泄漏进游戏状态。
		dataMapID := staticMapID(id.MapID)
		var returnAt scene.ReviveAt
		if pos, ok := world.mapReturns[dataMapID]; ok {
			// 幻想小岛资源号 21000 在正式协议中使用 60。
			if pos.MapID == 21000 {
				pos.MapID = 60
			}
			returnAt = scene.ReviveAt{Scene: domain.SceneID{MapID: pos.MapID}, Pos: pos}
		} else if *dsn != "" {
			return nil, fmt.Errorf("地图 %d 缺少回城配置", id.MapID)
		}
		var mask *data.CollisionMask
		if !maskTried[dataMapID] {
			if mapCatalog != nil && staticQ != nil {
				if loaded, err := mapCatalog.CollisionOfMap(ctx, staticQ, dataMapID); err != nil {
					if errors.Is(err, data.ErrCollisionMaskMissing) {
						maskTried[dataMapID] = true
						log.Warn("地图未配置碰撞图，本图怪物使用受控直线移动", "map", id.MapID,
							"dataMap", dataMapID)
					} else if errors.Is(err, data.ErrCollisionMaskNotConfigured) {
						return nil, fmt.Errorf("地图 %d 缺少碰撞图配置: %w", id.MapID, err)
					} else {
						return nil, fmt.Errorf("加载地图 %d 碰撞图失败: %w", id.MapID, err)
					}
				} else {
					maskCache[dataMapID] = loaded
					maskTried[dataMapID] = true
				}
			}
		}
		mask = maskCache[dataMapID]
		var walkable func(domain.Pos) bool // 缺失碰撞图时按已确认规则退化为直线移动
		if mask != nil {
			walkable = mask.Walkable
		}
		cfg := scene.Config{Trial: world.trial,
			DungeonParty: func(char domain.CharID) domain.DungeonParty {
				p, ok := parties.Get(parties.PartyOf(char))
				if !ok {
					return domain.DungeonParty{}
				}
				out := domain.DungeonParty{ID: p.ID, Leader: p.Leader}
				for _, member := range p.Members {
					out.Members = append(out.Members, member.Name)
					if loc, ok := onlineIdx.Find(member.Char); ok && loc.Profile.Level > out.MaxLevel {
						out.MaxLevel = loc.Profile.Level
					}
				}
				return out
			},
			DungeonNotify: func(name, text string) {
				if _, ctrl, ok := onlineIdx.ControlByName(name); ok {
					ctrl.Notify(text)
				}
			},
			ID: id, Saver: wb, SaveEvery: saveEvery, Log: log,
			Home:           returnAt,
			Revive:         returnAt,
			CombatTiming:   world.combatTiming,
			MonsterDialogs: world.monsterDialogs,
			Portals:        remapPortals(world.portals[dataMapID], dataMapID, id.MapID), Transports: world.transports, Shops: world.shops, Levels: world.levels, Defs: world.monsters,
			Walkable: walkable,
			Items:    world.items, Loot: world.loot, ItemIcons: world.itemIcons,
			EquipmentRoll: world.equipmentRoll, BossFixedDrops: world.bossFixedDrops,
			ItemIconsKnown: world.itemIconsKnown,
			Skills:         world.skills, Statuses: world.statuses,
			StatusOverlay: world.statusOverlay,
			Quests:        world.quests, NPCs: npcSpawns(world.npcs[dataMapID], id.MapID),
			Dungeons: world.dungeons, DungeonEncounters: world.dungeonEncounters,
			Works: world.works, Stamina: world.stamina,
			NianliCreation: world.nianliCreation,
			Crafts:         world.crafts, Refines: world.refines, Sockets: world.sockets, Washes: world.washes,
			WarehouseRule:     world.warehouseRule,
			Wardrobes:         world.wardrobes,
			WardrobeRule:      world.wardrobeRule,
			HairRules:         world.hairRules,
			StallOpenItems:    world.stallOpenItems,
			Rack:              world.rack,
			DepositRule:       world.depositRule,
			RequireWorkGather: true, Rest: world.rest,
			Death:   world.death,
			Penalty: world.penalty,
			Pets:    world.pets, PetLevels: world.petLvls, PetFoods: world.petFoods,
			PetPrefixes: world.petPrefixes, PetSkills: world.petSkills,
			PetLearningItems: world.petLearnItems, PetRule: world.petRule, RideInviteSeconds: world.rideInviteSeconds, PickupRange: world.pickupRange,
			SkipNPCSpawn: *noNPC,
			// **必须在这里给** —— placeNPCs 是在 scene.New() 里跑的, 而 Router
			// 回填 alloc 是在 New() 之后。不给的话 NPC 一个都落不下去,
			// 而且是静默的(实测: 龙城 198 个 NPC 一个都没出现)。
			Alloc: router.Alloc()}
		// 副本禁止被任何遗漏的调用路径建成公共场景。
		if _, dungeon := world.dungeons[id.MapID]; dungeon && id.Persistent() {
			return nil, fmt.Errorf("副本地图 %d 不能创建公共场景", id.MapID)
		}
		// 非零实例号 = 这是一个副本实例, 会限时、会自己关
		if !id.Persistent() {
			if d, ok := world.dungeons[id.MapID]; ok {
				cfg.Dungeon = &d
			}
		}
		// 有刷怪点的图才配刷怪器; 主城之类没有点位的图 Spawner 留 nil。
		if pts := remapSpawns(world.spawns[dataMapID], id.MapID); len(pts) > 0 && !world.trial.IsMap(id.MapID) {
			sp, skipped := spawn.New(pts, world.monsters, router.Alloc())
			if skipped > 0 {
				log.Warn("刷怪点引用不到怪物模板", "map", id.MapID, "丢弃", skipped)
			}
			cfg.Spawner = sp
		}
		return scene.New(cfg), nil
	}, log)
	// 场景不能直接共用进程信号 ctx。SIGINT 首先只应让网关停止接入并
	// 关闭现有连接；每个 Session.OnClose 还要向活着的场景投 Leave。若场景也
	// 同时收到 ctx.Done，会先于 OnClose 退出，真实关服时就出现“离场命令未投递”。
	// Router 的唯一停止入口是下方 Drain，由主程序严格按
	// gateway drained → scenes drained → writeback drained 的顺序调用。
	router.Start(context.Background())

	// ── RSA 握手解密 ──
	pemData, err := os.ReadFile(*privKey)
	if err != nil {
		log.Error("读私钥失败", "path", *privKey, "err", err)
		return 1
	}
	// 全服共用一个未知 opcode 观测器 —— 每个连接各记一份的话,
	// 玩家一断线就什么都没了, 而我们要的正是"整局见过哪些号"
	unknownOps := protocol.NewUnknownLog()
	// 账号在线表全服一份：同账号后登录成功后原子顶替并断开旧连接。
	accountSessions := session.NewAccountSessions()
	// 待处理的好友请求。不落盘 —— 加好友是当面同意的事
	friendReqs := social.NewRequests()
	trades := trade.NewRegistry()

	hsDec, err := wire.NewHandshakeDecryptor(pemData)
	if err != nil {
		log.Error("构造握手解密器失败", "err", err)
		return 1
	}
	log.Info("握手解密器已就绪")

	// ── 网关: 每连接建一个会话 ──
	//
	// NPC 不再由会话下发。它们是**场景里的真实体**(走 0x8046, 用场景分配的实体 id),
	// 玩家进图时由场景一次性发全图。以前那条"客户端请求场景加载 → 会话回一份
	// 静态 NPC 包"的路已经删掉 —— 那个请求号(0x1017)其实是移动上报, 从来就不是
	// 场景加载, 而且会话编的 2000000+ 实体 id 与场景里的对不上, 客户端点了没反应。
	factory := func(sink gnet.Sink) gnet.Handler {
		return session.New(sink, session.Deps{
			Store: st, Router: router, AccountSessions: accountSessions, CharGate: charGate,
			Items: world.items, HelpTopics: world.helpTopics, HelpTopicsKnown: world.helpTopicsKnown,
			WarehouseRule: world.warehouseRule, WardrobeRule: world.wardrobeRule,
			MailRule: world.mailRule, WriteBack: wb,
			ChatRules: world.chatRules, Announcements: world.announcements,
			Handshake: hsDec,
			SpawnPos:  spawnPos,
			Dungeons:  world.dungeons,
			GMLanding: func(mapID int32) (domain.Pos, bool) {
				return gmLandingPos(world, mapCatalog, mapID)
			},
			Unknown: unknownOps, Party: parties, Online: onlineIdx,
			Requests: friendReqs, Trades: trades,
			Log: log,
		})
	}
	srv := gnet.NewServer(*addr, factory, gnet.Options{Log: log})

	log.Info("区服启动", "addr", *addr, "帧长ms", domain.TickMS,
		"已接入的上行协议", len(protocol.ProvenOps()))
	_ = onlineIdx          // 索引由会话在进图/传送/登出时维护
	runErr := srv.Run(ctx) // 收到信号后 Run 返回, 网关已停止接客
	if runErr != nil {
		log.Error("网关退出", "err", runErr)
	}

	// 关服时记录当前进程遇到的未知 opcode。
	dumpUnknownOps(log, unknownOps)

	// ── 关服: 顺序不能改 ──
	drain := time.Duration(*drainSec) * time.Second
	sceneDrainErr := router.Drain(drain) // 1. 场景把在线角色排进写回队列, 然后退出
	if sceneDrainErr != nil {
		log.Error("区服关闭失败: 场景未全部停止", "err", sceneDrainErr)
	}
	// 场景超时时仍尝试落下已经交出的快照，但这只是最大限度降低损失；
	// 因为未停场景仍可能迟到入队，本次关服整体依然必须失败。
	writeBackErr := wb.Close(drain) // 2. 队列排空, 真正写进数据库
	if writeBackErr != nil {
		log.Error("区服关闭失败: 存档未全部落盘", "err", writeBackErr)
	}
	if runErr != nil || sceneDrainErr != nil || writeBackErr != nil {
		// 场景或存档未排空时，关闭过程没有完成。
		log.Error("区服未正常关闭",
			"运行错误", runErr, "场景排空错误", sceneDrainErr, "存档排空错误", writeBackErr)
		return 1
	}
	log.Info("区服关闭")
	return 0
}

// dumpUnknownOps 把这一局见到的未知上行 opcode 打成一张表。
//
// 定长包(MinLen == MaxLen)是很强的线索: 说明载荷里没有变长字符串,
// 长度本身就能筛掉一大半候选 —— 比如 4 字节多半是单个 id, 12 字节多半是三个 i32。
func dumpUnknownOps(log *slog.Logger, u *protocol.UnknownLog) {
	stats := u.Snapshot()
	if len(stats) == 0 {
		return
	}
	log.Info("本局见到的未接入上行协议", "种数", len(stats))
	for _, st := range stats {
		fixed := ""
		if st.MinLen == st.MaxLen {
			fixed = " (定长)"
		}
		log.Info(fmt.Sprintf("  0x%04x", uint16(st.Op)),
			"次数", st.Count,
			"载荷长度", fmt.Sprintf("%d~%d%s", st.MinLen, st.MaxLen, fixed),
			"样本摘要", protocol.UnknownSampleFingerprint(st.Sample))
	}
}

func npcSpawns(list []data.NPCPlacement, mapID int32) []domain.NPCSpawn {
	out := make([]domain.NPCSpawn, 0, len(list))
	for _, n := range list {
		pos := n.Pos
		pos.MapID = mapID
		role := domain.RoleOfNPC(n.Name, n.Sell, n.Trans)
		greeting := n.Greeting
		if greeting == "" {
			greeting = domain.DefaultNPCGreeting(role)
		}
		out = append(out, domain.NPCSpawn{
			Name: n.Name, Sprite: n.Sprite, Pos: pos, Dir: n.Dir, Portrait: n.Portrait,
			Script: n.Script,
			Sell:   n.Sell, Trans: n.Trans, Role: role, Greeting: greeting})
	}
	return out
}

// staticMapID 把协议/存档使用的线上地图号翻译成静态资源索引。
//
// 幻想小岛是目前唯一有实证的别名：真实建角/SavePos 都使用 60；客户端
// pworld.def 的 21000/22000/23000 则共同引用 qw0060。这里选 21000 作为
// 同一份静态资源的规范索引，不改变线上地图号。
func staticMapID(mapID int32) int32 {
	if mapID == 60 {
		return 21000
	}
	return mapID
}

func remapSpawns(list []domain.SpawnPoint, mapID int32) []domain.SpawnPoint {
	if len(list) == 0 {
		return nil
	}
	out := make([]domain.SpawnPoint, len(list))
	copy(out, list)
	for i := range out {
		out[i].Pos.MapID = mapID
	}
	return out
}

func remapPortals(list []domain.Portal, dataMapID, mapID int32) []domain.Portal {
	if len(list) == 0 {
		return nil
	}
	out := make([]domain.Portal, 0, len(list))
	for _, p := range list {
		t := p.Teleport
		t.Area = append(domain.Polygon(nil), p.Area...)
		for i := range t.Area {
			if t.Area[i].MapID == dataMapID {
				t.Area[i].MapID = mapID
			}
		}
		if t.To.MapID == dataMapID {
			t.To.MapID = mapID
		}
		if t.Back.MapID == dataMapID {
			t.Back.MapID = mapID
		}
		out = append(out, domain.NewPortal(t))
	}
	return out
}

// maskDSN 隐去 DSN 里的密码用于日志。
func maskDSN(dsn string) string {
	if i := strings.Index(dsn, "@"); i > 0 {
		if j := strings.Index(dsn, "://"); j > 0 && j+3 < i {
			return dsn[:j+3] + "***@" + dsn[i+1:]
		}
	}
	return dsn
}

// spawnPos 是已由真实建角包、SavePos 与客户端 pworld.def 交叉确认的出生点。
// 当前只有一种族样本；三个出生 pworld 共用同一坐标，因此 MVP 暂按种族共用。
func spawnPos(_ domain.Race) domain.Pos {
	return domain.Pos{MapID: 60, X: 1418, Y: 510}
}

// gmLandingPos 只从客户端/数据库静态数据中选择已经存在的地图与落点。
// 优先 NPC、刷怪点，最后才选一条进入该图的传送门落点；没有可靠坐标就拒绝
// goto，不能把在线角色送到 (0,0) 或不存在的地图。
func gmLandingPos(world worldData, catalog *data.MapCatalog, mapID int32) (domain.Pos, bool) {
	if mapID <= 0 || catalog == nil {
		return domain.Pos{}, false
	}
	dataMapID := staticMapID(mapID)
	if _, ok := catalog.ByID(dataMapID); !ok {
		return domain.Pos{}, false
	}
	if mapID == 60 {
		return spawnPos(domain.Warrior), true
	}
	if npcs := world.npcs[dataMapID]; len(npcs) > 0 {
		pos := npcs[0].Pos
		pos.MapID = mapID
		return pos, true
	}
	if points := world.spawns[dataMapID]; len(points) > 0 {
		pos := points[0].Pos
		pos.MapID = mapID
		return pos, true
	}

	var best domain.Portal
	bestSource := int32(1<<31 - 1)
	found := false
	for source, portals := range world.portals {
		for _, portal := range portals {
			if portal.To.MapID != dataMapID {
				continue
			}
			if !found || source < bestSource ||
				(source == bestSource && portal.ProcID < best.ProcID) {
				best, bestSource, found = portal, source, true
			}
		}
	}
	if !found {
		return domain.Pos{}, false
	}
	pos := best.To
	pos.MapID = mapID
	return pos, true
}

// seedChars 从种子文件导入初始角色, 供自测。生产由建号流程创建。
func seedChars(ctx context.Context, st store.Store, log *slog.Logger) {
	if *seedFile == "" {
		return
	}
	b, err := os.ReadFile(*seedFile)
	if err != nil {
		log.Warn("读种子失败", "err", err)
		return
	}
	var seed []struct {
		Account      string  `json:"account"`
		Name         string  `json:"name"`
		Race         uint8   `json:"race"`
		Gender       uint8   `json:"gender"`
		Hair         uint8   `json:"hair"`
		Face         uint8   `json:"face"`
		Slot         int32   `json:"slot"`
		Level        int32   `json:"level"`
		MapID        int32   `json:"mapId"`
		X            float64 `json:"x"`
		Y            float64 `json:"y"`
		LastLogin    string  `json:"lastLogin"`
		StatBlockHex string  `json:"statBlockHex"`
	}
	if err := json.Unmarshal(b, &seed); err != nil {
		log.Warn("解析种子失败", "err", err)
		return
	}
	for _, s := range seed {
		acc, _ := st.AccountByName(ctx, s.Account)
		if acc == nil {
			acc, _ = st.CreateAccount(ctx, s.Account, "")
		}
		if s.MapID == 0 {
			s.MapID = 7
		}
		// 将种子中的旧属性块解析为结构化角色属性后保存。
		sb, _ := hex.DecodeString(s.StatBlockHex)
		ap, attrs, honor, ok := protocol.ParseLegacyStatBlock(sb)
		if !ok && len(sb) > 0 {
			log.Warn("种子属性块不完整, 按空属性导入", "name", s.Name, "字节", len(sb))
		}
		ll, _ := time.Parse("2006-01-02 15:04", s.LastLogin)
		c := &domain.Character{
			AccountID: acc.ID, Slot: s.Slot, Name: s.Name,
			Race: domain.Race(s.Race), Level: s.Level,
			Pos: domain.Pos{MapID: s.MapID, X: s.X, Y: s.Y},
			Appear: domain.Appearance{
				Gender: s.Gender, Hair: s.Hair, Head: s.Face,
				EquipView:  [6]uint16{ap.Body, ap.Cap, ap.Backpack, ap.WeaponR, ap.WeaponL, ap.Face},
				AtkVariant: ap.AtkVariant, WeaponCType: ap.WeaponCType, AtkDist: ap.AtkDist,
			},
			LastLogin: ll, Attrs: attrs, Honor: honor,
		}
		if err := st.CreateChar(ctx, c); err != nil {
			log.Warn("导入角色失败", "name", s.Name, "err", err)
			continue
		}
		log.Info("导入角色", "acc", s.Account, "name", s.Name, "lv", s.Level)
	}
}

func dungeonEntranceCount(transports domain.TransportTable) int {
	n := 0
	for _, list := range transports {
		for _, dest := range list.Destinations {
			if dest.Dungeon {
				n++
			}
		}
	}
	return n
}

func dungeonGroupCount(dungeons domain.DungeonTable) int {
	groups := make(map[domain.DungeonID]struct{}, len(dungeons))
	for _, def := range dungeons {
		groups[def.ID] = struct{}{}
	}
	return len(groups)
}

func dungeonPortalCount(portals data.TeleportTable) int {
	n := 0
	for _, list := range portals {
		for _, portal := range list {
			if portal.Dungeon {
				n++
			}
		}
	}
	return n
}

// worldData 是开服一次性读进内存的全部静态配置。
//
// 读完就只读, 所有场景共用同一份 —— 371 张图各自持一份表的话内存会爆,
// 而这些表在运行期从不改动。
type worldData struct {
	trial          *domain.TrialRules
	pickupRange    int32
	mapReturns     data.MapReturnTable
	monsters       spawn.MapDefs
	monsterDialogs *domain.MonsterDialogs
	spawns         data.SpawnTable
	portals        data.TeleportTable
	levels         *domain.LevelTable
	items          data.ItemDefs
	itemIcons      []domain.ItemIconMapping
	itemIconsKnown bool
	loot           data.DropTable
	equipmentRoll  domain.EquipmentRollTable
	// colorProfileErr 记录“怪物类别指向了没有条数配置的染色档”这类不一致。
	// 它会让对应怪物掉出的装备静默全白板，因此与空矩阵一样拒绝启动。
	colorProfileErr   error
	bossFixedDrops    domain.BossFixedDropTable
	skills            domain.SkillTable
	statuses          domain.StatusTable
	statusOverlay     domain.StatusOverlay
	quests            domain.QuestTable
	npcs              data.NPCTable
	transports        domain.TransportTable
	shops             domain.ShopTable
	dungeons          domain.DungeonTable
	dungeonEncounters domain.DungeonEncounterTable
	works             domain.WorkTable
	nianliCreation    domain.NianliCreationTable
	crafts            domain.CraftTable
	refines           domain.RefineTable
	sockets           domain.SocketTable
	washes            domain.WashTable
	stamina           domain.StaminaRule
	rest              domain.RestRule
	combatTiming      domain.CombatTimingRule
	death             domain.DeathRule
	penalty           domain.LevelPenalty
	pets              domain.PetTable
	petLvls           *domain.PetLevelTable
	petFoods          map[domain.ItemID]domain.PetFood
	petPrefixes       domain.PetPrefixTable
	petSkills         domain.PetSkillTable
	petLearnItems     map[domain.ItemID]domain.PetLearningItem
	petRule           domain.PetRule
	rideInviteSeconds int32
	helpTopics        []domain.HelpTopic
	helpTopicsKnown   bool
	warehouseRule     domain.WarehouseRule
	mailRule          domain.MailRule
	chatRules         domain.ChatChannelRules
	announcements     domain.AnnouncementPool
	wardrobes         domain.WardrobeTable
	wardrobeRule      domain.WardrobeRule
	hairRules         domain.HairRules
	stallOpenItems    domain.StallOpenItems
	rack              domain.RackCatalog
	depositRule       domain.DepositRule
}

// loadWorldData 把静态配置全部读进内存。
//
// 大多数表读失败不致命：缺表的后果是对应玩法关掉(比如没有掉落表就不掉东西)，
// 而不是开不了服。装备随机属性矩阵是例外；装配完成后 run 会检查它，避免服务
// 看似正常却把所有普通类型掉落静默产成白板。
//
// 每张表都打一行"装了多少/总共多少", 对账全靠它: 数量不对说明导入环节出了问题,
// 而不是运行期的 bug。
func loadWorldData(ctx context.Context, st store.Store, log *slog.Logger) worldData {
	var w worldData
	pg, ok := st.(*store.Postgres)
	if !ok {
		log.Warn("内存存储: 不加载静态配置(没有怪、没有掉落、没有任务)")
		return w
	}
	q := pg.ReadOnly()
	loadDungeons(ctx, q, &w, log)

	var err error
	if w.mapReturns, err = data.LoadMapReturns(ctx, q); err != nil {
		log.Error("加载地图回城点失败", "err", err)
	} else {
		log.Info("地图回城点就绪", "地图", len(w.mapReturns))
	}
	var defs data.MonsterDefs
	if defs, err = data.LoadMonsters(ctx, q); err != nil {
		log.Warn("加载怪物模板失败", "err", err)
	}
	w.monsters = spawn.MapDefs(defs)
	if w.monsterDialogs, err = data.LoadMonsterDialogs(ctx, q); err != nil {
		log.Error("加载怪物台词失败，怪物台词目录不可用", "err", err)
	} else {
		log.Info("怪物台词目录就绪", "行", w.monsterDialogs.RowCount(),
			"怪物", w.monsterDialogs.MonsterCount(), "场景编号", "客户端 RES_SCENE_*")
	}
	if w.trial, err = data.LoadTrial(ctx, q, defs, w.dungeons); err != nil {
		log.Error("试炼配置加载失败", "err", err)
	}

	if w.dungeonEncounters, err = data.LoadDungeonEncounters(ctx, q, w.dungeons, defs); err != nil {
		log.Warn("加载副本首领阶段链失败", "err", err)
	} else {
		log.Info("副本首领阶段链就绪", "步骤", len(w.dungeonEncounters))
	}
	if len(w.monsters) > 0 {
		var ss data.SpawnStats
		if w.spawns, ss, err = data.LoadSpawns(ctx, q, defs); err != nil {
			log.Warn("加载刷怪点失败", "err", err)
		} else {
			log.Info("刷怪点就绪", "怪物模板", len(w.monsters), "有怪的图", len(w.spawns),
				"装入", ss.Loaded, "总点位", ss.Total,
				"缺模板", ss.NoMonster, "缺地图id", ss.NoMap)
		}
	}
	var ts data.TeleportStats
	if w.portals, ts, err = data.LoadTeleports(ctx, q, w.dungeons); err != nil {
		log.Warn("加载传送门失败", "err", err)
	} else {
		log.Info("传送门就绪", "装入", ts.Loaded, "总数", ts.Total,
			"有门的图", len(w.portals), "起点图无id", ts.NoMap,
			"目标图未定义", ts.NoDest, "无触发区域", ts.NoArea, "副本门", ts.Dungeon)
	}
	if w.levels, err = data.LoadLevels(ctx, q, domain.LevelCap); err != nil {
		log.Warn("加载经验曲线失败", "err", err)
	} else if w.levels != nil {
		// 打两个端点是**对账用**: 曲线导错了(比如少读一列)这两个数会明显不对
		log.Info("经验曲线就绪", "等级上限", w.levels.MaxLevel(),
			"1级升2级需要", w.levels.Need(1),
			"59级升60级需要", w.levels.Need(59))
	}
	var affixSkipped int
	if w.items, affixSkipped, err = data.LoadItemsVerbose(ctx, q); err != nil {
		log.Warn("加载物品模板失败", "err", err)
	} else if affixSkipped > 0 {
		log.Info("装备词条: 跳过无名字/概率触发的行", "跳过", affixSkipped)
	}
	if w.announcements, err = data.LoadPresentation(ctx, q, w.items); err != nil {
		log.Error("加载展示配置失败", "err", err)
	}
	if w.itemIcons, err = data.LoadItemIconMappings(ctx, q); err != nil {
		log.Warn("加载特殊物品图标失败，804d 停用", "err", err)
	} else {
		w.itemIconsKnown = true
		log.Info("特殊物品图标就绪", "映射", len(w.itemIcons))
	}
	if w.hairRules, err = data.LoadHairRules(ctx, q, w.items); err != nil {
		log.Warn("加载发型发色规则失败，理发店停用", "err", err)
	} else {
		log.Info("发型发色规则就绪", "发型", len(w.hairRules[domain.HairStyle]),
			"发色", len(w.hairRules[domain.HairColor]))
	}
	if w.stallOpenItems, err = data.LoadStallOpenItems(ctx, q, w.items); err != nil {
		log.Warn("加载开店古币失败，摆摊停用", "err", err)
	} else {
		log.Info("开店古币就绪", "出售摊", w.stallOpenItems[domain.StallSell],
			"收购摊", w.stallOpenItems[domain.StallBuy])
	}
	var shopItems int
	if w.shops, shopItems, err = data.LoadShops(ctx, q, w.items); err != nil {
		log.Warn("加载 NPC 商店失败，商店停用", "err", err)
	} else {
		log.Info("NPC 商店就绪", "商店", len(w.shops), "库存行", shopItems)
	}
	if w.rack, err = data.LoadRackCatalog(ctx, q, w.items); err != nil {
		log.Warn("加载神奇货架失败，商城停用", "err", err)
	} else {
		log.Info("神奇货架就绪", "分类", len(w.rack.Categories), "商品", len(w.rack.Goods))
	}
	if w.depositRule, err = data.LoadDepositRule(ctx, q); err != nil {
		log.Warn("加载充值规则失败，本地充值停用", "err", err)
	} else {
		log.Info("本地充值规则就绪", "自动入账", w.depositRule.AutoCredit,
			"每单位彩玉", w.depositRule.CaiyuPerUnit, "单次上限", w.depositRule.MaxAmount)
	}
	if w.helpTopics, err = data.LoadHelpTopics(ctx, q); err != nil {
		log.Warn("加载帮助主题失败，帮助面板停用", "err", err)
	} else {
		w.helpTopicsKnown = true
		log.Info("帮助主题就绪", "普通主题", len(w.helpTopics))
	}
	if w.warehouseRule, err = data.LoadWarehouseRule(ctx, q); err != nil {
		log.Warn("加载个人仓库规则失败，个人仓库停用", "err", err)
	} else {
		log.Info("个人仓库规则就绪", "初始页", w.warehouseRule.InitialPages,
			"每页格数", w.warehouseRule.SlotsPerPage, "堆叠上限", w.warehouseRule.MaxStack,
			"存物方向", w.warehouseRule.MoveDepositDir, "存钱模式", w.warehouseRule.MoneyDepositMode)
	}
	if w.mailRule, err = data.LoadMailRule(ctx, q); err != nil {
		log.Warn("加载邮件规则失败，邮件停用", "err", err)
	} else {
		log.Info("邮件规则就绪", "容量", w.mailRule.Capacity,
			"有效天数", w.mailRule.ExpireDays, "附件上限", w.mailRule.MaxAttach)
	}
	if w.chatRules, err = data.LoadChatChannelRules(ctx, q); err != nil {
		log.Warn("加载聊天频道规则失败，频道聊天停用", "err", err)
	} else {
		log.Info("聊天频道规则就绪", "频道", len(w.chatRules))
	}
	if w.wardrobeRule, err = data.LoadWardrobeRule(ctx, q); err != nil {
		log.Warn("加载衣柜规则失败，衣柜停用", "err", err)
	} else {
		var stats data.WardrobeStats
		w.wardrobes, stats, err = data.LoadWardrobeDefs(ctx, q, w.items)
		if err != nil {
			log.Warn("加载衣柜外观失败，衣柜停用", "err", err)
			w.wardrobeRule = domain.WardrobeRule{}
			w.wardrobes = nil
		} else {
			log.Info("衣柜外观就绪", "钥匙物品", w.wardrobeRule.KeyItem,
				"开启钥匙", w.wardrobeRule.OpenKeyCost, "开启容量", w.wardrobeRule.OpenCapacity,
				"容量上限", w.wardrobeRule.MaxCapacity, "装入", stats.Loaded,
				"原始唯一行", stats.Rows, "重复物品跳过", stats.DuplicateIDs,
				"非衣柜部位", stats.UnsupportedCategory, "缺物品", stats.MissingItem,
				"缺外观资源", stats.MissingAppearance)
		}
	}
	var sk data.SkillStats
	if w.skills, sk, err = data.LoadSkills(ctx, q); err != nil {
		log.Warn("加载技能表失败", "err", err)
	} else {
		log.Info("技能表就绪", "装入", len(w.skills), "总行", sk.Total,
			"被动", sk.Passive, "带已闭合状态", sk.WithStatus, "主动但效果待做", sk.NoEffect,
			"结构化直接效果", sk.StructuredEffects, "结构化状态", sk.StructuredStatuses,
			"可用", sk.Loaded-sk.Passive-sk.NoEffect)
	}
	if w.works, w.stamina, err = data.LoadWork(ctx, q); err != nil {
		log.Warn("加载打工表失败", "err", err)
	} else {
		log.Info("打工表就绪", "工种", len(w.works), "耐力上限", w.stamina.Max,
			"每分钟消耗", w.stamina.CostPerMin, "每天回满", w.stamina.RefillDaily)
	}
	if w.nianliCreation, err = data.LoadNianliCreationRules(ctx, q, w.items, w.skills); err != nil {
		log.Warn("加载念力造物规则失败，念力造物停用", "err", err)
	} else {
		log.Info("念力造物规则就绪", "技能", len(w.nianliCreation))
	}
	if w.crafts, err = data.LoadCraftRecipes(ctx, q, w.items); err != nil {
		log.Warn("加载合成配方失败", "err", err)
	} else {
		var recipes int
		for _, rows := range w.crafts.ByType {
			recipes += len(rows)
		}
		log.Info("合成配方就绪", "展示类型", len(w.crafts.ByType), "配方", recipes)
	}
	if w.refines, err = data.LoadRefineRecipes(ctx, q, w.items); err != nil {
		log.Warn("加载精炼配方失败", "err", err)
	} else {
		log.Info("精炼配方就绪", "配方", len(w.refines))
	}
	if w.sockets, err = data.LoadSocketRecipes(ctx, q, w.items); err != nil {
		log.Warn("加载打孔镶嵌配方失败", "err", err)
	} else {
		log.Info("打孔镶嵌配方就绪", "配方", len(w.sockets))
	}
	if w.washes, err = data.LoadWashRecipes(ctx, q, w.items); err != nil {
		log.Warn("加载洗练配方失败", "err", err)
	} else {
		log.Info("洗练配方就绪", "配方键", len(w.washes))
	}
	if w.rest, err = data.LoadRestRule(ctx, q); err != nil {
		log.Warn("加载坐下休息规则失败，坐下回血停用", "err", err)
	} else {
		log.Info("坐下休息规则就绪", "间隔毫秒", w.rest.IntervalMS,
			"最大生命百分比", w.rest.HealMaxHPPct, "固定恢复", w.rest.HealFlat)
	}
	if w.combatTiming, err = data.LoadCombatTimingRule(ctx, q); err != nil {
		log.Warn("加载战斗命中时序失败，使用测试兼容时序", "err", err)
	} else {
		log.Info("战斗命中时序就绪", "近战命中毫秒", w.combatTiming.MeleeHitDelayMS,
			"远程普攻速度", w.combatTiming.RangedBasicSpeedPXPerSec)
	}
	if w.death, err = data.LoadDeathRule(ctx, q); err != nil {
		log.Warn("加载死亡损失规则失败，死亡不扣经验与金钱", "err", err)
	} else {
		log.Info("死亡损失规则就绪", "最低等级", w.death.MinLevel,
			"经验万分比", w.death.ExpLossBP, "金钱万分比", w.death.MoneyLossBP)
	}
	if w.penalty, err = data.LoadLevelPenalty(ctx, q); err != nil {
		log.Warn("加载等级差惩罚规则失败，跨级打怪不递减", "err", err)
	} else {
		log.Info("等级差惩罚就绪", "经验满级差", w.penalty.ExpFullAbove,
			"经验每级万分比", w.penalty.ExpStepBP,
			"掉率满级差", w.penalty.DropFull, "掉率下限万分比", w.penalty.DropFloorBP)
	}
	if w.pets, err = data.LoadPets(ctx, q); err != nil {
		log.Warn("加载宠物表失败", "err", err)
	} else {
		log.Info("宠物表就绪", "种族", len(w.pets), "可捕捉", len(w.pets.Capturable()))
	}
	if w.petFoods, err = data.LoadPetFoods(ctx, q); err != nil {
		log.Warn("加载宠物食物失败", "err", err)
	} else {
		log.Info("宠物食物就绪", "种类", len(w.petFoods))
	}
	if w.petPrefixes, err = data.LoadPetPrefixes(ctx, q); err != nil {
		log.Warn("加载宠物前缀失败", "err", err)
	} else {
		log.Info("宠物前缀就绪", "种类", len(w.petPrefixes))
	}
	if w.pickupRange, err = data.LoadPickupRange(ctx, q); err != nil {
		log.Error("加载拾取规则失败", "err", err)
	}
	if w.petSkills, err = data.LoadPetSkills(ctx, q); err != nil {
		log.Warn("加载宠物技能失败", "err", err)
	} else {
		log.Info("宠物技能就绪", "种类", len(w.petSkills))
	}
	if w.rideInviteSeconds, err = data.LoadRideInviteSeconds(ctx, q); err != nil {
		log.Error("加载同乘规则失败", "err", err)
	}
	if w.petRule, err = data.LoadPetRule(ctx, q); err != nil {
		log.Warn("加载宠物规则失败", "err", err)
	} else {
		log.Info("宠物规则就绪", "自然领悟万分比", w.petRule.NaturalLearnChanceBP,
			"低信赖拒绝万分比", w.petRule.LowTrustRefuseBP,
			"0 信赖自动召回秒数", w.petRule.ZeroTrustRecallSec)
	}
	if w.petLearnItems, err = data.LoadPetLearningItems(ctx, q, w.petSkills); err != nil {
		log.Warn("加载宠物领悟道具失败", "err", err)
	} else {
		log.Info("宠物领悟道具就绪", "种类", len(w.petLearnItems))
	}
	if w.petLvls, err = data.LoadPetLevels(ctx, q, domain.LevelCap); err != nil {
		log.Warn("加载宠物经验曲线失败", "err", err)
	} else if w.petLvls != nil {
		log.Info("宠物经验曲线就绪", "最高级", w.petLvls.MaxLevel())
	}
	if len(w.items) > 0 {
		if w.equipmentRoll, err = data.LoadEquipmentRollTable(ctx, q); err != nil {
			log.Error("加载装备随机属性矩阵失败，启动将中止", "err", err)
		} else {
			var options int
			for _, pool := range w.equipmentRoll.Options {
				options += len(pool)
			}
			log.Info("装备随机属性矩阵就绪", "染色档位", len(w.equipmentRoll.Counts), "结果份额", options)
			if err := data.ValidateColorProfiles(defs, w.equipmentRoll); err != nil {
				w.colorProfileErr = err
				log.Error("染色档位与怪物配置不匹配，启动将中止", "err", err)
			}
		}
		if w.bossFixedDrops, err = data.LoadBossFixedDropTable(ctx, q, w.items); err != nil {
			log.Warn("加载副本BOSS固定装备池失败，固定装备保底停用", "err", err)
		} else {
			log.Info("副本BOSS固定装备池就绪", "规则", len(w.bossFixedDrops))
		}
		var ds data.DropStats
		if w.loot, ds, err = data.LoadDrops(ctx, q, w.items); err != nil {
			log.Warn("加载掉落表失败", "err", err)
		} else {
			log.Info("掉落表就绪", "物品模板", len(w.items), "有掉落的怪", len(w.loot),
				"固定掉落", ds.Loaded, "总行", ds.Total, "随机类别总行", ds.ByKind,
				"随机类别接入", ds.KindLoaded, "随机类别未解析", ds.KindUnresolved, "物品不存在", ds.NoItem,
				"概率非法", ds.BadRate)
		}
	}
	if w.quests, err = data.LoadQuests(ctx, q); err != nil {
		log.Warn("加载任务表失败", "err", err)
	} else {
		steps, stepErr := data.ApplyQuestSteps(ctx, q, w.quests)
		if stepErr != nil {
			// 扁平物品/怪物目标仍可工作；没有可靠步骤的对话任务保持 unknown，
			// fail-closed 而不是回退成接取后立即完成。
			log.Warn("加载任务步骤失败，对话/多阶段任务停用", "err", stepErr,
				"任务数", len(w.quests), "已覆盖", steps.Applied)
		} else {
			log.Info("任务表就绪", "任务数", len(w.quests), "步骤", steps.Steps,
				"步骤源额外任务", steps.Extra)
		}
	}
	var noMapID int
	if w.npcs, noMapID, err = data.LoadNPCs(ctx, q); err != nil {
		log.Warn("加载 NPC 失败", "err", err)
	} else {
		log.Info("NPC 就绪", "有 NPC 的图", len(w.npcs), "地图无 id 跳过", noMapID)
	}
	var transportRows int
	if w.transports, transportRows, err = data.LoadTransports(ctx, q, w.dungeons); err != nil {
		log.Warn("加载 NPC 传送列表失败，飞空艇停用", "err", err)
	} else {
		log.Info("NPC 传送列表就绪", "列表", len(w.transports), "目的地", transportRows)
	}
	if w.quests != nil {
		verified, verifyErr := data.ApplyQuestOverrides(ctx, q, w.quests, w.items,
			data.MonsterDefs(w.monsters), w.spawns, w.npcs, w.loot)
		if verifyErr != nil {
			// 核实规则坏掉时不能继续暴露客户端压平后的简化任务。整张任务表
			// fail-closed，直到 NPC、物品、怪物与落位重新闭合。
			log.Error("加载已核实任务规则失败，任务系统停用", "err", verifyErr)
			w.quests = nil
		} else {
			log.Info("已核实任务规则就绪", "任务", verified.Tasks,
				"禁用任务", verified.Disabled, "运行时任务", len(w.quests), "步骤", verified.Steps,
				"任务专属掉落关系", verified.SpecialDrops, "掉落组", verified.DropGroups,
				"必掉", verified.DropGuaranteed, "高85%", verified.DropHigh,
				"中50%", verified.DropMedium, "低20%", verified.DropLow)
		}
	}
	var stst data.StatusStats
	if w.statuses, stst, err = data.LoadStatuses(ctx, q); err != nil {
		log.Warn("加载状态表失败", "err", err)
	} else {
		log.Info("状态表就绪", "状态数", stst.Statuses, "定义(状态×等级)", stst.Defs,
			"过滤掉的效果行", stst.BadRows)
	}
	var ovst data.StatusOverlayStats
	if w.statusOverlay, ovst, err = data.LoadStatusOverlay(ctx, q); err != nil {
		log.Warn("加载状态叠加矩阵失败", "err", err)
	} else {
		log.Info("状态叠加矩阵就绪", "已有状态行", ovst.Existing,
			"新状态列", ovst.Incoming, "规则格", ovst.Cells)
	}
	return w
}

// loadDungeons 从 PostgreSQL 读取第一版副本定义与入口。入口复用
// game_transport_destinations 的 TransportUI 协议，不再运行时读取客户端 JSON/INI。
func loadDungeons(ctx context.Context, q data.Querier, w *worldData, log *slog.Logger) {
	tbl, err := data.LoadDungeons(ctx, q)
	if err != nil {
		log.Warn("加载副本定义失败", "err", err)
		return
	}
	w.dungeons = tbl
	log.Info("副本定义就绪", "副本数", dungeonGroupCount(tbl), "副本地图", len(tbl))
}
