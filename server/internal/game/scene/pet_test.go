package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

// 海龟：pet_id=1006，**同时也是 15 级的怪物 id=1006**（18/18 逐行核对过）。
// 捕捉等级 10、成功率 100→160、捕捉道具 3382。
var (
	海龟种 = domain.PetDef{
		ID: 1006, Name: "海龟",
		CaptureLevel: 10, CaptureTool: 3382, BaseCaptureRate: 100, MaxCaptureRate: 160,
		Init:   domain.Base{STR: 3, DEX: 2, AGI: 2, VIT: 6, INT: 6, SPI: 8},
		InitHP: 54, InitSP: 72,
		GrowthOdd:  domain.Base{VIT: 3, INT: 2, SPI: 3, DEX: 1, AGI: 1},
		GrowthEven: domain.Base{VIT: 3, INT: 2, SPI: 3, DEX: 1, AGI: 1},
		MaxLevel:   80, InitTrust: 50, InitStarve: 50,
	}
	// 抓不到的宠(486/504 都是这样)
	不可捕种 = domain.PetDef{ID: 1001, Name: "树妖", Init: domain.Base{VIT: 10}, InitHP: 90}

	海龟怪 = domain.MonsterDef{
		ID: 1006, Name: "海龟", Kind: domain.MonsterNormal, Level: 15, HP: 200, Exp: 60,
		Stats: domain.NewMonsterStats(15, 6, 42, 0, 0, 2000, 110),
	}
	捕捉球 = domain.ItemDef{ID: 3382, Name: "初级宠物球", InventoryTab: 1, InventoryTabKnown: true, Stackable: true}

	// 宠物载体物品：宠物背包上线后，每只宠物都要在背包第 4 页占一个载体物品。
	// 生产数据里 bag_tab 恒为 3，item_id 规律是 1800000000 + pet_id。
	海龟蛋 = domain.ItemDef{
		ID: 1800001006, Name: "海龟宠物蛋", InventoryTab: 3, InventoryTabKnown: true,
		Stackable: false, InstanceKind: domain.ItemInstancePet, PetCarrierSpecies: 1006,
	}
)

// lastReject 取事件里的拒绝原因; 没有拒绝时返回 0(RejectUnknown 也是 0,
// 但本文件里没有用到 RejectUnknown 的分支)。
func lastReject(evs []event.Event) event.RejectReason {
	if r, ok := firstOf[event.Rejected](evs); ok {
		return r.Reason
	}
	return 0
}

func hasEvent[T event.Event](evs []event.Event) bool {
	_, ok := firstOf[T](evs)
	return ok
}

// do 执行一条命令并把事件冲刷出去。
//
// exec 只把事件排进 outbox, 真正送到 Sink 的是 flush ——
// 少这一步的话 sink.take() 永远是空的, 断言就会全部退化成"什么都没发生"。
func do(s *Scene, c Command) {
	s.exec(c)
	s.flush()
}

func petTable(defs ...domain.PetDef) domain.PetTable {
	t := domain.PetTable{}
	for _, d := range defs {
		t[d.ID] = d
	}
	return t
}

// petScene 建一张有海龟怪的图，玩家自带捕捉球。
func petScene(t *testing.T) (*Scene, *fakeSink) {
	t.Helper()
	defs := spawn.MapDefs{海龟怪.ID: 海龟怪}
	sp, _ := spawn.New(
		[]domain.SpawnPoint{{ID: 1, Monster: 海龟怪.ID, Pos: domain.Pos{MapID: 7, X: 140, Y: 100}}},
		defs, domain.NewEntityAlloc())
	s := New(Config{
		ID: domain.SceneID{MapID: 7}, SaveEvery: 10_000, Seed: 42,
		Spawner: sp, Defs: defs, Levels: rewardLevels(),
		Alloc:     domain.NewEntityAlloc(),
		Items:     map[domain.ItemID]domain.ItemDef{3382: 捕捉球, 海龟蛋.ID: 海龟蛋},
		Pets:      petTable(海龟种, 不可捕种),
		PetLevels: domain.NewPetLevelTable(map[int32]int64{1: 300, 2: 1100, 3: 2700, 4: 5460}),
		PetRule: domain.PetRule{
			NaturalLearnChanceBP: 2000,
			InitialSkillMin:      0,
			InitialSkillMax:      0,
			LowTrustRefuseBP:     2000,
			HungerIntervalSec:    300,
			TrustHappySec:        60,
			TrustContentSec:      120,
			DeployHungerAdd:      1,
			DeathTrustLoss:       10,
			TradeTrustRequired:   50,
			TradeTrustLoss:       50,
		},
	})
	sink := join(t, s, 1, 100, "训练家", 140, 100)
	s.step()
	sink.take()

	p := s.players[1]
	p.Player.Char.Level = 30
	// 六维得给足: 默认的 1 级角色打 15 级怪是打不动的,
	// 而本文件要测的是"宠物有没有接上", 不是"数值平不平衡"
	p.Player.Char.Base = domain.Base{STR: 200, VIT: 100, INT: 20, SPI: 20, AGI: 20, DEX: 20}
	s.refreshStats(p)
	p.HP = p.MaxHP
	p.Player.Bag.Add(捕捉球, 10)
	return s, sink
}

// 一只打得动 15 级怪的宠。真实的 1 级海龟力量只有 3 —— 那是对的,
// 但用它测"宠物会不会出手"只会测出"1 级宠打不过 15 级怪"。
func 战力宠() domain.PetInstance {
	inst := domain.NewPetInstance(海龟种)
	inst.ID = 1
	// 场景召唤测试使用已经孵化、可正常出战的宠物；NewPetInstance 表示刚捕捉
	// 的实例，生产语义下默认尚未孵化。
	inst.Hatched = true
	inst.Base = domain.Base{STR: 200, VIT: 100, INT: 20, SPI: 20, AGI: 20, DEX: 20}
	inst.HP = inst.MaxHP(海龟种)
	return inst
}

