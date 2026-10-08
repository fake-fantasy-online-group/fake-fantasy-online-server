package session

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

const stallCommitTimeout = 5 * time.Second

func (s *Session) onStallCreate(req protocol.Request) {
	name := strings.TrimSpace(req.S1)
	if req.U8 > 1 || name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 24 {
		s.sendGMText("摆摊失败：摊位名称为空或过长")
		return
	}
	s.runSingleStallMutation(scene.CreateStall{
		ID: s.entityID(), Type: domain.StallType(req.U8), Name: name,
	})
}

func (s *Session) onStallAdd(req protocol.Request) {
	s.runSingleStallMutation(scene.ReserveStallAdd{ID: s.entityID(), Tab: req.Tab, Slot: int32(clientBagSlot(req.Tab, int(req.Slot))),
		Item: domain.ItemID(req.ID32), Count: req.Count, Price: req.Price})
}

func (s *Session) onStallDel(index int32) {
	s.runSingleStallMutation(scene.ReserveStallDel{ID: s.entityID(), Index: index})
}

func (s *Session) onStallEnd() {
	s.runSingleStallMutation(scene.ReserveStallEnd{ID: s.entityID()})
}

func (s *Session) onStallDeal(req protocol.Request) {
	if s.deps.WriteBack == nil || s.deps.Router == nil {
		s.sendGMText("摊位成交暂不可用")
		return
	}
	s.mu.Lock()
	sc, actor, inGame := s.scene, s.entity, s.stage == StageInGame
	s.mu.Unlock()
	if !inGame || actor == 0 {
		return
	}
	reply := make(chan scene.StallReserveResult, 1)
	cmd := scene.ReserveStallDeal{ID: actor, Owner: domain.EntityID(req.TargetID),
		Index: req.Index, Count: req.Count, Tab: req.Tab, Slot: int32(clientBagSlot(req.Tab, int(req.Slot))),
		ExpectedItem: domain.ItemID(req.ID32), ExpectedPrice: req.Price, Reply: reply}
	if !s.deps.Router.Post(sc, cmd) {
		s.sendGMText("摊位成交失败：场景不可用")
		return
	}
	reserved, ok := waitStallReservation(reply)
	if !ok {
		s.sendGMText("摊位成交超时，请重试")
		return
	}
	if reserved.Reason != "" || len(reserved.Snapshots) != 2 {
		s.sendGMText("摊位成交失败：" + fallbackReason(reserved.Reason))
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
	s.finalizeStall(sc, reserved, err == nil)
	if err != nil {
		s.log.Warn("摊位成交事务失败", "err", err)
		s.sendGMText("摊位成交失败：数据库提交失败，请重试")
	}
}

type stallReserveCommand interface {
	scene.Command
}

func (s *Session) runSingleStallMutation(raw stallReserveCommand) {
	if s.deps.WriteBack == nil || s.deps.Router == nil {
		s.sendGMText("摆摊功能暂不可用")
		return
	}
	s.mu.Lock()
	sc, inGame := s.scene, s.stage == StageInGame
	s.mu.Unlock()
	if !inGame {
		return
	}
	reply := make(chan scene.StallReserveResult, 1)
	switch cmd := raw.(type) {
	case scene.CreateStall:
		cmd.Reply = reply
		raw = cmd
	case scene.ReserveStallAdd:
		cmd.Reply = reply
		raw = cmd
	case scene.ReserveStallDel:
		cmd.Reply = reply
		raw = cmd
	case scene.ReserveStallEnd:
		cmd.Reply = reply
		raw = cmd
	default:
		return
	}
	if !s.deps.Router.Post(sc, raw) {
		s.sendGMText("摆摊操作失败：场景不可用")
		return
	}
	reserved, ok := waitStallReservation(reply)
	if !ok {
		s.sendGMText("摆摊操作超时，请重试")
		return
	}
	if reserved.Reason != "" || len(reserved.Snapshots) != 1 {
		s.sendGMText("摆摊操作失败：" + fallbackReason(reserved.Reason))
		return
	}
	commit := s.deps.WriteBack.CommitMutation(reserved.Snapshots[0],
		func(ctx context.Context, raw store.Store, snap domain.Snapshot) error {
			return raw.WithTx(ctx, func(tx store.Store) error { return tx.SaveSnapshot(ctx, snap) })
		})
	err := <-commit
	s.finalizeStall(sc, reserved, err == nil)
	if err != nil {
		s.log.Warn("摆摊单角色事务失败", "err", err)
		s.sendGMText("摆摊操作失败：数据库提交失败，请重试")
	}
}

func waitStallReservation(reply <-chan scene.StallReserveResult) (scene.StallReserveResult, bool) {
	select {
	case result := <-reply:
		return result, true
	case <-time.After(stallCommitTimeout):
		return scene.StallReserveResult{}, false
	}
}

func (s *Session) finalizeStall(sc domain.SceneID, reserved scene.StallReserveResult, commit bool) {
	done := make(chan struct{})
	if !s.deps.Router.Post(sc, scene.FinalizeStallReservation{Reservation: reserved.Reservation,
		Commit: commit, ActorMessage: reserved.ActorMessage, OwnerMessage: reserved.OwnerMessage,
		Done: done}) {
		return
	}
	select {
	case <-done:
	case <-time.After(stallCommitTimeout):
	}
}

func fallbackReason(reason string) string {
	if reason == "" {
		return "状态已经变化"
	}
	return reason
}

// recoverStaleStall 在角色取得 gate 后、进入场景前归还上次进程崩溃遗留的托管物权。
// 正常退出不会留下这些行；恢复失败时拒绝登录，不能顶着缺货背包继续覆盖存档。
func (s *Session) recoverStaleStall(ctx context.Context, ch *domain.Character, bag *domain.Bag,
	worn *domain.EquipSet, warehouse *domain.Warehouse, wardrobe *domain.Wardrobe,
	stall *domain.Stall) (*domain.Bag, error) {
	if stall == nil {
		return bag, nil
	}
	trial := bag.Clone()
	if trial == nil {
		return nil, fmt.Errorf("空背包")
	}
	if stall.Type == domain.StallSell {
		for _, item := range stall.Items {
			def, ok := s.deps.Items[item.Stack.Item]
			if !ok || trial.AddStack(def, item.Stack) != 0 {
				return nil, fmt.Errorf("托管物品 %d 无法归还背包", item.Stack.Item)
			}
		}
	}
	snap := domain.Snapshot{Char: ch, Bag: trial, Worn: worn, Warehouse: warehouse,
		Wardrobe: wardrobe, Stall: nil}
	if err := s.deps.Store.SaveSnapshot(ctx, snap); err != nil {
		return nil, fmt.Errorf("保存摊位恢复结果: %w", err)
	}
	s.log.Info("已恢复崩溃前摆摊托管", "char", ch.Name, "type", stall.Type,
		"items", len(stall.Items))
	return trial, nil
}
