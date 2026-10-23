package domain

import "testing"

func TestLegendaryShortenLongestSlowPreservesOtherInstances(t *testing.T) {
	s := NewStatusSet()
	now := Ticks(1000)
	slow := func(id StatusID, seconds int32) StatusDef {
		return StatusDef{ID: id, DurationSec: seconds, TypeKnown: true, Type: StatusBad, Affixes: []Affix{{Attr: AttrMoveSpeed, Value: -20, Mode: ModePercent}}}
	}
	s.Apply(slow(9101, 8), now, 11)
	s.Apply(slow(9102, 4), now, 12)
	root := slow(9103, 20)
	root.BlockMove = true
	s.Apply(root, now, 13)
	permanent := slow(9104, 0)
	permanent.Permanent = true
	s.Apply(permanent, now, 14)
	if !s.ShortenLongestSlow(now, 75) {
		t.Fatal("no slow shortened")
	}
	got := map[StatusID]Active{}
	s.Each(func(a Active) { got[a.Def.ID] = a })
	if got[9101].ExpireAt != now+Ticks(2000) || got[9101].Source != 11 || got[9101].Def.Affixes[0].Value != -20 {
		t.Fatal("wrong longest instance")
	}
	if got[9102].ExpireAt != now+Ticks(4000) || got[9103].ExpireAt != now+Ticks(20000) || got[9104].ExpireAt != 0 {
		t.Fatal("other status modified")
	}
}
func TestLegendaryShortenSlowStableTieAndInvalid(t *testing.T) {
	s := NewStatusSet()
	now := Ticks(500)
	for _, id := range []StatusID{9202, 9201} {
		s.Apply(StatusDef{ID: id, DurationSec: 4, TypeKnown: true, Type: StatusBad, Affixes: []Affix{{Attr: AttrMoveSpeed, Value: -10}}}, now, 1)
	}
	if s.ShortenLongestSlow(now, 0) {
		t.Fatal("zero percentage changed")
	}
	if !s.ShortenLongestSlow(now, 50) {
		t.Fatal("tie not shortened")
	}
	s.Each(func(a Active) {
		want := now + Ticks(4000)
		if a.Def.ID == 9201 {
			want = now + Ticks(2000)
		}
		if a.ExpireAt != want {
			t.Fatal("unstable tie")
		}
	})
	if s.ShortenLongestSlow(now+Ticks(5000), 75) {
		t.Fatal("expired slow modified")
	}
	s = NewStatusSet()
	s.Apply(StatusDef{ID: 9301, DurationSec: 4, TypeKnown: true, Type: StatusBad, Affixes: []Affix{{Attr: AttrMoveSpeed, Value: -10}}}, 0, 1)
	if !s.ShortenLongestSlow(0, 100) || s.Has(9301) {
		t.Fatal("full shortening turned into permanent")
	}
}
