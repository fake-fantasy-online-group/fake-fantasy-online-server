package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

// 真实数据: 树妖(1001) 掉树枝(4103) 31.4%、牛肉面(3002) 5.7%。
var (
	掉落树枝  = domain.ItemDef{ID: 4103, Name: "树枝", InventoryTab: 1, InventoryTabKnown: true, Stackable: true, Price: 2}
	掉落牛肉面 = domain.ItemDef{ID: 3002, Name: "牛肉面", InventoryTab: 1, InventoryTabKnown: true, Stackable: true, Price: 10}
	必掉的怪  = domain.MonsterDef{
		ID: 1001, Name: "树妖", Kind: domain.MonsterNormal, Level: 14, HP: 246, Exp: 58,
		Stats: domain.NewMonsterStats(14, 6, 42, 0, 0, 2000, 110),
	}
)

// lootScene 建一个会掉东西的场景。
//
// 不走 Router: 那会把 Run 循环也起起来, 而 Stop 会触发 shutdown 把实体全清掉。
// 分配器直接从 Config 给 —— 掉落物要的只是能发号, 不需要整个路由。
func lootScene(t *testing.T, entries []domain.DropEntry) (*Scene, domain.EntityID) {
	t.Helper()
	defs := spawn.MapDefs{必掉的怪.ID: 必掉的怪}
	items := map[domain.ItemID]domain.ItemDef{
		掉落树枝.ID: 掉落树枝, 掉落牛肉面.ID: 掉落牛肉面,
	}
	sp, _ := spawn.New(
		[]domain.SpawnPoint{{ID: 1, Monster: 必掉的怪.ID, Pos: domain.Pos{MapID: 14, X: 120, Y: 100}}},
		defs, domain.NewEntityAlloc())
	s := New(Config{
		ID: domain.SceneID{MapID: 14}, SaveEvery: 10_000, Seed: 3,
		Spawner: sp, Defs: defs, Items: items,
		Loot:  map[domain.MonsterID][]domain.DropEntry{必掉的怪.ID: entries},
		Alloc: domain.NewEntityAlloc(),
	})
	var mid domain.EntityID
	for id, e := range s.entities {
		if e.Kind == domain.KindMonster {
			mid = id
		}
	}
	if mid == 0 {
		t.Fatal("没刷出怪")
	}
	return s, mid
}

func killMonster(t *testing.T, s *Scene, mid domain.EntityID) {
	t.Helper()
	s.entities[1].Stats.MinAtk, s.entities[1].Stats.MaxAtk = 99999, 99999
	s.entities[1].Stats.Hit = 1 << 20
	s.exec(Attack{ID: 1, Target: mid})
	s.step()
}

// groundItems 数场上有几件掉落物。
func groundItems(s *Scene) int { return len(s.ground) }

// 打死怪要在地上留下东西, 而且周围的人看得见。
func TestKillDropsItemOnGround(t *testing.T) {
	s, mid := lootScene(t, []domain.DropEntry{{Item: 掉落树枝.ID, RatePct: 100}})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	sink.take()

	killMonster(t, s, mid)

	if groundItems(s) != 1 {
		t.Fatalf("必掉项应在地上留下 1 件, 实际 %d", groundItems(s))
	}
	sp, ok := firstOf[event.EntitySpawned](sink.take())
	if !ok {
		t.Fatal("掉落物应广播出场, 否则客户端看不到")
	}
	if sp.Kind != domain.KindDrop {
		t.Errorf("出场的应是掉落物, 实际 %v", sp.Kind)
	}
	if sp.Name != "树枝" {
		t.Errorf("名字应是树枝, 实际 %s", sp.Name)
	}
	if sp.Ground == nil {
		t.Fatal("掉落出场必须携带 0x8013 所需的物品与归属视图")
	}
	if sp.Ground.Item != 掉落树枝.ID || sp.Ground.Count != 1 || sp.Ground.Owner != 1 {
		t.Fatalf("掉落视图不符: %+v", *sp.Ground)
	}
	if sp.Ground.ProtectionMS != 30_000 {
		t.Fatalf("真实掉落归属保护应为 30000ms, 实际 %d", sp.Ground.ProtectionMS)
	}
}