// 场上那只海龟。
func theTurtle(t *testing.T, s *Scene) *domain.EntityID {
	t.Helper()
	for id, m := range s.monsters {
		if m.Monster != nil && m.Monster.TypeID == 海龟怪.ID {
			out := id
			return &out
		}
	}
	t.Fatal("图上没有海龟")
	return nil
}

func TestCaptureNeedsAWeakenedMonster(t *testing.T) {
	s, sink := petScene(t)
	id := *theTurtle(t, s)
	sink.take()

	// 满血就抓不动
	do(s, CapturePet{ID: 1, Target: id})
	if r := lastReject(sink.take()); r != event.RejectTargetNotWeak {
		t.Fatalf("满血捕捉该被拒(打残了才抓得动), 得到 %v", r)
	}
	// 打残到门槛以下
	s.monsters[id].HP = 10
	do(s, CapturePet{ID: 1, Target: id})
	got := sink.take()
	if r := lastReject(got); r != 0 {
		t.Fatalf("残血还被拒: %v", r)
	}
	if !hasEvent[event.PetCaptured](got) && !hasEvent[event.PetCaptureFailed](got) {
		t.Fatal("既没成功也没失败 —— 这一掷根本没发生")
	}
}

func TestCaptureRejectsUncapturableMonster(t *testing.T) {
	s, sink := petScene(t)
	id := *theTurtle(t, s)
	// 把它当成树妖(抓不到的那种)
	s.monsters[id].Monster.TypeID = domain.MonsterID(不可捕种.ID)
	s.monsters[id].HP = 10
	sink.take()

	do(s, CapturePet{ID: 1, Target: id})
	if r := lastReject(sink.take()); r != event.RejectNotCapturable {
		t.Fatalf("486/504 只宠是抓不到的, 该拒, 得到 %v", r)
	}
}

// 道具在**掷之前**扣。放在成功后扣的话失败就是免费的,
// 那最优解永远是满血狂点直到中奖。
func TestCaptureConsumesToolEvenOnFailure(t *testing.T) {
	s, sink := petScene(t)
	p := s.players[1]
	id := *theTurtle(t, s)
	s.monsters[id].HP = 10
	before := p.Player.Bag.CountOf(3382)
	sink.take()

	do(s, CapturePet{ID: 1, Target: id})
	if after := p.Player.Bag.CountOf(3382); after != before-1 {
		t.Fatalf("捕捉球 %d → %d, 该扣掉一个", before, after)
	}
}

func TestCaptureNeedsTheTool(t *testing.T) {
	s, sink := petScene(t)
	p := s.players[1]
	p.Player.Bag.Remove(3382, p.Player.Bag.CountOf(3382)) // 球全没了
	id := *theTurtle(t, s)
	s.monsters[id].HP = 10
	sink.take()

	do(s, CapturePet{ID: 1, Target: id})
	if r := lastReject(sink.take()); r != event.RejectNoCaptureTool {
		t.Fatalf("没球该拒, 得到 %v", r)
	}
}

// 抓走的怪从场上消失，但**不给经验、不掉落、不排重生** ——
// 排重生的话抓走一只等于凭空多刷一只，刷怪点就成了宠物贩卖机。
func TestCapturedMonsterLeavesWithoutRewards(t *testing.T) {
	s, sink := petScene(t)
	p := s.players[1]
	id := *theTurtle(t, s)
	s.monsters[id].HP = 1
	expBefore := p.Player.Char.Exp
	sink.take()

	// 血空时成功率取上限 160 → 必成
	do(s, CapturePet{ID: 1, Target: id})
	got := sink.take()
	if !hasEvent[event.PetCaptured](got) {
		t.Fatal("血都空了还没抓住 —— 成功率取的不是上限")
	}
	if _, still := s.monsters[id]; still {
		t.Fatal("抓走了怪还在场上")
	}
	if _, still := s.entities[id]; still {
		t.Fatal("抓走了实体还在 entities 里 —— 悬空引用")
	}
	if p.Player.Char.Exp != expBefore {
		t.Fatalf("抓宠给了经验 %d → %d, 抓不是杀", expBefore, p.Player.Char.Exp)
	}
	if len(p.Player.Char.Pets) != 1 {
		t.Fatalf("宠物栏里有 %d 只, 该是 1", len(p.Player.Char.Pets))
	}
	inst := p.Player.Char.Pets[0]
	if inst.Def != 海龟种.ID || inst.Level != 1 || inst.HP != 海龟种.InitHP {
		t.Fatalf("新宠不对: %+v", inst)
	}
}

func TestPetSlotsAreLimited(t *testing.T) {
	s, sink := petScene(t)
	p := s.players[1]
	for i := 0; i < domain.MaxPets; i++ {
		p.Player.Char.Pets = append(p.Player.Char.Pets, domain.NewPetInstance(海龟种))
	}
	id := *theTurtle(t, s)
	s.monsters[id].HP = 1
	sink.take()

	do(s, CapturePet{ID: 1, Target: id})
	if r := lastReject(sink.take()); r != event.RejectPetSlotsFull {
		t.Fatalf("宠物栏满了该拒, 得到 %v", r)
	}
}

// ── 召唤与跟随 ──

func summonOne(t *testing.T, s *Scene) *domain.PetInstance {
	t.Helper()
	p := s.players[1]
	p.Player.Char.Pets = append(p.Player.Char.Pets, 战力宠())
	do(s, SummonPet{ID: 1, Inst: 1})
	if p.Player.Pet == 0 {
		t.Fatal("召唤了却没放出来")
	}
	return &p.Player.Char.Pets[0]
}

