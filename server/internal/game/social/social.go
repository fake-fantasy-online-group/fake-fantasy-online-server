// Package social 管好友请求这类**待处理的社交意向**。
//
// 只放"还没落定"的东西：好友请求发出去了、对方还没点同意。
// 一旦点了同意，关系就写进数据库（`char_friends`），本包不再管它。
//
// **好友关系本身不进本包，也不进场景。**
//
//	队伍  场景要用（药师选目标、击杀分经验）→ 队伍号抄一份到实体上
//	好友  场景**一点都不需要** → 只在会话层活动，实体上没有任何好友字段
//
// 这个区别值得记一下：往实体上加字段是有代价的（每帧遍历、存档、传送带走），
// 只有真正进入帧循环的东西才配得上那个位置。
package social

import (
	"sort"
	"sync"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// 客户端 SystemInfoBar.Req.kind。编号来自 1.3.4 客户端已有自动化实测与
// SystemInfoBar.Respond 类型：0=交易、1=组队、2=好友、3=师徒、4=本地私聊通知。
// 家族邀请/申请使用后续类型 5/6，并在请求自身保存家族号与方向；私聊通知仍由
// 客户端自己维护，不进请求表。
const (
	RequestTrade uint8 = iota
	RequestParty
	RequestFriend
	RequestApprentice
	RequestLocalGossip
	RequestFamilyInvite
	RequestFamilyApply
	RequestRide
)

// Request 是一个待处理的系统交互请求，对应客户端 SystemInfoBar.Req。
type Request struct {
	ID       int32
	Kind     uint8
	From     domain.CharID
	FromID   domain.EntityID
	FromName string
	Text     string
	// PartyApplicant 区分“队长邀请我”与“玩家申请加入我的队伍”。客户端两者
	// 都显示为同一个 RequestParty kind，但接受时名册动作方向相反。
	PartyApplicant bool
	// FamilyID 是邀请或申请针对的家族；FamilyApplicant=true 表示发件人是
	// 申请加入的人，接受者是族长。false 表示接受者是被邀请人。
	FamilyID                  int64
	FamilyApplicant           bool
	Master                    domain.CharID
	Apprentice                domain.CharID
	RideDriver, RidePassenger domain.EntityID
	RideEpoch                 uint64
	RideScene                 domain.SceneID
	ExpiresAt                 time.Time
}

// Requests 是全服待处理的好友请求。
//
// 一把锁：请求是人类速度的操作，而且战斗帧循环从不碰它。
type Requests struct {
	mu   sync.Mutex
	next int32
	// pending 收件人 → 他收到的请求（请求号 → 请求）。
	//
	// 按收件人索引而不是发件人：所有操作都是"谁收到了什么"——
	// 弹窗、同意、拒绝、登出时清空，全部从收件人这边进。
	pending map[domain.CharID]map[int32]Request
}

// NewRequests 建一个请求表。
func NewRequests() *Requests {
	return &Requests{pending: map[domain.CharID]map[int32]Request{}}
}

// MaxPendingPerTarget 一个人最多同时挂多少个待处理请求。
//
// **服务端定。** 没有上限的话，一个脚本可以给同一个人发几万个请求，
// 那既是内存问题也是骚扰问题。20 个足够正常使用，
// 超了就顶掉最旧的 —— 拒绝新的会让"刚认识的人加不上"，那更糟。
const MaxPendingPerTarget = 20

// Add 记一个请求。同一个人重复发只留最后一次。
//
// 返回 false 表示这是**重复请求**（已经挂着了），调用方可以据此不再弹窗 ——
// 连点十次加好友不该让对方弹十次。
func (r *Requests) Add(to domain.CharID, req Request) bool {
	req.Kind = RequestFriend
	_, added := r.AddInteraction(to, req)
	return added
}

// AddInteraction 记一条客户端可确认的 SystemInfoBar 请求。
// 同一发件人、同一种请求重复发送时沿用原 requestId，避免对方弹出重复项。
func (r *Requests) AddInteraction(to domain.CharID, req Request) (Request, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	box := r.pending[to]
	if box == nil {
		box = map[int32]Request{}
		r.pending[to] = box
	}
	for id, old := range box {
		// 同乘仅最后一次邀请有效，连同旧请求号一起撤销，不能借旧确认接受新邀请。
		if req.Kind == RequestRide && old.Kind == RequestRide {
			delete(box, id)
			continue
		}
		if old.From == req.From && old.Kind == req.Kind {
			req.ID = id
			box[id] = req
			return req, false
		}
	}
	r.next++
	if r.next <= 0 {
		r.next = 1
	}
	req.ID = r.next
	box[req.ID] = req
	if len(box) > MaxPendingPerTarget {
		r.evictOldestLocked(box)
	}
	return req, true
}

// evictOldestLocked 顶掉一个。
//
// 没有时间戳，所以"最旧"取不到 —— 随便顶一个不是自己刚加的即可。
// 这条路径只有在被刷请求时才走到，那时精确性没有意义，挡住内存增长才有。
func (r *Requests) evictOldestLocked(box map[int32]Request) {
	for k := range box {
		delete(box, k)
		return
	}
}

// Take 取出并移除一个请求。
func (r *Requests) Take(to, from domain.CharID) (Request, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	box := r.pending[to]
	if box == nil {
		return Request{}, false
	}
	for id, req := range box {
		if req.From != from || req.Kind != RequestFriend {
			continue
		}
		delete(box, id)
		if len(box) == 0 {
			delete(r.pending, to)
		}
		return req, true
	}
	return Request{}, false
}

// TakeID 按客户端回传的 requestId 取出并移除一条请求。
func (r *Requests) TakeID(to domain.CharID, id int32) (Request, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	box := r.pending[to]
	if box == nil {
		return Request{}, false
	}
	req, ok := box[id]
	if !ok {
		return Request{}, false
	}
	delete(box, id)
	if len(box) == 0 {
		delete(r.pending, to)
	}
	return req, true
}

// List 列出某人收到的全部请求。
func (r *Requests) List(to domain.CharID) []Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	box := r.pending[to]
	out := make([]Request, 0, len(box))
	for _, req := range box {
		out = append(out, req)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Clear 清空某人的收件箱。登出时调 ——
// 请求是**在线才有意义**的：对方下线了再同意，通知谁去？
func (r *Requests) Clear(to domain.CharID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pending, to)
}

// DropFrom 把某个发件人挂在所有人那里的请求都撤掉。
//
// 发件人登出时调：他自己都不在了，别人点同意也通知不到他，
// 留着只会让对方点了之后什么都没发生。
func (r *Requests) DropFrom(from domain.CharID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for to, box := range r.pending {
		for id, req := range box {
			if req.From == from {
				delete(box, id)
			}
		}
		if len(box) == 0 {
			delete(r.pending, to)
		}
	}
}

// DropApprenticeship removes stale requests for this exact pair from either
// direction. A later confirmation must not recreate a relation just removed.
func (r *Requests) DropApprenticeship(master, apprentice domain.CharID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for to, box := range r.pending {
		for id, req := range box {
			if req.Kind == RequestApprentice && req.Master == master && req.Apprentice == apprentice {
				delete(box, id)
			}
		}
		if len(box) == 0 {
			delete(r.pending, to)
		}
	}
}

// Rename 更新仍在等待确认的请求展示名，避免改名后弹出旧名字。
func (r *Requests) Rename(from domain.CharID, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, box := range r.pending {
		for id, req := range box {
			if req.From == from {
				req.FromName = name
				box[id] = req
			}
		}
	}
}

// Count 返回挂着请求的人数。给监控与测试用。
func (r *Requests) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.pending)
}
