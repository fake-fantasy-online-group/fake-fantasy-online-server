package session

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/online"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/social"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

// controlFor 返回角色当前实体租约对应的会话控制面。先按角色号定位，再按名字取
// control，可复用 Registry 已有的“绑定实体必须仍匹配”校验。
func (s *Session) controlFor(who domain.CharID) (online.Location, online.Control, bool) {
	if s.deps.Online == nil {
		return online.Location{}, nil, false
	}
	loc, ok := s.deps.Online.Find(who)
	if !ok {
		return online.Location{}, nil, false
	}
	return s.deps.Online.ControlByName(loc.Name)
}

func (s *Session) systemRequestsPacket() []byte {
	s.mu.Lock()
	ch := s.char
	s.mu.Unlock()
	if ch == nil || s.deps.Requests == nil {
		return protocol.SystemRequests(nil)
	}
	requests := s.deps.Requests.List(domain.CharID(ch.ID))
	view := make([]protocol.SystemRequestView, 0, len(requests))
	for _, req := range requests {
		if s.blocksFrom(req.From) {
			continue
		}
		view = append(view, protocol.SystemRequestView{
			ID: req.ID, Kind: req.Kind, From: req.FromID,
			FromName: req.FromName, Text: req.Text,
		})
	}
	return protocol.SystemRequests(view)
}

func (s *Session) refreshSystemRequests() {
	s.mu.Lock()
	inGame := s.stage == StageInGame && s.char != nil
	s.mu.Unlock()
	if inGame {
		s.sink.Send(s.systemRequestsPacket())
	}
}

func (s *Session) refreshSystemRequestsOf(who domain.CharID) {
	_, control, ok := s.controlFor(who)
	if !ok {
		return
	}
	if delivery, ok := control.(online.SocialControl); ok {
		delivery.RefreshSystemRequests()
	}
}

func (s *Session) requestTipFor(who domain.CharID, id int32) {
	_, control, ok := s.controlFor(who)
	if !ok {
		return
	}
	if delivery, ok := control.(online.SocialControl); ok {
		delivery.RequestNewbieTip(id)
	}
}

// onSystemRequest 消费 0x8035 中那条请求对应的 0x104a 回执。requestId 是唯一
// 权威关联键；kind/from 均不由客户端回传，避免伪造接受另一类请求。
func (s *Session) onSystemRequest(requestID int32, accept bool) {
	s.mu.Lock()
	me, inGame := s.char, s.stage == StageInGame
	s.mu.Unlock()
	if !inGame || me == nil || s.deps.Requests == nil {
		return
	}
	req, ok := s.deps.Requests.TakeID(domain.CharID(me.ID), requestID)
	if !ok {
		s.refreshSystemRequests()
		return
	}
	if s.blocksFrom(req.From) {
		s.refreshSystemRequests()
		return
	}
	s.refreshSystemRequests()
	if !accept {
		if req.Kind == social.RequestParty && s.deps.Party != nil {
			invitee := domain.CharID(me.ID)
			if req.PartyApplicant {
				invitee = req.From
			}
			s.deps.Party.CancelInvite(invitee)
		}
		return
	}

	switch req.Kind {
	case social.RequestRide:
		s.acceptRideRequest(req)
	case social.RequestFriend:
		if reject := s.acceptFriendReady(context.Background(), me, req.From); reject != domain.FriendOK {
			s.sendGMText("添加好友失败")
		}
	case social.RequestParty:
		s.acceptPartyRequest(me, req)
	case social.RequestTrade:
		s.acceptTradeRequest(me, req)
	case social.RequestFamilyInvite, social.RequestFamilyApply:
		familyStore, ok := s.familyStore()
		if !ok {
			s.sendGMText("加入家族失败：家族功能不可用")
			return
		}
		joiner := domain.CharID(me.ID)
		if req.FamilyApplicant {
			joiner = req.From
		}
		if err := familyStore.JoinFamily(context.Background(), int64(joiner), req.FamilyID, true); err != nil {
			s.sendGMText("加入家族失败：" + err.Error())
			return
		}
		s.refreshFamilyMembers(me.ID, joiner)
	case social.RequestApprentice:
		apprenticeStore, ok := s.apprenticeStore()
		if !ok {
			s.sendGMText("建立师徒关系失败：师徒功能不可用")
			return
		}
		if err := apprenticeStore.CreateApprenticeship(context.Background(), int64(req.Master), int64(req.Apprentice)); err != nil {
			s.sendGMText("建立师徒关系失败：" + err.Error())
			return
		}
		s.deps.Requests.DropApprenticeship(req.Master, req.Apprentice)
		s.refreshSystemRequestsOf(req.Master)
		s.refreshSystemRequestsOf(req.Apprentice)
		s.refreshApprenticeCircle(req.Master)
	default:
		s.log.Debug("尚未接线的系统交互确认", "kind", req.Kind, "request", requestID)
	}
}