// 概率 0 的条目永远不掉。
func TestZeroRateNeverDrops(t *testing.T) {
	s, mid := lootScene(t, []domain.DropEntry{{Item: 掉落树枝.ID, RatePct: 0}})
	join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	killMonster(t, s, mid)

	if groundItems(s) != 0 {
		t.Fatalf("0%% 的条目不该掉, 实际掉了 %d 件", groundItems(s))
	}
}

// 一只怪最多掉几样要封顶 —— 掉落表每条独立掷, 合计能到 3200%,
// 不封顶的话一只 BOSS 会一次吐出几十个实体把广播撑爆。
func TestDropsAreCapped(t *testing.T) {
	var entries []domain.DropEntry
	for i := 0; i < 30; i++ {
		entries = append(entries, domain.DropEntry{Item: 掉落树枝.ID, RatePct: 100})
	}
	s, mid := lootScene(t, entries)
	join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	killMonster(t, s, mid)

	if n := groundItems(s); n != maxDropsPerKill {
		t.Fatalf("30 条必掉应被封顶到 %d 件, 实际 %d", maxDropsPerKill, n)
	}
}

// 捡起来要进背包, 地上的要消失。
func TestPickUpIntoBag(t *testing.T) {
	s, mid := lootScene(t, []domain.DropEntry{{Item: 掉落树枝.ID, RatePct: 100}})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	killMonster(t, s, mid)
	sink.take()

	var lootID domain.EntityID
	for id := range s.ground {
		lootID = id
	}
	s.exec(PickUp{ID: 1, Target: lootID})
	s.step()

	bag := s.entities[1].Player.Bag
	if bag.CountOf(掉落树枝.ID) != 1 {
		t.Fatalf("背包里应有 1 个树枝, 实际 %d", bag.CountOf(掉落树枝.ID))
	}
	if groundItems(s) != 0 {
		t.Fatal("捡走之后地上不该还有")
	}
	evs := sink.take()
	if _, ok := firstOf[event.InventorySnapshot](evs); !ok {
		t.Error("背包变了要通知本人")
	}
	if _, ok := firstOf[event.EntityDespawned](evs); !ok {
		t.Error("掉落物消失要广播, 否则客户端上会留个捡不掉的影子")
	}
}

// 隔着半张图捡不到 —— 不判距离的话客户端可以把地上的东西吸走。
func TestPickUpTooFar(t *testing.T) {
	s, mid := lootScene(t, []domain.DropEntry{{Item: 掉落树枝.ID, RatePct: 100}})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	killMonster(t, s, mid)
	sink.take()

	var lootID domain.EntityID
	for id := range s.ground {
		lootID = id
	}
	// 走远(还在同格, 免得掉落物出视野)
	s.exec(MoveTo{ID: 1, To: domain.Pos{MapID: 14, X: 280, Y: 280}})
	s.step()
	sink.take()

	s.exec(PickUp{ID: 1, Target: lootID})
	s.step()

	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectTooFar {
		t.Fatalf("够不着应回 RejectTooFar, 实际 %+v", rej)
	}
	if groundItems(s) != 1 {
		t.Error("没捡成的东西应还在地上")
	}
}

// 背包满了要回执, 而且东西留在地上 —— 不能凭空吞掉。
func TestPickUpBagFull(t *testing.T) {
	s, mid := lootScene(t, []domain.DropEntry{{Item: 掉落树枝.ID, RatePct: 100}})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	killMonster(t, s, mid)
	sink.take()

	// 把背包塞满别的东西
	p := s.entities[1]
	p.Player.Bag = domain.NewBag(1)
	p.Player.Bag.Add(掉落牛肉面, domain.MaxStack)

	var lootID domain.EntityID
	for id := range s.ground {
		lootID = id
	}
	s.exec(PickUp{ID: 1, Target: lootID})
	s.step()

	rej, ok := firstOf[event.Rejected](sink.take())
	if !ok || rej.Reason != event.RejectBagFull {
		t.Fatalf("背包满了应回 RejectBagFull, 实际 %+v", rej)
	}
	if groundItems(s) != 1 {
		t.Fatal("没捡成的东西应还在地上, 不能凭空吞掉")
	}
}

