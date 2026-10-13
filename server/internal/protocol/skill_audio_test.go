package protocol

import (
	"encoding/binary"
	"math/rand"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func assertCombatAudioPacket(t *testing.T, e event.DamageDealt, skillFeedback bool) {
	t.Helper()
	for _, observer := range []domain.EntityID{e.Src, e.Dst, 999} {
		packets := Encode(observer, e)
		if len(packets) != 1 || len(packets[0]) != 19 {
			t.Fatalf("unexpected combat packet layout: %x", packets)
		}
		p := packets[0]
		if binary.LittleEndian.Uint16(p) != SCCombat ||
			domain.EntityID(binary.LittleEndian.Uint32(p[2:])) != e.Src ||
			domain.EntityID(binary.LittleEndian.Uint32(p[6:])) != e.Dst ||
			int32(binary.LittleEndian.Uint32(p[10:])) != e.Amount ||
			int32(binary.LittleEndian.Uint32(p[15:])) != e.DstHP {
			t.Fatalf("audio classification changed combat payload: %x", p)
		}
		var want byte
		if e.Flag.Has(event.DamageCrit) {
			want |= 1
		}
		if e.Flag.Has(event.DamageMiss) {
			want |= 2
		}
		if e.Flag.Has(event.DamageFatal) {
			want |= 4
		}
		if skillFeedback {
			want |= 8
		}
		if p[14] != want {
			t.Fatalf("observer %d skill %d origin %q: flags=%d, want %d", observer, e.SkillID, e.Origin, p[14], want)
		}
	}
}

func TestSkillAudioCombatFlags(t *testing.T) {
	cases := []struct {
		name   string
		skill  int32
		origin domain.DamageOrigin
		quiet  bool
	}{
		{"basic", 0, domain.DamageNormalOrigin, false},
		{"physical-skill", 10201, domain.DamageNormalOrigin, true},
		{"magic-skill", 10501, domain.DamageNormalOrigin, true},
		{"equipment-proc", 10401, domain.DamageProcOrigin, true},
		{"skill-periodic-tick", 0, domain.DamageDOTOrigin, true},
		{"passive-basic-counter", 0, domain.DamageCounterOrigin, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// All combinations preserve crit, miss and fatal independently from the
			// audio bit; opening/magic remain internal damage-calculation flags.
			for flags := 0; flags < 256; flags++ {
				e := event.DamageDealt{Src: 7, Dst: 12, Amount: 42, DstHP: 987,
					SkillID: tc.skill, Origin: tc.origin, Flag: event.DamageFlag(flags)}
				assertCombatAudioPacket(t, e, tc.quiet)
			}
		})
	}
}

func TestSkillAudioActualStrikeAndInterleavedBasic(t *testing.T) {
	src := &entity.Entity{ID: 7, Stats: domain.Stats{MinAtk: 20, MaxAtk: 20, MAtk: 30, Hit: 1000}}
	dst := &entity.Entity{ID: 12, HP: 1000, MaxHP: 1000}
	tracker := &combat.HitTracker{}
	rng := rand.New(rand.NewSource(9))
	// A basic attack enhanced by passives/equipment is still a basic attack.
	// Each proc, area target and multi-hit segment gets its own skill event;
	// no caster-wide quiet window may swallow a genuine attack between them.
	blows := []combat.Blow{
		{GuaranteedHit: true, SkillPct: 150},
		{SkillID: 10401, Magic: true, Origin: domain.DamageProcOrigin, ProcDepth: 1},
		{SkillID: 10201, GuaranteedHit: true},
		{GuaranteedHit: true},
		{SkillID: 10201, GuaranteedHit: true},
		{SkillID: 10501, Magic: true},
		{GuaranteedHit: true},
	}
	for i, blow := range blows {
		before := dst.HP
		e := combat.Strike(src, dst, tracker, blow, rng)
		if e.SkillID != blow.SkillID || e.Amount <= 0 || e.DstHP != before-e.Amount {
			t.Fatalf("strike %d lost authoritative identity/damage: %+v", i, e)
		}
		assertCombatAudioPacket(t, e, blow.SkillID > 0)
	}
	// Periodic status ticks do not use combat.Strike or carry a SkillID.
	assertCombatAudioPacket(t, event.DamageDealt{Src: 7, Dst: 12, Amount: 3,
		DstHP: dst.HP - 3, Origin: domain.DamageDOTOrigin}, true)
}
