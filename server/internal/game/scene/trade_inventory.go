package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

type pendingTradeSettlement struct {
	reservation TradeSettlementReservation
	result      <-chan bool
	done        chan struct{}
	cancel      <-chan struct{}
}

func (s *Scene) trackTradeSettlement(reservation *TradeSettlementReservation, cancel <-chan struct{}) {
	result, done := make(chan bool, 1), make(chan struct{})
	reservation.Result, reservation.Done = result, done
	s.tradeSettlements = append(s.tradeSettlements, &pendingTradeSettlement{
		reservation: *reservation, result: result, done: done, cancel: cancel,
	})
}

func (s *Scene) removePendingTrade(reservation TradeSettlementReservation) chan struct{} {
	for i, pending := range s.tradeSettlements {
		if pending.reservation.Done != reservation.Done {
			continue
		}
		s.tradeSettlements = append(s.tradeSettlements[:i], s.tradeSettlements[i+1:]...)
		return pending.done
	}
	return nil
}

func (s *Scene) finishReadyTrades(wait bool) {
	for i := 0; i < len(s.tradeSettlements); {
		pending := s.tradeSettlements[i]
		var commit bool
		if wait {
			select {
			case commit = <-pending.result:
			case <-pending.cancel:
			}
		} else {
			select {
			case commit = <-pending.result:
			case <-pending.cancel:
			default:
				i++
				continue
			}
		}
		s.onFinalizeTradeSettlement(FinalizeTradeSettlement{Reservation: pending.reservation, Commit: commit})
	}
}

func (s *Scene) tradeInventoryBusy(id domain.EntityID) bool {
	p := s.players[id]
	return p != nil && p.Player != nil && p.Player.TradeBusy
}

// Only server-owned effects and lifecycle commands are deferred. Client slot
// operations are rejected: replaying an old slot after a completed trade could
// operate on a different item that has just arrived in that slot.
func (s *Scene) deferTradeMutation(id domain.EntityID, fn func()) bool {
	if !s.tradeInventoryBusy(id) {
		return false
	}
	if s.tradeDeferred == nil {
		s.tradeDeferred = make(map[domain.EntityID][]func())
	}
	s.tradeDeferred[id] = append(s.tradeDeferred[id], fn)
	return true
}

func (s *Scene) runDeferredTradeMutations(id domain.EntityID) {
	deferred := s.tradeDeferred[id]
	delete(s.tradeDeferred, id)
	for _, fn := range deferred {
		fn()
	}
}

// All externally requested inventory mutations pass this actor boundary. The
// receive/reply operations retain their normal failure replies instead of leaving
// a session waiting for a result that can never arrive.
func (s *Scene) rejectTradeInventoryCommand(c Command) bool {
	cmd, ok := c.(interface{ inventoryOwner() domain.EntityID })
	if !ok || !s.tradeInventoryBusy(cmd.inventoryOwner()) {
		return false
	}
	const reason = "交易正在结算，请稍后再操作物品"
	id := cmd.inventoryOwner()
	switch cmd := c.(type) {
	case GMCommand:
		replyTradeBusy(cmd.Reply, GMResult{Message: reason})
	case CreateStall:
		replyTradeBusy(cmd.Reply, StallReserveResult{Reason: reason})
	case ReserveStallAdd:
		replyTradeBusy(cmd.Reply, StallReserveResult{Reason: reason})
	case ReserveStallDel:
		replyTradeBusy(cmd.Reply, StallReserveResult{Reason: reason})
	case ReserveStallEnd:
		replyTradeBusy(cmd.Reply, StallReserveResult{Reason: reason})
	case ReserveStallDeal:
		replyTradeBusy(cmd.Reply, StallReserveResult{Reason: reason})
	case ReserveMailSend:
		replyTradeBusy(cmd.Reply, MailReserveResult{Reason: reason})
	case ReserveMailClaim:
		replyTradeBusy(cmd.Reply, MailReserveResult{Reason: reason})
	case ReserveRackPurchase:
		replyTradeBusy(cmd.Reply, RackReserveResult{Reason: reason})
	case ReserveRackRefund:
		replyTradeBusy(cmd.Reply, RackReserveResult{Reason: reason})
	case ReserveFamilyCreate:
		replyTradeBusy(cmd.Reply, FamilyReserveResult{Reason: reason})
	case ReserveFamilyMail:
		replyTradeBusy(cmd.Reply, FamilyReserveResult{Reason: reason})
	case ReserveFamilyStashDeposit:
		replyTradeBusy(cmd.Reply, FamilyReserveResult{Reason: reason})
	case ReserveFamilyStashWithdraw:
		replyTradeBusy(cmd.Reply, FamilyReserveResult{Reason: reason})
	case ReserveDeposit:
		replyTradeBusy(cmd.Reply, FamilyReserveResult{Reason: reason})
	default:
		s.emitTo(id, event.ServerNotice{Who: id, Text: reason})
	}
	return true
}

