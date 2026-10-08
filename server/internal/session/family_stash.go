package session

import (
	"context"
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

func (s *Session) familyStashReader() (store.FamilyStashReader, bool) {
	reader, ok := s.deps.Store.(store.FamilyStashReader)
	return reader, ok
}

func (s *Session) onFamilyStash(req protocol.Request) {
	switch req.Mode {
	case 0:
		s.refreshFamilyStash()
	case 1:
		s.depositFamilyStash(req.FromTab, req.FromSlot)
	case 2:
		s.withdrawFamilyStash(req.Index)
	}
}

func (s *Session) refreshFamilyStash() {
	character, inGame := s.currentCharacter()
	reader, ok := s.familyStashReader()
	if !inGame || !ok {
		return
	}
	stash, err := reader.LoadFamilyStash(context.Background(), character.ID)
	if err != nil {
		s.log.Error("读取家族仓库失败", "char", character.ID, "err", err)
		return
	}
	if stash == nil {
		s.sendGMText("你尚未加入家族")
		return
	}
	s.postToScene(scene.ShowFamilyStash{ID: s.entityID(), Stash: *stash})
}

func (s *Session) depositFamilyStash(tab uint8, slot int32) {
	character, inGame := s.currentCharacter()
	if !inGame || s.deps.WriteBack == nil {
		return
	}
	reserved, ok := s.reserveFamily(scene.ReserveFamilyStashDeposit{ID: s.entityID(),
		BagTab: tab, BagSlot: slot})
	if !ok {
		return
	}
	commit := s.deps.WriteBack.CommitMutation(reserved.Snapshot,
		func(ctx context.Context, raw store.Store, snap domain.Snapshot) error {
			return raw.WithTx(ctx, func(tx store.Store) error {
				stash, ok := tx.(store.FamilyStashWriter)
				if !ok {
					return fmt.Errorf("store: 家族仓库存入能力不可用")
				}
				if err := stash.DepositFamilyStash(ctx, snap.Char.ID, reserved.Stack, character.Name); err != nil {
					return err
				}
				return tx.SaveSnapshot(ctx, snap)
			})
		})
	err := <-commit
	s.finalizeFamily(reserved.Reservation, err == nil)
	if err != nil {
		s.sendGMText("存入家族仓库失败：" + err.Error())
	} else {
		s.sendGMText("物品已存入家族仓库")
	}
	s.refreshFamilyStash()
}

func (s *Session) withdrawFamilyStash(ordinal int32) {
	character, inGame := s.currentCharacter()
	reader, ok := s.familyStashReader()
	if !inGame || !ok || s.deps.WriteBack == nil {
		return
	}
	stash, err := reader.LoadFamilyStash(context.Background(), character.ID)
	if err != nil || stash == nil || !stash.CanTake || ordinal < 0 || int(ordinal) >= len(stash.Entries) {
		s.sendGMText("没有权限或仓库物品已变化")
		return
	}
	entry := stash.Entries[ordinal]
	reserved, ok := s.reserveFamily(scene.ReserveFamilyStashWithdraw{ID: s.entityID(), Stack: entry.Stack})
	if !ok {
		return
	}
	commit := s.deps.WriteBack.CommitMutation(reserved.Snapshot,
		func(ctx context.Context, raw store.Store, snap domain.Snapshot) error {
			return raw.WithTx(ctx, func(tx store.Store) error {
				writer, ok := tx.(store.FamilyStashWriter)
				if !ok {
					return fmt.Errorf("store: 家族仓库取出能力不可用")
				}
				if err := writer.WithdrawFamilyStash(ctx, snap.Char.ID, entry.Index, entry.Stack); err != nil {
					return err
				}
				return tx.SaveSnapshot(ctx, snap)
			})
		})
	err = <-commit
	s.finalizeFamily(reserved.Reservation, err == nil)
	if err != nil {
		s.sendGMText("取出家族仓库物品失败：" + err.Error())
	} else {
		s.sendGMText("已从家族仓库取出物品")
	}
	s.refreshFamilyStash()
}

func (s *Session) onFamilyStashPermission(position domain.FamilyPositionID) {
	character, inGame := s.currentCharacter()
	if !inGame || position < domain.FamilyLeader || position > domain.FamilyOrdinary {
		return
	}
	err := s.deps.Store.WithTx(context.Background(), func(tx store.Store) error {
		writer, ok := tx.(store.FamilyStashWriter)
		if !ok {
			return fmt.Errorf("store: 家族仓库权限能力不可用")
		}
		return writer.SetFamilyStashTakePosition(context.Background(), character.ID, position)
	})
	if err != nil {
		s.sendGMText("设置家族仓库权限失败：" + err.Error())
		return
	}
	s.refreshFamilyStash()
}
