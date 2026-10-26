package scene

import (
	"math"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func (s *Scene) onRenderMailAttachments(cmd RenderMailAttachments) {
	result := MailAttachmentDisplayResult{}
	defer func() { cmd.Reply <- result }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return
	}
	result.Items = make([]MailAttachmentDisplay, 0, len(cmd.Stacks))
	for _, st := range cmd.Stacks {
		def, ok := s.itemDef(st.Item)
		if !ok || st.Empty() {
			return
		}
		result.Items = append(result.Items, MailAttachmentDisplay{
			Name: itemDisplayName(def, st), Desc: s.ownedItemTooltip(def, st, p),
		})
	}
	result.OK = true
}

func (s *Scene) onReserveMailSend(cmd ReserveMailSend) {
	result := MailReserveResult{}
	defer func() { s.replyMailReservation(cmd.ID, cmd.Reply, result) }()
	select {
	case <-cmd.Cancel:
		result.Reason = "邮件操作已取消"
		return
	default:
	}
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil ||
		p.Player.TradeBusy || p.Player.MailBusy || p.Player.StallBusy || p.Player.Stall != nil ||
		p.Player.FamilyBusy || p.Player.RackBusy || cmd.Money < 0 {
		result.Reason = "附件或邮寄金额无效"
		return
	}
	money, err := inventoryMoney(p.Player.Char.Money)
	if err != nil || money < 0 || cmd.Money > money {
		result.Reason = "附件或邮寄金额无效"
		return
	}
	result.Reservation = MailReservation{
		OperationID: cmd.OperationID, Before: s.snapshotOf(p),
		BeforeBag: p.Player.Bag.Clone(), BeforeMoney: p.Player.Char.Money, BeforeCaiyu: p.Player.Char.Caiyu,
	}
	seen := make(map[int32]struct{}, len(cmd.Items))
	for i, ref := range cmd.Items {
		if ref.Slot < 0 || ref.Count <= 0 {
			result.Reason = "附件引用无效"
			return
		}
		if _, dup := seen[ref.Slot]; dup {
			result.Reason = "同一背包格不能重复作为附件"
			return
		}
		seen[ref.Slot] = struct{}{}
		st := p.Player.Bag.At(int(ref.Slot))
		def, ok := s.itemDef(st.Item)
		if !ok || st.Empty() || st.Count < ref.Count || st.Locked || st.Bound || domain.AstralRuneTransferProtected(st) || !def.CanMail ||
			!def.InventoryTabKnown || def.InventoryTab != ref.Tab || st.UID != 0 && ref.Count != 1 && !domain.IsAstralRuneStack(st) {
			result.Reason = "该物品不能邮寄"
			return
		}
		_, attached, splitOK := domain.SplitOwnedStack(st, ref.Count)
		if !splitOK {
			result.Reason = "附件数量无效"
			return
		}
		attached.Locked = false
		result.Attachments = append(result.Attachments, domain.MailAttachment{
			Ordinal: int32(i), Stack: attached,
		})
	}
	for _, ref := range cmd.Items {
		if !p.Player.Bag.RemoveAt(int(ref.Slot), ref.Count) {
			p.Player.Bag = result.Reservation.BeforeBag
			result.Reason = "附件状态已经变化"
			return
		}
	}
	p.Player.Char.Money = moneyFromInventory(money - cmd.Money)
	p.Player.MailBusy = true
	p.Player.MarkDirty()
	result.Snapshot = s.snapshotOf(p)
	s.trackMailReservation(cmd.ID, &result.Reservation)
}

