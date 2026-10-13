package scene

import (
	"math"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

func riftAbilityFixture(t *testing.T, ability endgameRiftAbility) (*Scene, *entity.Entity, *entity.Entity, *int64) {
	t.Helper()
	s, backend, now := riftFixture(t)
	activateTestRift(t, s, backend, *now)
	s.clearRiftMonsters()
	m := spawn.Encounter(s.alloc.Monster(), domain.Pos{MapID: 7, X: 140, Y: 100}, 0,
		domain.MonsterDef{ID: 1, Kind: domain.MonsterElite, HP: 5000,
			Stats: domain.Stats{MinAtk: 100, MaxAtk: 100, MaxHP: 5000}})
	s.entities[m.ID], s.monsters[m.ID] = m, m
	s.aoi.Enter(m)
	s.rift.baseStats[m.ID] = m.Stats
	s.rift.data.Run.RegisterMonster(m.ID, 1)
	m.Monster.Target = 1
	s.registerEndgameRiftAbilities(m, 1, 0, 13, 1)
	state := s.rift.abilities[m.ID]
	state.pattern, state.nextAt = []endgameRiftAbility{ability}, *now
	p := s.players[1]
	p.HP, p.MaxHP = 10000, 10000
	p.Stats.PhysResist = 0
	s.flush()
	p.Player.Sink.(*fakeSink).take()
	return s, m, p, now
}

func TestRiftAbilityPatternsDeterministicAndIsolated(t *testing.T) {
	s, m, _, now := riftAbilityFixture(t, riftEruption)
	for class := int32(1); class <= 3; class++ {
		s.registerEndgameRiftAbilities(m, class, 23, 999, 2)
		first := *s.rift.abilities[m.ID]
		for i := 0; i < 10; i++ {
			s.rng.Intn(100)
		}
		copyMonster := *m
		copyMonster.ID = s.alloc.Monster()
		s.registerEndgameRiftAbilities(&copyMonster, class, 23, 999, 2)
		second := *s.rift.abilities[copyMonster.ID]
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("class %d pattern depends on combat RNG/entity allocation", class)
		}
		seen := map[endgameRiftAbility]bool{}
		for _, a := range first.pattern {
			seen[a] = true
		}
		if len(first.pattern) != int(class) || len(seen) != int(class) {
			t.Fatalf("class %d pattern %v should contain distinct affixes", class, first.pattern)
		}
		if first.nextAt < *now+3000 || first.nextAt >= *now+6000 {
			t.Fatal("missing initial grace")
		}
	}
	s.registerEndgameRiftAbilities(m, 0, 23, 999, 2)
	varied := map[endgameRiftAbility]bool{}
	for seed := uint64(0); seed < 32; seed++ {
		s.registerEndgameRiftAbilities(m, 1, 23, seed, 2)
		varied[s.rift.abilities[m.ID].pattern[0]] = true
	}
	if len(varied) != 3 {
		t.Fatal("seed does not vary elite affixes")
	}
	if len(s.riftMonsterAffixes(m.ID)) != 1 {
		t.Fatal("missing readable aura")
	}
	if riftAbilitySeed(5, 1, 1, 1, 1) == riftAbilitySeed(5, 1, 1, 150, 1) || riftAbilitySeed(5, 1, 1, 1, 1) == riftAbilitySeed(5, 1, 1, 1, 2) {
		t.Fatal("tier/floor absent from trait seed")
	}
}

