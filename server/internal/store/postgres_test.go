package store

import (
	"context"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Postgres 集成测试。需要一个可连的 Postgres(默认与 server/docker-compose.yml 一致)。
// 设 fantasy_TEST_DSN 覆盖; 连不上则 Skip(CI 无 DB 时不阻塞)。
//
// 每个用例在独立的临时 schema 里跑, 用后清理, 互不干扰。
// testStore 连**测试库**并清空表。
//
// ⚠️ **默认库名是 fantasy_test，不是 fantasy。** 这不是洁癖 ——
// 这些测试会 `TRUNCATE characters, accounts`，指到开发库上就等于
// **每跑一次 make check 就把所有角色和账号删光**。
//
// 实测踩过：客户端联调种了个角色进去，跑完测试再登录就变成"角色不存在"，
// 而日志上什么异常都没有(TRUNCATE 是成功的)。查了半天才想到是自己的测试删的。
//
// 建库：
//
//	docker exec fantasy-postgres psql -U fantasy -c "CREATE DATABASE fantasy_test;"
//	for f in server/migrations/*.sql; do
//	  docker exec -i fantasy-postgres psql -U fantasy -d fantasy_test < "$f"; done
func testStore(t *testing.T) *Postgres {
	t.Helper()
	dsn := os.Getenv("fantasy_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://fantasy:fantasy_dev@127.0.0.1:5432/fantasy_test?sslmode=disable"
	}
	mustBeTestDB(t, dsn)

	ctx := context.Background()
	p, err := Open(ctx, dsn)
	if err != nil {
		t.Skipf("连不上测试库(%v), 跳过集成测试。建库方法见 testStore 的注释", err)
	}
	// 清空表(测试隔离)。上面的 mustBeTestDB 已经确认过这是测试库
	if _, err := p.pool.Exec(ctx, `TRUNCATE characters, accounts RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("清表: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

// mustBeTestDB 挡住"往开发库上跑清空型测试"。
//
// 判据是**库名以 _test 结尾**。定得死一点是故意的：
// 这条挡的是不可逆的数据丢失，宁可误伤一个命名不规范的测试库，
// 也不能放过一次把开发库清空的机会。
func mustBeTestDB(t *testing.T, dsn string) {
	t.Helper()
	name := dbNameOf(dsn)
	if strings.HasSuffix(name, "_test") {
		return
	}
	t.Fatalf("拒绝在库 %q 上跑清空型集成测试 —— 这些测试会 TRUNCATE characters/accounts。\n"+
		"库名必须以 _test 结尾。改 fantasy_TEST_DSN, 或建一个 fantasy_test(方法见 testStore 注释)。", name)
}

// dbNameOf 从 DSN 里取库名。取不到返回空串 —— 空串不以 _test 结尾, 所以会被挡下,
// 这正是想要的: 解析不出来时按"不安全"处理。
func dbNameOf(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/")
}

func TestPostgresAccountAndChar(t *testing.T) {
	p := testStore(t)
	ctx := context.Background()

	// 账号: 不存在 -> 建 -> 查
	if a, _ := p.AccountByName(ctx, "alice"); a != nil {
		t.Fatal("应不存在")
	}
	acc, err := p.CreateAccount(ctx, "alice", "hash1")
	if err != nil {
		t.Fatalf("建账号: %v", err)
	}
	if acc.ID == 0 {
		t.Fatal("账号 ID 未分配")
	}
	// 重名账号
	if _, err := p.CreateAccount(ctx, "alice", "x"); err != ErrDup {
		t.Fatalf("重名应 ErrDup, 得 %v", err)
	}

	// 建角色
	c := &domain.Character{
		AccountID: acc.ID, Slot: 0, Name: "片姐司马", Race: domain.Assassin,
		Level: 62, Pos: domain.Pos{MapID: 7, X: 8890, Y: 5101},
		Appear: domain.Appearance{Gender: 1, Hair: 1, Head: 1,
			EquipView:  [6]uint16{143, 204, 772, 1, 0, 0},
			AtkVariant: 1, WeaponCType: 2, AtkDist: 75},
		Attrs: []int32{4, 490, 546},
	}
	if err := p.CreateChar(ctx, c); err != nil {
		t.Fatalf("建角色: %v", err)
	}
	if c.ID == 0 {
		t.Fatal("角色 ID 未分配")
	}
	// 重名角色
	if err := p.CreateChar(ctx, &domain.Character{AccountID: acc.ID, Slot: 1, Name: "片姐司马"}); err != domain.ErrNameTaken {
		t.Fatalf("重名角色应 ErrNameTaken, 得 %v", err)
	}

	// 读回并逐字段核对(不丢档验证)
	got, err := p.CharByName(ctx, "片姐司马")
	if err != nil {
		t.Fatalf("查角色: %v", err)
	}
	if got.Level != 62 || got.Race != domain.Assassin || got.Pos.MapID != 7 ||
		got.Pos.X != 8890 || got.Appear.EquipView[2] != 772 {
		t.Fatalf("角色字段读回不一致: %+v", got)
	}
	// 属性数组与攻击表现是 0x8003/0x8004 直接要用的, 丢一个客户端就显示错
	if len(got.Attrs) != 3 || got.Attrs[1] != 490 {
		t.Fatalf("属性数组读回不一致: %v", got.Attrs)
	}
	if got.Appear.AtkVariant != 1 || got.Appear.WeaponCType != 2 || got.Appear.AtkDist != 75 {
		t.Fatalf("攻击表现读回不一致: %+v", got.Appear)
	}

	// 存档: 改等级+坐标, 再读回
	got.Level = 63
	got.Pos.X = 9000
	if err := p.SaveChar(ctx, got); err != nil {
		t.Fatalf("存档: %v", err)
	}
	re, _ := p.CharByName(ctx, "片姐司马")
	if re.Level != 63 || re.Pos.X != 9000 {
		t.Fatalf("存档未持久化: lv=%d x=%f", re.Level, re.Pos.X)
	}

	// 按账号列角色
	list, _ := p.CharsByAccount(ctx, acc.ID)
	if len(list) != 1 {
		t.Fatalf("账号角色数 %d, 期望 1", len(list))
	}
}

// TestPostgresTxRollback 事务内出错必须回滚, 不留半成品。
func TestPostgresTxRollback(t *testing.T) {
	p := testStore(t)
	ctx := context.Background()
	acc, _ := p.CreateAccount(ctx, "bob", "h")

	// 事务里建两个角色, 第二个故意重名 -> 整体回滚 -> 第一个也不该留下
	err := p.WithTx(ctx, func(tx Store) error {
		if err := tx.CreateChar(ctx, &domain.Character{AccountID: acc.ID, Slot: 0, Name: "勇士A", Pos: domain.Pos{MapID: 7}}); err != nil {
			return err
		}
		return tx.CreateChar(ctx, &domain.Character{AccountID: acc.ID, Slot: 1, Name: "勇士A", Pos: domain.Pos{MapID: 7}}) // 重名, 报错
	})
	if err == nil {
		t.Fatal("事务应失败")
	}
	if _, err := p.CharByName(ctx, "勇士A"); err != domain.ErrCharNotFound {
		t.Fatalf("回滚后不该有角色, 得 %v", err)
	}
}

// 背包存档往返: 存进去再读出来必须一模一样。
// 这一条打真库 —— 列名/类型对不上只有真写一次才会暴露。
func TestBagRoundTrip(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	acc, err := st.CreateAccount(ctx, "bagtest", "")
	if err == ErrDup {
		acc, _ = st.AccountByName(ctx, "bagtest")
	} else if err != nil {
		t.Fatal(err)
	}
	if old, _ := st.CharByName(ctx, "背包测试"); old != nil {
		st.DeleteChar(ctx, old.ID)
	}
	ch := &domain.Character{AccountID: acc.ID, Slot: 0, Name: "背包测试",
		Race: domain.Warrior, Level: 7, Base: domain.StartingBase(domain.Warrior),
		FreePoints: 12, BagSlots: 40, Pos: domain.Pos{MapID: 7, X: 1, Y: 2},
		Attrs: []int32{}}
	if err := st.CreateChar(ctx, ch); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.DeleteChar(context.Background(), ch.ID) })

	bag := domain.NewBag(40)
	bag.Set(0, domain.Stack{Item: 4103, Count: 37})                            // 树枝
	bag.Set(5, domain.Stack{UID: 90001, Item: 4001, Count: 1, Durability: 63}) // 一把用旧的剑
	bag.Set(39, domain.Stack{Item: 3002, Count: 99})                           // 最后一格

	if err := st.SaveSnapshot(ctx, domain.Snapshot{Char: ch, Bag: bag}); err != nil {
		t.Fatal(err)
	}

	got, err := st.LoadBag(ctx, ch.ID, 40)
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range []int{0, 5, 39} {
		if got.At(slot) != bag.At(slot) {
			t.Errorf("第 %d 格往返后不一致\n  存 %+v\n  读 %+v", slot, bag.At(slot), got.At(slot))
		}
	}
	if got.FreeSlots() != 37 {
		t.Errorf("应有 37 个空格, 实际 %d", got.FreeSlots())
	}

	// 角色行上的新列也要存住
	back, err := st.CharByName(ctx, "背包测试")
	if err != nil || back == nil {
		t.Fatal("读不回角色")
	}
	if back.FreePoints != 12 {
		t.Errorf("自由点应是 12, 实际 %d", back.FreePoints)
	}
	if back.Base != domain.StartingBase(domain.Warrior) {
		t.Errorf("六维没存住: %+v", back.Base)
	}

	// 装备也要一起往返
	worn := domain.NewEquipSet()
	worn.Set(domain.SlotWeapon, domain.Stack{UID: 90002, Item: 4002, Count: 1, Durability: 77})
	worn.Set(domain.SlotBody, domain.Stack{Item: 5001, Count: 1, Durability: 80})
	if err := st.SaveSnapshot(ctx, domain.Snapshot{Char: ch, Bag: bag, Worn: worn}); err != nil {
		t.Fatal(err)
	}
	gotWorn, err := st.LoadEquips(ctx, ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotWorn.Count() != 2 {
		t.Fatalf("应穿着 2 件, 实际 %d", gotWorn.Count())
	}
	if w := gotWorn.At(domain.SlotWeapon); w.Item != 4002 || w.Durability != 77 || w.UID != 90002 {
		t.Errorf("武器槽往返后不一致: %+v", w)
	}
	if gotWorn.At(domain.SlotBody).Count != 1 {
		t.Error("装备永远是一件一格, 读回来 Count 应是 1")
	}

	// 脱下之后库里也要少
	worn.Set(domain.SlotBody, domain.Stack{})
	if err := st.SaveSnapshot(ctx, domain.Snapshot{Char: ch, Bag: bag, Worn: worn}); err != nil {
		t.Fatal(err)
	}
	if w2, _ := st.LoadEquips(ctx, ch.ID); w2.Count() != 1 {
		t.Fatalf("脱下之后应只剩 1 件, 实际 %d", w2.Count())
	}

	// 整包重写: 少了东西也要真的少
	bag.RemoveAt(5, 1)
	if err := st.SaveSnapshot(ctx, domain.Snapshot{Char: ch, Bag: bag}); err != nil {
		t.Fatal(err)
	}
	got2, _ := st.LoadBag(ctx, ch.ID, 40)
	if !got2.At(5).Empty() {
		t.Fatal("扔掉的东西还在库里 —— 整包重写没删干净")
	}
}

// TestPetsRoundTrip 宠物存档的往返。
//
// **必须读回来对**：只测"写进去了"的话，SELECT 常量漏一列这种事查不出来 ——
// 这个坑在角色表上踩过一次(加了列却没加进 charCols)。
func TestPetsRoundTrip(t *testing.T) {
	p := testStore(t)
	ctx := context.Background()

	acc, err := p.CreateAccount(ctx, "petowner", "h")
	if err != nil {
		t.Fatalf("建账号: %v", err)
	}
	c := &domain.Character{
		AccountID: acc.ID, Slot: 0, Name: "养宠的", Race: domain.Warrior,
		Level: 30, Pos: domain.Pos{MapID: 7, X: 100, Y: 100},
		Attrs: []int32{0},
	}
	if err := p.CreateChar(ctx, c); err != nil {
		t.Fatalf("建角色: %v", err)
	}

	// 没养宠时该是空的, 不是报错
	if got, err := p.LoadPets(ctx, c.ID); err != nil || len(got) != 0 {
		t.Fatalf("新角色的宠物: %v / %v", got, err)
	}

	// 宠物背包上线后，每只宠物都要在背包第 4 页有一个载体物品：
	// char_pet_items.item_uid 外键指向 item_instances，而且 validatePetItems
	// 要求载体与宠物一一对应（缺一个或多一个都会被拒）。所以背包必须和宠物
	// 在同一次快照里提交。
	bag := domain.NewBag(domain.DefaultBagSlots)
	bag.Set(3*domain.BagPageSlots, domain.Stack{
		UID: 700001, Item: 1800001006, Count: 1, InstanceKind: domain.ItemInstancePet})
	bag.Set(3*domain.BagPageSlots+1, domain.Stack{
		UID: 700002, Item: 1800001074, Count: 1, InstanceKind: domain.ItemInstancePet})

	c.Pets = []domain.PetInstance{
		{ID: 1, Def: 1006, Name: "小龟", Level: 7, Exp: 123, ItemUID: 700001,
			Base: domain.Base{STR: 11, VIT: 24, INT: 18, SPI: 26, AGI: 8, DEX: 8},
			HP:   216, MP: 234, Trust: 60, Starve: 40},
		{ID: 2, Def: 1074, Level: 1, ItemUID: 700002, Base: domain.Base{VIT: 8, SPI: 6},
			HP: 72, MP: 54, Trust: 50, Starve: 50},
	}
	if err := p.SaveSnapshot(ctx, domain.Snapshot{Char: c, Bag: bag}); err != nil {
		t.Fatalf("存档: %v", err)
	}

	got, err := p.LoadPets(ctx, c.ID)
	if err != nil {
		t.Fatalf("读宠物: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("读回 %d 只, 存了 2 只", len(got))
	}
	// 逐字段对 —— 少读一列在这里才会暴露
	for i, want := range c.Pets {
		if !reflect.DeepEqual(got[i], want) {
			t.Fatalf("第 %d 只对不上:\n存 %+v\n取 %+v", i, want, got[i])
		}
	}

	// 放生最后一只: 空切片也必须写进去, 否则重登它又回来了
	c.Pets = nil
	if err := p.SaveSnapshot(ctx, domain.Snapshot{Char: c}); err != nil {
		t.Fatalf("再存档: %v", err)
	}
	if got, _ := p.LoadPets(ctx, c.ID); len(got) != 0 {
		t.Fatalf("放生后还剩 %d 只 —— 空的没写进去", len(got))
	}
}

// TestFriendsAreBidirectional 锁住"双向存两行"。
//
// 存单向的话，"我上线时通知我的好友们"就得反查「谁把我加成了好友」——
// 全表扫描，而且随在线人数增长。双向之后那件事退化成遍历自己的列表。
// 代价是加/删要写两行，所以**必须同事务**，否则会出现
// "我列表里有他、他列表里没我"，表现为"他上线我收到通知，我上线他收不到"。
func TestFriendsAreBidirectional(t *testing.T) {
	p := testStore(t)
	ctx := context.Background()

	acc, err := p.CreateAccount(ctx, "friendacc", "h")
	if err != nil {
		t.Fatalf("建账号: %v", err)
	}
	mk := func(slot int32, name string) *domain.Character {
		c := &domain.Character{AccountID: acc.ID, Slot: slot, Name: name,
			Race: domain.Warrior, Level: 10,
			Pos: domain.Pos{MapID: 7}, Attrs: []int32{0}}
		if err := p.CreateChar(ctx, c); err != nil {
			t.Fatalf("建角色 %s: %v", name, err)
		}
		return c
	}
	甲, 乙 := mk(0, "好友甲"), mk(1, "好友乙")

	// 一开始谁都没有
	if n, _ := p.CountFriends(ctx, 甲.ID); n != 0 {
		t.Fatalf("新角色有 %d 个好友", n)
	}

	if err := p.AddFriend(ctx, 甲.ID, 乙.ID); err != nil {
		t.Fatalf("加好友: %v", err)
	}
	// **两边都要有**
	for _, c := range []struct {
		me, other *domain.Character
	}{{甲, 乙}, {乙, 甲}} {
		list, err := p.LoadFriends(ctx, c.me.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 {
			t.Fatalf("%s 有 %d 个好友, 该是 1 —— 双向没写全", c.me.Name, len(list))
		}
		if list[0].Char != domain.CharID(c.other.ID) {
			t.Fatalf("%s 的好友是 %d, 该是 %d", c.me.Name, list[0].Char, c.other.ID)
		}
		// 名字从角色表连出来, 不冗余存 —— 对方改名后列表要跟着变
		if list[0].Name != c.other.Name {
			t.Fatalf("好友名字是 %q, 该是 %q", list[0].Name, c.other.Name)
		}
	}

	// 重复加不该报错也不该变成两行
	if err := p.AddFriend(ctx, 甲.ID, 乙.ID); err != nil {
		t.Fatalf("重复加好友报错了: %v", err)
	}
	if n, _ := p.CountFriends(ctx, 甲.ID); n != 1 {
		t.Fatalf("重复加之后有 %d 个好友", n)
	}

	// 备注只改自己这边 —— 我给他起的名字不该出现在他的列表里
	if err := p.SetFriendRemark(ctx, 甲.ID, 乙.ID, "老乙"); err != nil {
		t.Fatal(err)
	}
	mine, _ := p.LoadFriends(ctx, 甲.ID)
	if mine[0].Remark != "老乙" || mine[0].DisplayName() != "老乙" {
		t.Fatalf("备注是 %q", mine[0].Remark)
	}
	his, _ := p.LoadFriends(ctx, 乙.ID)
	if his[0].Remark != "" {
		t.Fatalf("我的备注跑到了对方列表里: %q", his[0].Remark)
	}
	if his[0].DisplayName() != 甲.Name {
		t.Fatalf("没备注时该显示角色名, 得到 %q", his[0].DisplayName())
	}

	// **单方面删除也是双向的** —— 留着单边会让对方一直收到我的上下线通知
	if err := p.RemoveFriend(ctx, 甲.ID, 乙.ID); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*domain.Character{甲, 乙} {
		if n, _ := p.CountFriends(ctx, c.ID); n != 0 {
			t.Fatalf("%s 删完还剩 %d 个好友 —— 单边残留", c.Name, n)
		}
	}
}

// 不能加自己。数据库那条 CHECK 是最后一道防线。
func TestCannotFriendYourself(t *testing.T) {
	p := testStore(t)
	ctx := context.Background()
	acc, _ := p.CreateAccount(ctx, "selfacc", "h")
	c := &domain.Character{AccountID: acc.ID, Name: "自恋狂", Race: domain.Warrior,
		Level: 1, Pos: domain.Pos{MapID: 7}, Attrs: []int32{0}}
	if err := p.CreateChar(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := p.AddFriend(ctx, c.ID, c.ID); err == nil {
		t.Fatal("加了自己为好友")
	}
	if n, _ := p.CountFriends(ctx, c.ID); n != 0 {
		t.Fatalf("有 %d 个好友", n)
	}
}
