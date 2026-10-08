package store

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// WriteBack 是存档写回队列 —— 场景与数据库之间的那道缓冲。
//
// 它存在的唯一目的: 让场景 goroutine 调 Save 时**立刻返回**。
// 场景里做同步写库, 一次慢查询就能卡住整张图的所有人, 而且这种卡顿是随机的、极难定位。
//
// 两个设计要点:
//
//	按角色 id 合并  队列是 map 不是 list, 同一个角色反复存只保留最新的一份。
//	              因此队列长度上界 = 在线人数, **永远不会溢出**, 也就永远不需要
//	              "队列满了丢弃" —— 丢一条移动广播可以接受, 丢一次存档不行。
//	失败留在队列    写库失败就把它放回去下轮重试(除非期间有更新的版本顶掉它)。
//	              dirty 标记在场景侧已经清了, 这里再丢就真的丢档了。
type WriteBack struct {
	st       Store
	log      *slog.Logger
	interval time.Duration

	mu      sync.Mutex
	pending map[int64]*writeBackEntry
	// inflight 是**已经从 pending 取出、正在写库**的快照。
	//
	// 没有它的话 Pending() 会在最要紧的时刻撒谎: drain 先把整批搬走再写库,
	// 数据库卡住时 pending 是空的, 而那批档一条都没落盘 ——
	// 关服超时那条日志会报"剩余 0", 运维据此以为一条没丢。
	// 按角色保留 entry，而不只记一个计数，是为了让“当前版本提交成功”的 waiter
	// 能精确跟随那份快照；同角色的新版本可以在旧版本 IO 期间继续进入 pending。
	inflight   map[int64]*writeBackEntry
	pairs      map[*writeBackPair]struct{}
	pairByChar map[int64]*writeBackPair
	wake       chan struct{}

	done   chan struct{}
	closed chan struct{}
	once   sync.Once
}

// writeBackEntry 把一份待写快照与等待其“成功提交”的观察者绑在一起。
// waiter 只在 SaveSnapshot 返回 nil 后关闭；暂时性失败会连同快照一起重试，
// 若期间出现更新版本，则 waiter 转移到更新版本，绝不能因合并而丢失。
type writeBackEntry struct {
	snap       domain.Snapshot
	waiters    []chan struct{}
	mutation   SnapshotMutation
	mutationCh chan error
}

type writeBackPair struct {
	snaps    [2]domain.Snapshot
	waiters  [2][]chan struct{}
	mutation PairSnapshotMutation
	result   chan error
	inflight bool
}

// SnapshotMutation 在写回队列的角色顺序线上提交一次“角色快照 + 外部物权”事务。
// 实现必须自己使用 Store.WithTx；返回错误时整笔事务视为没有发生。
type SnapshotMutation func(context.Context, Store, domain.Snapshot) error

// PairSnapshotMutation 在同一数据库事务内提交两个在线角色的最终候选快照。
// 摆摊成交使用它，保证货物和铜币不会只在买方或卖方一侧生效。
type PairSnapshotMutation func(context.Context, Store, domain.Snapshot, domain.Snapshot) error

// NewWriteBack 建写回队列。interval 是空闲时的兜底刷写间隔(0 用 1 秒)。
func NewWriteBack(st Store, interval time.Duration, log *slog.Logger) *WriteBack {
	if interval <= 0 {
		interval = time.Second
	}
	if log == nil {
		log = slog.Default()
	}
	return &WriteBack{
		st: st, log: log, interval: interval,
		pending:  map[int64]*writeBackEntry{},
		inflight: map[int64]*writeBackEntry{},
		pairs:    map[*writeBackPair]struct{}{}, pairByChar: map[int64]*writeBackPair{},
		wake:   make(chan struct{}, 1),
		done:   make(chan struct{}),
		closed: make(chan struct{}),
	}
}

