package party

import (
	"sync"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestInviteThenAcceptCreatesParty(t *testing.T) {
	r := NewRegistry()

	// 邀请人自己还没队伍 —— 这时**不该**先把队建出来,
	// 建了对方又不同意的话, 他就莫名其妙成了单人队的队长
	if rej := r.Invite(1, 2); rej != domain.PartyOK {
		t.Fatalf("邀请被拒: %v", rej)
	}
	if r.Count() != 0 {
		t.Fatalf("邀请阶段就建了 %d 支队 —— 对方还没同意", r.Count())
	}
	if r.PartyOf(1) != 0 {
		t.Fatal("邀请人被提前塞进了队伍")
	}

	p, rej := r.Accept(2, "乙", 1, "甲")
	if rej != domain.PartyOK {
		t.Fatalf("接受被拒: %v", rej)
	}
	if len(p.Members) != 2 {
		t.Fatalf("队里 %d 人, 该是 2", len(p.Members))
	}
	if p.Leader != 1 {
		t.Fatalf("队长是 %d, 该是邀请人 1", p.Leader)
	}
	if r.PartyOf(1) != p.ID || r.PartyOf(2) != p.ID {
		t.Fatal("反查表没更新")
	}
}

func TestCannotInviteSomeoneAlreadyInAParty(t *testing.T) {
	r := NewRegistry()
	r.Invite(1, 2)
	r.Accept(2, "乙", 1, "甲")

	if rej := r.Invite(3, 2); rej != domain.PartyAlreadyIn {
		t.Fatalf("邀请已经在队里的人该拒, 得到 %v", rej)
	}
	if rej := r.Invite(1, 1); rej != domain.PartySelfTarget {
		t.Fatalf("邀请自己该拒, 得到 %v", rej)
	}
}

func TestOnlyLeaderCanInvite(t *testing.T) {
	r := NewRegistry()
	r.Invite(1, 2)
	r.Accept(2, "乙", 1, "甲")

	if rej := r.Invite(2, 3); rej != domain.PartyNotLeader {
		t.Fatalf("队员邀请该拒, 得到 %v", rej)
	}
	if rej := r.Invite(1, 3); rej != domain.PartyOK {
		t.Fatalf("队长邀请该行, 得到 %v", rej)
	}
}

func TestPartyIsCappedAtMaxSize(t *testing.T) {
	r := NewRegistry()
	r.Invite(1, 2)
	r.Accept(2, "乙", 1, "甲")
	for c := domain.CharID(3); c <= domain.MaxPartySize; c++ {
		if rej := r.Invite(1, c); rej != domain.PartyOK {
			t.Fatalf("邀请第 %d 人被拒: %v", c, rej)
		}
		if _, rej := r.Accept(c, "丙", 1, "甲"); rej != domain.PartyOK {
			t.Fatalf("第 %d 人加入被拒: %v", c, rej)
		}
	}
	p, _ := r.Get(r.PartyOf(1))
	if len(p.Members) != domain.MaxPartySize {
		t.Fatalf("队里 %d 人, 该满 %d", len(p.Members), domain.MaxPartySize)
	}
	if rej := r.Invite(1, 99); rej != domain.PartyFull {
		t.Fatalf("满员还能邀请: %v", rej)
	}
}

// 队长走了把队长顺位给下一个，**不解散** ——
// 掉线是最常见的退队方式，解散的话整队人要重组一遍。
func TestLeaderLeavingPassesTheLead(t *testing.T) {
	r := NewRegistry()
	r.Invite(1, 2)
	r.Accept(2, "乙", 1, "甲")
	r.Invite(1, 3)
	r.Accept(3, "丙", 1, "甲")

	after, alive := r.Leave(1)
	if !alive {
		t.Fatal("三个人的队走了队长就解散了")
	}
	if after.Leader == 1 {
		t.Fatal("队长还是走掉的那个")
	}
	if !after.Has(after.Leader) {
		t.Fatal("新队长不在队里")
	}
	if len(after.Members) != 2 {
		t.Fatalf("剩 %d 人, 该是 2", len(after.Members))
	}
}

// 只剩一个人时真解散 —— 单人队没有任何意义，还会让他以为自己在组队。
func TestPartyDissolvesWhenOnlyOneLeft(t *testing.T) {
	r := NewRegistry()
	r.Invite(1, 2)
	r.Accept(2, "乙", 1, "甲")

	if _, alive := r.Leave(2); alive {
		t.Fatal("两人队走一个该解散")
	}
	if r.Count() != 0 {
		t.Fatalf("还剩 %d 支队", r.Count())
	}
	// **剩下那个人也必须被摘干净** —— 挂在一个不存在的队伍号上
	// 会让他之后所有的队友判定都失败得莫名其妙
	if got := r.PartyOf(1); got != 0 {
		t.Fatalf("解散后 1 号还挂在队伍 %d 上", got)
	}
}

func TestKick(t *testing.T) {
	r := NewRegistry()
	r.Invite(1, 2)
	r.Accept(2, "乙", 1, "甲")
	r.Invite(1, 3)
	r.Accept(3, "丙", 1, "甲")

	if _, rej := r.Kick(2, 3); rej != domain.PartyNotLeader {
		t.Fatalf("队员踢人该拒, 得到 %v", rej)
	}
	if _, rej := r.Kick(1, 1); rej != domain.PartySelfTarget {
		t.Fatalf("踢自己该拒, 得到 %v", rej)
	}
	if _, rej := r.Kick(1, 99); rej != domain.PartyNoSuchMember {
		t.Fatalf("踢不在队里的人该拒, 得到 %v", rej)
	}
	after, rej := r.Kick(1, 3)
	if rej != domain.PartyOK {
		t.Fatalf("队长踢人被拒: %v", rej)
	}
	if after.Has(3) || r.PartyOf(3) != 0 {
		t.Fatal("踢完还在队里")
	}
}

func TestSetLeaderAndDisband(t *testing.T) {
	r := NewRegistry()
	r.Invite(1, 2)
	r.Accept(2, "乙", 1, "甲")

	if rej := r.SetLeader(2, 1); rej != domain.PartyNotLeader {
		t.Fatalf("队员转让队长该拒, 得到 %v", rej)
	}
	if rej := r.SetLeader(1, 99); rej != domain.PartyNoSuchMember {
		t.Fatalf("转让给队外的人该拒, 得到 %v", rej)
	}
	if rej := r.SetLeader(1, 2); rej != domain.PartyOK {
		t.Fatalf("转让被拒: %v", rej)
	}
	p, _ := r.Get(r.PartyOf(1))
	if p.Leader != 2 {
		t.Fatalf("队长是 %d, 该是 2", p.Leader)
	}

	// 现在 1 号不是队长了, 解散不了
	if _, rej := r.Disband(1); rej != domain.PartyNotLeader {
		t.Fatalf("非队长解散该拒, 得到 %v", rej)
	}
	members, rej := r.Disband(2)
	if rej != domain.PartyOK {
		t.Fatalf("解散被拒: %v", rej)
	}
	if len(members) != 2 {
		t.Fatalf("解散时报了 %d 个成员, 该是 2 —— 少报一个那人的队伍号就清不掉", len(members))
	}
	if r.PartyOf(1) != 0 || r.PartyOf(2) != 0 {
		t.Fatal("解散后还有人挂着队伍号")
	}
}

// 取出去的必须是拷贝：拿到内部切片就等于绕过锁改名册。
func TestSnapshotIsACopy(t *testing.T) {
	r := NewRegistry()
	r.Invite(1, 2)
	p, _ := r.Accept(2, "乙", 1, "甲")

	p.Members[0].Name = "篡改"
	p.Leader = 999

	again, _ := r.Get(p.ID)
	if again.Leader == 999 {
		t.Fatal("改快照改到了名册里")
	}
	for _, m := range again.Members {
		if m.Name == "篡改" {
			t.Fatal("改快照的成员改到了名册里")
		}
	}
}

// 名册会被多个连接并发操作。-race 下跑一遍，别等到线上才发现。
func TestRegistryIsConcurrencySafe(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			leader := domain.CharID(n*10 + 1)
			member := domain.CharID(n*10 + 2)
			r.Invite(leader, member)
			r.Accept(member, "乙", leader, "甲")
			r.PartyOf(leader)
			r.Get(r.PartyOf(member))
			r.Leave(member)
		}(i)
	}
	wg.Wait()
	if r.Count() != 0 {
		t.Fatalf("跑完还剩 %d 支队", r.Count())
	}
}

