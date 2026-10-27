package domain

import (
	"encoding/json"
	"fmt"
)

// LegendaryPowerBook belongs to one character, never an account or an item.
// One coherent roll is retained per power; records do not mutate owned items.
type LegendaryPowerBook map[string]int32

func ReadLegendaryPowerBook(payload string) (LegendaryPowerBook, error) {
	if payload == "" {
		payload = "{}"
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &all); err != nil || all == nil {
		return nil, fmt.Errorf("invalid character progress payload")
	}
	book := LegendaryPowerBook{}
	if raw, ok := all["powerBook"]; ok {
		if err := json.Unmarshal(raw, &book); err != nil || book == nil {
			return nil, fmt.Errorf("invalid power book")
		}
	}
	if len(book) > len(legendaryPowerDefinitions) {
		return nil, fmt.Errorf("oversized power book")
	}
	for id, roll := range book {
		if _, ok := LegendaryPower(id); !ok || !LegendaryPowerRollValid(id, roll) {
			return nil, fmt.Errorf("invalid power book entry %s", id)
		}
	}
	return book, nil
}

func WriteLegendaryPowerBook(payload string, book LegendaryPowerBook) (string, error) {
	if _, err := ReadLegendaryPowerBook(payload); err != nil {
		return "", err
	}
	if payload == "" {
		payload = "{}"
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &all); err != nil {
		return "", err
	}
	raw, err := json.Marshal(book)
	if err != nil {
		return "", err
	}
	all["powerBook"] = raw
	raw, err = json.Marshal(all)
	if err != nil {
		return "", err
	}
	if _, err := ReadLegendaryPowerBook(string(raw)); err != nil {
		return "", err
	}
	return string(raw), nil
}

// RecordSalvagedPower accepts only the authoritative consumed instance, not a
// client supplied ID/value. Call it in the same transaction as destruction.
func (book LegendaryPowerBook) RecordSalvagedPower(st Stack) (bool, error) {
	if st.Legendary.PowerID == "" {
		return false, nil
	}
	if book == nil || st.UID <= 0 || st.Count != 1 || st.Legendary.Validate() != nil {
		return false, fmt.Errorf("invalid salvaged legendary")
	}
	id, roll := st.Legendary.PowerID, st.Legendary.PowerRollBP
	old, exists := book[id]
	if exists && EffectiveLegendaryPowerRollBP(id, old) >= EffectiveLegendaryPowerRollBP(id, roll) {
		return false, nil
	}
	book[id] = roll
	return true, nil
}

// ImprintPower changes exactly the recorded power and its coherent numeric
// roll. Native affixes, rarity, refinement, provenance, binding and UID survive.
func (book LegendaryPowerBook) ImprintPower(st Stack, def ItemDef, ch *Character, powerID string) (Stack, error) {
	roll, unlocked := book[powerID]
	power, exists := LegendaryPower(powerID)
	if !unlocked || !exists || !LegendaryPowerRollValid(powerID, roll) || !LegendaryPowerExecutable(powerID) {
		return st, cubeError("power_locked", "此威能尚未收入威能录")
	}
	if st.Locked {
		return st, cubeError("locked", "请先解除装备锁定")
	}
	if ch == nil || st.UID <= 0 || st.Count != 1 || st.Item != def.ID || def.Equip == nil || !def.LegendaryEnabled || st.Legendary.PowerID == "" || st.Legendary.Validate() != nil || st.Endgame.UniqueID != 0 || st.WingPowers != (WingPowers{}) || st.RolledAffixCount != 0 || st.SocketCount != 0 || st.InstanceKind != ItemInstanceEquipment && st.InstanceKind != ItemInstanceNone {
		return st, cubeError("requirements", "请选择背包中可刻印威能的暗金装备")
	}
	if !LegendaryClassForCharacter(ch).allows(power) {
		return st, cubeError("power_class", "此威能不属于当前职业")
	}
	if !LegendaryPowerEligibleForItem(power, def, st) {
		return st, cubeError("power_target", "此装备的部位、职业或来源等级不合此威能")
	}
	if st.Legendary.PowerID == powerID && EffectiveLegendaryPowerRollBP(powerID, st.Legendary.PowerRollBP) >= EffectiveLegendaryPowerRollBP(powerID, roll) {
		return st, cubeError("unchanged", "此装备已具有同等威能，无需再刻")
	}
	st.Legendary.PowerID = powerID
	st.Legendary.PowerRollBP = roll
	if LegendaryPowerHasRoll(powerID) {
		st.Legendary.PowerRollBP = EffectiveLegendaryPowerRollBP(powerID, roll)
	}
	return st, nil
}
