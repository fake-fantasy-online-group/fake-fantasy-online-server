// Package spawn 负责一张图上的怪：开场铺满、死后重生。
//
// 它**只造实体，不管实体**。造出来的 *entity.Entity 交给场景，此后归场景独占 ——
// spawn 自己不持有任何实体指针，只记「哪个刷怪点现在是空的」。
// 这条分工让 spawn 完全可以脱离场景单测。
//
// ⚠️ **重生间隔在客户端数据里不存在。**
// 45635 个刷怪点、每个只有四个键（id / index / init_pos / init_dir），
// 一个字节的时间信息都没有。所以下面那几个间隔是**服务端定的**，
// 不是还原出来的 —— 将来要调就直接调，不用担心违背原作。
package spawn

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// 重生间隔（逻辑帧）。**服务端决定，客户端数据里没有对应项。**
//
// 取值理由：普通怪 15 秒是这个年代 MMO 的常见手感 —— 短到不会站着等，
// 长到一队人不能把一张图刷成流水线。精英和 BOSS 按稀有度拉长。
// 摆设和采集物压根不重生（它们本来就不该死）。
const (
	RespawnNormal       domain.Tick = 150   // 15 秒
	RespawnElite        domain.Tick = 600   // 1 分钟
	RespawnBoss         domain.Tick = 18000 // 30 分钟
	EliteChanceInterval domain.Tick = 600   // 每分钟累加一次精英概率
	EliteKillChanceStep             = 1     // 每次有效击杀增加 1 个百分点
	EliteTimeChanceStep             = 1     // 每过一分钟增加 1 个百分点
)

// RespawnTicks 返回某类怪的重生间隔。0 表示不重生。
func RespawnTicks(k domain.MonsterKind) domain.Tick {
	switch k {
	case domain.MonsterElite:
		return RespawnElite
	case domain.MonsterBoss:
		return RespawnBoss
	case domain.MonsterProp, domain.MonsterGather:
		return 0 // 摆设与采集物不参与生死轮回
	}
	return RespawnNormal
}

// Defs 是"按配置 id 查怪物模板"的能力。
// 用接口而不是具体 map，是为了让 spawn 不依赖 data 包（那会让 game 层认识数据库）。
type Defs interface {
	Def(domain.MonsterID) (domain.MonsterDef, bool)
}

// MapDefs 是 Defs 的 map 实现，给测试和普通装配用。
type MapDefs map[domain.MonsterID]domain.MonsterDef

func (m MapDefs) Def(id domain.MonsterID) (domain.MonsterDef, bool) {
	d, ok := m[id]
	return d, ok
}

// Spawner 管一张图的刷怪点。只在场景 goroutine 内访问，无需加锁。
type Spawner struct {
	points map[int32]domain.SpawnPoint // 刷怪点 id -> 点位
	defs   Defs
	alloc  *domain.EntityAlloc

	// occupied 记哪些点上现在站着东西。
	// 存的是**刷怪点 id 的集合**，不是实体指针 —— 实体归场景，spawn 不该攥着它。
	occupied map[int32]bool

	// eliteChance 是本场景下一只符合条件的重生怪成为光圈精英的累计概率
	// （0~100）。任意可变精英的怪死亡与地图内每过一分钟都会累加，刷中后归零。
	eliteChance uint8
}

// New 建一张图的刷怪器。points 里引用不到模板的点会被丢掉并计数。
func New(points []domain.SpawnPoint, defs Defs, alloc *domain.EntityAlloc) (*Spawner, int) {
	s := &Spawner{
		points:   make(map[int32]domain.SpawnPoint, len(points)),
		defs:     defs,
		alloc:    alloc,
		occupied: make(map[int32]bool, len(points)),
	}
	skipped := 0
	for _, p := range points {
		if _, ok := defs.Def(p.Monster); !ok {
			skipped++
			continue
		}
		s.points[p.ID] = p
	}
	return s, skipped
}

// Count 返回本图有效刷怪点数。
func (s *Spawner) Count() int { return len(s.points) }

// Initial 把所有空着的刷怪点铺满，返回新造的实体。
// 开场调一次；场景重建时再调一次也是安全的（已占的点不会重复出怪）。
func (s *Spawner) Initial() []*entity.Entity {
	out := make([]*entity.Entity, 0, len(s.points))
	for id := range s.points {
		if e := s.Spawn(id); e != nil {
			out = append(out, e)
		}
	}
	return out
}

// Spawn 在指定刷怪点造一只怪。点不存在、或者上面已经有东西了，返回 nil。
func (s *Spawner) Spawn(pointID int32) *entity.Entity {
	return s.spawn(pointID, false, 0)
}

