package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestPositionDirtySnapshotAndReservationBarriers(t *testing.T) {
	saver := &durabilitySnapshotSaver{}
	s := newTestScene(t, saver)
	join(t, s, 1, 101, "walker", 10, 20)
	p := s.players[1]
	s.saveDirty() // Initial admission is deliberately a full snapshot.
	saver.snapshots = nil
	s.move(MoveTo{ID: p.ID, To: domain.Pos{MapID: 7, X: 45, Y: 20}}, false, true)
	s.saveDirty()
	if len(saver.snapshots) != 1 {
		t.Fatal("movement not queued")
	}
	got := saver.snapshots[0]
	if got.Parts != domain.SnapshotPosition || got.Char.Pos.X != 45 || got.Bag != nil || got.Worn != nil || got.Char.Level != 0 {
		t.Fatalf("movement cloned full aggregate: %+v", got)
	}
	for _, mail := range []bool{false, true} {
		saver.snapshots = nil
		p.Player.TradeBusy, p.Player.MailBusy = !mail, mail
		p.Player.MarkPositionDirty()
		s.saveDirty()
		if len(saver.snapshots) != 0 || !p.Player.Dirty() {
			t.Fatal("reservation leaked or consumed dirty state")
		}
		p.Player.TradeBusy, p.Player.MailBusy = false, false
		p.Player.MarkDirty()
		s.saveDirty()
		if len(saver.snapshots) != 1 || saver.snapshots[0].Parts != 0 || saver.snapshots[0].Char.Level == 0 {
			t.Fatal("full mutation not persisted after reservation")
		}
	}
}