func TestRiftAbilityShapesMatchWarningGeometry(t *testing.T) {
	origin := domain.Pos{MapID: 7}
	tests := []struct {
		ability endgameRiftAbility
		p       domain.Pos
		hit     bool
	}{
		{riftEruption, domain.Pos{MapID: 7, X: 105}, true},
		{riftEruption, domain.Pos{MapID: 7, X: 106}, false},
		{riftNova, domain.Pos{MapID: 7, X: 84}, false},
		{riftNova, domain.Pos{MapID: 7, X: 85}, true},
		{riftNova, domain.Pos{MapID: 7, X: 220}, true},
		{riftNova, domain.Pos{MapID: 7, X: 221}, false},
		{riftFissure, domain.Pos{MapID: 7, X: 380, Y: 38}, true},
		{riftFissure, domain.Pos{MapID: 7, X: 381}, false},
		{riftFissure, domain.Pos{MapID: 7, X: -1}, false},
		{riftFissure, domain.Pos{MapID: 7, X: 50, Y: 39}, false},
		{riftEruption, domain.Pos{MapID: 8}, false},
		{riftEruption, domain.Pos{MapID: 7, X: math.NaN()}, false},
		{riftFissure, domain.Pos{MapID: 7, X: math.Inf(1)}, false},
	}
	for _, tc := range tests {
		c := newRiftAbilityCast(tc.ability, origin, origin, 1, 0, riftRulesFixture())
		if got := c.contains(tc.p); got != tc.hit {
			t.Fatalf("ability %d pos %+v got %v", tc.ability, tc.p, got)
		}
	}
}

func TestRiftAbilityWarningDodgeAndCooldown(t *testing.T) {
	for _, dodge := range []bool{false, true} {
		s, m, p, now := riftAbilityFixture(t, riftEruption)
		s.stepEndgameRiftAbilities(*now)
		state := s.rift.abilities[m.ID]
		if state.pending == nil || !s.riftMonsterAbilityBusy(m.ID) {
			t.Fatal("missing warning/freeze")
		}
		views := s.riftAbilityTelegraphs(*now)
		if len(views) != 1 || views[0].RemainingMS != 1500 || views[0].Shape != "circle" || views[0].Ability != "烈焰爆裂" {
			t.Fatalf("bad tell: %+v", views)
		}
		if s.rift.lastBroadcast > *now-1000 {
			t.Fatal("warning waits for periodic broadcast")
		}
		*now = state.pending.dueAt - 1
		s.stepEndgameRiftAbilities(*now)
		if p.HP != 10000 {
			t.Fatal("damage before warning completed")
		}
		if dodge {
			p.Pos.X += 106
		}
		*now++
		s.stepEndgameRiftAbilities(*now)
		wantHP := int32(9860)
		if dodge {
			wantHP = 10000
		}
		if p.HP != wantHP {
			t.Fatalf("dodge %v hp=%d want=%d", dodge, p.HP, wantHP)
		}
		if state.pending != nil || len(s.riftAbilityTelegraphs(*now)) != 0 {
			t.Fatal("resolved warning remains")
		}
		if !s.riftMonsterAbilityBusy(m.ID) {
			t.Fatal("missing punish/recovery window")
		}
		*now += riftAbilityRecoveryMS
		if s.riftMonsterAbilityBusy(m.ID) {
			t.Fatal("recovery did not expire")
		}
		s.stepEndgameRiftAbilities(*now)
		if p.HP != wantHP || state.pending != nil {
			t.Fatal("same ability resolved twice or bypassed cooldown")
		}
	}
}

func TestRiftAbilityLocksAimAndUsesImpactPosition(t *testing.T) {
	s, m, p, now := riftAbilityFixture(t, riftFissure)
	s.stepEndgameRiftAbilities(*now)
	cast := s.rift.abilities[m.ID].pending
	center, end := cast.center, cast.end
	p.Pos.Y += 100
	*now = cast.dueAt
	s.stepEndgameRiftAbilities(*now)
	if p.HP != 10000 || cast.center != center || cast.end != end {
		t.Fatal("fissure tracked dodging player")
	}
}

