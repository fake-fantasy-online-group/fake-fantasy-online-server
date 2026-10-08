package session

import (
	"context"
	"errors"
	"strings"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/online"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/social"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

func (s *Session) apprenticeStore() (store.ApprenticeStore, bool) {
	value, ok := s.deps.Store.(store.ApprenticeStore)
	return value, ok
}

func (s *Session) onApprenticeRequest(req protocol.Request) {
	if req.Kind == protocol.ReqApprenticeList {
		s.refreshApprentice()
		return
	}
	me, inGame := s.currentCharacter()
	st, ok := s.apprenticeStore()
	if !inGame || !ok {
		s.sendGMText("师徒功能暂不可用")
		return
	}
	if req.U8 > 2 {
		s.sendGMText("师徒请求失败：操作类型无效")
		return
	}
	name := strings.TrimSpace(req.S1)
	if name == "" || strings.EqualFold(name, me.Name) {
		s.sendGMText("师徒请求失败：目标无效")
		return
	}
	if req.U8 == 2 {
		// 师徒列表的离线行会传 targetId=0；姓名只用来匹配本人持久关系，
		// 真正删除仍用快照里的双方 character ID 精确定位。
		s.onEndApprenticeship(st, me, name)
		return
	}
	if s.deps.Online == nil || s.deps.Requests == nil {
		s.sendGMText("师徒请求失败：在线服务不可用")
		return
	}
	loc, found := s.deps.Online.FindByEntity(domain.EntityID(req.TargetID))
	if !found || !strings.EqualFold(loc.Name, name) || loc.Char == domain.CharID(me.ID) {
		s.sendGMText("师徒请求失败：目标不在线或角色不匹配")
		return
	}
	if s.targetBlocks(loc.Char, domain.CharID(me.ID)) {
		s.sendGMText("师徒请求未送达")
		return
	}
	var master, apprentice domain.CharID
	switch req.U8 {
	case 0: // 拜师：发件人是徒弟，目标是师父。
		master, apprentice = loc.Char, domain.CharID(me.ID)
		if me.Level > domain.ApprenticeMaxLevel || loc.Profile.Level < domain.MasterMinLevel {
			s.sendGMText("师徒请求失败：师父必须高于30级，徒弟必须低于30级")
			return
		}
	case 1: // 收徒：发件人是师父，目标是徒弟。
		master, apprentice = domain.CharID(me.ID), loc.Char
		if me.Level < domain.MasterMinLevel || loc.Profile.Level > domain.ApprenticeMaxLevel {
			s.sendGMText("师徒请求失败：师父必须高于30级，徒弟必须低于30级")
			return
		}
	}
	if err := st.CheckApprenticeship(context.Background(), int64(master), int64(apprentice)); err != nil {
		switch {
		case errors.Is(err, store.ErrApprenticeLevel):
			s.sendGMText("师徒请求失败：" + store.ErrApprenticeLevel.Error())
		case errors.Is(err, store.ErrApprenticeHasMaster):
			s.sendGMText("师徒请求失败：" + store.ErrApprenticeHasMaster.Error())
		case errors.Is(err, store.ErrApprenticeCapacity):
			s.sendGMText("师徒请求失败：" + store.ErrApprenticeCapacity.Error())
		case errors.Is(err, store.ErrApprenticeInvalidPair):
			s.sendGMText("师徒请求失败：" + store.ErrApprenticeInvalidPair.Error())
		default:
			s.log.Error("检查师徒请求失败", "master", master, "apprentice", apprentice, "err", err)
			s.sendGMText("师徒请求失败：读取关系失败，请稍后重试")
		}
		return
	}
	text := "请求与你建立师徒关系"
	request, _ := s.deps.Requests.AddInteraction(loc.Char, social.Request{
		Kind: social.RequestApprentice, From: domain.CharID(me.ID), FromID: s.entityID(),
		FromName: me.Name, Text: text, Master: master, Apprentice: apprentice,
	})
	s.refreshSystemRequestsOf(loc.Char)
	s.log.Debug("发送师徒请求", "request", request.ID, "master", master, "apprentice", apprentice,
		"clientRole", req.U8)
}

func (s *Session) onEndApprenticeship(st store.ApprenticeStore, me *domain.Character, name string) {
	snapshot, err := st.LoadApprenticeSnapshot(context.Background(), me.ID)
	if err != nil {
		s.log.Error("读取待解除师徒关系失败", "char", me.ID, "err", err)
		s.sendGMText("解除师徒关系失败：读取关系失败")
		return
	}
	var master, apprentice domain.CharID
	if snapshot.Master != nil && strings.EqualFold(snapshot.Master.Name, name) {
		master, apprentice = snapshot.Master.Char, domain.CharID(me.ID)
	} else {
		for _, pupil := range snapshot.Apprentices {
			if strings.EqualFold(pupil.Name, name) {
				master, apprentice = domain.CharID(me.ID), pupil.Char
				break
			}
		}
	}
	if master == 0 || apprentice == 0 {
		s.sendGMText("解除师徒关系失败：双方没有师徒关系")
		return
	}
	if err := st.DeleteApprenticeship(context.Background(), int64(master), int64(apprentice)); err != nil {
		switch {
		case errors.Is(err, store.ErrApprenticeNotFound):
			s.sendGMText("解除师徒关系失败：" + store.ErrApprenticeNotFound.Error())
		case errors.Is(err, store.ErrApprenticeInvalidPair):
			s.sendGMText("解除师徒关系失败：" + store.ErrApprenticeInvalidPair.Error())
		default:
			s.log.Error("删除师徒关系失败", "master", master, "apprentice", apprentice, "err", err)
			s.sendGMText("解除师徒关系失败：操作失败，请稍后重试")
		}
		return
	}
	if s.deps.Requests != nil {
		s.deps.Requests.DropApprenticeship(master, apprentice)
		s.refreshSystemRequestsOf(master)
		s.refreshSystemRequestsOf(apprentice)
	}
	s.refreshApprenticeCircle(master)
	s.refreshApprenticeOf(apprentice)
	s.sendGMText("师徒关系已解除")
}

func (s *Session) apprenticePacket(ctx context.Context, charID int64) []byte {
	st, ok := s.apprenticeStore()
	if !ok {
		return protocol.ApprenticeSnapshot(protocol.ApprenticeView{})
	}
	snapshot, err := st.LoadApprenticeSnapshot(ctx, charID)
	if err != nil {
		s.log.Error("读取师徒快照失败", "char", charID, "err", err)
		return protocol.ApprenticeSnapshot(protocol.ApprenticeView{})
	}
	view := protocol.ApprenticeView{}
	person := func(p domain.SocialPerson) protocol.SocialPersonView {
		return protocol.SocialPersonView{Name: p.Name, Level: p.Level,
			Online: s.deps.Online != nil && s.deps.Online.Online(p.Char)}
	}
	if snapshot.Master != nil {
		view.Master = person(*snapshot.Master)
	}
	for _, peer := range snapshot.Peers {
		view.Peers = append(view.Peers, person(peer))
	}
	for _, apprentice := range snapshot.Apprentices {
		view.Apprentices = append(view.Apprentices, person(apprentice))
	}
	return protocol.ApprenticeSnapshot(view)
}

func (s *Session) refreshApprentice() {
	ch, inGame := s.currentCharacter()
	if !inGame {
		return
	}
	if packet := s.apprenticePacket(context.Background(), ch.ID); packet != nil {
		s.sink.Send(packet)
	}
}

func (s *Session) refreshApprenticeOf(who domain.CharID) {
	_, control, ok := s.controlFor(who)
	if !ok {
		return
	}
	if delivery, ok := control.(online.ApprenticeControl); ok {
		delivery.RefreshApprentice()
	}
}

func (s *Session) refreshApprenticeCircle(master domain.CharID) {
	st, ok := s.apprenticeStore()
	if !ok {
		return
	}
	snapshot, err := st.LoadApprenticeSnapshot(context.Background(), int64(master))
	if err != nil {
		return
	}
	s.refreshApprenticeOf(master)
	for _, apprentice := range snapshot.Apprentices {
		s.refreshApprenticeOf(apprentice.Char)
	}
}
