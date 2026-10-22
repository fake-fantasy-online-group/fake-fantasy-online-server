package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"testing"
)

func TestDungeonDamageFinalPhysicalMagicFixedAndDOT(t *testing.T) {
	s, p, m := setCardFixture(t, 1)
	s.id = domain.SceneID{MapID: 20502, Instance: 1}
	s.dungeon = &塔狱
	s.dungeons = domain.DungeonTable{20502: 塔狱}
	m.Monster.Kind = domain.MonsterBoss
	m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000, Hit: 1000000}
	p.Stats.Def = 0
	p.Stats.MDef = 0
	p.Status = domain.NewStatusSet()
	for _, difficulty := range []domain.DungeonDifficulty{domain.DungeonNormal, domain.DungeonHard} {
		s.dungeonDifficulty = difficulty
		for _, blow := range []combat.Blow{{}, {Magic: true}, {Magic: true, FlatDamage: 1000}} {
			p.HP = 100000
			p.MaxHP = 100000
			ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, blow)
			want := int32(500)
			if difficulty == domain.DungeonHard {
				want = 1500
			}
			if blow.FlatDamage > 0 {
				want *= 2
			}
			if ev.Amount != want {
				t.Fatalf("mode%d blow%+v amount%d want%d", difficulty, blow, ev.Amount, want)
			}
		}
		p.HP = 100000
		def := domain.StatusDef{ID: 999998, Level: 1, DurationSec: 30, HPChanges: []domain.ResourceChange{{Value: -1000}}, DungeonDamageScaleBP: s.dungeonMonsterDamage(m, p, 10000)}
		// Snapshot survives removal/death of the source and is not multiplied twice.
		delete(s.entities, m.ID)
		s.tickStatus(p, domain.Active{Def: def, Source: m.ID})
		want := int32(99500)
		if difficulty == domain.DungeonHard {
			want = 98500
		}
		if p.HP != want {
			t.Fatalf("DOT HP%d want%d", p.HP, want)
		}
		s.entities[m.ID] = m
	}
	s.rift = &endgameRiftState{}
	if got := s.dungeonMonsterDamage(m, p, 1000); got != 1000 {
		t.Fatal("rift damage scaled")
	}
	s.rift = nil
	if got := s.dungeonMonsterDamage(p, m, 1000); got != 1000 {
		t.Fatal("player damage scaled")
	}
}

func TestDungeonDOTSnapshotsAtApplicationWithoutMutatingTemplate(t *testing.T) {
	s, p, m := setCardFixture(t, 1)
	s.id = domain.SceneID{MapID: 20502, Instance: 1}
	s.dungeon = &塔狱
	s.dungeons = domain.DungeonTable{20502: 塔狱}
	s.dungeonDifficulty = domain.DungeonHard
	m.Monster.Kind = domain.MonsterBoss
	original := domain.StatusDef{ID: 999997, Level: 1, DurationSec: 30, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: -100}}}
	if !s.applyStatusDef(p, original, m.ID) {
		t.Fatal("status rejected")
	}
	var snapshot domain.Active
	p.Status.Each(func(a domain.Active) {
		if a.Def.ID == original.ID {
			snapshot = a
		}
	})
	if original.DungeonDamageScaleBP != 0 || snapshot.Def.DungeonDamageScaleBP != 15000 {
		t.Fatal("missing immutable source snapshot")
	}
	delete(s.entities, m.ID)
	p.HP = 1000
	p.MaxHP = 1000
	s.tickStatus(p, snapshot)
	if p.HP != 850 {
		t.Fatal("first tick", p.HP)
	}
	s.tickStatus(p, snapshot)
	if p.HP != 700 {
		t.Fatal("second tick compounded multiplier", p.HP)
	}
}
