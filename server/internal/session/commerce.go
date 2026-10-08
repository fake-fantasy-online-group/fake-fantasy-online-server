package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

func (s *Session) onCommerceRequest(req protocol.Request) {
	switch req.Kind {
	case protocol.ReqOpenRack:
		s.postToScene(scene.OpenRack{ID: s.entityID()})
	case protocol.ReqBuyRack:
		s.onRackBuy(domain.ItemID(req.ID32), req.U8, req.Count)
	case protocol.ReqFittingCatalog:
		s.postToScene(scene.OpenFitting{ID: s.entityID(), Kind: req.U8, Part: req.S1,
			Search: req.S2, Page: req.X, PerPage: req.Count, Paged: req.Mode == 1})
	case protocol.ReqFittingDesc:
		ids := make([]domain.ItemID, len(req.Vals))
		for i, id := range req.Vals {
			ids[i] = domain.ItemID(id)
		}
		s.postToScene(scene.FittingDetails{ID: s.entityID(), Items: ids})
	case protocol.ReqRackRefundList:
		s.refreshRackRefunds()
	case protocol.ReqRackRefund:
		s.onRackRefund(req)
	case protocol.ReqDeposit:
		s.onDeposit(req.S1, req.X)
	}
}

func (s *Session) rackRefundReader() (store.RackRefundReader, bool) {
	reader, ok := s.deps.Store.(store.RackRefundReader)
	return reader, ok
}

func (s *Session) reserveRack(raw scene.Command) (scene.RackReserveResult, bool) {
	s.mu.Lock()
	sceneID, inGame := s.scene, s.stage == StageInGame
	s.mu.Unlock()
	if !inGame {
		return scene.RackReserveResult{}, false
	}
	reply := make(chan scene.RackReserveResult, 1)
	switch cmd := raw.(type) {
	case scene.ReserveRackPurchase:
		cmd.Reply = reply
		raw = cmd
	case scene.ReserveRackRefund:
		cmd.Reply = reply
		raw = cmd
	default:
		return scene.RackReserveResult{}, false
	}
	if !s.deps.Router.Post(sceneID, raw) {
		s.sendGMText("货架操作失败：场景不可用")
		return scene.RackReserveResult{}, false
	}
	select {
	case result := <-reply:
		if result.Reason != "" || result.Snapshot.Char == nil {
			s.sendGMText("货架操作失败：" + fallbackReason(result.Reason))
			return result, false
		}
		return result, true
	case <-time.After(familyCommitTimeout):
		s.sendGMText("货架操作超时，请重试")
		return scene.RackReserveResult{}, false
	}
}

func (s *Session) finalizeRack(reservation scene.RackReservation, commit, refresh bool, message string) {
	s.mu.Lock()
	sceneID := s.scene
	s.mu.Unlock()
	done := make(chan struct{})
	if !s.deps.Router.Post(sceneID, scene.FinalizeRackReservation{Reservation: reservation,
		Commit: commit, RefreshCatalog: refresh, Message: message, Done: done}) {
		return
	}
	select {
	case <-done:
	case <-time.After(familyCommitTimeout):
	}
}

func (s *Session) onRackBuy(item domain.ItemID, source uint8, count int32) {
	if s.deps.WriteBack == nil {
		s.sendGMText("货架购买暂不可用：持久化服务未就绪")
		return
	}
	reserved, ok := s.reserveRack(scene.ReserveRackPurchase{ID: s.entityID(), Item: item,
		Source: source, Count: count})
	if !ok {
		return
	}
	purchasedAt := time.Now()
	commit := s.deps.WriteBack.CommitMutation(reserved.Snapshot,
		func(ctx context.Context, raw store.Store, snap domain.Snapshot) error {
			return raw.WithTx(ctx, func(tx store.Store) error {
				ledger, ok := tx.(store.RackLedgerWriter)
				if !ok {
					return fmt.Errorf("store: 货架购买流水能力不可用")
				}
				if err := ledger.RecordRackPurchase(ctx, snap.Char.ID, reserved.Item,
					reserved.UnitPrice, reserved.Bundle, reserved.Shares, purchasedAt); err != nil {
					return err
				}
				return tx.SaveSnapshot(ctx, snap)
			})
		})
	err := <-commit
	message := fmt.Sprintf("购买成功：%s×%d，消耗%d彩玉",
		reserved.Name, reserved.Bundle*reserved.Shares, reserved.Total)
	if err != nil {
		message = "购买失败，物品和彩玉已恢复"
		s.log.Error("提交货架购买失败", "item", reserved.Item, "err", err)
	}
	s.finalizeRack(reserved.Reservation, err == nil, true, message)
}

