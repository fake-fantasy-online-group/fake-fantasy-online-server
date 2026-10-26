package store

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	catalog "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
	"strings"
	"testing"
)

func TestSkillResetDurableBackendCommitsAndReplaysExactlyOnce(t *testing.T) {
	m, before := endgameFixture(t)
	before.Char.Level = 35
	before.Char.Race = domain.Warlock
	before.Char.SkillPointsKnown = true
	before.Char.SkillAllocationVersion = 5
	before.Char.SkillPoints = 31
	before.Char.Skills = domain.Learned{10401: 3, domain.SkillAlchemy: 90}
	before.Bag.Set(0, domain.Stack{Item: 3380, Count: 2})
	if err := m.SaveSnapshot(context.Background(), before); err != nil {
		t.Fatal(err)
	}
	after := before.Clone()
	if _, _, err := domain.ResetAllocatedSkills(after.Char, catalog.SkillProgressionCatalog()); err != nil {
		t.Fatal(err)
	}
	if !after.Bag.RemoveAt(0, 1) {
		t.Fatal("consume")
	}
	plan := domain.EndgameCommitPlan{Before: before, Snapshot: after, State: domain.EndgameState{Revision: 1, Payload: "{}"}, Receipt: domain.EndgameReceipt{CharID: before.Char.ID, RequestID: "skill-reset-v1-0-0", Operation: "skill.reset", Digest: strings.Repeat("a", 64), Revision: 1, Response: `{"kind":"skill.reset","ok":true}`}}
	backend := NewEndgameBackend(m, nil)
	if got := awaitEndgame(t, backend.Commit(plan)); got.Err != nil || got.Replayed {
		t.Fatal(got)
	}
	saved := durableSnapshot(t, m, before)
	if saved.Char.SkillPoints != 34 || saved.Char.Skills[10401] != 0 || saved.Char.Skills[domain.SkillAlchemy] != 90 || saved.Bag.CountOf(3380) != 1 {
		t.Fatal("invalid stored reset", saved.Char)
	}
	replay := awaitEndgame(t, backend.Commit(plan))
	if replay.Err != nil || !replay.Replayed || replay.Authoritative == nil || replay.Authoritative.Char.SkillPoints != 34 || replay.Authoritative.Bag.CountOf(3380) != 1 {
		t.Fatal("invalid replay", replay)
	}
}