// Save 把角色排进写回队列。**这是场景 goroutine 唯一允许调的存档入口。**
//
// 只加一次极短的互斥锁做 map 写入 —— 这与"场景里不许等锁"并不矛盾:
// 那条纪律禁的是**可能横跨 IO 的锁**, 而这把锁只护住一次 map 赋值, 不可能被 IO 持有。
func (w *WriteBack) Save(s domain.Snapshot) {
	w.enqueue(s, nil)
}

// SaveAck 与 Save 一样只排队、绝不等待 IO，并返回一个“已成功提交”通知。
// 数据库暂时失败时通知保持未完成，跟着该角色的重试或更新快照继续走；调用方因此
// 可以拿它做离场屏障，而不会把一次失败误当成已经安全落盘。
//
// Char 为空是调用方违反快照契约：没有任何东西可提交，返回的通知不会伪装成功。
func (w *WriteBack) SaveAck(s domain.Snapshot) <-chan struct{} {
	done := make(chan struct{})
	if s.Char == nil {
		w.log.Error("拒绝为空角色创建存档提交确认")
		return done
	}
	w.enqueue(s, done)
	return done
}

// CommitMutation 把跨邮件/背包等边界的事务排进与普通存档相同的角色顺序线。
// 返回通道只产生一次结果；它是会话层等待数据库提交的边界，不在场景 actor 内等待。
func (w *WriteBack) CommitMutation(s domain.Snapshot, mutation SnapshotMutation) <-chan error {
	result := make(chan error, 1)
	if s.Char == nil || mutation == nil {
		result <- fmt.Errorf("store: 空角色或空快照事务")
		close(result)
		return result
	}
	id := s.Char.ID
	w.mu.Lock()
	if w.pairByChar[id] != nil {
		w.mu.Unlock()
		result <- fmt.Errorf("store: 角色 %d 正在提交双角色事务", id)
		close(result)
		return result
	}
	if queued := w.pending[id]; queued != nil {
		if queued.mutation != nil {
			w.mu.Unlock()
			result <- fmt.Errorf("store: 角色 %d 已有快照事务在排队", id)
			close(result)
			return result
		}
		queued.snap = s
		queued.mutation = mutation
		queued.mutationCh = result
	} else {
		w.pending[id] = &writeBackEntry{snap: s, mutation: mutation, mutationCh: result}
	}
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
	return result
}

// CommitPairMutation 把两名角色挂到同一条写回顺序线上。调用方必须在结果返回前
// 保持两边库存保留态；失败后回滚内存并重新 Save，成功后候选快照已同时落盘。
func (w *WriteBack) CommitPairMutation(a, b domain.Snapshot, mutation PairSnapshotMutation) <-chan error {
	result := make(chan error, 1)
	if a.Char == nil || b.Char == nil || a.Char.ID == b.Char.ID || mutation == nil {
		result <- fmt.Errorf("store: 非法双角色快照事务")
		close(result)
		return result
	}
	ids := [2]int64{a.Char.ID, b.Char.ID}
	pair := &writeBackPair{snaps: [2]domain.Snapshot{a, b}, mutation: mutation, result: result}
	w.mu.Lock()
	if w.pairByChar[ids[0]] != nil || w.pairByChar[ids[1]] != nil {
		w.mu.Unlock()
		result <- fmt.Errorf("store: 双角色事务参与者已有事务在排队")
		close(result)
		return result
	}
	for _, id := range ids {
		if queued := w.pending[id]; queued != nil && queued.mutation != nil {
			w.mu.Unlock()
			result <- fmt.Errorf("store: 角色 %d 已有快照事务在排队", id)
			close(result)
			return result
		}
	}
	for i, id := range ids {
		if queued := w.pending[id]; queued != nil {
			pair.waiters[i] = append(pair.waiters[i], queued.waiters...)
			delete(w.pending, id)
		}
	}
	w.pairs[pair] = struct{}{}
	w.pairByChar[ids[0]], w.pairByChar[ids[1]] = pair, pair
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
	return result
}

