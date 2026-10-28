package scene

import (
	"bytes"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"os"
	"path/filepath"
	"testing"
)

func TestSpiritSharedWireFixtures(t *testing.T) {
	d, _ := domain.SpiritDefinitionForKind(9)
	state, err := domain.UnsealSpirit(d.Kind, d.Weights(), false, func(n int) int { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	state, err = state.Process(func(n int) int { return n - 1 })
	if err != nil {
		t.Fatal(err)
	}
	fixtures := map[string]domain.EndgameResponse{"hello": {Kind: "hello", RequestID: "spirit-hello-0001", OK: true, Capabilities: []string{domain.EndgameV1, domain.EndgameSpiritV1}, Spirits: domain.SpiritDefinitionViews(), SpiritAppearanceIDs: domain.SpiritAppearanceIDs()}, "preview": {Kind: "spirit.preview", RequestID: "spirit-preview-0001", OK: true, Revision: "0", Spirit: domain.NewSpiritView(d.ItemID, state)}, "success": {Kind: "spirit.result", RequestID: "spirit-process-0001", OK: true, Revision: "1", Spirit: domain.NewSpiritView(d.ItemID, state), Message: "灵加工完成"}}
	for name, v := range fixtures {
		raw, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		expected, readErr := os.ReadFile(filepath.Join("testdata", "spirit", name+".json"))
		if readErr != nil || !bytes.Equal(append(raw, '\n'), expected) {
			t.Fatalf("fixture %s differs: %v", name, readErr)
		}
		var r domain.EndgameResponse
		if err = json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		if dir := os.Getenv("SPIRIT_WIRE_FIXTURE_DIR"); dir != "" {
			if err = os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, name+".json"), append(raw, '\n'), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
}