// 归属保护: 谁打死的谁先捡。
func TestLootOwnerProtection(t *testing.T) {
	s, mid := lootScene(t, []domain.DropEntry{{Item: 掉落树枝.ID, RatePct: 100}})
	join(t, s, 1, 100, "甲", 100, 100)
	sinkB := join(t, s, 2, 200, "乙", 130, 100)
	s.step()
	killMonster(t, s, mid) // 甲打死的
	sinkB.take()

	var lootID domain.EntityID
	for id := range s.ground {
		lootID = id
	}
	// 乙抢
	s.exec(PickUp{ID: 2, Target: lootID})
	s.step()
	if _, ok := firstOf[event.Rejected](sinkB.take()); !ok {
		t.Fatal("保护期内别人不该捡得到")
	}
	if groundItems(s) != 1 {
		t.Fatal("没捡成东西应还在")
	}

	// 保护到期后谁都能捡
	for i := 0; i < lootOwnerTicks+1; i++ {
		s.step()
	}
	s.exec(PickUp{ID: 2, Target: lootID})
	s.step()
	if groundItems(s) != 0 {
		t.Fatal("保护到期后应该能捡")
	}
	if s.entities[2].Player.Bag.CountOf(掉落树枝.ID) != 1 {
		t.Fatal("乙没拿到东西")
	}
}

// 没人捡的东西要到期消失, 否则一张图刷一整天会堆满树枝。
func TestLootExpires(t *testing.T) {
	s, mid := lootScene(t, []domain.DropEntry{{Item: 掉落树枝.ID, RatePct: 100}})
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	killMonster(t, s, mid)
	sink.take()

	for i := 0; i < lootTTLTicks+1; i++ {
		s.step()
	}
	if groundItems(s) != 0 {
		t.Fatalf("到期的掉落物应消失, 实际还剩 %d 件", groundItems(s))
	}
	if _, ok := firstOf[event.EntityDespawned](sink.take()); !ok {
		t.Error("消失要广播")
	}
}

// 捡东西要标脏 —— 捡了不落盘, 重启就白捡了。
func TestPickUpMarksDirty(t *testing.T) {
	s, mid := lootScene(t, []domain.DropEntry{{Item: 掉落树枝.ID, RatePct: 100}})
	join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	killMonster(t, s, mid)
	s.entities[1].Player.TakeDirty() // 清掉之前的脏标记

	var lootID domain.EntityID
	for id := range s.ground {
		lootID = id
	}
	s.exec(PickUp{ID: 1, Target: lootID})
	s.step()

	if !s.entities[1].Player.Dirty() {
		t.Fatal("捡了东西却没标脏 —— 重启就白捡了")
	}
}

// 背包跟着人跨图走, 不能留在原场景。
func TestBagFollowsTeleport(t *testing.T) {
	r, rig := twoScenes(t)
	src, _ := r.Ensure(domain.SceneID{MapID: 7})
	dst, _ := r.Ensure(domain.SceneID{MapID: 14})

	bag := domain.NewBag(10)
	bag.Add(掉落树枝, 7)
	src.Post(Enter{ID: 1, Bag: bag, Sink: &fakeSink{},
		Char: &domain.Character{ID: 100, Name: "甲", Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}})
	inspect(t, src, rig, func(*Scene) {})

	src.Post(Teleport{ID: 1, To: domain.SceneID{MapID: 14},
		At: domain.Pos{MapID: 14, X: 500, Y: 500}})
	inspect(t, src, rig, func(*Scene) {})
	inspect(t, dst, rig, func(s *Scene) {
		e := s.EntityAt(1)
		if e == nil || e.Player.Bag == nil {
			t.Fatal("人到了但背包没跟过来")
		}
		if got := e.Player.Bag.CountOf(掉落树枝.ID); got != 7 {
			t.Fatalf("背包里应还有 7 个树枝, 实际 %d", got)
		}
	})
}
