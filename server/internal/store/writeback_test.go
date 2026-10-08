package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// slowStore 是一个可控的假存储: 能卡住写入、能报错、能计数。
// 内存实现太快, 测不出"写回队列是不是真的没让调用方等 IO"。
type slowStore struct {
	Store // 其余方法不用, 嵌进来免得写一堆空实现

	mu     sync.Mutex
	writes map[int64][]int32 // charID -> 依次写入的等级
	failN  int               // 前 failN 次写入报错
	calls  int

	// gate 控制写入节奏: nil = 不拦; 非 nil 时每次写入先从里面取一个令牌。
	// **建好之后只 close, 不重新赋值** —— 赋值会和写入 goroutine 竞态,
	// 而 close 过的 channel 永远立刻返回, 是安全的"全部放行"。
	gate    chan struct{}
	blocked chan struct{} // 写入撞上 gate 时通知一声
}

func newSlowStore() *slowStore { return &slowStore{writes: map[int64][]int32{}} }

// gated 返回一个每次写入都要等令牌的假存储。
func gatedStore() *slowStore {
	s := newSlowStore()
	s.gate = make(chan struct{})
	s.blocked = make(chan struct{}, 8)
	return s
}

func (s *slowStore) SaveChar(_ context.Context, c *domain.Character) error {
	if s.gate != nil {
		select {
		case s.blocked <- struct{}{}:
		default:
		}
		<-s.gate
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.calls <= s.failN {
		return errors.New("模拟数据库抖动")
	}
	s.writes[c.ID] = append(s.writes[c.ID], c.Level)
	return nil
}

func (s *slowStore) written(id int64) []int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int32(nil), s.writes[id]...)
}

func (s *slowStore) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func ch(id int64, lv int32) domain.Snapshot {
	return domain.Snapshot{Char: &domain.Character{ID: id, Name: "测试", Level: lv}}
}

// SaveSnapshot 转调 SaveChar —— 这些用例测的是队列行为, 不是背包写入。
func (s *slowStore) SaveSnapshot(ctx context.Context, snap domain.Snapshot) error {
	return s.SaveChar(ctx, snap.Char)
}

// waitFor 轮询等条件成立。比固定 sleep 稳, 也比 sleep 快。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("超时: %s", what)
}

// 这是写回队列存在的**全部理由**: 调用方不等 IO。
// 用一个卡死的数据库来验 —— 如果 Save 是同步的, 这个测试会直接超时。
func TestSaveNeverBlocksCaller(t *testing.T) {
	st := gatedStore()
	defer close(st.gate)

	wb := NewWriteBack(st, 10*time.Millisecond, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wb.Run(ctx)

	// 先让一次写入卡在数据库里
	wb.Save(ch(1, 1))
	<-st.blocked

	// 此时数据库是堵死的。场景侧再存 100 次, 必须全部立刻返回。
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			wb.Save(ch(int64(i), int32(i)))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("数据库卡住时 Save 阻塞了 —— 场景 goroutine 会被一次慢查询拖垮")
	}
}

// 同一个角色反复存只写最新的一份。
// 队列是 map 不是 list, 所以长度上界 = 在线人数, 永远不会溢出。
func TestSaveCoalescesByCharacter(t *testing.T) {
	st := newSlowStore()
	wb := NewWriteBack(st, time.Hour, nil) // 兜底刷写关掉, 只靠 Close 触发
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for lv := int32(1); lv <= 50; lv++ {
		wb.Save(ch(7, lv))
	}
	if n := wb.Pending(); n != 1 {
		t.Fatalf("同一角色存 50 次, 队列里应只剩 1 条, 实际 %d", n)
	}

	go wb.Run(ctx)
	wb.Close(2 * time.Second)

	got := st.written(7)
	if len(got) != 1 || got[0] != 50 {
		t.Fatalf("应只写一次且是最新的等级 50, 实际 %v", got)
	}
}

