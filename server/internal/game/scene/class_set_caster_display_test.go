package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestCasterSetActualCastBuffDisplay(t *testing.T) {
	for _, tc := range []struct {
		skill domain.SkillID
		view  int32
		name  string
		icon  int32
	}{
		{10409, classSetFireViewID, "火系增伤", 701}, {10401, classSetIceViewID, "冰系增伤", 709},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, p, target := casterSetFixture(t, 3, 2)
			fire, _ := s.skills.Get(10401, 1)
			fire.Icon = 701
			s.skills[domain.SkillKey{ID: 10401, Level: 1}] = fire
			ice, _ := s.skills.Get(10409, 1)
			ice.Icon = 709
			s.skills[domain.SkillKey{ID: 10409, Level: 1}] = ice
			s.exec(UseSkill{ID: p.ID, Skill: tc.skill, Target: target})
			s.stepCasts()
			check := func(want int32) {
				t.Helper()
				count := 0
				for _, b := range s.buffSnapshot(p).Buffs {
					if b.SkillID == tc.view {
						count++
						if b.Name != tc.name || b.Icon != tc.icon || b.RemainSec != want || b.Desc == "" {
							t.Fatalf("wrong buff %+v", b)
						}
					}
				}
				if count != 1 {
					t.Fatalf("want one buff got %d", count)
				}
			}
			check(3)
			def, _ := s.skills.Get(tc.skill, 1)
			s.startCasterSetCast(p, def)
			s.startCasterSetCast(p, def)
			check(3)
			s.tick += domain.Ticks(1000)
			check(2)
			s.tick += domain.Ticks(2000)
			s.stepCasterSets()
			if len(s.classSetCasterPresentation(p)) != 0 {
				t.Fatal("expired buff remains")
			}
			s.startCasterSetCast(p, def)
			p.HP = 0
			s.stepCasterSets()
			if len(s.classSetCasterPresentation(p)) != 0 {
				t.Fatal("dead buff remains")
			}
		})
	}
}
func TestCasterSetDisplayBothAndRemoval(t *testing.T) {
	s, p, _ := casterSetFixture(t, 3, 2)
	fire, _ := s.skills.Get(10401, 1)
	ice, _ := s.skills.Get(10409, 1)
	s.startCasterSetCast(p, fire)
	s.tick += domain.Ticks(1000)
	s.startCasterSetCast(p, ice)
	v := s.classSetCasterPresentation(p)
	if len(v) != 2 || v[0].buff.RemainSec != 3 || v[1].buff.RemainSec != 2 {
		t.Fatalf("independent timers %+v", v)
	}
	p.Player.Worn.Set(domain.SlotNeck, domain.Stack{})
	s.invalidateCombatAttributes(p)
	s.stepCasterSets()
	if len(s.classSetCasterPresentation(p)) != 0 {
		t.Fatal("unequipped buff remains")
	}
}

func TestCasterSetDisplayLifecycleEmitsRemoval(t *testing.T) {
	for _, mode := range []string{"expiry", "unequip", "broken", "death"} {
		t.Run(mode, func(t *testing.T) {
			s, p, _ := casterSetFixture(t, 3, 2)
			sink := p.Player.Sink.(*fakeSink)
			ice, _ := s.skills.Get(10409, 1)
			s.startCasterSetCast(p, ice)
			s.flush()
			sink.take()
			switch mode {
			case "expiry":
				s.tick += domain.Ticks(3000)
				s.stepCasterSets()
			case "unequip":
				p.Player.Worn.Set(domain.SlotNeck, domain.Stack{})
				s.refreshStats(p)
			case "broken":
				st := p.Player.Worn.At(domain.SlotNeck)
				st.Durability = 0
				p.Player.Worn.Set(domain.SlotNeck, st)
				s.refreshStats(p)
			case "death":
				p.HP = 0
				s.onDeath(p, 0)
			}
			s.flush()
			found := false
			for _, e := range sink.take() {
				if b, ok := e.(event.BuffSnapshot); ok && b.Who == p.ID {
					found = true
					for _, v := range b.Buffs {
						if v.SkillID == classSetFireViewID || v.SkillID == classSetIceViewID {
							t.Fatal("removal snapshot kept grant")
						}
					}
				}
			}
			if !found {
				t.Fatal("no removal snapshot sent")
			}
		})
	}
}