func (s *Session) refreshRackRefunds() {
	character, inGame := s.currentCharacter()
	reader, ok := s.rackRefundReader()
	if !inGame || !ok {
		return
	}
	rule, err := reader.LoadRackRefundRule(context.Background())
	if err != nil {
		s.log.Error("读取货架退货规则失败", "err", err)
		return
	}
	offers, err := reader.LoadRackRefundOffers(context.Background(), character.ID, rule)
	if err != nil {
		s.log.Error("读取货架可退流水失败", "char", character.ID, "err", err)
		return
	}
	s.postToScene(scene.ShowRackRefunds{ID: s.entityID(), Rule: rule, Offers: offers})
}

func (s *Session) onRackRefund(req protocol.Request) {
	if req.Day != 0 || req.RefundShares != 1 || s.deps.WriteBack == nil {
		s.sendGMText("退货请求格式无效")
		return
	}
	character, inGame := s.currentCharacter()
	reader, ok := s.rackRefundReader()
	if !inGame || !ok {
		return
	}
	rule, err := reader.LoadRackRefundRule(context.Background())
	if err != nil {
		s.sendGMText("退货规则暂不可用")
		return
	}
	offers, err := reader.LoadRackRefundOffers(context.Background(), character.ID, rule)
	if err != nil {
		s.sendGMText("退货流水暂不可用")
		return
	}
	var offer domain.RackRefundOffer
	for _, candidate := range offers {
		if candidate.Item == domain.ItemID(req.ID32) && candidate.UnitPrice == req.UnitPrice &&
			candidate.Bundle == req.Bundle {
			offer = candidate
			break
		}
	}
	if offer.Item == 0 || offer.RemainingShares < req.RefundShares {
		s.sendGMText("没有匹配的可退购买记录")
		return
	}
	credit := int64(offer.UnitPrice) * int64(rule.Percent) / 100
	if credit <= 0 {
		s.sendGMText("退货金额无效")
		return
	}
	reserved, ok := s.reserveRack(scene.ReserveRackRefund{ID: s.entityID(), Offer: offer,
		Shares: req.RefundShares, Credit: credit})
	if !ok {
		return
	}
	purchasedAfter := time.Now().Add(-time.Duration(rule.WindowDay) * 24 * time.Hour)
	commit := s.deps.WriteBack.CommitMutation(reserved.Snapshot,
		func(ctx context.Context, raw store.Store, snap domain.Snapshot) error {
			return raw.WithTx(ctx, func(tx store.Store) error {
				ledger, ok := tx.(store.RackLedgerWriter)
				if !ok {
					return fmt.Errorf("store: 货架退货流水能力不可用")
				}
				if err := ledger.ConsumeRackRefund(ctx, snap.Char.ID, reserved.Item,
					reserved.UnitPrice, reserved.Bundle, reserved.Shares, purchasedAfter); err != nil {
					return err
				}
				return tx.SaveSnapshot(ctx, snap)
			})
		})
	err = <-commit
	message := fmt.Sprintf("退货成功：%s×%d，返还%d彩玉",
		reserved.Name, reserved.Bundle*reserved.Shares, reserved.Total)
	if err != nil {
		message = "退货失败，物品和彩玉已恢复"
		s.log.Error("提交货架退货失败", "item", reserved.Item, "err", err)
	}
	s.finalizeRack(reserved.Reservation, err == nil, true, message)
	s.refreshRackRefunds()
}

func (s *Session) onDeposit(chain string, amount int32) {
	chain = strings.TrimSpace(chain)
	if chain == "" {
		chain = "local"
	}
	if s.deps.WriteBack == nil || s.deps.Router == nil {
		s.sink.Send(protocol.DepositResult(false, chain, "", amount, 0, "本地充值暂不可用", ""))
		return
	}
	reserved, ok := s.reserveFamily(scene.ReserveDeposit{ID: s.entityID(), Amount: amount})
	if !ok {
		s.sink.Send(protocol.DepositResult(false, chain, "", amount, 0, "充值申请未通过", ""))
		return
	}
	commit := s.deps.WriteBack.CommitMutation(reserved.Snapshot,
		func(ctx context.Context, raw store.Store, snap domain.Snapshot) error {
			return raw.WithTx(ctx, func(tx store.Store) error {
				commerce, ok := tx.(store.CommerceStore)
				if !ok {
					return fmt.Errorf("store: 充值记录能力不可用")
				}
				if _, err := commerce.RecordDeposit(ctx, snap.Char.ID, chain, amount, reserved.Credited); err != nil {
					return err
				}
				return tx.SaveSnapshot(ctx, snap)
			})
		})
	err := <-commit
	s.finalizeFamily(reserved.Reservation, err == nil)
	if err != nil {
		s.sink.Send(protocol.DepositResult(false, chain, "", amount, reserved.Reservation.BeforeCaiyu,
			"充值入账失败，请重试", ""))
		return
	}
	s.sink.Send(protocol.DepositResult(true, chain, "local", amount, reserved.Caiyu,
		fmt.Sprintf("本地服充值已到账：%d彩玉", reserved.Credited), ""))
}
