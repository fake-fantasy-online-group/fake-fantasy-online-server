// Package online 记着"每个在线角色现在在哪张图、是哪个实体"。
//
// 缺了它，任何"对另一个人做点什么"的功能都做不了 ——
// 会话手上只有自己的 `(场景, 实体 id)`，别人的一概不知道。
// 具体卡住的有：
//
//	组队    A 退队时 B 的场景侧 PartyID 刷新不了（上一轮留下的缺口）
//	私聊    不知道往哪个场景投
//	好友    上线/下线提示、"他在哪"
//	GM      跨场景踢人、传唤
//
// **本包只回答"在哪"，不负责投递。** 投递要 Router，而 Router 在 game/scene ——
// 让本包依赖它，就等于把一张全服索引焊死在场景包上。
// 会话层同时认识两边，由它拿着地址去投。
package online

import (
	"strings"
	"sync"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Location 是一个在线角色的位置。
//
// 存**实体 id 与场景 id**，不存指针 —— 与场景内的引用纪律一致：
// 拿到指针就等于绕过场景 goroutine 直接读写实体。
type Location struct {
	Char    domain.CharID
	Account int64
	Name    string
	Entity  domain.EntityID
	Scene   domain.SceneID
	Profile Profile
}

// Profile 是队伍面板需要的在线公开快照。它是值拷贝，不泄漏场景实体指针。
type Profile struct {
	Level, HP, MaxHP  int32
	Gender            uint8
	HairID, HairColor int32
}

// Control 是在线会话的极小管理面。Registry 仍不认识协议、连接或 Session；
// 它只保存由会话层提供的通知与断线能力，供 GM 公告和踢人使用。
type Control interface {
	Notify(text string)
	Disconnect()
}

// ModerationControl/ChatControl 是可选能力；Registry 的基础 Control 保持最小，
// 组队等只需要通知/断线的调用方不必被迫依赖聊天与 GM 状态。
type ModerationControl interface {
	Control
	SetMutedUntil(until time.Time)
	MutedUntil() time.Time
}

type ChatControl interface {
	Control
	DeliverChat(message ChatMessage)
}

// SocialControl 是客户端原生社交入口需要的可选会话能力。在线索引只负责找到
// 正确连接，不认识 0x8035/0x8036/0x8037 的字节格式。
type SocialControl interface {
	Control
	RefreshSystemRequests()
	RefreshFriends()
	DeliverGossip(from, text string)
	RequestNewbieTip(id int32)
}

// BlockControl 是接收会话的屏蔽门禁。发件人在投递聊天或社交请求前
// 只问接收方的会话缓存，不让全服广播路径逐人查 PostgreSQL。
type BlockControl interface {
	Control
	BlocksFrom(from domain.CharID) bool
	RefreshBlocks()
}

type MailControl interface {
	Control
	RefreshMailIndicator()
}

type FamilyControl interface {
	Control
	RefreshFamily()
	RefreshFamilyPositions()
}

type ApprenticeControl interface {
	Control
	RefreshApprentice()
}

// TradeControl is the protocol-neutral delivery surface for the native 1.3.4
// trade window. The online index only locates the current leased session; packet
// encoding remains in the session/protocol boundary.
type TradeControl interface {
	Control
	OpenTrade(other domain.EntityID, otherName string)
	UpdateTrade(TradeState)
	EndTrade(color uint8, text string)
}

type TradeItem struct {
	Pet     *domain.PetItemInfo
	ID      domain.ItemID
	Count   int32
	Name    string
	Info    string
	Quality uint8
}

type TradeState struct {
	MyLocked, OtherLocked       bool
	MyConfirmed, OtherConfirmed bool
	MyMoney, OtherMoney         int64
	MyItems, OtherItems         []TradeItem
}

// ChatMessage 是在线控制面转交的聊天值快照。在线索引不认识协议编码，接收会话
// 再把它翻成正式客户端的 0x8039 聊天栏消息和 0x8019 头顶气泡。
type ChatMessage struct {
	HornTier, HornSkin uint8
	Sender             domain.EntityID
	Name               string
	Theme              uint8
	Channel            uint8
	Text               string
	Shares             []domain.ChatShare
}

type controlBinding struct {
	entity  domain.EntityID
	control Control
}

// Recipient 是公告投递使用的在线地址与会话控制快照。
type Recipient struct {
	Location Location
	Control  Control
}

// Registry 是全服在线索引。
//
// 一把读写锁：写是登录/换图/登出（人类速度），读是私聊、组队刷新这类操作。
// **战斗帧循环不碰它** —— 场景里判队友是比实体上抄来的 PartyID，不查这张表。
type Registry struct {
	mu     sync.RWMutex
	byChar map[domain.CharID]Location
	// byName 名字 → 角色 id。私聊/邀请都是按名字找人的。
	// 存**折叠后的名字**（见 foldName）——玩家输入的大小写千奇百怪。
	byName  map[string]domain.CharID
	control map[domain.CharID]controlBinding
}

// NewRegistry 建一个索引。
func NewRegistry() *Registry {
	return &Registry{
		byChar:  map[domain.CharID]Location{},
		byName:  map[string]domain.CharID{},
		control: map[domain.CharID]controlBinding{},
	}
}

// foldName 把名字折成查找键。
//
// 只做大小写折叠，**不去空格**：角色名里的空格是名字的一部分，
// 去掉的话"张 三"和"张三"会变成同一个人，那是两个账号。
func foldName(s string) string { return strings.ToLower(s) }

// Enter 记下（或更新）一个角色的位置。
//
// 登录、每次传送落地都调它 —— 传送之后位置变了，不更新的话
// 私聊会投给一个已经没有这个实体的场景，消息静静消失。
//
// **同一个角色重复登录**（旧连接还没断，新连接又进来了）时，
// 新的直接覆盖旧的。返回被顶掉的旧位置，调用方可以据此把旧连接踢下线。
func (r *Registry) Enter(loc Location) (replaced Location, had bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	old, had := r.byChar[loc.Char]
	if had && loc.Profile.Level == 0 {
		loc.Profile = old.Profile
	}
	if had && old.Name != loc.Name {
		// 改过名: 旧键要摘掉, 否则按旧名字还能找到他
		delete(r.byName, foldName(old.Name))
	}
	r.byChar[loc.Char] = loc
	r.byName[foldName(loc.Name)] = loc.Char
	return old, had
}

// UpdateProfile 仅在实体租约仍匹配时刷新公开状态，防止旧连接覆盖重登后的新实体。
func (r *Registry) UpdateProfile(char domain.CharID, entity domain.EntityID, p Profile) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	loc, ok := r.byChar[char]
	if !ok || loc.Entity != entity {
		return false
	}
	loc.Profile = p
	r.byChar[char] = loc
	return true
}

