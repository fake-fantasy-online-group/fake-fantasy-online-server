package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestClassSetDragonAppearanceProjectionNeverPersists(t *testing.T) {
	s, p := meleeSetFixture(t, 5, 4)
	base := domain.Appearance{Gender: 1, Head: 7, Hair: 3, EquipView: [6]uint16{31, 32, 33, 34, 35, 36}, AtkVariant: 2, WeaponCType: 4, AtkDist: 75, Title: "title"}
	p.Player.Char.Appear = base
	p.Look.Appearance = base
	beforeRace, beforeKind := p.Look.Race, p.Kind
	s.refreshClassSetAppearance(p)
	got := p.Look.Appearance
	if got.EquipView != [6]uint16{1371} || got.AtkVariant != 0 || got.Head != 7 || got.Hair != 3 || got.Title != "title" {
		t.Fatalf("projection %+v", got)
	}
	if p.Player.Char.Appear != base || p.Look.Race != beforeRace || p.Kind != beforeKind {
		t.Fatal("transformation modified permanent identity/appearance")
	}
	if snap := s.snapshotOf(p); snap.Char.Appear != base {
		t.Fatal("save snapshot contains dragon")
	}
	// Death keeps the same persistent form for its native death animation.
	p.HP = 0
	if s.classSetAppearance(p, base).EquipView[0] != 1371 {
		t.Fatal("dead dragon lost appearance")
	}
	p.HP = 100
	st := p.Player.Worn.At(domain.SlotGlove)
	st.Durability = 0
	p.Player.Worn.Set(domain.SlotGlove, st)
	s.refreshStats(p)
	if p.Look.Appearance != base || p.Player.Char.Appear != base {
		t.Fatal("broken fourth piece did not restore original view")
	}
	p.Player.Char.Race = domain.Swordsman
	if s.classSetAppearance(p, base) != base {
		t.Fatal("wrong profession transformed")
	}
}

func TestClassSetDragonAppearanceAdmissionSelfAndPublicAgree(t *testing.T) {
	s, p := meleeSetFixture(t, 5, 4)
	p.Player.Char.Appear.Head = 7
	p.Player.Char.Appear.Hair = 3
	next := newCombatScene(t)
	next.items = s.items
	ch := p.Player.Char.Clone()
	ch.ID = 200
	ch.Name = "dragon"
	ch.Pos = domain.Pos{MapID: 7, X: 100, Y: 100}
	sink := &fakeSink{}
	next.onEnter(Enter{ID: 2, Char: ch, Worn: p.Player.Worn.Clone(), Sink: sink})
	next.flush()
	q := next.players[2]
	if q == nil {
		t.Fatal("admission failed")
	}
	self, ok := firstOf[event.SelfEntered](sink.take())
	if !ok || self.Char.Appear.EquipView != [6]uint16{1371} || self.Char.Appear != q.Look.Appearance {
		t.Fatal("self/public transformed views disagree")
	}
	if self.Char == q.Player.Char || q.Player.Char.Appear.EquipView[0] == 1371 || q.Player.Char.Appear.Head != 7 {
		t.Fatal("private character overwritten by bootstrap projection")
	}
	self.Char.Appear.Head = 9
	if q.Player.Char.Appear.Head != 7 {
		t.Fatal("self snapshot aliases permanent character")
	}
	// Wardrobe/equipment refresh must retain form while keeping canonical base.
	next.refreshAppearance(q)
	if q.Look.Appearance.EquipView != [6]uint16{1371} || q.Player.Char.Appear.EquipView[0] == 1371 {
		t.Fatal("appearance refresh lost projection boundary")
	}
}