func TestSummonPutsPetInScene(t *testing.T) {
	s, sink := petScene(t)
	sink.take()
	summonOne(t, s)

	p := s.players[1]
	e, ok := s.entities[p.Player.Pet]
	if !ok {
		t.Fatal("宠物不在 entities 里")
	}
	if e.Kind != domain.KindPet {
		t.Fatalf("宠物的 Kind 是 %v", e.Kind)
	}
	if _, ok := s.monsters[e.ID]; ok {
		t.Fatal("宠物混进了 monsters —— stepAI 会把它当敌人索敌")
	}
	if _, ok := s.pets[e.ID]; !ok {
		t.Fatal("宠物不在 pets 里")
	}
	if e.ID.SegKind() != domain.KindPet {
		t.Fatalf("宠物 id %d 不在宠物段里", e.ID)
	}
	// 生命上限走"保留手调差值"那条式子: InitHP + (当前体质 − 初始体质) × 9
	want := 海龟种.InitHP + (e.Pet.Inst.Base.VIT-海龟种.Init.VIT)*domain.HPPerVIT
	if e.MaxHP != want {
		t.Fatalf("宠物生命上限 %d, 按公式该是 %d", e.MaxHP, want)
	}
	if !hasEvent[event.PetSummoned](sink.take()) {
		t.Fatal("没发召唤事件")
	}
}

func TestCannotSummonTwo(t *testing.T) {
	s, sink := petScene(t)
	summonOne(t, s)
	p := s.players[1]
	p.Player.Char.Pets = append(p.Player.Char.Pets, domain.PetInstance{
		ID: 2, Def: 海龟种.ID, Level: 1, Base: 海龟种.Init, HP: 54, MP: 72})
	sink.take()

	do(s, SummonPet{ID: 1, Inst: 2})
	if r := lastReject(sink.take()); r != event.RejectPetAlreadyOut {
		t.Fatalf("已经放了一只, 该拒, 得到 %v", r)
	}
}

// 收回前必须把血抄回实例 —— 不抄的话"召回再召唤"就是免费满血。
func TestRecallCarriesHPBack(t *testing.T) {
	s, _ := petScene(t)
	inst := summonOne(t, s)
	p := s.players[1]
	e := s.entities[p.Player.Pet]
	e.HP = 20 // 挨了一顿

	do(s, RecallPet{ID: 1})
	if p.Player.Pet != 0 {
		t.Fatal("收回了还挂着实体 id")
	}
	if inst.HP != 20 {
		t.Fatalf("实例上的血是 %d, 该是收回时的 20 —— 否则再召唤就是免费满血", inst.HP)
	}
	// 再召唤要接着这个血
	do(s, SummonPet{ID: 1, Inst: 1})
	if got := s.entities[p.Player.Pet].HP; got != 20 {
		t.Fatalf("再召唤后血是 %d, 该接着 20", got)
	}
}

func TestPetFollowsOwner(t *testing.T) {
	s, _ := petScene(t)
	summonOne(t, s)
	p := s.players[1]
	e := s.entities[p.Player.Pet]

	// 主人走远(仍在牵引范围内), 宠物该跟上来
	p.Pos.X = 140 + petFollowDist + 200
	before := sqDist(e.Pos, p.Pos)
	for i := 0; i < 20; i++ {
		s.step()
	}
	if after := sqDist(e.Pos, p.Pos); after >= before {
		t.Fatalf("宠物没跟上来: 距离平方 %.0f → %.0f", before, after)
	}
}

// 跟丢太远直接拉回来。"慢慢追"的话主人跑图时宠物会越掉越远。
func TestPetWarpsWhenLeashSnaps(t *testing.T) {
	s, _ := petScene(t)
	summonOne(t, s)
	p := s.players[1]
	e := s.entities[p.Player.Pet]

	p.Pos.X = 140 + petLeashDist + 500
	s.step()
	if e.Pos != p.Pos {
		t.Fatalf("超出牵引距离该直接瞬移, 宠物在 %+v, 主人在 %+v", e.Pos, p.Pos)
	}
}

// 主人离场时宠物必须跟着摘掉 —— 留在场上就是一只没人管的野东西。
func TestPetLeavesWithOwner(t *testing.T) {
	s, _ := petScene(t)
	summonOne(t, s)
	p := s.players[1]
	petID := p.Player.Pet

	do(s, Leave{ID: 1})
	if _, still := s.entities[petID]; still {
		t.Fatal("主人走了宠物还在场上")
	}
	if _, still := s.pets[petID]; still {
		t.Fatal("宠物还留在 pets 表里")
	}
}

// ── 辅助单位边界 ──

func TestPetDoesNotJoinOwnersAttack(t *testing.T) {
	s, _ := petScene(t)
	summonOne(t, s)
	p := s.players[1]
	id := *theTurtle(t, s)
	m := s.monsters[id]
	m.Pos = p.Pos // 就在脚边
	e := s.entities[p.Player.Pet]
	e.Pos = p.Pos

	before := m.HP
	// 主人正常攻击，但辅助宠物不能锁定或攻击战斗目标。
	for i := 0; i < 40; i++ {
		s.exec(Attack{ID: 1, Target: id})
		s.step()
		if !m.Alive() {
			break
		}
	}
	if m.HP >= before {
		t.Fatal("打了 40 帧怪一点血没掉")
	}
	if e.Pet.Target != 0 {
		t.Fatalf("辅助宠物不该锁定战斗目标, 得到 %d", e.Pet.Target)
	}
}

// 即使队列里残留旧版本产生的宠物攻击，也必须在战斗结算边界丢弃。
func TestPetCannotDealDamage(t *testing.T) {
	s, _ := petScene(t)
	summonOne(t, s)
	p := s.players[1]
	id := *theTurtle(t, s)
	m := s.monsters[id]
	m.HP = 1

	expBefore := p.Player.Char.Exp
	petID := p.Player.Pet
	for i := 0; i < 10 && m.Alive(); i++ {
		s.attacks = append(s.attacks, pendingAttack{src: petID, dst: id})
		s.stepCombat()
	}
	if !m.Alive() || m.HP != 1 {
		t.Fatalf("辅助宠物造成了伤害: alive=%v hp=%d", m.Alive(), m.HP)
	}
	if p.Player.Char.Exp != expBefore {
		t.Fatalf("无效宠物攻击改变了主人经验: %d → %d",
			expBefore, p.Player.Char.Exp)
	}
}

