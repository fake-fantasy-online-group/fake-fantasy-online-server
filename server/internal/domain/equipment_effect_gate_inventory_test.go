package domain

import (
	"encoding/json"
	"testing"
)

// Diagnostic inventory, deliberately NOT a behavioral-coverage assertion.
// The independent gate maps these live definitions to executed consumer tests.
func TestEquipmentEffectGateLiveInventory(t *testing.T) {
	data, err := json.Marshal(EquipmentEffectDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	t.Log("EFFECT_GATE_INVENTORY=" + string(data))
}
