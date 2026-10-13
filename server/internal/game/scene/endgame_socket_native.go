package scene

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// The legacy furnace packet identifies slots, not item instances. Bind it to the
// last equipment preview, and recheck the same instances after receipt I/O. No
// native opcode or positional field is changed.
type nativeSocketSelection struct {
	ref      socketTargetRef
	target   domain.Stack
	runes    map[int]domain.Stack
	revision string
	command  InlayItem
	request  domain.EndgameRequest
}

func (s *Scene) rememberNativeSocketSelection(p *entity.Entity, ref socketTargetRef) {
	x := s.endgamePlayer(p)
	if x.receipt != nil || x.commit != nil {
		return
	}
	x.nativeSocket = nil
	st, _, ok := s.socketTarget(p, ref)
	if !ok || st.UID <= 0 || p.Player.Bag == nil {
		return
	}
	q := &nativeSocketSelection{ref: ref, target: st, runes: map[int]domain.Stack{}}
	p.Player.Bag.Each(func(slot int, rune domain.Stack) {
		if _, astral := domain.AstralRune(rune.Item, s.endgameCatalog); astral {
			q.runes[slot] = rune
		}
	})
	if x.loaded {
		q.revision = strconv.FormatInt(x.state.Revision, 10)
	}
	x.nativeSocket = q
	// Native-only sessions need the same durable state as negotiated clients.
	// Loading is read-only; an early drag is rejected until it has completed.
	if !x.loaded && s.endgameBackend != nil && x.load == nil && x.receipt == nil && x.commit == nil {
		s.onEndgame(EndgameRequest{ID: p.ID, Request: domain.EndgameRequest{Op: "snapshot"}})
	}
}

func (s *Scene) nativeSocketSelectionCurrent(p *entity.Entity, q *nativeSocketSelection, cmd InlayItem) bool {
	if q == nil || q.ref != (socketTargetRef{kind: cmd.TargetKind, equipSlot: cmd.EquipSlot, tab: cmd.Tab, bagSlot: cmd.BagSlot}) ||
		cmd.RuneBagSlot < 0 || int(cmd.RuneBagSlot) >= p.Player.Bag.Cap() {
		return false
	}
	target, _, ok := s.socketTarget(p, q.ref)
	rune, observed := q.runes[int(cmd.RuneBagSlot)]
	return ok && observed && target == q.target && p.Player.Bag.At(int(cmd.RuneBagSlot)) == rune
}

// A delayed astral drag must not fall into random gem insertion when another
// card has since replaced its source slot.
func (s *Scene) isNativeAstralSlot(p *entity.Entity, slot int32, current domain.ItemID) bool {
	if _, ok := domain.AstralRune(current, s.endgameCatalog); ok {
		return true
	}
	q := s.endgamePlayer(p).nativeSocket
	if q == nil || slot < 0 {
		return false
	}
	_, ok := q.runes[int(slot)]
	return ok
}

func (s *Scene) onNativeAstralInlay(p *entity.Entity, cmd InlayItem) {
	x := s.endgamePlayer(p)
	q := x.nativeSocket
	deny := func(message string) { s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: message}) }
	if s.endgameBackend == nil {
		deny("星篆结算暂不可用，请稍后重试。")
		return
	}
	if !x.loaded || x.load != nil || x.receipt != nil || x.commit != nil || endgameEconomicBusy(p) {
		deny("其他物品事务尚未结束，请稍候再镶嵌。")
		return
	}
	if !s.nativeSocketSelectionCurrent(p, q, cmd) {
		deny("装备或星篆已变化，请重新将装备放入镶嵌炉。")
		return
	}
	rune := p.Player.Bag.At(int(cmd.RuneBagSlot))
	def, ok := s.itemDef(rune.Item)
	if !ok || !def.InventoryTabKnown || def.InventoryTab != cmd.RuneTab {
		deny("请选择背包中的星篆。")
		return
	}
	if q.revision == "" {
		q.revision = strconv.FormatInt(x.state.Revision, 10)
	}
	// A failed/uncertain attempt retains its exact request and receipt identity.
	// A new drag receives an identity derived from immutable item UIDs and the
	// preview revision, so repeated native packets cannot consume twice.
	if q.request.RequestID == "" || q.command != cmd {
		r := domain.EndgameRequest{Op: "socket.insert", ExpectedRevision: q.revision,
			TargetUID: strconv.FormatInt(q.target.UID, 10), RuneUID: strconv.FormatInt(rune.UID, 10), Socket: int32(cmd.Hole)}
		h := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", p.Player.Char.ID, endgameDigest(r))))
		r.RequestID = "native-inlay-" + hex.EncodeToString(h[:24])
		q.command, q.request = cmd, r
	}
	s.onEndgame(EndgameRequest{ID: p.ID, Request: q.request})
}

// Native clients receive their usual notice and socket packet. Enhanced clients
// also receive the normal receipt/snapshot, but no separate workbench is opened.
func (s *Scene) replyNativeAstralInlay(p *entity.Entity, r domain.EndgameResponse) {
	x := s.endgamePlayer(p)
	q := x.nativeSocket
	if q == nil || q.request.RequestID == "" || q.request.RequestID != r.RequestID {
		return
	}
	message := r.Message
	if r.Error == "storage" {
		message = "星篆结算尚未确认，请将同一星篆拖回原孔重试。"
	}
	if message != "" {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: message})
	}
	st, _, ok := s.socketTarget(p, q.ref)
	if ok && st.UID == q.target.UID {
		s.emitTo(p.ID, s.socketInfo(p, q.ref))
		if r.OK {
			s.rememberNativeSocketSelection(p, q.ref)
		}
	} else {
		s.emitTo(p.ID, event.SocketInfo{Who: p.ID})
	}
}
