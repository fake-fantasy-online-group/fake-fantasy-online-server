package protocol

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestStatusIconWireKeepsIdentityResourceAndPermanentSentinelSeparate(t *testing.T) {
	packet := encodeBuffSnapshot(event.BuffSnapshot{Buffs: []event.BuffView{{SkillID: 1901001, Icon: 5017, RemainSec: -2, Name: "白虎卫身", Desc: "source-bound", Beneficial: true, PositionMode: 1}}})
	r := newReader(packet)
	if r.u16() != SCBuffSnapshot || r.u16() != 1 || r.i32() != 1901001 || r.i32() != 5017 || r.i32() != -2 || r.str() != "白虎卫身" || r.str() != "source-bound" || r.u8() != 1 || r.u8() != 1 || r.u8() != 0 || r.u8() != 0 || r.u8() != 0 || r.bad || r.p != len(packet) {
		t.Fatalf("buff snapshot wire drift: %x", packet)
	}
	packet = encodeEntityStatus(event.EntityStatusSnapshot{Who: 5, Icons: []event.StatusIconView{{Status: 1901001, Icon: 5017, Beneficial: true, RemainMS: -1}}})
	r = newReader(packet)
	if r.u16() != SCEntityStatus || r.i32() != 5 || r.u8() != 1 || r.i32() != 1901001 || r.i32() != 5017 || r.u8() != 1 || r.i32() != -1 || r.bad || r.p != len(packet) {
		t.Fatalf("entity status wire drift: %x", packet)
	}
}
