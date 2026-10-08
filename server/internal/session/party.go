package session

import (
	"unicode/utf8"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/social"
)

func (s *Session) partyIdentity() (*domain.Character, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.char, s.stage == StageInGame && s.char != nil
}

func (s *Session) onPartyCreate(name string) {
	ch, ok := s.partyIdentity()
	if !ok || s.deps.Party == nil || name == "" || !utf8.ValidString(name) || len([]byte(name)) > 64 {
		return
	}
	p, reject := s.deps.Party.Create(domain.CharID(ch.ID), ch.Name, name)
	if reject != domain.PartyOK {
		s.log.Debug("创建队伍被拒", "char", ch.Name, "原因", reject)
		return
	}
	s.pushPartyID(domain.CharID(ch.ID), p.ID)
	s.postToScene(scene.RequestNewbieTip{ID: s.entityID(), Tip: 17})
}

func (s *Session) onPartyLeave() {
	if ch, ok := s.partyIdentity(); ok {
		s.leaveParty(domain.CharID(ch.ID))
	}
}

func (s *Session) onPartyKick(targetEntity domain.EntityID) {
	ch, ok := s.partyIdentity()
	if !ok || s.deps.Party == nil || s.deps.Online == nil {
		return
	}
	target, found := s.deps.Online.FindByEntity(targetEntity)
	if !found {
		return
	}
	after, reject := s.deps.Party.Kick(domain.CharID(ch.ID), target.Char)
	if reject != domain.PartyOK {
		s.log.Debug("踢出队员被拒", "char", ch.Name, "target", target.Char, "原因", reject)
		return
	}
	s.pushPartyID(target.Char, 0)
	for _, m := range after.Members {
		pid := s.deps.Party.PartyOf(m.Char)
		s.pushPartyID(m.Char, pid)
		s.pushPartyRefresh(m.Char)
	}
}

func (s *Session) onPartyLeader(targetEntity domain.EntityID) {
	ch, ok := s.partyIdentity()
	if !ok || s.deps.Party == nil || s.deps.Online == nil {
		return
	}
	target, found := s.deps.Online.FindByEntity(targetEntity)
	if !found {
		return
	}
	if reject := s.deps.Party.SetLeader(domain.CharID(ch.ID), target.Char); reject != domain.PartyOK {
		s.log.Debug("转让队长被拒", "char", ch.Name, "target", target.Char, "原因", reject)
		return
	}
	pid := s.deps.Party.PartyOf(domain.CharID(ch.ID))
	if p, found := s.deps.Party.Get(pid); found {
		for _, m := range p.Members {
			s.pushPartyRefresh(m.Char)
		}
	}
}

func (s *Session) onPartyDismiss() {
	ch, ok := s.partyIdentity()
	if !ok || s.deps.Party == nil {
		return
	}
	members, reject := s.deps.Party.Disband(domain.CharID(ch.ID))
	if reject != domain.PartyOK {
		s.log.Debug("解散队伍被拒", "char", ch.Name, "原因", reject)
		return
	}
	for _, m := range members {
		s.pushPartyID(m.Char, 0)
	}
}

func (s *Session) onPartyInvite(targetEntity domain.EntityID, targetName string) {
	me, ok := s.partyIdentity()
	if !ok || s.deps.Party == nil || s.deps.Online == nil || s.deps.Requests == nil {
		return
	}
	target, found := s.deps.Online.FindByEntity(targetEntity)
	if !found || target.Name != targetName {
		return
	}
	if s.targetBlocks(target.Char, domain.CharID(me.ID)) {
		return
	}
	if reject := s.deps.Party.Invite(domain.CharID(me.ID), target.Char); reject != domain.PartyOK {
		s.log.Debug("组队邀请被拒", "char", me.Name, "target", target.Name, "原因", reject)
		return
	}
	_, added := s.deps.Requests.AddInteraction(target.Char, social.Request{
		Kind: social.RequestParty, From: domain.CharID(me.ID), FromID: s.entityID(),
		FromName: me.Name, Text: me.Name + "邀请你加入队伍",
	})
	if added {
		s.refreshSystemRequestsOf(target.Char)
	}
}

// onPartyJoinRequest 处理 0x1053：申请人点队长请求入队。客户端仍用
// 0x8035 的 RequestParty 展示确认，因此请求方向只保存在服务端内部。
func (s *Session) onPartyJoinRequest(targetEntity domain.EntityID, targetName string) {
	me, ok := s.partyIdentity()
	if !ok || s.deps.Party == nil || s.deps.Online == nil || s.deps.Requests == nil {
		return
	}
	target, found := s.deps.Online.FindByEntity(targetEntity)
	if !found || target.Name != targetName || target.Char == domain.CharID(me.ID) {
		return
	}
	if s.targetBlocks(target.Char, domain.CharID(me.ID)) {
		return
	}
	pid := s.deps.Party.PartyOf(target.Char)
	partyView, found := s.deps.Party.Get(pid)
	if !found || partyView.Leader != target.Char {
		return
	}
	// 先用名册的统一邀请门禁验证队伍容量、申请人状态和队长身份；接受时
	// 再以相反方向调用 Accept，把申请人加入目标队伍。
	if reject := s.deps.Party.Invite(target.Char, domain.CharID(me.ID)); reject != domain.PartyOK {
		s.log.Debug("申请入队被拒", "char", me.Name, "target", target.Name, "原因", reject)
		return
	}
	_, added := s.deps.Requests.AddInteraction(target.Char, social.Request{
		Kind: social.RequestParty, From: domain.CharID(me.ID), FromID: s.entityID(),
		FromName: me.Name, Text: me.Name + "申请加入你的队伍", PartyApplicant: true,
	})
	if added {
		s.refreshSystemRequestsOf(target.Char)
	}
}

func (s *Session) acceptPartyRequest(me *domain.Character, req social.Request) {
	if s.deps.Party == nil {
		return
	}
	who, whoName := domain.CharID(me.ID), me.Name
	inviter, inviterName := req.From, req.FromName
	if req.PartyApplicant {
		who, whoName = req.From, req.FromName
		inviter, inviterName = domain.CharID(me.ID), me.Name
	}
	p, reject := s.deps.Party.Accept(who, whoName, inviter, inviterName)
	if reject != domain.PartyOK {
		s.log.Debug("接受组队邀请被拒", "char", me.Name, "from", req.FromName, "原因", reject)
		return
	}
	for _, member := range p.Members {
		s.pushPartyID(member.Char, p.ID)
		s.pushPartyRefresh(member.Char)
	}
	// Tip18 的语义是“成功加入别人的队伍”，只给真正的新成员；申请入队时
	// 当前 Session 是队长，不能把提示错发给队长。
	if req.PartyApplicant {
		s.requestTipFor(req.From, 18)
	} else {
		s.postToScene(scene.RequestNewbieTip{ID: s.entityID(), Tip: 18})
	}
}