// FindByEntity 解析客户端上报的在线实体号。组队踢人/转队长不能把临时实体号
// 当成持久化角色号使用。
func (r *Registry) FindByEntity(entity domain.EntityID) (Location, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, loc := range r.byChar {
		if loc.Entity == entity {
			return loc, true
		}
	}
	return Location{}, false
}

// Leave 把一个角色从索引里摘掉。
//
// entity 是**调用方以为的那个实体 id**。只有对得上才摘 ——
// 这是防"重登把新会话摘掉"的关键：
// 玩家掉线重连时，新会话先 Enter（写入新实体 id），旧会话的 OnClose 才姗姗来迟；
// 不比对的话旧会话会把新会话的条目删掉，之后这个人对全服"不在线"，
// 私聊收不到、组队刷不了，而他自己在游戏里玩得好好的。
//
// 返回是否真的摘掉了。
func (r *Registry) Leave(char domain.CharID, entity domain.EntityID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	loc, ok := r.byChar[char]
	if !ok || loc.Entity != entity {
		return false // 已经被新会话顶替了, 不是我的条目
	}
	delete(r.byChar, char)
	delete(r.control, char)
	// 按名字反查的那条也要摘, 而且要确认指向的还是自己 ——
	// 重名不可能(角色名唯一), 但改名之后可能残留
	if id, ok := r.byName[foldName(loc.Name)]; ok && id == char {
		delete(r.byName, foldName(loc.Name))
	}
	return true
}

// AttachControl 把会话控制能力绑定到当前实体租约。实体已经被新连接替换时拒绝
// 绑定，防止旧连接重新覆盖新连接的踢人/公告目标。
func (r *Registry) AttachControl(char domain.CharID, entity domain.EntityID, control Control) bool {
	if control == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	loc, ok := r.byChar[char]
	if !ok || loc.Entity != entity {
		return false
	}
	r.control[char] = controlBinding{entity: entity, control: control}
	return true
}

// ControlByName 返回与当前在线实体匹配的管理面。回调必须在锁外调用。
func (r *Registry) ControlByName(name string) (Location, Control, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byName[foldName(name)]
	if !ok {
		return Location{}, nil, false
	}
	loc, ok := r.byChar[id]
	if !ok {
		return Location{}, nil, false
	}
	binding, ok := r.control[id]
	if !ok || binding.entity != loc.Entity || binding.control == nil {
		return Location{}, nil, false
	}
	return loc, binding.control, true
}

// Recipients 取公告接收者快照。调用方拿到结果后逐个投递，不在 Registry 锁内
// 执行连接操作。
func (r *Registry) Recipients() []Recipient {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Recipient, 0, len(r.byChar))
	for id, loc := range r.byChar {
		binding, ok := r.control[id]
		if !ok || binding.entity != loc.Entity || binding.control == nil {
			continue
		}
		out = append(out, Recipient{Location: loc, Control: binding.control})
	}
	return out
}

// Find 按角色 id 查位置。
func (r *Registry) Find(char domain.CharID) (Location, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	loc, ok := r.byChar[char]
	return loc, ok
}

// FindByName 按角色名查位置。大小写不敏感。
func (r *Registry) FindByName(name string) (Location, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byName[foldName(name)]
	if !ok {
		return Location{}, false
	}
	loc, ok := r.byChar[id]
	return loc, ok
}

// Online 报告某个角色在不在线。
func (r *Registry) Online(char domain.CharID) bool {
	_, ok := r.Find(char)
	return ok
}

// Count 返回在线人数。
//
// `0x107d`（选服界面每 5 秒问一次服务器状态）要的就是这个数，
// 客户端读取该在线人数用于显示服务器状态。
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byChar)
}

// Locate 批量查一组角色的位置，查不到的直接跳过。
//
// 给"通知队伍里每个人"这类场景用：一次锁拿完，
// 逐个调 Find 的话，中间有人下线就会拿到半新半旧的一组位置。
func (r *Registry) Locate(chars []domain.CharID) []Location {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Location, 0, len(chars))
	for _, c := range chars {
		if loc, ok := r.byChar[c]; ok {
			out = append(out, loc)
		}
	}
	return out
}

// Snapshot 取一份全量快照。给 GM 工具与监控用，不要放进热路径。
func (r *Registry) Snapshot() []Location {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Location, 0, len(r.byChar))
	for _, loc := range r.byChar {
		out = append(out, loc)
	}
	return out
}

// PrivateShareControl delivers server-resolved immutable item shares.
type PrivateShareControl interface {
	DeliverGossipShares(from, text string, shares []domain.ChatShare)
}
type AnnouncementControl interface{ Announce(text string) }