func TestRiftAbilityCancelsOnLifecycleAndControlChanges(t *testing.T) {
	cases := map[string]func(*Scene, *entity.Entity, *entity.Entity){
		"economic commit":    func(s *Scene, m, p *entity.Entity) { s.rift.economicPending = true },
		"loading floor":      func(s *Scene, m, p *entity.Entity) { s.rift.floorLoading = true },
		"retired floor":      func(s *Scene, m, p *entity.Entity) { s.rift.retired = true },
		"transition":         func(s *Scene, m, p *entity.Entity) { s.rift.transition = &endgameRiftTransition{} },
		"terminal":           func(s *Scene, m, p *entity.Entity) { s.rift.data.Run.Phase = domain.RiftCompleted },
		"unready":            func(s *Scene, m, p *entity.Entity) { s.rift.ready[100] = false },
		"departed":           func(s *Scene, m, p *entity.Entity) { s.rift.data.Run.Members[100].Departed = true },
		"map loading":        func(s *Scene, m, p *entity.Entity) { s.mapLoads = map[domain.EntityID]*mapLoad{p.ID: {}} },
		"protected":          func(s *Scene, m, p *entity.Entity) { p.Player.Protect(s.tick + reviveGraceTicks) },
		"dead owner":         func(s *Scene, m, p *entity.Entity) { m.HP = 0 },
		"owner removed":      func(s *Scene, m, p *entity.Entity) { delete(s.monsters, m.ID) },
		"unregistered owner": func(s *Scene, m, p *entity.Entity) { delete(s.rift.data.Run.Monsters, m.ID) },
		"different map":      func(s *Scene, m, p *entity.Entity) { p.Pos.MapID = 8 },
		"wall":               func(s *Scene, m, p *entity.Entity) { s.walkable = func(domain.Pos) bool { return false } },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			s, m, p, now := riftAbilityFixture(t, riftEruption)
			s.stepEndgameRiftAbilities(*now)
			due := s.rift.abilities[m.ID].pending.dueAt
			change(s, m, p)
			*now = due
			s.stepEndgameRiftAbilities(*now)
			if p.HP != 10000 {
				t.Fatalf("%s caused damage", name)
			}
		})
	}
}

func TestRiftAbilityLateReadyPlayerCannotReceiveHiddenImpact(t *testing.T) {
	s, m, p, now := riftAbilityFixture(t, riftEruption)
	// Add a second admitted character, still awaiting its map-ready handshake.
	peer := *p
	player := *p.Player
	character := *p.Player.Char
	character.ID = 200
	player.Char = &character
	peer.ID, peer.Player = 2, &player
	s.players[2], s.entities[2] = &peer, &peer
	s.rift.data.Run.Members[200] = &domain.RiftMember{Level: 60}
	s.stepEndgameRiftAbilities(*now)
	cast := s.rift.abilities[m.ID].pending
	s.rift.ready[200] = true
	*now = cast.dueAt
	s.stepEndgameRiftAbilities(*now)
	if peer.HP != 10000 || p.HP >= 10000 {
		t.Fatal("late-ready player received an unseen cast or original target was skipped")
	}
}

func TestRiftAbilityPausedTransferPreservesAffixesAndNewTell(t *testing.T) {
	s, m, p, now := riftAbilityFixture(t, riftEruption)
	s.stepEndgameRiftAbilities(*now)
	before := s.riftMonsterAffixes(m.ID)
	s.cancelEndgameRiftAbilities(*now)
	state := s.rift.abilities[m.ID]
	if state.pending != nil || !reflect.DeepEqual(before, s.riftMonsterAffixes(m.ID)) {
		t.Fatal("cancel discarded traits")
	}
	*now += 2000
	s.stepEndgameRiftAbilities(*now)
	if state.pending == nil || state.pending.dueAt <= *now || p.HP != 10000 {
		t.Fatal("resume skipped fresh warning")
	}
	s.clearEndgameRiftAbilities()
	if len(s.rift.abilities) != 0 {
		t.Fatal("retiring monsters leaked ability state")
	}
}

