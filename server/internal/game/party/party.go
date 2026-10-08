// Package party 管队伍名册。
//
// **为什么不放进场景**：队伍是跨场景的 —— 队长在龙城、队员在野外是常态。
// 而场景 Actor 模型的前提是"一个实体只被它所在场景的 goroutine 读写"，
// 把队伍塞进任何一个场景，另一张图的成员就没法安全地读它。
//
// 所以分成两半：
//
//	名册（谁在队里、谁是队长、有哪些邀请）  本包，一把锁，会话层用
//	"是不是队友" 这个判断                场景里比两个 domain.PartyID 的值
//
// 后者不需要本包 —— 场景**不 import 这个包**（分层表里也没这条边）。
// 队伍变动时由会话层往受影响的场景投一条 SetParty 命令，把新的 PartyID
// 抄到实体身上。跟传送时的所有权交接是同一套纪律。
package party

import (
	"sync"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Member 是队伍里的一个人。
//
// 存**角色 id 而不是实体 id**：实体 id 离场即失效，而队伍要熬过换图 ——
// 换图时实体 id 会变，但你还在这支队里。
type Member struct {
	Char domain.CharID
	Name string
}

// Party 是一支队伍的名册。
//
// 取出去的都是拷贝，外面拿不到内部切片 —— 拿到了就等于绕过锁改名册。
type Party struct {
	ID      domain.PartyID
	Leader  domain.CharID
	Name    string
	Members []Member
}

// Has 报告某人在不在这支队里。
func (p Party) Has(c domain.CharID) bool {
	for _, m := range p.Members {
		if m.Char == c {
			return true
		}
	}
	return false
}

// Registry 是全服的队伍名册。
//
// 一把大锁够用：组队操作是**人类速度**的（邀请、同意、退出），
// 每秒撑死几十次，而战斗里每帧要判的"是不是队友"根本不走这里。
type Registry struct {
	mu      sync.Mutex
	next    uint32
	parties map[domain.PartyID]*Party
	// byChar 角色 → 所在队伍。查"我在哪队"是最热的操作，给它一张反查表。
	byChar map[domain.CharID]domain.PartyID
	// invites 被邀请人 → 邀请人及其当时的队伍。一个人同时只记最后一个邀请 ——
	// 攒一串邀请再逐个弹窗只会变成骚扰。
	invites map[domain.CharID]partyInvite
}

type partyInvite struct {
	From  domain.CharID
	Party domain.PartyID
}

// NewRegistry 建一个名册。
func NewRegistry() *Registry {
	return &Registry{
		parties: map[domain.PartyID]*Party{},
		byChar:  map[domain.CharID]domain.PartyID{},
		invites: map[domain.CharID]partyInvite{},
	}
}

// PartyOf 返回某人所在队伍的号。0 表示没组队。
func (r *Registry) PartyOf(c domain.CharID) domain.PartyID {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byChar[c]
}

// Get 取一支队伍的快照。第二个返回值 false 表示队伍不存在。
func (r *Registry) Get(id domain.PartyID) (Party, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.parties[id]
	if !ok {
		return Party{}, false
	}
	return snapshot(p), true
}

// snapshot 深拷一份名册。调用方必须已持锁。
func snapshot(p *Party) Party {
	return Party{ID: p.ID, Leader: p.Leader, Name: p.Name,
		Members: append([]Member(nil), p.Members...)}
}

// Create 显式创建一支可命名的单人队伍。与 Invite 的“接受时再隐式建队”并存：
// 客户端 PartyCreate 本来就要求创建后立即展示队伍面板。
func (r *Registry) Create(leader domain.CharID, leaderName, partyName string) (Party, domain.PartyReject) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, in := r.byChar[leader]; in {
		return Party{}, domain.PartyAlreadyIn
	}
	p := r.newParty(leader, leaderName)
	p.Name = partyName
	return snapshot(p), domain.PartyOK
}

// Invite 队长（或还没组队的人）邀请另一个人。
//
// 邀请人还没队伍时**不立刻建队** —— 建了对方又不同意的话，
// 邀请人就莫名其妙变成一个单人队的队长了。等对方同意时再建。
func (r *Registry) Invite(from, to domain.CharID) domain.PartyReject {
	r.mu.Lock()
	defer r.mu.Unlock()

	if from == to {
		return domain.PartySelfTarget
	}
	if _, in := r.byChar[to]; in {
		return domain.PartyAlreadyIn
	}
	if pid, in := r.byChar[from]; in {
		p := r.parties[pid]
		if p.Leader != from {
			return domain.PartyNotLeader
		}
		if len(p.Members) >= domain.MaxPartySize {
			return domain.PartyFull
		}
	}
	r.invites[to] = partyInvite{From: from, Party: r.byChar[from]} // Party=0 表示邀请人还没队伍
	return domain.PartyOK
}