// 宠物那份经验是额外的，不从主人那份里扣 —— 扣的话没人会带宠物。
func TestPetExpIsExtraNotDeducted(t *testing.T) {
	s, _ := petScene(t)
	inst := summonOne(t, s)
	p := s.players[1]
	id := *theTurtle(t, s)
	m := s.monsters[id]
	m.HP = 1

	expBefore := p.Player.Char.Exp
	for i := 0; i < 10 && m.Alive(); i++ {
		s.attacks = append(s.attacks, pendingAttack{src: 1, dst: id})
		s.stepCombat()
	}
	gained := p.Player.Char.Exp - expBefore
	if gained != 海龟怪.Exp {
		t.Fatalf("主人拿到 %d 经验, 该是整份 %d —— 宠物那份不能从这里扣",
			gained, 海龟怪.Exp)
	}
	want := domain.SharedExp(海龟怪.Exp)
	if inst.Exp != want && inst.Level == 1 {
		t.Fatalf("宠物拿到 %d, 该是 %d(主人那份的 %d%%)",
			inst.Exp, want, domain.PetExpShare)
	}
}

func TestPetCannotOutlevelOwner(t *testing.T) {
	s, _ := petScene(t)
	inst := summonOne(t, s)
	p := s.players[1]
	p.Player.Char.Level = 2 // 主人才 2 级

	// 灌一大笔经验
	s.grantPetExp(p, 1_000_000)
	if inst.Level > 2 {
		t.Fatalf("宠物到了 %d 级, 主人才 2 级 —— 宠物不能超过主人", inst.Level)
	}
}

func TestPetLevelUpRefreshesStats(t *testing.T) {
	s, _ := petScene(t)
	inst := summonOne(t, s)
	p := s.players[1]
	p.Player.Char.Level = 60
	e := s.entities[p.Player.Pet]
	beforeHP := e.MaxHP

	s.grantPetExp(p, 10_000)
	if inst.Level <= 1 {
		t.Fatal("灌了一万经验还没升级")
	}
	if e.Level != inst.Level {
		t.Fatalf("实体等级 %d 与实例等级 %d 对不上", e.Level, inst.Level)
	}
	if e.MaxHP <= beforeHP {
		t.Fatalf("升级后生命上限 %d 没超过 %d", e.MaxHP, beforeHP)
	}
	// 升级补满血, 而且是按**新**上限补 —— 按旧上限补的话新涨的那几点白涨
	if e.HP != e.MaxHP {
		t.Fatalf("升级后血 %d / 上限 %d, 该补满", e.HP, e.MaxHP)
	}
	if e.Stats.MaxHP != e.MaxHP {
		t.Fatalf("Stats.MaxHP=%d 与 MaxHP=%d 分了叉 —— 两个来源迟早读到走样的那个",
			e.Stats.MaxHP, e.MaxHP)
	}
}

// ── 饥渴 ──
//
// 口径全部实证: ov_petgrow 四档 + 食物描述文本, 两边逐个吻合。
// 见 domain/pet.go 的注释。

var (
	// 真值: 476 只宠都是这四档
	标准饥渴 = domain.StarveRule{
		Tiers: [4]domain.StarveTier{
			{Delta: -1, Interval: 180},  // 寄养(MVP 没有)
			{Delta: -1, Interval: 360},  // 收回
			{Delta: +1, Interval: 1800}, // 跟随
			{Delta: +1, Interval: 900},  // 打架
		},
		Online: domain.StarveTier{Delta: +1, Interval: 14400},
	}
	// 三档草食粮, 真值取自 game_descs 3020/3025/3026
	百叶草 = domain.PetFood{Item: 3020, Name: "百叶草", Reduce: 3, Min: 0, Max: 50}
	百花草 = domain.PetFood{Item: 3025, Name: "百花草", Reduce: 2, Min: 51, Max: 75}
	百果草 = domain.PetFood{Item: 3026, Name: "百果草", Reduce: 1, Min: 76, Max: 100}
	无尽淳 = domain.PetFood{Item: 3031, Name: "无尽淳", Reduce: 5, Trust: 2, Anytime: true}
)

func foodTable() map[domain.ItemID]domain.PetFood {
	t := map[domain.ItemID]domain.PetFood{}
	for _, f := range []domain.PetFood{百叶草, 百花草, 百果草, 无尽淳} {
		t[f.Item] = f
	}
	return t
}

// 越饿, 每次喂食效果越差 —— 这是原表设计好的惩罚, 不是我们加的。
func TestFoodTiersGetWeakerAsHungerRises(t *testing.T) {
	for _, c := range []struct {
		starve int32
		food   domain.PetFood
		want   int32
	}{
		{3, 百叶草, 3}, {50, 百叶草, 3},
		{51, 百花草, 2}, {75, 百花草, 2},
		{76, 百果草, 1}, {100, 百果草, 1},
	} {
		inst := domain.NewPetInstance(海龟种)
		inst.Starve = c.starve
		got, _, ok := inst.Feed(c.food)
		if !ok {
			t.Fatalf("饥渴 %d 用 %s 该能用", c.starve, c.food.Name)
		}
		if got != c.want {
			t.Fatalf("饥渴 %d 用 %s 降了 %d, 该是 %d", c.starve, c.food.Name, got, c.want)
		}
	}
}

