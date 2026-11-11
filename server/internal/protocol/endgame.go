package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

const OpEndgame = 0x10d0
const SCEndgame = 0x80d0

func EncodeEndgame(v domain.EndgameResponse) []byte {
	b, err := json.Marshal(v)
	if err != nil || len(b) > domain.EndgameMaxJSONBytes {
		return nil
	}
	return NewW(SCEndgame).U16(domain.EndgameSchema).Str(string(b)).Bytes()
}

// DecodeEndgame accepts a closed schema, with no duplicate JSON object keys or
// optional native-packet tails. IDs remain strings to preserve all 63 bits.
func DecodeEndgame(payload []byte) (v domain.EndgameRequest, ok bool) {
	r := newReader(payload)
	if r.u16() != domain.EndgameSchema {
		return v, false
	}
	raw := r.str()
	if !r.done() || len(raw) > domain.EndgameMaxJSONBytes || !utf8.ValidString(raw) {
		return v, false
	}
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.UseNumber()
	if err := endgameJSONValue(d, 0); err != nil {
		return v, false
	}
	if _, err := d.Token(); err != io.EOF {
		return v, false
	}
	// encoding/json matches struct fields case-insensitively; this explicit key
	// set rejects aliases such as Op/OP as well as genuinely unknown fields.
	var keys map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &keys) != nil {
		return v, false
	}
	allowed := map[string]bool{"powerId": true, "skillId": true, "op": true, "requestId": true, "expectedRevision": true, "capabilities": true, "catalog": true, "cursor": true, "targetUid": true, "runeUid": true, "socket": true, "recipeId": true, "quoteId": true, "runId": true, "tier": true, "floorIndex": true, "itemUids": true, "npcEntityId": true, "cubeInputs": true, "spiritItem": true, "spiritMaterial": true}
	for key := range keys {
		if !allowed[key] {
			return v, false
		}
	}
	if selected, exists := keys["cubeInputs"]; exists {
		var rows []map[string]json.RawMessage
		if json.Unmarshal(selected, &rows) != nil {
			return v, false
		}
		for _, row := range rows {
			for key := range row {
				if key != "uid" && key != "itemId" && key != "count" {
					return v, false
				}
			}
		}
	}
	// Spirit selections are the native bag address; reject aliases and extra keys.
	for _, name := range []string{"spiritItem", "spiritMaterial"} {
		if selected, exists := keys[name]; exists {
			var row map[string]json.RawMessage
			if json.Unmarshal(selected, &row) != nil || row == nil {
				return v, false
			}
			for key := range row {
				if key != "tab" && key != "slot" && key != "itemId" && key != "uid" {
					return v, false
				}
			}
		}
	}
	d = json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	if d.Decode(&v) != nil || !validEndgameRequest(v) {
		return v, false
	}
	return v, true
}
func endgameJSONValue(d *json.Decoder, depth int) error {
	if depth > 12 {
		return fmt.Errorf("JSON nesting")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, container := t.(json.Delim)
	if !container {
		return nil
	}
	switch delim {
	case '{':
		keys := map[string]bool{}
		for d.More() {
			t, err = d.Token()
			if err != nil {
				return err
			}
			k, ok := t.(string)
			if !ok || keys[k] {
				return fmt.Errorf("duplicate key")
			}
			keys[k] = true
			if err = endgameJSONValue(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err = endgameJSONValue(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected delimiter")
	}
	_, err = d.Token()
	return err
}
func endgameASCII(s string, min, max int) bool {
	if len(s) < min || len(s) > max {
		return false
	}
	for _, c := range s {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
func endgameDecimal(s string, zero bool) bool {
	n, e := strconv.ParseInt(s, 10, 64)
	return e == nil && (n > 0 || zero && n == 0) && strconv.FormatInt(n, 10) == s
}
func validEndgameRequest(v domain.EndgameRequest) bool {
	if v.NPCEntityID != 0 && v.Op != "rift.start" && v.Op != "cube.quote" && v.Op != "cube.execute" && v.Op != "cube.recover" && v.Op != "powerbook.recover" && !spiritWorkbenchOp(v.Op) {
		return false
	}
	switch v.Op {
	case "spirit.open", "spirit.preview", "spirit.unseal", "spirit.process", "spirit.rollback", "spirit.reset", "spirit.fuse", "spirit.separate", "spirit.appearance":
		if !validSpiritRequest(v) {
			return false
		}
	case "powerbook.list", "powerbook.extract", "powerbook.apply", "powerbook.recover", "powerbook.imprint", "skill.ranks", "skill.upgrade", "hello", "capabilities", "snapshot", "catalog", "socket.insert", "socket.remove", "cube.quote", "cube.execute", "cube.recover", "rift.start", "rift.state", "rift.claim", "rift.leave", "rift.advance":
	default:
		return false
	}
	if !spiritOp(v.Op) && (v.SpiritItem != nil || v.SpiritMaterial != nil) {
		return false
	}
	if v.PowerID != "" && ((v.Op != "powerbook.imprint" && v.Op != "powerbook.extract") || !endgameASCII(v.PowerID, 1, 32)) {
		return false
	}
	if v.Op == "powerbook.imprint" && (v.PowerID == "" || v.TargetUID == "" || v.RuneUID != "" || v.RecipeID != 0 || v.QuoteID != "" || len(v.ItemUIDs) != 0 || len(v.CubeInputs) != 0 || v.Socket != 0 || v.Tier != 0 || v.RunID != "" || v.Cursor != "" || v.Catalog != "" || v.FloorIndex != 0 || len(v.Capabilities) != 0) {
		return false
	}
	if v.Op == "powerbook.extract" || v.Op == "powerbook.apply" || v.Op == "powerbook.recover" {
		if v.RecipeID != 0 || v.QuoteID != "" || len(v.ItemUIDs) != 0 || len(v.CubeInputs) != 0 || v.Socket != 0 || v.Tier != 0 || v.RunID != "" || v.Cursor != "" || v.Catalog != "" || v.FloorIndex != 0 || len(v.Capabilities) != 0 {
			return false
		}
		switch v.Op {
		case "powerbook.extract":
			if v.PowerID == "" || v.TargetUID != "" || v.RuneUID != "" || v.NPCEntityID != 0 {
				return false
			}
		case "powerbook.apply":
			if v.PowerID != "" || v.TargetUID == "" || v.RuneUID == "" || v.NPCEntityID != 0 {
				return false
			}
		case "powerbook.recover":
			if v.PowerID != "" || v.TargetUID != "" || v.RuneUID != "" || v.NPCEntityID <= 0 {
				return false
			}
		}
	}
	if v.Op == "skill.upgrade" && v.SkillID <= 0 {
		return false
	}
	if v.Op != "skill.upgrade" && v.SkillID != 0 {
		return false
	}
	if v.Mutates() && (!endgameASCII(v.RequestID, 16, 64) || !endgameDecimal(v.ExpectedRevision, true)) {
		return false
	}
	if v.RequestID != "" && !endgameASCII(v.RequestID, 1, 64) {
		return false
	}
	if v.ExpectedRevision != "" && !endgameDecimal(v.ExpectedRevision, true) {
		return false
	}
	if v.TargetUID != "" && !endgameDecimal(v.TargetUID, false) {
		return false
	}
	if v.RuneUID != "" && !endgameDecimal(v.RuneUID, false) {
		return false
	}
	if len(v.CubeInputs) > domain.CubeInputCapacity || len(v.CubeInputs) > 0 && (v.Op != "cube.quote" || v.RecipeID != 0 || v.TargetUID != "" || v.RuneUID != "") {
		return false
	}
	for _, input := range v.CubeInputs {
		if input.Count <= 0 || input.UID == "" && input.ItemID <= 0 || input.UID != "" && (!endgameDecimal(input.UID, false) || input.ItemID != 0 || input.Count != 1) {
			return false
		}
	}
	// Preserve the legacy budget, adding one slot per distinct supported
	// extension. Published clients advertise both codex and spirit in hello.
	maxCapabilities := 8
	codex, spirit := false, false
	for _, c := range v.Capabilities {
		if c == domain.EndgamePowerCodexV1 && !codex {
			codex = true
			maxCapabilities++
		}
		if c == domain.EndgameSpiritV1 && !spirit {
			spirit = true
			maxCapabilities++
		}
	}
	if len(v.ItemUIDs) > 10 || len(v.Capabilities) > maxCapabilities {
		return false
	}
	for _, id := range v.ItemUIDs {
		if !endgameDecimal(id, false) {
			return false
		}
	}
	for _, c := range v.Capabilities {
		if !endgameASCII(c, 1, 32) {
			return false
		}
	}
	for _, s := range []string{v.Catalog, v.Cursor, v.QuoteID, v.RunID} {
		if s != "" && !endgameASCII(s, 1, 64) {
			return false
		}
	}
	if v.Op == "rift.advance" && (v.RunID == "" || v.FloorIndex < 1) {
		return false
	}
	return v.FloorIndex >= 0 && v.Socket >= 0 && v.Socket < 5 && v.RecipeID >= 0 && v.Tier >= 0 && v.Tier <= domain.RiftMaxTier
}

func spiritOp(op string) bool { return len(op) > 7 && op[:7] == "spirit." }

// spiritWorkbenchOp lists the operations the native spirit panel sends with
// the fairy NPC entity; fuse/separate/appearance never carry an NPC.
func spiritWorkbenchOp(op string) bool {
	switch op {
	case "spirit.open", "spirit.preview", "spirit.unseal", "spirit.process", "spirit.rollback", "spirit.reset":
		return true
	}
	return false
}

func validSpiritSelection(s *domain.SpiritSelection) bool {
	return s != nil && s.Tab >= 0 && s.Tab < domain.BagTabCount && s.Slot >= 0 && s.Slot < domain.BagPageSlots && s.ItemID > 0 &&
		(s.UID == "" || endgameDecimal(s.UID, false))
}

// validSpiritRequest mirrors the native panel request shapes.
func validSpiritRequest(v domain.EndgameRequest) bool {
	if v.RuneUID != "" || v.Socket != 0 || v.RecipeID != 0 || v.QuoteID != "" || v.RunID != "" || v.Tier != 0 || len(v.ItemUIDs) != 0 ||
		v.Catalog != "" || v.Cursor != "" || v.FloorIndex != 0 || len(v.CubeInputs) != 0 || v.PowerID != "" || v.SkillID != 0 || len(v.Capabilities) != 0 {
		return false
	}
	if spiritWorkbenchOp(v.Op) {
		if v.NPCEntityID == 0 || v.TargetUID != "" {
			return false
		}
		switch v.Op {
		case "spirit.open":
			return v.SpiritItem == nil && v.SpiritMaterial == nil
		case "spirit.preview":
			return validSpiritSelection(v.SpiritItem) && v.SpiritMaterial == nil
		}
		return validSpiritSelection(v.SpiritItem) && validSpiritSelection(v.SpiritMaterial) &&
			(v.SpiritItem.Tab != v.SpiritMaterial.Tab || v.SpiritItem.Slot != v.SpiritMaterial.Slot)
	}
	// fuse / separate / appearance: a bag tool on a UID-guarded target, no NPC.
	return v.NPCEntityID == 0 && v.SpiritMaterial == nil && validSpiritSelection(v.SpiritItem) && v.TargetUID != "" &&
		(v.SpiritItem.UID == "" || v.SpiritItem.UID != v.TargetUID)
}
