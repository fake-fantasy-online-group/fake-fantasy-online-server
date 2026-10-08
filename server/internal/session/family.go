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
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

const familyCommitTimeout = 5 * time.Second

func (s *Session) onFamilyRequest(req protocol.Request) {
	switch req.Kind {
	case protocol.ReqFamilyInvite:
		s.onFamilyInvite(req)
	case protocol.ReqFamilyLeave:
		s.onFamilyLeave()
	case protocol.ReqFamilyCreate:
		s.onFamilyCreate(req)
	case protocol.ReqFamilyList:
		s.refreshFamilyBrowse()
	case protocol.ReqFamilyApply:
		s.onFamilyApply(req.S1)
	case protocol.ReqFamilyManage:
		s.onFamilyManage(req)
	case protocol.ReqFamilyMail:
		s.onFamilyMail(req.S1, req.S2)
	case protocol.ReqFamilyPositions:
		s.refreshFamilyPositions()
	case protocol.ReqFamilyRefresh:
		s.refreshFamily()
	case protocol.ReqFamilyPosName:
		s.onFamilyPositionName(domain.FamilyPositionID(req.X), req.S1)
	case protocol.ReqFamilyStash:
		s.onFamilyStash(req)
	case protocol.ReqFamilyStashPerm:
		s.onFamilyStashPermission(domain.FamilyPositionID(req.U8))
	}
}

func (s *Session) familyStore() (store.FamilyStore, bool) {
	fs, ok := s.deps.Store.(store.FamilyStore)
	return fs, ok
}

func (s *Session) currentCharacter() (*domain.Character, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.char, s.stage == StageInGame && s.char != nil
}

func (s *Session) familyPacket(ctx context.Context, charID int64) []byte {
	fs, ok := s.familyStore()
	if !ok {
		return protocol.FamilySnapshot(protocol.FamilyView{})
	}
	family, err := fs.LoadFamily(ctx, charID)
	if err != nil {
		s.log.Error("读取家族快照失败", "char", charID, "err", err)
		return protocol.FamilySnapshot(protocol.FamilyView{})
	}
	if family == nil {
		return protocol.FamilySnapshot(protocol.FamilyView{})
	}
	if family.MemberCap < 0 || family.MemberCap > 65535 {
		s.log.Error("家族容量超出协议范围", "family", family.ID, "capacity", family.MemberCap)
		return nil
	}
	view := protocol.FamilyView{Name: family.Name, Level: family.Level,
		MemberCap: uint16(family.MemberCap), Proclaim: family.Proclaim, Resist: family.Resist,
		MyPosition: uint8(family.MyPosition), CanInvite: family.CanInvite,
		CanMail: family.CanMail, OpenSelf: true}
	for _, member := range family.Members {
		view.Members = append(view.Members, protocol.FamilyMemberView{
			Name: member.Name, Level: member.Level, Position: member.PositionName,
			Contribution: fmt.Sprintf("%d", member.Contribution),
			Online:       s.deps.Online != nil && s.deps.Online.Online(member.Char),
		})
	}
	pkt := protocol.FamilySnapshot(view)
	if pkt == nil {
		s.log.Error("家族成员超过协议上限", "family", family.ID, "members", len(view.Members))
	}
	return pkt
}

func (s *Session) refreshFamily() {
	ch, inGame := s.currentCharacter()
	if !inGame {
		return
	}
	if pkt := s.familyPacket(context.Background(), ch.ID); pkt != nil {
		s.sink.Send(pkt)
	}
}

func (s *Session) refreshFamilyBrowse() {
	_, inGame := s.currentCharacter()
	fs, ok := s.familyStore()
	if !inGame || !ok {
		return
	}
	entries, err := fs.BrowseFamilies(context.Background())
	if err != nil {
		s.sendGMText("读取家族列表失败")
		return
	}
	view := make([]protocol.FamilyBrowseView, 0, len(entries))
	for _, entry := range entries {
		members, cap := entry.Members, entry.MemberCap
		if members > 255 {
			members = 255
		}
		if cap > 255 {
			cap = 255
		}
		view = append(view, protocol.FamilyBrowseView{Name: entry.Name, Level: entry.Level,
			Members: uint8(members), Capacity: uint8(cap), Text: entry.Proclaim})
	}
	s.sink.Send(protocol.FamilyBrowse(view))
}

