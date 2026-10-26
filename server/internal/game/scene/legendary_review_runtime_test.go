package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

// Generic state-transfer regressions survive the approved removal of the old
// TY timed-reserve/ledger mechanics. They do not reintroduce those abilities.
func TestLegendaryReviewPermanentStateTransfer(t *testing.T) {
	s, p, _ := legendaryMeleeFixture(t, "ZS-A04")
	power, _ := domain.LegendaryPower("ZS-A04")
	c := legendaryContext{Scene: s, Owner: p, Power: power, SourceUID: 991, NowMS: s.tick.Millis()}
	st := c.State(0, "review-source-bound")
	st.ExpiresMS = -1
	st.Value = 42
	handoff := s.captureLegendaryRuntime(p)
	dest, q, _ := legendaryMeleeFixture(t, "ZS-A04")
	dest.restoreLegendaryRuntime(q, handoff)
	d := legendaryContext{Scene: dest, Owner: q, Power: power, SourceUID: 991, NowMS: dest.tick.Millis()}
	got := d.State(0, "review-source-bound")
	if got.ExpiresMS != -1 || got.Value != 42 {
		t.Fatalf("permanent state lost: %+v", got)
	}
}
func TestLegendaryReviewDeletedStateNotRevivedByTransfer(t *testing.T) {
	s, p, _ := legendaryMeleeFixture(t, "ZS-A04")
	power, _ := domain.LegendaryPower("ZS-A04")
	c := legendaryContext{Scene: s, Owner: p, Power: power, SourceUID: 991}
	st := c.State(0, "review-consumed")
	st.ExpiresMS = -1
	st.Value = 42
	c.DeleteState(0, "review-consumed")
	handoff := s.captureLegendaryRuntime(p)
	dest, q, _ := legendaryMeleeFixture(t, "ZS-A04")
	dest.restoreLegendaryRuntime(q, handoff)
	d := legendaryContext{Scene: dest, Owner: q, Power: power, SourceUID: 991}
	if len(d.States("review-consumed")) != 0 {
		t.Fatal("consumed state revived")
	}
}
func TestLegendaryReviewNativeDOTRespectsOrdinaryShield(t *testing.T) {
	s, _, sink := statusScene(t)
	p := beefUp(s)
	s.applyStatusDef(p, domain.StatusDef{ID: 99983, DurationSec: 10, DamageShield: 80}, p.ID)
	p.Stats.MDef, p.Stats.MagicResist = 0, 0 // isolate shield consumption
	sink.take()
	s.tickStatus(p, domain.Active{Source: p.ID, Def: domain.StatusDef{ID: 99984, HPChanges: []domain.ResourceChange{{Value: -100}}}})
	s.flush()
	hit, ok := firstOf[event.DamageDealt](sink.take())
	if !ok || hit.Amount != 20 {
		t.Fatalf("native DOT must consume shield: %+v found=%v", hit, ok)
	}
}
