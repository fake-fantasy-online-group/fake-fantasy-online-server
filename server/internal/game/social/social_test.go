package social

import (
	"sync"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func req(from domain.CharID, name string) Request {
	return Request{From: from, FromName: name}
}

func TestAddAndTake(t *testing.T) {
	r := NewRequests()
	if !r.Add(200, req(100, "甲")) {
		t.Fatal("第一次发该返回 true")
	}
	got, ok := r.Take(200, 100)
	if !ok || got.FromName != "甲" {
		t.Fatalf("取到 %+v ok=%v", got, ok)
	}
	// 取过就没了
	if _, ok := r.Take(200, 100); ok {
		t.Fatal("同一个请求取了两次")
	}
}

// 连点十次加好友不该让对方弹十次窗。
func TestDuplicateRequestDoesNotRepop(t *testing.T) {
	r := NewRequests()
	if !r.Add(200, req(100, "甲")) {
		t.Fatal("第一次该返回 true")
	}
	for i := 0; i < 9; i++ {
		if r.Add(200, req(100, "甲")) {
			t.Fatal("重复发还返回 true —— 对方会被弹窗刷屏")
		}
	}
	if len(r.List(200)) != 1 {
		t.Fatalf("收件箱里有 %d 条, 该合并成 1 条", len(r.List(200)))
	}
}

// 一个脚本给同一个人发几万条请求, 既是内存问题也是骚扰问题。
func TestPendingIsCapped(t *testing.T) {
	r := NewRequests()
	for i := 1; i <= MaxPendingPerTarget*3; i++ {
		r.Add(999, req(domain.CharID(i), "路人"))
	}
	if n := len(r.List(999)); n > MaxPendingPerTarget {
		t.Fatalf("挂了 %d 条, 上限该是 %d", n, MaxPendingPerTarget)
	}
}

// 空盒子要收掉, 否则每个登录过的人都会留一个空 map。
func TestEmptyBoxIsCollected(t *testing.T) {
	r := NewRequests()
	r.Add(200, req(100, "甲"))
	if r.Count() != 1 {
		t.Fatalf("有 %d 个收件箱", r.Count())
	}
	r.Take(200, 100)
	if r.Count() != 0 {
		t.Fatalf("取空之后还留着 %d 个收件箱", r.Count())
	}
}

func TestClearOnLogout(t *testing.T) {
	r := NewRequests()
	r.Add(200, req(100, "甲"))
	r.Add(200, req(300, "丙"))
	r.Clear(200)
	if len(r.List(200)) != 0 {
		t.Fatal("登出没清空收件箱")
	}
	if r.Count() != 0 {
		t.Fatalf("还剩 %d 个收件箱", r.Count())
	}
}

// 发件人下线时他发出去的请求要全部撤回 ——
// 留着的话对方点了同意, 而他早就不在了, 点了什么都不会发生。
func TestDropFromWithdrawsEverywhere(t *testing.T) {
	r := NewRequests()
	r.Add(200, req(100, "甲"))
	r.Add(300, req(100, "甲"))
	r.Add(300, req(400, "丁"))

	r.DropFrom(100)

	if len(r.List(200)) != 0 {
		t.Fatal("200 那边还挂着 100 的请求")
	}
	// 400 的请求不受影响
	got := r.List(300)
	if len(got) != 1 || got[0].From != 400 {
		t.Fatalf("300 的收件箱成了 %+v, 该只剩 400 那条", got)
	}
	// 200 的盒子空了该收掉, 300 的还有一条要留着
	if r.Count() != 1 {
		t.Fatalf("剩 %d 个收件箱, 该是 1", r.Count())
	}
}

func TestConcurrentRequests(t *testing.T) {
	r := NewRequests()
	var wg sync.WaitGroup
	for i := 1; i <= 30; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			from := domain.CharID(n)
			r.Add(999, req(from, "路人"))
			r.List(999)
			r.Take(999, from)
			r.DropFrom(from)
			r.Count()
		}(i)
	}
	wg.Wait()
}