// Accept 接受邀请。
//
// 返回队伍快照与**需要通知的角色**：加入之后所有成员的队伍面板都得刷新，
// 而只有名册知道现在有谁 —— 让调用方自己去查会漏掉刚退队的人。
func (r *Registry) Accept(who domain.CharID, whoName string,
	inviter domain.CharID, inviterName string) (Party, domain.PartyReject) {
	r.mu.Lock()
	defer r.mu.Unlock()

	invite, invited := r.invites[who]
	if !invited || invite.From != inviter {
		return Party{}, domain.PartyNotIn
	}
	delete(r.invites, who)
	pid := invite.Party

	if _, in := r.byChar[who]; in {
		return Party{}, domain.PartyAlreadyIn
	}

	var p *Party
	if pid == 0 {
		// 邀请人当时还没队伍: 现在建, 他当队长
		if _, in := r.byChar[inviter]; in {
			// 等这段时间他自己进了别的队 —— 邀请作废
			return Party{}, domain.PartyAlreadyIn
		}
		p = r.newParty(inviter, inviterName)
	} else {
		var ok bool
		p, ok = r.parties[pid]
		if !ok {
			return Party{}, domain.PartyNotIn // 队伍已经散了
		}
	}
	if len(p.Members) >= domain.MaxPartySize {
		return Party{}, domain.PartyFull
	}
	p.Members = append(p.Members, Member{Char: who, Name: whoName})
	r.byChar[who] = p.ID
	return snapshot(p), domain.PartyOK
}

// CancelInvite 撤销某人当前挂着的邀请。客户端拒绝 0x8035 请求或请求失效时调用。
func (r *Registry) CancelInvite(who domain.CharID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.invites, who)
}

// newParty 建一支队伍。调用方必须已持锁。
func (r *Registry) newParty(leader domain.CharID, name string) *Party {
	r.next++
	p := &Party{ID: domain.PartyID(r.next), Leader: leader,
		Members: []Member{{Char: leader, Name: name}}}
	r.parties[p.ID] = p
	r.byChar[leader] = p.ID
	return p
}

// Leave 退队。
//
// 返回退队后的队伍快照（可能已经解散，那时 ok 为 false）。
//
// **队长走了就把队长给下一个人，不解散。** 解散的话，队长一掉线整队人
// 就要重新组一遍 —— 而掉线是最常见的退队方式。
// 只剩一个人时才真正解散：一个人的队伍没有任何意义，还会让他以为自己在组队。
func (r *Registry) Leave(who domain.CharID) (Party, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.removeLocked(who)
}

func (r *Registry) removeLocked(who domain.CharID) (Party, bool) {
	pid, in := r.byChar[who]
	if !in {
		return Party{}, false
	}
	delete(r.byChar, who)
	p, ok := r.parties[pid]
	if !ok {
		return Party{}, false
	}
	for i, m := range p.Members {
		if m.Char == who {
			p.Members = append(p.Members[:i], p.Members[i+1:]...)
			break
		}
	}
	if len(p.Members) <= 1 {
		// 散伙: 把剩下那个也摘干净, 否则他会挂在一个不存在的队伍号上
		for _, m := range p.Members {
			delete(r.byChar, m.Char)
		}
		delete(r.parties, pid)
		return snapshot(p), false
	}
	if p.Leader == who {
		p.Leader = p.Members[0].Char // 顺位给下一个, 不解散
	}
	return snapshot(p), true
}

// Kick 队长踢人。
func (r *Registry) Kick(leader, target domain.CharID) (Party, domain.PartyReject) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if leader == target {
		return Party{}, domain.PartySelfTarget
	}
	pid, in := r.byChar[leader]
	if !in {
		return Party{}, domain.PartyNotIn
	}
	p := r.parties[pid]
	if p.Leader != leader {
		return Party{}, domain.PartyNotLeader
	}
	if !p.Has(target) {
		return Party{}, domain.PartyNoSuchMember
	}
	after, _ := r.removeLocked(target)
	return after, domain.PartyOK
}

// SetLeader 转让队长。
func (r *Registry) SetLeader(from, to domain.CharID) domain.PartyReject {
	r.mu.Lock()
	defer r.mu.Unlock()

	if from == to {
		return domain.PartySelfTarget
	}
	pid, in := r.byChar[from]
	if !in {
		return domain.PartyNotIn
	}
	p := r.parties[pid]
	if p.Leader != from {
		return domain.PartyNotLeader
	}
	if !p.Has(to) {
		return domain.PartyNoSuchMember
	}
	p.Leader = to
	return domain.PartyOK
}

// Rename 更新队伍名册中的展示名。角色号才是身份，改名不能改变成员关系。
func (r *Registry) Rename(who domain.CharID, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pid, ok := r.byChar[who]
	if !ok {
		return
	}
	p := r.parties[pid]
	if p == nil {
		return
	}
	for i := range p.Members {
		if p.Members[i].Char == who {
			p.Members[i].Name = name
			return
		}
	}
}

// Disband 队长解散队伍。返回被解散时还在队里的人（他们的 PartyID 都要清掉）。
func (r *Registry) Disband(leader domain.CharID) ([]Member, domain.PartyReject) {
	r.mu.Lock()
	defer r.mu.Unlock()

	pid, in := r.byChar[leader]
	if !in {
		return nil, domain.PartyNotIn
	}
	p := r.parties[pid]
	if p.Leader != leader {
		return nil, domain.PartyNotLeader
	}
	members := append([]Member(nil), p.Members...)
	for _, m := range p.Members {
		delete(r.byChar, m.Char)
	}
	delete(r.parties, pid)
	return members, domain.PartyOK
}

// Count 返回现存队伍数。给监控与测试用。
func (r *Registry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.parties)
}