func TestExpShareMath(t *testing.T) {
	// 单人不打折
	if got := domain.PartyExpShare(100, 1); got != 100 {
		t.Fatalf("单人拿 %d, 该是 100", got)
	}
	// 两人: 总量 110, 每人 55
	if got := domain.PartyExpShare(100, 2); got != 55 {
		t.Fatalf("两人每人 %d, 该是 55", got)
	}
	// 五人: 总量 140, 每人 28 —— **仍然低于单人**,
	// 组队是为了打得动更硬的怪, 不是为了刷得更快
	if got := domain.PartyExpShare(100, 5); got != 28 {
		t.Fatalf("五人每人 %d, 该是 28", got)
	}
	if domain.PartyExpShare(100, 5) >= domain.PartyExpShare(100, 1) {
		t.Fatal("五人人均不该超过单人 —— 那样所有人都会去挂机组队")
	}
	// 先乘后除: 小数值上不能被整数除法吃干净
	if got := domain.PartyExpShare(3, 2); got != 1 {
		t.Fatalf("3 经验两人分得 %d", got)
	}
}

func TestExpLevelGap(t *testing.T) {
	if !domain.CanShareExp(60, 40) {
		t.Fatal("差 20 级该还能分")
	}
	if domain.CanShareExp(60, 39) {
		t.Fatal("差 21 级不该分 —— 否则满级号带 1 级号几分钟就能拉到几十级")
	}
	// 只卡低的一侧: 高等级队员帮打低级怪是自然的
	if !domain.CanShareExp(10, 60) {
		t.Fatal("高等级队员该能分(虽然没多少)")
	}
}
