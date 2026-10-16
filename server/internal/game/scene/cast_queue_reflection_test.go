package scene

import (
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// A reflected native skill can kill its own caster and synchronously clear the
// rift population. Queue ownership must survive this indirect death path too.
func TestCastQueueReflectedDeathStartsGuardianArrival(t *testing.T) {
	s, _, _ := arrivalRiftFixture(t)
	r := s.rift.data.Run
	r.Progress = r.Rules.ProgressMax - r.Rules.NormalProgress
	p := s.players[1]
	p.Status = domain.NewStatusSet()
	p.Status.Apply(domain.StatusDef{
		ID: 1011, Level: 1, DurationSec: 100, MagicFullReflectChancePct: 100,
	}, s.tick, p.ID)
	var caster *entity.Entity
	for id, m := range s.monsters {
		if r.Monsters[id].Class == 0 {
			caster = m
			break
		}
	}
	if caster == nil {
		t.Fatal("missing normal monster")
	}
	caster.HP = 1
	caster.Stats.MAtk = 1000000
	beforeHP, beforeExp := p.HP, p.Player.Char.Exp
	s.casts = []pendingCast{
		{src: caster.ID, targets: []domain.EntityID{p.ID}, def: 火球术, released: true},
		{src: caster.ID, targets: []domain.EntityID{p.ID}, def: 火球术, released: true},
	}
	s.stepCastsAt(time.Now(), false)
	if caster.Alive() || r.Phase != domain.RiftBossArriving || len(s.monsters) != 0 {
		t.Fatalf("reflected death did not clear population: alive=%v phase=%v monsters=%d", caster.Alive(), r.Phase, len(s.monsters))
	}
	if len(s.casts) != 0 || s.steppingCasts {
		t.Fatalf("cast queue not settled: casts=%d stepping=%v", len(s.casts), s.steppingCasts)
	}
	if p.HP != beforeHP || p.Player.Char.Exp != beforeExp {
		t.Fatalf("reflection damaged player or awarded native XP: hp=%d want=%d exp=%d want=%d", p.HP, beforeHP, p.Player.Char.Exp, beforeExp)
	}
}