// 档位不对不能静默吞掉 —— 玩家会以为喂了却没效果, 然后一直喂。
func TestWrongFoodTierIsRejected(t *testing.T) {
	inst := domain.NewPetInstance(海龟种)
	inst.Starve = 80 // 高饥渴档
	if _, _, ok := inst.Feed(百叶草); ok {
		t.Fatal("饥渴 80 用低档粮(0~50)该被拒")
	}
	if inst.Starve != 80 {
		t.Fatalf("被拒了还改了饥渴度: %d", inst.Starve)
	}
	// 不饿就别喂 —— 白扣一份粮, 玩家会以为是 bug
	饱 := domain.NewPetInstance(海龟种)
	饱.Starve, 饱.Trust = 0, domain.MaxTrust
	if _, _, ok := 饱.Feed(百叶草); ok {
		t.Fatal("不饿还能喂 —— 那一份粮白扣了")
	}
	// 但无尽淳在不饿时仍然 +2 信赖, 那一口不算白喂
	饱2 := domain.NewPetInstance(海龟种)
	饱2.Starve, 饱2.Trust = 0, 10
	if _, tr, ok := 饱2.Feed(无尽淳); !ok || tr != 2 {
		t.Fatalf("不饿时无尽淳该只加信赖, 得到 信赖%d ok=%v", tr, ok)
	}

	// 无尽淳任何时候都能用
	d, tr, ok := inst.Feed(无尽淳)
	if !ok || d != 5 || tr != 2 {
		t.Fatalf("无尽淳该降 5 加 2 信赖, 得到 降%d 信赖%d ok=%v", d, tr, ok)
	}
}

func TestFeedIsClampedAtZero(t *testing.T) {
	inst := domain.NewPetInstance(海龟种)
	inst.Starve = 2
	got, _, ok := inst.Feed(百叶草) // 降 3, 但只剩 2
	if !ok {
		t.Fatal("该能喂")
	}
	if inst.Starve != 0 {
		t.Fatalf("饥渴度成了 %d, 该钳在 0", inst.Starve)
	}
	if got != 2 {
		t.Fatalf("报了降 %d, 该报**实际**降的 2 —— 否则客户端飘的数和面板对不上", got)
	}
}

// 状态决定用哪一档: 打架比跟随饿得快。
func TestFightingMakesPetHungrierFasterThanFollowing(t *testing.T) {
	跟随 := 标准饥渴.TierFor(domain.PetFollowing)
	打架 := 标准饥渴.TierFor(domain.PetFighting)
	if 跟随.Delta <= 0 || 打架.Delta <= 0 {
		t.Fatal("放出来的两档都该是往上涨")
	}
	if 打架.Interval >= 跟随.Interval {
		t.Fatalf("打架 %d 秒一跳、跟随 %d 秒一跳 —— 出力多的该饿得快",
			打架.Interval, 跟随.Interval)
	}
	// 收回来是往下降的
	if 标准饥渴.TierFor(domain.PetRecalled).Delta >= 0 {
		t.Fatal("收回来该恢复")
	}
}

// petStarveScene 建一个宠物已经放出来、且带食物表的场景。
func petStarveScene(t *testing.T) (*Scene, *entity.Entity, *fakeSink) {
	t.Helper()
	s, sink := petScene(t)
	s.petFoods = foodTable()
	p := s.players[1]
	inst := 战力宠()
	inst.Def = 海龟种.ID
	p.Player.Char.Pets = append(p.Player.Char.Pets, inst)
	// 给这只宠配上标准四档
	def := s.petDefs[海龟种.ID]
	def.Starve = 标准饥渴
	s.petDefs[海龟种.ID] = def
	do(s, SummonPet{ID: 1, Inst: 1})
	if p.Player.Pet == 0 {
		t.Fatal("没放出来")
	}
	sink.take()
	return s, s.entities[p.Player.Pet], sink
}

func TestPetGetsHungryOverTime(t *testing.T) {
	s, e, _ := petStarveScene(t)
	inst := e.Pet.Inst
	before := inst.Starve

	// 跟随档 1800 秒 = 18000 帧, 跑够两跳
	for i := 0; i < 18000*2+20; i++ {
		s.step()
	}
	if inst.Starve <= before {
		t.Fatalf("跑了一小时饥渴度还是 %d(出生 %d) —— 计时器没接上", inst.Starve, before)
	}
}

// 新版需求计时使用全局饥饿间隔，同一状态下不能每帧重排截止时间。
func TestGlobalHungerTimerIsStable(t *testing.T) {
	s, e, _ := petStarveScene(t)
	s.step()
	first := e.Pet.NextHungerAt
	if first == 0 {
		t.Fatal("召唤后没排上计时器")
	}
	for i := 0; i < 5; i++ {
		s.step()
	}
	if e.Pet.NextHungerAt != first {
		t.Fatalf("全局饥饿计时器从 %d 挪到了 %d", first, e.Pet.NextHungerAt)
	}
}

// 新规则由信赖决定是否出战；饥渴到上限本身不再沿用旧版的硬拒绝。
func TestStarvedPetUsesTrustBasedSummonRule(t *testing.T) {
	s, _ := petScene(t)
	p := s.players[1]
	inst := 战力宠()
	inst.Starve = domain.MaxStarve // 饿透了
	p.Player.Char.Pets = append(p.Player.Char.Pets, inst)

	do(s, SummonPet{ID: 1, Inst: 1})
	if p.Player.Pet == 0 {
		t.Fatal("信赖正常的宠物被旧饥渴规则拒绝出战")
	}
	if got := p.Player.Char.Pets[0].Starve; got != domain.MaxStarve {
		t.Fatalf("出战消耗应钳在饥渴上限, 得到 %d", got)
	}
}

func TestFeedPetCommand(t *testing.T) {
	s, _, sink := petStarveScene(t)
	p := s.players[1]
	inst := &p.Player.Char.Pets[0]
	inst.Starve = 40
	p.Player.Bag.Add(domain.ItemDef{ID: 3020, Name: "百叶草", InventoryTab: 1, InventoryTabKnown: true, Stackable: true}, 5)
	sink.take()

	do(s, FeedPet{ID: 1, Inst: 1, Item: 3020})
	ev, ok := firstOf[event.PetFed](sink.take())
	if !ok {
		t.Fatal("没发喂食事件")
	}
	if ev.DeltaStarve != 3 || ev.Starve != 37 {
		t.Fatalf("喂完 饥渴=%d 降了%d, 该是 37 / 3", ev.Starve, ev.DeltaStarve)
	}
	if got := p.Player.Bag.CountOf(3020); got != 4 {
		t.Fatalf("食物剩 %d 份, 该扣掉一份", got)
	}
	if !p.Player.Dirty() {
		t.Fatal("喂完没标脏 —— 重启就白喂了")
	}
}

