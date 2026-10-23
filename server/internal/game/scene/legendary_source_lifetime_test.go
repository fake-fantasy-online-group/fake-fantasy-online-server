package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestLegendaryForeignPermanentShieldSceneLifetime(t *testing.T) {
	for _, leaves := range []string{"source", "recipient"} {
		t.Run(leaves, func(t *testing.T) {
			s, source, _ := legendaryMeleeFixture(t, "ZS-A04")
			join(t, s, 2, 200, "recipient", 110, 100)
			target := s.players[2]
			source.Status = domain.NewStatusSet()
			target.Status = domain.NewStatusSet()
			permanent := domain.StatusDef{ID: 90001, Level: 1, Permanent: true, DamageShield: 100, LegendaryPowerID: "JK-D08", LegendarySourceUID: 991}
			target.Status.Apply(permanent, s.tick, source.ID)
			source.Status.Apply(permanent, s.tick, source.ID)
			timed := permanent
			timed.ID = 90002
			timed.Permanent = false
			timed.DurationSec = 30
			target.Status.Apply(timed, s.tick, source.ID)
			native := permanent
			native.ID = 90003
			native.LegendaryPowerID = ""
			native.LegendarySourceUID = 0
			target.Status.Apply(native, s.tick, source.ID)
			if leaves == "source" {
				s.revokeLegendaryForeignShields(source)
			} else {
				s.revokeLegendaryForeignShields(target)
			}
			kept := map[domain.StatusID]bool{}
			target.Status.Each(func(a domain.Active) { kept[a.Def.ID] = true })
			if kept[90001] || !kept[90002] || !kept[90003] {
				t.Fatalf("wrong status lifetime %+v", kept)
			}
			self := false
			source.Status.Each(func(a domain.Active) {
				if a.Def.ID == 90001 {
					self = true
				}
			})
			if !self {
				t.Fatal("source self shield lost during scene boundary")
			}
			runtime := source.CapturePlayerRuntime(s.tick)
			if runtime.Status == nil {
				t.Fatal("self state excluded from transfer")
			}
		})
	}
}