func (s *Session) onGossip(toName, text string, shares int) {
	s.mu.Lock()
	me, entity, inGame := s.char, s.entity, s.stage == StageInGame
	s.mu.Unlock()
	text = strings.TrimSpace(text)
	if !inGame || me == nil || entity == 0 || shares != 0 || text == "" ||
		!utf8.ValidString(text) || utf8.RuneCountInString(text) > maxChatRunes {
		return
	}
	if s.deps.Online == nil {
		return
	}
	_, control, ok := s.deps.Online.ControlByName(toName)
	if !ok {
		s.sendGMText("对方不在线：" + toName)
		return
	}
	delivery, ok := control.(online.SocialControl)
	if !ok {
		return
	}
	if controlBlocks(control, domain.CharID(me.ID)) {
		s.sendGMText("消息未送达")
		return
	}
	delivery.DeliverGossip(me.Name, text)
	// 只有消息已经投递到真实在线客户端后，才成立“第一次密聊”。
	s.postToScene(scene.RequestNewbieTip{ID: entity, Tip: 34})
}

func (s *Session) resolveShares(refs []protocol.ShareRef) ([]domain.ChatShare, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	if len(refs) > 4 || s.deps.Router == nil {
		return nil, fmt.Errorf("分享引用无效")
	}
	s.mu.Lock()
	id, sc := s.entity, s.scene
	s.mu.Unlock()
	rs := make([]scene.ChatShareRef, 0, len(refs))
	for _, r := range refs {
		rs = append(rs, scene.ChatShareRef{Kind: r.Kind, Tab: r.Tab, Slot: int32(clientBagSlotIfBag(r.Kind, r.Tab, int(r.Slot)))})
	}
	ch := make(chan scene.ChatShareResult, 1)
	if !s.deps.Router.Post(sc, scene.ResolveChatShares{ID: id, Refs: rs, Reply: ch}) {
		return nil, fmt.Errorf("分享场景不可用")
	}
	select {
	case r := <-ch:
		if r.Reason != "" {
			return nil, fmt.Errorf("%s", r.Reason)
		}
		return r.Shares, nil
	case <-time.After(2 * time.Second):
		return nil, fmt.Errorf("分享超时")
	}
}
func (s *Session) onGossipRequest(req protocol.Request) {
	if len(req.Shares) == 0 {
		s.onGossip(req.S1, req.S2, 0)
		return
	}
	s.mu.Lock()
	me, id, stage := s.char, s.entity, s.stage
	s.mu.Unlock()
	text := strings.TrimSpace(req.S2)
	if stage != StageInGame || me == nil || text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > maxChatRunes || s.deps.Online == nil {
		return
	}
	_, control, ok := s.deps.Online.ControlByName(req.S1)
	if !ok {
		s.sendGMText("对方不在线：" + req.S1)
		return
	}
	if controlBlocks(control, domain.CharID(me.ID)) {
		s.sendGMText("消息未送达")
		return
	}
	delivery, ok := control.(online.PrivateShareControl)
	if !ok {
		s.sendGMText("对方暂不支持分享消息")
		return
	}
	shares, err := s.resolveShares(req.Shares)
	if err != nil {
		s.sendGMText(err.Error())
		return
	}
	delivery.DeliverGossipShares(me.Name, text, shares)
	s.postToScene(scene.RequestNewbieTip{ID: id, Tip: 34})
}
