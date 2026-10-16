package protocol

import (
	"bytes"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func legacyEncodeMove(e event.EntityMoved) []byte {
	speed := e.Speed
	if speed <= 0 {
		speed = defaultMoveSpeed
	}
	if e.Snap {
		speed = -1 // 客户端立即校正坐标、清空路径，并保留当前 curDir/atkFrame
	} else if e.Stop {
		// 0 不改写客户端已存的移动速度；这条单点路径在目标处自然结束。
		// 若改发默认正速度，客户端会把“停止确认”重新解释成一次移动起步。
		speed = 0
	}
	return NewW(SCEntityMove).
		I32(int32(e.ID)).
		I32(speed).
		U8(1). // 路径点数
		I32(int32(e.To.X)).
		I32(int32(e.To.Y)).
		Bytes()
}

func legacyEncodeDespawn(e event.EntityDespawned) []byte {
	switch e.Kind {
	case domain.KindDrop:
		return NewW(SCGroundRemove).I32(int32(e.ID)).Bytes()
	case domain.KindNPC:
		return NewW(SCNPCRemove).I32(int32(e.ID)).Bytes()
	}
	return NewW(SCEntityDespawn).I32(int32(e.ID)).U8(0).Bytes()
}

func legacyEncodeDamage(e event.DamageDealt) []byte {
	flags := wireHitFlag(e.Flag)
	// Skill release/fire audio is delivered separately by 0x800e. Its damage
	// must not also play the equipped weapon's ordinary hit sound/projectile.
	// Status ticks have no SkillID, but are explicitly marked as periodic.
	// Classify each result, not its caster: a basic hit followed by an equipment
	// proc (or interleaved with a multi-hit skill) retains its own weapon sound.
	if e.SkillID > 0 || e.Origin == domain.DamageDOTOrigin {
		flags |= wireHitSkill
	}
	return NewW(SCCombat).
		I32(int32(e.Src)).
		I32(int32(e.Dst)).
		I32(e.Amount).
		U8(flags).
		I32(e.DstHP).
		Bytes()
}

var fixedPacketBytes []byte

func TestFixedPacketParity(t *testing.T) {
	for _, speed := range []int32{-1, 0, 1, 100, 2147483647} {
		for _, snap := range []bool{false, true} {
			for _, stop := range []bool{false, true} {
				e := event.EntityMoved{ID: 123, Speed: speed, Snap: snap, Stop: stop, To: domain.Pos{X: -99, Y: 12345}}
				if !bytes.Equal(encodeMove(e), legacyEncodeMove(e)) {
					t.Fatal("move mismatch")
				}
			}
		}
	}
	for _, kind := range []domain.EntityKind{domain.KindPlayer, domain.KindMonster, domain.KindPet, domain.KindDrop, domain.KindNPC} {
		e := event.EntityDespawned{ID: 123, Kind: kind}
		if !bytes.Equal(encodeDespawn(e), legacyEncodeDespawn(e)) {
			t.Fatal("despawn mismatch")
		}
	}
	for f := 0; f < 256; f++ {
		for _, skill := range []int32{0, 1} {
			e := event.DamageDealt{Src: 1, Dst: 2, Amount: 123, DstHP: 456, Flag: event.DamageFlag(f), SkillID: skill}
			if !bytes.Equal(encodeDamage(e), legacyEncodeDamage(e)) {
				t.Fatal("damage mismatch")
			}
			e.Origin = domain.DamageDOTOrigin
			if !bytes.Equal(encodeDamage(e), legacyEncodeDamage(e)) {
				t.Fatal("damage mismatch")
			}
		}
	}
	a := encodeMove(event.EntityMoved{ID: 1})
	saved := append([]byte(nil), a...)
	for i := 0; i < 1000; i++ {
		fixedPacketBytes = encodeMove(event.EntityMoved{ID: domain.EntityID(i)})
	}
	if !bytes.Equal(a, saved) {
		t.Fatal("retained packet mutated")
	}
}
func BenchmarkFixedPackets(b *testing.B) {
	for _, tc := range []struct {
		name     string
		old, new func() []byte
	}{
		{"Move", func() []byte { return legacyEncodeMove(event.EntityMoved{ID: 1, Speed: 100}) }, func() []byte { return encodeMove(event.EntityMoved{ID: 1, Speed: 100}) }},
		{"Damage", func() []byte { return legacyEncodeDamage(event.DamageDealt{Src: 1, Dst: 2, Amount: 42}) }, func() []byte { return encodeDamage(event.DamageDealt{Src: 1, Dst: 2, Amount: 42}) }},
		{"Despawn", func() []byte { return legacyEncodeDespawn(event.EntityDespawned{ID: 1}) }, func() []byte { return encodeDespawn(event.EntityDespawned{ID: 1}) }},
	} {
		b.Run(tc.name+"/Baseline", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				fixedPacketBytes = tc.old()
			}
		})
		b.Run(tc.name+"/Exact", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				fixedPacketBytes = tc.new()
			}
		})
	}
}

func TestFixedPacketCapacityAndOwnership(t *testing.T) {
	for _, tc := range []struct {
		name   string
		size   int
		encode func(int) []byte
	}{
		{"Move", 19, func(id int) []byte { return encodeMove(event.EntityMoved{ID: domain.EntityID(id)}) }},
		{"Damage", 19, func(id int) []byte {
			return encodeDamage(event.DamageDealt{Src: domain.EntityID(id), Amount: int32(id)})
		}},
		{"EntityDespawn", 7, func(id int) []byte {
			return encodeDespawn(event.EntityDespawned{ID: domain.EntityID(id), Kind: domain.KindPlayer})
		}},
		{"DropDespawn", 6, func(id int) []byte {
			return encodeDespawn(event.EntityDespawned{ID: domain.EntityID(id), Kind: domain.KindDrop})
		}},
		{"NPCDespawn", 6, func(id int) []byte {
			return encodeDespawn(event.EntityDespawned{ID: domain.EntityID(id), Kind: domain.KindNPC})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			retained := tc.encode(123)
			saved := bytes.Clone(retained)
			if len(retained) != tc.size || cap(retained) != tc.size {
				t.Fatalf("length/capacity = %d/%d, want %d/%d", len(retained), cap(retained), tc.size, tc.size)
			}
			for i := 0; i < 1000; i++ {
				next := tc.encode(i)
				for j := range next {
					next[j] = 0xff
				}
			}
			if !bytes.Equal(retained, saved) {
				t.Fatal("later encoding or caller mutation modified retained packet")
			}
		})
	}
}