func (w *WriteBack) enqueue(s domain.Snapshot, waiter chan struct{}) {
	if s.Char == nil {
		return
	}
	id := s.Char.ID
	w.mu.Lock()
	if pair := w.pairByChar[id]; pair != nil && !pair.inflight {
		for i := range pair.snaps {
			if pair.snaps[i].Char != nil && pair.snaps[i].Char.ID == id {
				pair.snaps[i] = s
				if waiter != nil {
					pair.waiters[i] = append(pair.waiters[i], waiter)
				}
				w.mu.Unlock()
				return
			}
		}
	}
	if queued := w.pending[id]; queued != nil {
		// pending 中的旧版本还没有开始 IO，可以直接由更新快照顶掉；已经挂在
		// 旧版本上的确认必须保留，因为新快照包含并超越了它所代表的状态。
		queued.snap = s
		if waiter != nil {
			queued.waiters = append(queued.waiters, waiter)
		}
	} else {
		entry := &writeBackEntry{snap: s}
		if waiter != nil {
			entry.waiters = append(entry.waiters, waiter)
		}
		w.pending[id] = entry
	}
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default: // 已经有人叫醒过了, 不用重复叫
	}
}

// Run 跑写回循环, 阻塞到 ctx 取消或 Close。应在独立 goroutine 里调。
//
// 退出前会把队列排空 —— 关服时"最后那批存档"就是靠这里。
func (w *WriteBack) Run(ctx context.Context) {
	defer close(w.closed)
	tk := time.NewTicker(w.interval)
	defer tk.Stop()

	for {
		select {
		case <-ctx.Done():
			w.drainUntilEmpty(context.WithoutCancel(ctx))
			return
		case <-w.done:
			w.drainUntilEmpty(context.WithoutCancel(ctx))
			return
		case <-w.wake:
			w.drain(ctx)
		case <-tk.C:
			w.drain(ctx)
		}
	}
}

// drainUntilEmpty 是停止阶段的可靠排空：一次数据库失败会把 entry 与 waiter
// 放回 pending，因此不能像普通 tick 那样只 drain 一轮就退出。它持续重试到真正
// 没有 pending/inflight；外层 Close 的 timeout 只负责报告停服失败，绝不能关闭
// w.closed 冒充“已经排空”。生产关服顺序保证调用它之后不会再有场景入队。
func (w *WriteBack) drainUntilEmpty(ctx context.Context) {
	for {
		w.drain(ctx)
		w.mu.Lock()
		remaining := len(w.pending) + len(w.inflight)
		w.mu.Unlock()
		if remaining == 0 {
			return
		}
		time.Sleep(w.interval)
	}
}

// drain 把当前队列全部写库。失败的放回队列, 但**不覆盖更新的版本** ——
// 重试写回一份旧数据会把玩家的进度回退, 那比丢一次写还糟。
func (w *WriteBack) drain(ctx context.Context) {
	w.drainPairs(ctx)
	w.mu.Lock()
	if len(w.pending) == 0 {
		w.mu.Unlock()
		return
	}
	batch := w.pending
	w.pending = make(map[int64]*writeBackEntry, len(batch))
	for id, entry := range batch {
		w.inflight[id] = entry
	}
	w.mu.Unlock()

	for id, entry := range batch {
		snap := entry.snap
		var err error
		if entry.mutation != nil {
			err = entry.mutation(ctx, w.st, snap)
		} else {
			err = w.st.SaveSnapshot(ctx, snap)
		}
		// **失败的条目在同一把锁里放回队列**, 与 inflight 减一是原子的 ——
		// 分成两步的话中间那一瞬它既不在 inflight 也不在 pending, 查出来就是丢了
		var committed []chan struct{}
		w.mu.Lock()
		delete(w.inflight, id)
		if err != nil && entry.mutation == nil {
			if newer := w.pending[id]; newer != nil {
				// 旧版本写失败但已有更新版本时，不能把旧快照塞回去覆盖更新状态；
				// 同时也不能丢掉旧版本的提交观察者，它们改由更新版本兑现。
				newer.waiters = append(newer.waiters, entry.waiters...)
			} else {
				w.pending[id] = entry
			}
		} else if err == nil {
			committed = entry.waiters
		} else {
			// 自定义事务由会话层根据业务错误回滚内存状态；期间合并进来的更新
			// 仍含“已预留”的物品，不能让它随后单独落库。回滚完成后场景会重新 Save。
			delete(w.pending, id)
		}
		w.mu.Unlock()
		if entry.mutationCh != nil {
			entry.mutationCh <- err
			close(entry.mutationCh)
		}
		if err != nil {
			if entry.mutation == nil {
				w.log.Error("存档失败, 将重试", "char", snap.Char.Name, "id", snap.Char.ID, "err", err)
			} else {
				w.log.Warn("角色快照事务失败，等待业务回滚", "char", snap.Char.Name, "id", snap.Char.ID, "err", err)
			}
			continue
		}
		for _, waiter := range committed {
			close(waiter)
		}
	}
}

