package session

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

func (s *Session) consumeHorn(item domain.ItemID, skin uint8) error {
	if s.deps.Router == nil || s.deps.WriteBack == nil {
		return fmt.Errorf("喇叭存档服务不可用")
	}
	s.mu.Lock()
	id, sc := s.entity, s.scene
	s.mu.Unlock()
	reply := make(chan scene.MailReserveResult, 1)
	if !s.deps.Router.Post(sc, scene.ReserveHorn{ID: id, Item: item, Skin: skin, Reply: reply}) {
		return fmt.Errorf("喇叭场景不可用")
	}
	r := <-reply
	if r.Reason != "" {
		return fmt.Errorf("%s", r.Reason)
	}
	err := <-s.deps.WriteBack.CommitMutation(r.Snapshot, func(ctx context.Context, st store.Store, snap domain.Snapshot) error {
		return st.SaveSnapshot(ctx, snap)
	})
	s.finalizeMailReservation(sc, id, r.Reservation, err == nil)
	if err != nil {
		return fmt.Errorf("喇叭扣除失败，请重试")
	}
	return nil
}