func TestFeedRejects(t *testing.T) {
	s, _, sink := petStarveScene(t)
	p := s.players[1]
	p.Player.Char.Pets[0].Starve = 40
	sink.take()

	// 背包里没有
	do(s, FeedPet{ID: 1, Inst: 1, Item: 3020})
	if r := lastReject(sink.take()); r != event.RejectNoItem {
		t.Fatalf("背包没食物该拒, 得到 %v", r)
	}
	// 不是食物
	p.Player.Bag.Add(domain.ItemDef{ID: 9001, Name: "石头", InventoryTab: 1, InventoryTabKnown: true}, 1)
	do(s, FeedPet{ID: 1, Inst: 1, Item: 9001})
	if r := lastReject(sink.take()); r != event.RejectNotFood {
		t.Fatalf("不是食物该拒, 得到 %v", r)
	}
	// 档位不对
	p.Player.Bag.Add(domain.ItemDef{ID: 3026, Name: "百果草", InventoryTab: 1, InventoryTabKnown: true, Stackable: true}, 1)
	do(s, FeedPet{ID: 1, Inst: 1, Item: 3026})
	if r := lastReject(sink.take()); r != event.RejectWrongFoodTier {
		t.Fatalf("饥渴 40 用高档粮(76~100)该拒, 得到 %v", r)
	}
	// 没这只宠
	do(s, FeedPet{ID: 1, Inst: 99, Item: 3026})
	if r := lastReject(sink.take()); r != event.RejectNoPet {
		t.Fatalf("没这只宠该拒, 得到 %v", r)
	}
}

// 旧种族表里的零饥渴曲线不再控制在线需求计时；统一规则接管后，至少应保证
// 部署消耗之外不会在首个全局间隔到达前额外变化或发生除零。
func TestLegacyZeroStarvePetUsesGlobalNeedTimer(t *testing.T) {
	s, _ := petScene(t)
	p := s.players[1]
	def := s.petDefs[海龟种.ID]
	def.Starve = domain.StarveRule{} // 全零
	s.petDefs[海龟种.ID] = def
	inst := 战力宠()
	p.Player.Char.Pets = append(p.Player.Char.Pets, inst)
	do(s, SummonPet{ID: 1, Inst: 1})

	e := s.entities[p.Player.Pet]
	before := e.Pet.Inst.Starve
	for i := 0; i < 2000; i++ {
		s.step()
	}
	if got := e.Pet.Inst.Starve; got != before {
		t.Fatalf("首个全局需求间隔前饥渴度 %d → %d", before, got)
	}
}

// ── 辅助宠物不代替玩家拾取 ──

var 掉落物 = domain.ItemDef{ID: 3002, Name: "牛肉面", InventoryTab: 1, InventoryTabKnown: true, Stackable: true, Price: 10}

// pickupScene 建一个宠物会捡东西、地上有东西的场景。
func pickupScene(t *testing.T) (*Scene, *entity.Entity, *fakeSink) {
	t.Helper()
	s, sink := petScene(t)
	s.items[掉落物.ID] = 掉落物

	def := s.petDefs[海龟种.ID]
	def.CanPickUp = true
	def.Starve = domain.StarveRule{} // 本组测试不关心饥渴
	s.petDefs[海龟种.ID] = def

	p := s.players[1]
	p.Player.Char.Pets = append(p.Player.Char.Pets, 战力宠())
	do(s, SummonPet{ID: 1, Inst: 1})
	if p.Player.Pet == 0 {
		t.Fatal("没放出来")
	}
	sink.take()
	return s, s.entities[p.Player.Pet], sink
}

// dropAt 在指定位置放一件归 owner 的掉落物, 返回它的实体 id。
func dropAt(s *Scene, x, y float64, owner domain.EntityID) domain.EntityID {
	s.spawnLoot(掉落物, domain.Pos{MapID: 7, X: x, Y: y}, owner)
	for id, e := range s.ground {
		if e.Pos.X == x && e.Pos.Y == y {
			return id
		}
	}
	return 0
}

func TestPetDoesNotFetchOwnersDrop(t *testing.T) {
	s, e, _ := pickupScene(t)
	p := s.players[1]
	id := dropAt(s, 200, 100, p.ID) // 主人在 (140,100), 东西在 60 单位外
	if id == 0 {
		t.Fatal("没放出掉落物")
	}
	before := p.Player.Bag.CountOf(掉落物.ID)

	for i := 0; i < 60; i++ {
		s.step()
		if _, still := s.ground[id]; !still {
			break
		}
	}
	if _, still := s.ground[id]; !still {
		t.Fatal("辅助宠物自动捡走了掉落物")
	}
	if got := p.Player.Bag.CountOf(掉落物.ID); got != before {
		t.Fatalf("辅助宠物改变了主人背包数量: %d → %d", before, got)
	}
	_ = e
}

// 归属保护照样管用：派宠物去抢别人的东西，等于绕开归属规则的官方外挂。
func TestPetRespectsLootOwnership(t *testing.T) {
	s, _, _ := pickupScene(t)
	p := s.players[1]
	别人 := domain.EntityID(999)
	id := dropAt(s, 200, 100, 别人)

	for i := 0; i < 60; i++ {
		s.step()
	}
	if _, still := s.ground[id]; !still {
		t.Fatal("宠物把别人保护期内的东西捡走了")
	}
	if p.Player.Bag.CountOf(掉落物.ID) != 0 {
		t.Fatal("抢到了主人背包里")
	}
}

