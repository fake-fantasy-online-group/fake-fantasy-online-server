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
	defer func() { cmd.Reply <- result }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil ||
		p.Player.TradeBusy || p.Player.MailBusy || cmd.Money < 0 {
		result.Reason = "附件或邮寄金额无效"
		return
	}
	money, err := inventoryMoney(p.Player.Char.Money)
	if err != nil || money < 0 || cmd.Money > money {
		result.Reason = "附件或邮寄金额无效"
		return
	}
	result.Reservation = MailReservation{
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
		if !ok || st.Empty() || st.Count < ref.Count || st.Locked || st.Bound || !def.CanMail ||
			!def.InventoryTabKnown || def.InventoryTab != ref.Tab || st.UID != 0 && ref.Count != 1 {
			result.Reason = "该物品不能邮寄"
			return
		}
		attached := st
		attached.Count = ref.Count
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
}

func (s *Scene) onReserveMailClaim(cmd ReserveMailClaim) {
	result := MailReserveResult{}
	defer func() { cmd.Reply <- result }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil ||
		p.Player.TradeBusy || p.Player.MailBusy || cmd.Mail.Money < 0 || cmd.Mail.Caiyu < 0 ||
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
}

func (s *Scene) onFinalizeMailReservation(cmd FinalizeMailReservation) {
	defer close(cmd.Done)
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil {
		return
	}
	p.Player.MailBusy = false
	if !cmd.Commit {
		p.Player.Bag = cmd.Reservation.BeforeBag
		p.Player.Char.Money = cmd.Reservation.BeforeMoney
		p.Player.Char.Caiyu = cmd.Reservation.BeforeCaiyu
		p.Player.MarkDirty()
		if s.saver != nil {
			s.saver.Save(s.snapshotOf(p))
		}
	}
	_ = s.pushInventory(p)
}
