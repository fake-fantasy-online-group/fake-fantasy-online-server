package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestLegendaryFixedProcHasNoImplicitAttackCritOrRecursion(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A04")
	p.Stats.MinAtk, p.Stats.MaxAtk, p.Stats.CritRate = 1000, 1000, 10000
	c := s.legendaryContexts(p, m, combat.Blow{}, event.DamageDealt{})[0]
	before := m.HP
	c.Damage(m, 70, false, false)
	if loss := before - m.HP; loss != 70 {
		t.Fatalf("fixed payload got %d want 70", loss)
	}
	if states := c.States("ready"); len(states) != 0 {
		t.Fatal("proc generated new ready state", states)
	}
	m.Stats.PhysResist = 20
	before = m.HP
	c.Damage(m, 70, false, false)
	if loss := before - m.HP; loss != 50 {
		t.Fatalf("native resist got %d want 50", loss)
	}
}
func TestLegendarySettledEchoPreservesShieldsWithoutDoubleMitigation(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A04")
	m.Stats.PhysResist = 999
	m.Status = domain.NewStatusSet()
	m.Status.Apply(domain.StatusDef{ID: 997, DurationSec: 100, PhysicalDamageReductionPct: 80, DamageShield: 30}, s.tick, p.ID)
	c := s.legendaryContexts(p, m, combat.Blow{}, event.DamageDealt{})[0]
	before := m.HP
	c.Damage(m, 70, false, true)
	if loss := before - m.HP; loss != 40 {
		t.Fatalf("settled/shield got %d want40", loss)
	}
}
func TestLegendaryIndependentCooldownAndTransferRebase(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A04")
	s.tick = domain.Ticks(100000)
	c := s.legendaryContexts(p, m, combat.Blow{}, event.DamageDealt{})[0]
	if !c.Ready(5000) || c.Ready(5000) {
		t.Fatal("own cooldown")
	}
	other := c
	other.SourceUID++
	if !other.Ready(5000) {
		t.Fatal("independent item starved")
	}
	state := c.State(p.ID, "ready")
	state.Stacks = 2
	state.ExpiresMS = c.NowMS + 10000
	state.Name = "收势"
	state.Good = true
	mark := c.State(m.ID, "mark")
	mark.Stacks = 1
	mark.ExpiresMS = c.NowMS + 10000
	handoff := s.captureLegendaryRuntime(p)
	dest, q, _ := legendaryMeleeFixture(t, "ZS-A04")
	dest.tick = domain.Ticks(2000)
	dest.restoreLegendaryRuntime(q, handoff)
	d := dest.legendaryContexts(q, nil, combat.Blow{}, event.DamageDealt{})[0]
	if got := d.State(q.ID, "ready"); got.Stacks != 2 || got.ExpiresMS != 12000 {
		t.Fatalf("rebase %+v", got)
	}
	if d.Ready(5000) {
		t.Fatal("transfer reset cooldown")
	}
	if len(d.States("mark")) != 0 {
		t.Fatal("foreign target crossed maps")
	}
	dest.tick += domain.Ticks(5000)
	d.NowMS = dest.tick.Millis()
	if !d.Ready(5000) {
		t.Fatal("rebased cooldown not expired")
	}
}
func TestLegendaryPermanentStatusDoesNotOverflow(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A04")
	c := s.legendaryContexts(p, m, combat.Blow{}, event.DamageDealt{})[0]
	st := c.State(p.ID, "permanent")
	st.Name = "test"
	st.Stacks = 1
	views := s.legendaryStatusPresentation(p)
	if len(views) != 1 || views[0].buff.RemainSec <= 0 {
		t.Fatalf("permanent overflow %+v", views)
	}
}
func TestLegendaryProcGroupsDoNotUseGlobalLock(t *testing.T) {
	s, p, _ := legendaryMeleeFixture(t, "ZS-A04")
	a := domain.EquipmentProc{ID: "a", SharedGroup: "a", ChanceBP: 10000, ICDMS: 4000}
	b := a
	b.ID = "b"
	b.SharedGroup = "b"
	if !s.reserveEquipmentProc(p, &a, 1) || !s.reserveEquipmentProc(p, &b, 1) {
		t.Fatal("independent powers starved")
	}
	if s.reserveEquipmentProc(p, &a, 2) {
		t.Fatal("own ICD bypassed")
	}
}

func TestLegendaryRefundSharedCapCountsOnlyPaidAndEffective(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A04")
	p.MP = 10
	p.MaxMP = 100
	c := s.legendaryContexts(p, m, combat.Blow{}, event.DamageDealt{})[0]
	c.RootID = 99
	c.PaidMP = 20
	if got := c.RefundMP(15); got != 15 {
		t.Fatal(got)
	}
	c.SourceUID++
	if got := c.RefundMP(15); got != 5 {
		t.Fatal("second power exceeded same paid cost", got)
	}
	if got := c.RefundMP(1); got != 0 {
		t.Fatal(got)
	}
	c.RootID++
	c.PaidMP = 0
	if c.RefundMP(10) != 0 {
		t.Fatal("free cast minted MP")
	}
}

func TestLegendaryQueuedFatalPayloadSettlesOnlyOnce(t *testing.T) {
	s, p, m := legendaryMeleeFixture(t, "ZS-A04")
	m.HP = 50
	sink := p.Player.Sink.(*fakeSink)
	sink.evs = nil
	end := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
	c := s.legendaryContexts(p, m, combat.Blow{}, event.DamageDealt{})[0]
	c.Damage(m, 100, false, false)
	c.Damage(m, 100, false, false)
	end()
	s.flush()
	n := 0
	for _, e := range sink.evs {
		if hit, ok := e.(event.DamageDealt); ok && hit.Dst == m.ID && hit.Origin == domain.DamageProcOrigin {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("fatal victim settled %d times", n)
	}
}
func TestLegendaryAbandonedSnapshotClearedWithOwner(t *testing.T) {
	s, p, _ := legendaryMeleeFixture(t, "ZS-A04")
	s.legendaryState().nativeDamageSnapshots = map[uint64]*combat.NativeDamageSnapshot{1: {Owner: p.ID, Amount: 100}, 2: {Owner: 999, Amount: 100}}
	s.clearLegendaryOwner(p.ID)
	if _, ok := s.legendaryState().nativeDamageSnapshots[1]; ok {
		t.Fatal("abandoned cast snapshot survived owner cleanup")
	}
	if _, ok := s.legendaryState().nativeDamageSnapshots[2]; !ok {
		t.Fatal("unrelated owner's snapshot removed")
	}
}
