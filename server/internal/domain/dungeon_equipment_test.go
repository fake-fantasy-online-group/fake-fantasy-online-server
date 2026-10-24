package domain

import (
	"slices"
	"testing"
)

func dungeonEquipmentFixture() map[ItemID]ItemDef {
	items := map[ItemID]ItemDef{}
	for _, w := range dungeonEquipmentManifest.Weapons {
		items[w.BaseID] = ItemDef{ID: w.BaseID, Name: w.Name, Equip: &EquipDef{Affixes: []Affix{{Attr: AttrSTR, Value: 1}}, NativeDropEligible: true}}
	}
	for _, set := range dungeonEquipmentManifest.Sets {
		suit := &EquipmentSuit{ID: set.BaseID, Name: "original set", Bonuses: []EquipmentSuitBonus{{Pieces: 2, Affix: Affix{Attr: AttrSTR, Value: 300}, Probability: 100}}}
		for _, id := range set.Members {
			suit.Groups = append(suit.Groups, []ItemID{id})
			items[id] = ItemDef{ID: id, Name: "member", Equip: &EquipDef{Suit: suit, NativeDropEligible: true}}
		}
	}
	return items
}
func TestDungeonEquipmentRegistrationPreservesDungeonOriginalsAndEnhancesTaskOriginals(t *testing.T) {
	items := dungeonEquipmentFixture()
	if err := RegisterDungeonEquipment(items); err != nil {
		t.Fatal(err)
	}
	weapons, tasks, sets := 0, 0, 0
	for _, w := range dungeonEquipmentManifest.Weapons {
		old := items[w.BaseID]
		if w.TaskOriginal {
			tasks++
			if old.Equip.Need.BeginnerAllowed != w.BeginnerAllowedOverride {
				t.Fatal("beginner exception", w.BaseID)
			}
			if w.HardID != 0 || old.HardDungeonOnly || old.Name != w.Name || !slices.Equal(old.Equip.Affixes, w.Affixes) {
				t.Fatal("task original not enhanced in place", w.BaseID)
			}
			want := ItemID(0)
			if w.MechanismEnabled {
				want = w.BaseID
			}
			if EnhancedWeaponBaseID(w.BaseID) != want {
				t.Fatal("task mechanism tier", w.BaseID)
			}
			continue
		}
		if len(old.Equip.Affixes) != 1 || old.Equip.Affixes[0].Value != 1 || EnhancedWeaponBaseID(w.BaseID) != 0 {
			t.Fatal("ordinary dungeon weapon changed", w.BaseID)
		}
		if w.HardID == 0 {
			t.Fatal("unclassified weapon", w.BaseID)
		}
		weapons++
		hard := items[w.HardID]
		if !hard.HardDungeonOnly || hard.DungeonBaseID != w.BaseID || hard.Name != "真*"+old.Name || NativeEquipmentDropEligible(hard) || EnhancedWeaponBaseID(hard.ID) != w.BaseID {
			t.Fatal("bad hard weapon", w.BaseID)
		}
		if len(hard.Equip.Affixes) != len(w.Affixes) {
			t.Fatal("affix count")
		}
		for i, a := range w.Affixes {
			if hard.Equip.Affixes[i] != a {
				t.Fatal("affix value")
			}
		}
	}
	for _, set := range dungeonEquipmentManifest.Sets {
		for _, id := range set.Members {
			sets++
			old, hard := items[id], items[id+HardDungeonItemOffset]
			if hard.Name != "真*"+old.Name || hard.Equip.Suit.Name != "真*"+old.Equip.Suit.Name {
				t.Fatal("true set names require one literal star")
			}
			if old.Equip.Suit.EnhancedClass != 0 || old.Equip.Suit.ID != set.BaseID || hard.Equip.Suit.EnhancedClass != set.BaseID || hard.Equip.Suit.ID != set.HardID {
				t.Fatal("set version isolation")
			}
			if len(old.Equip.Suit.Bonuses) != 4 || old.Equip.Suit.Bonuses[0].Pieces != 2 || hard.Equip.Suit.Bonuses[0].Affix.Value != 300 {
				t.Fatal("set bonus lost")
			}
			for _, g := range hard.Equip.Suit.Groups {
				for _, member := range g {
					if !items[member].HardDungeonOnly {
						t.Fatal("mixed set membership")
					}
				}
			}
		}
	}
	if weapons != 42 || tasks != 18 || sets != 47 {
		t.Fatal("coverage", weapons, tasks, sets)
	}
	if err := RegisterDungeonEquipment(items); err == nil {
		t.Fatal("repeated registration must fail before mutation")
	}
}
func TestDungeonEquipmentExactProvenance(t *testing.T) {
	for _, source := range dungeonEquipmentManifest.Sources {
		if got := HardDungeonDropVariant(source.BaseID, source.MapID, source.MonsterID); got != HardDungeonItemOffset+source.BaseID {
			t.Fatal("source missing")
		}
		if HardDungeonDropVariant(source.BaseID, -source.MapID, source.MonsterID) != 0 || HardDungeonDropVariant(source.BaseID, source.MapID, -source.MonsterID) != 0 {
			t.Fatal("source not exact")
		}
	}
	if len(dungeonEquipmentManifest.Sources) != 467 {
		t.Fatal("unexpected source coverage", len(dungeonEquipmentManifest.Sources))
	}
}

