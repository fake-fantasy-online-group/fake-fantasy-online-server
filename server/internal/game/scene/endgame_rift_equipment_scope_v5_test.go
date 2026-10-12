package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestV5RiftOrdinaryHighLevelPoolIsNotUniqueWhitelist(t *testing.T) {
	for _, hasUnique := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary_fallback_no_unique", true: "guaranteed_slot_and_ordinary_slot"}[hasUnique], func(t *testing.T) {
			s, b, now := riftFixture(t)
			activateTestRift(t, s, b, *now)
			for _, u := range domain.UniqueCatalog() {
				delete(s.items, u.BaseID)
			}
			//15036 is original66-level火纹金戟, a native-drop base outside259.
			id := domain.ItemID(15036)
			if _, ok := domain.UniqueForBase(id); ok {
				t.Fatal("ordinary test base unexpectedly promoted")
			}
			def := domain.ItemDef{ID: id, Name: "火纹金戟", InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: 13, LevelReq: 66, ResourceLevel: 11, Type: 1, Durable: 100, Need: domain.Requirement{Level: 66, Professions: 1, ProfessionRestricted: true}}}
			if !domain.NativeEquipmentDropEligible(def) {
				t.Fatal("original ordinary type pool lost66-level base")
			}
			s.items[id] = def
			blocked := def
			blocked.ID = 15037
			copyEquip := *def.Equip
			copyEquip.NoTypeDrop = true
			copyEquip.LevelReq = 70
			blocked.Equip = &copyEquip
			s.items[blocked.ID] = blocked
			// Ineligible high-level unique candidates must not win either wearer filter.
			addUnique := func(id domain.ItemID, level int32, professions, sex uint8) {
				u := def
				u.ID = id
				e := *def.Equip
				e.LevelReq = level
				e.Need = domain.Requirement{Level: level, Professions: professions, ProfessionRestricted: true, Sex: sex}
				u.Equip = &e
				s.items[id] = u
			}
			addUnique(2053, 59, 2, 0) // Swordman's head is invalid for this warrior.
			addUnique(2902, 48, 1, 2) // Female clothing is invalid for this male wearer.
			if hasUnique {
				addUnique(1610, 54, 1, 0)
				addUnique(1609, 48, 1, 0)
			}
			s.equipmentRoll = domain.EquipmentRollTable{Counts: map[domain.ColorProfile][]domain.WeightedAffixCount{domain.DefaultColorProfile: {{Count: 1, Weight: 1}}}, Options: map[domain.EquipmentRollKey][]domain.EquipmentRollOption{{Level: 11, Type: 1}: {{CardID: 1, Affix: domain.Affix{Attr: domain.AttrMaxHP, Value: 100}, Weight: 1}}}}
			s.rift.data.Run.Tier = 10
			s.rift.data.Run.Timely = true
			for char, m := range s.rift.data.Run.Members {
				m.Level = 100
				m.Eligible = true
				s.rift.data.Run.Members[char] = m
				s.rift.data.Profiles[char] = endgameRiftIdentity{Race: domain.Warrior, Gender: 1, Employed: true}
			}
			s.generateEndgameRiftGrants()
			if len(s.rift.data.Grants) != 1 {
				t.Fatal("missing reward grant")
			}
			for _, grant := range s.rift.data.Grants {
				count := 0
				for _, item := range grant.Items {
					if item.InstanceKind != domain.ItemInstanceEquipment {
						continue
					}
					count++
					if hasUnique && count == 1 {
						if item.Item != 1610 || item.Endgame.UniqueID != 1610 || item.Endgame.UniqueContentVersion != domain.UniqueContentVersion {
							t.Fatal("guaranteed slot failed legal259 highest-base selection", item)
						}
					} else if item.Item != id || item.Endgame.UniqueID != 0 || item.Endgame.UniqueContentVersion != 0 {
						t.Fatal("ordinary high-level pool clipped or illegally promoted", item)
					}
				}
				if count != int(domain.PlanRiftRewards(10, true).Equipment) {
					t.Fatal("equipment count changed", count)
				}
			}
		})
	}
}