func (s *Scene) onReserveMailClaim(cmd ReserveMailClaim) {
	result := MailReserveResult{}
	defer func() { s.replyMailReservation(cmd.ID, cmd.Reply, result) }()
	select {
	case <-cmd.Cancel:
		result.Reason = "邮件操作已取消"
		return
	default:
	}
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil ||
		p.Player.TradeBusy || p.Player.MailBusy || p.Player.StallBusy || p.Player.Stall != nil ||
		p.Player.FamilyBusy || p.Player.RackBusy || cmd.Mail.Money < 0 || cmd.Mail.Caiyu < 0 ||
		p.Player.Char.Caiyu > math.MaxInt64-cmd.Mail.Caiyu {
		result.Reason = "邮件内容暂时无法领取"
		return
	}
	money, err := inventoryMoney(p.Player.Char.Money)
	afterMoney, added := checkedAdd64(money, cmd.Mail.Money)
	if err != nil || money < 0 || !added {
		result.Reason = "领取金额超过随身金钱上限"
		return
	}
	result.Reservation = MailReservation{
		OperationID: cmd.OperationID, Before: s.snapshotOf(p),
		BeforeBag: p.Player.Bag.Clone(), BeforeMoney: p.Player.Char.Money, BeforeCaiyu: p.Player.Char.Caiyu,
	}
	trial := p.Player.Bag.Clone()
	for _, attachment := range cmd.Mail.Attachments {
		def, ok := s.itemDef(attachment.Stack.Item)
		if !ok || trial.AddStack(def, attachment.Stack) != 0 {
			result.Reason = "背包空间不足"
			return
		}
	}
	p.Player.Bag = trial
	p.Player.Char.Money = moneyFromInventory(afterMoney)
	p.Player.Char.Caiyu += cmd.Mail.Caiyu
	p.Player.MailBusy = true
	p.Player.MarkDirty()
	result.Snapshot = s.snapshotOf(p)
	s.trackMailReservation(cmd.ID, &result.Reservation)
}

// Completion uses a dedicated channel, independent of mailbox capacity. Only
// a known durable outcome may release the reservation.
type pendingMailReservation struct {
	id          domain.EntityID
	reservation MailReservation
}

func (s *Scene) trackMailReservation(id domain.EntityID, reservation *MailReservation) {
	reservation.Result, reservation.Done = make(chan bool, 1), make(chan struct{})
	s.mailReservations = append(s.mailReservations, pendingMailReservation{id, *reservation})
}

func (s *Scene) finishReadyMail(wait bool) {
	for i := 0; i < len(s.mailReservations); {
		pending := s.mailReservations[i]
		var commit bool
		if wait {
			commit = <-pending.reservation.Result
		} else {
			select {
			case commit = <-pending.reservation.Result:
			default:
				i++
				continue
			}
		}
		s.onFinalizeMailReservation(FinalizeMailReservation{
			ID: pending.id, Reservation: pending.reservation, Commit: commit,
		})
	}
}

func (s *Scene) onFinalizeMailReservation(cmd FinalizeMailReservation) {
	if cmd.Done != nil && cmd.Done != cmd.Reservation.Done {
		defer close(cmd.Done)
	}
	var reserved *MailReservation
	for i, pending := range s.mailReservations {
		if pending.id == cmd.ID && pending.reservation.Done == cmd.Reservation.Done &&
			pending.reservation.OperationID == cmd.Reservation.OperationID {
			copy := pending.reservation
			reserved = &copy
			s.mailReservations = append(s.mailReservations[:i], s.mailReservations[i+1:]...)
			break
		}
	}
	if reserved == nil {
		return
	}
	defer close(reserved.Done)
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil {
		return
	}
	if !cmd.Commit {
		p.Player.Bag = reserved.BeforeBag
		p.Player.Char.Money = reserved.BeforeMoney
		p.Player.Char.Caiyu = reserved.BeforeCaiyu
	}
	p.Player.MailBusy = false
	p.Player.MarkDirty()
	_ = s.pushInventory(p)
	s.runDeferredTradeMutations(cmd.ID)
	s.finishTradeLifecycle(cmd.ID)
}

// Reply channels are buffered by the session. An abandoned or invalid caller
// must not block the scene, nor leave a reservation without an outcome owner.
func (s *Scene) replyMailReservation(id domain.EntityID, reply chan<- MailReserveResult, result MailReserveResult) {
	select {
	case reply <- result:
		return
	default:
	}
	if result.Reservation.Done != nil {
		s.onFinalizeMailReservation(FinalizeMailReservation{ID: id, Reservation: result.Reservation})
	}
}

// Panic shutdown cannot execute queued business commands. Reject accepted but
// unexecuted prepares so a session waiting for a late reply can finish safely.
func (s *Scene) rejectUnpreparedMail(c Command) {
	result := MailReserveResult{Reason: "角色场景已关闭"}
	switch cmd := c.(type) {
	case ReserveMailSend:
		s.replyMailReservation(cmd.ID, cmd.Reply, result)
	case ReserveMailClaim:
		s.replyMailReservation(cmd.ID, cmd.Reply, result)
	case ReserveHorn:
		s.replyMailReservation(cmd.ID, cmd.Reply, result)
	}
}
