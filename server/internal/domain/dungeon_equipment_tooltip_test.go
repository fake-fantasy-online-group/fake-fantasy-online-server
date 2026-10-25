package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDungeonWeaponNamedEffectsPreserveRegistrationAndExactScope(t *testing.T) {
	items := dungeonEquipmentFixture()
	for id, def := range items {
		def.Description = "原有来历。"
		items[id] = def
	}
	before := dungeonEquipmentFixture()
	if err := RegisterDungeonEquipment(items); err != nil {
		t.Fatal(err)
	}
	active, effects, inert := 0, 0, 0
	for _, w := range dungeonEquipmentManifest.Weapons {
		id := w.BaseID
		if w.HardID != 0 {
			id = w.HardID
			if got := DungeonWeaponEffectLines(w.BaseID); len(got) != 0 {
				t.Fatalf("ordinary original %d gained effects: %q", w.BaseID, got)
			}
			if !reflect.DeepEqual(items[w.BaseID].Equip, before[w.BaseID].Equip) || items[w.BaseID].Description != "原有来历。" {
				t.Fatalf("ordinary original %d changed", w.BaseID)
			}
			if items[id].Name != "真*"+w.Name || strings.Count(items[id].Name, "*") != 1 {
				t.Fatalf("true-version name changed: %q", items[id].Name)
			}
		}
		lines := DungeonWeaponEffectLines(id)
		if !w.MechanismEnabled {
			inert++
			if len(lines) != 0 || len(w.EffectNames) != 0 || !strings.Contains(items[id].Description, "无技能特效。") {
				t.Fatalf("no-effect task %d gained a named effect", id)
			}
			continue
		}
		active++
		effects += len(lines)
		if items[id].Description != "原有来历。" || len(lines) == 0 {
			t.Fatalf("mechanics mixed into lore or missing on %d: %q", id, items[id].Description)
		}
		var single string
		byClass := map[string]string{}
		if json.Unmarshal(w.Effect, &single) == nil {
			byClass[""] = single
		} else if err := json.Unmarshal(w.Effect, &byClass); err != nil {
			t.Fatal(err)
		}
		if len(w.EffectNames) != len(byClass) || len(lines) != len(byClass) {
			t.Fatalf("effect names/descriptions mismatch for %d", id)
		}
		for class, description := range byClass {
			name := w.EffectNames[class]
			if name == "" {
				t.Fatalf("missing or invented skill name for %d/%s: %q", id, class, name)
			}
			label := name
			if class != "" {
				label += "（" + class + "）"
			}
			want := label + "：" + strings.TrimPrefix(strings.TrimPrefix(description, name), "的")
			found := false
			for _, line := range lines {
				if line == want {
					found = true
				}
			}
			if !found {
				t.Fatalf("named effect changed its authored meaning for %d: want %q, got %q", id, want, lines)
			}
		}
	}
	if active != 48 || effects != 56 || inert != 12 || len(DungeonWeaponEffectLines(999999)) != 0 {
		t.Fatalf("coverage changed: active=%d effects=%d inert=%d", active, effects, inert)
	}
}

func TestDungeonWeaponNamedEffectOnlyRemovesAdjacentSubject(t *testing.T) {
	for _, tc := range []struct{ name, class, text, want string }{
		{"罡气剑", "", "罡气剑不再击退敌人。", "罡气剑：不再击退敌人。"},
		{"饮血剑", "", "饮血剑的生命恢复量提高60%。", "饮血剑：生命恢复量提高60%。"},
		{"光盾术", "药师", "对仍有自己光盾的队友再次施放光盾术时，消耗剩余护盾。", "光盾术（药师）：对仍有自己光盾的队友再次施放光盾术时，消耗剩余护盾。"},
		{"秘术：消除", "术士", "秘术：消除改为夺取防御魔法。", "秘术：消除（术士）：改为夺取防御魔法。"},
	} {
		if got := dungeonWeaponNamedEffect(tc.name, tc.class, tc.text); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
	}
}