// 即使归属保护已经结束，辅助宠物也不代替玩家执行拾取。
func TestPetStillDoesNotPickUpAfterOwnershipExpires(t *testing.T) {
	s, _, _ := pickupScene(t)
	p := s.players[1]
	id := dropAt(s, 200, 100, domain.EntityID(999))
	s.ground[id].Drop.OwnerUntil = s.tick // 保护期到此为止

	for i := 0; i < 60; i++ {
		s.step()
		if _, still := s.ground[id]; !still {
			break
		}
	}
	if _, still := s.ground[id]; !still {
		t.Fatal("保护期结束后辅助宠物自动捡走了掉落物")
	}
	if p.Player.Bag.CountOf(掉落物.ID) != 0 {
		t.Fatal("辅助宠物把掉落物放进了主人背包")
	}
}

// 太远的不去 —— 宠物不该为了一根树枝跑到屏幕外面。
func TestPetIgnoresFarAwayDrops(t *testing.T) {
	s, _, _ := pickupScene(t)
	p := s.players[1]
	id := dropAt(s, 140+petPickupRange+200, 100, p.ID)

	for i := 0; i < 60; i++ {
		s.step()
	}
	if _, still := s.ground[id]; !still {
		t.Fatal("宠物跑到很远的地方去捡了")
	}
}

// 不会捡的宠就不捡。381 只是这样。
func TestPetWithoutPickupFlagDoesNotFetch(t *testing.T) {
	s, _, _ := pickupScene(t)
	def := s.petDefs[海龟种.ID]
	def.CanPickUp = false
	s.petDefs[海龟种.ID] = def
	// 重新召唤一次, 让实体上的 Def 拷贝也更新
	p := s.players[1]
	do(s, RecallPet{ID: 1})
	do(s, SummonPet{ID: 1, Inst: 1})

	id := dropAt(s, 200, 100, p.ID)
	for i := 0; i < 60; i++ {
		s.step()
	}
	if _, still := s.ground[id]; !still {
		t.Fatal("不带拾取标志的宠也去捡了")
	}
}

// 主人进入战斗也不能让辅助宠物恢复旧版的锁敌或拾取行为。
func TestOwnersCombatDoesNotActivateLegacyPetBehaviors(t *testing.T) {
	s, e, _ := pickupScene(t)
	p := s.players[1]
	m := s.monsters[*theTurtle(t, s)]
	m.Pos = p.Pos
	e.Pos = p.Pos
	id := dropAt(s, 200, 100, p.ID)

	// 主人开打，宠物仍只保持辅助跟随。
	for i := 0; i < 10; i++ {
		s.exec(Attack{ID: 1, Target: m.ID})
		s.step()
		if !m.Alive() {
			break
		}
	}
	if e.Pet.Target != 0 {
		t.Fatalf("辅助宠物不该接战斗目标, 得到 %d", e.Pet.Target)
	}
	if _, still := s.ground[id]; !still {
		t.Fatal("辅助宠物自动捡走了掉落物")
	}
}

// 辅助宠物不发起拾取，因此满背包时也不应制造拾取拒绝事件。
func TestFullBagDoesNotProducePetPickupWarnings(t *testing.T) {
	s, _, sink := pickupScene(t)
	p := s.players[1]
	p.Player.Bag = domain.NewBag(1)
	p.Player.Bag.Add(domain.ItemDef{ID: 9999, Name: "石头", InventoryTab: 1, InventoryTabKnown: true}, 1)
	dropAt(s, 150, 100, p.ID)
	sink.take()

	for i := 0; i < 40; i++ {
		s.step()
	}
	n := 0
	for _, ev := range sink.take() {
		if r, ok := ev.(event.Rejected); ok && r.Reason == event.RejectBagFull {
			n++
		}
	}
	if n != 0 {
		t.Fatalf("辅助宠物未拾取却产生了 %d 次背包满提示", n)
	}
}

// ── 孤儿宠：主人没了，宠还在 ──
//
// `despawnOrphanPet` 是**防御分支**。三条移除玩家的路径里：
//
//	onLeave    先 recallPet ✓
//	onTeleport 先 recallPet ✓
//	尸体计时器  没收宠，但 stepPets 在主人死的那一帧就已经收了
//
// 所以正常玩不出孤儿。但兜底分支得真的兜得住 —— 兜不住的结果是
// 场上留一只没人管、谁也打不着的实体，而且它永远不会被清掉。

func TestNormalPathsNeverOrphanAPet(t *testing.T) {
	for _, c := range []struct {
		name string
		act  func(s *Scene)
	}{
		{"离场", func(s *Scene) { do(s, Leave{ID: 1}) }},
		{"主人死了", func(s *Scene) {
			p := s.players[1]
			p.HP = 0
			s.step()
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, _ := petScene(t)
			p := s.players[1]
			p.Player.Char.Pets = append(p.Player.Char.Pets, 战力宠())
			do(s, SummonPet{ID: 1, Inst: 1})
			petID := p.Player.Pet
			if petID == 0 {
				t.Fatal("没放出来")
			}

			c.act(s)
			s.step()

			if _, still := s.pets[petID]; still {
				t.Fatal("主人没了宠物还留在 pets 表里 —— 它谁也打不着, 也不会被清掉")
			}
			if _, still := s.entities[petID]; still {
				t.Fatal("宠物还留在 entities 里 —— 悬空引用")
			}
		})
	}
}

// 兜底分支本身要能用：手工造出孤儿状态，看它清不清得掉。
func TestOrphanPetIsCleanedUp(t *testing.T) {
	s, _ := petScene(t)
	p := s.players[1]
	inst := 战力宠()
	p.Player.Char.Pets = append(p.Player.Char.Pets, inst)
	do(s, SummonPet{ID: 1, Inst: 1})
	petID := p.Player.Pet
	e := s.entities[petID]
	e.HP = 33 // 挨过打, 血要抄回实例

	// 造孤儿: 把主人从 players 里摘掉, 但不动宠物
	// (对应尸体计时器那条路径 —— 它 delete(s.players) 时不管宠)
	delete(s.players, 1)

	s.step()

	if _, still := s.pets[petID]; still {
		t.Fatal("孤儿宠没被清掉")
	}
	if _, still := s.entities[petID]; still {
		t.Fatal("孤儿宠还在 entities 里")
	}
	// 血也要抄回去 —— 主人重新登录时那只宠该是 33 血, 不是满血
	if got := p.Player.Char.Pets[0].HP; got != 33 {
		t.Fatalf("实例上的血是 %d, 该是摘掉时的 33 —— 否则孤儿一次就白送满血", got)
	}
}

