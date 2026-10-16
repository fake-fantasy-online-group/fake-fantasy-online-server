package session

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
)

func (s *Session) consumeHorn(item domain.ItemID, skin uint8) error {
	if s.deps.Router == nil || s.deps.WriteBack == nil {
		return fmt.Errorf("喇叭存档服务不可用")
	}
	s.mu.Lock()
	id, sc := s.entity, s.scene
	s.mu.Unlock()
	operationID, err := newMailOperationID()
	if err != nil {
		return fmt.Errorf("喇叭操作暂不可用")
	}
	cancel := make(chan struct{})
	reply := make(chan scene.MailReserveResult, 1)
	if !s.deps.Router.Post(sc, scene.ReserveHorn{OperationID: operationID, Cancel: cancel, ID: id, Item: item, Skin: skin, Reply: reply}) {
		return fmt.Errorf("喇叭场景不可用")
	}
	r, received := s.awaitMailReservation(sc, id, reply, cancel)
	if !received {
		return fmt.Errorf("喇叭操作超时，请重试")
	}
	if r.Reason != "" {
		return fmt.Errorf("%s", r.Reason)
	}
	// A horn is an absolute inventory replacement, with no external side effect
	// until the caller broadcasts. Unknown COMMIT retains the actor fence and
	// retries the same candidate; only a definite rejection can restore it.
	err = s.commitReservedHorn(r)
	s.finalizeMailReservation(sc, id, r.Reservation, err == nil)
	if err != nil {
		return fmt.Errorf("喇叭扣除失败，请重试")
	}
	return nil
}

func (s *Session) commitReservedHorn(reserved scene.MailReserveResult) error {
	return <-s.deps.WriteBack.CommitSnapshots(reserved.Snapshot, nil)
}
