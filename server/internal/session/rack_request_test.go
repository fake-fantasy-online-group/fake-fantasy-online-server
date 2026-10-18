package session

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
	"testing"
	"time"
)

func TestRackWaitTimeoutCancelsUnpublishedRequest(t *testing.T) {
	r := &scene.RackRequest{}
	timeout := make(chan time.Time, 1)
	timeout <- time.Now()
	_, ok := waitRackReservation(make(chan scene.RackReserveResult, 1), r, timeout)
	if ok || r.Cancel() {
		t.Fatal("timeout did not take cancellation ownership")
	}
}
func TestRackWaitReceivesResult(t *testing.T) {
	ch := make(chan scene.RackReserveResult, 1)
	ch <- scene.RackReserveResult{Reason: "test rejection"}
	r, ok := waitRackReservation(ch, &scene.RackRequest{}, make(chan time.Time))
	if !ok || r.Reason != "test rejection" {
		t.Fatal("reply not preserved")
	}
}

func TestRackWaitTimeoutDrainsClaimedReply(t *testing.T) {
	actor := scene.New(scene.Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: domain.SceneID{MapID: 7}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	go func() { actor.Run(ctx); close(stopped) }()
	defer func() { cancel(); <-stopped }()
	request := &scene.RackRequest{}
	reply := make(chan scene.RackReserveResult, 1)
	if !actor.Post(scene.ReserveRackPurchase{ID: 99, Reply: reply, Request: request}) {
		t.Fatal("post rejected")
	}
	var prepared scene.RackReserveResult
	select {
	case prepared = <-reply:
	case <-time.After(time.Second):
		t.Fatal("actor did not publish")
	}
	// The actor has claimed publication. Delay transport to exercise the timeout
	// branch deterministically, rather than select randomly on two ready channels.
	timeout := make(chan time.Time)
	close(timeout)
	result := make(chan bool, 1)
	go func() {
		value, ok := waitRackReservation(reply, request, timeout)
		result <- ok && value.Reason == prepared.Reason
	}()
	select {
	case <-result:
		t.Fatal("published result was abandoned")
	case <-time.After(time.Millisecond):
	}
	reply <- prepared
	select {
	case ok := <-result:
		if !ok {
			t.Fatal("published result was not drained")
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not complete")
	}
}