func TestRiftAbilityLagDoesNotReplayDamageAndUsesScaledStats(t *testing.T) {
	s, m, p, now := riftAbilityFixture(t, riftEruption)
	m.Stats.MinAtk, m.Stats.MaxAtk = 1000, 1000
	s.stepEndgameRiftAbilities(*now)
	*now += 60000
	s.stepEndgameRiftAbilities(*now)
	if p.HP != 8600 {
		t.Fatalf("expected one scaled strike, hp %d", p.HP)
	}
	s.stepEndgameRiftAbilities(*now)
	if p.HP != 8600 {
		t.Fatal("replayed missed cooldowns")
	}
	s.flush()
	events := p.Player.Sink.(*fakeSink).take()
	if countOf[event.DamageDealt](events) != 1 {
		t.Fatal("damage not emitted through native authoritative event")
	}
}

func TestRiftAbilityNoUnengagedOrUnseenCasts(t *testing.T) {
	for _, reason := range []string{"no target", "invisible", "wall", "far", "unready"} {
		s, m, p, now := riftAbilityFixture(t, riftEruption)
		switch reason {
		case "no target":
			m.Monster.Target = 0
		case "wall":
			s.walkable = func(domain.Pos) bool { return false }
		case "far":
			p.Pos.X += 1000
		case "unready":
			s.rift.ready[100] = false
		case "invisible":
			p.Status.Apply(domain.StatusDef{ID: 1900, Invisible: true, DurationSec: 10}, s.tick, p.ID)
		}
		s.stepEndgameRiftAbilities(*now)
		if s.rift.abilities[m.ID].pending != nil {
			t.Fatalf("started cast: %s", reason)
		}
	}
}

func TestRiftAbilityMitigationAndReflectionUseNativePipeline(t *testing.T) {
	s, m, p, now := riftAbilityFixture(t, riftEruption)
	p.Status.Apply(domain.StatusDef{ID: 1037, DurationSec: 30, PhysicalDamageReductionPct: 50, PhysicalReflectPct: 50}, s.tick, p.ID)
	s.stepEndgameRiftAbilities(*now)
	*now = s.rift.abilities[m.ID].pending.dueAt
	s.stepEndgameRiftAbilities(*now)
	if p.HP != 9930 || m.HP != 4965 {
		t.Fatalf("mitigation/reflect bypassed: player=%d monster=%d", p.HP, m.HP)
	}
}

func TestRiftAbilityReflectionDeathStopsDamageAndNativeLoot(t *testing.T) {
	s, m, p, now := riftAbilityFixture(t, riftEruption)
	m.HP = 1
	p.Status.Apply(domain.StatusDef{ID: 1037, DurationSec: 30, PhysicalReflectPct: 100}, s.tick, p.ID)
	peer := *p
	player := *p.Player
	character := *p.Player.Char
	character.ID = 200
	player.Char = &character
	peer.ID, peer.Player, peer.Status = 2, &player, domain.NewStatusSet()
	s.players[2], s.entities[2] = &peer, &peer
	s.rift.data.Run.Members[200] = &domain.RiftMember{Level: 60}
	s.rift.ready[200] = true
	beforeExp := p.Player.Char.Exp
	// Reflection reaches the guardian threshold and replaces the population.
	s.rift.data.Run.Progress = domain.RiftProgressMax - 1
	s.stepEndgameRiftAbilities(*now)
	*now = s.rift.abilities[m.ID].pending.dueAt
	s.stepEndgameRiftAbilities(*now)
	if m.Alive() || peer.HP != 10000 {
		t.Fatal("dead source continued damaging party")
	}
	if p.Player.Char.Exp != beforeExp {
		t.Fatal("rift affix reflected kill produced native experience")
	}
	for _, e := range s.entities {
		if e.Kind == domain.KindDrop {
			t.Fatal("rift affix reflected kill produced native loot")
		}
	}
}

