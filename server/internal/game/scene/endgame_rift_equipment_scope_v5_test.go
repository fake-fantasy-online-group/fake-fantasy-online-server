package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
	"testing"
)

// Old rule snapshots retain their source-selection policy, but newly generated
// rewards obey the current preascension ceiling and unique socket restriction.
func TestV5RiftPendingLegacyRunExcludesAscensionAndOver60(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	for id, d := range s.items {
		if d.Equip != nil {
			delete(s.items, id)
		}
	}
	add := func(id domain.ItemID, lv int32, ascended bool) {
		s.items[id] = domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{NativeDropEligible: true, LevelReq: lv, AscensionOnly: ascended, Need: domain.Requirement{Professions: 1, ProfessionRestricted: true}}}
	}
	add(1610, 54, false)
	add(15036, 66, false)
	add(15037, 60, true)
	r := s.rift.data.Run
	r.Tier = 10
	r.Timely = true
	r.Rules.NormalBossEquipment = 10
	for char, m := range r.Members {
		m.Level = 100
		m.Eligible = true
		r.Members[char] = m
		s.rift.data.Profiles[char] = endgameRiftIdentity{Race: domain.Warrior, Gender: 1, Employed: true}
	}
	s.generateEndgameRiftGrants()
	for _, grant := range s.rift.data.Grants {
		count := 0
		for _, st := range grant.Items {
			if st.InstanceKind != domain.ItemInstanceEquipment {
				continue
			}
			count++
			if st.Item != 1610 {
				t.Fatal("pending legacy run emitted prohibited equipment", st.Item)
			}
			if st.Endgame.UniqueID != 0 && st.SocketCount != 0 {
				t.Fatal("new unique contains sockets")
			}
		}
		plan := domain.PlanRiftRewards(r.Tier, r.Timely, r.Rules)
		if count != int(plan.Equipment+plan.NormalBossEquipment) {
			t.Fatal("reward count changed", count)
		}
	}
}
