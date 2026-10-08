package session

import (
	"context"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/online"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/social"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/trade"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

const tradeOfferValidationTimeout = 2 * time.Second
const tradeSettlementTimeout = 5 * time.Second

func (s *Session) onTradeRequest(targetEntity domain.EntityID) {
	s.mu.Lock()
	me, myEntity, myScene, inGame := s.char, s.entity, s.scene, s.stage == StageInGame
	s.mu.Unlock()
	if !inGame || me == nil || myEntity == 0 || s.deps.Online == nil ||
		s.deps.Requests == nil || s.deps.Trades == nil {
		return
	}
	target, ok := s.deps.Online.FindByEntity(targetEntity)
	if !ok || target.Char == domain.CharID(me.ID) || target.Scene != myScene {
		s.sendGMText("交易请求失败：目标不在身边")
		return
	}
	if s.targetBlocks(target.Char, domain.CharID(me.ID)) {
		s.sendGMText("交易请求未送达")
		return
	}
	_, first := s.deps.Requests.AddInteraction(target.Char, social.Request{
		Kind: social.RequestTrade, From: domain.CharID(me.ID), FromID: myEntity,
		FromName: me.Name, Text: me.Name + "请求与你交易",
	})
	if first {
		s.refreshSystemRequestsOf(target.Char)
	}
}

func (s *Session) acceptTradeRequest(me *domain.Character, req social.Request) {
	if me == nil || s.deps.Online == nil || s.deps.Trades == nil {
		return
	}
	mine, ok := s.deps.Online.Find(domain.CharID(me.ID))
	if !ok {
		return
	}
	from, ok := s.deps.Online.Find(req.From)
	if !ok || from.Entity != req.FromID || from.Scene != mine.Scene {
		s.sendGMText("交易请求已失效")
		return
	}
	views, ok := s.deps.Trades.Start(
		trade.Participant{Char: from.Char, Entity: from.Entity, Name: from.Name, Scene: from.Scene},
		trade.Participant{Char: mine.Char, Entity: mine.Entity, Name: mine.Name, Scene: mine.Scene},
	)
	if !ok {
		s.sendGMText("交易请求失败：一方正在交易中")
		return
	}
	s.openTradeViews(views)
	s.deliverTradeViews(views)
}

func (s *Session) onTradeOffer(money int64, refs []protocol.TradeItemRef) {
	s.mu.Lock()
	me, entity, sc, inGame := s.char, s.entity, s.scene, s.stage == StageInGame
	s.mu.Unlock()
	if !inGame || me == nil || entity == 0 || s.deps.Router == nil || s.deps.Trades == nil {
		return
	}
	items := make([]scene.TradeOfferRef, len(refs))
	for i, ref := range refs {
		items[i] = scene.TradeOfferRef{Tab: ref.Tab, Slot: int32(clientBagSlot(ref.Tab, int(ref.Slot))), Count: ref.Count}
	}
	reply := make(chan scene.TradeOfferResult, 1)
	if !s.deps.Router.Post(sc, scene.ValidateTradeOffer{ID: entity, Money: money, Items: items, Reply: reply}) {
		return
	}
	charID := domain.CharID(me.ID)
	go func() {
		select {
		case result := <-reply:
			if !result.OK {
				s.sendGMText("交易报价无效")
				return
			}
			resolved := make([]trade.Item, len(result.Items))
			for i, item := range result.Items {
				resolved[i] = trade.Item{
					Tab: item.Tab, Slot: item.Slot, Stack: item.Stack, Name: item.Name,
					Info: item.Info, Quality: item.Quality, Pet: item.Pet,
				}
			}
			views, ok := s.deps.Trades.Offer(charID, entity, trade.Offer{Money: result.Money, Items: resolved})
			if !ok {
				return
			}
			s.deliverTradeViews(views)
			// Tip 13 describes putting an item into the trade list. Do not fire it for
			// an empty refresh packet or merely opening the window.
			if len(resolved) > 0 || result.Money > 0 {
				s.requestTipFor(charID, 13)
			}
		case <-time.After(tradeOfferValidationTimeout):
			s.log.Warn("交易报价校验超时", "char", charID, "entity", entity)
		}
	}()
}

func (s *Session) onTradeLock(on bool) {
	s.mu.Lock()
	me, entity, inGame := s.char, s.entity, s.stage == StageInGame
	s.mu.Unlock()
	if !inGame || me == nil || s.deps.Trades == nil {
		return
	}
	views, newly, ok := s.deps.Trades.Lock(domain.CharID(me.ID), entity, on)
	if !ok {
		return
	}
	s.deliverTradeViews(views)
	if newly {
		// Tip 14 is bound to the accepted readiness transition, not to a button
		// press that may refer to no active trade.
		s.requestTipFor(domain.CharID(me.ID), 14)
	}
}

func (s *Session) onTradeConfirm() {
	if s.deps.Trades == nil || s.deps.Router == nil || s.deps.WriteBack == nil {
		s.sendGMText("交易结算暂不可用")
		return
	}
	s.mu.Lock()
	me, entity, sc, inGame := s.char, s.entity, s.scene, s.stage == StageInGame
	s.mu.Unlock()
	if !inGame || me == nil || entity == 0 {
		return
	}
	views, bothConfirmed, ok := s.deps.Trades.Confirm(domain.CharID(me.ID), entity)
	if !ok {
		s.sendGMText("交易确认失败：请等待双方锁定报价")
		return
	}
	s.deliverTradeViews(views)
	if !bothConfirmed {
		return
	}
	if views.A.Self.Scene != sc || views.B.Self.Scene != sc {
		s.abortTradeSettlement(domain.CharID(me.ID), entity, "交易确认失败：双方已不在同一场景")
		return
	}

	reply := make(chan scene.TradeSettlementResult, 1)
	cancelReservation := make(chan struct{})
	cmd := scene.ReserveTradeSettlement{
		A: views.A.Self.Entity, B: views.B.Self.Entity,
		AOffer: sceneTradeSettlementOffer(views.A.Mine),
		BOffer: sceneTradeSettlementOffer(views.A.Theirs),
		Reply:  reply, Cancel: cancelReservation,
	}
	if !s.deps.Router.Post(sc, cmd) {
		s.abortTradeSettlement(domain.CharID(me.ID), entity, "交易确认失败：场景不可用")
		return
	}
	var reserved scene.TradeSettlementResult
	select {
	case reserved = <-reply:
	case <-time.After(tradeSettlementTimeout):
		close(cancelReservation)
		s.abortTradeSettlement(domain.CharID(me.ID), entity, "交易确认超时，请重新锁定")
		return
	}
	if reserved.Reason != "" || len(reserved.Snapshots) != 2 {
		reason := reserved.Reason
		if reason == "" {
			reason = "交易状态已经变化"
		}
		s.abortTradeSettlement(domain.CharID(me.ID), entity, "交易确认失败："+reason)
		return
	}
	commit := s.deps.WriteBack.CommitPairMutation(reserved.Snapshots[0], reserved.Snapshots[1],
		func(ctx context.Context, raw store.Store, a, b domain.Snapshot) error {
			return raw.WithTx(ctx, func(tx store.Store) error {
				if err := tx.SaveSnapshot(ctx, a); err != nil {
					return err
				}
				return tx.SaveSnapshot(ctx, b)
			})
		})
	err := <-commit
	s.finalizeTradeSettlement(sc, reserved.Reservation, err == nil)
	if err != nil {
		s.log.Warn("玩家交易双角色事务失败", "char", me.ID, "err", err)
		s.abortTradeSettlement(domain.CharID(me.ID), entity, "交易失败：数据库提交失败，请重新锁定")
		return
	}
	finalViews, finished := s.deps.Trades.FinishSettlement(domain.CharID(me.ID), entity, true)
	if !finished {
		finalViews = views
	}
	s.endTradeViews(finalViews, "交易完成")
}

func sceneTradeSettlementOffer(offer trade.Offer) scene.TradeSettlementOffer {
	out := scene.TradeSettlementOffer{Money: offer.Money, Items: make([]scene.TradeOfferItem, len(offer.Items))}
	for i, item := range offer.Items {
		out.Items[i] = scene.TradeOfferItem{
			Tab: item.Tab, Slot: item.Slot, Stack: item.Stack, Name: item.Name,
			Info: item.Info, Quality: item.Quality, Pet: item.Pet,
		}
	}
	return out
}

func (s *Session) finalizeTradeSettlement(sc domain.SceneID, reservation scene.TradeSettlementReservation,
	commit bool) {
	if reservation.Result != nil {
		reservation.Result <- commit
		select {
		case <-reservation.Done:
		case <-time.After(tradeSettlementTimeout):
			s.log.Warn("交易结算场景收尾超时", "scene", sc)
		}
		return
	}
	done := make(chan struct{})
	if !s.deps.Router.PostLifecycle(sc, scene.FinalizeTradeSettlement{
		Reservation: reservation, Commit: commit, Done: done,
	}) {
		return
	}
	select {
	case <-done:
	case <-time.After(tradeSettlementTimeout):
		s.log.Warn("交易结算场景收尾超时", "scene", sc)
	}
}

func (s *Session) abortTradeSettlement(who domain.CharID, entity domain.EntityID, message string) {
	views, ok := s.deps.Trades.FinishSettlement(who, entity, false)
	if ok {
		s.deliverTradeViews(views)
		s.notifyTradeViews(views, message)
		return
	}
	s.sendGMText(message)
}

func (s *Session) notifyTradeViews(views trade.Views, message string) {
	for _, view := range []trade.View{views.A, views.B} {
		_, control, ok := s.controlFor(view.Self.Char)
		if ok {
			control.Notify(message)
		}
	}
}

func (s *Session) onTradeCancel() {
	s.mu.Lock()
	me, entity := s.char, s.entity
	s.mu.Unlock()
	if me == nil || s.deps.Trades == nil {
		return
	}
	views, ok := s.deps.Trades.Cancel(domain.CharID(me.ID), entity)
	if ok {
		s.endTradeViews(views, "交易已取消")
	}
}

func (s *Session) openTradeViews(views trade.Views) {
	for _, view := range []trade.View{views.A, views.B} {
		_, control, ok := s.controlFor(view.Self.Char)
		if !ok {
			continue
		}
		if delivery, ok := control.(online.TradeControl); ok {
			delivery.OpenTrade(view.Other.Entity, view.Other.Name)
		}
	}
}

func (s *Session) deliverTradeViews(views trade.Views) {
	for _, view := range []trade.View{views.A, views.B} {
		loc, control, ok := s.controlFor(view.Self.Char)
		if !ok || loc.Entity != view.Self.Entity {
			continue
		}
		delivery, ok := control.(online.TradeControl)
		if !ok {
			continue
		}
		delivery.UpdateTrade(online.TradeState{
			MyLocked: view.MyLocked, OtherLocked: view.OtherLocked,
			MyConfirmed: view.MyConfirmed, OtherConfirmed: view.OtherConfirmed,
			MyMoney: view.Mine.Money, OtherMoney: view.Theirs.Money,
			MyItems: onlineTradeItems(view.Mine.Items), OtherItems: onlineTradeItems(view.Theirs.Items),
		})
	}
}

func (s *Session) endTradeViews(views trade.Views, message string) {
	for _, view := range []trade.View{views.A, views.B} {
		loc, control, ok := s.controlFor(view.Self.Char)
		if !ok || loc.Entity != view.Self.Entity {
			continue
		}
		if delivery, ok := control.(online.TradeControl); ok {
			delivery.EndTrade(0, message)
		}
	}
}

func onlineTradeItems(items []trade.Item) []online.TradeItem {
	out := make([]online.TradeItem, len(items))
	for i, item := range items {
		out[i] = online.TradeItem{
			ID: item.Stack.Item, Count: item.Stack.Count, Name: item.Name,
			Info: item.Info, Quality: item.Quality, Pet: item.Pet,
		}
	}
	return out
}