func (s *Session) refreshFamilyPositions() {
	ch, inGame := s.currentCharacter()
	fs, ok := s.familyStore()
	if !inGame || !ok {
		return
	}
	positions, err := fs.FamilyPositions(context.Background(), ch.ID)
	if err != nil {
		s.sendGMText("读取家族职位失败")
		return
	}
	s.sink.Send(protocol.FamilyPositions(positions))
}

func (s *Session) refreshFamilyOf(who domain.CharID) {
	_, control, ok := s.controlFor(who)
	if !ok {
		return
	}
	if delivery, ok := control.(online.FamilyControl); ok {
		delivery.RefreshFamily()
	}
}

func (s *Session) refreshFamilyMembers(charID int64, extra ...domain.CharID) {
	fs, ok := s.familyStore()
	if !ok {
		return
	}
	members, err := fs.FamilyMemberIDs(context.Background(), charID)
	if err != nil {
		s.log.Warn("读取待刷新的家族成员失败", "char", charID, "err", err)
	}
	seen := map[domain.CharID]struct{}{}
	for _, id := range append(members, extra...) {
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		s.refreshFamilyOf(id)
	}
}

func (s *Session) onFamilyCreate(req protocol.Request) {
	name, proclaim := strings.TrimSpace(req.S1), strings.TrimSpace(req.S2)
	if name == "" || !utf8.ValidString(name) || !utf8.ValidString(proclaim) ||
		utf8.RuneCountInString(name) > 30 || utf8.RuneCountInString(proclaim) > 500 {
		s.sendGMText("创建家族失败：名称或宣言格式不正确")
		return
	}
	ch, inGame := s.currentCharacter()
	fs, ok := s.familyStore()
	if !inGame || !ok || s.deps.WriteBack == nil || s.deps.Router == nil {
		s.sendGMText("家族功能暂不可用")
		return
	}
	if family, err := fs.LoadFamily(context.Background(), ch.ID); err != nil || family != nil {
		s.sendGMText("创建家族失败：已经加入家族或状态不可用")
		return
	}
	reserved, ok := s.reserveFamily(scene.ReserveFamilyCreate{ID: s.entityID()})
	if !ok {
		return
	}
	commit := s.deps.WriteBack.CommitMutation(reserved.Snapshot,
		func(ctx context.Context, raw store.Store, snap domain.Snapshot) error {
			return raw.WithTx(ctx, func(tx store.Store) error {
				familyStore, ok := tx.(store.FamilyStore)
				if !ok {
					return fmt.Errorf("store: 家族事务能力不可用")
				}
				if err := familyStore.CreateFamily(ctx, snap.Char.ID, name, proclaim); err != nil {
					return err
				}
				return tx.SaveSnapshot(ctx, snap)
			})
		})
	err := <-commit
	s.finalizeFamily(reserved.Reservation, err == nil)
	if err != nil {
		s.sendGMText("创建家族失败：" + err.Error())
		return
	}
	s.sendGMText("家族创建成功，已扣除10金币")
	s.refreshFamily()
	s.refreshFamilyPositions()
}

func (s *Session) reserveFamily(raw scene.Command) (scene.FamilyReserveResult, bool) {
	s.mu.Lock()
	sceneID, inGame := s.scene, s.stage == StageInGame
	s.mu.Unlock()
	if !inGame {
		return scene.FamilyReserveResult{}, false
	}
	reply := make(chan scene.FamilyReserveResult, 1)
	switch cmd := raw.(type) {
	case scene.ReserveFamilyCreate:
		cmd.Reply = reply
		raw = cmd
	case scene.ReserveFamilyMail:
		cmd.Reply = reply
		raw = cmd
	case scene.ReserveDeposit:
		cmd.Reply = reply
		raw = cmd
	case scene.ReserveFamilyStashDeposit:
		cmd.Reply = reply
		raw = cmd
	case scene.ReserveFamilyStashWithdraw:
		cmd.Reply = reply
		raw = cmd
	default:
		return scene.FamilyReserveResult{}, false
	}
	if !s.deps.Router.Post(sceneID, raw) {
		s.sendGMText("操作失败：场景不可用")
		return scene.FamilyReserveResult{}, false
	}
	select {
	case result := <-reply:
		if result.Reason != "" || result.Snapshot.Char == nil {
			s.sendGMText("操作失败：" + fallbackReason(result.Reason))
			return result, false
		}
		return result, true
	case <-time.After(familyCommitTimeout):
		s.sendGMText("操作超时，请重试")
		return scene.FamilyReserveResult{}, false
	}
}

