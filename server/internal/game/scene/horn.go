package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// ReserveHorn consumes one real, unlocked horn on the actor. The session commits
// this snapshot through the ordered writeback queue before broadcasting text.
type ReserveHorn struct {
	ID    domain.EntityID
	Item  domain.ItemID
	Skin  uint8
	Reply chan<- MailReserveResult
}

func (ReserveHorn) CmdName() string { return "ReserveHorn" }
func (s *Scene) onReserveHorn(cmd ReserveHorn) {
	result := MailReserveResult{}
	defer func() { cmd.Reply <- result }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || !p.Alive() || p.Player.Stall != nil {
		result.Reason = "当前不能使用喇叭"
		return
	}
	def, ok := s.itemDef(cmd.Item)
	if !ok || def.HornTier == 0 || cmd.Skin != def.HornSkin || p.Level < def.UseLevel {
		result.Reason = "喇叭或外观无效"
		return
	}
	slot := -1
	p.Player.Bag.Each(func(i int, st domain.Stack) {
		if slot < 0 && st.Item == cmd.Item && !st.Empty() && !st.Locked {
			slot = i
		}
	})
	if slot < 0 {
		result.Reason = "没有可使用的喇叭"
		return
	}
	result.Reservation = MailReservation{BeforeBag: p.Player.Bag.Clone(), BeforeMoney: p.Player.Char.Money, BeforeCaiyu: p.Player.Char.Caiyu}
	if !p.Player.Bag.RemoveAt(slot, 1) {
		result.Reason = "喇叭数量不足"
		return
	}
	p.Player.MarkDirty()
	result.Snapshot = s.snapshotOf(p)
}
