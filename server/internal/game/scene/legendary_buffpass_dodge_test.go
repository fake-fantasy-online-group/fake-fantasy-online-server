package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestLegendaryDodgeChanceSourcesExclusionsAndCap(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10229: 1}, "CK-D10", "CK-D05")
	if got := s.legendaryDodgeChanceBP(m, p, combat.Blow{}); got != 1500 {
		t.Fatalf("CK-D10 learned dodge %d want1500", got)
	}
	for _, origin := range []domain.DamageOrigin{domain.DamageDOTOrigin, domain.DamageAuraOrigin} {
		if got := s.legendaryDodgeChanceBP(m, p, combat.Blow{Origin: origin}); got != 0 {
			t.Fatal("periodic/aura damage dodgeable", origin, got)
		}
	}
	if got := s.legendaryDodgeChanceBP(p, p, combat.Blow{}); got != 0 {
		t.Fatal("self damage dodgeable")
	}
	oldA, oldB := legendaryDodgeProviders["CK-D10"], legendaryDodgeProviders["CK-D05"]
	defer func() { legendaryDodgeProviders["CK-D10"], legendaryDodgeProviders["CK-D05"] = oldA, oldB }()
	legendaryDodgeProviders["CK-D10"] = func(*legendaryContext) int32 { return 3000 }
	legendaryDodgeProviders["CK-D05"] = func(*legendaryContext) int32 { return 2000 }
	if got := s.legendaryDodgeChanceBP(m, p, combat.Blow{}); got != 4400 {
		t.Fatalf("independent sources combine as 1-(0.7*0.8): %d", got)
	}
	legendaryDodgeProviders["CK-D10"] = func(*legendaryContext) int32 { return 9000 }
	if got := s.legendaryDodgeChanceBP(m, p, combat.Blow{}); got != legendaryDodgeCapBP {
		t.Fatalf("dodge cap %d", got)
	}
	legendaryDodgeProviders["CK-D10"], legendaryDodgeProviders["CK-D05"] = oldA, oldB
	delete(p.Player.Char.Skills, 10229)
	if got := s.legendaryDodgeChanceBP(m, p, combat.Blow{}); got != 0 {
		t.Fatal("CK-D10 dodge without native skill", got)
	}
}

func TestLegendaryPartyGuardShackledAttackerOnAllies(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{10325: 1}, "YS-D10")
	join(t, s, 2, 102, "ally", 120, 100)
	ally := s.players[2]
	p.Player.Party, ally.Player.Party = 1, 1
	if got := s.legendaryPartyGuardIncoming(m, ally, combat.Blow{}, event.DamageDealt{Src: m.ID, Dst: ally.ID}, 1000); got != 1000 {
		t.Fatal("guard without shackle", got)
	}
	nativeRepairCast(t, s, p, 10325, m.ID)
	if got := s.legendaryPartyGuardIncoming(m, ally, combat.Blow{}, event.DamageDealt{Src: m.ID, Dst: ally.ID}, 1000); got != 750 {
		t.Fatal("shackled attacker damage to ally not reduced 25%", got)
	}
	if got := s.legendaryPartyGuardIncoming(m, p, combat.Blow{}, event.DamageDealt{Src: m.ID, Dst: p.ID}, 1000); got != 1000 {
		t.Fatal("party guard must not apply to the wearer itself", got)
	}
	ally.Player.Party = 2
	if got := s.legendaryPartyGuardIncoming(m, ally, combat.Blow{}, event.DamageDealt{Src: m.ID, Dst: ally.ID}, 1000); got != 1000 {
		t.Fatal("guard leaked outside the party", got)
	}
}
