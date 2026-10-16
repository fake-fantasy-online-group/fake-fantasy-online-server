package protocol

import (
	"bytes"
	"encoding/binary"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestCombatDamageEncodingAndOwnership(t *testing.T) {
	for flag := 0; flag < 256; flag++ {
		for _, skill := range []int32{-1, 0, 1} {
			for _, origin := range []domain.DamageOrigin{"", domain.DamageDOTOrigin} {
				ev := event.DamageDealt{Src: 123, Dst: 456, Amount: -789, DstHP: 2147483647, Flag: event.DamageFlag(flag), SkillID: skill, Origin: origin}
				got := CombatDamage(ev)
				want := Encode(999, ev)
				if len(want) != 1 || !bytes.Equal(got, want[0]) || !bytes.Equal(got, legacyEncodeDamage(ev)) {
					t.Fatal("combat bytes differ")
				}
				flags := wireHitFlag(ev.Flag)
				if skill > 0 || origin == domain.DamageDOTOrigin {
					flags |= wireHitSkill
				}
				if len(got) != 19 || binary.LittleEndian.Uint16(got) != SCCombat || int32(binary.LittleEndian.Uint32(got[2:])) != 123 || int32(binary.LittleEndian.Uint32(got[6:])) != 456 || int32(binary.LittleEndian.Uint32(got[10:])) != -789 || got[14] != flags || int32(binary.LittleEndian.Uint32(got[15:])) != 2147483647 {
					t.Fatalf("incorrect wire fields: %x", got)
				}
				saved := append([]byte(nil), got...)
				other := CombatDamage(ev)
				other[0] ^= 255
				if !bytes.Equal(got, saved) {
					t.Fatal("packet storage shared between calls")
				}
			}
		}
	}
}
