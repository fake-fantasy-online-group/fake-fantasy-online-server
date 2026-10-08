package store

import (
	"sync"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// CharacterGate 同时保护角色存档和账号钱包：一个账号同一时刻只能有一个角色拥有租约。
//
// 租约不在 TCP 断开或收到 0x1049 时立刻释放，而要等场景交出的最终快照真正提交
// 成功后才释放。这样同连接立即重进、以及新 TCP 秒连，都不能从数据库读到离场前的
// 旧 Bag/Worn 或账号彩玉，再用那份旧状态覆盖仍在重试的最终存档。
// 切换到同账号的其它角色也必须等待这份最终存档提交。
type CharacterGate interface {
	TryEnter(charID, accountID int64) (CharacterLease, bool)
}

// CharacterLease 是一次“已获准进入 → 最终存档提交”的完整所有权。
// Save 只把最终快照排给 WriteBack，调用它的场景 goroutine 不等待数据库 IO。
// Release 仅供进场加载/投递失败、角色尚未交给场景时撤销租约。
type CharacterLease interface {
	Save(domain.Snapshot)
	Release()
}

type characterGate struct {
	writer *WriteBack

	mu       sync.Mutex
	next     uint64
	held     map[int64]uint64
	accounts map[int64]uint64
}

// NewCharacterGate 为一个 WriteBack 建全服唯一的角色闸门。
// 调用方必须把同一个返回值注入所有 Session；每连接各建一份就失去隔离意义。
func NewCharacterGate(writer *WriteBack) CharacterGate {
	return &characterGate{writer: writer, held: make(map[int64]uint64), accounts: make(map[int64]uint64)}
}

func (g *characterGate) TryEnter(charID, accountID int64) (CharacterLease, bool) {
	if charID <= 0 || accountID <= 0 || g.writer == nil {
		return nil, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, busy := g.held[charID]; busy {
		return nil, false
	}
	if _, busy := g.accounts[accountID]; busy {
		return nil, false
	}
	g.next++
	token := g.next
	g.held[charID] = token
	g.accounts[accountID] = token
	return &characterLease{gate: g, charID: charID, accountID: accountID, token: token}, true
}

type characterLease struct {
	gate      *characterGate
	charID    int64
	accountID int64
	token     uint64
	once      sync.Once
}

// Save 把最终快照排队，并在 WriteBack 确认事务成功后释放角色闸门。
func (l *characterLease) Save(snap domain.Snapshot) {
	if snap.Char == nil || (snap.Char.ID != l.charID || snap.Char.AccountID != l.accountID) {
		l.gate.writer.log.Error("离场快照与角色租约不匹配, 保持闸门关闭",
			"gateChar", l.charID, "snapshotChar", snapshotCharID(snap))
		return
	}
	l.once.Do(func() {
		l.releaseAfter(l.gate.writer.SaveAck(snap))
	})
}

// Release 撤销尚未交给场景的进入租约。角色一旦进入场景，必须走 Save，
// 不能靠 Release 绕过持久化屏障。
func (l *characterLease) Release() {
	l.once.Do(func() { l.gate.release(l.charID, l.accountID, l.token) })
}

func (l *characterLease) releaseAfter(committed <-chan struct{}) {
	// 已经提交完成（CommitCurrent 常见）的路径同步释放，避免一次无意义的调度窗口。
	select {
	case <-committed:
		l.gate.release(l.charID, l.accountID, l.token)
		return
	default:
	}
	// 独立 goroutine 只等一次关闭通知，不持锁也不做 IO；数据库失败时它会继续等待，
	// 与 WriteBack 中保留下来的 waiter 一起跨重试存活。
	go func() {
		<-committed
		l.gate.release(l.charID, l.accountID, l.token)
	}()
}

func (g *characterGate) release(charID, accountID int64, token uint64) {
	g.mu.Lock()
	if g.held[charID] == token {
		delete(g.held, charID)
	}
	if g.accounts[accountID] == token {
		delete(g.accounts, accountID)
	}
	g.mu.Unlock()
}

func snapshotCharID(snap domain.Snapshot) int64 {
	if snap.Char == nil {
		return 0
	}
	return snap.Char.ID
}
