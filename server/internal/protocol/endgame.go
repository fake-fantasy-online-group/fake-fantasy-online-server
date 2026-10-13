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
	allowed := map[string]bool{"skillId": true, "op": true, "requestId": true, "expectedRevision": true, "capabilities": true, "catalog": true, "cursor": true, "targetUid": true, "runeUid": true, "socket": true, "recipeId": true, "quoteId": true, "runId": true, "tier": true, "floorIndex": true, "itemUids": true, "npcEntityId": true, "cubeInputs": true}
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
	if v.NPCEntityID != 0 && v.Op != "rift.start" && v.Op != "cube.quote" && v.Op != "cube.execute" {
		return false
	}
	switch v.Op {
	case "skill.ranks", "skill.upgrade", "hello", "snapshot", "catalog", "socket.insert", "socket.remove", "cube.quote", "cube.execute", "rift.start", "rift.state", "rift.claim", "rift.leave", "rift.advance":
	default:
		return false
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
	if len(v.CubeInputs) > 12 || len(v.CubeInputs) > 0 && (v.Op != "cube.quote" || v.RecipeID != 0 || v.TargetUID != "" || v.RuneUID != "") {
		return false
	}
	for _, input := range v.CubeInputs {
		if input.Count <= 0 || input.UID == "" && input.ItemID <= 0 || input.UID != "" && (!endgameDecimal(input.UID, false) || input.ItemID != 0 || input.Count != 1) {
			return false
		}
	}
	if len(v.ItemUIDs) > 10 || len(v.Capabilities) > 8 {
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
