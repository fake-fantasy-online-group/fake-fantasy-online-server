package session

import (
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/social"
)

func (s *Session) checkSharedRide(sc domain.SceneID, command scene.SharedRide) scene.SharedRideResult {
	reply := make(chan scene.SharedRideResult, 1)
	command.Reply = reply
	if s.deps.Router == nil {
		return scene.SharedRideResult{Reason: "同乘服务不可用"}
	}
	s.deps.Router.Post(sc, command)
	select {
	case result := <-reply:
		return result
	case <-time.After(3 * time.Second):
		return scene.SharedRideResult{Reason: "同乘操作超时"}
	}
}

// reverse 是客户端请求搭乘他人；RideInvite 则是坐骑主人发出邀请。
// 两个方向都经过接收人的原生确认，不接受无邀请的直接上车。
func (s *Session) onRideInvite(targetID domain.EntityID, reverse bool) {
	me, ok := s.partyIdentity()
	if !ok || s.deps.Online == nil || s.deps.Requests == nil {
		return
	}
	if targetID == 0 && reverse {
		s.postToScene(scene.LeaveSharedRide{ID: s.entityID()})
		return
	}
	self, found := s.deps.Online.Find(domain.CharID(me.ID))
	target, otherFound := s.deps.Online.FindByEntity(targetID)
	if !found || !otherFound || self.Scene != target.Scene || self.Char == target.Char || s.targetBlocks(target.Char, self.Char) {
		return
	}
	driver, passenger := self.Entity, target.Entity
	text := self.Name + "邀请你同乘，是否同意？"
	if reverse {
		driver, passenger = target.Entity, self.Entity
		text = self.Name + "请求与你同乘，是否同意？"
	}
	check := s.checkSharedRide(self.Scene, scene.SharedRide{Driver: driver, Passenger: passenger})
	if !check.OK {
		s.sendGMText(check.Reason)
		return
	}
	s.deps.Requests.AddInteraction(target.Char, social.Request{
		Kind: social.RequestRide, From: self.Char, FromID: self.Entity, FromName: self.Name, Text: text,
		RideDriver: driver, RidePassenger: passenger, RideEpoch: check.Epoch, RideScene: self.Scene,
		ExpiresAt: time.Now().Add(time.Duration(check.InviteSeconds) * time.Second),
	})
	s.refreshSystemRequestsOf(target.Char)
	s.sendGMText("同乘邀请已发送")
}

func (s *Session) acceptRideRequest(req social.Request) {
	if !time.Now().Before(req.ExpiresAt) {
		s.sendGMText("该邀请信息已失效")
		return
	}
	s.mu.Lock()
	sc, entity := s.scene, s.entity
	s.mu.Unlock()
	if sc != req.RideScene || (entity != req.RideDriver && entity != req.RidePassenger) {
		s.sendGMText("该邀请信息已失效")
		return
	}
	result := s.checkSharedRide(sc, scene.SharedRide{Driver: req.RideDriver, Passenger: req.RidePassenger,
		Commit: true, Epoch: req.RideEpoch, ExpiresAt: req.ExpiresAt})
	if !result.OK {
		s.sendGMText(result.Reason)
	}
}
