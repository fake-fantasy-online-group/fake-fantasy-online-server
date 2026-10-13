package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestNativeStatusSourceIconsPreserved(t *testing.T) {
	for _, tc := range []struct {
		id          domain.StatusID
		icon, skill int32
	}{
		{domain.StatusWeaken, 7069, 10324}, {1014, 5038, 10307}, {1037, 7084, 10425}, {1038, 7093, 10435},
	} {
		def := domain.StatusDef{ID: tc.id, Name: "native", DurationSec: 30}
		applyStatusSource(&def, tc.icon, tc.skill)
		if def.Icon != tc.icon || def.SourceSkillID != tc.skill {
			t.Fatalf("source icon lost: %+v", def)
		}
		applyStatusSource(&def, 9999, 9999)
		if def.Icon != tc.icon || def.SourceSkillID != tc.skill {
			t.Fatalf("existing icon overwritten: %+v", def)
		}
	}
}

func TestTriggeredStatusCarriesActualOriginIcon(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "source", 100, 100)
	join(t, s, 2, 200, "target", 120, 100)
	source, target := s.players[1], s.players[2]
	for _, tc := range []struct {
		carrier domain.StatusDef
		result  domain.StatusID
	}{
		{domain.StatusDef{ID: 1018, Level: 1, DurationSec: 30, Icon: 5030, SourceSkillID: 10211, PoisonOnHitChance: 100, PoisonOnHitLevel: 1}, domain.StatusPoison},
		{domain.StatusDef{ID: 1038, Level: 1, DurationSec: 30, Icon: 7093, SourceSkillID: 10435, FreezeAttackerChance: 100}, domain.StatusFreeze},
	} {
		source.Status = domain.NewStatusSet()
		target.Status = domain.NewStatusSet()
		source.Status.Apply(tc.carrier, s.tick, source.ID)
		s.statuses = statusTable(domain.StatusDef{ID: tc.result, Level: 1, Name: "secondary", DurationSec: 10})
		if !s.applyStatus(target, tc.result, 1, source.ID) {
			t.Fatal("status not applied")
		}
		statuses := s.visibleStatuses(target)
		if len(statuses) != 1 {
			t.Fatalf("unexpected rows %+v", statuses)
		}
		st := statuses[0]
		if st.buff.Icon != tc.carrier.Icon || st.icon.Icon != tc.carrier.Icon || st.buff.SkillID != tc.carrier.SourceSkillID || st.icon.Status != int32(tc.result) {
			t.Fatalf("identity/resource/source conflated: %+v", st)
		}
		if st.buff.RemainSec != 10 {
			t.Fatalf("presentation changed duration: %+v", st)
		}
	}
}

func TestAllEquipmentAuraIconsReachBothSnapshotChannels(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "source", 100, 100)
	p := s.players[1]
	expected := map[string]int32{"white_tiger_guard": 5017, "ember_shelter": 7084, "holy_feather": 7006, "empty_talisman_ward": 5062, "north_gate_ice": 7093, "backward_flame": 7085, "paper_donkey_frost": 7086}
	for id, icon := range expected {
		cacheTestAura(s, p, 991, 1200)
		c := s.equipmentState().cache[p.ID]
		c.effects[0].Effect.Aura.ID = id
		s.equipmentState().cache[p.ID] = c
		s.stepEquipmentAuras()
		found := false
		for _, st := range s.equipmentAuraStatuses(p) {
			if st.buff.SkillID != equipmentAuraStatusID(id) {
				continue
			}
			found = true
			if st.buff.Icon != icon || st.icon.Icon != icon || st.icon.Status != st.buff.SkillID {
				t.Fatalf("%s wrong icon/identity %+v", id, st)
			}
			if st.buff.RemainSec != -2 || st.icon.RemainMS != -1 {
				t.Fatalf("%s gained a countdown", id)
			}
		}
		if !found {
			t.Fatalf("missing aura %s", id)
		}
	}
	if equipmentAuraIconID("unknown") != 0 {
		t.Fatal("unknown aura must not get a guessed icon")
	}
}