// Respawn 在击杀后的重生入口造怪。roll 必须在 0~99；低于当前累计概率时，
// 普通模板替换为其 EliteID 指向的精英模板，并把该点的累计概率清零。
func (s *Spawner) Respawn(pointID int32, roll uint8) *entity.Entity {
	return s.spawn(pointID, true, roll)
}

func (s *Spawner) spawn(pointID int32, allowElite bool, roll uint8) *entity.Entity {
	if s.occupied[pointID] {
		return nil
	}
	p, ok := s.points[pointID]
	if !ok {
		return nil
	}
	d, ok := s.defs.Def(p.Monster)
	if !ok {
		return nil // New 里筛过，正常到不了这
	}
	eliteRing := false
	if allowElite && roll < s.eliteChance {
		if elite, ok := s.eliteDef(d); ok {
			d = elite
			eliteRing = true
			s.eliteChance = 0
		}
	}
	s.occupied[pointID] = true
	return build(s.alloc.Monster(), p, d, eliteRing)
}

func (s *Spawner) eliteDef(base domain.MonsterDef) (domain.MonsterDef, bool) {
	if base.Kind != domain.MonsterNormal || base.EliteID == 0 {
		return domain.MonsterDef{}, false
	}
	elite, ok := s.defs.Def(base.EliteID)
	return elite, ok && elite.Kind == domain.MonsterElite
}

// Release 把一个刷怪点标记为空。怪被打死、或者实体因别的原因离场时调。
func (s *Spawner) Release(pointID int32) { delete(s.occupied, pointID) }

// RecordKill 累计本场景共用的精英保底概率。没有有效精英模板的点不参与，
// 避免无意义地积到 100% 后仍永远只能刷普通怪。
func (s *Spawner) RecordKill(pointID int32) {
	p, ok := s.points[pointID]
	if !ok {
		return
	}
	base, ok := s.defs.Def(p.Monster)
	if !ok {
		return
	}
	if _, ok := s.eliteDef(base); !ok {
		return
	}
	s.addEliteChance(EliteKillChanceStep)
}

// RecordMinute 累计本地图每过一分钟获得的精英概率。
func (s *Spawner) RecordMinute() {
	s.addEliteChance(EliteTimeChanceStep)
}

func (s *Spawner) addEliteChance(step int) {
	next := int(s.eliteChance) + step
	if next > 100 {
		next = 100
	}
	s.eliteChance = uint8(next)
}

// Occupied 报告该点上是否有活物。给测试与指标用。
func (s *Spawner) Occupied(pointID int32) bool { return s.occupied[pointID] }

// build 把「刷怪点 + 模板」拼成一只实体。
func build(id domain.EntityID, p domain.SpawnPoint, d domain.MonsterDef, eliteRing bool) *entity.Entity {
	return &entity.Entity{
		ID:    id,
		Kind:  domain.KindMonster,
		Name:  d.Name,
		Level: d.Level,
		HP:    d.HP,
		MaxHP: d.HP,
		Pos:   p.Pos,
		Dir:   uint8(p.Dir),
		Look:  domain.Look{ModelID: d.Sprite},
		Stats: d.Stats,
		Monster: &entity.Monster{
			TypeID:       d.ID,
			Kind:         d.Kind,
			EliteRing:    eliteRing,
			ColorProfile: d.ColorProfile,
			SpawnID:      p.ID,
			Home:         p.Pos, // 只记录刷怪点；脱战不回位，允许怪被聚到传送门附近

			Aggressive:    d.Aggressive,
			NoAttack:      d.NoAttack,
			NoBasicAttack: d.NoBasicAttack,
			ViewDist:      d.ViewDist,
			TraceDist:     d.TraceDist,
			AI:            d.AI,
		},
	}
}

// Summon 由场景技能系统创建一只不占刷怪点的临时怪物。它沿用正常模板和
// 0x800f 出场路径，但死亡后不掉落、不重生。
func Summon(id domain.EntityID, pos domain.Pos, d domain.MonsterDef, owner domain.EntityID) *entity.Entity {
	e := build(id, domain.SpawnPoint{Pos: pos}, d, false)
	e.Monster.SpawnID = 0
	e.Monster.Home = pos
	e.Monster.Summoner = owner
	return e
}

// Encounter 创建不占静态刷怪点、但正常参与经验、任务和掉落的副本阶段实体。
// 它与技能召唤物不同：Summoner 保持 0，因此击杀结果不能被静默跳过。
func Encounter(id domain.EntityID, pos domain.Pos, dir uint8, d domain.MonsterDef) *entity.Entity {
	return build(id, domain.SpawnPoint{Pos: pos, Dir: int32(dir)}, d, false)
}
