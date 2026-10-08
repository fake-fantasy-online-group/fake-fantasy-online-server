package session

import (
	"context"
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/online"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/social"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

// 好友在会话层的那一半。
//
// **好友关系不进场景，也不进实体。**
//
//	队伍  场景要用（药师选目标、击杀分经验）→ 队伍号抄一份到实体上
//	好友  场景一点都不需要 → 只在这里活动
//
// 往实体上加字段是有代价的（每帧遍历、存档、传送带走），
// 只有真正进入帧循环的东西才配得上那个位置。
//
// 上下线通知靠 `game/online` 索引 —— 这是那张索引的第二个兑现（第一个是跨场景刷队伍号）。
// 关系**双向存两行**，所以"通知我的好友们"就是遍历我自己的列表，不需要反查。

// friendsOf 读当前角色的好友列表。
func (s *Session) friendsOf(ctx context.Context, charID int64) []domain.Friend {
	if s.deps.Store == nil {
		return nil
	}
	list, err := s.deps.Store.LoadFriends(ctx, charID)
	if err != nil {
		s.log.Error("读好友失败", "char", charID, "err", err)
		return nil
	}
	return list
}

// onlineFriends 返回我的好友里**现在在线**的那些。
//
// 拆成单独一个函数是为了它能被测 —— 通知本身要等下行包格式(见 sendToFriend),
// 但"该通知谁"这件事现在就该是对的, 而且它才是容易错的那一半。
func (s *Session) onlineFriends(ctx context.Context, me *domain.Character) []online.Location {
	if s.deps.Online == nil || me == nil {
		return nil
	}
	var out []online.Location
	for _, f := range s.friendsOf(ctx, me.ID) {
		loc, ok := s.deps.Online.Find(f.Char)
		if !ok {
			continue // 不在线, 跳过
		}
		out = append(out, loc)
	}
	return out
}

// notifyFriends 把"我上线了/我下线了"告诉在线的好友。
//
// 只通知**在线**的：离线的人下次登录时会自己拉一遍列表，
// 攒一堆"他 3 小时前上线过"的消息给他没有意义。
func (s *Session) notifyFriends(ctx context.Context, me *domain.Character, isOnline bool) {
	for _, loc := range s.onlineFriends(ctx, me) {
		s.sendToFriend(loc, me, isOnline)
	}
}

// sendToFriend 给一个在线好友发上下线通知。
//
// ⚠️ **客户端没有"某个好友上下线"这种单条通知包。** 已定义的是 `0x8037` 好友列表
// 快照:`U8 friendCount` + friendCount × (`Str name`, `U8 online`)。
// 所以上下线要表达成"重发一份整表"给对方,而不是发一条增量 ——
// 那需要在这里能取到接收方的完整好友列表,目前还没接。
// 当前只记录目标与上线状态，尚未向对方推送好友列表。
func (s *Session) sendToFriend(loc online.Location, me *domain.Character, isOnline bool) {
	state := "下线"
	if isOnline {
		state = "上线"
	}
	s.log.Debug("好友状态通知", "通知谁", loc.Name, "谁的状态", me.Name, "状态", state)
	s.refreshFriendsOf(loc.Char)
}

// friendPacket 从持久化关系构造客户端 0x8037 快照。好友按角色名排序，让同一份
// 数据生成稳定字节；online 只来自全服在线索引，不相信客户端自报。
func (s *Session) friendPacket(ctx context.Context, charID int64) []byte {
	list := s.friendsOf(ctx, charID)
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	view := make([]protocol.FriendView, 0, len(list))
	for _, friend := range list {
		view = append(view, protocol.FriendView{
			Name:   friend.Name,
			Online: s.deps.Online != nil && s.deps.Online.Online(friend.Char),
		})
	}
	return protocol.FriendList(view)
}

func (s *Session) refreshFriendList() {
	s.mu.Lock()
	ch, inGame := s.char, s.stage == StageInGame
	s.mu.Unlock()
	if !inGame || ch == nil {
		return
	}
	s.sink.Send(s.friendPacket(context.Background(), ch.ID))
}

func (s *Session) refreshFriendsOf(who domain.CharID) {
	if s.deps.Online == nil {
		return
	}
	loc, control, ok := s.controlFor(who)
	if !ok {
		return
	}
	if delivery, ok := control.(online.SocialControl); ok {
		delivery.RefreshFriends()
		s.log.Debug("刷新好友列表", "char", loc.Name)
	}
}

// requestFriend 发一个好友请求。
//
// **要求对方在线**：加好友是当面同意的事，给离线的人发请求，
// 他上线时那个请求早就没了（请求不落盘），点了也没反应。
func (s *Session) requestFriend(ctx context.Context, targetName string) domain.FriendReject {
	s.mu.Lock()
	me := s.char
	s.mu.Unlock()
	if me == nil || s.deps.Online == nil || s.deps.Requests == nil {
		return domain.FriendNotFound
	}

	loc, ok := s.deps.Online.FindByName(targetName)
	if !ok {
		return domain.FriendOffline
	}
	if loc.Char == domain.CharID(me.ID) {
		return domain.FriendSelf
	}
	if s.targetBlocks(loc.Char, domain.CharID(me.ID)) {
		return domain.FriendNotFound
	}

	mine, _ := s.deps.Store.CountFriends(ctx, me.ID)
	theirs, _ := s.deps.Store.CountFriends(ctx, int64(loc.Char))
	already := s.alreadyFriends(ctx, me.ID, loc.Char)
	if r := domain.CanAddFriend(domain.CharID(me.ID), loc.Char, mine, theirs, already); r != domain.FriendOK {
		return r
	}

	_, first := s.deps.Requests.AddInteraction(loc.Char, social.Request{
		Kind: social.RequestFriend, From: domain.CharID(me.ID), FromID: s.entityID(),
		FromName: me.Name, Text: me.Name + "请求添加你为好友",
	})
	if first {
		s.log.Debug("好友请求", "从", me.Name, "到", loc.Name)
		s.refreshSystemRequestsOf(loc.Char)
	}
	// 重复发不再打扰对方, 但对发起人仍然算成功 ——
	// 报个错只会让他以为没发出去, 然后接着点
	return domain.FriendOK
}

// acceptFriend 同意一个好友请求。
func (s *Session) acceptFriend(ctx context.Context, from domain.CharID) domain.FriendReject {
	s.mu.Lock()
	me := s.char
	s.mu.Unlock()
	if me == nil || s.deps.Requests == nil {
		return domain.FriendNotFound
	}
	if _, ok := s.deps.Requests.Take(domain.CharID(me.ID), from); !ok {
		return domain.FriendNoRequest
	}
	return s.acceptFriendReady(ctx, me, from)
}

// acceptFriendReady 完成已经由请求表验证过的好友确认。
func (s *Session) acceptFriendReady(ctx context.Context, me *domain.Character, from domain.CharID) domain.FriendReject {

	// **再查一次上限。** 从发请求到点同意之间隔着人的反应时间,
	// 那段时间里两边都可能加满了
	mine, _ := s.deps.Store.CountFriends(ctx, me.ID)
	theirs, _ := s.deps.Store.CountFriends(ctx, int64(from))
	already := s.alreadyFriends(ctx, me.ID, from)
	if r := domain.CanAddFriend(domain.CharID(me.ID), from, mine, theirs, already); r != domain.FriendOK {
		return r
	}

	if err := s.deps.Store.AddFriend(ctx, me.ID, int64(from)); err != nil {
		s.log.Error("加好友失败", "err", err)
		return domain.FriendNotFound
	}
	s.refreshFriendsOf(domain.CharID(me.ID))
	s.refreshFriendsOf(from)
	if mine == 0 {
		s.requestTipFor(domain.CharID(me.ID), 10)
	}
	if theirs == 0 {
		s.requestTipFor(from, 10)
	}
	s.log.Info("加好友", "谁", me.Name, "和", from)
	return domain.FriendOK
}

// removeFriend 删好友。**双向删** —— 留着单边的话，
// 对方会一直收到我的上下线通知，而我这边看不到他。
func (s *Session) removeFriend(ctx context.Context, other domain.CharID) domain.FriendReject {
	s.mu.Lock()
	me := s.char
	s.mu.Unlock()
	if me == nil {
		return domain.FriendNotFound
	}
	if !s.alreadyFriends(ctx, me.ID, other) {
		return domain.FriendNotFriend
	}
	if err := s.deps.Store.RemoveFriend(ctx, me.ID, int64(other)); err != nil {
		s.log.Error("删好友失败", "err", err)
		return domain.FriendNotFound
	}
	return domain.FriendOK
}

// alreadyFriends 查两个人是不是已经是好友。
func (s *Session) alreadyFriends(ctx context.Context, me int64, other domain.CharID) bool {
	for _, f := range s.friendsOf(ctx, me) {
		if f.Char == other {
			return true
		}
	}
	return false
}

func (s *Session) onFriendRequest(name string) {
	if reject := s.requestFriend(context.Background(), name); reject != domain.FriendOK {
		s.sendGMText("好友请求失败")
	}
}

func (s *Session) onFriendDelete(name string) {
	other, err := s.deps.Store.CharByName(context.Background(), name)
	if err != nil || other == nil {
		s.sendGMText("好友不存在：" + name)
		return
	}
	if reject := s.removeFriend(context.Background(), domain.CharID(other.ID)); reject != domain.FriendOK {
		s.sendGMText("删除好友失败")
		return
	}
	s.refreshFriendsOf(domain.CharID(other.ID))
	s.refreshFriendList()
}