func (s *Session) finalizeFamily(reservation scene.FamilyReservation, commit bool) {
	s.mu.Lock()
	sceneID := s.scene
	s.mu.Unlock()
	done := make(chan struct{})
	if !s.deps.Router.Post(sceneID, scene.FinalizeFamilyReservation{
		Reservation: reservation, Commit: commit, Done: done}) {
		return
	}
	select {
	case <-done:
	case <-time.After(familyCommitTimeout):
	}
}

func (s *Session) onFamilyInvite(req protocol.Request) {
	me, inGame := s.currentCharacter()
	fs, ok := s.familyStore()
	if !inGame || !ok || s.deps.Online == nil || s.deps.Requests == nil {
		return
	}
	family, err := fs.LoadFamily(context.Background(), me.ID)
	if err != nil || family == nil || !family.CanInvite {
		s.sendGMText("邀请失败：没有邀请权限")
		return
	}
	loc, found := s.deps.Online.FindByEntity(domain.EntityID(req.TargetID))
	if !found || !strings.EqualFold(loc.Name, strings.TrimSpace(req.S1)) || loc.Char == domain.CharID(me.ID) {
		s.sendGMText("邀请失败：目标不在线或角色不匹配")
		return
	}
	if s.targetBlocks(loc.Char, domain.CharID(me.ID)) {
		s.sendGMText("邀请未送达")
		return
	}
	if targetFamily, err := fs.LoadFamily(context.Background(), int64(loc.Char)); err != nil || targetFamily != nil {
		s.sendGMText("邀请失败：对方已经加入家族")
		return
	}
	reqView, _ := s.deps.Requests.AddInteraction(loc.Char, social.Request{
		Kind: social.RequestFamilyInvite, From: domain.CharID(me.ID), FromID: s.entityID(),
		FromName: me.Name, Text: "邀请你加入家族「" + family.Name + "」", FamilyID: family.ID,
	})
	s.refreshSystemRequestsOf(loc.Char)
	s.log.Debug("发送家族邀请", "request", reqView.ID, "family", family.Name, "to", loc.Name)
}

func (s *Session) onFamilyApply(name string) {
	me, inGame := s.currentCharacter()
	fs, ok := s.familyStore()
	if !inGame || !ok || s.deps.Requests == nil {
		return
	}
	if family, err := fs.LoadFamily(context.Background(), me.ID); err != nil || family != nil {
		s.sendGMText("申请失败：已经加入家族")
		return
	}
	familyID, leader, resist, err := fs.FamilyByName(context.Background(), name)
	if err != nil {
		s.sendGMText("申请失败：" + err.Error())
		return
	}
	if resist {
		s.sendGMText("申请失败：该家族暂不接受申请")
		return
	}
	loc, control, onlineNow := s.controlFor(leader)
	if !onlineNow {
		s.sendGMText("申请失败：族长当前不在线")
		return
	}
	if controlBlocks(control, domain.CharID(me.ID)) {
		s.sendGMText("申请未送达")
		return
	}
	s.deps.Requests.AddInteraction(leader, social.Request{
		Kind: social.RequestFamilyApply, From: domain.CharID(me.ID), FromID: s.entityID(),
		FromName: me.Name, Text: "申请加入你的家族", FamilyID: familyID, FamilyApplicant: true,
	})
	s.refreshSystemRequestsOf(leader)
	s.log.Debug("发送家族申请", "family", name, "leader", loc.Name, "from", me.Name)
}

