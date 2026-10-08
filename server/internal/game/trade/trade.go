// Package trade owns the process-wide, short-lived state of a two-player trade.
//
// A trade spans two client sessions and may outlive a map frame, so it cannot be
// owned by either player's scene actor. Inventory validation still happens in the
// actor that owns the player; this package only records the already-validated
// offer and the two-party handshake.
package trade

import (
	"sync"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

type Participant struct {
	Char   domain.CharID
	Entity domain.EntityID
	Name   string
	Scene  domain.SceneID
}

type Item struct {
	Pet     *domain.PetItemInfo
	Tab     uint8
	Slot    int32
	Stack   domain.Stack
	Name    string
	Info    string
	Quality uint8
}

type Offer struct {
	Money int64
	Items []Item
}

type View struct {
	Self, Other                 Participant
	Mine, Theirs                Offer
	MyLocked, OtherLocked       bool
	MyConfirmed, OtherConfirmed bool
}

type Views struct {
	A, B View
}

type side struct {
	participant Participant
	offer       Offer
	offered     bool
	locked      bool
	confirmed   bool
}

type exchange struct {
	sides    [2]side
	settling bool
}

// Registry is the authoritative in-memory index of active trade handshakes.
// Trades are deliberately not persisted: reconnecting invalidates entity leases
// and must always cancel the old handshake.
type Registry struct {
	mu     sync.Mutex
	byChar map[domain.CharID]*exchange
}

func NewRegistry() *Registry {
	return &Registry{byChar: make(map[domain.CharID]*exchange)}
}

func (r *Registry) Start(a, b Participant) (Views, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a.Char == 0 || b.Char == 0 || a.Char == b.Char || a.Entity == 0 || b.Entity == 0 ||
		a.Scene != b.Scene || r.byChar[a.Char] != nil || r.byChar[b.Char] != nil {
		return Views{}, false
	}
	x := &exchange{sides: [2]side{{participant: a}, {participant: b}}}
	r.byChar[a.Char], r.byChar[b.Char] = x, x
	return viewsOf(x), true
}

// Offer replaces one side's already scene-validated offer. Changing an offer
// clears both sides' lock/confirm bits, matching the client's safety model.
func (r *Registry) Offer(who domain.CharID, entity domain.EntityID, offer Offer) (Views, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	x, idx := r.lookupLocked(who, entity)
	if x == nil || x.settling {
		return Views{}, false
	}
	x.sides[idx].offer = cloneOffer(offer)
	x.sides[idx].offered = true
	for i := range x.sides {
		x.sides[i].locked = false
		x.sides[i].confirmed = false
	}
	return viewsOf(x), true
}

// Lock records a readiness decision. newly is true only for the first false→true
// transition and is used by the NewbieTip trigger.
func (r *Registry) Lock(who domain.CharID, entity domain.EntityID, on bool) (views Views, newly bool, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	x, idx := r.lookupLocked(who, entity)
	if x == nil || x.settling {
		return Views{}, false, false
	}
	old := x.sides[idx].locked
	x.sides[idx].locked = on
	if !on {
		x.sides[idx].confirmed = false
	}
	return viewsOf(x), on && !old, true
}

// Confirm records confirmation only after both parties are locked. Settlement is
// intentionally left to the scene-owned inventory transaction; callers must not
// interpret bothConfirmed as an item transfer by itself.
func (r *Registry) Confirm(who domain.CharID, entity domain.EntityID) (views Views, bothConfirmed bool, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	x, idx := r.lookupLocked(who, entity)
	if x == nil || x.settling || !x.sides[0].locked || !x.sides[1].locked {
		return Views{}, false, false
	}
	x.sides[idx].confirmed = true
	both := x.sides[0].confirmed && x.sides[1].confirmed
	if both {
		x.settling = true
	}
	return viewsOf(x), both, true
}

func (r *Registry) Cancel(who domain.CharID, entity domain.EntityID) (Views, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	x, _ := r.lookupLocked(who, entity)
	if x == nil || x.settling {
		return Views{}, false
	}
	views := viewsOf(x)
	delete(r.byChar, x.sides[0].participant.Char)
	delete(r.byChar, x.sides[1].participant.Char)
	return views, true
}

// FinishSettlement closes a committed trade, or resets both safety decisions after
// a rejected/failed settlement so the same window can be reviewed and retried.
func (r *Registry) FinishSettlement(who domain.CharID, entity domain.EntityID, commit bool) (Views, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	x, _ := r.lookupLocked(who, entity)
	if x == nil || !x.settling {
		return Views{}, false
	}
	if commit {
		views := viewsOf(x)
		delete(r.byChar, x.sides[0].participant.Char)
		delete(r.byChar, x.sides[1].participant.Char)
		return views, true
	}
	x.settling = false
	for i := range x.sides {
		x.sides[i].locked = false
		x.sides[i].confirmed = false
	}
	return viewsOf(x), true
}

func (r *Registry) lookupLocked(who domain.CharID, entity domain.EntityID) (*exchange, int) {
	x := r.byChar[who]
	if x == nil {
		return nil, 0
	}
	if x.sides[1].participant.Char == who && (entity == 0 || x.sides[1].participant.Entity == entity) {
		return x, 1
	}
	if x.sides[0].participant.Char == who && (entity == 0 || x.sides[0].participant.Entity == entity) {
		return x, 0
	}
	return nil, 0
}

func viewsOf(x *exchange) Views {
	return Views{A: viewOf(x, 0), B: viewOf(x, 1)}
}

func viewOf(x *exchange, i int) View {
	j := 1 - i
	return View{
		Self: x.sides[i].participant, Other: x.sides[j].participant,
		Mine: cloneOffer(x.sides[i].offer), Theirs: cloneOffer(x.sides[j].offer),
		MyLocked: x.sides[i].locked, OtherLocked: x.sides[j].locked,
		MyConfirmed: x.sides[i].confirmed, OtherConfirmed: x.sides[j].confirmed,
	}
}

func cloneOffer(in Offer) Offer {
	return Offer{Money: in.Money, Items: append([]Item(nil), in.Items...)}
}