func TestRiftPoolRejectsHardSourceTagRegardlessOfGenericFlags(t *testing.T) {
	id := HardDungeonItemOffset + 1321
	items := map[ItemID]ItemDef{id: {ID: id, HardDungeonOnly: true, Equip: &EquipDef{NativeDropEligible: true, LevelReq: 20}}}
	rules := &RiftRules{Version: 7, EquipmentPools: []RiftEquipmentPool{{MonsterLevel: 20, SourceMonsters: []MonsterID{3049}, Items: []ItemID{id}}}}
	if err := rules.validateEquipmentPools(items); err == nil {
		t.Fatal("hard-only item accepted by explicit rift pool")
	}
	if NativeEquipmentDropEligible(items[id]) {
		t.Fatal("generic flag overrode hard-only source tag")
	}
}

func TestDungeonEquipmentIconsUseNativeOverrideAndRemainStable(t *testing.T) {
	items := dungeonEquipmentFixture()
	for id, def := range items {
		def.Icon = 123
		items[id] = def
	}
	if err := RegisterDungeonEquipment(items); err != nil {
		t.Fatal(err)
	}
	existing := []ItemIconMapping{{Item: 1321, Icon: 999}}
	got, err := DungeonEquipmentIconMappings(existing, items)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 90 || len(existing) != 1 {
		t.Fatal("mapping count or mutated source")
	}
	for _, m := range got {
		if m.Item == HardDungeonItemOffset+1321 && m.Icon != 999 {
			t.Fatal("native override lost")
		}
	}
	again, err := DungeonEquipmentIconMappings(got, items)
	if err != nil || len(again) != len(got) {
		t.Fatal("duplicate icon mappings", err)
	}
}

func TestDungeonAdditionalAncientDropsExactRateAndScope(t *testing.T) {
	seen := map[[3]int32]bool{}
	for _, d := range dungeonEquipmentManifest.AddedDrops {
		key := [3]int32{int32(d.BaseID), d.MapID, int32(d.MonsterID)}
		if seen[key] || d.RatePct != 0.12 {
			t.Fatal("duplicate or wrong rate", d)
		}
		seen[key] = true
		switch d.BaseID {
		case 1326, 1626, 1826, 1928:
			t.Fatal("duplicated existing ancient", d)
		}
		if HardDungeonDropVariant(d.BaseID, d.MapID, d.MonsterID) != HardDungeonItemOffset+d.BaseID {
			t.Fatal("missing hard provenance")
		}
	}
	if len(seen) != 56 {
		t.Fatal("new ancient coverage", len(seen))
	}
	if len(AdditionalLostTempleDrops(20901, 3111)) != 8 || len(AdditionalLostTempleDrops(20902, 3114)) != 0 || len(AdditionalLostTempleDrops(23800, 3111)) != 0 {
		t.Fatal("scope mismatch")
	}
}