func (w *WriteBack) drainPairs(ctx context.Context) {
	w.mu.Lock()
	if len(w.pairs) == 0 {
		w.mu.Unlock()
		return
	}
	pairs := make([]*writeBackPair, 0, len(w.pairs))
	for pair := range w.pairs {
		// 旧的单角色写入若仍在 IO，留到下一轮；写回循环本身串行，通常不会命中。
		if w.inflight[pair.snaps[0].Char.ID] != nil || w.inflight[pair.snaps[1].Char.ID] != nil {
			continue
		}
		delete(w.pairs, pair)
		pair.inflight = true
		pairs = append(pairs, pair)
	}
	w.mu.Unlock()
	for _, pair := range pairs {
		err := pair.mutation(ctx, w.st, pair.snaps[0], pair.snaps[1])
		ids := [2]int64{pair.snaps[0].Char.ID, pair.snaps[1].Char.ID}
		var committed []chan struct{}
		w.mu.Lock()
		for i, id := range ids {
			delete(w.pairByChar, id)
			if err != nil {
				// 保留态即将由场景回滚；期间产生的候选普通快照不能单独落盘。
				delete(w.pending, id)
			} else {
				committed = append(committed, pair.waiters[i]...)
			}
		}
		w.mu.Unlock()
		pair.result <- err
		close(pair.result)
		if err != nil {
			w.log.Warn("双角色快照事务失败，等待业务回滚",
				"charA", pair.snaps[0].Char.Name, "charB", pair.snaps[1].Char.Name, "err", err)
			continue
		}
		for _, waiter := range committed {
			close(waiter)
		}
	}
}

// Close 请求停止并等队列排空。关服第三步(前两步: 网关停止接客、场景存档)。
// 超时或 Run 已退出后仍有迟到快照时返回错误，让主程序以非零状态退出；只写日志会让
// 外层误报一次“正常关服”，而那些未落盘角色正是最需要被明确暴露的失败。
func (w *WriteBack) Close(timeout time.Duration) error {
	w.once.Do(func() { close(w.done) })
	select {
	case <-w.closed:
		if n := w.Pending(); n != 0 {
			err := fmt.Errorf("store: 写回循环已停止但仍有 %d 个角色未落盘", n)
			w.log.Error("写回队列停止后仍有存档未落盘", "剩余", n)
			return err
		}
		return nil
	case <-time.After(timeout):
		n := w.Pending()
		err := fmt.Errorf("store: 写回队列在 %s 内未排空，仍有 %d 个角色未落盘", timeout, n)
		w.log.Error("写回队列排空超时, 有存档未落盘", "剩余", n, "timeout", timeout)
		return err
	}
}

// Pending 返回**还没落盘**的角色数：排队的 + 正在写的。
//
// 把 inflight 算进来是刻意的: 这个数的唯一用途是回答"这次关服丢了多少",
// 而正在写库却卡住的那些恰恰是丢得最多的一批。
func (w *WriteBack) Pending() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.pending) + len(w.inflight) + len(w.pairByChar)
}
