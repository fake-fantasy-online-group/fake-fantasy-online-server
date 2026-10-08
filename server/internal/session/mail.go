package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/online"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

const (
	// Official 1.5.8 MailDlg.DrawList/DrawRead command identities.
	mailActionClaim     uint8 = 0
	mailActionDelete    uint8 = 1
	mailActionReturn    uint8 = 2
	mailActionDeleteAll uint8 = 3
	mailActionRead      uint8 = 4
)

func (s *Session) mailBinding() (domain.EntityID, domain.SceneID, *domain.Character, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.entity, s.scene, s.char, s.stage == StageInGame && s.char != nil
}

func (s *Session) mailStore() (store.MailStore, bool) {
	ms, ok := s.deps.Store.(store.MailStore)
	return ms, ok && s.deps.MailRule.Valid()
}

func (s *Session) refreshMailList() {
	id, sc, ch, inGame := s.mailBinding()
	ms, ok := s.mailStore()
	if !inGame || !ok {
		s.sendGMText("邮件系统暂不可用")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mails, err := ms.LoadMails(ctx, ch.ID, s.deps.MailRule)
	if err != nil {
		s.log.Error("读取邮件失败", "char", ch.Name, "err", err)
		s.sendGMText("读取邮件失败，请稍后重试")
		return
	}
	var stacks []domain.Stack
	for _, mail := range mails {
		for _, attachment := range mail.Attachments {
			stacks = append(stacks, attachment.Stack)
		}
	}
	var displays []scene.MailAttachmentDisplay
	if len(stacks) > 0 {
		reply := make(chan scene.MailAttachmentDisplayResult, 1)
		if s.deps.Router == nil || !s.deps.Router.Post(sc, scene.RenderMailAttachments{ID: id, Stacks: stacks, Reply: reply}) {
			s.sendGMText("读取邮件附件失败：角色场景不可用")
			return
		}
		select {
		case result := <-reply:
			if !result.OK || len(result.Items) != len(stacks) {
				s.log.Error("邮件附件显示数据无效", "char", ch.Name)
				s.sendGMText("读取邮件附件失败，请稍后重试")
				return
			}
			displays = result.Items
		case <-time.After(2 * time.Second):
			s.sendGMText("读取邮件附件超时，请稍后重试")
			return
		}
	}
	views := make([]protocol.MailView, 0, len(mails))
	var unread int32
	displayIndex := 0
	for _, mail := range mails {
		if !mail.Read {
			unread++
		}
		view := protocol.MailView{
			ID: mail.ID, From: mail.SenderName, Title: mail.Title, Body: mail.Body,
			Money: mail.Money, Caiyu: mail.Caiyu, Type: mail.Type, Read: mail.Read,
			CanClaim: mail.CanClaim(), CanReturn: mail.CanReturn(), Time: mail.CreatedAt.Unix(),
		}
		for _, attachment := range mail.Attachments {
			display := displays[displayIndex]
			displayIndex++
			view.Attachments = append(view.Attachments, protocol.MailAttachmentView{
				ItemID: int32(attachment.Stack.Item), Count: attachment.Stack.Count,
				Name: display.Name, Desc: display.Desc,
			})
		}
		views = append(views, view)
	}
	s.sink.Send(protocol.MailList(int32(len(mails)), s.deps.MailRule.Capacity, unread, views))
	// MailNew(true) is a new-arrival event: a visible official mailbox responds
	// by requesting this list again. Never echo it from a list response.
	if unread == 0 {
		s.sink.Send(protocol.MailNew(false))
	}
}

func (s *Session) refreshMailIndicator() {
	_, _, ch, inGame := s.mailBinding()
	ms, ok := s.mailStore()
	if !inGame || !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mails, err := ms.LoadMails(ctx, ch.ID, s.deps.MailRule)
	if err != nil {
		return
	}
	hasNew := false
	for _, mail := range mails {
		if !mail.Read {
			hasNew = true
			break
		}
	}
	s.sink.Send(protocol.MailNew(hasNew))
}

func (s *Session) onSendMail(req protocol.Request) {
	id, sc, _, inGame := s.mailBinding()
	ms, ok := s.mailStore()
	if !inGame || !ok || s.deps.WriteBack == nil {
		s.sendGMText("邮件系统暂不可用")
		return
	}
	to := strings.TrimSpace(req.S1)
	title := strings.TrimSpace(req.S2)
	body := strings.TrimSpace(req.S3)
	if to == "" || !utf8.ValidString(to) || utf8.RuneCountInString(to) > 24 ||
		utf8.RuneCountInString(title) > 40 || utf8.RuneCountInString(body) > 500 ||
		req.Money < 0 || req.Caiyu != 0 || len(req.MailItems) > int(s.deps.MailRule.MaxAttach) || req.U8 > 2 {
		s.sendGMText("邮件内容、金额或附件无效；彩玉邮件暂未开放")
		return
	}
	if title == "" {
		title = "无标题"
	}
	refs := make([]scene.MailItemRef, 0, len(req.MailItems))
	for _, ref := range req.MailItems {
		refs = append(refs, scene.MailItemRef{Tab: ref.Tab, Slot: int32(clientBagSlot(ref.Tab, int(ref.Slot))), Count: ref.Count})
	}
	reply := make(chan scene.MailReserveResult, 1)
	if !s.deps.Router.Post(sc, scene.ReserveMailSend{ID: id, Money: req.Money, Items: refs, Reply: reply}) {
		s.sendGMText("邮件发送失败：角色场景不可用")
		return
	}
	var reserved scene.MailReserveResult
	select {
	case reserved = <-reply:
	case <-time.After(3 * time.Second):
		s.sendGMText("邮件发送超时，请重试")
		return
	}
	if reserved.Reason != "" {
		s.sendGMText("邮件发送失败：" + reserved.Reason)
		return
	}
	draft := domain.MailDraft{RecipientName: to, Title: title, Body: body,
		Money: req.Money, Type: req.U8, Attachments: reserved.Attachments}
	commit := s.deps.WriteBack.CommitMutation(reserved.Snapshot,
		func(ctx context.Context, _ store.Store, snap domain.Snapshot) error {
			_, err := ms.SendMail(ctx, snap, draft, s.deps.MailRule)
			return err
		})
	err := <-commit
	s.finalizeMailReservation(sc, id, reserved.Reservation, err == nil)
	if err != nil {
		s.sendGMText(mailErrorMessage("发送", err))
		return
	}
	s.sendGMText("邮件发送成功")
	if s.deps.Online != nil {
		if _, control, ok := s.deps.Online.ControlByName(to); ok {
			if mailControl, ok := control.(online.MailControl); ok {
				mailControl.RefreshMailIndicator()
			}
		}
	}
	s.refreshMailList()
}

func (s *Session) onMailAction(req protocol.Request) {
	id, sc, ch, inGame := s.mailBinding()
	ms, ok := s.mailStore()
	if !inGame || !ok || (req.U8 == mailActionDeleteAll && req.MailID != 0) ||
		(req.U8 != mailActionDeleteAll && req.MailID <= 0) {
		return
	}
	s.log.Info("收到邮件操作", "action", req.U8, "mail", req.MailID, "char", ch.Name)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	switch req.U8 {
	case mailActionRead:
		if err := ms.MarkMailRead(ctx, ch.ID, req.MailID); err != nil {
			s.sendGMText(mailErrorMessage("读取", err))
		}
	case mailActionClaim:
		if s.deps.WriteBack == nil {
			s.sendGMText("邮件领取暂不可用")
			return
		}
		mails, err := ms.LoadMails(ctx, ch.ID, s.deps.MailRule)
		if err != nil {
			s.sendGMText(mailErrorMessage("领取", err))
			return
		}
		var selected *domain.Mail
		for i := range mails {
			if mails[i].ID == req.MailID {
				selected = &mails[i]
				break
			}
		}
		if selected == nil {
			s.sendGMText("邮件不存在")
			return
		}
		reply := make(chan scene.MailReserveResult, 1)
		if !s.deps.Router.Post(sc, scene.ReserveMailClaim{ID: id, Mail: *selected, Reply: reply}) {
			s.sendGMText("邮件领取失败：角色场景不可用")
			return
		}
		reserved := <-reply
		if reserved.Reason != "" {
			s.sendGMText("邮件领取失败：" + reserved.Reason)
			return
		}
		commit := s.deps.WriteBack.CommitMutation(reserved.Snapshot,
			func(ctx context.Context, _ store.Store, snap domain.Snapshot) error {
				return ms.ClaimMail(ctx, snap, req.MailID)
			})
		err = <-commit
		s.finalizeMailReservation(sc, id, reserved.Reservation, err == nil)
		if err != nil {
			s.sendGMText(mailErrorMessage("领取", err))
		} else {
			s.sendGMText("邮件附件已领取")
		}
	case mailActionReturn:
		if err := ms.ReturnMail(ctx, ch.ID, req.MailID, s.deps.MailRule); err != nil {
			s.sendGMText(mailErrorMessage("退回", err))
		}
	case mailActionDelete:
		if err := ms.DeleteMail(ctx, ch.ID, req.MailID); err != nil {
			s.sendGMText(mailErrorMessage("删除", err))
		}
	case mailActionDeleteAll:
		mails, err := ms.LoadMails(ctx, ch.ID, s.deps.MailRule)
		if err != nil {
			s.sendGMText(mailErrorMessage("删除", err))
			return
		}
		for _, mail := range mails {
			// Reuse the transactional ownership and unclaimed-asset protections
			// of single-mail deletion; bulk deletion cannot discard attachments.
			if err := ms.DeleteMail(ctx, ch.ID, mail.ID); err != nil &&
				!errors.Is(err, store.ErrMailCannotDelete) && !errors.Is(err, store.ErrMailNotFound) {
				s.sendGMText(mailErrorMessage("删除", err))
				break
			}
		}
	default:
		s.sendGMText(fmt.Sprintf("邮件操作编号 %d 尚未闭合，请告诉我刚才点击的按钮", req.U8))
	}
	s.refreshMailList()
}

func (s *Session) finalizeMailReservation(sc domain.SceneID, id domain.EntityID,
	reservation scene.MailReservation, commit bool) {
	done := make(chan struct{})
	if !s.deps.Router.PostLifecycle(sc, scene.FinalizeMailReservation{
		ID: id, Reservation: reservation, Commit: commit, Done: done,
	}) {
		return
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

func mailErrorMessage(action string, err error) string {
	switch {
	case errors.Is(err, store.ErrMailRecipientNotFound):
		return "邮件" + action + "失败：收件人不存在"
	case errors.Is(err, store.ErrMailSelf):
		return "不能给自己发送邮件"
	case errors.Is(err, store.ErrMailboxFull):
		return "邮件" + action + "失败：邮箱已满"
	case errors.Is(err, store.ErrMailNothingToClaim):
		return "该邮件没有可领取内容"
	case errors.Is(err, store.ErrMailCannotDelete):
		return "请先领取或退回附件，再删除邮件"
	case errors.Is(err, store.ErrMailCannotReturn):
		return "该邮件不能退回"
	case errors.Is(err, store.ErrMailNotFound):
		return "邮件不存在"
	default:
		return "邮件" + action + "失败，请稍后重试"
	}
}