// 客户端手上只有 0x800b 发下去的那份列表, 它回报的是**那份列表的下标**。
// 两边数法不一致的表现是"点第三只放出来的是第二只" —— 而且不报错。
func TestSummonPetAtUsesSnapshotSlotOrder(t *testing.T) {
	s, sink := petScene(t)
	p := s.players[1]
	// 中间夹一只种族表里没有的宠: 快照会跳过它, 槽位号必须跟着跳。
	野种 := domain.PetInstance{ID: 7, Def: 999999, Level: 1}
	甲 := 战力宠()
	乙 := 战力宠()
	乙.ID = 9
	p.Player.Char.Pets = []domain.PetInstance{甲, 野种, 乙}
	sink.take()

	snap := s.petSnapshot(p)
	if len(snap.Pets) != 2 {
		t.Fatalf("快照里有 %d 只 —— 种族表里没有的那只该跳过", len(snap.Pets))
	}
	if snap.Pets[1].Slot != 1 {
		t.Fatalf("第二行的槽位号是 %d, 该是 1", snap.Pets[1].Slot)
	}

	// 客户端点"放出第 1 只"(0 基), 该出来的是乙, 不是那只野种
	do(s, SummonPetAt{ID: 1, Slot: 1})
	if p.Player.Pet == 0 {
		t.Fatal("按槽位放宠没放出来")
	}
	if got := s.entities[p.Player.Pet].Pet.Inst.ID; got != 乙.ID {
		t.Fatalf("放出来的是实例 %d, 该是 %d", got, 乙.ID)
	}
}

// 槽位号越界只能被拒, 不能悄悄放出别的宠 —— 更不能崩。
func TestSummonPetAtOutOfRangeIsRejected(t *testing.T) {
	s, sink := petScene(t)
	p := s.players[1]
	p.Player.Char.Pets = []domain.PetInstance{战力宠()}
	sink.take()

	do(s, SummonPetAt{ID: 1, Slot: 5})
	if p.Player.Pet != 0 {
		t.Fatal("越界的槽位号居然放出了一只宠")
	}
	if !hasEvent[event.Rejected](sink.take()) {
		t.Fatal("越界该有拒绝回执 —— 静默失败在联调时和'客户端没发'分不开")
	}
	do(s, SummonPetAt{ID: 1, Slot: -1})
	if p.Player.Pet != 0 {
		t.Fatal("负数槽位号也放出了宠")
	}
}

// 宠物栏一变就要重发整份快照: 客户端那边是整份覆盖, 没有增量。
func TestPetSnapshotIsPushedOnEveryChange(t *testing.T) {
	s, sink := petScene(t)
	p := s.players[1]
	p.Player.Char.Pets = []domain.PetInstance{战力宠()}
	sink.take()

	do(s, SummonPetAt{ID: 1, Slot: 0})
	snap, ok := firstOf[event.PetSnapshot](sink.take())
	if !ok {
		t.Fatal("放出宠之后没重发宠物栏快照")
	}
	if snap.ActiveSlot != 0 || !snap.Show {
		t.Fatalf("放出之后的快照: 出战槽=%d show=%v", snap.ActiveSlot, snap.Show)
	}

	do(s, RecallPet{ID: 1})
	snap2, ok := firstOf[event.PetSnapshot](sink.take())
	if !ok {
		t.Fatal("收回之后没重发宠物栏快照")
	}
	if snap2.ActiveSlot != event.NoActivePet || snap2.Show {
		t.Fatalf("收回之后的快照: 出战槽=%d show=%v", snap2.ActiveSlot, snap2.Show)
	}
	// 收回时血量已经抄回实例, 快照里就该是那份血
	if len(snap2.Pets) != 1 {
		t.Fatalf("收回后宠物栏里有 %d 只", len(snap2.Pets))
	}
}

// 初次进世界必须发一份宠物栏 —— 哪怕一只宠都没有。
// 不发的话客户端保留的是上一次登录/上一个角色的残留。
// 跨图落地则**不能**重发: 那条路上客户端的本人状态一直在, 重发等于把
// 已经在场上的宠再走一遍出场流程。
func TestPetSnapshotOnLoginNotOnTransfer(t *testing.T) {
	s, _ := petScene(t)
	// petScene 里的 join 已经把 bootstrap 事件排进去了, 上面被 take 掉;
	// 这里另起一个人重新走一遍初次入场。
	sink2 := join(t, s, 2, 200, "二号", 140, 100)
	s.flush()
	snap, ok := firstOf[event.PetSnapshot](sink2.take())
	if !ok {
		t.Fatal("初次进世界没发宠物栏快照")
	}
	if snap.ActiveSlot != event.NoActivePet || len(snap.Pets) != 0 {
		t.Fatalf("没有宠时的快照该是空栏无出战: %+v", snap)
	}

	// 跨图落地: Runtime 非空就是交接, 这条路上不该再有宠物栏快照
	sink3 := &fakeSink{}
	s.exec(Enter{
		ID: 3, Char: &domain.Character{ID: 300, Name: "三号",
			Pos: domain.Pos{MapID: 7, X: 150, Y: 100}},
		Sink: sink3, Runtime: &entity.PlayerRuntime{},
	})
	s.flush()
	if hasEvent[event.PetSnapshot](sink3.take()) {
		t.Fatal("跨图落地重发了宠物栏快照")
	}
}