func TestRiftGuardianCyclesAllAffixesWithoutFloorDamageMultiplier(t *testing.T) {
	s, m, _, now := riftAbilityFixture(t, riftEruption)
	s.registerEndgameRiftAbilities(m, 3, 10, 71, 9)
	state := s.rift.abilities[m.ID]
	pattern := append([]endgameRiftAbility(nil), state.pattern...)
	for i := 0; i < 6; i++ {
		*now = state.nextAt
		s.stepEndgameRiftAbilities(*now)
		if state.pending == nil || state.pending.ability != pattern[i%3] {
			t.Fatal("guardian lost deterministic rotation")
		}
		*now = state.pending.dueAt
		s.stepEndgameRiftAbilities(*now)
	}
	one := newRiftAbilityCast(riftEruption, m.Pos, m.Pos, 3, 0, riftRulesFixture())
	s.registerEndgameRiftAbilities(m, 3, 10, 71, 99)
	two := newRiftAbilityCast(riftEruption, m.Pos, m.Pos, 3, 0, riftRulesFixture())
	if one.damagePct != two.damagePct || one.dueAt != two.dueAt {
		t.Fatal("floor number changed damage/warning")
	}
}

func TestRiftAbilityWarningLimitAndControlInterrupt(t *testing.T) {
	s, m, p, now := riftAbilityFixture(t, riftEruption)
	for i := 0; i < 6; i++ {
		other := *m
		monster := *m.Monster
		other.ID, other.Monster = s.alloc.Monster(), &monster
		s.monsters[other.ID], s.entities[other.ID] = &other, &other
		s.rift.data.Run.RegisterMonster(other.ID, 1)
		s.registerEndgameRiftAbilities(&other, 1, i+1, 13, 1)
		s.rift.abilities[other.ID].nextAt = *now
	}
	s.stepEndgameRiftAbilities(*now)
	if len(s.riftAbilityTelegraphs(*now)) != riftMaxTelegraphs {
		t.Fatal("overlapping tells not bounded")
	}
	for _, source := range s.monsters {
		if source.Status == nil {
			source.Status = domain.NewStatusSet()
		}
		source.Status.Apply(domain.StatusDef{ID: domain.StatusStun, DurationSec: 30, Control: domain.ControlStun}, s.tick, p.ID)
	}
	*now += 2000
	s.stepEndgameRiftAbilities(*now)
	if p.HP != 10000 || len(s.riftAbilityTelegraphs(*now)) != 0 {
		t.Fatal("stun did not cancel warning")
	}
}

func TestRiftAbilityMagicTemplateUsesScaledMagicChannel(t *testing.T) {
	s, m, p, now := riftAbilityFixture(t, riftEruption)
	m.Stats.MinAtk, m.Stats.MaxAtk, m.Stats.MAtk = 0, 0, 1000
	p.Stats.MDef, p.Stats.MagicResist = 100, 7
	s.stepEndgameRiftAbilities(*now)
	*now = s.rift.abilities[m.ID].pending.dueAt
	s.stepEndgameRiftAbilities(*now)
	if p.HP != 8670 {
		t.Fatalf("magic template scaling/defense bypassed, hp %d", p.HP)
	}
}

func TestRiftAbilityTier150DamageNeverWrapsOrHeals(t *testing.T) {
	s, m, p, now := riftAbilityFixture(t, riftEruption)
	m.Stats.MinAtk, m.Stats.MaxAtk = math.MaxInt32, math.MaxInt32
	m.Stats.CritRate = 10000
	p.HP, p.MaxHP = math.MaxInt32, math.MaxInt32
	s.rift.data.Run.Tier = 150
	s.stepEndgameRiftAbilities(*now)
	*now = s.rift.abilities[m.ID].pending.dueAt
	s.stepEndgameRiftAbilities(*now)
	if p.HP != 0 {
		t.Fatalf("high-tier strike wrapped/healed: hp %d", p.HP)
	}
	if s.rift.data.Run.PenaltyMS != domain.RiftDeathPenaltyMS {
		t.Fatal("fatal ability did not record native rift death penalty")
	}
}