func (s *Session) onFamilyLeave() {
	me, inGame := s.currentCharacter()
	fs, ok := s.familyStore()
	if !inGame || !ok {
		return
	}
	members, _ := fs.FamilyMemberIDs(context.Background(), me.ID)
	if err := fs.LeaveFamily(context.Background(), me.ID); err != nil {
		s.sendGMText("退出家族失败：" + err.Error())
		return
	}
	s.sendGMText("已退出家族")
	for _, id := range members {
		s.refreshFamilyOf(id)
	}
}

func (s *Session) onFamilyManage(req protocol.Request) {
	me, inGame := s.currentCharacter()
	fs, ok := s.familyStore()
	if !inGame || !ok {
		return
	}
	members, _ := fs.FamilyMemberIDs(context.Background(), me.ID)
	var target domain.CharID
	var err error
	switch req.U8 {
	case 0:
		err = fs.SetFamilyProclaim(context.Background(), me.ID, req.S2)
	case 1:
		err = fs.SetFamilyResist(context.Background(), me.ID, req.X != 0)
	case 2:
		target, err = fs.SetFamilyMemberPosition(context.Background(), me.ID, req.S1, domain.FamilyPositionID(req.X))
	case 3:
		target, err = fs.SetFamilyMemberPosition(context.Background(), me.ID, req.S1, domain.FamilyOrdinary)
	case 4:
		target, err = fs.TransferFamilyLeader(context.Background(), me.ID, req.S1)
	case 5:
		target, err = fs.KickFamilyMember(context.Background(), me.ID, req.S1)
	default:
		s.sendGMText(fmt.Sprintf("家族管理操作编号%d尚未闭合，请告诉我刚才点击的按钮", req.U8))
		return
	}
	if err != nil {
		s.sendGMText("家族管理失败：" + err.Error())
		return
	}
	for _, id := range append(members, target) {
		if id != 0 {
			s.refreshFamilyOf(id)
		}
	}
	s.refreshFamilyPositions()
}

func (s *Session) onFamilyPositionName(position domain.FamilyPositionID, name string) {
	me, inGame := s.currentCharacter()
	fs, ok := s.familyStore()
	if !inGame || !ok {
		return
	}
	if err := fs.RenameFamilyPosition(context.Background(), me.ID, position, name); err != nil {
		s.sendGMText("修改家族职位失败：" + err.Error())
		return
	}
	s.refreshFamilyPositions()
	s.refreshFamilyMembers(me.ID)
}

func (s *Session) onFamilyMail(title, body string) {
	me, inGame := s.currentCharacter()
	fs, ok := s.familyStore()
	if !inGame || !ok || s.deps.WriteBack == nil || s.deps.Router == nil {
		return
	}
	family, err := fs.LoadFamily(context.Background(), me.ID)
	if err != nil || family == nil || !family.CanMail {
		s.sendGMText("发送家族邮件失败：没有权限")
		return
	}
	reserved, ok := s.reserveFamily(scene.ReserveFamilyMail{ID: s.entityID()})
	if !ok {
		return
	}
	expireDays := s.deps.MailRule.ExpireDays
	if expireDays <= 0 {
		expireDays = 30
	}
	var recipients []domain.CharID
	commit := s.deps.WriteBack.CommitMutation(reserved.Snapshot,
		func(ctx context.Context, raw store.Store, snap domain.Snapshot) error {
			return raw.WithTx(ctx, func(tx store.Store) error {
				familyStore, ok := tx.(store.FamilyStore)
				if !ok {
					return fmt.Errorf("store: 家族邮件能力不可用")
				}
				var err error
				recipients, err = familyStore.SendFamilyMail(ctx, snap.Char.ID, snap.Char.Name,
					strings.TrimSpace(title), strings.TrimSpace(body), time.Now().AddDate(0, 0, int(expireDays)))
				if err != nil {
					return err
				}
				return tx.SaveSnapshot(ctx, snap)
			})
		})
	err = <-commit
	s.finalizeFamily(reserved.Reservation, err == nil)
	if err != nil {
		s.sendGMText("发送家族邮件失败：" + err.Error())
		return
	}
	s.sendGMText("家族邮件发送成功，已消耗" + reserved.Consumed)
	for _, recipient := range recipients {
		if _, control, ok := s.controlFor(recipient); ok {
			if mail, ok := control.(online.MailControl); ok {
				mail.RefreshMailIndicator()
			}
		}
	}
}