func replyTradeBusy[T any](reply chan<- T, result T) {
	select {
	case reply <- result:
	default:
	}
}

func (s *Scene) deferTradeLeave(cmd Leave) bool {
	if !s.tradeInventoryBusy(cmd.ID) {
		return false
	}
	if s.tradeLeaves == nil {
		s.tradeLeaves = make(map[domain.EntityID]Leave)
	}
	if previous, ok := s.tradeLeaves[cmd.ID]; ok && cmd.FinalSaver == nil {
		cmd.FinalSaver = previous.FinalSaver
	}
	s.tradeLeaves[cmd.ID] = cmd
	return true
}

func (s *Scene) deferTradeTeleport(cmd Teleport) bool {
	if !s.tradeInventoryBusy(cmd.ID) {
		return false
	}
	if s.tradeTeleports == nil {
		s.tradeTeleports = make(map[domain.EntityID]Teleport)
	}
	s.tradeTeleports[cmd.ID] = cmd
	return true
}

func (s *Scene) finishTradeLifecycle(id domain.EntityID) {
	leave, leaving := s.tradeLeaves[id]
	teleport, teleporting := s.tradeTeleports[id]
	delete(s.tradeLeaves, id)
	delete(s.tradeTeleports, id)
	if leaving {
		s.onLeave(leave)
		return
	}
	// During shutdown the source scene must keep ownership for its final save.
	select {
	case <-s.quit:
		return
	default:
	}
	if teleporting {
		s.onTeleport(teleport)
	}
}
func (cmd Attack) inventoryOwner() domain.EntityID                     { return cmd.ID }
func (cmd UseSkill) inventoryOwner() domain.EntityID                   { return cmd.ID }
func (cmd Equip) inventoryOwner() domain.EntityID                      { return cmd.ID }
func (cmd Unequip) inventoryOwner() domain.EntityID                    { return cmd.ID }
func (cmd ChangeSetItem) inventoryOwner() domain.EntityID              { return cmd.ID }
func (cmd ChangeSetSwap) inventoryOwner() domain.EntityID              { return cmd.ID }
func (cmd BuyItem) inventoryOwner() domain.EntityID                    { return cmd.ID }
func (cmd SellItem) inventoryOwner() domain.EntityID                   { return cmd.ID }
func (cmd MoveBagItem) inventoryOwner() domain.EntityID                { return cmd.ID }
func (cmd SortBag) inventoryOwner() domain.EntityID                    { return cmd.ID }
func (cmd WarehouseMove) inventoryOwner() domain.EntityID              { return cmd.ID }
func (cmd WarehouseMoney) inventoryOwner() domain.EntityID             { return cmd.ID }
func (cmd ExpandWarehouse) inventoryOwner() domain.EntityID            { return cmd.ID }
func (cmd WarehouseSplit) inventoryOwner() domain.EntityID             { return cmd.ID }
func (cmd WarehouseRearrange) inventoryOwner() domain.EntityID         { return cmd.ID }
func (cmd SortWarehouse) inventoryOwner() domain.EntityID              { return cmd.ID }
func (cmd WardrobeStore) inventoryOwner() domain.EntityID              { return cmd.ID }
func (cmd WardrobeWear) inventoryOwner() domain.EntityID               { return cmd.ID }
func (cmd WardrobeRemove) inventoryOwner() domain.EntityID             { return cmd.ID }
func (cmd WardrobeMove) inventoryOwner() domain.EntityID               { return cmd.ID }
func (cmd ChangeHair) inventoryOwner() domain.EntityID                 { return cmd.ID }
func (cmd DropBagItem) inventoryOwner() domain.EntityID                { return cmd.ID }
func (cmd SplitBagItem) inventoryOwner() domain.EntityID               { return cmd.ID }
func (cmd SetItemLock) inventoryOwner() domain.EntityID                { return cmd.ID }
func (cmd Repair) inventoryOwner() domain.EntityID                     { return cmd.ID }
func (cmd RepairConfirm) inventoryOwner() domain.EntityID              { return cmd.ID }
func (cmd PickUp) inventoryOwner() domain.EntityID                     { return cmd.ID }
func (cmd UseItem) inventoryOwner() domain.EntityID                    { return cmd.ID }
func (cmd ApplyAvatar) inventoryOwner() domain.EntityID                { return cmd.ID }
func (cmd StartWork) inventoryOwner() domain.EntityID                  { return cmd.ID }
func (cmd GatherWork) inventoryOwner() domain.EntityID                 { return cmd.ID }
func (cmd CapturePet) inventoryOwner() domain.EntityID                 { return cmd.ID }
func (cmd HatchPetAt) inventoryOwner() domain.EntityID                 { return cmd.ID }
func (cmd FeedPet) inventoryOwner() domain.EntityID                    { return cmd.ID }
func (cmd AcceptQuest) inventoryOwner() domain.EntityID                { return cmd.ID }
func (cmd CompleteQuest) inventoryOwner() domain.EntityID              { return cmd.ID }
func (cmd AbandonQuest) inventoryOwner() domain.EntityID               { return cmd.ID }
func (cmd UpgradeSkill) inventoryOwner() domain.EntityID               { return cmd.ID }
func (cmd LearnLife) inventoryOwner() domain.EntityID                  { return cmd.ID }
func (cmd CraftItem) inventoryOwner() domain.EntityID                  { return cmd.ID }
func (cmd RefineItem) inventoryOwner() domain.EntityID                 { return cmd.ID }
func (cmd DrillItem) inventoryOwner() domain.EntityID                  { return cmd.ID }
func (cmd InlayItem) inventoryOwner() domain.EntityID                  { return cmd.ID }
func (cmd UseItemOn) inventoryOwner() domain.EntityID                  { return cmd.ID }
func (cmd WashAffix) inventoryOwner() domain.EntityID                  { return cmd.ID }
func (cmd Revive) inventoryOwner() domain.EntityID                     { return cmd.ID }
func (cmd GMCommand) inventoryOwner() domain.EntityID                  { return cmd.ID }
func (cmd CreateStall) inventoryOwner() domain.EntityID                { return cmd.ID }
func (cmd ReserveStallAdd) inventoryOwner() domain.EntityID            { return cmd.ID }
func (cmd ReserveStallDel) inventoryOwner() domain.EntityID            { return cmd.ID }
func (cmd ReserveStallEnd) inventoryOwner() domain.EntityID            { return cmd.ID }
func (cmd ReserveStallDeal) inventoryOwner() domain.EntityID           { return cmd.ID }
func (cmd ReserveMailSend) inventoryOwner() domain.EntityID            { return cmd.ID }
func (cmd ReserveMailClaim) inventoryOwner() domain.EntityID           { return cmd.ID }
func (cmd ReserveRackPurchase) inventoryOwner() domain.EntityID        { return cmd.ID }
func (cmd ReserveRackRefund) inventoryOwner() domain.EntityID          { return cmd.ID }
func (cmd ReserveFamilyCreate) inventoryOwner() domain.EntityID        { return cmd.ID }
func (cmd ReserveFamilyMail) inventoryOwner() domain.EntityID          { return cmd.ID }
func (cmd ReserveFamilyStashDeposit) inventoryOwner() domain.EntityID  { return cmd.ID }
func (cmd ReserveFamilyStashWithdraw) inventoryOwner() domain.EntityID { return cmd.ID }
func (cmd ReserveDeposit) inventoryOwner() domain.EntityID             { return cmd.ID }

func (cmd TrialAction) inventoryOwner() domain.EntityID { return cmd.ID }

func (cmd SetPetSkill) inventoryOwner() domain.EntityID  { return cmd.ID }
func (cmd UsePetSkill) inventoryOwner() domain.EntityID  { return cmd.ID }
func (cmd AddPetPoint) inventoryOwner() domain.EntityID  { return cmd.ID }
func (cmd RenamePet) inventoryOwner() domain.EntityID    { return cmd.ID }
func (cmd SetPetShown) inventoryOwner() domain.EntityID  { return cmd.ID }
func (cmd SwapPetSlots) inventoryOwner() domain.EntityID { return cmd.ID }
func (cmd SummonPetAt) inventoryOwner() domain.EntityID  { return cmd.ID }
func (cmd TogglePetAt) inventoryOwner() domain.EntityID  { return cmd.ID }
func (cmd SummonPet) inventoryOwner() domain.EntityID    { return cmd.ID }
func (cmd RecallPet) inventoryOwner() domain.EntityID    { return cmd.ID }
