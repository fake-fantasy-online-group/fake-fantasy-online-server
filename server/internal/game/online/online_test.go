package online

import (
	"sync"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func loc(char domain.CharID, name string, ent domain.EntityID, mapID int32) Location {
	return Location{Char: char, Name: name, Entity: ent,
		Scene: domain.SceneID{MapID: mapID}}
}

func TestEnterAndFind(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Find(100); ok {
		t.Fatal("空索引里找到了人")
	}

	r.Enter(loc(100, "甲", 7, 1))
	got, ok := r.Find(100)
	if !ok {
		t.Fatal("进图之后查不到")
	}
	if got.Entity != 7 || got.Scene.MapID != 1 {
		t.Fatalf("位置是 %+v", got)
	}
	if r.Count() != 1 {
		t.Fatalf("在线 %d 人", r.Count())
	}
}

// 传送落地会再调一次 Enter，位置必须被覆盖 ——
// 不覆盖的话私聊会投给一个已经没有这个实体的场景，消息静静消失。
func TestEnterOverwritesOnTeleport(t *testing.T) {
	r := NewRegistry()
	r.Enter(loc(100, "甲", 7, 1))
	r.Enter(loc(100, "甲", 7, 285)) // 换图, 实体 id 跟着人走所以没变

	got, _ := r.Find(100)
	if got.Scene.MapID != 285 {
		t.Fatalf("传送后还记着旧图 %d", got.Scene.MapID)
	}
	if r.Count() != 1 {
		t.Fatalf("换个图变成了 %d 个人", r.Count())
	}
}

func TestFindByNameIsCaseInsensitive(t *testing.T) {
	r := NewRegistry()
	r.Enter(loc(100, "XiaoMing", 7, 1))

	for _, n := range []string{"XiaoMing", "xiaoming", "XIAOMING"} {
		if _, ok := r.FindByName(n); !ok {
			t.Fatalf("按 %q 查不到", n)
		}
	}
	if _, ok := r.FindByName("别人"); ok {
		t.Fatal("查到了不存在的人")
	}
}

// 名字里的空格是名字的一部分，不能去掉 ——
// 去掉的话"张 三"和"张三"会变成同一个人，那是两个账号。
func TestNamesWithSpacesStayDistinct(t *testing.T) {
	r := NewRegistry()
	r.Enter(loc(100, "张 三", 7, 1))
	r.Enter(loc(200, "张三", 8, 1))

	a, ok := r.FindByName("张 三")
	if !ok || a.Char != 100 {
		t.Fatalf("带空格的名字查到了 %+v", a)
	}
	b, ok := r.FindByName("张三")
	if !ok || b.Char != 200 {
		t.Fatalf("不带空格的名字查到了 %+v", b)
	}
}

func TestLeaveRemoves(t *testing.T) {
	r := NewRegistry()
	r.Enter(loc(100, "甲", 7, 1))

	if !r.Leave(100, 7) {
		t.Fatal("摘不掉")
	}
	if r.Online(100) {
		t.Fatal("摘完还在线")
	}
	if _, ok := r.FindByName("甲"); ok {
		t.Fatal("按名字还能查到 —— 反查表没摘干净")
	}
	if r.Count() != 0 {
		t.Fatalf("还剩 %d 人", r.Count())
	}
	// 再摘一次不该出事
	if r.Leave(100, 7) {
		t.Fatal("摘了两次都成功")
	}
}

// **这一条是整个包最要紧的。**
//
// 掉线重连时顺序是：新会话先 Enter，旧会话的 OnClose 才姗姗来迟。
// 摘索引不比对实体 id 的话，旧会话会把新会话的条目删掉 ——
// 之后这个人对全服"不在线"：私聊收不到、组队刷不了，
// 而他自己在游戏里玩得好好的，完全不知道发生了什么。
func TestStaleSessionCannotEvictTheNewOne(t *testing.T) {
	r := NewRegistry()
	r.Enter(loc(100, "甲", 7, 1)) // 旧连接, 实体 7

	// 重连: 新会话拿到新的实体 id
	replaced, had := r.Enter(loc(100, "甲", 99, 1))
	if !had || replaced.Entity != 7 {
		t.Fatalf("顶替时该报出旧位置, 得到 %+v had=%v", replaced, had)
	}

	// 旧会话现在才收到 OnClose, 拿着**旧的**实体 id 来摘
	if r.Leave(100, 7) {
		t.Fatal("旧会话把新会话的条目摘掉了")
	}
	got, ok := r.Find(100)
	if !ok {
		t.Fatal("重连之后人不见了 —— 私聊/组队全会失效")
	}
	if got.Entity != 99 {
		t.Fatalf("索引里是实体 %d, 该是新会话的 99", got.Entity)
	}
	// 新会话正常退出时摘得掉
	if !r.Leave(100, 99) {
		t.Fatal("新会话摘不掉自己")
	}
}

// 改名之后按旧名字不该还能找到。
func TestRenameDropsTheOldNameKey(t *testing.T) {
	r := NewRegistry()
	r.Enter(loc(100, "旧名", 7, 1))
	r.Enter(loc(100, "新名", 7, 1))

	if _, ok := r.FindByName("旧名"); ok {
		t.Fatal("按旧名字还能查到")
	}
	if got, ok := r.FindByName("新名"); !ok || got.Char != 100 {
		t.Fatalf("按新名字查到 %+v", got)
	}
}

func TestLocateSkipsOffline(t *testing.T) {
	r := NewRegistry()
	r.Enter(loc(100, "甲", 7, 1))
	r.Enter(loc(300, "丙", 9, 285))

	got := r.Locate([]domain.CharID{100, 200, 300})
	if len(got) != 2 {
		t.Fatalf("查 3 个人返回 %d 条, 该跳过不在线的那个", len(got))
	}
	seen := map[domain.CharID]bool{}
	for _, l := range got {
		seen[l.Char] = true
	}
	if !seen[100] || !seen[300] {
		t.Fatalf("返回的是 %+v", got)
	}
}

// 索引被所有连接并发读写。-race 下跑一遍。
func TestRegistryIsConcurrencySafe(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c := domain.CharID(n)
			e := domain.EntityID(n)
			r.Enter(Location{Char: c, Name: string(rune('a' + n%26)), Entity: e,
				Scene: domain.SceneID{MapID: int32(n % 4)}})
			r.Find(c)
			r.FindByName("a")
			r.Count()
			r.Locate([]domain.CharID{c})
			r.Snapshot()
			r.Leave(c, e)
		}(i)
	}
	wg.Wait()
	if r.Count() != 0 {
		t.Fatalf("跑完还剩 %d 人", r.Count())
	}
}

// 快照必须是拷贝，拿到内部映射就等于绕过锁。
func TestSnapshotIsIndependent(t *testing.T) {
	r := NewRegistry()
	r.Enter(loc(100, "甲", 7, 1))

	snap := r.Snapshot()
	snap[0].Name = "篡改"

	again, _ := r.Find(100)
	if again.Name != "甲" {
		t.Fatalf("改快照改到了索引里: %q", again.Name)
	}
}