// 写库失败要放回队列重试。dirty 标记在场景侧已经清了, 这里再丢就真丢档了。
func TestFailedSaveIsRetried(t *testing.T) {
	st := newSlowStore()
	st.failN = 2
	wb := NewWriteBack(st, 10*time.Millisecond, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wb.Run(ctx)

	wb.Save(ch(9, 42))
	waitFor(t, "失败两次后重试成功", func() bool { return len(st.written(9)) == 1 })
	if got := st.written(9); got[0] != 42 {
		t.Fatalf("重试写入的等级应为 42, 实际 %d", got[0])
	}
	if st.callCount() < 3 {
		t.Fatalf("应至少尝试 3 次(失败2 + 成功1), 实际 %d", st.callCount())
	}
}

// 重试**不能覆盖更新的版本**。写回一份旧数据会让玩家的进度回退, 比丢一次写更糟。
func TestRetryDoesNotOverwriteNewerVersion(t *testing.T) {
	st := gatedStore()
	st.failN = 1

	wb := NewWriteBack(st, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wb.Run(ctx)

	wb.Save(ch(5, 10)) // 这一次会失败
	<-st.blocked       // 确认它已经进到写库里卡着了

	// 趁它卡着, 塞一个更新的版本进队列
	wb.Save(ch(5, 20))

	close(st.gate) // 放行: 旧的那次写入失败, 会试图把 (5,10) 放回队列
	wb.Close(2 * time.Second)

	got := st.written(5)
	if len(got) == 0 {
		t.Fatal("最终应该写进去一次")
	}
	if last := got[len(got)-1]; last != 20 {
		t.Fatalf("最后落库的应是较新的等级 20, 实际 %d —— 重试把旧数据盖回去了", last)
	}
}

// 关服时队列必须排空。这是"关服第三步"的全部意义。
func TestCloseDrainsQueue(t *testing.T) {
	st := newSlowStore()
	wb := NewWriteBack(st, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wb.Run(ctx)

	for i := int64(1); i <= 20; i++ {
		wb.Save(ch(i, int32(i)))
	}
	wb.Close(2 * time.Second)

	if n := wb.Pending(); n != 0 {
		t.Fatalf("Close 后队列应为空, 实际还剩 %d 条 —— 这些就是丢掉的档", n)
	}
	for i := int64(1); i <= 20; i++ {
		if len(st.written(i)) != 1 {
			t.Fatalf("角色 %d 没被写入", i)
		}
	}
}

// Run 的调用方若取消 ctx，也要排空当时已入队的快照。主程序的停服
// 信号不走这条路：它必须等 Router.Drain 交出最终快照后再 Close。
func TestContextCancelDrains(t *testing.T) {
	st := newSlowStore()
	wb := NewWriteBack(st, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())

	stopped := make(chan struct{})
	go func() { wb.Run(ctx); close(stopped) }()

	wb.Save(ch(3, 99))
	cancel()

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Run 没在 ctx 取消后退出")
	}
	if got := st.written(3); len(got) != 1 || got[0] != 99 {
		t.Fatalf("ctx 取消时应把队列写完, 实际 %v", got)
	}
}

// 服务端停服信号只停网关与场景，不能提前停写回循环。场景在
// Router.Drain 中才会交出最后一批在线存档；这批必须由随后的 Close 排空。
func TestShutdownSignalBeforeFinalSceneSave(t *testing.T) {
	st := newSlowStore()
	wb := NewWriteBack(st, time.Hour, nil)
	go wb.Run(context.Background()) // 与网关 signal context 刻意分离

	// 模拟网关已因停服信号退出后，Router.Drain 才收到场景最终快照。
	wb.Save(ch(7, 88))
	wb.Close(2 * time.Second)

	if got := st.written(7); len(got) != 1 || got[0] != 88 {
		t.Fatalf("场景最终存档未在写回关闭前落盘: %v", got)
	}
	if n := wb.Pending(); n != 0 {
		t.Fatalf("关服后写回队列仍有 %d 条未落盘", n)
	}
}

// ── 关服链路 ──
//
// 这一段测的是**最贵也最难发现**的一类 bug：每次重启都悄悄丢一批进度。
// 正常路径已经有测试了，缺的是出事那一侧 —— 数据库卡住时 Close 会怎样。

// 数据库卡死时 Close 必须**超时返回并报出剩多少**，不能永远等下去。
//
// 永远等的话，关服脚本会挂在那里，运维只能 kill -9 —— 那才是真的全丢。
func TestCloseTimesOutInsteadOfHanging(t *testing.T) {
	st := gatedStore() // 每次写入都要等令牌, 而这个测试永不发令牌
	wb := NewWriteBack(st, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wb.Run(ctx)

	for i := int64(1); i <= 5; i++ {
		wb.Save(ch(i, 10))
	}
	// 等第一次写入真的撞上闸门, 确保 Close 时队列正忙
	select {
	case <-st.blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("写入没有撞上闸门, 测试前提不成立")
	}

	start := time.Now()
	wb.Close(150 * time.Millisecond)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("Close 等了 %v —— 数据库卡住时它必须超时返回", elapsed)
	}
	// 超时之后队列里还剩着东西, 那就是没落盘的档。
	// **这个数必须查得到** —— 它是运维判断"这次重启丢了多少"的唯一依据
	if wb.Pending() == 0 {
		t.Fatal("闸门没放行却报告队列已空")
	}
	close(st.gate) // 放行, 免得 Run 的 goroutine 泄漏到别的测试里
}

// Close 可以重复调。关服脚本重试、或者信号处理和主流程都调了一次，都不该 panic。
func TestCloseIsIdempotent(t *testing.T) {
	st := newSlowStore()
	wb := NewWriteBack(st, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wb.Run(ctx)

	wb.Save(ch(1, 5))
	wb.Close(2 * time.Second)
	wb.Close(2 * time.Second) // 第二次: close 已关闭的 channel 会 panic, once 挡住它
	wb.Close(2 * time.Second)

	if got := st.written(1); len(got) != 1 {
		t.Fatalf("重复 Close 导致写了 %d 次", len(got))
	}
}

// Close 之后再 Save 不该 panic，也不该把数据吞进一个永远不会再排空的队列。
//
// 关服时序上这是真会发生的：场景还在跑最后几帧，队列已经被关了。
func TestSaveAfterCloseDoesNotPanic(t *testing.T) {
	st := newSlowStore()
	wb := NewWriteBack(st, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wb.Run(ctx)

	wb.Close(2 * time.Second)
	wb.Save(ch(7, 3)) // 迟到的存档

	// 不 panic 就算过。它写不写得进去取决于时序,
	// 但**必须能查出来还剩几条** —— 否则运维连"丢了没"都不知道
	_ = wb.Pending()
}
